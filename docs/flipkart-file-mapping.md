# Flipkart source file mapping

This registry is isolated from Amazon. Flipkart headers, dates, payment signs,
matching rules and claim calculations are never used for Amazon imports.

Every file follows the same deterministic stages: detect source type, detect
sheet and header row, map columns, normalize values, validate, compare with
active history, persist the original row and recalculate only affected cases.

## Website returns

The Website file is the master return source. The case month comes from
`Return Requested Date`, even when one workbook contains several months.

| Flipkart column | Canonical field | Use |
| --- | --- | --- |
| Order ID | `order_id` | Required primary match |
| Order Item ID | `order_item_id` | Strong item verification |
| Return ID | `return_id` | Return verification |
| Tracking ID | `tracking_id` | Logistics verification |
| Shipment ID | `shipment_id` | Logistics verification |
| Replacement Order Item ID | `replacement_order_item_id` | Replacement reference |
| SKU | `sku` | Seller item identity |
| FSN | `fsn` | Verification only; never substitutes for SKU |
| Product | `product_name` | Display/reference |
| Total Price | `total_price` | Decimal money |
| Quantity | `quantity` | Integer quantity |
| FF Type | `ff_type` | Fulfilment reference |
| Return Requested Date | `return_requested_date` | Return month and base return date |
| Return Approval Date | `return_approval_date` | Claim/date evidence |
| Completed Date | `completed_date` | Latest-status fallback and completion evidence |
| Out For Delivery Date | `out_for_delivery_date` | Latest-status fallback |
| Return Delivery Promise Date | `return_delivery_promise_date` | Logistics evidence |
| Picked Up Date | `picked_up_date` | Logistics evidence |
| Return Status | `return_status` | Current Flipkart status |
| Completion Status | `completion_status` | Reference |
| Return Type | `return_type` | Classification |
| Return Reason | `return_reason` | Classification |
| Return Sub-reason | `return_sub_reason` | Classification |
| Remaining supplied columns | Original header | Preserved in raw row payload |

Status updated date uses `Completed Date`, then `Out For Delivery Date`, then
`Return Approval Date`, then `Return Requested Date`. Analysis/comparison tabs
without a valid source signature are ignored.

`Order Item ID`, `Return ID`, `Shipment ID` and `FSN` are never silently mapped
as `Order ID` or `SKU`.

## Physical received

Every worksheet is scanned independently because header rows and SKU positions
can differ by month.

| Flipkart column | Canonical field | Use |
| --- | --- | --- |
| Order ID | `order_id` | Required primary match |
| Order Item ID | `order_item_id` | Secondary verification |
| Return ID | `return_id` | Secondary verification |
| Tracking ID | `tracking_id` | Logistics verification |
| Shipment ID | `shipment_id` | Logistics verification |
| SKU | `sku` | Product verification |
| FSN | `fsn` | Product verification only |
| Quantity | `quantity` | Quantity check |
| Physical Received Date / date | `physical_received_date` | Receipt timing |
| Return Status / Return Type | Canonical status/type | Reference |
| Location ID / Location Name | Canonical location | Warehouse reference |

An Order-ID-only physical row is accepted only when the Order ID identifies one
unique Website case. A matching receipt can arrive in any later month. The case
stays in its original Website month and the receipt month is recorded separately.

## Payment and settlement

The parser detects grouped/preamble layouts and finds the row containing the
actual payment headers.

| Flipkart column | Canonical field | Calculation |
| --- | --- | --- |
| Payment Date | `payment_date` | Payment month |
| Order ID | `order_id` | Required primary match |
| Order item ID | `order_item_id` | Preferred item match |
| Refund (Rs.) | `refund_amount` | Signed product debit/adjustment |
| Sale Amount (Rs.) | `sale_amount` | Sale evidence |
| Protection Fund (Rs.) | `protection_fund` | Actual SPF product recovery |
| Marketplace Fee | `marketplace_fee` | Fee recovery only |
| NEFT ID / Neft Type | Canonical NEFT fields | Settlement evidence |
| Bank Settlement Value | `bank_settlement_value` | Settlement evidence |
| Commission, fixed, collection, shipping and reverse-shipping fees | Canonical fee fields | Preserved for accounting |
| TCS, TDS and GST on MP fees | Canonical tax fields | Preserved for accounting |
| Seller SKU, Quantity, Return Type, Item Return Status, Invoice ID/Date | Canonical reference fields | Verification/reporting |

A settlement row can create Sale, Refund, SPF Reimbursement and Fee Refund
transactions. Refund values retain their sign. All matching Refund rows in the
return month and next month are summed. A negative net is `Debited`; zero or
positive is `Not Debited`. A debit followed by an equal positive adjustment is
therefore not a loss.

Only actual Protection Fund/SPF reimbursement reduces product loss. Marketplace
fee or commission refunds remain separate.

## Matching and payment window

- Website to Physical: Order ID, strengthened by Order Item ID and Return ID.
- Website to Payment: Order ID + Order Item ID when available; unique Order ID
  fallback only. Ambiguous multi-item orders require review.
- SKU or FSN alone never proves a match.
- Physical matching searches all active physical history.
- Payment matching searches only the Website return month plus the next month.
- If either required payment month has not been loaded, status is
  `Window Incomplete`.
- If both months are loaded but no matching payment row exists, status is
  `Payment Not Found`; it is never converted to a zero transaction.
- Received cases are not investigated as missing-return payment claims.

## Preservation and upload behavior

- CSV, XLSX and ZIP are supported, including bulk selection in the UI.
- Header detection scans preamble rows and validates source-specific signatures.
- Every original source column is retained in the row payload, including unknown
  future columns. Canonical mapped values are stored separately in the payload.
- IDs are read as text, dates are normalized, and money is stored as integer
  paise for calculation.
- New and changed rows are persisted; unchanged rows are not duplicated.
- Removing/excluding an import recalculates the marketplace without deleting
  manual claim history.
