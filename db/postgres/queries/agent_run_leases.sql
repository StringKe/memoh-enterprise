-- name: CreateAgentRunLease :one
INSERT INTO agent_run_leases (
  run_id,
  runner_instance_id,
  bot_id,
  bot_group_id,
  session_id,
  user_id,
  permission_snapshot_version,
  allowed_tool_scopes,
  workspace_executor_target,
  workspace_id,
  expires_at,
  lease_version
) VALUES (
  sqlc.arg(run_id),
  sqlc.arg(runner_instance_id),
  sqlc.arg(bot_id),
  sqlc.narg(bot_group_id),
  sqlc.arg(session_id),
  sqlc.arg(user_id),
  sqlc.arg(permission_snapshot_version),
  sqlc.arg(allowed_tool_scopes)::text[],
  sqlc.arg(workspace_executor_target),
  sqlc.arg(workspace_id),
  sqlc.arg(expires_at),
  sqlc.arg(lease_version)
)
RETURNING *;

-- name: GetAgentRunLease :one
SELECT *
FROM agent_run_leases
WHERE run_id = sqlc.arg(run_id);

-- name: GetActiveAgentRunLease :one
SELECT *
FROM agent_run_leases
WHERE run_id = sqlc.arg(run_id)
  AND revoked_at IS NULL
  AND completed_at IS NULL
  AND expires_at > now();

-- name: ValidateActiveAgentRunLease :one
SELECT *
FROM agent_run_leases
WHERE run_id = sqlc.arg(run_id)
  AND runner_instance_id = sqlc.arg(runner_instance_id)
  AND lease_version = sqlc.arg(lease_version)
  AND bot_id = sqlc.arg(bot_id)
  AND session_id = sqlc.arg(session_id)
  AND workspace_id = sqlc.arg(workspace_id)
  AND workspace_executor_target = sqlc.arg(workspace_executor_target)
  AND revoked_at IS NULL
  AND completed_at IS NULL
  AND expires_at > now();

-- name: RevokeAgentRunLease :one
UPDATE agent_run_leases
SET revoked_at = now(),
    updated_at = now()
WHERE run_id = sqlc.arg(run_id)
  AND revoked_at IS NULL
RETURNING *;

-- name: CompleteAgentRunLease :one
UPDATE agent_run_leases
SET completed_at = now(),
    updated_at = now()
WHERE run_id = sqlc.arg(run_id)
  AND completed_at IS NULL
RETURNING *;

-- name: ExpireAgentRunLeases :execrows
UPDATE agent_run_leases
SET revoked_at = COALESCE(revoked_at, now()),
    updated_at = now()
WHERE expires_at <= now()
  AND revoked_at IS NULL
  AND completed_at IS NULL;
