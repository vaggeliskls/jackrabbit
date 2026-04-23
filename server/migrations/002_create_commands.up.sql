CREATE TABLE commands (
  id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  target_type        TEXT NOT NULL,
  target_value       TEXT NOT NULL,
  payload            JSONB NOT NULL,
  status             TEXT NOT NULL DEFAULT 'queued',
  assigned_runner    TEXT REFERENCES runners(slug),
  retry_count        INT             DEFAULT 0,
  max_retries        INT             DEFAULT 0,
  timeout_secs       INT             DEFAULT 300,
  exit_code          INT,
  error_message      TEXT,
  kill_requested_at  TIMESTAMPTZ,
  deadline           TIMESTAMPTZ,
  created_at         TIMESTAMPTZ     DEFAULT now(),
  claimed_at         TIMESTAMPTZ,
  started_at         TIMESTAMPTZ,
  finished_at        TIMESTAMPTZ
);

CREATE INDEX idx_commands_status_target ON commands(status, target_type, target_value);
CREATE INDEX idx_commands_assigned_status ON commands(assigned_runner, status);
CREATE INDEX idx_commands_deadline ON commands(deadline) WHERE status = 'queued';
CREATE INDEX idx_commands_status ON commands(status);
