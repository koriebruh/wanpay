-- name: InsertAdmin :one
INSERT INTO admins (id, username, password_hash, role)
VALUES (gen_random_uuid()::TEXT, $1, $2, $3)
RETURNING *;

-- name: GetAdminByID :one
SELECT * FROM admins
WHERE id = $1;

-- name: GetAdminByUsername :one
SELECT * FROM admins
WHERE username = $1;
