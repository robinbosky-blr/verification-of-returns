# Validation record

## Automated contracts

The domain suite covers normalization and exact money parsing, missing payment history, order-only payments, positive accounting net with a refund loss, fee refunds, full and partial product reimbursement, reimbursement reversals, current/following month physical receipts, December/January, duplicate website attempts, earliest refund date, both claim windows including inclusive boundaries, multi-SKU blank-payment ambiguity and FBA separation.

Parser tests cover Amazon CSV preambles, forward filling, section boundaries, worksheet-based physical periods, XLSX files, and ZIP archives containing supported structured files. HTTP tests cover sign-in requirements, cross-origin write rejection and CSV formula escaping.

The database integration suite uses a disposable schema and checks COPY imports, overlapping snapshots, exact duplicate uploads, multiple identical legitimate payment occurrences, exclude/restore recalculation and transaction rollback on a malformed row. CI supplies PostgreSQL 18.6 for these tests.

## Business acceptance still required

The historical website, physical and corrected payment files could not be retrieved from the available files. No customer data has been invented or published. The prior 1,676-case workbook totals cannot be certified without those source files. Upload them through the application and compare the case-level differences, not only the grand totals.

## Deployment checks

The Docker configuration provides a reproducible local installation. A live server, HTTPS DNS configuration, backup scheduler and off-machine backup credentials must be supplied by the deployment environment. The repository does not claim a remote deployment is running.

## Results from this build environment

- Go domain, parser and HTTP tests: passed with the race detector.
- Go vet: passed.
- Svelte check: zero errors and zero warnings.
- Production frontend build: passed.
- Pure matching/calculation benchmark, 10,000 synthetic cases: about 26.7 ms per run and 25.4 MB allocated on the available EPYC host. This excludes uploads, parsing, database writes, exports and network latency; it is not a production end-to-end service target.
- Browser: sign-in screen rendered and was visually inspected. Broader authenticated navigation was not verified because the browser rejected the preview reload.
- PostgreSQL integration: not certified in this environment. Native PostgreSQL could not start under the available execution permissions. A WASM PostgreSQL protocol test runtime failed on COPY protocol synchronization, so its output is not a substitute for the PostgreSQL 18 CI run.
- Docker image startup and backup restoration: not run here; Docker is unavailable.
- GitHub upload: blocked by the connected integration (HTTP 403, Resource not accessible by integration). The handoff includes a complete Git bundle and a push helper; no remote commit is claimed.
