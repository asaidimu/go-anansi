// RPC example: demonstrates two transport channels — JSON and Anansi —
// for remote procedure calls over OS pipes.
//
// Architecture:
//
//	Parent (this binary, no flags)
//	  ├── spawns Child (this binary --child) with stdin/stdout pipes
//	  ├── sends RPC requests using the chosen channel
//	  ├── reads RPC responses from the child
//	  └── prints results and performance metrics
//
//	Child (this binary --child)
//	  ├── reads RPC requests from stdin
//	  ├── invokes the "echo" procedure
//	  ├── sends RPC responses back to stdout
//	  └── exits when done
//
// Run from repo root:
//
//	go run ./example/rpc                  # runs both channels
//	go run ./example/rpc -channel=json    # JSON only
//	go run ./example/rpc -channel=anansi  # Anansi only
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
	"unsafe"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	anansi "github.com/asaidimu/go-anansi/v8/core/encoding/anansi"
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
	Method    string         `json:"method"`
	ID        string         `json:"id"`
	Payload   string         `json:"payload"`
	Timestamp int64          `json:"timestamp"`
	Nested    map[string]any `json:"nested,omitempty"`
	Tags      []string       `json:"tags,omitempty"`
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
//
// The channel is JSON-free end to end. All seven wire keys (method, id,
// payload, timestamp, nested.code, nested.reason, tags — including the
// flattened-object child paths) are precomputed once per channel from the
// compiled schema, exactly as the codec's own computeLeafKey would compute
// them. Documents are then filled and drained in a single DataContainer.Walk
// pass each: typed backing slices are captured once via slot() and raw values
// are appended/read directly, with no per-field boxing, no map materialization
// and no intermediate JSON.
// ---------------------------------------------------------------------------

// rpcFieldKeys holds the DataContainerKey for every addressable field of the
// RPC schema, precomputed once at channel construction.
type rpcFieldKeys struct {
	method, id, payload, timestamp container.DataContainerKey
	nestedCode, nestedReason       container.DataContainerKey
	tags                           container.DataContainerKey
}

// AnansiChannel is a zero-JSON transport channel over the Anansi wire format.
type AnansiChannel struct {
	cs   *definition.CompiledSchema
	keys rpcFieldKeys
}

func NewAnansiChannel() *AnansiChannel {
	cs := compileSchema()
	return &AnansiChannel{cs: cs, keys: precomputeKeys(cs)}
}

func (c *AnansiChannel) Name() string { return "Anansi" }

// precomputeKeys mirrors the codec's computeLeafKey for every RPC field,
// using only the public schema API (cs.Address + NewDataPoint +
// NewDataContainerKey). Flattened object children (nested.*) are addressed at
// their full resolved path: [root.field, child.field].
func precomputeKeys(cs *definition.CompiledSchema) rpcFieldKeys {
	var keys rpcFieldKeys
	root := cs.Schemas[0]
	for j := uint16(0); j < root.FieldCount; j++ {
		abs := int(root.FieldStart) + int(j)
		fd := cs.Descriptors[abs]
		path := definition.ResolvedPath{definition.NewResolvedStep(0, uint8(j))}

		switch cs.FieldsMeta[abs].Name {
		case "method":
			keys.method = leafKey(cs, fd, path)
		case "id":
			keys.id = leafKey(cs, fd, path)
		case "payload":
			keys.payload = leafKey(cs, fd, path)
		case "timestamp":
			keys.timestamp = leafKey(cs, fd, path)
		case "tags":
			keys.tags = leafKey(cs, fd, path)
		case "nested":
			childIdx := fd.ChildSchemaIdx()
			child := cs.Schemas[childIdx]
			for c := uint16(0); c < child.FieldCount; c++ {
				cabs := int(child.FieldStart) + int(c)
				cfd := cs.Descriptors[cabs]
				cpath := definition.ResolvedPath{
					definition.NewResolvedStep(0, uint8(j)),
					definition.NewResolvedStep(childIdx, uint8(c)),
				}
				switch cs.FieldsMeta[cabs].Name {
				case "code":
					keys.nestedCode = leafKey(cs, cfd, cpath)
				case "reason":
					keys.nestedReason = leafKey(cs, cfd, cpath)
				}
			}
		}
	}
	return keys
}

// leafKey resolves a field's path to its DataContainerKey — a direct mirror of
// the codec's unexported computeLeafKey, so keys computed here always agree
// with keys the codec computes internally for the same field.
func leafKey(cs *definition.CompiledSchema, fd definition.FieldDescriptor, path definition.ResolvedPath) container.DataContainerKey {
	addr := cs.Address(path)
	if addr == 0 {
		return container.NewDataContainerKey(container.DataPoint(fd.DataPoint()), uint32(fd))
	}
	dp, err := container.NewDataPoint(fd.DataType(), int32(addr))
	if err != nil {
		panic(fmt.Sprintf("rpc: build data point: %v", err))
	}
	return container.NewDataContainerKey(dp, uint32(fd))
}

// fillDocument populates doc from msg in a single Walk pass: the container's
// typed backing slices are captured once via slot(), raw values are appended
// directly, and positions are recorded per precomputed key. The container
// invariant (positive indices index into the matching typed slice) is
// identical to what Append* maintains; mutation is safe here because the
// container is being materialized for encode.
func (c *AnansiChannel) fillDocument(doc *container.DataContainer, msg *RPCMessage) {
	_, _ = doc.Walk(func(positions map[int64]int32, slot func(container.DataType, ...int) unsafe.Pointer) (any, error) {
		strs := (*[]string)(slot(container.TypeString, 4))
		ints := (*[]int64)(slot(container.TypeInt, 2))
		arrs := (*[][]string)(slot(container.TypeArrayString, 1))

		// Strings: method, id, payload (+ nested.reason when present).
		*strs = append(*strs, msg.Method, msg.ID, msg.Payload)
		positions[int64(c.keys.method)] = 0
		positions[int64(c.keys.id)] = 1
		positions[int64(c.keys.payload)] = 2
		next := int32(3)
		if msg.Nested != nil {
			if raw, ok := msg.Nested["reason"]; ok && raw != nil {
				*strs = append(*strs, coerceString(raw))
				positions[int64(c.keys.nestedReason)] = next
				next++
			}
		}

		// Ints: timestamp (+ nested.code when present).
		*ints = append(*ints, msg.Timestamp)
		positions[int64(c.keys.timestamp)] = 0
		inext := int32(1)
		if msg.Nested != nil {
			if raw, ok := msg.Nested["code"]; ok && raw != nil {
				*ints = append(*ints, coerceInt(raw))
				positions[int64(c.keys.nestedCode)] = inext
				inext++
			}
		}

		// Tags: a single array-of-string value.
		if msg.Tags != nil {
			*arrs = append(*arrs, msg.Tags)
			positions[int64(c.keys.tags)] = 0
		}
		return nil, nil
	})
}

// drainDocument extracts the RPCMessage from a decoded document in a single
// Walk pass: values are read straight out of the decoder-filled typed slices
// via positions lookups. No Dump, no boxing, no JSON.
func (c *AnansiChannel) drainDocument(doc *container.DataContainer) *RPCMessage {
	msg := &RPCMessage{}
	_, _ = doc.Walk(func(positions map[int64]int32, slot func(container.DataType, ...int) unsafe.Pointer) (any, error) {
		strs := *(*[]string)(slot(container.TypeString))
		ints := *(*[]int64)(slot(container.TypeInt))
		arrs := *(*[][]string)(slot(container.TypeArrayString))

		if idx, ok := positions[int64(c.keys.method)]; ok && idx >= 0 && int(idx) < len(strs) {
			msg.Method = strs[idx]
		}
		if idx, ok := positions[int64(c.keys.id)]; ok && idx >= 0 && int(idx) < len(strs) {
			msg.ID = strs[idx]
		}
		if idx, ok := positions[int64(c.keys.payload)]; ok && idx >= 0 && int(idx) < len(strs) {
			msg.Payload = strs[idx]
		}
		if idx, ok := positions[int64(c.keys.timestamp)]; ok && idx >= 0 && int(idx) < len(ints) {
			msg.Timestamp = ints[idx]
		}

		codeSet, reasonSet := false, false
		var code int64
		var reason string
		if idx, ok := positions[int64(c.keys.nestedCode)]; ok && idx >= 0 && int(idx) < len(ints) {
			code, codeSet = ints[idx], true
		}
		if idx, ok := positions[int64(c.keys.nestedReason)]; ok && idx >= 0 && int(idx) < len(strs) {
			reason, reasonSet = strs[idx], true
		}
		if codeSet || reasonSet {
			msg.Nested = make(map[string]any, 2)
			if codeSet {
				// float64 mirrors encoding/json's numeric semantics for
				// map[string]any so both channels agree on the wire.
				msg.Nested["code"] = float64(code)
			}
			if reasonSet {
				msg.Nested["reason"] = reason
			}
		}

		if idx, ok := positions[int64(c.keys.tags)]; ok && idx >= 0 && int(idx) < len(arrs) {
			msg.Tags = arrs[idx]
		}
		return nil, nil
	})
	return msg
}

// writeFrame prefixes wire with a uvarint length header and writes it out.
func writeFrame(w io.Writer, wire []byte) (int, error) {
	var hdr [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(hdr[:], uint64(len(wire)))
	if _, err := w.Write(hdr[:n]); err != nil {
		return 0, fmt.Errorf("write length: %w", err)
	}
	if _, err := w.Write(wire); err != nil {
		return 0, fmt.Errorf("write data: %w", err)
	}
	return len(wire), nil
}

func (c *AnansiChannel) Write(w io.Writer, msg *RPCMessage) (int, error) {
	doc := container.NewDataContainer()
	c.fillDocument(doc, msg)

	wire, err := anansi.EncodeDense(c.cs, doc, schemaVersion)
	if err != nil {
		return 0, fmt.Errorf("anansi encode: %w", err)
	}
	return writeFrame(w, wire)
}

func (c *AnansiChannel) Read(r io.Reader) (*RPCMessage, error) {
	buf, err := readFrame(r)
	if err != nil {
		return nil, err
	}

	doc, _, err := anansi.DecodeAnansi(c.cs, buf)
	if err != nil {
		return nil, fmt.Errorf("anansi decode: %w", err)
	}
	return c.drainDocument(doc), nil
}

// WritePooled writes using a pooled DataContainer for reduced allocations.
// The returned frame does not alias the container.
func (c *AnansiChannel) WritePooled(w io.Writer, msg *RPCMessage, pool *container.Pool) (int, error) {
	doc := pool.Get()
	defer pool.Put(doc)

	c.fillDocument(doc, msg)

	wire, err := anansi.EncodeDense(c.cs, doc, schemaVersion)
	if err != nil {
		return 0, fmt.Errorf("anansi encode: %w", err)
	}
	return writeFrame(w, wire)
}

// ReadPooled reads using a pooled DataContainer for reduced allocations.
//
// The returned message's strings and tags alias the pooled container's
// backing buffers; use the message before the pool cycles (i.e. before the
// next Get/decode into the same container), or copy out the fields you need.
func (c *AnansiChannel) ReadPooled(r io.Reader, pool *container.Pool) (*RPCMessage, error) {
	buf, err := readFrame(r)
	if err != nil {
		return nil, err
	}

	doc := pool.Get()
	defer pool.Put(doc)

	_, err = anansi.DecodeAnansiInto(c.cs, buf, doc, pool)
	if err != nil {
		return nil, fmt.Errorf("anansi decode: %w", err)
	}
	return c.drainDocument(doc), nil
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

// readFrame reads one uvarint-length-prefixed frame.
func readFrame(r io.Reader) ([]byte, error) {
	msgLen, err := readUvarint(r)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, msgLen)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("read data (%d bytes): %w", msgLen, err)
	}
	return buf, nil
}

// coerceInt normalizes the numeric representations that can appear in
// RPCMessage.Nested (JSON unmarshal produces float64; direct Go construction
// may use any integer type) into the schema's integer type.
func coerceInt(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case int32:
		return int64(n)
	case float64:
		return int64(n)
	case float32:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	default:
		return 0
	}
}

func coerceString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
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
