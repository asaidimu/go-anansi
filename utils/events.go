package utils

import (
	"io"
	"log/slog"
	"sync"
	"time"

	goevents "github.com/asaidimu/go-events/v2"
)

// memoryStore is a thread-safe in-memory implementation of goevents.Store,
// used to keep a bus fully ephemeral (no Pebble state directory).
type memoryStore struct {
	mu    sync.RWMutex
	store map[string][]byte
}

func newMemoryStore() *memoryStore {
	return &memoryStore{store: make(map[string][]byte)}
}

func (s *memoryStore) Set(key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store[key] = value
	return nil
}

func (s *memoryStore) Get(key string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.store[key]
	if !ok {
		return nil, goevents.ErrStoreKeyNotFound
	}
	return v, nil
}

func (s *memoryStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.store, key)
	return nil
}

func (s *memoryStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store = nil
	return nil
}

// NewInMemoryGoEventsBus creates a fully in-memory go-events v2 EventBus.
// Nothing is persisted: the event log and state store both live in memory and
// are lost when the returned bus is closed. busKey namespaces the (empty)
// base directory used to satisfy config validation.
func NewInMemoryGoEventsBus(busKey string) (*goevents.EventBus, error) {
	cfg := goevents.DefaultConfig("memory", busKey)
	cfg.Log = goevents.NewMemoryLog()
	cfg.Store = newMemoryStore()
	cfg.PollInterval = 25 * time.Millisecond
	cfg.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	return goevents.NewEventBus(cfg)
}