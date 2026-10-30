package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func myntraCaseKey(r Row) string {
	line := Normalize(r.OrderLineID)
	if line == "" { line = Normalize(r.SKU) }
	return Normalize(r.OrderID) + "\x1f" + Normalize(r.SKU) + "\x1f" + line
}

func myntraRawDate(r Row, names ...string) time.Time {
	for _, name := range names {
		for key, value := range r.Raw {
			if headerLike(key) == headerLike(name) {
				if parsed, err := parseMyntraDate(value); err == nil { return parsed }
			}
		}
	}
	return time.Time{}
}

func headerLike(value string) string {
	value = strings.ToLower(value)
	return strings.Map(func(r rune) rune { if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') { return r }; return -1 }, value)
}

func parseMyntraDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{"2006-01-02", "02/01/2006", "02-01-2006", "2-Jan-2006", time.RFC3339} {
		if t, err := time.ParseInLocation(layout, value, IST); err == nil { return t, nil }
	}
	return time.Time{}, fmt.Errorf("invalid date")
}

// reconcileMyntra is an isolated Myntra SPF engine. It deliberately does not
// call Amazon or Flipkart claim, status, date, or settlement rules.
func reconcileMyntra(rows []Row, now time.Time) ([]Result, error) {
	websites := map[string][]Row{}
	physicalByLine, physicalByProduct := map[string][]Row{}, map[string][]Row{}
	paymentsByPacket, paymentsBySeller, paymentsByLine := map[string][]Row{}, map[string][]Row{}, map[string][]Row{}
	for _, row := range rows {
		switch row.Kind {
		case "website": websites[myntraCaseKey(row)] = append(websites[myntraCaseKey(row)], row)
		case "physical":
			if row.OrderLineID != "" { physicalByLine[Normalize(row.OrderLineID)] = append(physicalByLine[Normalize(row.OrderLineID)], row) }
			physicalByProduct[Key(row.OrderID, row.SKU)] = append(physicalByProduct[Key(row.OrderID, row.SKU)], row)
		case "payment":
			if row.PacketID != "" { paymentsByPacket[Normalize(row.PacketID)] = append(paymentsByPacket[Normalize(row.PacketID)], row) }
			if row.SellerOrderID != "" { paymentsBySeller[Normalize(row.SellerOrderID)] = append(paymentsBySeller[Normalize(row.SellerOrderID)], row) }
			if row.OrderLineID != "" { paymentsByLine[Normalize(row.OrderLineID)] = append(paymentsByLine[Normalize(row.OrderLineID)], row) }
		}
	}
	out := make([]Result, 0, len(websites))
	for key, events := range websites {
		sort.SliceStable(events, func(i,j int) bool { return events[i].Updated.Before(events[j].Updated) })
		latest := events[len(events)-1]
		base := latest.Date
		isRTO := strings.Contains(Normalize(latest.Status+" "+latest.Channel), "RTO")
		if isRTO { if d := myntraRawDate(latest, "order_rto_date", "rto_creation_date"); !d.IsZero() { base = d } }
		if base.IsZero() { base = latest.Updated }
		result := Result{Marketplace:"myntra", Key:key, OrderID:latest.OrderID, SKU:latest.SKU, Month:Month(base), Status:latest.Status, ReturnDate:base, Updated:latest.Updated, Receipt:"Not Received", Financial:"Payment Match Pending", ClaimStatus:"Review", RuleVersion:MyntraRuleVersion, Evidence:append([]Row{}, events...), Notes:[]string{}}
		received := physicalByLine[Normalize(latest.OrderLineID)]
		if latest.OrderLineID == "" { received = physicalByProduct[Key(latest.OrderID, latest.SKU)] }
		if len(received)>0 {
			result.Receipt="Received"; result.Delivered=received[0].Date; result.ReceivedMonth=Month(received[0].Date); result.Financial="Not Investigated — Received"; result.Evidence=append(result.Evidence, received...); result.ClaimStatus="No Claim"
			classification := Normalize(latest.Status+" "+latest.Reason)
			if strings.Contains(classification,"DAMAGED") || strings.Contains(classification,"WRONG") || strings.Contains(classification,"USED") || strings.Contains(classification,"EMPTY") || strings.Contains(classification,"STAIN") {
				days := 30; model := "PPMP"
				if strings.Contains(Normalize(latest.Channel),"OMNI") { days=7; model="Omni" }
				result.ClaimRule=model+" unacceptable received item · receipt +"+fmt.Sprint(days)+" days"; result.ClaimOpen=Day(received[0].Date); result.ClaimDeadline=result.ClaimOpen.AddDate(0,0,days); RefreshClaim(&result,now)
			}
			out=append(out,result); continue
		}
		var tx []Row; matchMethod := ""
		if latest.PacketID!="" { tx=paymentsByPacket[Normalize(latest.PacketID)]; if len(tx)>0 { matchMethod="Packet ID" } }
		if len(tx)==0 && latest.SellerOrderID!="" { tx=paymentsBySeller[Normalize(latest.SellerOrderID)]; if len(tx)>0 { matchMethod="Seller Order ID" } }
		if len(tx)==0 && latest.OrderLineID!="" { tx=paymentsByLine[Normalize(latest.OrderLineID)]; if len(tx)>0 { matchMethod="Order Line ID" } }
		if len(tx)>0 {
			seen:=map[string]bool{}; var positive, negative int64
			for _, p := range tx { id:=p.ReferenceID+"\x1f"+p.Date.String()+"\x1f"+p.PacketID+"\x1f"+p.OrderLineID+"\x1f"+fmt.Sprint(p.Amount); if seen[id] { continue }; seen[id]=true; result.Evidence=append(result.Evidence,p); result.Net+=p.Amount; if p.Amount<0 { negative += -p.Amount } else { positive += p.Amount } }
			result.OrderTotal=positive; result.RefundTotal=-negative; result.Scenario="Matched by "+matchMethod
			if result.Net < 0 { result.Financial="Financial Loss Detected"; result.Loss=-result.Net } else { result.Financial="No Financial Debit Detected"; result.ClaimStatus="No Claim" }
		}
		if result.Financial=="Financial Loss Detected" {
			result.ClaimOpen=Day(base).AddDate(0,0,60); result.ClaimDeadline=Day(base).AddDate(0,0,120)
			if isRTO { result.ClaimRule="Missing RTO · RTO creation +60 to +120 days" } else { result.ClaimRule="Missing customer return · return creation +60 to +120 days" }
			RefreshClaim(&result,now)
		}
		out=append(out,result)
	}
	sort.Slice(out,func(i,j int)bool{if out[i].Month==out[j].Month{return out[i].Key<out[j].Key};return out[i].Month<out[j].Month})
	return out,nil
}
