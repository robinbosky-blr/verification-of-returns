package domain

import (
	"testing"
	"time"
)

func myntraDate(value string) time.Time {
	t, _ := time.ParseInLocation("2006-01-02", value, IST)
	return t
}

func TestMyntraUsesLineSafePhysicalMatchingAndPacketPayment(t *testing.T) {
	rows := []Row{
		{Kind:"website", Marketplace:"myntra", OrderID:"M1", SKU:"SKU-A", OrderLineID:"L1", PacketID:"P1", Date:myntraDate("2026-01-01"), Updated:myntraDate("2026-01-02"), Status:"Return"},
		{Kind:"website", Marketplace:"myntra", OrderID:"M1", SKU:"SKU-A", OrderLineID:"L2", PacketID:"P2", Date:myntraDate("2026-01-01"), Updated:myntraDate("2026-01-02"), Status:"Return"},
		{Kind:"physical", Marketplace:"myntra", OrderID:"M1", SKU:"SKU-A", OrderLineID:"L1", Date:myntraDate("2026-01-10")},
		{Kind:"payment", Marketplace:"myntra", OrderID:"M1", PacketID:"P2", Date:myntraDate("2026-01-15"), Type:"Return", Amount:-50000, ReferenceID:"TX1"},
	}
	results, err := ReconcileMarketplace("myntra", rows, myntraDate("2026-03-10"))
	if err != nil { t.Fatal(err) }
	if len(results) != 2 { t.Fatalf("got %d results", len(results)) }
	byLine := map[string]Result{}
	for _, result := range results { byLine[result.Key[len(result.Key)-2:]] = result }
	if byLine["L1"].Receipt != "Received" { t.Fatalf("L1 receipt=%q", byLine["L1"].Receipt) }
	if byLine["L2"].Financial != "Financial Loss Detected" || byLine["L2"].Scenario != "Matched by Packet ID" { t.Fatalf("L2=%+v", byLine["L2"]) }
	if byLine["L2"].ClaimOpen != myntraDate("2026-03-02") || byLine["L2"].ClaimDeadline != myntraDate("2026-05-01") { t.Fatalf("window %v to %v", byLine["L2"].ClaimOpen, byLine["L2"].ClaimDeadline) }
}

func TestMyntraRTOUsesRTOCreationDate(t *testing.T) {
	row := Row{Kind:"website", Marketplace:"myntra", OrderID:"M2", SKU:"SKU-B", OrderLineID:"L3", PacketID:"P3", Date:myntraDate("2026-01-01"), Updated:myntraDate("2026-01-02"), Status:"RTO", Raw:map[string]string{"order_rto_date":"2026-01-20"}}
	payment := Row{Kind:"payment", Marketplace:"myntra", PacketID:"P3", Date:myntraDate("2026-02-01"), Amount:-10000, ReferenceID:"TX2"}
	results, err := ReconcileMarketplace("myntra", []Row{row,payment}, myntraDate("2026-03-25"))
	if err != nil { t.Fatal(err) }
	if len(results)!=1 || results[0].ClaimOpen != myntraDate("2026-03-21") || results[0].ClaimDeadline != myntraDate("2026-05-20") { t.Fatalf("result=%+v", results) }
}

func TestMyntraDoesNotChangeAmazonRules(t *testing.T) {
	rows := []Row{{Kind:"website", OrderID:"A", SKU:"S", Date:myntraDate("2026-01-01"), Updated:myntraDate("2026-01-02"), Status:"In-transit"}, {Kind:"payment", OrderID:"A", SKU:"S", Date:myntraDate("2026-01-03"), Type:"Refund", Amount:-10000}}
	result, err := ReconcileMarketplace("amazon", rows, myntraDate("2026-03-10"))
	if err != nil { t.Fatal(err) }
	if len(result)!=1 || result[0].RuleVersion != RuleVersion || result[0].ClaimRule != "Refund + 50–75 days" { t.Fatalf("amazon changed: %+v", result) }
}
