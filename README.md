# ReturnScope — Robinbosky returns reconciliation

Amazon-first web application with a Svelte 5 / SvelteKit 3 interface, Go API and worker, and PostgreSQL. Flipkart and Myntra are reserved in the marketplace selector. All parsing, matching, money calculations, claim dates and exports run in Go.

## Run on Windows, macOS or Linux

Install Docker Desktop with Compose, then clone this repository. On Windows, Docker Desktop must be running with the WSL 2 engine.

```sh
git clone https://github.com/robinbosky-blr/verification-of-returns.git
cd verification-of-returns
cp .env.example .env
```

On Windows PowerShell use `Copy-Item .env.example .env` instead of `cp` if needed. Edit `.env` and set a unique `POSTGRES_PASSWORD` and an `ADMIN_PASSWORD` of at least 12 characters. Use URL-safe letters and numbers for the database password because it is used in a connection URL.

```sh
docker compose up -d --build
```

Windows users can instead run `powershell -ExecutionPolicy Bypass -File scripts/Start-Returns.ps1`; the launcher creates local credentials securely on first use and starts Docker.

Open **http://localhost:8088** and sign in with `ADMIN_PASSWORD`. This default binds only to your own computer. PostgreSQL and the API have no public ports. Data and original files persist in Docker volumes across restarts and image updates. Do not use `docker compose down -v` on a real installation: it deletes those volumes.

## Use the application

1. Select Amazon and open **Data imports**.
2. Upload Website return data, Physical received data and Payment data, in any order.
3. Each file is preserved before a background job validates it. Polling shows queued, completed or failed status. A validation failure rejects the entire file and leaves published results unchanged.
4. Physical files need a received date on each row, worksheet names such as `2026-07`, or an explicit physical month. For sheets named `July`, `August`, etc., the selected month supplies the year. Never select one fallback month for a multi-month file with unlabelled sheets.
5. Open **Overview**, **Reports**, or **Claims**. The year and month filter applies to the website return cohort. The four primary reports are Received, Not Received, Debited and Not Debited; Reimbursed and Claims are also available.
6. Open a case's arrow to inspect dates, amounts, applied rule, notes and the actual source rows used. Export filtered results to CSV or Excel.
7. Exclude an incorrect completed import in Import history. This retains the original file and queues reconciliation for the affected orders. Restore it to include it again. A corrected file is a new immutable import.

`testdata/` contains **synthetic examples only**. Importing the three files produces four logical cases, two physically received, one debited with INR 270 loss, and one not debited. The December example tests a January receipt. No real customer or financial files are committed to this public repository.

## Approved calculations

The executable contract is documented in [docs/rules.md](docs/rules.md) and tested in the domain engine. In particular:

- One normalized Order ID + SKU is one logical case; latest valid status, earliest return requested date defines its cohort.
- Physical matching checks the cohort month and following month, including December/January.
- Only Not Received cases proceed to payment investigation. Exact Order ID + SKU matching is primary. Blank-SKU payments are allocated only to uniquely identifiable single-SKU orders; ambiguous cases require review.
- Order, Refund, Fulfilment Fee Refund and product Reimbursement remain separate. Overall payment net does not decide loss.
- For Returned/Delivered to Seller, deadline is `max(delivered date, refund date) + 15 calendar days`.
- Otherwise the missing-return window is `refund date + 50` through `refund date + 75`, inclusive.
- Fully reimbursed cases have zero remaining product loss and no missing-product claim. Partial reimbursement leaves only the unrecovered refund amount.
- FBA/Amazon Centre is kept separate and requires review; this release does not apply Seller Flex rules to it.

Missing data and ambiguous transactions remain visible as **Payment Data Missing / Payment Review**. Therefore `Not Received = Debited + Not Debited + Unknown/Review`; incomplete evidence never becomes a false no-loss result.

## Architecture and operations

See [docs/architecture.md](docs/architecture.md), [docs/operations.md](docs/operations.md), and [docs/validation.md](docs/validation.md).

```text
web/       SvelteKit static UI, Caddy proxy
backend/   Go API, PostgreSQL job worker, deterministic engine, CSV/XLSX/ZIP parser
scripts/   backup and restore helpers
testdata/ synthetic fixtures
```

The API and worker are separate processes from the same Go executable. Imports use PostgreSQL COPY in batches. SHA-256 protects duplicate uploads; canonical row fingerprints prevent overlapping snapshots from inflating counts. Successful import, reconciliation results and run audit publish in one transaction. A process crash releases the transaction lock and leaves the job queued for another worker attempt. An invalid file becomes failed with an actionable error.

## Development and checks

Requires Go 1.27.3, Node 24 and PostgreSQL 18 (CI uses 18.6).

```sh
cd backend
go test -race ./...
go vet ./...
# Set TEST_DATABASE_URL to a disposable PostgreSQL database to run integration tests.
# Set DATABASE_URL, ADMIN_PASSWORD and COOKIE_SECURE=false for local development.
go run ./cmd/returns -mode all
```

In another terminal:

```sh
cd web
npm ci
npm run check
npm run dev
```

Vite proxies `/api` to port 8080. Production compilation: `npm run build`.

A dependency lockfile and Go checksums are committed. CI checks the backend, PostgreSQL import lifecycle and frontend build on every push. Source datasets from the earlier conversation were not available during development; the historical 1,676-case result is **not** certified by these synthetic tests. Validate the real files before using results to submit business claims.

## Amazon V1 dashboard and claim tracking (latest approved workflow)

The current Amazon implementation keeps four independent facts for every return case:

1. **Product:** Received / Not Received using Order ID + SKU and current-month + next-month physical matching.
2. **Money:** Refund debit, product reimbursement, fulfilment-fee refund, recovered amount, and remaining product loss. Fulfilment-fee refunds never reduce product loss.
3. **System claim action:** Scheduled / Raise Claim / Expired / No Claim / Review from the approved Amazon date rules.
4. **Manual claim status:** Not Raised / Pending / Approved / Rejected / Partially Approved. Manual values are stored outside reconciliation results and are never overwritten by later Amazon imports.

Dashboard metric cards are drillable. The active marketplace/year/month filter is applied to the preview rows. Financial summary shows refund debit, actual product recovery, current product loss, and no-loss case count.

Manual claim details stored permanently:

- claim raised flag/date
- current claim status and update timestamp
- Amazon claim ID
- requested and approved amounts
- rejection reason
- claim notes and internal user notes
- full claim update history with previous/current payload and changed-at timestamp

Imports are incremental. Identical files are deduplicated by hash; identical rows are deduplicated; new rows are added; changed website rows participate in the latest status calculation; and only affected orders are reconciled. Existing claim-action records remain untouched when source data is reprocessed.
