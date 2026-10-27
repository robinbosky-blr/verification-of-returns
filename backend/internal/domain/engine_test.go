package domain

import (
	"fmt"
	"testing"
	"time"
)

func d(s string) time.Time {
	v, e := time.ParseInLocation("2006-01-02", s, IST)
	if e != nil {
		panic(e)
	}
	return v
}
func website(status string) Row {
	return Row{Kind: "website", OrderID: "A", SKU: "SKU", Date: d("2026-07-01"), Updated: d("2026-10-08"), Status: status}
}
func payment(typ string, n int64) Row {
	return Row{Kind: "payment", OrderID: "A", SKU: "SKU", Date: d("2026-10-03"), Type: typ, Amount: n}
}
func run(t *testing.T, rows []Row, now string) Result {
	t.Helper()
	rs, e := Reconcile(rows, d(now))
	if e != nil || len(rs) != 1 {
		t.Fatalf("results=%v error=%v", rs, e)
	}
	return rs[0]
}
func TestFinancialScenarios(t *testing.T) {
	for _, tc := range []struct {
		name      string
		tx        []Row
		financial string
		loss      int64
	}{
		{"missing", nil, "Payment Data Missing", 0},
		{"order only", []Row{payment("Order", 25000)}, "Not Debited", 0},
		{"positive net still loss", []Row{payment("Order", 30000), payment("Refund", -27000)}, "Debited", 27000},
		{"fee refund is not product recovery", []Row{payment("Order", 25000), payment("Refund", -27000), payment("Fulfilment Fee Refund", 5000)}, "Debited", 27000},
		{"full recovery", []Row{payment("Refund", -50000), payment("SAFE-T Reimbursement", 50000)}, "Already Reimbursed", 0},
		{"partial recovery", []Row{payment("Refund", -50000), payment("Reimbursements", 40000)}, "Partially Reimbursed", 10000},
		{"reversed reimbursement", []Row{payment("Refund", -50000), payment("Reimbursements", 40000), payment("Reimbursements", -40000)}, "Debited", 50000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := run(t, append([]Row{website("In-transit")}, tc.tx...), "2026-10-23")
			if r.Financial != tc.financial || r.Loss != tc.loss {
				t.Fatalf("got %s loss %d", r.Financial, r.Loss)
			}
			if r.Financial == "Already Reimbursed" && r.ClaimStatus != "No Claim" {
				t.Fatal(r.ClaimStatus)
			}
		})
	}
}
func TestReceiptMonthsAndDecember(t *testing.T) {
	for _, m := range []string{"2026-07-31", "2026-08-15", "2026-09-01"} {
		r := run(t, []Row{website("In-transit"), {Kind: "physical", OrderID: "A", SKU: "SKU", Date: d(m)}}, "2026-10-23")
		expected := m != "2026-09-01"
		if (r.Receipt == "Received") != expected {
			t.Fatal(m, r)
		}
	}
	w := website("Returned to Seller")
	w.Date = d("2026-12-31")
	r := run(t, []Row{w, {Kind: "physical", OrderID: "A", SKU: "SKU", Date: d("2027-01-31")}}, "2027-02-01")
	if r.Receipt != "Received" || r.ReceivedMonth != "2027-01" {
		t.Fatal(r)
	}
}
func TestLaterDateAndInclusiveDeadline(t *testing.T) {
	w := website("Returned to Seller")
	r := run(t, []Row{w, payment("Refund", -50000)}, "2026-10-23")
	if r.ClaimDeadline.Format("2006-01-02") != "2026-10-23" || r.ClaimStatus != "Raise Claim" {
		t.Fatal(r)
	}
	p := payment("Refund", -50000)
	p.Date = d("2026-10-15")
	r = run(t, []Row{w, p}, "2026-10-31")
	if r.ClaimDeadline.Format("2006-01-02") != "2026-10-30" || r.ClaimStatus != "Expired" {
		t.Fatal(r)
	}
}
func TestNotDeliveredBoundary(t *testing.T) {
	for _, status := range []string{"Return to Seller", "Return Initiated", "In-transit to Seller Flex Node", "Return Has Been Abandoned", "Customer Cancelled Pickup"} {
		for _, tc := range []struct {
			days int
			want string
		}{{49, "Scheduled"}, {50, "Raise Claim"}, {75, "Raise Claim"}, {76, "Expired"}} {
			rows := []Row{website(status), payment("Refund", -10000)}
			now := d("2026-10-03").AddDate(0, 0, tc.days)
			rs, e := Reconcile(rows, now)
			if e != nil || rs[0].ClaimStatus != tc.want {
				t.Fatal(status, tc, rs, e)
			}
		}
	}
}
func TestMultiSKUNeverAllocatesBlankPayment(t *testing.T) {
	w := website("In-transit")
	w2 := w
	w2.SKU = "OTHER"
	p := payment("Refund", -10000)
	p.SKU = ""
	rs, e := Reconcile([]Row{w, w2, p}, d("2026-10-23"))
	if e != nil {
		t.Fatal(e)
	}
	for _, r := range rs {
		if r.Financial != "Payment Review" || r.Loss != 0 || r.ClaimStatus != "Review" {
			t.Fatal(r)
		}
	}
}

func TestPhysicalOnlySKUDoesNotMakeOrderLevelPaymentAmbiguous(t *testing.T) {
	w := website("In-transit")
	p := payment("Refund", -10000)
	p.SKU = ""
	physical := Row{Kind: "physical", OrderID: "A", SKU: "UNMATCHED-WAREHOUSE-SKU", Date: d("2026-09-01")}
	r := run(t, []Row{w, physical, p}, "2026-10-23")
	if r.Financial != "Debited" || r.RefundTotal != -10000 {
		t.Fatal(r)
	}
}
func TestDedupLatestAndEarliestRefund(t *testing.T) {
	w := website("In-transit")
	old := w
	old.Updated = d("2026-07-01")
	old.Status = "Return Initiated"
	p := payment("Refund", -2000)
	p2 := p
	p2.Date = d("2026-10-10")
	r := run(t, []Row{old, w, p, p2}, "2026-10-23")
	if r.Status != "In-transit" || r.RefundTotal != -4000 || !r.RefundDate.Equal(p.Date) {
		t.Fatal(r)
	}
}
func TestFBASeparate(t *testing.T) {
	w := website("In-transit")
	w.Channel = "FBA"
	r := run(t, []Row{w, payment("Refund", -10000)}, "2026-10-23")
	if r.ClaimStatus != "Review" || !r.ClaimOpen.IsZero() {
		t.Fatal(r)
	}
}
func TestMoneyExact(t *testing.T) {
	for s, want := range map[string]int64{"₹1,234.56": 123456, "-197.06": -19706, "(500.00)": -50000, "0.01": 1} {
		v, e := Money(s)
		if e != nil || v != want {
			t.Fatal(s, v, e)
		}
	}
	for _, s := range []string{"1.001", "NaN", "9e9", "--3"} {
		if _, e := Money(s); e == nil {
			t.Fatal(s)
		}
	}
}
func BenchmarkReconcile10000(b *testing.B) {
	rows := make([]Row, 0, 30000)
	for i := 0; i < 10000; i++ {
		w := website("In-transit")
		w.OrderID = fmt.Sprint(i)
		p := payment("Refund", -20000)
		p.OrderID = w.OrderID
		rows = append(rows, w, p)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, e := Reconcile(rows, d("2026-10-23")); e != nil {
			b.Fatal(e)
		}
	}
}

func TestLatestImportWinsWhenLastUpdatedTies(t *testing.T) {
	old := website("Return Initiated")
	old.ImportedAt = d("2026-10-20")
	newer := website("Returned to Seller")
	newer.ImportedAt = d("2026-10-24")
	r := run(t, []Row{old, newer, payment("Refund", -10000)}, "2026-10-24")
	if r.Status != "Returned to Seller" {
		t.Fatalf("expected latest imported website row to win same-date tie; got %q", r.Status)
	}
}

func TestManualClaimStatusRemovesCaseFromAutomatedRaiseQueue(t *testing.T) {
	r := run(t, []Row{website("In-transit"), payment("Refund", -10000)}, "2026-11-22")
	if r.ClaimStatus != "Raise Claim" {
		t.Fatalf("expected automated Raise Claim, got %q", r.ClaimStatus)
	}
	r.UserClaimStatus = "Pending"
	s := Summarize([]Result{r})
	if s.Raise != 0 || s.ClaimPending != 1 {
		t.Fatalf("manual pending must leave automated work queue: %+v", s)
	}
}
