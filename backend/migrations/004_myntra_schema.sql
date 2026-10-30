CREATE SCHEMA IF NOT EXISTS myntra;

ALTER TABLE imports DROP CONSTRAINT IF EXISTS imports_marketplace_check;
ALTER TABLE imports ADD CONSTRAINT imports_marketplace_check CHECK (marketplace IN ('amazon','flipkart','myntra'));
ALTER TABLE import_rows DROP CONSTRAINT IF EXISTS import_rows_marketplace_check;
ALTER TABLE import_rows ADD CONSTRAINT import_rows_marketplace_check CHECK (marketplace IN ('amazon','flipkart','myntra'));
ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_marketplace_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_marketplace_check CHECK (marketplace IN ('amazon','flipkart','myntra'));
ALTER TABLE reconciliation_runs DROP CONSTRAINT IF EXISTS reconciliation_runs_marketplace_check;
ALTER TABLE reconciliation_runs ADD CONSTRAINT reconciliation_runs_marketplace_check CHECK (marketplace IN ('amazon','flipkart','myntra'));
ALTER TABLE results DROP CONSTRAINT IF EXISTS results_marketplace_check;
ALTER TABLE results ADD CONSTRAINT results_marketplace_check CHECK (marketplace IN ('amazon','flipkart','myntra'));
ALTER TABLE audit_logs DROP CONSTRAINT IF EXISTS audit_logs_marketplace_check;
ALTER TABLE audit_logs ADD CONSTRAINT audit_logs_marketplace_check CHECK (marketplace IN ('amazon','flipkart','myntra'));
ALTER TABLE claim_actions DROP CONSTRAINT IF EXISTS claim_actions_marketplace_check;
ALTER TABLE claim_actions ADD CONSTRAINT claim_actions_marketplace_check CHECK (marketplace IN ('amazon','flipkart','myntra'));
ALTER TABLE claim_action_history DROP CONSTRAINT IF EXISTS claim_action_history_marketplace_check;
ALTER TABLE claim_action_history ADD CONSTRAINT claim_action_history_marketplace_check CHECK (marketplace IN ('amazon','flipkart','myntra'));

INSERT INTO claim_rules(version,marketplace,rules) VALUES ('myntra-spf-v1','myntra','{"missing_return":{"start":60,"end":120},"missing_rto":{"start":60,"end":120},"omni_received_issue":{"end":7},"ppmp_received_issue":{"end":30},"payment_search_months":2}'::jsonb) ON CONFLICT (version) DO NOTHING;
CREATE OR REPLACE VIEW myntra.imports AS SELECT * FROM imports WHERE marketplace='myntra' WITH LOCAL CHECK OPTION;
CREATE OR REPLACE VIEW myntra.import_rows AS SELECT * FROM import_rows WHERE marketplace='myntra' WITH LOCAL CHECK OPTION;
CREATE OR REPLACE VIEW myntra.jobs AS SELECT * FROM jobs WHERE marketplace='myntra' WITH LOCAL CHECK OPTION;
CREATE OR REPLACE VIEW myntra.results AS SELECT * FROM results WHERE marketplace='myntra' WITH LOCAL CHECK OPTION;
CREATE OR REPLACE VIEW myntra.claim_actions AS SELECT * FROM claim_actions WHERE marketplace='myntra' WITH LOCAL CHECK OPTION;
CREATE OR REPLACE VIEW myntra.claim_action_history AS SELECT * FROM claim_action_history WHERE marketplace='myntra' WITH LOCAL CHECK OPTION;
CREATE OR REPLACE VIEW myntra.claim_rules AS SELECT * FROM claim_rules WHERE marketplace='myntra' WITH LOCAL CHECK OPTION;
