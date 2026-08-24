// Step 2 — GO SERVER: decode the client's packets with the production Go
// codec (definition.Compile/Link + anansi.DecodeAnansiInto / DecodeBatchRows),
// verify contents, then answer with a Dense single and a COLUMNAR batch
// result set for the client to decode.
//
// Run from repo root:  go run ./examples/encoding/go-server

package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	anansi "github.com/asaidimu/go-anansi/v8/core/encoding/anansi"
	cjson "github.com/asaidimu/go-anansi/v8/core/encoding/json"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
)

const schemaJSON = `{
  "version": "1.0.0",
  "name": "orders",
  "fields": {
    "f01": { "name": "order_id", "type": "string", "required": true },
    "f02": { "name": "total",    "type": "number" },
    "f03": { "name": "paid",     "type": "boolean" },
    "f04": { "name": "receipt",  "type": "bytes" },
    "f05": { "name": "tags",     "type": "array", "schema": { "type": "string" } },
    "f06": { "name": "customer", "type": "object", "schema": { "id": "customer" } },
    "f07": { "name": "lines",    "type": "array",  "schema": { "id": "line" } }
  },
  "schemas": {
    "customer": {
      "name": "customer",
      "fields": {
        "c1": { "name": "email", "type": "string" },
        "c2": { "name": "tier",  "type": "string" }
      }
    },
    "line": {
      "name": "line",
      "fields": {
        "l1": { "name": "sku", "type": "string" },
        "l2": { "name": "qty", "type": "integer" }
      }
    }
  }
}`

func mustHex(h string) []byte {
	b, err := hex.DecodeString(h)
	if err != nil {
		panic(err)
	}
	return b
}

func compile() *definition.CompiledSchema {
	s, err := definition.FromJSON([]byte(schemaJSON))
	if err != nil {
		panic(err)
	}
	rs, err := definition.Compile(s)
	if err != nil {
		panic(err)
	}
	cs, err := definition.Link(rs)
	if err != nil {
		panic(err)
	}
	return cs
}

// dump renders a decoded document through the JSON codec so nested values
// can be read by dotted path ("customer.email").
func dump(cs *definition.CompiledSchema, doc *container.DataContainer) map[string]any {
	m, err := cjson.Dump(cs, doc)
	if err != nil {
		panic(err)
	}
	return m
}

func buildResults(cs *definition.CompiledSchema) []*container.DataContainer {
	full := `{"order_id":"ORD-2026-0001","total":129.99,"paid":true}`
	partial := `{"order_id":"ORD-2026-0002","total":42.5}`

	mk := func(payload string) *container.DataContainer {
		d := container.NewDataContainer()
		if err := cjson.DecodeJSONInto(cs, []byte(payload), d, nil); err != nil {
			panic(err)
		}
		return d
	}
	return []*container.DataContainer{mk(full), mk(partial)}
}

func main() {
	reqRaw, err := os.ReadFile("examples/encoding/out/client-request.json")
	if err != nil {
		fmt.Println("run the ts-client step first:", err)
		os.Exit(1)
	}
	var req struct {
		FullVersion uint16 `json:"fullVersion"`
		Dense       string `json:"dense"`
		Sparse      string `json:"sparse"`
		Batch       string `json:"batch"`
	}
	if err := json.Unmarshal(reqRaw, &req); err != nil {
		panic(err)
	}

	cs := compile()
	fmt.Println("server ← client  (Go codec decoding TS-encoded packets)")

	doc, version, err := anansi.DecodeAnansi(cs, mustHex(req.Dense))
	if err != nil {
		panic(err)
	}
	d1 := dump(cs, doc)
	fmt.Printf("  dense : v%d order_id=%q total=%.2f\n",
		version, d1["order_id"], d1["total"])

	doc, _, err = anansi.DecodeAnansi(cs, mustHex(req.Sparse))
	if err != nil {
		panic(err)
	}
	d2 := dump(cs, doc)
	lines, _ := d2["lines"].([]any)
	fmt.Printf("  sparse: customer.email=%q lines=%d\n",
		d2["customer.email"], len(lines))

	docs, version, err := anansi.DecodeBatchRows(cs, mustHex(req.Batch), nil)
	if err != nil {
		panic(err)
	}
	fmt.Printf("  batch : v%d %d records decoded\n", version, len(docs))

	// ── server → client response ────────────────────────────────────
	resultSet := buildResults(cs)

	resp := struct {
		FullVersion uint16 `json:"fullVersion"`
		Single      string `json:"single"`
		Columnar    string `json:"columnar"`
	}{FullVersion: 7}

	wire, err := anansi.EncodeDense(cs, resultSet[0], resp.FullVersion)
	if err != nil {
		panic(err)
	}
	resp.Single = hex.EncodeToString(wire)

	colWire, err := anansi.EncodeBatchColumnar(cs, resultSet, resp.FullVersion)
	if err != nil {
		panic(err)
	}
	resp.Columnar = hex.EncodeToString(colWire)

	out, _ := json.MarshalIndent(resp, "", "  ")
	if err := os.WriteFile("examples/encoding/out/server-response.json", out, 0o644); err != nil {
		panic(err)
	}
	fmt.Println("server → client  wrote out/server-response.json")
}
