-- name: InsertAdmin :one
INSERT INTO admins (id, email, password_hash, role, is_active)
VALUES (gen_random_uuid()::TEXT, $1, $2, $3, true)
RETURNING *;

-- name: GetAdminByID :one
SELECT * FROM admins
WHERE id = $1;

-- name: GetAdminByEmail :one
SELECT * FROM admins
WHERE email = $1;

-- name: UpdateAdminLastLogin :exec
UPDATE admins SET last_login_at = NOW(), updated_at = NOW()
WHERE id = $1;

-- name: UpdateAdmin :one
UPDATE admins SET role = $2, is_active = $3, updated_at = NOW()
WHERE id = $1 RETURNING *;

-- name: ListAdmins :many
SELECT * FROM admins ORDER BY created_at DESC LIMIT $1 OFFSET $2;

-- name: CountAdmins :one
SELECT COUNT(*) FROM admins;
