package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

func WithinTransaction(ctx context.Context, database *sql.DB, operation func(*sql.Tx) error) (err error) {
	if database == nil || operation == nil {
		return fmt.Errorf("database and transaction operation are required")
	}
	transaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			_ = transaction.Rollback()
			panic(recovered)
		}
	}()

	if err := operation(transaction); err != nil {
		if rollbackErr := transaction.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			return errors.Join(err, fmt.Errorf("rollback transaction: %w", rollbackErr))
		}
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}
