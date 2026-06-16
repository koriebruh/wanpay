package main

import (
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/spf13/cobra"

	"wanpey/core/internal/infrastructure/config"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Database migration management",
}

func newMigrator() (*migrate.Migrate, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	// "file://migrations" resolves relative to CWD — always run via `make migrate-up` from project root.
	m, err := migrate.New("file://migrations", cfg.Database.DSN)
	if err != nil {
		return nil, fmt.Errorf("init migrator: %w", err)
	}
	return m, nil
}

var migrateUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Apply all pending migrations",
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := newMigrator()
		if err != nil {
			return err
		}
		defer m.Close()

		if err := m.Up(); err != nil {
			if errors.Is(err, migrate.ErrNoChange) {
				fmt.Println("no pending migrations")
				return nil
			}
			return fmt.Errorf("migrate up: %w", err)
		}
		fmt.Println("migrations applied successfully")
		return nil
	},
}

var migrateDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Rollback the last migration",
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := newMigrator()
		if err != nil {
			return err
		}
		defer m.Close()

		if err := m.Steps(-1); err != nil {
			return fmt.Errorf("migrate down: %w", err)
		}
		fmt.Println("rolled back 1 migration")
		return nil
	},
}

var migrateStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current migration version",
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := newMigrator()
		if err != nil {
			return err
		}
		defer m.Close()

		version, dirty, err := m.Version()
		if err != nil {
			if errors.Is(err, migrate.ErrNilVersion) {
				fmt.Println("no migrations applied yet")
				return nil
			}
			return fmt.Errorf("get version: %w", err)
		}

		fmt.Printf("current version: %d", version)
		if dirty {
			fmt.Print(" (dirty — last migration failed, manual intervention required)")
		}
		fmt.Println()
		return nil
	},
}

func init() {
	migrateCmd.AddCommand(migrateUpCmd)
	migrateCmd.AddCommand(migrateDownCmd)
	migrateCmd.AddCommand(migrateStatusCmd)
}
