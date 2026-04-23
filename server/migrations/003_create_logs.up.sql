CREATE TABLE logs (
  id          BIGSERIAL PRIMARY KEY,
  command_id  UUID        REFERENCES commands(id) ON DELETE CASCADE,
  runner_slug TEXT        REFERENCES runners(slug),
  source      TEXT        NOT NULL,
  level       TEXT,
  line        TEXT        NOT NULL,
  seq         BIGINT      NOT NULL,
  ts          TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_logs_command_seq ON logs(command_id, seq);
CREATE INDEX idx_logs_runner_ts ON logs(runner_slug, ts);
CREATE INDEX idx_logs_ts ON logs(ts);
