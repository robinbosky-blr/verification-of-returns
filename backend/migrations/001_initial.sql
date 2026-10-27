CREATE TABLE IF NOT EXISTS schema_versions(version integer PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS imports (
 id text PRIMARY KEY, marketplace text NOT NULL DEFAULT 'amazon' CHECK(marketplace='amazon'),
 kind text NOT NULL CHECK(kind IN ('website','physical','payment')), filename text NOT NULL,
 sha256 text NOT NULL, object_key text NOT NULL, coverage_month text NOT NULL DEFAULT '',
 status text NOT NULL DEFAULT 'queued', active boolean NOT NULL DEFAULT true,
 rows_count integer NOT NULL DEFAULT 0, error text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(), completed_at timestamptz,
 UNIQUE(marketplace,kind,sha256,coverage_month)
);
CREATE TABLE IF NOT EXISTS import_rows (
 import_id text NOT NULL REFERENCES imports(id), kind text NOT NULL, fingerprint text NOT NULL,
 occurrence integer NOT NULL, order_id text NOT NULL, sku text NOT NULL, row_date date NOT NULL,
 payload jsonb NOT NULL, PRIMARY KEY(import_id,fingerprint,occurrence)
);
CREATE INDEX IF NOT EXISTS rows_order ON import_rows(order_id,sku);
CREATE INDEX IF NOT EXISTS rows_date ON import_rows(kind,row_date);
CREATE TABLE IF NOT EXISTS jobs (
 id bigserial PRIMARY KEY, import_id text REFERENCES imports(id), kind text NOT NULL,
 status text NOT NULL DEFAULT 'queued', attempts integer NOT NULL DEFAULT 0,
 error text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(), finished_at timestamptz
);
CREATE INDEX IF NOT EXISTS jobs_queue ON jobs(id) WHERE status='queued';
CREATE TABLE IF NOT EXISTS reconciliation_runs (
 id bigserial PRIMARY KEY, rule_version text NOT NULL, status text NOT NULL DEFAULT 'running',
 summary jsonb, created_at timestamptz NOT NULL DEFAULT now(), completed_at timestamptz
);
CREATE TABLE IF NOT EXISTS results (
 case_key text PRIMARY KEY, order_id text NOT NULL, sku text NOT NULL, month text NOT NULL,
 receipt text NOT NULL, financial text NOT NULL, run_id bigint NOT NULL REFERENCES reconciliation_runs(id), payload jsonb NOT NULL
);
CREATE INDEX IF NOT EXISTS results_period ON results(month,receipt,financial);
CREATE INDEX IF NOT EXISTS results_order ON results(order_id,sku);
CREATE TABLE IF NOT EXISTS audit_logs (id bigserial PRIMARY KEY, action text NOT NULL, details jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS sessions (hash text PRIMARY KEY, expires_at timestamptz NOT NULL);
CREATE TABLE IF NOT EXISTS claim_rules (version text PRIMARY KEY, marketplace text NOT NULL, rules jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now());
INSERT INTO claim_rules(version,marketplace,rules) VALUES('amazon-seller-v1','amazon','{"delivered_days":15,"not_received_open_days":50,"not_received_expiry_days":75,"timezone":"Asia/Kolkata","source":"User approved workflow","fba":"separate, review only"}') ON CONFLICT DO NOTHING;
INSERT INTO schema_versions(version) VALUES(1) ON CONFLICT DO NOTHING;
