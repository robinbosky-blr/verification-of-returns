package domain

import (
	"testing"
	"time"
)

func flipkartDate(value string) time.Time {
	t, _ := time.ParseInLocation("2006-01-02", value, IST)
	return t
}

func TestFlipkartProductMissingUsesDay60To120(t *testing.T) {
	rows := []Row{
		{Kind: "website", OrderID: "OD1", SKU: "SKU1", Date: flipkartDate("2026-01-01"), Updated: flipkartDate("2026-01-02"), Status: "Return Approved"},
		{Kind: "payment", OrderID: "OD1", SKU: "SKU1", Date: flipkartDate("2026-01-05"), Type: "Refund", Amount: 35000},
	}
	results, err := ReconcileMarketplace("flipkart", rows, flipkartDate("2026-03-15"))
	if err != nil || len(results) != 1 {
		t.Fatalf("results=%v error=%v", results, err)
	}
	got := results[0]
	if got.RuleVersion != FlipkartRuleVersion || got.ClaimStatus != "Raise Claim" {
		t.Fatalf("rule=%q status=%q", got.RuleVersion, got.ClaimStatus)
	}
	if got.ClaimOpen.Format("2006-01-02") != "2026-03-02" || got.ClaimDeadline.Format("2006-01-02") != "2026-05-01" {
		t.Fatalf("window=%s..%s", got.ClaimOpen, got.ClaimDeadline)
	}
	if got.Loss != 35000 || got.RefundTotal != -35000 {
		t.Fatalf("refund=%d loss=%d", got.RefundTotal, got.Loss)
	}
}

func TestFlipkartReceivedDamageUsesFourteenDays(t *testing.T) {
	rows := []Row{
		{Kind: "website", OrderID: "OD2", SKU: "SKU2", Date: flipkartDate("2026-04-01"), Updated: flipkartDate("2026-04-02"), Status: "Returned", Reason: "Damaged product"},
		{Kind: "physical", OrderID: "OD2", SKU: "SKU2", Date: flipkartDate("2026-04-10")},
	}
	results, err := ReconcileMarketplace("flipkart", rows, flipkartDate("2026-04-12"))
	if err != nil || len(results) != 1 {
		t.Fatalf("results=%v error=%v", results, err)
	}
	got := results[0]
	if got.Receipt != "Received" || got.ClaimStatus != "Raise Claim" || got.ClaimDeadline.Format("2006-01-02") != "2026-04-24" {
		t.Fatalf("receipt=%q status=%q deadline=%s", got.Receipt, got.ClaimStatus, got.ClaimDeadline)
	}
}

func TestMarketplaceEnginesStaySeparate(t *testing.T) {
	rows := []Row{{Kind: "website", OrderID: "OD3", SKU: "SKU3", Date: flipkartDate("2026-01-01"), Updated: flipkartDate("2026-01-02"), Status: "Return Approved"}, {Kind: "payment", OrderID: "OD3", SKU: "SKU3", Date: flipkartDate("2026-01-05"), Type: "Refund", Amount: 10000}}
	amazon, err := ReconcileMarketplace("amazon", rows, flipkartDate("2026-03-15"))
	if err != nil {
		t.Fatal(err)
	}
	flipkart, err := ReconcileMarketplace("flipkart", rows, flipkartDate("2026-03-15"))
	if err != nil {
		t.Fatal(err)
	}
	if amazon[0].RuleVersion == flipkart[0].RuleVersion || flipkart[0].RuleVersion != FlipkartRuleVersion {
		t.Fatalf("amazon=%q flipkart=%q", amazon[0].RuleVersion, flipkart[0].RuleVersion)
	}
}

func TestFlipkartAllocatesOrderOnlyPhysicalAndItemPayment(t *testing.T) {
	rows := []Row{
		{Kind: "website", OrderID: "OD4", SKU: "SKU4", ReferenceID: "ITEM4", Date: flipkartDate("2026-07-01"), Updated: flipkartDate("2026-07-02"), Status: "delivered"},
		{Kind: "physical", OrderID: "OD4", Date: flipkartDate("2026-07-20")},
		{Kind: "payment", OrderID: "OD4", ReferenceID: "ITEM4", Date: flipkartDate("2026-07-29"), Type: "REFUND", Amount: -21200},
	}
	results, err := ReconcileMarketplace("flipkart", rows, flipkartDate("2026-08-01"))
	if err != nil || len(results) != 1 { t.Fatal(results, err) }
	if results[0].Receipt != "Received" || results[0].RefundTotal != -21200 || results[0].Loss != 21200 { t.Fatal(results[0]) }
}
