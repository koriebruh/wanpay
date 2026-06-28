-- Dev seed: super_admin account for local development.
-- Password: Admin123!  (bcrypt $2a$10$...)
-- DO NOT use in production.

INSERT INTO admins (id, email, password_hash, role, is_active)
VALUES (
    'a0000000-0000-0000-0000-000000000001',
    'admin@wanpey.dev',
    '$2a$10$92IXUNpkjO0rOQ5byMi.Ye4oKoEa3Ro9llC/.og/at2.uheWG/igi', -- password: password
    'super_admin',
    true
)
ON CONFLICT (email) DO NOTHING;

INSERT INTO admins (id, email, password_hash, role, is_active)
VALUES (
    'a0000000-0000-0000-0000-000000000002',
    'ops@wanpey.dev',
    '$2a$10$92IXUNpkjO0rOQ5byMi.Ye4oKoEa3Ro9llC/.og/at2.uheWG/igi', -- password: password
    'ops',
    true
)
ON CONFLICT (email) DO NOTHING;

INSERT INTO admins (id, email, password_hash, role, is_active)
VALUES (
    'a0000000-0000-0000-0000-000000000003',
    'finance@wanpey.dev',
    '$2a$10$92IXUNpkjO0rOQ5byMi.Ye4oKoEa3Ro9llC/.og/at2.uheWG/igi', -- password: password
    'finance',
    true
)
ON CONFLICT (email) DO NOTHING;
