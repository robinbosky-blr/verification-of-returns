# Flipkart approved contract — flipkart-spf-v1

Flipkart runs in a separate marketplace namespace. Amazon statuses, payment labels, SAFE-T dates, parsers and claim rules are never applied to Flipkart rows.

## Matching boundary

- A Flipkart case is keyed by normalized Order ID + Seller SKU.
- Return, physical-receipt and payment/SPF files are parsed with the Flipkart column registry only.
- Imports, rows, reconciliation results, manual claim records, histories and audit entries carry `marketplace=flipkart`.
- PostgreSQL exposes isolated `amazon` and `flipkart` schemas backed by marketplace-filtered, check-option views.
- Browser/PWA mode stores Amazon and Flipkart in different IndexedDB records.

Exact production column aliases must be confirmed against the original Flipkart files before the mapping registry is treated as locked.

## Claim windows

| Flipkart scenario | System window |
| --- | --- |
| Damaged, defective or wrong item physically received | Raise within 14 days from physical receipt date |
| Return not received / Product Missing | Scheduled until day 60; eligible day 60 through day 120 from return creation |
| Do Not Expect Physical Delivery (DNE) | Scheduled until day 60; eligible through day 120 from return request/creation |
| In Transit but physically received | Wait 24 hours from receipt, then ticket; outer deadline remains day 120 |
| SPF claim rejected | Appeal within 7 days from rejection |
| Flipkart decision | Track 12-day TAT from claim raised date |

The 7-day appeal and 12-day decision TAT are manual-claim follow-up dates. They do not replace the automatic Product Missing or received-condition eligibility windows.

## Flipkart money contract

- Customer return/refund debit creates product exposure.
- Refund transactions retain their sign and are netted only across the Website
  return month plus the next month.
- Until both required payment months are loaded, the financial status is
  `Window Incomplete`. When both are loaded and no match exists, it is
  `Payment Not Found` rather than a zero transaction.
- A negative net refund is `Debited`; a zero or positive net refund is
  `Not Debited`, including a debit that was fully reversed.
- Only actual Flipkart SPF/product reimbursement reduces current product loss.
- Fee or commission refunds are reported separately and never reduce product loss.
- Unsupported Flipkart transaction descriptions are classified as `Payment Review`; they are not silently treated as reimbursement.
- Physically received cases are not investigated as missing-return payment claims.

## Manual claim contract

Manual statuses remain `Not Raised`, `Pending`, `Approved`, `Rejected`, and `Partially Approved`. Claim IDs, amounts, dates, reasons and notes are marketplace-scoped and imports cannot overwrite them.
