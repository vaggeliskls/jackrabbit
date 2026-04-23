CREATE TABLE metric_rollups (
  id          BIGSERIAL PRIMARY KEY,
  command_id  UUID        REFERENCES commands(id) ON DELETE CASCADE,
  runner_slug TEXT        REFERENCES runners(slug),
  resolution  TEXT        NOT NULL,
  bucket_ts   TIMESTAMPTZ NOT NULL,
  avg_cpu     FLOAT,
  avg_mem     FLOAT,
  avg_gpu     FLOAT,
  avg_gpu_mem FLOAT,
  UNIQUE (command_id, runner_slug, resolution, bucket_ts)
);

CREATE INDEX idx_metric_rollups_command_resolution ON metric_rollups(command_id, resolution, bucket_ts);
CREATE INDEX idx_metric_rollups_resolution_ts ON metric_rollups(resolution, bucket_ts);
