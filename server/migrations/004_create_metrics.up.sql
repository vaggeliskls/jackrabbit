CREATE TABLE metrics (
  id          BIGSERIAL PRIMARY KEY,
  command_id  UUID        REFERENCES commands(id) ON DELETE CASCADE,
  runner_slug TEXT        REFERENCES runners(slug),
  cpu_percent FLOAT,
  mem_mb      FLOAT,
  gpu_percent FLOAT,
  gpu_mem_mb  FLOAT,
  rolled_up   BOOLEAN     DEFAULT false,
  sample_ts   TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_metrics_command_ts ON metrics(command_id, sample_ts);
CREATE INDEX idx_metrics_rolled_up ON metrics(rolled_up, sample_ts) WHERE rolled_up = false;
CREATE INDEX idx_metrics_sample_ts ON metrics(sample_ts);
