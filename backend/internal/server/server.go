package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/robinbosky-blr/verification-of-returns/backend/internal/domain"
	"github.com/robinbosky-blr/verification-of-returns/backend/internal/storage"
	"github.com/robinbosky-blr/verification-of-returns/backend/internal/store"
	"github.com/xuri/excelize/v2"
)

type Server struct {
	DB       *store.Store
	Files    storage.Store
	Password string
	Secure   bool
	mu       sync.Mutex
	attempts map[string][]time.Time
}

func marketplace(r *http.Request) string {
	value := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("marketplace")))
	if value == "" && r.Method == http.MethodPost { value = strings.ToLower(strings.TrimSpace(r.FormValue("marketplace"))) }
	if value == "" { return "amazon" }
	if value == "amazon" || value == "flipkart" { return value }
	return ""
}

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, map[string]string{"error": msg})
}
func (s *Server) Handler() http.Handler {
	s.attempts = map[string][]time.Time{}
	m := http.NewServeMux()
	m.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		if e := s.DB.Pool.Ping(r.Context()); e != nil {
			fail(w, 503, "Database unavailable")
			return
		}
		JSON(w, 200, map[string]string{"status": "ok"})
	})
	m.HandleFunc("POST /api/login", s.login)
	m.HandleFunc("GET /api/session", func(w http.ResponseWriter, r *http.Request) { JSON(w, 200, map[string]string{"user": "Admin"}) })
	m.HandleFunc("POST /api/logout", func(w http.ResponseWriter, r *http.Request) {
		if c, e := r.Cookie("returns_session"); e == nil {
			h := sha256.Sum256([]byte(c.Value))
			s.DB.Pool.Exec(r.Context(), "DELETE FROM sessions WHERE hash=$1", hex.EncodeToString(h[:]))
		}
		http.SetCookie(w, &http.Cookie{Name: "returns_session", Path: "/", HttpOnly: true, Secure: s.Secure, SameSite: http.SameSiteStrictMode, MaxAge: -1})
		JSON(w, 200, map[string]bool{"ok": true})
	})
	m.HandleFunc("GET /api/imports", func(w http.ResponseWriter, r *http.Request) {
		market := marketplace(r); if market == "" { fail(w, 400, "Unsupported marketplace"); return }
		v, e := s.DB.Imports(r.Context(), market)
		if e != nil {
			fail(w, 500, "Could not load imports")
			return
		}
		JSON(w, 200, v)
	})
	m.HandleFunc("POST /api/imports", s.upload)
	m.HandleFunc("POST /api/imports/{id}/active", func(w http.ResponseWriter, r *http.Request) {
		market := marketplace(r); if market == "" { fail(w, 400, "Unsupported marketplace"); return }
		var b struct {
			Active bool `json:"active"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&b) != nil {
			fail(w, 400, "Invalid request")
			return
		}
		if e := s.DB.SetActive(r.Context(), market, r.PathValue("id"), b.Active); e != nil {
			fail(w, 400, e.Error())
			return
		}
		JSON(w, 202, map[string]string{"status": "queued"})
	})
	m.HandleFunc("DELETE /api/imports/{id}", func(w http.ResponseWriter, r *http.Request) {
		market := marketplace(r); if market == "" { fail(w, 400, "Unsupported marketplace"); return }
		imported, e := s.DB.DeleteImport(r.Context(), market, r.PathValue("id"))
		if errors.Is(e, pgx.ErrNoRows) {
			fail(w, 404, "Import not found")
			return
		}
		if e != nil {
			slog.Error("delete import", "error", e)
			fail(w, 500, "Could not delete import")
			return
		}
		if s.Files != nil && imported.Object != "" {
			if e = s.Files.Delete(r.Context(), imported.Object); e != nil {
				slog.Warn("delete import object", "import", imported.ID, "error", e)
			}
		}
		JSON(w, 202, map[string]string{"status": "queued"})
	})
	m.HandleFunc("POST /api/reconcile", func(w http.ResponseWriter, r *http.Request) {
		market := marketplace(r); if market == "" { fail(w, 400, "Unsupported marketplace"); return }
		if e := s.DB.Enqueue(r.Context(), market); e != nil {
			fail(w, 500, "Could not queue reconciliation")
			return
		}
		JSON(w, 202, map[string]string{"status": "queued"})
	})
	m.HandleFunc("GET /api/results", s.results)
	m.HandleFunc("GET /api/case", func(w http.ResponseWriter, r *http.Request) {
		market := marketplace(r); if market == "" { fail(w, 400, "Unsupported marketplace"); return }
		v, e := s.DB.Detail(r.Context(), market, r.URL.Query().Get("key"))
		if errors.Is(e, pgx.ErrNoRows) {
			fail(w, 404, "Case not found")
			return
		}
		if e != nil {
			fail(w, 500, "Could not load case")
			return
		}
		JSON(w, 200, v)
	})
	m.HandleFunc("PUT /api/case/claim", func(w http.ResponseWriter, r *http.Request) {
		var a store.ClaimAction
		if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&a); e != nil {
			fail(w, 400, "Invalid claim update")
			return
		}
		a.Marketplace = marketplace(r); if a.Marketplace == "" { fail(w, 400, "Unsupported marketplace"); return }
		v, e := s.DB.UpdateClaim(r.Context(), a, "Admin")
		if e != nil {
			fail(w, 400, e.Error())
			return
		}
		JSON(w, 200, v)
	})
	m.HandleFunc("GET /api/case/claim-history", func(w http.ResponseWriter, r *http.Request) {
		market := marketplace(r); if market == "" { fail(w, 400, "Unsupported marketplace"); return }
		v, e := s.DB.ClaimHistory(r.Context(), market, r.URL.Query().Get("key"))
		if e != nil {
			fail(w, 400, e.Error())
			return
		}
		JSON(w, 200, v)
	})
	m.HandleFunc("GET /api/export", s.export)
	m.HandleFunc("GET /api/meta", func(w http.ResponseWriter, r *http.Request) {
		market := marketplace(r); if market == "" { fail(w, 400, "Unsupported marketplace"); return }
		v, e := s.DB.JSONRows(r.Context(), fmt.Sprintf(`SELECT jsonb_build_object('month',month,'count',count(*)) FROM results WHERE marketplace='%s' GROUP BY month ORDER BY month DESC`, market))
		if e != nil {
			fail(w, 500, "Could not load months")
			return
		}
		jobs, e := s.DB.JSONRows(r.Context(), fmt.Sprintf(`SELECT to_jsonb(j) FROM (SELECT id,kind,status,error,created_at,finished_at FROM jobs WHERE marketplace='%s' ORDER BY id DESC LIMIT 25) j`, market))
		if e != nil {
			fail(w, 500, "Could not load jobs")
			return
		}
		rule := domain.RuleVersion; if market == "flipkart" { rule = domain.FlipkartRuleVersion }
		JSON(w, 200, map[string]any{"months": v, "jobs": jobs, "rule_version": rule})
	})
	m.HandleFunc("GET /api/audit", func(w http.ResponseWriter, r *http.Request) {
		market := marketplace(r); if market == "" { fail(w, 400, "Unsupported marketplace"); return }
		v, e := s.DB.JSONRows(r.Context(), fmt.Sprintf("SELECT to_jsonb(a) FROM (SELECT * FROM audit_logs WHERE marketplace='%s' ORDER BY id DESC LIMIT 200) a", market))
		if e != nil {
			fail(w, 500, "Could not load audit log")
			return
		}
		JSON(w, 200, v)
	})
	m.HandleFunc("GET /api/rules", func(w http.ResponseWriter, r *http.Request) {
		market := marketplace(r); if market == "" { fail(w, 400, "Unsupported marketplace"); return }
		v, e := s.DB.JSONRows(r.Context(), fmt.Sprintf("SELECT to_jsonb(c) FROM claim_rules c WHERE marketplace='%s' ORDER BY created_at DESC", market))
		if e != nil {
			fail(w, 500, "Could not load rules")
			return
		}
		JSON(w, 200, v)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != "GET" && r.Method != "HEAD" {
			if origin := r.Header.Get("Origin"); origin != "" {
				u, e := url.Parse(origin)
				if e != nil || u.Host != r.Host {
					fail(w, 403, "Request origin rejected")
					return
				}
			}
		}
		if r.URL.Path != "/api/login" && r.URL.Path != "/api/health" {
			c, e := r.Cookie("returns_session")
			if e != nil {
				fail(w, 401, "Please sign in")
				return
			}
			h := sha256.Sum256([]byte(c.Value))
			var ok bool
			e = s.DB.Pool.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM sessions WHERE hash=$1 AND expires_at>now())", hex.EncodeToString(h[:])).Scan(&ok)
			if e != nil || !ok {
				fail(w, 401, "Session expired; sign in again")
				return
			}
		}
		m.ServeHTTP(w, r)
	})
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	now := time.Now()
	s.mu.Lock()
	recent := []time.Time{}
	for _, t := range s.attempts[ip] {
		if now.Sub(t) < 15*time.Minute {
			recent = append(recent, t)
		}
	}
	if len(recent) >= 10 {
		s.mu.Unlock()
		fail(w, 429, "Too many sign-in attempts; try again in 15 minutes")
		return
	}
	s.attempts[ip] = append(recent, now)
	s.mu.Unlock()
	var b struct {
		Password string `json:"password"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&b) != nil {
		fail(w, 400, "Invalid request")
		return
	}
	a := sha256.Sum256([]byte(b.Password))
	expected := sha256.Sum256([]byte(s.Password))
	if subtle.ConstantTimeCompare(a[:], expected[:]) != 1 {
		fail(w, 401, "Incorrect password")
		return
	}
	token := store.ID() + store.ID()
	h := sha256.Sum256([]byte(token))
	_, e := s.DB.Pool.Exec(r.Context(), "INSERT INTO sessions(hash,expires_at) VALUES($1,$2)", hex.EncodeToString(h[:]), time.Now().Add(12*time.Hour))
	if e != nil {
		fail(w, 500, "Could not create session")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "returns_session", Value: token, Path: "/", HttpOnly: true, Secure: s.Secure, SameSite: http.SameSiteStrictMode, MaxAge: 43200})
	JSON(w, 200, map[string]string{"user": "Admin"})
}
func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 101<<20)
	if e := r.ParseMultipartForm(2 << 20); e != nil {
		fail(w, 400, "Upload must be below 100 MB")
		return
	}
	defer r.MultipartForm.RemoveAll()
	market := marketplace(r)
	if market == "" {
		fail(w, 400, "Choose Amazon or Flipkart")
		return
	}
	kind := r.FormValue("kind")
	if kind != "website" && kind != "physical" && kind != "payment" {
		fail(w, 400, "Choose website, physical, or payment")
		return
	}
	if m := r.FormValue("month"); m != "" {
		if _, e := time.Parse("2006-01", m); e != nil {
			fail(w, 400, "Coverage month must be YYYY-MM")
			return
		}
	}
	f, h, e := r.FormFile("file")
	if e != nil {
		fail(w, 400, "Choose a CSV, XLSX, or ZIP file")
		return
	}
	defer f.Close()
	ext := strings.ToLower(filepath.Ext(h.Filename))
	if ext != ".csv" && ext != ".xlsx" && ext != ".zip" {
		fail(w, 400, "Only CSV, XLSX, and ZIP files are supported. PDF, Word, image, video, and other file types are rejected")
		return
	}
	tmp, e := os.CreateTemp("", "returns-upload-*")
	if e != nil {
		fail(w, 500, "Could not stage upload")
		return
	}
	defer os.Remove(tmp.Name())
	hash := sha256.New()
	n, e := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(f, (100<<20)+1))
	tmp.Close()
	if e != nil || n > 100<<20 {
		fail(w, 400, "Upload exceeds 100 MB")
		return
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	key := market + "/" + kind + "/" + digest + ext
	if e = s.Files.Put(r.Context(), key, tmp.Name()); e != nil {
		slog.Error("store upload", "error", e)
		fail(w, 500, "Could not preserve original file")
		return
	}
	imp, duplicate, e := s.DB.CreateImport(r.Context(), store.Import{ID: store.ID(), Marketplace: market, Kind: kind, Filename: filepath.Base(h.Filename), Hash: digest, Object: key, Month: r.FormValue("month")})
	if e != nil {
		slog.Error("queue upload", "error", e)
		fail(w, 500, "Could not queue import")
		return
	}
	JSON(w, 202, map[string]any{"import": imp, "duplicate": duplicate})
}
func filter(r *http.Request) store.Filter {
	q := r.URL.Query()
	return store.Filter{Marketplace: marketplace(r), Month: q.Get("month"), Year: q.Get("year"), Search: q.Get("search"), Receipt: q.Get("receipt"), Financial: q.Get("financial"), Claim: q.Get("claim"), UserClaim: q.Get("claim_status"), Status: q.Get("status"), SKU: q.Get("sku")}
}
func (s *Server) results(w http.ResponseWriter, r *http.Request) {
	f := filter(r)
	if f.Marketplace == "" { fail(w, 400, "Unsupported marketplace"); return }
	rs, e := s.DB.Results(r.Context(), f)
	if e != nil {
		slog.Error("results", "error", e)
		fail(w, 500, "Could not load results")
		return
	}
	summary := domain.Summarize(rs)
	monthly := map[string][]domain.Result{}
	for _, v := range rs {
		monthly[v.Month] = append(monthly[v.Month], v)
	}
	byMonth := map[string]domain.Summary{}
	for k, v := range monthly {
		byMonth[k] = domain.Summarize(v)
	}
	p, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if p < 1 {
		p = 1
	}
	size, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if size < 1 || size > 100 {
		size = 50
	}
	start := min((p-1)*size, len(rs))
	end := min(start+size, len(rs))
	JSON(w, 200, map[string]any{"rows": rs[start:end], "total": len(rs), "summary": summary, "monthly": byMonth, "page": p, "limit": size})
}
func safeCell(s string) string {
	if len(s) > 0 && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}
func date(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return domain.Day(t).Format("2006-01-02")
}
func amount(n int64) string {
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}
	return fmt.Sprintf("%s%d.%02d", sign, n/100, n%100)
}
func (s *Server) export(w http.ResponseWriter, r *http.Request) {
	f := filter(r)
	if f.Marketplace == "" { fail(w, 400, "Unsupported marketplace"); return }
	rs, e := s.DB.Results(r.Context(), f)
	if e != nil {
		fail(w, 500, "Could not export results")
		return
	}
	recoveryHeader := "Product Reimbursement (INR)"
	claimIDHeader := "Amazon Claim ID"
	if f.Marketplace == "flipkart" { recoveryHeader = "SPF Reimbursement (INR)"; claimIDHeader = "Flipkart SPF Claim ID" }
	headers := []string{"Order ID", "SKU", "Return Month", "Return Status", "Physical Status", "Received Month", "Financial Status", "Payment Scenario", "Refund Date", "Refund Total (INR)", "Fee Refund (INR)", recoveryHeader, "Remaining Loss (INR)", "Claim Rule", "Claim Opens", "Claim Deadline", "System Claim Action", "Claim Raised", "User Claim Status", "Claim Raised Date", "Claim Updated Date", claimIDHeader, "Requested Amount (INR)", "Approved Amount (INR)", "Rejection Reason", "Claim Notes", "User Notes", "Rule Version"}
	data := [][]string{headers}
	for _, v := range rs {
		data = append(data, []string{safeCell(v.OrderID), safeCell(v.SKU), v.Month, safeCell(v.Status), v.Receipt, v.ReceivedMonth, v.Financial, v.Scenario, date(v.RefundDate), amount(v.RefundTotal), amount(v.FeeRefund), amount(v.Reimbursement), amount(v.Loss), v.ClaimRule, date(v.ClaimOpen), date(v.ClaimDeadline), v.ClaimStatus, strconv.FormatBool(v.ClaimRaised), v.UserClaimStatus, date(v.ClaimRaisedDate), date(v.ClaimUpdatedDate), safeCell(v.AmazonClaimID), amount(v.ClaimAmountRequested), amount(v.ClaimAmountApproved), safeCell(v.ClaimRejectionReason), safeCell(v.ClaimNotes), safeCell(v.UserNotes), v.RuleVersion})
	}
	if r.URL.Query().Get("format") == "xlsx" {
		f := excelize.NewFile()
		defer f.Close()
		f.SetSheetName("Sheet1", "Returns")
		sw, e := f.NewStreamWriter("Returns")
		if e != nil {
			fail(w, 500, "Could not create workbook")
			return
		}
		for i, row := range data {
			cells := make([]any, len(row))
			for j, v := range row {
				cells[j] = v
			}
			cell, _ := excelize.CoordinatesToCellName(1, i+1)
			if e = sw.SetRow(cell, cells); e != nil {
				fail(w, 500, "Could not write workbook")
				return
			}
		}
		if e = sw.Flush(); e != nil {
			fail(w, 500, "Could not finish workbook")
			return
		}
		f.NewSheet("Summary")
		summary := domain.Summarize(rs)
		summaryRows := [][]any{
			{"Metric", "Value", "Unit"},
			{"Total return cases", summary.Total, "cases"},
			{"Received", summary.Received, "cases"},
			{"Not Received", summary.NotReceived, "cases"},
			{"Debited", summary.Debited, "cases"},
			{"Not Debited", summary.NotDebited, "cases"},
			{"Payment missing / review", summary.Unknown, "cases"},
			{"Already Reimbursed", summary.Reimbursed, "cases"},
			{"Partially Reimbursed", summary.Partial, "cases"},
			{"Raise Claim", summary.Raise, "cases"},
			{"Scheduled", summary.Scheduled, "cases"},
			{"Expired", summary.Expired, "cases"},
			{"Refund Debited", amount(summary.RefundDebited), "INR"},
			{strings.TrimSuffix(recoveryHeader, " (INR)"), amount(summary.Reimbursement), "INR"},
			{"Fee Refund", amount(summary.FeeRefund), "INR"},
			{"Recovered product amount", amount(summary.Recovered), "INR"},
			{"Remaining Product Loss", amount(summary.Loss), "INR"},
		}
		for i, row := range summaryRows {
			cell, _ := excelize.CoordinatesToCellName(1, i+1)
			if e = f.SetSheetRow("Summary", cell, &row); e != nil {
				fail(w, 500, "Could not write summary")
				return
			}
		}
		reportSheets := []string{"Received", "Not Received", "Debited", "Not Debited", "Reimbursed", "Claims"}
		for _, sheet := range reportSheets {
			f.NewSheet(sheet)
			stream, err := f.NewStreamWriter(sheet)
			if err != nil {
				fail(w, 500, "Could not create report")
				return
			}
			headerCells := make([]any, len(headers))
			for j, v := range headers {
				headerCells[j] = v
			}
			stream.SetRow("A1", headerCells)
			line := 2
			for i, v := range rs {
				include := false
				switch sheet {
				case "Received", "Not Received":
					include = v.Receipt == sheet
				case "Debited":
					include = v.Receipt == "Not Received" && (v.Financial == "Debited" || v.Financial == "Already Reimbursed" || v.Financial == "Partially Reimbursed")
				case "Not Debited":
					include = v.Financial == sheet
				case "Reimbursed":
					include = v.Financial == "Already Reimbursed" || v.Financial == "Partially Reimbursed"
				case "Claims":
					include = v.ClaimStatus == "Raise Claim" || v.ClaimStatus == "Scheduled" || v.ClaimStatus == "Expired"
				}
				if !include {
					continue
				}
				cells := make([]any, len(data[i+1]))
				for j, value := range data[i+1] {
					cells[j] = value
				}
				cell, _ := excelize.CoordinatesToCellName(1, line)
				if e = stream.SetRow(cell, cells); e != nil {
					fail(w, 500, "Could not write report")
					return
				}
				line++
			}
			if e = stream.Flush(); e != nil {
				fail(w, 500, "Could not finish report")
				return
			}
		}
		for _, sheet := range f.GetSheetList() {
			f.SetColWidth(sheet, "A", "AB", 23)
			f.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
		}
		index, _ := f.GetSheetIndex("Summary")
		f.SetActiveSheet(index)
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-returns.xlsx"`, f.Marketplace))
		if e = f.Write(w); e != nil {
			slog.Error("xlsx response", "error", e)
		}
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-returns.csv"`, f.Marketplace))
	c := csv.NewWriter(w)
	c.WriteAll(data)
}
