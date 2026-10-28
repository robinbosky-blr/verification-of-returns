package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Reconcile is deterministic: money is integer paise, dates are calendar days in IST.
func Reconcile(rows []Row, now time.Time) ([]Result, error) {
	return reconcileAmazon(rows, now)
}

// ReconcileMarketplace is the hard boundary between marketplace rule engines.
// Amazon and Flipkart never share date, status, payment, or claim calculations.
func ReconcileMarketplace(marketplace string, rows []Row, now time.Time) ([]Result, error) {
	switch strings.ToLower(strings.TrimSpace(marketplace)) {
	case "amazon", "":
		return reconcileAmazon(rows, now)
	case "flipkart":
		return reconcileFlipkart(rows, now)
	default:
		return nil, fmt.Errorf("unsupported marketplace %q", marketplace)
	}
}

func reconcileAmazon(rows []Row, now time.Time) ([]Result, error) {
	websites := map[string][]Row{}
	physical := map[string][]Row{}
	payments := map[string][]Row{}
	orderSKUs := map[string]map[string]bool{}
	blank := map[string][]Row{}
	for _, r := range rows {
		k := Key(r.OrderID, r.SKU)
		switch r.Kind {
		case "website":
			websites[k] = append(websites[k], r)
		case "physical":
			physical[k] = append(physical[k], r)
		case "payment":
			if r.SKU == "" {
				blank[r.OrderID] = append(blank[r.OrderID], r)
			} else {
				payments[k] = append(payments[k], r)
			}
		}
		if r.Kind == "website" && r.SKU != "" {
			if orderSKUs[r.OrderID] == nil {
				orderSKUs[r.OrderID] = map[string]bool{}
			}
			orderSKUs[r.OrderID][r.SKU] = true
		}
	}
	out := make([]Result, 0, len(websites))
	for k, events := range websites {
		sort.SliceStable(events, func(i, j int) bool {
			if events[i].Updated.Equal(events[j].Updated) {
				if !events[i].ImportedAt.Equal(events[j].ImportedAt) {
					return events[i].ImportedAt.Before(events[j].ImportedAt)
				}
				return events[i].Status < events[j].Status
			}
			return events[i].Updated.Before(events[j].Updated)
		})
		latest := events[len(events)-1]
		first := events[0].Date
		for _, e := range events {
			if e.Date.Before(first) {
				first = e.Date
			}
		}
		r := Result{Marketplace: "amazon", Key: k, OrderID: latest.OrderID, SKU: latest.SKU, Month: Month(first), Status: latest.Status, ReturnDate: first, Updated: latest.Updated, Delivered: latest.Delivered, Receipt: "Not Received", Financial: "Payment Data Missing", ClaimStatus: "Review", RuleVersion: RuleVersion, Evidence: append([]Row{}, events...), Notes: []string{}}
		next := Month(Day(first).AddDate(0, 1, 0))
		// AddDate on Jan 31 may skip February; derive next from the first of the month.
		base, _ := time.ParseInLocation("2006-01", r.Month, IST)
		next = Month(base.AddDate(0, 1, 0))
		for _, p := range physical[k] {
			m := Month(p.Date)
			if m == r.Month {
				r.CurrentPhysical = true
			}
			if m == next {
				r.NextPhysical = true
			}
			if m == r.Month || m == next {
				r.Evidence = append(r.Evidence, p)
			}
		}
		if r.CurrentPhysical || r.NextPhysical {
			r.Receipt = "Received"
			r.ReceivedMonth = next
			if r.CurrentPhysical {
				r.ReceivedMonth = r.Month
			}
			r.Financial = "Not Investigated — Received"
			r.ClaimStatus = "No Claim"
			out = append(out, r)
			continue
		}
		tx := append([]Row{}, payments[k]...)
		ambiguous := false
		for _, p := range blank[r.OrderID] {
			if len(orderSKUs[r.OrderID]) == 1 {
				tx = append(tx, p)
				r.Notes = append(r.Notes, "Unique order-level payment fallback")
			} else {
				ambiguous = true
				r.Notes = append(r.Notes, "Unallocated blank-SKU payment: review multi-SKU order")
			}
		}
		if len(tx) == 0 {
			if ambiguous {
				r.Financial = "Payment Review"
			}
			out = append(out, r)
			continue
		}
		hasRefund := false
		unsupported := false
		for _, p := range tx {
			r.Evidence = append(r.Evidence, p)
			r.Net += p.Amount
			switch Normalize(p.Type) {
			case "ORDER":
				r.OrderTotal += p.Amount
			case "REFUND":
				hasRefund = true
				r.RefundTotal += p.Amount
				if r.RefundDate.IsZero() || p.Date.Before(r.RefundDate) {
					r.RefundDate = p.Date
				}
			case "FULFILMENT FEE REFUND", "FULFILLMENT FEE REFUND":
				r.FeeRefund += p.Amount
			case "SAFE-T REIMBURSEMENT", "SAFE T REIMBURSEMENT", "REIMBURSEMENTS", "REIMBURSEMENT":
				r.Reimbursement += p.Amount
			default:
				unsupported = true
				r.Notes = append(r.Notes, "Unclassified payment type: "+p.Type)
			}
		}
		r.Scenario = "Order Only"
		if hasRefund {
			r.Scenario = "Order + Refund"
			if r.OrderTotal == 0 {
				r.Scenario = "Refund Only"
			}
			if r.FeeRefund != 0 {
				r.Scenario += " + Fulfilment Fee Refund"
			}
			if r.Reimbursement != 0 {
				r.Scenario += " + Reimbursement"
			}
		}
		if !hasRefund || r.RefundTotal >= 0 {
			r.Financial = "Not Debited"
			r.ClaimStatus = "No Claim"
		} else {
			r.Financial = "Debited"
			gross := -r.RefundTotal
			r.Recovered = max(int64(0), min(gross, r.Reimbursement))
			r.Loss = gross - r.Recovered
			if r.Loss == 0 {
				r.Financial = "Already Reimbursed"
				r.ClaimStatus = "No Claim"
			} else {
				if r.Recovered > 0 {
					r.Financial = "Partially Reimbursed"
				}
				applyClaims(&r, latest, now)
			}
		}
		if ambiguous || unsupported {
			r.Notes = append(r.Notes, "Financial allocation requires review before claiming")
			r.ClaimStatus = "Review"
			r.Financial = "Payment Review"
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Month == out[j].Month {
			return out[i].Key < out[j].Key
		}
		return out[i].Month < out[j].Month
	})
	s := Summarize(out)
	if s.Total != s.Received+s.NotReceived || s.NotReceived != s.Debited+s.NotDebited+s.Unknown {
		return nil, fmt.Errorf("reconciliation invariant failed")
	}
	return out, nil
}
func deliveredStatus(s string) bool {
	switch Normalize(s) {
	case "RETURNED TO SELLER", "DELIVERED TO SELLER":
		return true
	}
	return false
}
func applyClaims(r *Result, latest Row, now time.Time) {
	if strings.Contains(Normalize(latest.Channel), "FBA") || strings.Contains(Normalize(latest.Channel), "AMAZON CENTRE") {
		r.ClaimRule = "FBA — separate workflow"
		r.ClaimStatus = "Review"
		return
	}
	if deliveredStatus(r.Status) {
		r.ClaimRule = "Later delivered/refund + 15 days"
		del := r.Delivered
		if del.IsZero() {
			del = latest.Updated
			r.Notes = append(r.Notes, "Delivered date uses Last Updated On for delivered status")
		}
		r.Delivered = del
		if del.IsZero() {
			r.ClaimStatus = "Review"
			return
		}
		r.ClaimOpen = Day(del)
		if Day(r.RefundDate).After(r.ClaimOpen) {
			r.ClaimOpen = Day(r.RefundDate)
		}
		r.ClaimDeadline = r.ClaimOpen.AddDate(0, 0, 15)
	} else {
		r.ClaimRule = "Refund + 50–75 days"
		r.ClaimOpen = Day(r.RefundDate).AddDate(0, 0, 50)
		r.ClaimDeadline = Day(r.RefundDate).AddDate(0, 0, 75)
	}
	RefreshClaim(r, now)
}
func RefreshClaim(r *Result, now time.Time) {
	if r.ClaimOpen.IsZero() || r.ClaimStatus == "Review" && r.Financial == "Payment Review" {
		return
	}
	today := Day(now)
	switch {
	case today.Before(r.ClaimOpen):
		r.ClaimStatus = "Scheduled"
	case today.After(r.ClaimDeadline):
		r.ClaimStatus = "Expired"
	default:
		r.ClaimStatus = "Raise Claim"
	}
}
func Summarize(rs []Result) Summary {
	s := Summary{}
	for _, r := range rs {
		s.Total++
		manual := r.UserClaimStatus
		if manual == "" {
			manual = "Not Raised"
		}
		if manual == "Not Raised" {
			switch r.ClaimStatus {
			case "Raise Claim":
				s.Raise++
			case "Scheduled":
				s.Scheduled++
			case "Expired":
				s.Expired++
			}
		}
		switch manual {
		case "Pending":
			s.ClaimPending++
		case "Approved":
			s.ClaimApproved++
		case "Rejected":
			s.ClaimRejected++
		case "Partially Approved":
			s.ClaimPartialApproved++
		}
		if r.Receipt == "Received" {
			s.Received++
			continue
		}
		s.NotReceived++
		switch r.Financial {
		case "Debited", "Partially Reimbursed", "Already Reimbursed":
			s.Debited++
		case "Not Debited":
			s.NotDebited++
		default:
			s.Unknown++
		}
		if r.Financial == "Already Reimbursed" {
			s.Reimbursed++
		}
		if r.Financial == "Partially Reimbursed" {
			s.Partial++
		}
		if r.Financial == "Not Debited" || r.Financial == "Already Reimbursed" {
			s.NoLossCases++
		}
		if r.RefundTotal < 0 {
			s.RefundDebited -= r.RefundTotal
		}
		s.Reimbursement += r.Reimbursement
		s.FeeRefund += r.FeeRefund
		s.Loss += r.Loss
		s.Recovered += r.Recovered
	}
	return s
}
