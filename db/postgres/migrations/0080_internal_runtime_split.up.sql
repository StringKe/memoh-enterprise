-- 0080_internal_runtime_split
-- Durable internal outbox, connector fencing leases, and agent run leases.

CREATE TABLE IF NOT EXISTS event_outbox (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  topic TEXT NOT NULL,
  payload_type TEXT NOT NULL,
  payload BYTEA NOT NULL,
  payload_json JSONB,
  idempotency_key TEXT NOT NULL,
  aggregate_type TEXT,
  aggregate_id UUID,
  partition_key TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT event_outbox_idempotency_key_unique UNIQUE (idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_event_outbox_topic_created
  ON event_outbox(topic, created_at);
CREATE INDEX IF NOT EXISTS idx_event_outbox_aggregate
  ON event_outbox(aggregate_type, aggregate_id)
  WHERE aggregate_type IS NOT NULL AND aggregate_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS event_deliveries (
  event_id UUID NOT NULL REFERENCES event_outbox(id) ON DELETE CASCADE,
  consumer_name TEXT NOT NULL,
  topic TEXT NOT NULL,
  partition_key TEXT,
  available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_by TEXT,
  locked_until TIMESTAMPTZ,
  attempts INTEGER NOT NULL DEFAULT 0,
  max_attempts INTEGER NOT NULL DEFAULT 10,
  last_error TEXT,
  delivered_at TIMESTAMPTZ,
  dead_lettered_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (event_id, consumer_name),
  CONSTRAINT event_deliveries_attempts_check CHECK (attempts >= 0),
  CONSTRAINT event_deliveries_max_attempts_check CHECK (max_attempts > 0)
);

CREATE INDEX IF NOT EXISTS idx_event_deliveries_claim
  ON event_deliveries(consumer_name, topic, available_at)
  WHERE delivered_at IS NULL AND dead_lettered_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_event_deliveries_stale_lock
  ON event_deliveries(consumer_name, locked_until)
  WHERE locked_until IS NOT NULL AND delivered_at IS NULL AND dead_lettered_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_event_deliveries_partition
  ON event_deliveries(consumer_name, topic, partition_key, available_at)
  WHERE delivered_at IS NULL AND dead_lettered_at IS NULL;

CREATE TABLE IF NOT EXISTS channel_connector_leases (
  channel_config_id UUID PRIMARY KEY REFERENCES bot_channel_configs(id) ON DELETE CASCADE,
  channel_type TEXT NOT NULL,
  owner_id TEXT NOT NULL,
  owner_instance_id TEXT NOT NULL,
  lease_version BIGINT NOT NULL DEFAULT 1,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT channel_connector_leases_version_check CHECK (lease_version > 0)
);

CREATE INDEX IF NOT EXISTS idx_channel_connector_leases_type_expires
  ON channel_connector_leases(channel_type, expires_at);
CREATE INDEX IF NOT EXISTS idx_channel_connector_leases_owner
  ON channel_connector_leases(owner_instance_id, expires_at);

CREATE TABLE IF NOT EXISTS agent_run_leases (
  run_id UUID PRIMARY KEY,
  runner_instance_id TEXT NOT NULL,
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  bot_group_id UUID REFERENCES bot_groups(id) ON DELETE SET NULL,
  session_id UUID NOT NULL REFERENCES bot_sessions(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES iam_users(id) ON DELETE CASCADE,
  permission_snapshot_version BIGINT NOT NULL DEFAULT 1,
  allowed_tool_scopes TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
  workspace_executor_target TEXT NOT NULL,
  workspace_id TEXT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  lease_version BIGINT NOT NULL DEFAULT 1,
  revoked_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT agent_run_leases_version_check CHECK (lease_version > 0)
);

CREATE INDEX IF NOT EXISTS idx_agent_run_leases_active
  ON agent_run_leases(run_id, expires_at)
  WHERE revoked_at IS NULL AND completed_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_agent_run_leases_runner
  ON agent_run_leases(runner_instance_id, expires_at)
  WHERE revoked_at IS NULL AND completed_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_agent_run_leases_session
  ON agent_run_leases(session_id, created_at DESC);
