-- name: InsertPaymentAudit :one
INSERT INTO payment_audits (id, payment_id, event_type, old_status, new_status, actor, metadata)
VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListPaymentAuditsByPaymentID :many
SELECT * FROM payment_audits
WHERE payment_id = $1
ORDER BY created_at ASC;
