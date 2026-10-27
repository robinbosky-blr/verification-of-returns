package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/robinbosky-blr/verification-of-returns/backend/internal/domain"
	"github.com/robinbosky-blr/verification-of-returns/backend/migrations"
	"sort"
	"time"
)

type Store struct{ Pool *pgxpool.Pool }

func ID() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func Open(ctx context.Context, url string) (*Store, error) {
	p, e := pgxpool.New(ctx, url)
	if e != nil {
		return nil, e
	}
	if e = p.Ping(ctx); e != nil {
		p.Close()
		return nil, e
	}
	s := &Store{p}
	return s, s.Migrate(ctx)
}
func (s *Store) Migrate(ctx context.Context) error {
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(839103)"); e != nil {
		return e
	}
	entries, e := migrations.Files.ReadDir(".")
	if e != nil {
		return e
	}
	names := []string{}
	for _, entry := range entries {
		if !entry.IsDir() && len(entry.Name()) > 4 && entry.Name()[len(entry.Name())-4:] == ".sql" {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		b, re := migrations.Files.ReadFile(name)
		if re != nil {
			return re
		}
		if _, re = tx.Exec(ctx, string(b)); re != nil {
			return fmt.Errorf("migration %s: %w", name, re)
		}
	}
	return tx.Commit(ctx)
}
func Audit(ctx context.Context, tx pgx.Tx, a string, data any) error {
	b, e := json.Marshal(data)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, "INSERT INTO audit_logs(action,details) VALUES($1,$2)", a, string(b))
	return e
}

type Import struct {
	ID       string    `json:"id"`
	Kind     string    `json:"kind"`
	Filename string    `json:"filename"`
	Hash     string    `json:"sha256"`
	Object   string    `json:"-"`
	Month    string    `json:"coverage_month"`
	Status   string    `json:"status"`
	Active   bool      `json:"active"`
	Rows     int       `json:"rows"`
	Error    string    `json:"error"`
	Created  time.Time `json:"created_at"`
}

const importCols = "id,kind,filename,sha256,object_key,coverage_month,status,active,rows_count,error,created_at"

func scanImport(row pgx.Row) (i Import, e error) {
	e = row.Scan(&i.ID, &i.Kind, &i.Filename, &i.Hash, &i.Object, &i.Month, &i.Status, &i.Active, &i.Rows, &i.Error, &i.Created)
	return
}
func (s *Store) CreateImport(ctx context.Context, i Import) (Import, bool, error) {
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return i, false, e
	}
	defer tx.Rollback(ctx)
	tag, e := tx.Exec(ctx, "INSERT INTO imports(id,kind,filename,sha256,object_key,coverage_month) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING", i.ID, i.Kind, i.Filename, i.Hash, i.Object, i.Month)
	if e != nil {
		return i, false, e
	}
	if tag.RowsAffected() == 0 {
		old, e := scanImport(tx.QueryRow(ctx, "SELECT "+importCols+" FROM imports WHERE kind=$1 AND sha256=$2 AND coverage_month=$3", i.Kind, i.Hash, i.Month))
		return old, true, e
	}
	if _, e = tx.Exec(ctx, "INSERT INTO jobs(import_id,kind) VALUES($1,'import')", i.ID); e != nil {
		return i, false, e
	}
	if e = Audit(ctx, tx, "import.queued", map[string]string{"id": i.ID, "kind": i.Kind, "file": i.Filename}); e != nil {
		return i, false, e
	}
	if e = tx.Commit(ctx); e != nil {
		return i, false, e
	}
	i.Status = "queued"
	i.Active = true
	return i, false, nil
}
func (s *Store) Imports(ctx context.Context) ([]Import, error) {
	rows, e := s.Pool.Query(ctx, "SELECT "+importCols+" FROM imports ORDER BY created_at DESC LIMIT 200")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Import{}
	for rows.Next() {
		i, e := scanImport(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, i)
	}
	return out, rows.Err()
}
func (s *Store) SetActive(ctx context.Context, id string, active bool) error {
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(839104)"); e != nil {
		return e
	}
	tag, e := tx.Exec(ctx, "UPDATE imports SET active=$2 WHERE id=$1 AND status='completed'", id, active)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("only completed imports can be changed")
	}
	if _, e = tx.Exec(ctx, "INSERT INTO jobs(import_id,kind) VALUES($1,'reconcile')", id); e != nil {
		return e
	}
	if e = Audit(ctx, tx, "import.activation", map[string]any{"id": id, "active": active}); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (s *Store) DeleteImport(ctx context.Context, id string) (Import, error) {
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return Import{}, e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(839104)"); e != nil {
		return Import{}, e
	}
	imported, e := scanImport(tx.QueryRow(ctx, "SELECT "+importCols+" FROM imports WHERE id=$1", id))
	if e != nil {
		return Import{}, e
	}
	if _, e = tx.Exec(ctx, "DELETE FROM jobs WHERE import_id=$1", id); e != nil {
		return Import{}, e
	}
	if _, e = tx.Exec(ctx, "DELETE FROM import_rows WHERE import_id=$1", id); e != nil {
		return Import{}, e
	}
	if _, e = tx.Exec(ctx, "DELETE FROM imports WHERE id=$1", id); e != nil {
		return Import{}, e
	}
	var objectReferences int
	if e = tx.QueryRow(ctx, "SELECT count(*) FROM imports WHERE object_key=$1", imported.Object).Scan(&objectReferences); e != nil {
		return Import{}, e
	}
	if objectReferences > 0 {
		imported.Object = ""
	}
	if _, e = tx.Exec(ctx, "INSERT INTO jobs(kind) VALUES('reconcile')"); e != nil {
		return Import{}, e
	}
	if e = Audit(ctx, tx, "import.deleted", map[string]any{"id": imported.ID, "kind": imported.Kind, "file": imported.Filename, "rows": imported.Rows}); e != nil {
		return Import{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return Import{}, e
	}
	return imported, nil
}
func (s *Store) Enqueue(ctx context.Context) error {
	_, e := s.Pool.Exec(ctx, "INSERT INTO jobs(kind) VALUES('reconcile')")
	return e
}

type Filter struct{ Month, Year, Search, Receipt, Financial, Claim, UserClaim, Status, SKU string }

type ClaimAction struct {
	CaseKey              string     `json:"case_key"`
	ClaimRaised          bool       `json:"claim_raised"`
	ClaimRaisedDate      *time.Time `json:"claim_raised_date"`
	ClaimStatus          string     `json:"claim_status"`
	ClaimUpdatedAt       time.Time  `json:"claim_updated_at"`
	AmazonClaimID        string     `json:"amazon_claim_id"`
	ClaimAmountRequested int64      `json:"claim_amount_requested"`
	ClaimAmountApproved  int64      `json:"claim_amount_approved"`
	ClaimRejectionReason string     `json:"claim_rejection_reason"`
	ClaimNotes           string     `json:"claim_notes"`
	UserNotes            string     `json:"user_notes"`
}

func validClaimStatus(v string) bool {
	switch v {
	case "Not Raised", "Pending", "Approved", "Rejected", "Partially Approved":
		return true
	}
	return false
}
func applyClaim(r *domain.Result, a ClaimAction) {
	r.ClaimRaised = a.ClaimRaised
	if a.ClaimRaisedDate != nil {
		r.ClaimRaisedDate = *a.ClaimRaisedDate
	}
	r.UserClaimStatus = a.ClaimStatus
	r.ClaimUpdatedDate = a.ClaimUpdatedAt
	r.AmazonClaimID = a.AmazonClaimID
	r.ClaimAmountRequested = a.ClaimAmountRequested
	r.ClaimAmountApproved = a.ClaimAmountApproved
	r.ClaimRejectionReason = a.ClaimRejectionReason
	r.ClaimNotes = a.ClaimNotes
	r.UserNotes = a.UserNotes
}
func (s *Store) claimActions(ctx context.Context, keys []string) (map[string]ClaimAction, error) {
	out := map[string]ClaimAction{}
	if len(keys) == 0 {
		return out, nil
	}
	rows, e := s.Pool.Query(ctx, `SELECT case_key,claim_raised,claim_raised_date,claim_status,claim_updated_at,amazon_claim_id,claim_amount_requested,claim_amount_approved,claim_rejection_reason,claim_notes,user_notes FROM claim_actions WHERE case_key=ANY($1)`, keys)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var a ClaimAction
		if e = rows.Scan(&a.CaseKey, &a.ClaimRaised, &a.ClaimRaisedDate, &a.ClaimStatus, &a.ClaimUpdatedAt, &a.AmazonClaimID, &a.ClaimAmountRequested, &a.ClaimAmountApproved, &a.ClaimRejectionReason, &a.ClaimNotes, &a.UserNotes); e != nil {
			return nil, e
		}
		out[a.CaseKey] = a
	}
	return out, rows.Err()
}
func (s *Store) UpdateClaim(ctx context.Context, a ClaimAction, changedBy string) (ClaimAction, error) {
	if a.CaseKey == "" {
		return a, fmt.Errorf("case key is required")
	}
	if !validClaimStatus(a.ClaimStatus) {
		return a, fmt.Errorf("invalid claim status")
	}
	if a.ClaimStatus != "Not Raised" {
		a.ClaimRaised = true
	}
	if a.ClaimStatus == "Not Raised" {
		a.ClaimRaised = false
		a.ClaimRaisedDate = nil
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return a, e
	}
	defer tx.Rollback(ctx)
	var exists bool
	if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM results WHERE case_key=$1)", a.CaseKey).Scan(&exists); e != nil || !exists {
		if e == nil {
			e = fmt.Errorf("case not found")
		}
		return a, e
	}
	var prev json.RawMessage = []byte(`{}`)
	_ = tx.QueryRow(ctx, `SELECT to_jsonb(c) FROM claim_actions c WHERE case_key=$1`, a.CaseKey).Scan(&prev)
	if a.ClaimRaised && a.ClaimRaisedDate == nil {
		d := time.Now()
		a.ClaimRaisedDate = &d
	}
	e = tx.QueryRow(ctx, `INSERT INTO claim_actions(case_key,claim_raised,claim_raised_date,claim_status,amazon_claim_id,claim_amount_requested,claim_amount_approved,claim_rejection_reason,claim_notes,user_notes) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(case_key) DO UPDATE SET claim_raised=EXCLUDED.claim_raised,claim_raised_date=EXCLUDED.claim_raised_date,claim_status=EXCLUDED.claim_status,claim_updated_at=now(),amazon_claim_id=EXCLUDED.amazon_claim_id,claim_amount_requested=EXCLUDED.claim_amount_requested,claim_amount_approved=EXCLUDED.claim_amount_approved,claim_rejection_reason=EXCLUDED.claim_rejection_reason,claim_notes=EXCLUDED.claim_notes,user_notes=EXCLUDED.user_notes RETURNING claim_updated_at`, a.CaseKey, a.ClaimRaised, a.ClaimRaisedDate, a.ClaimStatus, a.AmazonClaimID, a.ClaimAmountRequested, a.ClaimAmountApproved, a.ClaimRejectionReason, a.ClaimNotes, a.UserNotes).Scan(&a.ClaimUpdatedAt)
	if e != nil {
		return a, e
	}
	cur, _ := json.Marshal(a)
	if changedBy == "" {
		changedBy = "Admin"
	}
	if _, e = tx.Exec(ctx, `INSERT INTO claim_action_history(case_key,previous,current,changed_by) VALUES($1,$2,$3,$4)`, a.CaseKey, prev, string(cur), changedBy); e != nil {
		return a, e
	}
	if e = Audit(ctx, tx, "claim.updated", map[string]any{"case_key": a.CaseKey, "claim_status": a.ClaimStatus, "claim_raised": a.ClaimRaised}); e != nil {
		return a, e
	}
	return a, tx.Commit(ctx)
}

func (s *Store) ClaimHistory(ctx context.Context, caseKey string) ([]json.RawMessage, error) {
	if caseKey == "" {
		return nil, fmt.Errorf("case key is required")
	}
	rows, e := s.Pool.Query(ctx, `SELECT jsonb_build_object('id',id,'case_key',case_key,'previous',previous,'current',current,'changed_by',changed_by,'changed_at',changed_at) FROM claim_action_history WHERE case_key=$1 ORDER BY changed_at DESC,id DESC`, caseKey)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		out = append(out, json.RawMessage(b))
	}
	return out, rows.Err()
}

func (s *Store) Results(ctx context.Context, f Filter) ([]domain.Result, error) {
	rows, e := s.Pool.Query(ctx, `SELECT payload - 'evidence' FROM results WHERE ($1='' OR month=$1) AND ($2='' OR left(month,4)=$2) AND ($3='' OR strpos(lower(order_id||' '||sku),lower($3))>0) AND ($4='' OR receipt=$4) AND ($5='' OR financial=$5 OR ($5='Debited' AND financial IN ('Partially Reimbursed','Already Reimbursed'))) AND ($6='' OR payload->>'status'=$6) AND ($7='' OR sku=$7) ORDER BY month DESC,order_id,sku`, f.Month, f.Year, f.Search, f.Receipt, f.Financial, f.Status, f.SKU)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []domain.Result{}
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		var r domain.Result
		if e = json.Unmarshal(b, &r); e != nil {
			return nil, e
		}
		domain.RefreshClaim(&r, time.Now())
		out = append(out, r)
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	keys := make([]string, 0, len(out))
	for _, r := range out {
		keys = append(keys, r.Key)
	}
	actions, e := s.claimActions(ctx, keys)
	if e != nil {
		return nil, e
	}
	filtered := make([]domain.Result, 0, len(out))
	for _, r := range out {
		if a, ok := actions[r.Key]; ok {
			applyClaim(&r, a)
		} else {
			r.UserClaimStatus = "Not Raised"
		}
		if f.Claim != "" {
			if r.ClaimStatus != f.Claim || (r.UserClaimStatus != "" && r.UserClaimStatus != "Not Raised") {
				continue
			}
		}
		if f.UserClaim != "" && r.UserClaimStatus != f.UserClaim {
			continue
		}
		filtered = append(filtered, r)
	}
	return filtered, nil
}
func (s *Store) Detail(ctx context.Context, key string) (domain.Result, error) {
	var b []byte
	var r domain.Result
	e := s.Pool.QueryRow(ctx, "SELECT payload FROM results WHERE case_key=$1", key).Scan(&b)
	if e == nil {
		e = json.Unmarshal(b, &r)
		domain.RefreshClaim(&r, time.Now())
		if e == nil {
			if m, ce := s.claimActions(ctx, []string{key}); ce == nil {
				if a, ok := m[key]; ok {
					applyClaim(&r, a)
				} else {
					r.UserClaimStatus = "Not Raised"
				}
			} else {
				e = ce
			}
		}
	}
	return r, e
}
func (s *Store) JSONRows(ctx context.Context, q string) ([]json.RawMessage, error) {
	rows, e := s.Pool.Query(ctx, q)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		out = append(out, json.RawMessage(b))
	}
	return out, rows.Err()
}
