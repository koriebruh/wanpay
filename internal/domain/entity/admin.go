package entity

import "time"

type AdminRole string

const (
	AdminRoleAdmin      AdminRole = "admin"
	AdminRoleSuperAdmin AdminRole = "super_admin"
)

// Admin is an internal operator account for the Wanpey platform.
// Admins manage merchants, set fees, and monitor operations.
// Passwords are stored as bcrypt hashes. JWTs are issued on login.
type Admin struct {
	ID           string
	Username     string
	PasswordHash string
	Role         AdminRole
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (a *Admin) IsSuperAdmin() bool {
	return a.Role == AdminRoleSuperAdmin
}
