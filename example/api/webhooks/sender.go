package webhooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
)

// Sender handles HTTP webhook delivery with retries and timeouts
type Sender struct {
	client      *http.Client
	maxRetries  int
	retryDelay  time.Duration
}

func NewSender(timeout time.Duration, maxRetries int) *Sender {
	return &Sender{
		client: &http.Client{Timeout: timeout},
		maxRetries: maxRetries,
		retryDelay: time.Second,
	}
}

// Send delivers a webhook with retry logic
func (s *Sender) Send(ctx context.Context, callback EventCallback, event base.PersistenceEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event payload: %w", err)
	}

	return s.SendWithRetry(ctx, callback.URL, callback.Headers, payload)
}

// SendWithRetry implements exponential backoff
func (s *Sender) SendWithRetry(ctx context.Context, url string, headers map[string]string, payload []byte) error {
	var err error
	for i := 0; i < s.maxRetries; i++ {
		err = s.sendRequest(ctx, url, headers, payload)
		if err == nil {
			return nil
		}
		time.Sleep(s.retryDelay * time.Duration(i*i))
	}
	return fmt.Errorf("failed to send webhook after %d retries: %w", s.maxRetries, err)
}

func (s *Sender) sendRequest(ctx context.Context, url string, headers map[string]string, payload []byte) error {
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook request failed with status code: %d", resp.StatusCode)
	}

	return nil
}
