package app

import (
	"database/sql"
	"fmt"

	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	sqliteExecutor "github.com/asaidimu/go-anansi/v6/sqlite/executor"
	sqliteQuery "github.com/asaidimu/go-anansi/v6/sqlite/query"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
	"go.uber.org/zap"
)

// Database holds the database connection and Anansi interactor.
type Database struct {
	DB         *sql.DB
	Interactor query.DatabaseInteractor
}

// NewDatabase initializes a new SQLite database connection and Anansi interactor.
func NewDatabase(cfg *Config, logger *zap.Logger) (*Database, error) {
	db, err := sql.Open("sqlite3", cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	executor, err := sqliteExecutor.NewSQLiteExecutor(db, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create SQLite interactor: %w", err)
	}
	queryFactory := sqliteQuery.NewSQLiteFactory()
	interactor, err := native.NewNativeInteractor(executor, queryFactory, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create native interactor: %w", err)
	}

	return &Database{
		DB:         db,
		Interactor: interactor,
	}, nil
}

// Close closes the database connection.
func (d *Database) Close() error {
	return d.DB.Close()
}
