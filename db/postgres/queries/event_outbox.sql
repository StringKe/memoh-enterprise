-- name: EnqueueEvent :one
INSERT INTO event_outbox (
  topic,
  payload_type,
  payload,
  payload_json,
  idempotency_key,
  aggregate_type,
  aggregate_id,
  partition_key
) VALUES (
  sqlc.arg(topic),
  sqlc.arg(payload_type),
  sqlc.arg(payload),
  sqlc.narg(payload_json),
  sqlc.arg(idempotency_key),
  sqlc.narg(aggregate_type),
  sqlc.narg(aggregate_id),
  sqlc.narg(partition_key)
)
ON CONFLICT (idempotency_key) DO UPDATE
SET idempotency_key = EXCLUDED.idempotency_key
RETURNING *;

-- name: CreateEventDeliveries :many
INSERT INTO event_deliveries (
  event_id,
  consumer_name,
  topic,
  partition_key,
  available_at,
  max_attempts
)
SELECT
  sqlc.arg(event_id),
  unnest(sqlc.arg(consumer_names)::text[]),
  sqlc.arg(topic),
  sqlc.narg(partition_key),
  sqlc.arg(available_at),
  sqlc.arg(max_attempts)
ON CONFLICT (event_id, consumer_name) DO NOTHING
RETURNING *;

-- name: ClaimEventDeliveries :many
WITH claimed AS (
  SELECT event_deliveries.event_id, event_deliveries.consumer_name
  FROM event_deliveries
  WHERE event_deliveries.consumer_name = sqlc.arg(consumer_name)
    AND event_deliveries.topic = sqlc.arg(topic)
    AND event_deliveries.delivered_at IS NULL
    AND event_deliveries.dead_lettered_at IS NULL
    AND event_deliveries.available_at <= now()
    AND (event_deliveries.locked_until IS NULL OR event_deliveries.locked_until <= now())
  ORDER BY event_deliveries.partition_key NULLS LAST, event_deliveries.available_at ASC, event_deliveries.created_at ASC
  LIMIT sqlc.arg(limit_count)
  FOR UPDATE SKIP LOCKED
),
updated AS (
  UPDATE event_deliveries d
  SET locked_by = sqlc.arg(locked_by),
      locked_until = sqlc.arg(locked_until),
      attempts = d.attempts + 1,
      updated_at = now()
  FROM claimed
  WHERE d.event_id = claimed.event_id
    AND d.consumer_name = claimed.consumer_name
  RETURNING d.*
)
SELECT
  updated.event_id,
  updated.consumer_name,
  updated.topic,
  updated.partition_key,
  updated.available_at,
  updated.locked_by,
  updated.locked_until,
  updated.attempts,
  updated.max_attempts,
  updated.last_error,
  updated.delivered_at,
  updated.dead_lettered_at,
  updated.created_at,
  updated.updated_at,
  event_outbox.payload_type,
  event_outbox.payload,
  event_outbox.payload_json,
  event_outbox.idempotency_key,
  event_outbox.aggregate_type,
  event_outbox.aggregate_id,
  event_outbox.created_at AS event_created_at
FROM updated
JOIN event_outbox ON event_outbox.id = updated.event_id
ORDER BY updated.partition_key NULLS LAST, updated.available_at ASC, updated.created_at ASC;

-- name: AckEventDelivery :one
UPDATE event_deliveries
SET delivered_at = now(),
    locked_by = NULL,
    locked_until = NULL,
    updated_at = now()
WHERE event_id = sqlc.arg(event_id)
  AND consumer_name = sqlc.arg(consumer_name)
  AND locked_by = sqlc.arg(locked_by)
  AND delivered_at IS NULL
  AND dead_lettered_at IS NULL
RETURNING *;

-- name: FailEventDelivery :one
UPDATE event_deliveries
SET last_error = sqlc.arg(last_error),
    locked_by = NULL,
    locked_until = NULL,
    available_at = CASE
      WHEN attempts >= max_attempts THEN available_at
      ELSE sqlc.arg(next_available_at)
    END,
    dead_lettered_at = CASE
      WHEN attempts >= max_attempts THEN now()
      ELSE dead_lettered_at
    END,
    updated_at = now()
WHERE event_id = sqlc.arg(event_id)
  AND consumer_name = sqlc.arg(consumer_name)
  AND locked_by = sqlc.arg(locked_by)
  AND delivered_at IS NULL
  AND dead_lettered_at IS NULL
RETURNING *;

-- name: DeadLetterEventDelivery :one
UPDATE event_deliveries
SET last_error = sqlc.arg(last_error),
    locked_by = NULL,
    locked_until = NULL,
    dead_lettered_at = now(),
    updated_at = now()
WHERE event_id = sqlc.arg(event_id)
  AND consumer_name = sqlc.arg(consumer_name)
  AND delivered_at IS NULL
RETURNING *;

-- name: GetEventDelivery :one
SELECT *
FROM event_deliveries
WHERE event_id = sqlc.arg(event_id)
  AND consumer_name = sqlc.arg(consumer_name);
