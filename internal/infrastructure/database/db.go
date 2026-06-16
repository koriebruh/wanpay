package database

import (
	"context"
	"database/sql"
)

// SQLDB is the contract for all SQL operations in the application.
// Swap the underlying engine (Postgres → MySQL → CockroachDB) by changing only the provider,
// not the application code or repository layer.
type SQLDB interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
	PingContext(ctx context.Context) error
}
