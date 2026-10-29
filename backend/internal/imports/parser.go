package imports

import (
	"archive/zip"
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/robinbosky-blr/verification-of-returns/backend/internal/domain"
	"github.com/xuri/excelize/v2"
)

type Options struct{ Marketplace, Kind, Filename, Month string }
type Stats struct {
	Rows   int `json:"rows"`
	Sheets int `json:"sheets"`
}

var headerNoise = regexp.MustCompile(`[^a-z0-9]`)

func header(s string) string {
	return headerNoise.ReplaceAllString(strings.ToLower(strings.TrimPrefix(s, "\ufeff")), "")
}

// uniqueSourceHeaders preserves every original source column in Row.Raw. Empty
// headers are ignored and duplicate labels receive a stable suffix.
func uniqueSourceHeaders(row []string) []string {
	out := make([]string, len(row))
	seen := map[string]int{}
	for i, value := range row {
		name := strings.TrimSpace(strings.TrimPrefix(value, "\ufeff"))
		if name == "" {
			continue
		}
		seen[name]++
		out[i] = name
		if seen[name] > 1 {
			out[i] = fmt.Sprintf("%s #%d", name, seen[name])
		}
	}
	return out
}

var aliases = map[string][]string{
	"order":     {"customerorderid", "orderid", "amazonorderid", "merchantorderid", "orderno", "ordernumber"},
	"sku":       {"msku", "sku", "sellersku", "merchantsku"},
	"date":      {"returnrequesteddate", "returnrequestdate", "returncreationdate", "returninitiateddate", "returnrequestedon", "returncreationtime", "returncreatedon", "returndate", "carrierpickupdate", "pickupdate", "receiveddate", "receivedon", "dateofreceived", "physicalreceiveddate", "physicallyreceiveddate", "date", "datetime", "transactiondate", "transactionposteddate", "posteddate", "postedon"},
	"updated":   {"lastupdatedon", "lastupdateddate", "lastupdated", "updatedat", "statusupdatedon"},
	"delivered": {"returndelivereddate", "delivereddate", "deliveredtosellerdate"},
	"status":    {"returnstatus", "shipmentstatus", "status"},
	"type":      {"type", "transactiontype", "paymenttype"},
	"amount":    {"total", "totalinr", "totalamount", "amount", "transactionamount", "transactiontotal", "netamount", "netproceeds", "nettotal"},
	"quantity":  {"quantity", "qty", "returnquantity"},
	"channel":   {"fulfilment", "fulfillment", "fulfilmentchannel", "fulfillmentchannel", "returntype"},
	"reason":    {"returnreason", "reason", "subreason", "customerreturnreason", "returnsubreason"},
	"reference": {"transactionid", "amazontransactionid", "referenceid", "transactionreference", "transactionreferenceid", "settlementid", "settlementidentifier"},
	"approval": {"returnapprovaldate"},
	"completed": {"completeddate"},
	"out_for_delivery": {"outfordeliverydate"},
	"sale": {"saleamountrs"},
	"refund": {"refundrs"},
	"protection": {"protectionfundrs"},
	"marketplace_fee": {"marketplacefeerssumvai", "marketplacefeers"},
}

var flipkartSourceAliases = map[string]map[string][]string{
	"website": {
		// Order Item ID, Return ID, Shipment ID and FSN are verification fields.
		// They must never be silently promoted to the primary Order ID or SKU.
		"order": {"orderid"},
		"sku": {"sku", "sellersku", "merchantsku"},
		"date": {"returncreationdate", "returnrequesteddate", "returnrequestdate", "returndate", "createddate"},
		"updated": {"lastupdateddate", "lastupdatedon", "statusupdateddate", "updateddate"},
		"approval": {"returnapprovaldate"},
		"completed": {"completeddate"},
		"out_for_delivery": {"outfordeliverydate"},
		"delivered": {"returndelivereddate", "delivereddatetoseller", "completeddate"},
		"status": {"returnstatus", "status", "shipmentstatus"},
		"channel": {"returntype", "deliverytype", "fulfilmenttype"},
		"reason": {"returnreason", "returnsubreason", "reason", "subreason", "customerreturnreason"},
		"reference": {"orderitemid"},
	},
	"physical": {
		"order": {"orderid"},
		"sku": {"sku", "sellersku"},
		"date": {"receiveddate", "physicalreceiveddate", "returndate", "date"},
		"reference": {"orderitemid"},
	},
	"payment": {
		"order": {"orderid"},
		"sku": {"sku", "sellersku"},
		"date": {"paymentdate", "transactiondate", "settlementdate", "date"},
		"type": {"transactiontype", "paymenttype", "type", "description"},
		"amount": {"amount", "totalamount", "netamount", "settlementvalue", "total"},
		"reference": {"orderitemid", "transactionid", "referenceid", "settlementid", "claimid"},
		"sale": {"saleamountrs"},
		"refund": {"refundrs"},
		"protection": {"protectionfundrs"},
		"marketplace_fee": {"marketplacefeerssumvai", "marketplacefeers"},
	},
}

var sourceAliases = map[string]map[string][]string{
	"website": {
		"order": {"customerorderid", "amazonorderid", "orderid", "merchantorderid", "orderno", "ordernumber"},
		// Amazon's website export contains both SKU and mSKU. Reconciliation must use mSKU.
		"sku":  {"msku", "merchantsku", "sellersku"},
		"date": {"returncreationdate", "returncreatedon", "returncreationtime", "carrierpickupdate", "pickupdate"},
	},
	"physical": {
		"order": {"orderid", "customerorderid", "amazonorderid", "merchantorderid", "orderno", "ordernumber"},
		"sku":   {"sku", "msku", "sellersku", "merchantsku"},
		"date":  {"returndate", "receiveddate", "receivedon", "dateofreceived", "physicalreceiveddate", "physicallyreceiveddate", "date"},
	},
	"payment": {
		"order": {"orderid", "amazonorderid", "customerorderid", "merchantorderid", "orderno", "ordernumber"},
		"sku":   {"sku", "msku", "sellersku", "merchantsku"},
		"date":  {"datetime", "transactiondate", "transactionposteddate", "posteddate", "postedon", "date"},
	},
}

var fieldOrder = []string{"order", "sku", "date", "updated", "approval", "completed", "out_for_delivery", "delivered", "status", "type", "amount", "sale", "refund", "protection", "marketplace_fee", "quantity", "channel", "reason", "reference"}

func fields(row []string, kind, marketplace string) map[string]int {
	m := map[string]int{}
	headers := make([]string, len(row))
	for i, s := range row {
		headers[i] = header(s)
	}
	for _, field := range fieldOrder {
		candidates := aliases[field]
		configuredAliases := sourceAliases
		if strings.EqualFold(marketplace, "flipkart") {
			configuredAliases = flipkartSourceAliases
		}
		if configured, ok := configuredAliases[kind][field]; ok {
			candidates = configured
		}
		for _, candidate := range candidates {
			for i, value := range headers {
				if value == candidate {
					m[field] = i
					break
				}
			}
			if _, ok := m[field]; ok {
				break
			}
		}
	}
	return m
}
func validHeader(m map[string]int, kind, marketplace string) bool {
	if _, ok := m["order"]; !ok {
		return false
	}
	if _, ok := m["sku"]; kind == "website" && !ok {
		return false
	}
	if _, ok := m["sku"]; kind == "physical" && !strings.EqualFold(marketplace, "flipkart") && !ok { return false }
	required := []string{}
	switch kind {
	case "website":
		if strings.EqualFold(marketplace, "flipkart") { required = []string{"date", "status"} } else { required = []string{"updated", "status"} }
	case "payment":
		if strings.EqualFold(marketplace, "flipkart") {
			if _, ok := m["date"]; !ok { return false }
			_, hasType := m["type"]; _, hasAmount := m["amount"]
			_, hasSale := m["sale"]; _, hasRefund := m["refund"]; _, hasProtection := m["protection"]
			return (hasType && hasAmount) || hasSale || hasRefund || hasProtection
		}
		required = []string{"date", "type", "amount"}
	}
	for _, k := range required {
		if _, ok := m[k]; !ok {
			return false
		}
	}
	return true
}
func ParseDate(s string) (time.Time, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\u00a0", " "))
	if s == "" {
		return time.Time{}, fmt.Errorf("date is missing")
	}
	s = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(s, " IST"), " UTC"), " GMT+05:30")
	for _, f := range []string{time.RFC3339, "2006-01-02 15:04:05-07:00", "2006-01-02 15:04:05", "2006-01-02", "02-Jan-2006", "2-Jan-2006", "2 January, 2006", "02 January, 2006", "2 Jan, 2006", "02 Jan, 2006", "02 Jan 2006 15:04:05", "2 Jan 2006 15:04:05", "02-Jan-2006 15:04:05", "2 Jan 2006 3:04:05 PM", "2 Jan 2006 3:04:05 pm", "02/01/2006 15:04:05", "02/01/2006", "2/1/2006", "02.01.2006 15:04:05", "2.1.2006 15:04:05", "02.01.2006", "2.1.2006", "02-01-2006", "2-1-2006", "01/02/06", "Jan 2, 2006 3:04:05 PM"} {
		if t, e := time.ParseInLocation(f, s, domain.IST); e == nil {
			return t, nil
		}
	}
	if n, e := strconv.ParseFloat(s, 64); e == nil && n > 30000 && n < 100000 {
		t, e := excelize.ExcelDateToTime(n, false)
		if e == nil {
			y, m, d := t.Date()
			return time.Date(y, m, d, t.Hour(), t.Minute(), t.Second(), 0, domain.IST), nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized date %q; use DD/MM/YYYY or ISO", s)
}
func sheetMonth(sheet, fallback string) (time.Time, error) {
	if t, e := time.ParseInLocation("2006-01", sheet, domain.IST); e == nil {
		return t, nil
	}
	for _, f := range []string{"January 2006", "Jan 2006", "Jan-2006", "January-2006", "January_2006", "Jan_2006", "01-2006", "01_2006"} {
		if t, e := time.ParseInLocation(f, strings.TrimSpace(sheet), domain.IST); e == nil {
			return t, nil
		}
	}
	year := time.Now().In(domain.IST).Year()
	if fallback != "" {
		t, e := time.ParseInLocation("2006-01", fallback, domain.IST)
		if e != nil {
			return t, e
		}
		year = t.Year()
		for m := time.January; m <= time.December; m++ {
			if strings.EqualFold(sheet, m.String()) || strings.EqualFold(sheet, m.String()[:3]) {
				return time.Date(year, m, 1, 0, 0, 0, 0, domain.IST), nil
			}
		}
		return t, nil
	}
	for m := time.January; m <= time.December; m++ {
		if strings.EqualFold(sheet, m.String()) || strings.EqualFold(sheet, m.String()[:3]) {
			return time.Date(year, m, 1, 0, 0, 0, 0, domain.IST), nil
		}
	}
	return time.Time{}, fmt.Errorf("physical receipt date missing; select coverage month or name sheets YYYY-MM")
}
func Parse(path string, o Options, emit func(domain.Row) error) (Stats, error) {
	var st Stats
	parseSheet := func(name string, optional bool, next func() ([]string, error)) error {
		var cols map[string]int
		var sourceHeaders []string
		var previous string
		line := 0
		sawContent := false
		for {
			cells, e := next()
			if e == io.EOF {
				break
			}
			line++
			if e != nil {
				return fmt.Errorf("%s row %d: %w", name, line, e)
			}
			nonempty := false
			for _, v := range cells {
				if strings.TrimSpace(v) != "" {
					nonempty = true
					break
				}
			}
			if !nonempty {
				previous = ""
				continue
			}
			sawContent = true
			possible := fields(cells, o.Kind, o.Marketplace)
			if validHeader(possible, o.Kind, o.Marketplace) {
				cols = possible
				sourceHeaders = uniqueSourceHeaders(cells)
				previous = ""
				continue
			}
			if cols == nil {
				if line > 50 {
					if optional { return nil }
					return fmt.Errorf("%s: required %s headers not found in first 50 rows", name, o.Kind)
				}
				continue
			}
			get := func(k string) string {
				if i, ok := cols[k]; ok && i < len(cells) {
					return strings.TrimSpace(cells[i])
				}
				return ""
			}
			order := domain.Normalize(get("order"))
			sku := domain.Normalize(get("sku"))
			if order == "" && sku == "" {
				previous = ""
				continue
			}
			if o.Kind == "physical" {
				if order != "" {
					previous = order
				} else {
					order = previous
				}
			}
			if order == "" && o.Kind == "payment" {
				continue
			}
			if order == "" {
				return fmt.Errorf("%s row %d: Order ID missing; no previous Order ID in this section", name, line)
			}
			if sku == "" && o.Kind != "payment" && !(strings.EqualFold(o.Marketplace, "flipkart") && o.Kind == "physical") {
				return fmt.Errorf("%s row %d: SKU is missing", name, line)
			}
			r := domain.Row{Marketplace: o.Marketplace, Kind: o.Kind, OrderID: order, SKU: sku, Status: get("status"), Reason: get("reason"), Type: get("type"), Channel: get("channel"), ReferenceID: domain.Normalize(get("reference")), Quantity: 1, Source: name, Line: line, Raw: map[string]string{}}
			for i, key := range sourceHeaders {
				if key != "" && i < len(cells) {
					r.Raw[key] = cells[i]
				}
			}
			for k, i := range cols {
				if i < len(cells) {
					r.Raw["canonical."+k] = cells[i]
				}
			}
			if q := get("quantity"); q != "" {
				v, e := strconv.ParseInt(q, 10, 64)
				if e != nil || v < 0 {
					return fmt.Errorf("%s row %d: invalid quantity", name, line)
				}
				r.Quantity = v
			}
			if o.Kind == "website" {
				updatedValue := get("updated")
				if strings.EqualFold(o.Marketplace, "flipkart") {
					for _, candidate := range []string{get("completed"), get("out_for_delivery"), get("approval"), get("date")} {
						if candidate != "" { updatedValue = candidate; break }
					}
				}
				r.Updated, e = ParseDate(updatedValue)
				if e != nil {
					return fmt.Errorf("%s row %d updated date: %w", name, line, e)
				}
				if get("date") == "" {
					r.Date = r.Updated
				} else {
					r.Date, e = ParseDate(get("date"))
					if e != nil {
						return fmt.Errorf("%s row %d return date: %w", name, line, e)
					}
				}
				if r.Status == "" {
					return fmt.Errorf("%s row %d: status missing", name, line)
				}
			} else {
				if get("date") == "" && o.Kind == "physical" {
					r.Date, e = sheetMonth(name, o.Month)
				} else {
					r.Date, e = ParseDate(get("date"))
				}
				if e != nil {
					return fmt.Errorf("%s row %d: %w", name, line, e)
				}
				r.Updated = r.Date
			}
			if d := get("delivered"); d != "" {
				r.Delivered, e = ParseDate(d)
				if e != nil {
					return e
				}
			}
			if o.Kind == "payment" && strings.EqualFold(o.Marketplace, "flipkart") {
				_, hasSale := cols["sale"]
				_, hasRefund := cols["refund"]
				_, hasProtection := cols["protection"]
				if hasSale || hasRefund || hasProtection {
					emittedProduct := false
					for _, transaction := range []struct{ Type, Field string }{{"SALE", "sale"}, {"REFUND", "refund"}, {"SPF REIMBURSEMENT", "protection"}} {
						amount, parseErr := domain.Money(get(transaction.Field))
						if parseErr != nil {
							return fmt.Errorf("%s row %d %s: %w", name, line, transaction.Field, parseErr)
						}
						if amount == 0 {
							continue
						}
						payment := r
						payment.Type = transaction.Type
						payment.Amount = amount
						if e = emit(payment); e != nil {
							return e
						}
						st.Rows++
						emittedProduct = true
					}
					if !emittedProduct && hasRefund && strings.TrimSpace(get("refund")) != "" {
						payment := r
						payment.Type = "REFUND"
						if e = emit(payment); e != nil {
							return e
						}
						st.Rows++
					}
					fee, parseErr := domain.Money(get("marketplace_fee"))
					if parseErr != nil {
						return fmt.Errorf("%s row %d marketplace fee: %w", name, line, parseErr)
					}
					if fee > 0 {
						payment := r
						payment.Type = "FEE REFUND"
						payment.Amount = fee
						if e = emit(payment); e != nil {
							return e
						}
						st.Rows++
					}
					continue
				}
			}
			if o.Kind == "payment" {
				r.Amount, e = domain.Money(get("amount"))
				if e != nil {
					return fmt.Errorf("%s row %d amount: %w", name, line, e)
				}
				if r.Type == "" {
					return fmt.Errorf("%s row %d: payment type missing", name, line)
				}
			}
			if e = emit(r); e != nil {
				return e
			}
			st.Rows++
		}
		if cols == nil && !sawContent {
			return nil
		}
		if cols == nil {
			if optional { return nil }
			return fmt.Errorf("%s: required headers missing", name)
		}
		st.Sheets++
		return nil
	}
	parseXLSX := func(input io.Reader) error {
		f, e := excelize.OpenReader(input, excelize.Options{UnzipSizeLimit: 512 << 20, UnzipXMLSizeLimit: 16 << 20})
		if e != nil {
			return e
		}
		defer f.Close()
		sheets := f.GetSheetList()
		for _, s := range sheets {
			rows, e := f.Rows(s)
			if e != nil {
				return e
			}
			err := parseSheet(s, strings.EqualFold(o.Marketplace, "flipkart") && len(sheets) > 1, func() ([]string, error) {
				if !rows.Next() {
					if e := rows.Error(); e != nil {
						return nil, e
					}
					return nil, io.EOF
				}
				return rows.Columns()
			})
			rows.Close()
			if err != nil {
				return err
			}
		}
		return nil
	}
	parseCSV := func(input io.Reader, name string) error {
		b := bufio.NewReader(input)
		peek, _ := b.Peek(4096)
		r := csv.NewReader(b)
		r.FieldsPerRecord = -1
		r.TrimLeadingSpace = true
		if strings.Count(string(peek), ";") > strings.Count(string(peek), ",") {
			r.Comma = ';'
		}
		lastLine := 0
		pending := []string(nil)
		next := func() ([]string, error) {
			if pending != nil {
				v := pending
				pending = nil
				return v, nil
			}
			v, e := r.Read()
			if e != nil {
				return v, e
			}
			line, _ := r.FieldPos(0)
			gap := lastLine > 0 && line > lastLine+1
			lastLine = line
			if gap {
				pending = v
				return []string{}, nil
			}
			return v, nil
		}
		return parseSheet(name, false, next)
	}
	switch strings.ToLower(filepath.Ext(o.Filename)) {
	case ".xlsx":
		f, e := excelize.OpenFile(path, excelize.Options{UnzipSizeLimit: 512 << 20, UnzipXMLSizeLimit: 16 << 20})
		if e != nil {
			return st, e
		}
		defer f.Close()
		sheets := f.GetSheetList()
		for _, s := range sheets {
			rows, e := f.Rows(s)
			if e != nil {
				return st, e
			}
			err := parseSheet(s, strings.EqualFold(o.Marketplace, "flipkart") && len(sheets) > 1, func() ([]string, error) {
				if !rows.Next() {
					if e := rows.Error(); e != nil {
						return nil, e
					}
					return nil, io.EOF
				}
				return rows.Columns()
			})
			rows.Close()
			if err != nil {
				return st, err
			}
		}
	case ".csv":
		f, e := openCSV(path)
		if e != nil {
			return st, e
		}
		defer f.Close()
		if e := parseCSV(f, "CSV"); e != nil {
			return st, e
		}
	case ".zip":
		z, e := zip.OpenReader(path)
		if e != nil {
			return st, fmt.Errorf("invalid ZIP archive: %w", e)
		}
		defer z.Close()
		supported := 0
		for _, entry := range z.File {
			if entry.FileInfo().IsDir() || strings.HasPrefix(entry.Name, "__MACOSX/") || filepath.Base(entry.Name) == ".DS_Store" {
				continue
			}
			ext := strings.ToLower(filepath.Ext(entry.Name))
			if ext != ".csv" && ext != ".xlsx" {
				return st, fmt.Errorf("ZIP contains unsupported file %q; only CSV and XLSX are allowed", entry.Name)
			}
			if entry.UncompressedSize64 > 100<<20 {
				return st, fmt.Errorf("ZIP entry %q exceeds 100 MB", entry.Name)
			}
			r, e := entry.Open()
			if e != nil {
				return st, e
			}
			if ext == ".csv" {
				e = parseCSV(r, entry.Name)
			} else {
				e = parseXLSX(r)
			}
			r.Close()
			if e != nil {
				return st, fmt.Errorf("%s: %w", entry.Name, e)
			}
			supported++
		}
		if supported == 0 {
			return st, fmt.Errorf("ZIP contains no supported CSV or XLSX files")
		}
	default:
		return st, fmt.Errorf("only CSV, XLSX, and ZIP files are supported")
	}
	if st.Rows == 0 {
		return st, fmt.Errorf("no data rows found")
	}
	return st, nil
}
