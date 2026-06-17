package database

import (
	"context"
	"database/sql"
	"fmt"
)

type txKey struct{}

// Querier is the minimal SQL contract shared by *sql.DB and *sql.Tx.
// All repository implementations depend on Querier — never on a concrete DB type.
// Swapping the RDBMS means changing only the provider (ProvideDB), not repositories.
// The interface mirrors postgres.DBTX so Querier can be passed directly to postgres.New().
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// SQLDB extends Querier with lifecycle operations.
// The DI container resolves SQLDB; repositories resolve Querier via QuerierFromContext.
type SQLDB interface {
	Querier
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
	PingContext(ctx context.Context) error
}

// WithTx stores tx in ctx so every repository method in the same call chain
// automatically uses the same transaction without changing their signatures.
func WithTx(ctx context.Context, tx *sql.Tx) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

// TxFromContext retrieves the active *sql.Tx from context, or nil if none was set.
// Postgres repositories use this to create a transactional sqlc Queries instance.
func TxFromContext(ctx context.Context) *sql.Tx {
	tx, _ := ctx.Value(txKey{}).(*sql.Tx)
	return tx
}

// QuerierFromContext returns the active *sql.Tx if one was stored via WithTx,
// otherwise returns the default db. Repositories call this at the start of every method.
func QuerierFromContext(ctx context.Context, db Querier) Querier {
	if tx, ok := ctx.Value(txKey{}).(*sql.Tx); ok {
		return tx
	}
	return db
}

// RunInTx executes fn inside a single transaction. Commits on success, rolls back on any error.
// Panics are caught, the transaction is rolled back, then the panic is re-raised.
// Usage: inject the active tx into ctx so repositories pick it up automatically.
//
//	err = database.RunInTx(ctx, db, nil, func(ctx context.Context) error {
//	    if err := paymentRepo.Update(ctx, p); err != nil { return err }
//	    return mutationRepo.Save(ctx, m)
//	})
func RunInTx(ctx context.Context, db SQLDB, opts *sql.TxOptions, fn func(ctx context.Context) error) error {
	tx, err := db.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()

	txCtx := WithTx(ctx, tx)
	if err := fn(txCtx); err != nil {
		_ = tx.Rollback()
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
