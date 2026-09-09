package sqlite_test

import (
	"strings"
	"testing"

	"github.com/asaidimu/go-anansi/v8/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestConfigFileDSN_Defaults(t *testing.T) {
	c := sqlite.Config{}.FileDSN()

	assert.Contains(t, c, "file:anansi.db?")
	assert.Contains(t, c, "cache=shared")
	assert.Contains(t, c, "_fk=1")
	assert.Contains(t, c, "_journal_mode=WAL")
	assert.Contains(t, c, "_busy_timeout=5000")
}

func TestConfigFileDSN_CustomPath(t *testing.T) {
	c := sqlite.Config{Path: "test.db"}.FileDSN()

	assert.True(t, strings.HasPrefix(c, "file:test.db?"), "DSN should use custom path")
}

func TestConfigFileDSN_Overrides(t *testing.T) {
	c := sqlite.Config{
		Path:               "custom.db",
		JournalMode:        "DELETE",
		BusyTimeoutMs:      1000,
		DisableForeignKeys: true,
		Cache:              "private",
		MaxOpenConns:       2,
		MaxIdleConns:       1,
	}.FileDSN()

	assert.Contains(t, c, "file:custom.db?")
	assert.Contains(t, c, "_journal_mode=DELETE")
	assert.Contains(t, c, "_busy_timeout=1000")
	assert.Contains(t, c, "_fk=0")
	assert.Contains(t, c, "cache=private")
}

func TestConfigFileDSN_NegativeBusyTimeout_DisablesIt(t *testing.T) {
	c := sqlite.Config{BusyTimeoutMs: -1}.FileDSN()

	assert.Contains(t, c, "_busy_timeout=0")
}

func TestConfigMemoryDSN_Defaults(t *testing.T) {
	c := sqlite.Config{}.MemoryDSN()

	assert.Contains(t, c, "file:anansi?mode=memory")
	assert.Contains(t, c, "cache=shared")
	assert.Contains(t, c, "_fk=1")
	assert.Contains(t, c, "_journal_mode=WAL")
	assert.Contains(t, c, "_busy_timeout=5000")
}

func TestConfigMemoryDSN_CustomName(t *testing.T) {
	c := sqlite.Config{Path: "mydb"}.MemoryDSN()

	assert.Contains(t, c, "file:mydb?mode=memory")
}

func TestNewMemoryInteractor_Defeaults(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping sqlite open in short mode")
	}

	h, err := sqlite.NewMemoryInteractor(sqlite.Config{})
	require.NoError(t, err)
	defer h.Cleanup()

	assert.NotNil(t, h.DB)
	assert.NotNil(t, h.Interactor)
	assert.NoError(t, h.DB.Ping())
}

func TestNewMemoryInteractor_Overrides(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping sqlite open in short mode")
	}

	l := zap.NewNop()
	h, err := sqlite.NewMemoryInteractor(sqlite.Config{
		Path:               "custom_test",
		BusyTimeoutMs:      2000,
		DisableForeignKeys: true,
		MaxOpenConns:       8,
		MaxIdleConns:       2,
		Logger:             l,
	})
	require.NoError(t, err)
	defer h.Cleanup()

	assert.NotNil(t, h.DB)
	assert.NotNil(t, h.Interactor)

	stats := h.DB.Stats()
	assert.Equal(t, 8, stats.MaxOpenConnections)
}

func TestNewInteractor_FileDB(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping sqlite open in short mode")
	}

	tmp := t.TempDir() + "/test.db"

	h, err := sqlite.NewInteractor(sqlite.Config{
		Path: tmp,
	})
	require.NoError(t, err)
	defer h.Cleanup()

	assert.NotNil(t, h.DB)
	assert.NotNil(t, h.Interactor)
	assert.NoError(t, h.DB.Ping())
}

func TestHandleClose_NilSafe(t *testing.T) {
	var h *sqlite.Handle
	assert.NoError(t, h.Close())
}

func TestConfigWithDefaults_NegativeConnCounts(t *testing.T) {
	c := sqlite.Config{
		MaxOpenConns: -1,
		MaxIdleConns: -1,
	}
	dsn := c.FileDSN()
	// Negative should resolve to 0 (unlimited) — verify DSN still builds
	assert.Contains(t, dsn, "file:anansi.db?")
}
