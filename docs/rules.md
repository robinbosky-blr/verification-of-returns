# Amazon approved contract — amazon-seller-v1

This release implements the user-approved workflow from the final conversation. It does not silently replace it with newly researched marketplace policies. Claim rules are application records with an immutable version, and each result records the executable version. Future policy changes require a new version and explicit tests.

| Fact | Backend behavior |
|---|---|
| Identifiers | Text only. Trim surrounding spaces, normalize NBSP / zero-width spaces and case for comparison. Retain raw input evidence. |
| Website attempts | Group by normalized Order ID + SKU. Latest Last Updated On determines status. Earliest return requested date determines the case's month. |
| Physical forward fill | Carry Order ID down within a sheet section. Reset at an empty row, repeated header or new worksheet. Reject orphan SKUs. |
| Physical matching | Check website month M and M+1 only. Current-month receipt takes precedence in display. |
| Payment investigation | Only cases absent from both physical months. Exact Order ID + SKU first. |
| Order fallback | Blank-SKU transactions only; require exactly one known SKU across that order's evidence. Multi-SKU ambiguity is Review, never arbitrary allocation. |
| Refund debit | Sum of matching Refund transactions is negative. Earliest matching Refund timestamp supplies refund date. |
| Accounting net | Sum all matched amounts for reference; never use net alone to decide product loss. |
| Gross refund loss | `max(0, -refund_total)` |
| Product recovered | `min(gross_refund_loss, max(0, reimbursement_total))` |
| Remaining product loss | Gross refund loss minus product recovered. Uses integer paise throughout. |
| Fee refund | Separate recovery of fulfilment fees. Does not clear missing-product loss. |
| Full reimbursement | Zero remaining product loss; no missing-product claim. |
| Partial reimbursement | Unrecovered refund amount remains outstanding. |
| Delivered status | Exact normalized Returned to Seller or Delivered to Seller. Return to Seller alone is intermediate. |
| Delivered date | Explicit delivered date if present; otherwise latest Last Updated On only for delivered status. |
| Delivered deadline | Later of delivered date / refund date + 15 calendar days. |
| Not delivered | Claim opens refund + 50, expires refund + 75. |
| Daily status | Before opening: Scheduled. Opening through deadline inclusive: Raise Claim. After deadline: Expired. |
| Calendar | Asia/Kolkata, date-only comparisons, AddDate calendar arithmetic. |
| FBA / Amazon Centre | Separate workflow; Review, no Seller Flex/Easy Ship claim dates. |
| Unknown payment types | Retain evidence and totals, require allocation review before claiming. |
| Missing payment file/history | Payment Data Missing. No invented refund date or claim eligibility. |

## Import identity

Originals are immutable. Duplicate file detection is scoped to Amazon + source kind + SHA-256 + explicit coverage month (the month is part of interpretation for undated physical files).

Canonical website and physical rows share a fingerprint across overlapping imports. Payment rows also carry an occurrence index so two identical legitimate transactions **within one file** remain two transactions. Across overlapping snapshots, the largest occurrence count wins. Without a transaction ID, otherwise identical rows from distinct non-overlapping extracts cannot be proven to be separate transactions; retain full source exports for reconciliation rather than isolated fragments with identical transaction fields. Never overwrite older files to hide discrepancies.

## Summary invariants

- Total cases = Received + Not Received.
- Not Received = Debited + Not Debited + Unknown/Review.
- Debited includes partial and fully reimbursed refund debits; Reimbursed is also a separate report.
- Received cases are not financially investigated for missing-product claims.
- Remaining product loss cannot be negative.
- Claim status does not override financial evidence.

Operational case counts are logical cases, not unit quantities. This follows the approved Order ID + SKU workflow. Multi-unit partial physical receipt requires a later explicitly approved quantity workflow.
