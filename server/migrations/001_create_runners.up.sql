CREATE TABLE runners (
  slug               TEXT PRIMARY KEY,
  name               TEXT NOT NULL,
  tags               TEXT[]          DEFAULT '{}',
  status             TEXT            DEFAULT 'offline',
  concurrency_limit  INT             DEFAULT 4,
  gpu_capable        BOOLEAN         DEFAULT false,
  active_count       INT             DEFAULT 0,
  last_seen          TIMESTAMPTZ,
  orphaned_at        TIMESTAMPTZ,
  created_at         TIMESTAMPTZ     DEFAULT now(),
  updated_at         TIMESTAMPTZ     DEFAULT now()
);

CREATE INDEX idx_runners_status ON runners(status);
CREATE INDEX idx_runners_tags ON runners USING GIN(tags);
