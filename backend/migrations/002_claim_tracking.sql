CREATE TABLE IF NOT EXISTS claim_actions (
  case_key text PRIMARY KEY,
  claim_raised boolean NOT NULL DEFAULT false,
  claim_raised_date date,
  claim_status text NOT NULL DEFAULT 'Not Raised' CHECK (claim_status IN ('Not Raised','Pending','Approved','Rejected','Partially Approved')),
  claim_updated_at timestamptz NOT NULL DEFAULT now(),
  amazon_claim_id text NOT NULL DEFAULT '',
  claim_amount_requested bigint NOT NULL DEFAULT 0,
  claim_amount_approved bigint NOT NULL DEFAULT 0,
  claim_rejection_reason text NOT NULL DEFAULT '',
  claim_notes text NOT NULL DEFAULT '',
  user_notes text NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS claim_actions_status_idx ON claim_actions(claim_status, claim_updated_at DESC);

CREATE TABLE IF NOT EXISTS claim_action_history (
  id bigserial PRIMARY KEY,
  case_key text NOT NULL,
  previous jsonb NOT NULL DEFAULT '{}',
  current jsonb NOT NULL DEFAULT '{}',
  changed_by text NOT NULL DEFAULT 'Admin',
  changed_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS claim_action_history_case_idx ON claim_action_history(case_key, changed_at DESC);
INSERT INTO schema_versions(version) VALUES(2) ON CONFLICT DO NOTHING;
