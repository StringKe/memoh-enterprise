-- name: CreateBot :one
INSERT INTO bots (owner_user_id, group_id, display_name, avatar_url, timezone, is_active, display_enabled, metadata, status, settings_override_mask)
VALUES (
  sqlc.arg(owner_user_id),
  sqlc.narg(group_id),
  sqlc.arg(display_name),
  sqlc.arg(avatar_url),
  sqlc.arg(timezone),
  sqlc.arg(is_active),
  sqlc.arg(display_enabled),
  sqlc.arg(metadata),
  COALESCE(NULLIF(sqlc.arg(status)::text, ''), 'ready'),
  COALESCE(sqlc.narg(settings_override_mask)::jsonb, '{}'::jsonb)
)
RETURNING id, owner_user_id, group_id, display_name, avatar_url, timezone, is_active, status, language, reasoning_enabled, reasoning_effort, chat_model_id, search_provider_id, memory_provider_id, heartbeat_enabled, heartbeat_interval, heartbeat_prompt, display_enabled, settings_override_mask, metadata, created_at, updated_at;

-- name: GetBotByID :one
SELECT id, owner_user_id, group_id, display_name, avatar_url, timezone, is_active, status, language, reasoning_enabled, reasoning_effort, chat_model_id, search_provider_id, memory_provider_id, heartbeat_enabled, heartbeat_interval, heartbeat_prompt, display_enabled, compaction_enabled, compaction_threshold, compaction_ratio, compaction_model_id, settings_override_mask, metadata, created_at, updated_at
FROM bots
WHERE id = $1;

-- name: ListBotsByOwner :many
SELECT id, owner_user_id, group_id, display_name, avatar_url, timezone, is_active, status, language, reasoning_enabled, reasoning_effort, chat_model_id, search_provider_id, memory_provider_id, heartbeat_enabled, heartbeat_interval, heartbeat_prompt, display_enabled, settings_override_mask, metadata, created_at, updated_at
FROM bots
WHERE owner_user_id = $1
ORDER BY created_at DESC;

-- name: ListAccessibleBots :many
SELECT bots.id, bots.owner_user_id, bots.group_id, bots.display_name, bots.avatar_url, bots.timezone, bots.is_active, bots.status, bots.language, bots.reasoning_enabled, bots.reasoning_effort, bots.chat_model_id, bots.search_provider_id, bots.memory_provider_id, bots.heartbeat_enabled, bots.heartbeat_interval, bots.heartbeat_prompt, bots.display_enabled, bots.settings_override_mask, bots.metadata, bots.created_at, bots.updated_at
FROM bots
LEFT JOIN bot_groups ON bot_groups.id = bots.group_id
WHERE bots.owner_user_id = sqlc.arg(user_id)
  OR bot_groups.visibility IN ('organization', 'public')
  OR EXISTS (
    SELECT 1
    FROM iam_principal_roles pr
    JOIN iam_roles r ON r.id = pr.role_id
    JOIN iam_role_permissions rp ON rp.role_id = r.id
    JOIN iam_permissions p ON p.id = rp.permission_id
    WHERE p.key = 'bot.read'
      AND pr.resource_type = 'bot'
      AND (pr.resource_id = bots.id OR pr.resource_id IS NULL)
      AND (
        (pr.principal_type = 'user' AND pr.principal_id = sqlc.arg(user_id))
        OR (
          pr.principal_type = 'group'
          AND pr.principal_id IN (
            SELECT group_id FROM iam_group_members WHERE user_id = sqlc.arg(user_id)
          )
        )
      )
  )
  OR EXISTS (
    SELECT 1
    FROM iam_principal_roles pr
    JOIN iam_roles r ON r.id = pr.role_id
    JOIN iam_role_permissions rp ON rp.role_id = r.id
    JOIN iam_permissions p ON p.id = rp.permission_id
    WHERE p.key = 'bot_group.read'
      AND pr.resource_type = 'bot_group'
      AND bots.group_id IS NOT NULL
      AND (pr.resource_id = bots.group_id OR pr.resource_id IS NULL)
      AND (
        (pr.principal_type = 'user' AND pr.principal_id = sqlc.arg(user_id))
        OR (
          pr.principal_type = 'group'
          AND pr.principal_id IN (
            SELECT group_id FROM iam_group_members WHERE user_id = sqlc.arg(user_id)
          )
        )
      )
  )
  OR EXISTS (
    SELECT 1
    FROM iam_principal_roles pr
    JOIN iam_roles r ON r.id = pr.role_id
    JOIN iam_role_permissions rp ON rp.role_id = r.id
    JOIN iam_permissions p ON p.id = rp.permission_id
    WHERE p.key = 'system.admin'
      AND pr.resource_type = 'system'
      AND pr.resource_id IS NULL
      AND (
        (pr.principal_type = 'user' AND pr.principal_id = sqlc.arg(user_id))
        OR (
          pr.principal_type = 'group'
          AND pr.principal_id IN (
            SELECT group_id FROM iam_group_members WHERE user_id = sqlc.arg(user_id)
          )
        )
      )
  )
ORDER BY bots.created_at DESC;

-- name: UpdateBotProfile :one
UPDATE bots
SET display_name = sqlc.arg(display_name),
    avatar_url = sqlc.arg(avatar_url),
    timezone = sqlc.arg(timezone),
    is_active = sqlc.arg(is_active),
    group_id = sqlc.narg(group_id),
    display_enabled = sqlc.arg(display_enabled),
    metadata = sqlc.arg(metadata),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id, owner_user_id, group_id, display_name, avatar_url, timezone, is_active, status, language, reasoning_enabled, reasoning_effort, chat_model_id, search_provider_id, memory_provider_id, heartbeat_enabled, heartbeat_interval, heartbeat_prompt, display_enabled, settings_override_mask, metadata, created_at, updated_at;

-- name: UpdateBotOwner :one
UPDATE bots
SET owner_user_id = $2,
    updated_at = now()
WHERE id = $1
RETURNING id, owner_user_id, group_id, display_name, avatar_url, timezone, is_active, status, language, reasoning_enabled, reasoning_effort, chat_model_id, search_provider_id, memory_provider_id, heartbeat_enabled, heartbeat_interval, heartbeat_prompt, display_enabled, settings_override_mask, metadata, created_at, updated_at;

-- name: UpdateBotStatus :exec
UPDATE bots
SET status = $2,
    updated_at = now()
WHERE id = $1;

-- name: DeleteBotByID :exec
DELETE FROM bots WHERE id = $1;

-- name: ListHeartbeatEnabledBots :many
WITH effective AS (
  SELECT
    bots.id,
    bots.owner_user_id,
    bots.status,
    CASE
      WHEN bots.group_id IS NOT NULL
        AND NOT COALESCE((bots.settings_override_mask->>'heartbeat_enabled')::boolean, false)
        THEN COALESCE(bot_group_settings.heartbeat_enabled, false)
      ELSE bots.heartbeat_enabled
    END::boolean AS heartbeat_enabled,
    CASE
      WHEN bots.group_id IS NOT NULL
        AND NOT COALESCE((bots.settings_override_mask->>'heartbeat_interval')::boolean, false)
        THEN COALESCE(bot_group_settings.heartbeat_interval, 30)
      ELSE bots.heartbeat_interval
    END::integer AS heartbeat_interval,
    CASE
      WHEN bots.group_id IS NOT NULL
        AND NOT COALESCE((bots.settings_override_mask->>'heartbeat_prompt')::boolean, false)
        THEN COALESCE(bot_group_settings.heartbeat_prompt, '')
      ELSE bots.heartbeat_prompt
    END::text AS heartbeat_prompt
  FROM bots
  LEFT JOIN bot_group_settings ON bot_group_settings.group_id = bots.group_id
)
SELECT id, owner_user_id, heartbeat_enabled, heartbeat_interval, heartbeat_prompt
FROM effective
WHERE heartbeat_enabled = true AND status = 'ready';

-- name: GetBotHeartbeatConfig :one
SELECT
  bots.id,
  bots.owner_user_id,
  bots.status,
  CASE
    WHEN bots.group_id IS NOT NULL
      AND NOT COALESCE((bots.settings_override_mask->>'heartbeat_enabled')::boolean, false)
      THEN COALESCE(bot_group_settings.heartbeat_enabled, false)
    ELSE bots.heartbeat_enabled
  END::boolean AS heartbeat_enabled,
  CASE
    WHEN bots.group_id IS NOT NULL
      AND NOT COALESCE((bots.settings_override_mask->>'heartbeat_interval')::boolean, false)
      THEN COALESCE(bot_group_settings.heartbeat_interval, 30)
    ELSE bots.heartbeat_interval
  END::integer AS heartbeat_interval,
  CASE
    WHEN bots.group_id IS NOT NULL
      AND NOT COALESCE((bots.settings_override_mask->>'heartbeat_prompt')::boolean, false)
      THEN COALESCE(bot_group_settings.heartbeat_prompt, '')
    ELSE bots.heartbeat_prompt
  END::text AS heartbeat_prompt
FROM bots
LEFT JOIN bot_group_settings ON bot_group_settings.group_id = bots.group_id
WHERE bots.id = $1;
