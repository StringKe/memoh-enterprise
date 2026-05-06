-- name: AcquireChannelConnectorLease :one
WITH upsert AS (
  INSERT INTO channel_connector_leases (
    channel_config_id,
    channel_type,
    owner_id,
    owner_instance_id,
    lease_version,
    expires_at
  ) VALUES (
    sqlc.arg(channel_config_id),
    sqlc.arg(channel_type),
    sqlc.arg(owner_id),
    sqlc.arg(owner_instance_id),
    1,
    sqlc.arg(expires_at)
  )
  ON CONFLICT (channel_config_id) DO UPDATE
  SET channel_type = EXCLUDED.channel_type,
      owner_id = EXCLUDED.owner_id,
      owner_instance_id = EXCLUDED.owner_instance_id,
      lease_version = channel_connector_leases.lease_version + 1,
      expires_at = EXCLUDED.expires_at,
      updated_at = now()
  WHERE channel_connector_leases.expires_at < now()
  RETURNING *
)
SELECT * FROM upsert;

-- name: RenewChannelConnectorLease :one
UPDATE channel_connector_leases
SET expires_at = sqlc.arg(expires_at),
    updated_at = now()
WHERE channel_config_id = sqlc.arg(channel_config_id)
  AND owner_instance_id = sqlc.arg(owner_instance_id)
  AND lease_version = sqlc.arg(lease_version)
  AND expires_at > now()
RETURNING *;

-- name: ReleaseChannelConnectorLease :execrows
DELETE FROM channel_connector_leases
WHERE channel_config_id = sqlc.arg(channel_config_id)
  AND owner_instance_id = sqlc.arg(owner_instance_id)
  AND lease_version = sqlc.arg(lease_version);

-- name: GetCurrentChannelConnectorLease :one
SELECT *
FROM channel_connector_leases
WHERE channel_config_id = sqlc.arg(channel_config_id);

-- name: CheckChannelConnectorLease :one
SELECT EXISTS (
  SELECT 1
  FROM channel_connector_leases
  WHERE channel_config_id = sqlc.arg(channel_config_id)
    AND owner_instance_id = sqlc.arg(owner_instance_id)
    AND lease_version = sqlc.arg(lease_version)
    AND expires_at > now()
)::boolean;
