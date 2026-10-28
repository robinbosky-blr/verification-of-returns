# Flipkart source file mapping

These mappings are isolated from Amazon and are based on the supplied Flipkart workbooks.

## Website returns workbook

The parser finds the raw sheet by its headers and skips analysis/comparison sheets.

| Application field | Flipkart column |
| --- | --- |
| Order ID | Order ID |
| Order item reference | Order Item ID |
| Seller SKU | SKU |
| Return date and case month | Return Requested Date |
| Latest status | Return Status |
| Status updated date | Completed Date, then Out For Delivery Date, then Return Approval Date, then Return Requested Date |
| Delivered date | Completed Date |
| Return type | Return Type |
| Return reason | Return Reason or Return Sub-reason |

Rows are grouped by the month of `Return Requested Date`, even when one workbook contains several months.

## Physical received workbook

Every worksheet is scanned independently because the header is on row 2 and the SKU position changes between months.

| Application field | Flipkart column |
| --- | --- |
| Physical received date | date or DATE |
| Order ID | Order ID |
| Order item reference | Order Item ID |
| Seller SKU | SKU |

If a physical row contains only an Order ID, it is accepted and assigned only when that Order ID identifies one unique website case. Ambiguous rows remain unallocated for review instead of being guessed.

## Payment and settlement workbook

The Flipkart settlement header is read from row 2.

| Application transaction | Flipkart column |
| --- | --- |
| Transaction date | Payment Date |
| Order ID | Order ID |
| Order item reference | Order item ID |
| Sale | Sale Amount (Rs.) |
| Product refund | Refund (Rs.) |
| SPF recovery | Protection Fund (Rs.) |
| Fee refund | Positive Marketplace Fee (Rs.) only |

A settlement row may create several transactions. Product loss uses the product refund less actual Protection Fund recovery. Marketplace fees never reduce product loss.

## Upload behavior

- CSV, XLSX and ZIP are supported.
- Each upload control accepts multiple files.
- Workbooks may contain one month or several months.
- Dashboard month assignment always comes from the website return date.
- Physical receipt and payment rows may occur in later months and are linked back by Order Item ID, then by a unique Order ID.
- Unsupported or ambiguous rows are not silently assigned.
