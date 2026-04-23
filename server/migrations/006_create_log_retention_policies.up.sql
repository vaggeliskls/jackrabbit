CREATE TABLE log_retention_policies (
  id           SERIAL PRIMARY KEY,
  scope        TEXT NOT NULL,
  scope_id     TEXT,
  max_age_days INT,
  max_size_mb  INT,
  created_at   TIMESTAMPTZ DEFAULT now(),
  updated_at   TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_log_retention_scope ON log_retention_policies(scope, scope_id);
