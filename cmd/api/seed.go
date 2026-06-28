package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/bcrypt"

	"wanpey/core/internal/domain/entity"
	"wanpey/core/internal/infrastructure/config"
	"wanpey/core/internal/infrastructure/database/postgres"
)

var (
	seedEmail    string
	seedPassword string
	seedRole     string
)

// seedAdminCmd bootstraps the first admin account. After public merchant
// registration was removed, this is the only way to get into the system.
var seedAdminCmd = &cobra.Command{
	Use:   "seed-admin",
	Short: "Create an admin account (bootstrap)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(seedPassword) < 8 {
			return fmt.Errorf("password must be at least 8 characters")
		}
		validRoles := map[string]bool{
			string(entity.AdminRoleSuperAdmin): true,
			string(entity.AdminRoleOps):        true,
			string(entity.AdminRoleFinance):    true,
		}
		if !validRoles[seedRole] {
			return fmt.Errorf("role must be one of: super_admin, ops, finance")
		}

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		db, err := postgres.Connect(cfg.Database)
		if err != nil {
			return fmt.Errorf("open db: %w", err)
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(seedPassword), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("hash password: %w", err)
		}

		repo := postgres.NewAdminRepo(db)
		a := &entity.Admin{
			Email:        seedEmail,
			PasswordHash: string(hash),
			Role:         entity.AdminRole(seedRole),
		}
		if err := repo.Save(context.Background(), a); err != nil {
			return fmt.Errorf("create admin (email may already exist): %w", err)
		}
		fmt.Printf("admin %q created (role=%s)\n", a.Email, a.Role)
		return nil
	},
}

func init() {
	seedAdminCmd.Flags().StringVar(&seedEmail, "email", "", "admin email address (required)")
	seedAdminCmd.Flags().StringVar(&seedPassword, "password", "", "admin password, min 8 chars (required)")
	seedAdminCmd.Flags().StringVar(&seedRole, "role", "super_admin", "admin role: super_admin | ops | finance")
	_ = seedAdminCmd.MarkFlagRequired("email")
	_ = seedAdminCmd.MarkFlagRequired("password")
}
