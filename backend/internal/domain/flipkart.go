package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// reconcileFlipkart implements only the user-approved Flipkart workflow.
// It deliberately does not call or reuse Amazon claim/payment rules.
func reconcileFlipkart(rows []Row, now time.Time) ([]Result, error) {
	websites := map[string][]Row{}
	physical := map[string][]Row{}
	payments := map[string][]Row{}
	orderKeys := map[string]map[string]struct{}{}
	itemKeys := map[string]map[string]struct{}{}
	for _, row := range rows {
		if row.Kind != "website" { continue }
		key := Key(row.OrderID, row.SKU)
		websites[key] = append(websites[key], row)
		if orderKeys[row.OrderID] == nil { orderKeys[row.OrderID] = map[string]struct{}{} }
		orderKeys[row.OrderID][key] = struct{}{}
		if row.ReferenceID != "" { if itemKeys[row.ReferenceID] == nil { itemKeys[row.ReferenceID] = map[string]struct{}{} }; itemKeys[row.ReferenceID][key] = struct{}{} }
	}
	uniqueKey := func(keys map[string]struct{}) string { if len(keys) != 1 { return "" }; for key := range keys { return key }; return "" }
	resolve := func(row Row) string {
		if row.SKU != "" { key := Key(row.OrderID, row.SKU); if _, ok := websites[key]; ok { return key } }
		if row.ReferenceID != "" { if key := uniqueKey(itemKeys[row.ReferenceID]); key != "" { return key } }
		return uniqueKey(orderKeys[row.OrderID])
	}
	ambiguous := map[string]int{}
	for _, row := range rows {
		if row.Kind == "website" { continue }
		key := resolve(row)
		if key == "" { ambiguous[row.OrderID]++; continue }
		if row.Kind == "physical" { physical[key] = append(physical[key], row) }
		if row.Kind == "payment" { payments[key] = append(payments[key], row) }
	}
	out := make([]Result, 0, len(websites))
	for key, events := range websites {
		sort.SliceStable(events, func(i, j int) bool {
			if events[i].Updated.Equal(events[j].Updated) {
				return events[i].ImportedAt.Before(events[j].ImportedAt)
			}
			return events[i].Updated.Before(events[j].Updated)
		})
		latest := events[len(events)-1]
		first := events[0].Date
		for _, event := range events {
			if event.Date.Before(first) {
				first = event.Date
			}
		}
		receivedRows := append([]Row{}, physical[key]...)
		sort.Slice(receivedRows, func(i, j int) bool { return receivedRows[i].Date.Before(receivedRows[j].Date) })
		received := len(receivedRows) > 0
		result := Result{
			Marketplace: "flipkart", Key: key, OrderID: latest.OrderID, SKU: latest.SKU,
			Month: Month(first), Status: latest.Status, ReturnDate: first, Updated: latest.Updated,
			Receipt: "Not Received", Financial: "Payment Data Missing", ClaimStatus: "Review",
			RuleVersion: FlipkartRuleVersion, Evidence: append([]Row{}, events...), Notes: []string{},
		}
		if count := ambiguous[latest.OrderID]; count > 0 { result.Notes = append(result.Notes, fmt.Sprintf("%d Flipkart row(s) need Order Item ID or SKU for allocation", count)) }
		if received {
			result.Receipt = "Received"
			result.ReceivedMonth = Month(receivedRows[0].Date)
			result.Delivered = receivedRows[0].Date
			result.Evidence = append(result.Evidence, receivedRows...)
		}
		tx := payments[key]
		hasRefund := false
		unsupported := false
		for _, payment := range tx {
			result.Evidence = append(result.Evidence, payment)
			result.Net += payment.Amount
			typ := Normalize(payment.Type)
			switch {
			case typ == "ORDER" || typ == "SALE" || typ == "SALE OF PRODUCT" || typ == "PRODUCT SALE":
				result.OrderTotal += payment.Amount
			case typ == "REFUND" || typ == "RETURN" || typ == "CUSTOMER REFUND" || typ == "REFUND FOR RETURN" || typ == "RETURN REFUND":
				hasRefund = true
				result.RefundTotal -= abs64(payment.Amount)
				if result.RefundDate.IsZero() || payment.Date.Before(result.RefundDate) {
					result.RefundDate = payment.Date
				}
			case strings.Contains(typ, "SPF") || strings.Contains(typ, "SELLER PROTECTION FUND") || strings.Contains(typ, "REIMBURSEMENT"):
				result.Reimbursement += abs64(payment.Amount)
			case strings.Contains(typ, "FEE REFUND") || strings.Contains(typ, "COMMISSION REFUND"):
				result.FeeRefund += abs64(payment.Amount)
			default:
				unsupported = true
				result.Notes = append(result.Notes, "Unclassified Flipkart payment type: "+payment.Type)
			}
		}
		if len(tx) == 0 {
			if received {
				result.Financial = "Not Investigated — Received"
			}
		} else if !hasRefund {
			result.Financial = "Not Debited"
			result.Scenario = "Sale Only"
		} else {
			result.Scenario = "Refund Only"
			if result.OrderTotal != 0 {
				result.Scenario = "Sale + Refund"
			}
			gross := abs64(result.RefundTotal)
			result.Recovered = min(gross, max(int64(0), result.Reimbursement))
			result.Loss = gross - result.Recovered
			switch {
			case result.Loss == 0:
				result.Financial = "Already Reimbursed"
			case result.Recovered > 0:
				result.Financial = "Partially Reimbursed"
			default:
				result.Financial = "Debited"
			}
			if result.Reimbursement != 0 {
				result.Scenario += " + SPF Recovery"
			}
		}
		classification := Normalize(strings.Join([]string{latest.Status, latest.Reason, latest.Channel}, " "))
		switch {
		case received && containsAny(classification, "DAMAGED", "DEFECTIVE", "WRONG ITEM"):
			setFlipkartWindow(&result, receivedRows[0].Date, receivedRows[0].Date.AddDate(0, 0, 14), "SPF received condition claim · within 14 days", now)
		case received && strings.Contains(classification, "IN TRANSIT"):
			setFlipkartWindow(&result, receivedRows[0].Date.AddDate(0, 0, 1), first.AddDate(0, 0, 120), "In transit but physically received · wait 24 hours", now)
		case !received && (hasRefund || strings.Contains(classification, "DO NOT EXPECT") || strings.Contains(classification, "DNE")):
			rule := "Product Missing · day 60 to day 120"
			if strings.Contains(classification, "DO NOT EXPECT") || strings.Contains(classification, "DNE") {
				rule = "DNE physical delivery · day 60 to day 120"
			}
			setFlipkartWindow(&result, first.AddDate(0, 0, 60), first.AddDate(0, 0, 120), rule, now)
		case result.Financial == "Already Reimbursed" || result.Financial == "Not Debited" || received:
			result.ClaimStatus = "No Claim"
		default:
			result.ClaimStatus = "Review"
		}
		if unsupported {
			result.Financial = "Payment Review"
			result.ClaimStatus = "Review"
			result.Notes = append(result.Notes, "Flipkart payment allocation requires review")
		}
		out = append(out, result)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Month == out[j].Month {
			return out[i].Key < out[j].Key
		}
		return out[i].Month < out[j].Month
	})
	summary := Summarize(out)
	if summary.Total != summary.Received+summary.NotReceived || summary.NotReceived != summary.Debited+summary.NotDebited+summary.Unknown {
		return nil, fmt.Errorf("flipkart reconciliation invariant failed")
	}
	return out, nil
}

func abs64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func containsAny(value string, choices ...string) bool {
	for _, choice := range choices {
		if strings.Contains(value, choice) {
			return true
		}
	}
	return false
}

func setFlipkartWindow(result *Result, open, deadline time.Time, rule string, now time.Time) {
	result.ClaimRule = rule
	result.ClaimOpen = Day(open)
	result.ClaimDeadline = Day(deadline)
	today := Day(now)
	switch {
	case today.Before(result.ClaimOpen):
		result.ClaimStatus = "Scheduled"
	case today.After(result.ClaimDeadline):
		result.ClaimStatus = "Expired"
	default:
		result.ClaimStatus = "Raise Claim"
	}
}
