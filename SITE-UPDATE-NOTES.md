# Amazon Returns Control Centre — Latest Approved Update

Date: 24 October 2026

## Dashboard
- Return, Received, Not Received, Debited, and Not Debited cards are clickable and open exact row previews for the current marketplace/year/month filter.
- Financial summary separates Total Refund, Product Recovered, Current Product Loss, No-loss Cases, and Fulfilment Fee Refund.
- Fulfilment Fee Refund does not reduce product loss.
- Added monthly Loss vs Recovery chart.
- Claim summary separates automated work queues from user-entered claim status.

## Permanent claim tracking
Manual claim data is stored separately from recalculated Amazon data and is never overwritten by new imports.

Current user status is exactly one of:
- Not Raised
- Pending
- Approved
- Rejected
- Partially Approved

Stored fields:
- claim_raised
- claim_raised_date
- claim_status
- claim_updated_at
- amazon_claim_id
- claim_amount_requested
- claim_amount_approved
- claim_rejection_reason
- claim_notes
- user_notes

Every manual update writes a history record with previous/current values and timestamp.

## Automated vs manual claim state
- Automated System Claim Action remains: Scheduled / Raise Claim / Expired / No Claim / Already Reimbursed.
- Manual Claim Status remains independent.
- A case marked Pending/Approved/Rejected/Partially Approved is removed from the automated Raise Claim/Scheduled/Expired action queue where appropriate, so it is not repeatedly presented as needing a new submission.

## Incremental import behavior
- Original imports remain preserved.
- Same logical source row is not double counted.
- New Order ID + SKU records are inserted.
- Updated website records replace the canonical current version for affected cases while prior imports remain auditable.
- Physical updates recalculate only affected cases.
- Payment rows use a stable transaction fingerprint when Amazon supplies a settlement/transaction reference so corrected cumulative payment exports replace the older logical transaction instead of being counted twice.
- Reconciliation is re-run only for affected Order ID + SKU cases.

## Product loss rule
- Product Loss = absolute buyer Refund minus actual product Reimbursement, floored at zero.
- Fulfilment Fee Refund is fee recovery only and is reported separately.
- Claim approval by itself does not count as recovered money; recovery changes only when reimbursement exists in payment data.

## Validation performed in this environment
- Go source formatting check: passed.
- Domain reconciliation tests, including same-date latest-import selection and manual-claim queue behavior: passed under an isolated local Go 1.23 compatibility test module.
- Full backend build was not runnable here because this environment has Go 1.23.2 and the project requires Go 1.27.0.
- Full SvelteKit build was not runnable here because the dependency cache is incomplete and internet access is disabled; local Node is 22.16.0 while the selected SvelteKit toolchain expects a newer Node 22 patch.
