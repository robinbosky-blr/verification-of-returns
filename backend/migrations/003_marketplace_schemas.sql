CREATE SCHEMA IF NOT EXISTS amazon;
CREATE SCHEMA IF NOT EXISTS flipkart;

ALTER TABLE imports DROP CONSTRAINT IF EXISTS imports_marketplace_check;
ALTER TABLE imports ADD CONSTRAINT imports_marketplace_check CHECK (marketplace IN ('amazon','flipkart'));

ALTER TABLE import_rows ADD COLUMN IF NOT EXISTS marketplace text NOT NULL DEFAULT 'amazon' CHECK (marketplace IN ('amazon','flipkart'));
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS marketplace text NOT NULL DEFAULT 'amazon' CHECK (marketplace IN ('amazon','flipkart'));
ALTER TABLE reconciliation_runs ADD COLUMN IF NOT EXISTS marketplace text NOT NULL DEFAULT 'amazon' CHECK (marketplace IN ('amazon','flipkart'));
ALTER TABLE results ADD COLUMN IF NOT EXISTS marketplace text NOT NULL DEFAULT 'amazon' CHECK (marketplace IN ('amazon','flipkart'));
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS marketplace text NOT NULL DEFAULT 'amazon' CHECK (marketplace IN ('amazon','flipkart'));
ALTER TABLE claim_actions ADD COLUMN IF NOT EXISTS marketplace text NOT NULL DEFAULT 'amazon' CHECK (marketplace IN ('amazon','flipkart'));
ALTER TABLE claim_action_history ADD COLUMN IF NOT EXISTS marketplace text NOT NULL DEFAULT 'amazon' CHECK (marketplace IN ('amazon','flipkart'));

ALTER TABLE results DROP CONSTRAINT IF EXISTS results_pkey;
ALTER TABLE results ADD CONSTRAINT results_pkey PRIMARY KEY (marketplace,case_key);
ALTER TABLE claim_actions DROP CONSTRAINT IF EXISTS claim_actions_pkey;
ALTER TABLE claim_actions ADD CONSTRAINT claim_actions_pkey PRIMARY KEY (marketplace,case_key);

CREATE INDEX IF NOT EXISTS rows_marketplace_order ON import_rows(marketplace,order_id,sku);
CREATE INDEX IF NOT EXISTS jobs_marketplace_queue ON jobs(marketplace,id) WHERE status='queued';
CREATE INDEX IF NOT EXISTS results_marketplace_period ON results(marketplace,month,receipt,financial);
CREATE INDEX IF NOT EXISTS claim_actions_marketplace_status ON claim_actions(marketplace,claim_status,claim_updated_at DESC);

INSERT INTO claim_rules(version,marketplace,rules) VALUES(
  'flipkart-spf-v1',
  'flipkart',
  '{"damaged_defective_wrong_received_days":14,"product_missing_open_days":60,"product_missing_expiry_days":120,"dne_open_days":60,"dne_expiry_days":120,"in_transit_received_wait_hours":24,"rejected_appeal_days":7,"decision_tat_days":12,"timezone":"Asia/Kolkata","source":"User approved Flipkart workflow"}'
) ON CONFLICT DO NOTHING;

CREATE OR REPLACE VIEW amazon.imports AS SELECT * FROM public.imports WHERE marketplace='amazon' WITH LOCAL CHECK OPTION;
CREATE OR REPLACE VIEW flipkart.imports AS SELECT * FROM public.imports WHERE marketplace='flipkart' WITH LOCAL CHECK OPTION;
CREATE OR REPLACE VIEW amazon.import_rows AS SELECT * FROM public.import_rows WHERE marketplace='amazon' WITH LOCAL CHECK OPTION;
CREATE OR REPLACE VIEW flipkart.import_rows AS SELECT * FROM public.import_rows WHERE marketplace='flipkart' WITH LOCAL CHECK OPTION;
CREATE OR REPLACE VIEW amazon.jobs AS SELECT * FROM public.jobs WHERE marketplace='amazon' WITH LOCAL CHECK OPTION;
CREATE OR REPLACE VIEW flipkart.jobs AS SELECT * FROM public.jobs WHERE marketplace='flipkart' WITH LOCAL CHECK OPTION;
CREATE OR REPLACE VIEW amazon.results AS SELECT * FROM public.results WHERE marketplace='amazon' WITH LOCAL CHECK OPTION;
CREATE OR REPLACE VIEW flipkart.results AS SELECT * FROM public.results WHERE marketplace='flipkart' WITH LOCAL CHECK OPTION;
CREATE OR REPLACE VIEW amazon.claim_actions AS SELECT * FROM public.claim_actions WHERE marketplace='amazon' WITH LOCAL CHECK OPTION;
CREATE OR REPLACE VIEW flipkart.claim_actions AS SELECT * FROM public.claim_actions WHERE marketplace='flipkart' WITH LOCAL CHECK OPTION;
CREATE OR REPLACE VIEW amazon.claim_action_history AS SELECT * FROM public.claim_action_history WHERE marketplace='amazon' WITH LOCAL CHECK OPTION;
CREATE OR REPLACE VIEW flipkart.claim_action_history AS SELECT * FROM public.claim_action_history WHERE marketplace='flipkart' WITH LOCAL CHECK OPTION;
CREATE OR REPLACE VIEW amazon.claim_rules AS SELECT * FROM public.claim_rules WHERE marketplace='amazon' WITH LOCAL CHECK OPTION;
CREATE OR REPLACE VIEW flipkart.claim_rules AS SELECT * FROM public.claim_rules WHERE marketplace='flipkart' WITH LOCAL CHECK OPTION;

INSERT INTO schema_versions(version) VALUES(3) ON CONFLICT DO NOTHING;
