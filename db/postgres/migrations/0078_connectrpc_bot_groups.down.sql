-- 0078_connectrpc_bot_groups
-- Roll back Bot Groups and enterprise integration API tokens.
-- Destructive rollback: Bot Group assignments, group defaults, and integration token records are dropped.
-- Rollback test note: data loss for Bot Groups and Integration Tokens is intentional by design.

DROP INDEX IF EXISTS idx_integration_api_tokens_created_by_user_id;
DROP INDEX IF EXISTS idx_integration_api_tokens_scope_bot_group_id;
DROP INDEX IF EXISTS idx_integration_api_tokens_scope_bot_id;
DROP TABLE IF EXISTS integration_api_tokens;

DROP TABLE IF EXISTS bot_group_settings;

UPDATE bots SET group_id = NULL WHERE group_id IS NOT NULL;
DROP INDEX IF EXISTS idx_bots_group_id;

ALTER TABLE bots
  DROP COLUMN IF EXISTS settings_override_mask,
  DROP COLUMN IF EXISTS group_id;

DROP INDEX IF EXISTS idx_bot_groups_owner_user_id;
DROP TABLE IF EXISTS bot_groups;
