package webhooks

import (
    "context"
    "sync"
    "time"
    
    "github.com/asaidimu/go-anansi/v6/core/persistence/base"
    "go.uber.org/zap"
	"github.com/google/uuid"
)

// EventCallback represents a callback for handling events via HTTP (e.g., webhooks)
type EventCallback struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

// Manager handles webhook lifecycle and cleanup
type Manager struct {
    sender      *Sender
    callbacks   map[string]*CallbackInfo
    mutex       sync.RWMutex
    logger      *zap.Logger
    cleanupTick time.Duration
}

type CallbackInfo struct {
    Callback     EventCallback
    CreatedAt    time.Time
    LastUsed     time.Time
    FailureCount int
    MaxFailures  int
}

func NewManager(logger *zap.Logger) *Manager {
	m := &Manager{
		sender:      NewSender(10*time.Second, 3),
		callbacks:   make(map[string]*CallbackInfo),
		logger:      logger,
		cleanupTick: 1 * time.Minute,
	}
	return m
}

// StartCleanup starts the cleanup process for old/failed webhooks
func (m *Manager) StartCleanup(ctx context.Context) {
	ticker := time.NewTicker(m.cleanupTick)
	go func() {
		for {
			select {
			case <-ticker.C:
				m.cleanup()
			case <-ctx.Done():
				ticker.Stop()
				return
			}
		}
	}()
}

func (m *Manager) cleanup() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for id, info := range m.callbacks {
		if info.FailureCount >= info.MaxFailures || time.Since(info.LastUsed) > 30*24*time.Hour {
			delete(m.callbacks, id)
			m.logger.Info("Cleaned up webhook", zap.String("id", id))
		}
	}
}

// Register adds a new webhook with automatic cleanup
func (m *Manager) Register(eventType base.PersistenceEventType, url string, headers map[string]string) string {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	id := uuid.New().String()
	m.callbacks[id] = &CallbackInfo{
		Callback: EventCallback{
			URL:     url,
			Headers: headers,
		},
		CreatedAt:    time.Now(),
		LastUsed:     time.Now(),
		MaxFailures:  5,
	}
	return id
}

// Unregister removes a webhook
func (m *Manager) Unregister(id string) bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, ok := m.callbacks[id]; ok {
		delete(m.callbacks, id)
		return true
	}
	return false
}

// Close gracefully shuts down the manager
func (m *Manager) Close() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.callbacks = make(map[string]*CallbackInfo)
	return nil
}
