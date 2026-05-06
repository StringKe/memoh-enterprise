-- name: CreateBotGroup :one
INSERT INTO bot_groups (owner_user_id, name, description, visibility, metadata)
VALUES (sqlc.arg(owner_user_id), sqlc.arg(name), sqlc.arg(description), sqlc.arg(visibility), sqlc.arg(metadata))
RETURNING id, owner_user_id, name, description, visibility, metadata, created_at, updated_at;

-- name: GetBotGroupByID :one
SELECT id, owner_user_id, name, description, visibility, metadata, created_at, updated_at
FROM bot_groups
WHERE id = sqlc.arg(id);

-- name: GetBotGroupByOwnerAndID :one
SELECT id, owner_user_id, name, description, visibility, metadata, created_at, updated_at
FROM bot_groups
WHERE owner_user_id = sqlc.arg(owner_user_id)
  AND id = sqlc.arg(id);

-- name: ListAccessibleBotGroups :many
SELECT id, owner_user_id, name, description, visibility, metadata, created_at, updated_at
FROM bot_groups
WHERE owner_user_id = sqlc.arg(user_id)
  OR visibility IN ('organization', 'public')
  OR EXISTS (
    SELECT 1
    FROM iam_principal_roles pr
    JOIN iam_roles r ON r.id = pr.role_id
    JOIN iam_role_permissions rp ON rp.role_id = r.id
    JOIN iam_permissions p ON p.id = rp.permission_id
    WHERE p.key = 'bot_group.read'
      AND pr.resource_type = 'bot_group'
      AND (pr.resource_id = bot_groups.id OR pr.resource_id IS NULL)
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
ORDER BY name ASC, created_at DESC;

-- name: UpdateBotGroup :one
UPDATE bot_groups
SET name = sqlc.arg(name),
    description = sqlc.arg(description),
    visibility = sqlc.arg(visibility),
    metadata = sqlc.arg(metadata),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id, owner_user_id, name, description, visibility, metadata, created_at, updated_at;

-- name: DeleteBotGroup :exec
DELETE FROM bot_groups
WHERE id = sqlc.arg(id);

-- name: CountBotsInGroup :one
SELECT count(*)::bigint
FROM bots
WHERE group_id = sqlc.arg(group_id);

-- name: UpsertBotGroupSettings :one
INSERT INTO bot_group_settings (
  group_id,
  timezone,
  language,
  reasoning_enabled,
  reasoning_effort,
  chat_model_id,
  search_provider_id,
  memory_provider_id,
  heartbeat_enabled,
  heartbeat_interval,
  heartbeat_prompt,
  heartbeat_model_id,
  compaction_enabled,
  compaction_threshold,
  compaction_ratio,
  compaction_model_id,
  title_model_id,
  image_model_id,
  discuss_probe_model_id,
  tts_model_id,
  transcription_model_id,
  browser_context_id,
  persist_full_tool_results,
  show_tool_calls_in_im,
  tool_approval_config,
  overlay_provider,
  overlay_enabled,
  overlay_config,
  metadata
) VALUES (
  sqlc.arg(group_id),
  sqlc.narg(timezone),
  sqlc.narg(language),
  sqlc.narg(reasoning_enabled),
  sqlc.narg(reasoning_effort),
  sqlc.narg(chat_model_id),
  sqlc.narg(search_provider_id),
  sqlc.narg(memory_provider_id),
  sqlc.narg(heartbeat_enabled),
  sqlc.narg(heartbeat_interval),
  sqlc.narg(heartbeat_prompt),
  sqlc.narg(heartbeat_model_id),
  sqlc.narg(compaction_enabled),
  sqlc.narg(compaction_threshold),
  sqlc.narg(compaction_ratio),
  sqlc.narg(compaction_model_id),
  sqlc.narg(title_model_id),
  sqlc.narg(image_model_id),
  sqlc.narg(discuss_probe_model_id),
  sqlc.narg(tts_model_id),
  sqlc.narg(transcription_model_id),
  sqlc.narg(browser_context_id),
  sqlc.narg(persist_full_tool_results),
  sqlc.narg(show_tool_calls_in_im),
  sqlc.narg(tool_approval_config),
  sqlc.narg(overlay_provider),
  sqlc.narg(overlay_enabled),
  sqlc.narg(overlay_config),
  sqlc.arg(metadata)
)
ON CONFLICT (group_id) DO UPDATE SET
  timezone = EXCLUDED.timezone,
  language = EXCLUDED.language,
  reasoning_enabled = EXCLUDED.reasoning_enabled,
  reasoning_effort = EXCLUDED.reasoning_effort,
  chat_model_id = EXCLUDED.chat_model_id,
  search_provider_id = EXCLUDED.search_provider_id,
  memory_provider_id = EXCLUDED.memory_provider_id,
  heartbeat_enabled = EXCLUDED.heartbeat_enabled,
  heartbeat_interval = EXCLUDED.heartbeat_interval,
  heartbeat_prompt = EXCLUDED.heartbeat_prompt,
  heartbeat_model_id = EXCLUDED.heartbeat_model_id,
  compaction_enabled = EXCLUDED.compaction_enabled,
  compaction_threshold = EXCLUDED.compaction_threshold,
  compaction_ratio = EXCLUDED.compaction_ratio,
  compaction_model_id = EXCLUDED.compaction_model_id,
  title_model_id = EXCLUDED.title_model_id,
  image_model_id = EXCLUDED.image_model_id,
  discuss_probe_model_id = EXCLUDED.discuss_probe_model_id,
  tts_model_id = EXCLUDED.tts_model_id,
  transcription_model_id = EXCLUDED.transcription_model_id,
  browser_context_id = EXCLUDED.browser_context_id,
  persist_full_tool_results = EXCLUDED.persist_full_tool_results,
  show_tool_calls_in_im = EXCLUDED.show_tool_calls_in_im,
  tool_approval_config = EXCLUDED.tool_approval_config,
  overlay_provider = EXCLUDED.overlay_provider,
  overlay_enabled = EXCLUDED.overlay_enabled,
  overlay_config = EXCLUDED.overlay_config,
  metadata = EXCLUDED.metadata,
  updated_at = now()
RETURNING *;

-- name: GetBotGroupSettings :one
SELECT *
FROM bot_group_settings
WHERE group_id = sqlc.arg(group_id);

-- name: DeleteBotGroupSettings :exec
DELETE FROM bot_group_settings
WHERE group_id = sqlc.arg(group_id);
