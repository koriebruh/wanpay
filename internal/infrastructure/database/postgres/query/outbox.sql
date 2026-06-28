-- name: InsertOutboxEvent :one
INSERT INTO outbox (id, event_type, payload, target_url, merchant_id, next_retry_at)
VALUES (gen_random_uuid(), $1, $2, $3, $4, NOW())
RETURNING *;

-- name: LeaseOutboxEvents :many
SELECT * FROM outbox
WHERE delivered_at  IS NULL
  AND failed_at     IS NULL
  AND next_retry_at <= NOW()
  AND attempt_count < max_attempts
ORDER BY next_retry_at ASC
LIMIT $1
FOR UPDATE SKIP LOCKED;

-- name: MarkOutboxDelivered :exec
UPDATE outbox
SET delivered_at  = NOW(),
    attempt_count = attempt_count + 1
WHERE id = $1;

-- name: MarkOutboxFailed :exec
UPDATE outbox
SET attempt_count = attempt_count + 1,
    last_error    = $2,
    next_retry_at = $3,
    failed_at     = CASE
                        WHEN attempt_count + 1 >= max_attempts THEN NOW()
                        ELSE NULL
                    END
WHERE id = $1;

-- name: MarkOutboxFailedFinal :exec
UPDATE outbox
SET attempt_count = max_attempts,
    last_error    = $2,
    failed_at     = NOW()
WHERE id = $1;

-- name: ScheduleOutboxRetry :exec
UPDATE outbox
SET attempt_count = $2,
    last_error    = $3,
    next_retry_at = NOW() + ($4 || ' seconds')::interval
WHERE id = $1;

-- name: ListOutboxByMerchant :many
SELECT * FROM outbox
WHERE merchant_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountOutboxByMerchant :one
SELECT COUNT(*) FROM outbox
WHERE merchant_id = $1;
