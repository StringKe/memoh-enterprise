-- name: CreateStructuredDataSpace :one
INSERT INTO structured_data_spaces (
  owner_type,
  owner_bot_id,
  owner_bot_group_id,
  schema_name,
  role_name,
  display_name,
  metadata
) VALUES (
  sqlc.arg(owner_type),
  sqlc.narg(owner_bot_id),
  sqlc.narg(owner_bot_group_id),
  sqlc.arg(schema_name),
  sqlc.arg(role_name),
  sqlc.arg(display_name),
  sqlc.arg(metadata)
)
RETURNING *;

-- name: GetStructuredDataSpaceByID :one
SELECT *
FROM structured_data_spaces
WHERE id = sqlc.arg(id);

-- name: GetStructuredDataSpaceByBot :one
SELECT *
FROM structured_data_spaces
WHERE owner_type = 'bot'
  AND owner_bot_id = sqlc.arg(owner_bot_id);

-- name: GetStructuredDataSpaceByBotGroup :one
SELECT *
FROM structured_data_spaces
WHERE owner_type = 'bot_group'
  AND owner_bot_group_id = sqlc.arg(owner_bot_group_id);

-- name: ListStructuredDataSpaces :many
SELECT *
FROM structured_data_spaces
ORDER BY owner_type ASC, created_at DESC;

-- name: ListStructuredDataSpacesForBotActor :many
SELECT DISTINCT s.*
FROM structured_data_spaces s
LEFT JOIN structured_data_grants g ON g.space_id = s.id
WHERE
  (s.owner_type = 'bot' AND s.owner_bot_id = sqlc.arg(bot_id))
  OR (
    sqlc.narg(bot_group_id)::uuid IS NOT NULL
    AND s.owner_type = 'bot_group'
    AND s.owner_bot_group_id = sqlc.narg(bot_group_id)
  )
  OR (g.target_type = 'bot' AND g.target_bot_id = sqlc.arg(bot_id))
  OR (
    sqlc.narg(bot_group_id)::uuid IS NOT NULL
    AND g.target_type = 'bot_group'
    AND g.target_bot_group_id = sqlc.narg(bot_group_id)
  )
ORDER BY s.owner_type ASC, s.created_at DESC;

-- name: UpsertStructuredDataGrantForBot :one
INSERT INTO structured_data_grants (
  space_id,
  target_type,
  target_bot_id,
  privileges,
  created_by_user_id
) VALUES (
  sqlc.arg(space_id),
  'bot',
  sqlc.arg(target_bot_id),
  sqlc.arg(privileges),
  sqlc.narg(created_by_user_id)
)
ON CONFLICT (space_id, target_bot_id) WHERE target_type = 'bot'
DO UPDATE SET
  privileges = EXCLUDED.privileges,
  updated_at = now()
RETURNING *;

-- name: UpsertStructuredDataGrantForBotGroup :one
INSERT INTO structured_data_grants (
  space_id,
  target_type,
  target_bot_group_id,
  privileges,
  created_by_user_id
) VALUES (
  sqlc.arg(space_id),
  'bot_group',
  sqlc.arg(target_bot_group_id),
  sqlc.arg(privileges),
  sqlc.narg(created_by_user_id)
)
ON CONFLICT (space_id, target_bot_group_id) WHERE target_type = 'bot_group'
DO UPDATE SET
  privileges = EXCLUDED.privileges,
  updated_at = now()
RETURNING *;

-- name: ListStructuredDataGrantsBySpace :many
SELECT *
FROM structured_data_grants
WHERE space_id = sqlc.arg(space_id)
ORDER BY target_type ASC, created_at DESC;

-- name: GetStructuredDataGrantByID :one
SELECT *
FROM structured_data_grants
WHERE id = sqlc.arg(id);

-- name: DeleteStructuredDataGrant :exec
DELETE FROM structured_data_grants
WHERE id = sqlc.arg(id);

-- name: CreateStructuredDataAudit :one
INSERT INTO structured_data_audit (
  space_id,
  actor_type,
  actor_user_id,
  actor_bot_id,
  operation,
  statement,
  success,
  error,
  row_count,
  duration_ms
) VALUES (
  sqlc.narg(space_id),
  sqlc.arg(actor_type),
  sqlc.narg(actor_user_id),
  sqlc.narg(actor_bot_id),
  sqlc.arg(operation),
  sqlc.arg(statement),
  sqlc.arg(success),
  sqlc.arg(error),
  sqlc.arg(row_count),
  sqlc.arg(duration_ms)
)
RETURNING *;
