package domain

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const RuleVersion = "amazon-seller-v1"
const FlipkartRuleVersion = "flipkart-spf-v1"

var IST = time.FixedZone("Asia/Kolkata", 19800)

type Row struct {
	Marketplace string            `json:"marketplace"`
	Kind        string            `json:"kind"`
	OrderID     string            `json:"order_id"`
	SKU         string            `json:"sku"`
	Date        time.Time         `json:"date"`
	Updated     time.Time         `json:"updated"`
	Delivered   time.Time         `json:"delivered"`
	Status      string            `json:"status"`
	Reason      string            `json:"reason"`
	Type        string            `json:"type"`
	Channel     string            `json:"channel"`
	ReferenceID string            `json:"reference_id"`
	ImportedAt  time.Time         `json:"imported_at"`
	Amount      int64             `json:"amount"`
	Quantity    int64             `json:"quantity"`
	Source      string            `json:"source"`
	Line        int               `json:"line"`
	Raw         map[string]string `json:"raw"`
}

func Normalize(s string) string {
	return strings.ToUpper(strings.TrimSpace(strings.Map(func(r rune) rune {
		if r == '\u200b' || r == '\ufeff' {
			return -1
		}
		if unicode.IsSpace(r) {
			return ' '
		}
		return r
	}, s)))
}
func Key(order, sku string) string { return Normalize(order) + "\x1f" + Normalize(sku) }
func Day(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	y, m, d := t.In(IST).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, IST)
}
func Month(t time.Time) string { return t.In(IST).Format("2006-01") }
func Money(s string) (int64, error) {
	s = strings.TrimSpace(s)
	s = strings.NewReplacer(",", "", "₹", "", "INR", "", "Rs.", "").Replace(s)
	if s == "" {
		return 0, nil
	}
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		s = "-" + s[1:len(s)-1]
	}
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(strings.TrimPrefix(s, "-"), "+")
	p := strings.Split(s, ".")
	if len(p) > 2 {
		return 0, fmt.Errorf("invalid amount")
	}
	n, e := strconv.ParseInt(p[0], 10, 64)
	if e != nil || n < 0 || n > 9000000000000 {
		return 0, fmt.Errorf("invalid amount")
	}
	f := "00"
	if len(p) == 2 {
		if len(p[1]) > 2 {
			return 0, fmt.Errorf("amount has more than two decimals")
		}
		f = (p[1] + "00")[:2]
	}
	c, e := strconv.ParseInt(f, 10, 64)
	if e != nil {
		return 0, e
	}
	n = n*100 + c
	if neg {
		n = -n
	}
	return n, nil
}

type Result struct {
	Marketplace          string    `json:"marketplace"`
	Key                  string    `json:"key"`
	OrderID              string    `json:"order_id"`
	SKU                  string    `json:"sku"`
	Month                string    `json:"month"`
	Status               string    `json:"status"`
	ReturnDate           time.Time `json:"return_date"`
	Updated              time.Time `json:"updated"`
	Delivered            time.Time `json:"delivered"`
	Receipt              string    `json:"receipt"`
	ReceivedMonth        string    `json:"received_month"`
	CurrentPhysical      bool      `json:"current_physical"`
	NextPhysical         bool      `json:"next_physical"`
	Financial            string    `json:"financial"`
	Scenario             string    `json:"scenario"`
	OrderTotal           int64     `json:"order_total"`
	RefundTotal          int64     `json:"refund_total"`
	FeeRefund            int64     `json:"fee_refund"`
	Reimbursement        int64     `json:"reimbursement"`
	Net                  int64     `json:"net"`
	Loss                 int64     `json:"loss"`
	Recovered            int64     `json:"recovered"`
	RefundDate           time.Time `json:"refund_date"`
	ClaimRule            string    `json:"claim_rule"`
	ClaimOpen            time.Time `json:"claim_open"`
	ClaimDeadline        time.Time `json:"claim_deadline"`
	ClaimStatus          string    `json:"claim_status"` // automated System Claim Action
	ClaimRaised          bool      `json:"claim_raised"`
	ClaimRaisedDate      time.Time `json:"claim_raised_date"`
	UserClaimStatus      string    `json:"user_claim_status"`
	ClaimUpdatedDate     time.Time `json:"claim_updated_date"`
	AmazonClaimID        string    `json:"amazon_claim_id"`
	ClaimAmountRequested int64     `json:"claim_amount_requested"`
	ClaimAmountApproved  int64     `json:"claim_amount_approved"`
	ClaimRejectionReason string    `json:"claim_rejection_reason"`
	ClaimNotes           string    `json:"claim_notes"`
	UserNotes            string    `json:"user_notes"`
	RuleVersion          string    `json:"rule_version"`
	Notes                []string  `json:"notes"`
	Evidence             []Row     `json:"evidence"`
}
type Summary struct {
	Total                int   `json:"total"`
	Received             int   `json:"received"`
	NotReceived          int   `json:"not_received"`
	Debited              int   `json:"debited"`
	NotDebited           int   `json:"not_debited"`
	Unknown              int   `json:"unknown"`
	Reimbursed           int   `json:"reimbursed"`
	Partial              int   `json:"partial"`
	Raise                int   `json:"raise"`
	Scheduled            int   `json:"scheduled"`
	Expired              int   `json:"expired"`
	RefundDebited        int64 `json:"refund_debited"`
	Reimbursement        int64 `json:"reimbursement"`
	FeeRefund            int64 `json:"fee_refund"`
	Loss                 int64 `json:"loss"`
	Recovered            int64 `json:"recovered"`
	NoLossCases          int   `json:"no_loss_cases"`
	ClaimPending         int   `json:"claim_pending"`
	ClaimApproved        int   `json:"claim_approved"`
	ClaimRejected        int   `json:"claim_rejected"`
	ClaimPartialApproved int   `json:"claim_partial_approved"`
}
