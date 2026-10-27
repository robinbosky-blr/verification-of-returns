# Architecture

A modular monolith, deployed as Go API + Go worker + PostgreSQL + Caddy/static SvelteKit UI. There is no financial computation, spreadsheet parsing or export assembly in the browser.

## Data path

Upload -> immutable raw object -> queued job -> streaming parser -> COPY batches -> canonical deduplication -> affected-order reconciliation -> atomic current results + run + audit.

Tables separate immutable imports, parsed evidence, jobs, current results, run metadata, rule versions, sessions and audit records. Results carry complete case evidence; table APIs omit it and fetch it only for the case drawer. Report responses are limited to 100 rows; UI uses 25. SQL narrows period, search and receipt/financial filters. The backend scans matching result payloads for totals and time-sensitive claim classification; summary caching or SQL aggregate materialization is an appropriate later optimization for multi-million-case installations.

A shared PostgreSQL transaction advisory lock serializes result publication and import activation. Worker jobs are selected with FOR UPDATE SKIP LOCKED. All imported rows, result replacement and audit events commit together. A crash before commit leaves published data intact and the job queued. Validation failures roll back staged rows and record a failed job/import separately.

Incremental work is keyed by affected Order IDs, including every SKU under those orders. This safely handles physical receipts in later months and late reimbursements for older return months. A full reprocess is available in Data imports. Original and canonical rows are retained; excluding an import queues a recalculation without deleting it.

CSV parsing and XLSX worksheet iteration stream source rows into 1,000-row database batches. ZIP uploads may contain only supported CSV and XLSX files; unsupported entries reject the whole import. XLSX and ZIP decompression are capped. Canonical fingerprint maps and the affected-order reconciliation set are held in memory; memory is proportional to the affected dataset, not strictly constant. Upload limit is 100 MiB and row limit is 2 million. Benchmark your representative largest files before raising limits. Very large broad imports should be partitioned by source period. Do not promise fixed processing times independent of file size or hardware.

Money uses int64 paise, with exact decimal parsing. Unknown transaction types require review. Dates use IST calendar arithmetic. The engine is deterministic for a given evidence set and as-of date. Daily claim status is refreshed at read/export time, so no browser timer or nightly job is needed for correctness.

## Storage

The default filesystem adapter uses a persistent Docker volume. The same interface supports S3-compatible storage through MinIO's Go SDK; provision the bucket privately, configure the S3 environment fields, and enable provider versioning and lifecycle policies. S3 configuration is shared by API and worker. No raw storage keys are returned in public API responses.

## Scope

Amazon Seller Flex / Easy Ship workflow is implemented. FBA evidence is held for review. Flipkart and Myntra need their own parsers and rule engines. No Amazon login, automatic claim submission, marketplace API access or AI financial decision-making is included.
