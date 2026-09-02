// RPC example: demonstrates two transport channels — JSON and Anansi —
// for remote procedure calls over OS pipes.
//
// Architecture:
//
//      Parent (this binary, no flags)
//        ├── spawns Child (this binary --child) with stdin/stdout pipes
//        ├── sends RPC requests using the chosen channel
//        ├── reads RPC responses from the child
//        └── prints results and performance metrics
//
//      Child (this binary --child)
//        ├── reads RPC requests from stdin
//        ├── invokes the "echo" procedure
//        ├── sends RPC responses back to stdout
//        └── exits when done
//
// Run from repo root:
//
//      go run ./example/rpc                  # runs both channels
//      go run ./example/rpc -channel=json    # JSON only
//      go run ./example/rpc -channel=anansi  # Anansi only
package main

import (
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	anansi "github.com/asaidimu/go-anansi/v8/core/encoding/anansi"
	cjson "github.com/asaidimu/go-anansi/v8/core/encoding/json"
	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// ---------------------------------------------------------------------------
// Schema
// ---------------------------------------------------------------------------

const rpcSchema = `{
  "version": "1.0.0",
  "name": "rpc_message",
  "fields": {
    "f01": { "name": "method",    "type": "string",  "required": true },
    "f02": { "name": "id",        "type": "string",  "required": true },
    "f03": { "name": "payload",   "type": "string" },
    "f04": { "name": "timestamp", "type": "integer" },
    "f05": { "name": "nested",    "type": "object", "schema": { "id": "params" } },
    "f06": { "name": "tags",      "type": "array",  "schema": { "type": "string" } }
  },
  "schemas": {
    "params": {
      "name": "params",
      "fields": {
        "p1": { "name": "code",   "type": "integer" },
        "p2": { "name": "reason", "type": "string" }
      }
    }
  }
}`

const schemaVersion uint16 = 1

func compileSchema() *definition.CompiledSchema {
	s, err := definition.FromJSON([]byte(rpcSchema))
	if err != nil {
		panic(fmt.Sprintf("schema parse: %v", err))
	}
	rs, err := definition.Compile(s)
	if err != nil {
		panic(fmt.Sprintf("schema compile: %v", err))
	}
	cs, err := definition.Link(rs)
	if err != nil {
		panic(fmt.Sprintf("schema link: %v", err))
	}
	return cs
}

// ---------------------------------------------------------------------------
// RPC Message
// ---------------------------------------------------------------------------

type RPCMessage struct {
	Method    string            `json:"method"`
	ID        string            `json:"id"`
	Payload   string            `json:"payload"`
	Timestamp int64             `json:"timestamp"`
	Nested    map[string]any    `json:"nested,omitempty"`
	Tags      []string          `json:"tags,omitempty"`
}

// ---------------------------------------------------------------------------
// Channel interface
// ---------------------------------------------------------------------------

type Channel interface {
	Name() string
	Write(w io.Writer, msg *RPCMessage) (int, error)
	Read(r io.Reader) (*RPCMessage, error)
}

// ---------------------------------------------------------------------------
// JSON Channel
// ---------------------------------------------------------------------------

type JSONChannel struct{}

func (c *JSONChannel) Name() string { return "JSON" }

func (c *JSONChannel) Write(w io.Writer, msg *RPCMessage) (int, error) {
	data, err := json.Marshal(msg)
	if err != nil {
		return 0, fmt.Errorf("json marshal: %w", err)
	}
	hdr := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(hdr, uint64(len(data)))
	if _, err := w.Write(hdr[:n]); err != nil {
		return 0, fmt.Errorf("write length: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return 0, fmt.Errorf("write data: %w", err)
	}
	return len(data), nil
}

func (c *JSONChannel) Read(r io.Reader) (*RPCMessage, error) {
	msgLen, err := readUvarint(r)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, msgLen)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("read data (%d bytes): %w", msgLen, err)
	}
	var msg RPCMessage
	if err := json.Unmarshal(buf, &msg); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}
	return &msg, nil
}

// ---------------------------------------------------------------------------
// Anansi Channel
// ---------------------------------------------------------------------------

type AnansiChannel struct {
	cs *definition.CompiledSchema
}

func NewAnansiChannel() *AnansiChannel {
	return &AnansiChannel{cs: compileSchema()}
}

func (c *AnansiChannel) Name() string { return "Anansi" }

func (c *AnansiChannel) Write(w io.Writer, msg *RPCMessage) (int, error) {
	doc := container.NewDataContainer()
	setString(c.cs, doc, "method", msg.Method)
	setString(c.cs, doc, "id", msg.ID)
	setString(c.cs, doc, "payload", msg.Payload)
	setInt(c.cs, doc, "timestamp", msg.Timestamp)
	if msg.Nested != nil {
		setNested(c.cs, doc, "nested", msg.Nested)
	}
	if msg.Tags != nil {
		setTags(c.cs, doc, "tags", msg.Tags)
	}

	wire, err := anansi.EncodeDense(c.cs, doc, schemaVersion)
	if err != nil {
		return 0, fmt.Errorf("anansi encode: %w", err)
	}
	hdr := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(hdr, uint64(len(wire)))
	if _, err := w.Write(hdr[:n]); err != nil {
		return 0, fmt.Errorf("write length: %w", err)
	}
	if _, err := w.Write(wire); err != nil {
		return 0, fmt.Errorf("write data: %w", err)
	}
	return len(wire), nil
}

func (c *AnansiChannel) Read(r io.Reader) (*RPCMessage, error) {
	msgLen, err := readUvarint(r)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, msgLen)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("read data (%d bytes): %w", msgLen, err)
	}

	doc, _, err := anansi.DecodeAnansi(c.cs, buf)
	if err != nil {
		return nil, fmt.Errorf("anansi decode: %w", err)
	}

	m, err := cjson.Dump(c.cs, doc)
	if err != nil {
		return nil, fmt.Errorf("anansi dump: %w", err)
	}

	msg := &RPCMessage{
		Method:    getString(m, "method"),
		ID:        getString(m, "id"),
		Payload:   getString(m, "payload"),
		Timestamp: getInt64(m, "timestamp"),
		Tags:      getStringSlice(m, "tags"),
	}
	if nested, ok := m["nested"].(map[string]any); ok {
		msg.Nested = nested
	}
	return msg, nil
}

// WritePooled writes using a pooled DataContainer for reduced allocations.
func (c *AnansiChannel) WritePooled(w io.Writer, msg *RPCMessage, pool *container.Pool) (int, error) {
	doc := pool.Get()
	defer pool.Put(doc)

	setString(c.cs, doc, "method", msg.Method)
	setString(c.cs, doc, "id", msg.ID)
	setString(c.cs, doc, "payload", msg.Payload)
	setInt(c.cs, doc, "timestamp", msg.Timestamp)
	if msg.Nested != nil {
		setNested(c.cs, doc, "nested", msg.Nested)
	}
	if msg.Tags != nil {
		setTags(c.cs, doc, "tags", msg.Tags)
	}

	wire, err := anansi.EncodeDense(c.cs, doc, schemaVersion)
	if err != nil {
		return 0, fmt.Errorf("anansi encode: %w", err)
	}
	hdr := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(hdr, uint64(len(wire)))
	if _, err := w.Write(hdr[:n]); err != nil {
		return 0, fmt.Errorf("write length: %w", err)
	}
	if _, err := w.Write(wire); err != nil {
		return 0, fmt.Errorf("write data: %w", err)
	}
	return len(wire), nil
}

// ReadPooled reads using a pooled DataContainer for reduced allocations.
func (c *AnansiChannel) ReadPooled(r io.Reader, pool *container.Pool) (*RPCMessage, error) {
	msgLen, err := readUvarint(r)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, msgLen)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("read data (%d bytes): %w", msgLen, err)
	}

	doc := pool.Get()
	defer pool.Put(doc)

	_, err = anansi.DecodeAnansiInto(c.cs, buf, doc, pool)
	if err != nil {
		return nil, fmt.Errorf("anansi decode: %w", err)
	}

	m, err := cjson.Dump(c.cs, doc)
	if err != nil {
		return nil, fmt.Errorf("anansi dump: %w", err)
	}

	msg := &RPCMessage{
		Method:    getString(m, "method"),
		ID:        getString(m, "id"),
		Payload:   getString(m, "payload"),
		Timestamp: getInt64(m, "timestamp"),
		Tags:      getStringSlice(m, "tags"),
	}
	if nested, ok := m["nested"].(map[string]any); ok {
		msg.Nested = nested
	}
	return msg, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func readUvarint(r io.Reader) (uint64, error) {
	var buf [binary.MaxVarintLen64]byte
	var n int
	for n = 0; n < len(buf); n++ {
		if _, err := io.ReadFull(r, buf[n:n+1]); err != nil {
			return 0, fmt.Errorf("read length prefix: %w", err)
		}
		if buf[n]&0x80 == 0 {
			n++
			break
		}
	}
	val, _ := binary.Uvarint(buf[:n])
	return val, nil
}

func setString(cs *definition.CompiledSchema, doc *container.DataContainer, field, value string) {
	_ = cjson.DecodeJSONInto(cs, []byte(fmt.Sprintf(`{"%s":"%s"}`, field, value)), doc, nil)
}

func setInt(cs *definition.CompiledSchema, doc *container.DataContainer, field string, value int64) {
	_ = cjson.DecodeJSONInto(cs, []byte(fmt.Sprintf(`{"%s":%d}`, field, value)), doc, nil)
}

func setNested(cs *definition.CompiledSchema, doc *container.DataContainer, field string, value map[string]any) {
	data, _ := json.Marshal(value)
	_ = cjson.DecodeJSONInto(cs, []byte(fmt.Sprintf(`{"%s":%s}`, field, data)), doc, nil)
}

func setTags(cs *definition.CompiledSchema, doc *container.DataContainer, field string, value []string) {
	data, _ := json.Marshal(value)
	_ = cjson.DecodeJSONInto(cs, []byte(fmt.Sprintf(`{"%s":%s}`, field, data)), doc, nil)
}

func getString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getInt64(m map[string]any, key string) int64 {
	if v, ok := m[key].(float64); ok {
		return int64(v)
	}
	return 0
}

func getStringSlice(m map[string]any, key string) []string {
	if arr, ok := m[key].([]any); ok {
		result := make([]string, 0, len(arr))
		for _, v := range arr {
			if s, ok := v.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

// ---------------------------------------------------------------------------
// Client / Server
// ---------------------------------------------------------------------------

type Client struct {
	ch     Channel
	reader io.Reader
	writer io.Writer
}

func NewClient(ch Channel, r io.Reader, w io.Writer) *Client {
	return &Client{ch: ch, reader: r, writer: w}
}

func (c *Client) Call(method, payload string) (*RPCMessage, error) {
	req := &RPCMessage{
		Method:    method,
		ID:        fmt.Sprintf("req-%d", time.Now().UnixNano()),
		Payload:   payload,
		Timestamp: time.Now().Unix(),
		Tags:      []string{"rpc", "client"},
	}
	if _, err := c.ch.Write(c.writer, req); err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	resp, err := c.ch.Read(c.reader)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	return resp, nil
}

type Server struct {
	ch     Channel
	reader io.Reader
	writer io.Writer
}

func NewServer(ch Channel, r io.Reader, w io.Writer) *Server {
	return &Server{ch: ch, reader: r, writer: w}
}

func (s *Server) Handle() error {
	req, err := s.ch.Read(s.reader)
	if err != nil {
		return fmt.Errorf("read request: %w", err)
	}
	if req.Method == "shutdown" {
		return io.EOF
	}
	resp := &RPCMessage{
		Method:    req.Method,
		ID:        "resp-" + req.ID,
		Payload:   fmt.Sprintf("echo: %s", req.Payload),
		Timestamp: time.Now().Unix(),
		Nested:    map[string]any{"code": float64(200), "reason": "ok"},
		Tags:      []string{"rpc", "server", "echo"},
	}
	if _, err := s.ch.Write(s.writer, resp); err != nil {
		return fmt.Errorf("send response: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Child process
// ---------------------------------------------------------------------------

func runChild(ch Channel) {
	server := NewServer(ch, os.Stdin, os.Stdout)
	for {
		err := server.Handle()
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Fprintf(os.Stderr, "child: %v\n", err)
			return
		}
	}
}

func getChannel(name string) Channel {
	switch strings.ToLower(name) {
	case "json":
		return &JSONChannel{}
	case "anansi":
		return NewAnansiChannel()
	default:
		return &JSONChannel{}
	}
}

// ---------------------------------------------------------------------------
// Parent process
// ---------------------------------------------------------------------------

func runParent(ch Channel, numRequests int) {
	fmt.Printf("\n=== %s Channel ===\n", ch.Name())

	cmd := exec.Command(os.Args[0], "--child", ch.Name())
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		panic(fmt.Sprintf("stdin pipe: %v", err))
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		panic(fmt.Sprintf("stdout pipe: %v", err))
	}

	if err := cmd.Start(); err != nil {
		panic(fmt.Sprintf("start child: %v", err))
	}

	client := NewClient(ch, stdout, stdin)
	start := time.Now()

	for i := 0; i < numRequests; i++ {
		payload := fmt.Sprintf("request-%d", i+1)
		resp, err := client.Call("echo", payload)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parent: call %d failed: %v\n", i+1, err)
			continue
		}
		fmt.Printf("  [%d] id=%s payload=%s\n", i+1, resp.ID, resp.Payload)
	}

	// Send shutdown
	shutdownReq := &RPCMessage{
		Method:    "shutdown",
		ID:        "shutdown-1",
		Payload:   "",
		Timestamp: time.Now().Unix(),
	}
	ch.Write(stdin, shutdownReq)
	stdin.Close()

	if err := cmd.Wait(); err != nil && err.Error() != "exit status 0" {
		fmt.Fprintf(os.Stderr, "parent: child exit: %v\n", err)
	}

	elapsed := time.Since(start)
	fmt.Printf("  Total: %d requests in %v (%v per request)\n", numRequests, elapsed, elapsed/time.Duration(numRequests))
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

func main() {
	// Child mode: --child <channel>
	if len(os.Args) > 1 && os.Args[1] == "--child" {
		chName := "json"
		if len(os.Args) > 2 {
			chName = os.Args[2]
		}
		runChild(getChannel(chName))
		return
	}

	// Parent mode
	channelFlag := flag.String("channel", "both", "Channel to use: json, anansi, or both")
	flag.Parse()

	numRequests := 100
	channel := strings.ToLower(*channelFlag)

	switch channel {
	case "json":
		runParent(&JSONChannel{}, numRequests)
	case "anansi":
		runParent(NewAnansiChannel(), numRequests)
	case "both":
		runParent(&JSONChannel{}, numRequests)
		runParent(NewAnansiChannel(), numRequests)
	default:
		fmt.Fprintf(os.Stderr, "unknown channel: %s (use json, anansi, or both)\n", channel)
		os.Exit(1)
	}
}
