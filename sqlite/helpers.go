package sqlite

import (
	"database/sql"
	"fmt"
	"strconv"

	"github.com/asaidimu/go-anansi/v8/core/query"
	"github.com/asaidimu/go-anansi/v8/core/query/native"
	sqliteExecutor "github.com/asaidimu/go-anansi/v8/sqlite/executor"
	sqliteQuery "github.com/asaidimu/go-anansi/v8/sqlite/query"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)

// Defaults applied when the corresponding Config field is left at its
// zero value. They match the historically recommended Playground settings:
// single-writer SQLite with WAL (1 writer + up to 3 readers), foreign keys
// enforced, and a busy timeout so transient lock contention is retried
// inside SQLite instead of surfacing SQLITE_BUSY immediately.
const (
	DefaultJournalMode  = "WAL"
	DefaultBusyTimeout  = 5000
	DefaultCache        = "shared"
	DefaultMaxOpenConns = 4
	DefaultMaxIdleConns = 4

	DefaultFilePath   = "anansi.db"
	DefaultMemoryName = "anansi"
)

// Config controls how a SQLite interactor is constructed. The zero value is
// usable: every field falls back to its Default* constant (logger to Nop).
type Config struct {
	// Path is the file path for NewInteractor, or the shared-memory name
	// for NewMemoryInteractor. Empty falls back to DefaultFilePath /
	// DefaultMemoryName respectively.
	Path string

	// JournalMode defaults to "WAL". Set explicitly (e.g. "DELETE") to overwrite.
	JournalMode string

	// BusyTimeoutMs defaults to DefaultBusyTimeout. Zero means default;
	// use a negative value to disable (0ms timeout).
	BusyTimeoutMs int

	// DisableForeignKeys defaults to false (i.e. FK enforcement ON via
	// _fk=1). Set to true to overwrite with _fk=0.
	DisableForeignKeys bool

	// Cache defaults to "shared". Set explicitly (e.g. "private") to overwrite.
	Cache string

	// MaxOpenConns / MaxIdleConns default to 4/4 (1 writer + 3 readers
	// under WAL). Zero means default; negative means 0 (unlimited for
	// MaxOpenConns in database/sql semantics).
	MaxOpenConns int
	MaxIdleConns int

	// Logger defaults to zap.NewNop() when nil.
	Logger *zap.Logger
}

// Handle bundles the configured *sql.DB with its wired DatabaseInteractor.
// Cleanup closes the DB (ignoring the error); Close returns it.
type Handle struct {
	DB         *sql.DB
	Interactor query.DatabaseInteractor
	Cleanup    func()
}

// Close closes the underlying *sql.DB.
func (h *Handle) Close() error {
	if h == nil || h.DB == nil {
		return nil
	}
	return h.DB.Close()
}

func (c Config) withDefaults() Config {
	if c.JournalMode == "" {
		c.JournalMode = DefaultJournalMode
	}
	if c.Cache == "" {
		c.Cache = DefaultCache
	}
	if c.BusyTimeoutMs == 0 {
		c.BusyTimeoutMs = DefaultBusyTimeout
	} else if c.BusyTimeoutMs < 0 {
		c.BusyTimeoutMs = 0
	}
	if c.MaxOpenConns == 0 {
		c.MaxOpenConns = DefaultMaxOpenConns
	} else if c.MaxOpenConns < 0 {
		c.MaxOpenConns = 0
	}
	if c.MaxIdleConns == 0 {
		c.MaxIdleConns = DefaultMaxIdleConns
	} else if c.MaxIdleConns < 0 {
		c.MaxIdleConns = 0
	}
	if c.Logger == nil {
		c.Logger = zap.NewNop()
	}
	return c
}

func fkFlag(disable bool) string {
	if disable {
		return "0"
	}
	return "1"
}

// FileDSN builds the file-backed DSN for cfg.Path, applying defaults.
func (c Config) FileDSN() string {
	c = c.withDefaults()
	path := c.Path
	if path == "" {
		path = DefaultFilePath
	}
	return fmt.Sprintf("file:%s?cache=%s&_fk=%s&_journal_mode=%s&_busy_timeout=%s",
		path, c.Cache, fkFlag(c.DisableForeignKeys), c.JournalMode, strconv.Itoa(c.BusyTimeoutMs))
}

// MemoryDSN builds a named shared-memory DSN for cfg.Path, applying defaults.
func (c Config) MemoryDSN() string {
	c = c.withDefaults()
	name := c.Path
	if name == "" {
		name = DefaultMemoryName
	}
	return fmt.Sprintf("file:%s?mode=memory&cache=%s&_fk=%s&_journal_mode=%s&_busy_timeout=%s",
		name, c.Cache, fkFlag(c.DisableForeignKeys), c.JournalMode, strconv.Itoa(c.BusyTimeoutMs))
}

func open(cfg Config, dsn string) (*Handle, error) {
	cfg = cfg.withDefaults()

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}

	// SQLite is single-writer. With WAL mode, one connection can write
	// while up to 3 others read concurrently. Limiting the pool prevents
	// unbounded connection creation that amplifies lock contention.
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite: ping failed: %w", err)
	}

	executor, err := sqliteExecutor.NewSQLiteExecutor(db, cfg.Logger)
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	queryFactory := sqliteQuery.NewSQLiteFactory(cfg.Logger)
	interactor, err := native.NewNativeInteractor(executor, queryFactory, cfg.Logger)
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	h := &Handle{DB: db, Interactor: interactor}
	h.Cleanup = func() { _ = db.Close() }
	return h, nil
}

// NewInteractor opens a file-backed SQLite database with the recommended
// flags (overridable via Config) and wires executor + query factory +
// native interactor.
func NewInteractor(cfg Config) (*Handle, error) {
	return open(cfg, cfg.FileDSN())
}

// NewMemoryInteractor opens a named shared-memory SQLite database with the
// same recommended flags (overridable via Config) and wires executor +
// query factory + native interactor.
func NewMemoryInteractor(cfg Config) (*Handle, error) {
	return open(cfg, cfg.MemoryDSN())
}
