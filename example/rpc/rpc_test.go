package main

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
)

// TestRPCBothChannels verifies both JSON and Anansi channels work correctly.
func TestRPCBothChannels(t *testing.T) {
	channels := []struct {
		name    string
		channel Channel
	}{
		{"JSON", &JSONChannel{}},
		{"Anansi", NewAnansiChannel()},
	}

	for _, ch := range channels {
		t.Run(ch.name, func(t *testing.T) {
			var buf bytes.Buffer
			server := NewServer(ch.channel, &buf, &buf)

			// Send request
			req := &RPCMessage{
				Method:  "echo",
				ID:      "test-1",
				Payload: "hello",
			}
			if _, err := ch.channel.Write(&buf, req); err != nil {
				t.Fatalf("write request: %v", err)
			}

			// Handle request
			if err := server.Handle(); err != nil {
				t.Fatalf("handle: %v", err)
			}

			// Read response
			resp, err := ch.channel.Read(&buf)
			if err != nil {
				t.Fatalf("read response: %v", err)
			}

			if resp.Payload != "echo: hello" {
				t.Errorf("payload = %q, want %q", resp.Payload, "echo: hello")
			}
			if resp.Method != "echo" {
				t.Errorf("method = %q, want %q", resp.Method, "echo")
			}
		})
	}
}

// TestChannelEquivalence verifies the JSON and Anansi channels agree on every
// field of a fully-populated message — guarding against silent field drops
// (timestamp and nested.* were both silently lost by the pre-Walk read path).
func TestChannelEquivalence(t *testing.T) {
	sent := &RPCMessage{
		Method:    "echo",
		ID:        "eq-1",
		Payload:   "equivalence payload",
		Timestamp: 1700000123,
		Nested:    map[string]any{"code": float64(404), "reason": "not found"},
		Tags:      []string{"a", "b", "c"},
	}

	var jbuf, abuf bytes.Buffer
	jch := &JSONChannel{}
	ach := NewAnansiChannel()

	if _, err := jch.Write(&jbuf, sent); err != nil {
		t.Fatalf("json write: %v", err)
	}
	if _, err := ach.Write(&abuf, sent); err != nil {
		t.Fatalf("anansi write: %v", err)
	}

	gotJSON, err := jch.Read(&jbuf)
	if err != nil {
		t.Fatalf("json read: %v", err)
	}
	gotAnansi, err := ach.Read(&abuf)
	if err != nil {
		t.Fatalf("anansi read: %v", err)
	}

	if !reflect.DeepEqual(gotJSON, gotAnansi) {
		t.Errorf("channels disagree:\n  json:   %+v\n  anansi: %+v", gotJSON, gotAnansi)
	}
	if !reflect.DeepEqual(gotAnansi, sent) {
		t.Errorf("anansi round trip changed message:\n  sent: %+v\n  got:  %+v", sent, gotAnansi)
	}
}

// TestPooledChannelEquivalence verifies the pooled read/write paths survive
// pool reuse across Clear: each message is fully compared before the pool
// cycles again.
func TestPooledChannelEquivalence(t *testing.T) {
	ch := NewAnansiChannel()
	pool := container.NewPool()

	msgs := []*RPCMessage{
		{Method: "m1", ID: "id-1", Payload: "one", Timestamp: 1,
			Nested: map[string]any{"code": float64(1), "reason": "r1"},
			Tags:   []string{"x"}},
		{Method: "m2", ID: "id-2", Payload: "two", Timestamp: 22},
		{Method: "m3", ID: "id-3", Payload: "three", Timestamp: 333,
			Tags: []string{"p", "q", "r"}},
	}

	for i, sent := range msgs {
		var buf bytes.Buffer
		if _, err := ch.WritePooled(&buf, sent, pool); err != nil {
			t.Fatalf("msg %d write: %v", i, err)
		}
		got, err := ch.ReadPooled(&buf, pool)
		if err != nil {
			t.Fatalf("msg %d read: %v", i, err)
		}
		if !reflect.DeepEqual(got, sent) {
			t.Errorf("msg %d mismatch:\n  sent: %+v\n  got:  %+v", i, sent, got)
		}
	}
}

// BenchmarkJSONRoundTrip benchmarks a full JSON RPC round trip.
func BenchmarkJSONRoundTrip(b *testing.B) {
	ch := &JSONChannel{}
	msg := &RPCMessage{
		Method:    "echo",
		ID:        "bench-1",
		Payload:   "benchmark payload data",
		Timestamp: 1234567890,
		Nested:    map[string]any{"code": float64(200), "reason": "ok"},
		Tags:      []string{"benchmark", "test"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		if _, err := ch.Write(&buf, msg); err != nil {
			b.Fatal(err)
		}
		_, err := ch.Read(&buf)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkAnansiRoundTrip benchmarks a full Anansi RPC round trip.
func BenchmarkAnansiRoundTrip(b *testing.B) {
	ch := NewAnansiChannel()
	msg := &RPCMessage{
		Method:    "echo",
		ID:        "bench-1",
		Payload:   "benchmark payload data",
		Timestamp: 1234567890,
		Nested:    map[string]any{"code": float64(200), "reason": "ok"},
		Tags:      []string{"benchmark", "test"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		if _, err := ch.Write(&buf, msg); err != nil {
			b.Fatal(err)
		}
		_, err := ch.Read(&buf)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkJSONWrite benchmarks only the JSON write path.
func BenchmarkJSONWrite(b *testing.B) {
	ch := &JSONChannel{}
	msg := &RPCMessage{
		Method:    "echo",
		ID:        "bench-1",
		Payload:   "benchmark payload data",
		Timestamp: 1234567890,
		Nested:    map[string]any{"code": float64(200), "reason": "ok"},
		Tags:      []string{"benchmark", "test"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		if _, err := ch.Write(&buf, msg); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkAnansiWrite benchmarks only the Anansi write path.
func BenchmarkAnansiWrite(b *testing.B) {
	ch := NewAnansiChannel()
	msg := &RPCMessage{
		Method:    "echo",
		ID:        "bench-1",
		Payload:   "benchmark payload data",
		Timestamp: 1234567890,
		Nested:    map[string]any{"code": float64(200), "reason": "ok"},
		Tags:      []string{"benchmark", "test"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		if _, err := ch.Write(&buf, msg); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkJSONRead benchmarks only the JSON read path.
func BenchmarkJSONRead(b *testing.B) {
	ch := &JSONChannel{}
	msg := &RPCMessage{
		Method:    "echo",
		ID:        "bench-1",
		Payload:   "benchmark payload data",
		Timestamp: 1234567890,
		Nested:    map[string]any{"code": float64(200), "reason": "ok"},
		Tags:      []string{"benchmark", "test"},
	}

	// Pre-encode
	var buf bytes.Buffer
	ch.Write(&buf, msg) //nolint:errcheck
	encoded := buf.Bytes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(encoded)
		_, err := ch.Read(r)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkAnansiRead benchmarks only the Anansi read path.
func BenchmarkAnansiRead(b *testing.B) {
	ch := NewAnansiChannel()
	msg := &RPCMessage{
		Method:    "echo",
		ID:        "bench-1",
		Payload:   "benchmark payload data",
		Timestamp: 1234567890,
		Nested:    map[string]any{"code": float64(200), "reason": "ok"},
		Tags:      []string{"benchmark", "test"},
	}

	// Pre-encode
	var buf bytes.Buffer
	ch.Write(&buf, msg) //nolint:errcheck
	encoded := buf.Bytes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(encoded)
		_, err := ch.Read(r)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkAnansiRoundTripPooled benchmarks a full Anansi RPC round trip with pooled documents.
func BenchmarkAnansiRoundTripPooled(b *testing.B) {
	ch := NewAnansiChannel()
	pool := container.NewPool()
	msg := &RPCMessage{
		Method:    "echo",
		ID:        "bench-1",
		Payload:   "benchmark payload data",
		Timestamp: 1234567890,
		Nested:    map[string]any{"code": float64(200), "reason": "ok"},
		Tags:      []string{"benchmark", "test"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		if _, err := ch.WritePooled(&buf, msg, pool); err != nil {
			b.Fatal(err)
		}
		_, err := ch.ReadPooled(&buf, pool)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkAnansiWritePooled benchmarks only the Anansi write path with pooled documents.
func BenchmarkAnansiWritePooled(b *testing.B) {
	ch := NewAnansiChannel()
	pool := container.NewPool()
	msg := &RPCMessage{
		Method:    "echo",
		ID:        "bench-1",
		Payload:   "benchmark payload data",
		Timestamp: 1234567890,
		Nested:    map[string]any{"code": float64(200), "reason": "ok"},
		Tags:      []string{"benchmark", "test"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		if _, err := ch.WritePooled(&buf, msg, pool); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkAnansiReadPooled benchmarks only the Anansi read path with pooled documents.
func BenchmarkAnansiReadPooled(b *testing.B) {
	ch := NewAnansiChannel()
	pool := container.NewPool()
	msg := &RPCMessage{
		Method:    "echo",
		ID:        "bench-1",
		Payload:   "benchmark payload data",
		Timestamp: 1234567890,
		Nested:    map[string]any{"code": float64(200), "reason": "ok"},
		Tags:      []string{"benchmark", "test"},
	}

	// Pre-encode
	var buf bytes.Buffer
	ch.Write(&buf, msg) //nolint:errcheck
	encoded := buf.Bytes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(encoded)
		_, err := ch.ReadPooled(r, pool)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkWireSize compares the wire size of JSON vs Anansi.
func BenchmarkWireSize(b *testing.B) {
	channels := []struct {
		name    string
		channel Channel
	}{
		{"JSON", &JSONChannel{}},
		{"Anansi", NewAnansiChannel()},
	}

	msg := &RPCMessage{
		Method:    "echo",
		ID:        "bench-1",
		Payload:   "benchmark payload data",
		Timestamp: 1234567890,
		Nested:    map[string]any{"code": float64(200), "reason": "ok"},
		Tags:      []string{"benchmark", "test"},
	}

	for _, ch := range channels {
		b.Run(ch.name, func(b *testing.B) {
			var buf bytes.Buffer
			ch.channel.Write(&buf, msg) //nolint:errcheck
			b.Logf("Wire size: %d bytes", buf.Len())
			b.SetBytes(int64(buf.Len()))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var buf bytes.Buffer
				ch.channel.Write(&buf, msg) //nolint:errcheck
			}
		})
	}
}
