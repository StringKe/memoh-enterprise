-- 0081_structured_data
-- PostgreSQL-backed bot and bot-group structured data spaces.

CREATE TABLE IF NOT EXISTS structured_data_spaces (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_type TEXT NOT NULL,
  owner_bot_id UUID REFERENCES bots(id) ON DELETE CASCADE,
  owner_bot_group_id UUID REFERENCES bot_groups(id) ON DELETE CASCADE,
  schema_name TEXT NOT NULL,
  role_name TEXT NOT NULL,
  display_name TEXT NOT NULL DEFAULT '',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT structured_data_spaces_owner_type_check CHECK (owner_type IN ('bot', 'bot_group')),
  CONSTRAINT structured_data_spaces_owner_check CHECK (
    (owner_type = 'bot' AND owner_bot_id IS NOT NULL AND owner_bot_group_id IS NULL)
    OR (owner_type = 'bot_group' AND owner_bot_id IS NULL AND owner_bot_group_id IS NOT NULL)
  ),
  CONSTRAINT structured_data_spaces_schema_name_unique UNIQUE (schema_name),
  CONSTRAINT structured_data_spaces_role_name_unique UNIQUE (role_name)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_structured_data_spaces_owner_bot
  ON structured_data_spaces(owner_bot_id)
  WHERE owner_type = 'bot';

CREATE UNIQUE INDEX IF NOT EXISTS idx_structured_data_spaces_owner_bot_group
  ON structured_data_spaces(owner_bot_group_id)
  WHERE owner_type = 'bot_group';

CREATE INDEX IF NOT EXISTS idx_structured_data_spaces_owner_type
  ON structured_data_spaces(owner_type);

CREATE TABLE IF NOT EXISTS structured_data_grants (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  space_id UUID NOT NULL REFERENCES structured_data_spaces(id) ON DELETE CASCADE,
  target_type TEXT NOT NULL,
  target_bot_id UUID REFERENCES bots(id) ON DELETE CASCADE,
  target_bot_group_id UUID REFERENCES bot_groups(id) ON DELETE CASCADE,
  privileges TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
  created_by_user_id UUID REFERENCES iam_users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT structured_data_grants_target_type_check CHECK (target_type IN ('bot', 'bot_group')),
  CONSTRAINT structured_data_grants_target_check CHECK (
    (target_type = 'bot' AND target_bot_id IS NOT NULL AND target_bot_group_id IS NULL)
    OR (target_type = 'bot_group' AND target_bot_id IS NULL AND target_bot_group_id IS NOT NULL)
  ),
  CONSTRAINT structured_data_grants_privileges_check CHECK (
    privileges <@ ARRAY['read', 'write', 'ddl']::TEXT[]
  )
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_structured_data_grants_space_target_bot
  ON structured_data_grants(space_id, target_bot_id)
  WHERE target_type = 'bot';

CREATE UNIQUE INDEX IF NOT EXISTS idx_structured_data_grants_space_target_bot_group
  ON structured_data_grants(space_id, target_bot_group_id)
  WHERE target_type = 'bot_group';

CREATE INDEX IF NOT EXISTS idx_structured_data_grants_space_id
  ON structured_data_grants(space_id);

CREATE TABLE IF NOT EXISTS structured_data_audit (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  space_id UUID REFERENCES structured_data_spaces(id) ON DELETE SET NULL,
  actor_type TEXT NOT NULL,
  actor_user_id UUID REFERENCES iam_users(id) ON DELETE SET NULL,
  actor_bot_id UUID REFERENCES bots(id) ON DELETE SET NULL,
  operation TEXT NOT NULL,
  statement TEXT NOT NULL DEFAULT '',
  success BOOLEAN NOT NULL,
  error TEXT NOT NULL DEFAULT '',
  row_count BIGINT NOT NULL DEFAULT 0,
  duration_ms BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT structured_data_audit_actor_type_check CHECK (actor_type IN ('user', 'bot', 'system')),
  CONSTRAINT structured_data_audit_operation_check CHECK (
    operation IN ('ensure_space', 'execute_sql', 'grant', 'revoke', 'describe_space')
  )
);

CREATE INDEX IF NOT EXISTS idx_structured_data_audit_space_created_at
  ON structured_data_audit(space_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_structured_data_audit_actor_bot_created_at
  ON structured_data_audit(actor_bot_id, created_at DESC)
  WHERE actor_bot_id IS NOT NULL;
