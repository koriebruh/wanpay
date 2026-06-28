package entity

import "time"

type AdminRole string

const (
	AdminRoleSuperAdmin AdminRole = "super_admin"
	AdminRoleOps        AdminRole = "ops"
	AdminRoleFinance    AdminRole = "finance"
)

// Admin is an internal operator account for the Wanpey platform.
// Admins manage merchants, set fees, and monitor operations.
// Passwords are stored as bcrypt hashes. JWTs are issued on login.
type Admin struct {
	ID           string
	Email        string
	PasswordHash string
	Role         AdminRole
	IsActive     bool
	LastLoginAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (a *Admin) IsSuperAdmin() bool {
	return a.Role == AdminRoleSuperAdmin
}
