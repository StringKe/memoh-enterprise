-- 0078_connectrpc_bot_groups
-- Add Bot Groups, Bot settings override masks, and enterprise integration API tokens.

CREATE TABLE IF NOT EXISTS bot_groups (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_user_id UUID NOT NULL REFERENCES iam_users(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT bot_groups_owner_name_unique UNIQUE (owner_user_id, name)
);

CREATE INDEX IF NOT EXISTS idx_bot_groups_owner_user_id ON bot_groups(owner_user_id);

CREATE TABLE IF NOT EXISTS bot_group_settings (
  group_id UUID PRIMARY KEY REFERENCES bot_groups(id) ON DELETE CASCADE,
  timezone TEXT,
  language TEXT,
  reasoning_enabled BOOLEAN,
  reasoning_effort TEXT,
  chat_model_id UUID REFERENCES models(id) ON DELETE SET NULL,
  search_provider_id UUID REFERENCES search_providers(id) ON DELETE SET NULL,
  memory_provider_id UUID REFERENCES memory_providers(id) ON DELETE SET NULL,
  heartbeat_enabled BOOLEAN,
  heartbeat_interval INTEGER,
  heartbeat_prompt TEXT,
  heartbeat_model_id UUID REFERENCES models(id) ON DELETE SET NULL,
  compaction_enabled BOOLEAN,
  compaction_threshold INTEGER,
  compaction_ratio INTEGER,
  compaction_model_id UUID REFERENCES models(id) ON DELETE SET NULL,
  title_model_id UUID REFERENCES models(id) ON DELETE SET NULL,
  image_model_id UUID REFERENCES models(id) ON DELETE SET NULL,
  discuss_probe_model_id UUID REFERENCES models(id) ON DELETE SET NULL,
  tts_model_id UUID REFERENCES models(id) ON DELETE SET NULL,
  transcription_model_id UUID REFERENCES models(id) ON DELETE SET NULL,
  browser_context_id UUID REFERENCES browser_contexts(id) ON DELETE SET NULL,
  persist_full_tool_results BOOLEAN,
  show_tool_calls_in_im BOOLEAN,
  tool_approval_config JSONB,
  overlay_provider TEXT,
  overlay_enabled BOOLEAN,
  overlay_config JSONB,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT bot_group_settings_reasoning_effort_check CHECK (
    reasoning_effort IS NULL OR reasoning_effort IN ('low', 'medium', 'high')
  )
);

ALTER TABLE bots
  ADD COLUMN IF NOT EXISTS group_id UUID REFERENCES bot_groups(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS settings_override_mask JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE INDEX IF NOT EXISTS idx_bots_group_id ON bots(group_id);

UPDATE bots
SET settings_override_mask = jsonb_build_object(
  'timezone', true,
  'language', true,
  'reasoning_enabled', true,
  'reasoning_effort', true,
  'chat_model_id', true,
  'search_provider_id', true,
  'memory_provider_id', true,
  'heartbeat_enabled', true,
  'heartbeat_interval', true,
  'heartbeat_prompt', true,
  'heartbeat_model_id', true,
  'compaction_enabled', true,
  'compaction_threshold', true,
  'compaction_ratio', true,
  'compaction_model_id', true,
  'title_model_id', true,
  'image_model_id', true,
  'discuss_probe_model_id', true,
  'tts_model_id', true,
  'transcription_model_id', true,
  'browser_context_id', true,
  'persist_full_tool_results', true,
  'show_tool_calls_in_im', true,
  'tool_approval_config', true,
  'overlay_provider', true,
  'overlay_enabled', true,
  'overlay_config', true
)
WHERE settings_override_mask = '{}'::jsonb;

CREATE TABLE IF NOT EXISTS integration_api_tokens (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  token_hash TEXT NOT NULL,
  scope_type TEXT NOT NULL,
  scope_bot_id UUID REFERENCES bots(id) ON DELETE CASCADE,
  scope_bot_group_id UUID REFERENCES bot_groups(id) ON DELETE CASCADE,
  allowed_event_types JSONB NOT NULL DEFAULT '[]'::jsonb,
  allowed_action_types JSONB NOT NULL DEFAULT '[]'::jsonb,
  expires_at TIMESTAMPTZ,
  disabled_at TIMESTAMPTZ,
  last_used_at TIMESTAMPTZ,
  created_by_user_id UUID REFERENCES iam_users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT integration_api_tokens_token_hash_unique UNIQUE (token_hash),
  CONSTRAINT integration_api_tokens_scope_type_check CHECK (scope_type IN ('global', 'bot', 'bot_group')),
  CONSTRAINT integration_api_tokens_scope_check CHECK (
    (scope_type = 'global' AND scope_bot_id IS NULL AND scope_bot_group_id IS NULL)
    OR (scope_type = 'bot' AND scope_bot_id IS NOT NULL AND scope_bot_group_id IS NULL)
    OR (scope_type = 'bot_group' AND scope_bot_id IS NULL AND scope_bot_group_id IS NOT NULL)
  )
);

CREATE INDEX IF NOT EXISTS idx_integration_api_tokens_scope_bot_id
  ON integration_api_tokens(scope_bot_id);

CREATE INDEX IF NOT EXISTS idx_integration_api_tokens_scope_bot_group_id
  ON integration_api_tokens(scope_bot_group_id);

CREATE INDEX IF NOT EXISTS idx_integration_api_tokens_created_by_user_id
  ON integration_api_tokens(created_by_user_id);
