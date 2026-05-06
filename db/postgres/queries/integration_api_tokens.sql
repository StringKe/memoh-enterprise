-- name: CreateIntegrationAPIToken :one
INSERT INTO integration_api_tokens (
  name,
  token_hash,
  scope_type,
  scope_bot_id,
  scope_bot_group_id,
  allowed_event_types,
  allowed_action_types,
  expires_at,
  created_by_user_id
) VALUES (
  sqlc.arg(name),
  sqlc.arg(token_hash),
  sqlc.arg(scope_type),
  sqlc.narg(scope_bot_id),
  sqlc.narg(scope_bot_group_id),
  sqlc.arg(allowed_event_types),
  sqlc.arg(allowed_action_types),
  sqlc.narg(expires_at),
  sqlc.narg(created_by_user_id)
)
RETURNING *;

-- name: GetIntegrationAPITokenByID :one
SELECT *
FROM integration_api_tokens
WHERE id = sqlc.arg(id);

-- name: GetIntegrationAPITokenByHash :one
SELECT *
FROM integration_api_tokens
WHERE token_hash = sqlc.arg(token_hash);

-- name: ListIntegrationAPITokens :many
SELECT *
FROM integration_api_tokens
ORDER BY created_at DESC;

-- name: DisableIntegrationAPIToken :one
UPDATE integration_api_tokens
SET disabled_at = now(),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DisableAllIntegrationAPITokens :exec
UPDATE integration_api_tokens
SET disabled_at = COALESCE(disabled_at, now()),
    updated_at = now()
WHERE disabled_at IS NULL;

-- name: TouchIntegrationAPITokenUsed :exec
UPDATE integration_api_tokens
SET last_used_at = now(),
    updated_at = now()
WHERE id = sqlc.arg(id);

-- name: DeleteIntegrationAPIToken :exec
DELETE FROM integration_api_tokens
WHERE id = sqlc.arg(id);
