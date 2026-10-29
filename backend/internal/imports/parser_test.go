package imports

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/robinbosky-blr/verification-of-returns/backend/internal/domain"
	"github.com/xuri/excelize/v2"
)

func TestZIPWithSupportedCSV(t *testing.T) {
	p := filepath.Join(t.TempDir(), "payment.zip")
	f, e := os.Create(p)
	if e != nil {
		t.Fatal(e)
	}
	z := zip.NewWriter(f)
	w, e := z.Create("payment.csv")
	if e != nil {
		t.Fatal(e)
	}
	_, e = w.Write([]byte("date,type,order id,sku,total\n24/10/2026,Refund,A,S1,-350.00\n"))
	if e != nil {
		t.Fatal(e)
	}
	if e = z.Close(); e != nil {
		t.Fatal(e)
	}
	if e = f.Close(); e != nil {
		t.Fatal(e)
	}
	var rows []domain.Row
	_, e = Parse(p, Options{Kind: "payment", Filename: "payment.zip"}, func(r domain.Row) error {
		rows = append(rows, r)
		return nil
	})
	if e != nil || len(rows) != 1 || rows[0].Amount != -35000 {
		t.Fatal(rows, e)
	}
}

func TestCSVPhysicalForwardFill(t *testing.T) {
	p := filepath.Join(t.TempDir(), "p.csv")
	os.WriteFile(p, []byte("Order ID,SKU,Received Date\nA,SKU1,01/07/2026\n,SKU2,02/07/2026\nB,SKU3,03/07/2026\n"), 0600)
	var rows []domain.Row
	_, e := Parse(p, Options{Kind: "physical", Filename: "p.csv"}, func(r domain.Row) error { rows = append(rows, r); return nil })
	if e != nil || len(rows) != 3 || rows[1].OrderID != "A" {
		t.Fatal(rows, e)
	}
}
func TestForwardFillNeverCrossesSection(t *testing.T) {
	p := filepath.Join(t.TempDir(), "p.csv")
	os.WriteFile(p, []byte("Order ID,SKU,Date\nA,S1,01/07/2026\n\n,S2,01/07/2026\n"), 0600)
	_, e := Parse(p, Options{Kind: "physical", Filename: "p.csv"}, func(r domain.Row) error { return nil })
	if e == nil {
		t.Fatal("expected missing order error")
	}
}
func TestPaymentPreambleAndMoney(t *testing.T) {
	p := filepath.Join(t.TempDir(), "p.csv")
	os.WriteFile(p, []byte("Amazon transaction report\nReport period\ndate/time,type,order id,sku,total\n3 Oct 2026 3:04:05 PM IST,Refund,A,S1,-197.06\n"), 0600)
	var row domain.Row
	_, e := Parse(p, Options{Kind: "payment", Filename: "p.csv"}, func(r domain.Row) error { row = r; return nil })
	if e != nil || row.Amount != -19706 {
		t.Fatal(row, e)
	}
}
func TestXLSXMonthAndSheets(t *testing.T) {
	p := filepath.Join(t.TempDir(), "p.xlsx")
	f := excelize.NewFile()
	f.SetSheetName("Sheet1", "2026-07")
	f.NewSheet("2026-08")
	for _, s := range f.GetSheetList() {
		f.SetSheetRow(s, "A1", &[]any{"Order ID", "SKU"})
		f.SetSheetRow(s, "A2", &[]any{"A", "S1"})
		f.SetSheetRow(s, "A3", &[]any{"", "S2"})
	}
	f.SaveAs(p)
	f.Close()
	var rs []domain.Row
	_, e := Parse(p, Options{Kind: "physical", Filename: "p.xlsx"}, func(r domain.Row) error { rs = append(rs, r); return nil })
	if e != nil || len(rs) != 4 || domain.Month(rs[2].Date) != "2026-08" {
		t.Fatal(rs, e)
	}
}

func TestWebsiteCarrierPickupDateHeader(t *testing.T) {
	p := filepath.Join(t.TempDir(), "website.csv")
	os.WriteFile(p, []byte("Return Type,Customer Order ID,mSKU,Return Status,Carrier Pick-up date,Last Updated On\nCustomer Return,A,S1,Return Initiated,13.01.2026,14.01.2026\n"), 0600)
	var rows []domain.Row
	_, e := Parse(p, Options{Kind: "website", Filename: "website.csv"}, func(r domain.Row) error { rows = append(rows, r); return nil })
	if e != nil || len(rows) != 1 || domain.Month(rows[0].Date) != "2026-01" {
		t.Fatal(rows, e)
	}
}

func TestWebsiteUsesMerchantSKUEvenWhenListingSKUComesFirst(t *testing.T) {
	p := filepath.Join(t.TempDir(), "website.csv")
	os.WriteFile(p, []byte("Customer Order ID,SKU,mSKU,Return Status,Pick -up date,Last Updated On\n171-1,X001LISTING,MERCHANT-SKU-40,Returned to Seller,31-Jul-2026,10-Aug-2026\n"), 0600)
	var rows []domain.Row
	_, e := Parse(p, Options{Kind: "website", Filename: "website.csv"}, func(r domain.Row) error { rows = append(rows, r); return nil })
	if e != nil || len(rows) != 1 || rows[0].SKU != "MERCHANT-SKU-40" || domain.Month(rows[0].Date) != "2026-07" {
		t.Fatal(rows, e)
	}
}

func TestPhysicalDottedDate(t *testing.T) {
	p := filepath.Join(t.TempDir(), "physical.csv")
	os.WriteFile(p, []byte("Order Id,SKU,Return Type,Return Date.,Qty\nA,S1,Customer Return,13.01.2026,1\n"), 0600)
	var rows []domain.Row
	_, e := Parse(p, Options{Kind: "physical", Filename: "physical.csv"}, func(r domain.Row) error { rows = append(rows, r); return nil })
	if e != nil || len(rows) != 1 || rows[0].Date.Day() != 13 {
		t.Fatal(rows, e)
	}
}

func TestPaymentMissingOrderIsSkipped(t *testing.T) {
	p := filepath.Join(t.TempDir(), "payment.csv")
	os.WriteFile(p, []byte("date,type,order id,sku,total\n13.01.2026,Refund,,S1,-100.00\n14.01.2026,Refund,A,S1,-200.00\n"), 0600)
	var rows []domain.Row
	_, e := Parse(p, Options{Kind: "payment", Filename: "payment.csv"}, func(r domain.Row) error { rows = append(rows, r); return nil })
	if e != nil || len(rows) != 1 || rows[0].OrderID != "A" || rows[0].Amount != -20000 {
		t.Fatal(rows, e)
	}
}

func TestWebsiteMissingReturnDateUsesLastUpdated(t *testing.T) {
	p := filepath.Join(t.TempDir(), "website.csv")
	os.WriteFile(p, []byte("Customer Order ID,mSKU,Return Requested On,Last Updated On,Return Status\nA,S1,,14.01.2026,Return Initiated\n"), 0600)
	var rows []domain.Row
	_, e := Parse(p, Options{Kind: "website", Filename: "website.csv"}, func(r domain.Row) error { rows = append(rows, r); return nil })
	if e != nil || len(rows) != 1 || !rows[0].Date.Equal(rows[0].Updated) {
		t.Fatal(rows, e)
	}
}

func TestPhysicalMonthSheetDefaultsCurrentYear(t *testing.T) {
	p := filepath.Join(t.TempDir(), "physical.xlsx")
	f := excelize.NewFile()
	f.SetSheetName("Sheet1", "feb")
	f.SetSheetRow("feb", "A1", &[]any{"Order ID", "SKU"})
	f.SetSheetRow("feb", "A2", &[]any{"A", "S1"})
	f.SaveAs(p)
	f.Close()
	var rows []domain.Row
	_, e := Parse(p, Options{Kind: "physical", Filename: "physical.xlsx"}, func(r domain.Row) error { rows = append(rows, r); return nil })
	if e != nil || len(rows) != 1 || rows[0].Date.Month() != time.February || rows[0].Date.Year() != time.Now().In(domain.IST).Year() {
		t.Fatal(rows, e)
	}
}

func TestPaymentRecognizesExtendedHeadersWithoutSKU(t *testing.T) {
	p := filepath.Join(t.TempDir(), "payment.csv")
	os.WriteFile(p, []byte("Transaction Date,Transaction Type,Merchant Order ID,Total Amount\n14.01.2026,Refund,A,-200.00\n"), 0600)
	var rows []domain.Row
	_, e := Parse(p, Options{Kind: "payment", Filename: "payment.csv"}, func(r domain.Row) error { rows = append(rows, r); return nil })
	if e != nil || len(rows) != 1 || rows[0].OrderID != "A" || rows[0].Amount != -20000 {
		t.Fatal(rows, e)
	}
}

func TestFlipkartWebsiteSkipsAnalysisSheets(t *testing.T) {
	p := filepath.Join(t.TempDir(), "website.xlsx")
	f := excelize.NewFile()
	f.SetSheetName("Sheet1", "raw")
	f.NewSheet("analysis")
	f.SetSheetRow("raw", "A1", &[]any{"Order ID", "Order Item ID", "SKU", "Return Requested Date", "Return Approval Date", "Completed Date", "Return Status", "Return Type", "Return Reason"})
	f.SetSheetRow("raw", "A2", &[]any{"OD1", "ITEM1", "SKU1", "28 August, 2026", "28 August, 2026", "10 September, 2026", "delivered", "customer_return", "QUALITY_ISSUE"})
	f.SetSheetRow("analysis", "A1", &[]any{"July Order ID", "vlookup", "Loss verification"})
	f.SetSheetRow("analysis", "A2", &[]any{"OD1", "#N/A", "review"})
	if e := f.SaveAs(p); e != nil { t.Fatal(e) }; f.Close()
	var rows []domain.Row
	stats, e := Parse(p, Options{Marketplace: "flipkart", Kind: "website", Filename: "website.xlsx"}, func(r domain.Row) error { rows = append(rows, r); return nil })
	if e != nil || len(rows) != 1 || stats.Sheets != 1 || rows[0].ReferenceID != "ITEM1" || domain.Month(rows[0].Date) != "2026-08" {
		t.Fatal(stats, rows, e)
	}
}

func TestFlipkartPhysicalAllowsOrderOnlyRows(t *testing.T) {
	p := filepath.Join(t.TempDir(), "physical.csv")
	os.WriteFile(p, []byte("date,Order ID,Qty,Order Item ID,SKU\n22.07.2026,OD1,1,,\n"), 0600)
	var rows []domain.Row
	_, e := Parse(p, Options{Marketplace: "flipkart", Kind: "physical", Filename: "physical.csv"}, func(r domain.Row) error { rows = append(rows, r); return nil })
	if e != nil || len(rows) != 1 || rows[0].OrderID != "OD1" || rows[0].SKU != "" { t.Fatal(rows, e) }
}

func TestFlipkartSettlementLayoutCreatesProductTransactions(t *testing.T) {
	p := filepath.Join(t.TempDir(), "payment.csv")
	os.WriteFile(p, []byte("NEFT ID,Payment Date,Order ID,Order item ID,Sale Amount (Rs.),Protection Fund (Rs.),Refund (Rs.),Marketplace Fee (Rs.) = SUM (V:AI)\nN1,2026-07-29,OD1,ITEM1,212,50,-212,5\n"), 0600)
	var rows []domain.Row
	_, e := Parse(p, Options{Marketplace: "flipkart", Kind: "payment", Filename: "payment.csv"}, func(r domain.Row) error { rows = append(rows, r); return nil })
	if e != nil || len(rows) != 4 { t.Fatal(rows, e) }
	want := map[string]int64{"SALE": 21200, "REFUND": -21200, "SPF REIMBURSEMENT": 5000, "FEE REFUND": 500}
	for _, row := range rows { if row.Amount != want[row.Type] || row.ReferenceID != "ITEM1" { t.Fatal(rows) } }
}

func TestFlipkartPrimaryOrderNeverFallsBackToOrderItem(t *testing.T) {
	p := filepath.Join(t.TempDir(), "website.csv")
	os.WriteFile(p, []byte("Order Item ID,SKU,Return Requested Date,Return Status\nITEM1,SKU1,01/07/2026,approved\n"), 0600)
	_, e := Parse(p, Options{Marketplace: "flipkart", Kind: "website", Filename: "website.csv"}, func(r domain.Row) error { return nil })
	if e == nil { t.Fatal("expected missing primary Order ID header") }
}

func TestFlipkartFSNIsNotSilentlyUsedAsSKU(t *testing.T) {
	p := filepath.Join(t.TempDir(), "website.csv")
	os.WriteFile(p, []byte("Order ID,FSN,Return Requested Date,Return Status\nOD1,FSN1,01/07/2026,approved\n"), 0600)
	_, e := Parse(p, Options{Marketplace: "flipkart", Kind: "website", Filename: "website.csv"}, func(r domain.Row) error { return nil })
	if e == nil { t.Fatal("expected missing Seller SKU header") }
}

func TestFlipkartPreservesAllOriginalColumns(t *testing.T) {
	p := filepath.Join(t.TempDir(), "website.csv")
	os.WriteFile(p, []byte("Order ID,Order Item ID,SKU,Return Requested Date,Return Status,Vendor Name,Custom Future Column\nOD1,ITEM1,SKU1,01/07/2026,approved,Vendor A,keep me\n"), 0600)
	var row domain.Row
	_, e := Parse(p, Options{Marketplace: "flipkart", Kind: "website", Filename: "website.csv"}, func(r domain.Row) error { row = r; return nil })
	if e != nil { t.Fatal(e) }
	if row.Raw["Vendor Name"] != "Vendor A" || row.Raw["Custom Future Column"] != "keep me" || row.Raw["canonical.order"] != "OD1" { t.Fatal(row.Raw) }
}

func TestFlipkartExplicitZeroRefundIsPreserved(t *testing.T) {
	p := filepath.Join(t.TempDir(), "payment.csv")
	os.WriteFile(p, []byte("Payment Date,Order ID,Order item ID,Refund (Rs.)\n2026-07-29,OD1,ITEM1,0\n"), 0600)
	var rows []domain.Row
	_, e := Parse(p, Options{Marketplace: "flipkart", Kind: "payment", Filename: "payment.csv"}, func(r domain.Row) error { rows = append(rows, r); return nil })
	if e != nil || len(rows) != 1 || rows[0].Type != "REFUND" || rows[0].Amount != 0 { t.Fatal(rows, e) }
}

func TestFlipkartBlankReturnStatusDoesNotBlockOtherRows(t *testing.T) {
	p := filepath.Join(t.TempDir(), "website.csv")
	os.WriteFile(p, []byte("Order ID,Order Item ID,SKU,Return Requested Date,Return Status\nOD1,ITEM1,SKU1,01/07/2026,\nOD2,ITEM2,SKU2,02/07/2026,approved\n"), 0600)
	var rows []domain.Row
	_, e := Parse(p, Options{Marketplace: "flipkart", Kind: "website", Filename: "website.csv"}, func(r domain.Row) error { rows = append(rows, r); return nil })
	if e != nil || len(rows) != 2 { t.Fatal(rows, e) }
	if rows[0].Status != "Return Status Not Found" || rows[1].Status != "approved" { t.Fatal(rows) }
}
