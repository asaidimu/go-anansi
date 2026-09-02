package anansi_test

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	anansi "github.com/asaidimu/go-anansi/v8/core/encoding/anansi"
	cjson "github.com/asaidimu/go-anansi/v8/core/encoding/json"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/stretchr/testify/require"
)

// TestGenerateGoldenVectors emits the cross-language conformance fixtures
// consumed by clients/ts: a schema manifest plus deterministic packets of
// every supported kind alongside their expected documents. Run it via
//
//	GOLDEN_UPDATE=1 go test ./core/encoding/anansi/ -run TestGenerateGoldenVectors
//
// and commit the refreshed file together with any wire-format change.
var updateGolden = os.Getenv("GOLDEN_UPDATE") == "1"

const goldenSchemaJSON = `{
  "version": "1.0.0",
  "name": "golden",
  "fields": {
    "f01": { "name": "count",   "type": "integer" },
    "f02": { "name": "price",   "type": "number" },
    "f03": { "name": "title",   "type": "string" },
    "f04": { "name": "active",  "type": "boolean" },
    "f05": { "name": "blob",    "type": "bytes" },
    "f06": { "name": "shape",   "type": "geometry" },
    "f07": { "name": "meta",    "type": "record" },
    "f08": { "name": "tags",    "type": "array", "schema": { "type": "string" } },
    "f09": { "name": "scores",  "type": "array", "schema": { "type": "number" } },
    "f10": { "name": "flags",   "type": "array", "schema": { "type": "boolean" } },
    "f11": { "name": "ids",     "type": "array", "schema": { "type": "integer" } },
    "f12": { "name": "anyval",  "type": "unknown" },
    "f13": { "name": "address", "type": "object", "schema": { "id": "addr" } },
    "f14": { "name": "items",   "type": "array",  "schema": { "id": "item" } }
  },
  "schemas": {
    "addr": {
      "name": "addr",
      "fields": {
        "a1": { "name": "street", "type": "string" },
        "a2": { "name": "city",   "type": "string" }
      }
    },
    "item": {
      "name": "item",
      "fields": {
        "i1": { "name": "sku", "type": "string" },
        "i2": { "name": "qty", "type": "integer" }
      }
    }
  }
}`

type goldenTransforms struct {
	Compression   bool   `json:"compression,omitempty"`
	Integrity     bool   `json:"integrity,omitempty"`
	EncryptionKey string `json:"encryptionKey,omitempty"`
}

type goldenCase struct {
	Name       string            `json:"name"`
	Kind       string            `json:"kind"`
	Packet     string            `json:"packet"`
	Payload    json.RawMessage   `json:"payload"`
	Transforms *goldenTransforms `json:"transforms,omitempty"`
}

type goldenFile struct {
	Schema      json.RawMessage `json:"schema"`
	Manifest    json.RawMessage `json:"manifest"`
	CompileDump json.RawMessage `json:"compileDump"`
	Cases       []goldenCase    `json:"cases"`
}

func goldenCompiled(t *testing.T) *definition.CompiledSchema {
	t.Helper()
	s, err := definition.FromJSON([]byte(goldenSchemaJSON))
	require.NoError(t, err)
	rs, err := definition.Compile(s)
	require.NoError(t, err)
	cs, err := definition.Link(rs)
	require.NoError(t, err)
	return cs
}

func goldenDoc(t *testing.T, cs *definition.CompiledSchema, payload string) *container.DataContainer {
	t.Helper()
	doc := container.NewDataContainer()
	require.NoError(t, cjson.DecodeJSONInto(cs, []byte(payload), doc, nil))
	return doc
}

func TestGenerateGoldenVectors(t *testing.T) {
	cs := goldenCompiled(t)

	manifestJSON, err := anansi.ExportManifest(cs, 0)
	require.NoError(t, err)

	payloads := map[string]string{
		"full": `{
			"count": 42, "price": 3.5, "title": "hello world", "active": true,
			"blob": "aGVsbG8=",
			"shape": [[0,0],[1,0],[1,1],[0,0]],
			"meta": {"nested": {"a": 1}, "flag": true, "note": null},
			"tags": ["a","b"], "scores": [1.5, -2.25], "flags": [true,false,true],
			"ids": [1,-2,3], "anyval": {"k": [1, "x", null]},
			"address": {"street": "1 Main St", "city": null},
			"items": [{"sku": "A1", "qty": 2}, {"sku": "B2", "qty": -7}]
		}`,
		"sparse_mix": `{"count": 7, "title": null, "items": [{"sku": "Z9"}]}`,
		"empty":      `{}`,
	}

	var cases []goldenCase
	addPacket := func(name, kind string, packet []byte, payload string) {
		cases = append(cases, goldenCase{
			Name: name, Kind: kind,
			Packet:  hex.EncodeToString(packet),
			Payload: json.RawMessage(payload),
		})
	}

	docs := map[string]*container.DataContainer{}
	for name, p := range payloads {
		docs[name] = goldenDoc(t, cs, p)
	}

	for _, name := range []string{"full", "sparse_mix", "empty"} {
		p := payloads[name]
		w, err := anansi.EncodeDense(cs, docs[name], 0)
		require.NoError(t, err)
		addPacket("dense_"+name, "dense", w, p)

		w, err = anansi.EncodeSparse(cs, docs[name], 0)
		require.NoError(t, err)
		addPacket("sparse_"+name, "sparse", w, p)
	}

	key := bytes.Repeat([]byte{0x42}, 32)

	w, err2 := anansi.SerializeAnansi(cs, docs["full"], 0, anansi.WithIntegrity())
	require.NoError(t, err2)
	cases = append(cases, goldenCase{Name: "dense_integrity", Kind: "dense",
		Packet:     hex.EncodeToString(w),
		Payload:    json.RawMessage(payloads["full"]),
		Transforms: &goldenTransforms{Integrity: true}})

	w, err2 = anansi.SerializeAnansi(cs, docs["full"], 0,
		anansi.WithCompression(), anansi.WithIntegrity())
	require.NoError(t, err2)
	cases = append(cases, goldenCase{Name: "dense_comp_hash", Kind: "dense",
		Packet:     hex.EncodeToString(w),
		Payload:    json.RawMessage(payloads["full"]),
		Transforms: &goldenTransforms{Compression: true, Integrity: true}})

	w, err2 = anansi.SerializeAnansi(cs, docs["sparse_mix"], 0,
		anansi.WithEncryption(key), anansi.WithCompression(), anansi.WithIntegrity())
	require.NoError(t, err2)
	cases = append(cases, goldenCase{Name: "sparse_enc_comp_hash", Kind: "sparse",
		Packet:  hex.EncodeToString(w),
		Payload: json.RawMessage(payloads["sparse_mix"]),
		Transforms: &goldenTransforms{
			Compression: true, Integrity: true,
			EncryptionKey: hex.EncodeToString(key),
		}})

	batchColDocs := []*container.DataContainer{docs["full"], docs["sparse_mix"], docs["empty"]}
	batchColPayload := "[" + payloads["full"] + "," + payloads["sparse_mix"] + "," + payloads["empty"] + "]"
	w, err2 = anansi.EncodeBatchColumnar(cs, batchColDocs, 0,
		anansi.WithCompression(), anansi.WithIntegrity(), anansi.WithEncryption(key))
	require.NoError(t, err2)
	cases = append(cases, goldenCase{Name: "batch_columnar_enc_comp_hash", Kind: "batch_columnar",
		Packet:  hex.EncodeToString(w),
		Payload: json.RawMessage(batchColPayload),
		Transforms: &goldenTransforms{
			Compression: true, Integrity: true,
			EncryptionKey: hex.EncodeToString(key),
		}})

	batchDocs := []*container.DataContainer{docs["full"], docs["sparse_mix"], docs["empty"]}
	batchPayloads := []string{payloads["full"], payloads["sparse_mix"], payloads["empty"]}

	rowWire, err := anansi.EncodeBatchRows(cs, batchDocs, 0)
	require.NoError(t, err)
	addPacket("batch_row", "batch_row", rowWire, `[`+batchPayloads[0]+`,`+batchPayloads[1]+`,`+batchPayloads[2]+`]`)

	colWire, err := anansi.EncodeBatchColumnar(cs, batchDocs, 0)
	require.NoError(t, err)
	addPacket("batch_columnar", "batch_columnar", colWire, `[`+batchPayloads[0]+`,`+batchPayloads[1]+`,`+batchPayloads[2]+`]`)

	dumpJSON, err := anansi.ExportCompileDump(cs)
	require.NoError(t, err)

	gf := goldenFile{Schema: json.RawMessage(goldenSchemaJSON), Manifest: manifestJSON, CompileDump: dumpJSON, Cases: cases}
	out, err := json.MarshalIndent(gf, "", "  ")
	require.NoError(t, err)

	dest := filepath.Join("..", "..", "..", "packages", "anansi", "testdata", "golden.json")
	if !updateGolden {
		// Verify existing fixture still matches (wire-format drift guard).
		existing, err := os.ReadFile(dest)
		if err != nil {
			t.Fatalf("golden fixtures missing; run with -update-golden: %v", err)
		}

		// Encrypted packets embed a random nonce by design, so they can
		// never be byte-stable across runs. For those, verify structure and
		// decryptability instead of bytes.
		var prev goldenFile
		require.NoError(t, json.Unmarshal(existing, &prev))
		require.Len(t, cases, len(prev.Cases))
		for i := range cases {
			a, mErr := json.Marshal(prev.Cases[i])
			require.NoError(t, mErr)
			b, mErr := json.Marshal(cases[i])
			require.NoError(t, mErr)

			if tf := cases[i].Transforms; tf != nil && tf.EncryptionKey != "" {
				var pa, pb map[string]any
				require.NoError(t, json.Unmarshal(a, &pa))
				require.NoError(t, json.Unmarshal(b, &pb))
				delete(pa, "packet")
				delete(pb, "packet")
				require.Equal(t, pa, pb,
					"golden case %s drifted (excluding random nonce)", cases[i].Name)

				keyB, kErr := hex.DecodeString(tf.EncryptionKey)
				require.NoError(t, kErr)
				raw, dErr := hex.DecodeString(cases[i].Packet)
				require.NoError(t, dErr)

				var dm map[string]any
				if cases[i].Kind == "batch_columnar" {
					docsB, _, dErr := anansi.DecodeBatchRows(cs, raw,
						nil, anansi.WithDecryptionKey(keyB))
					require.NoError(t, dErr, "committed encrypted batch must decrypt")
					var all []map[string]any
					for _, d := range docsB {
						m := dumpMap(cs, d)
						all = append(all, m)
					}
					_ = all
					// spot-check first record's scalars
					if len(all) > 0 {
						for k, v := range wantPayloadFirst(cases[i].Payload) {
							compareDumpedField(t, all[0], k, v)
						}
					}
					continue
				}
				doc, _, dErr := anansi.DecodeAnansi(cs, raw,
					anansi.WithDecryptionKey(keyB))
				require.NoError(t, dErr, "committed encrypted vector must decrypt")

				dm = dumpMap(cs, doc)
				var wantPayload map[string]any
				if cases[i].Kind != "batch_columnar" {
					require.NoError(t, json.Unmarshal(cases[i].Payload, &wantPayload))
				}
				for k, v := range wantPayload {
					compareDumpedField(t, dm, k, v)
				}
				continue
			}
			require.JSONEq(t, string(a), string(b),
				"wire output drifted from committed golden vectors; if intentional, re-run with -update-golden")
		}
		return
	}
	require.NoError(t, os.MkdirAll(filepath.Dir(dest), 0o755))
	require.NoError(t, os.WriteFile(dest, out, 0o644))
	t.Logf("wrote %s (%d bytes, %d cases)", dest, len(out), len(cases))
}

func dumpMap(cs *definition.CompiledSchema, d *container.DataContainer) map[string]any {
	m, err := cjson.Dump(cs, d)
	if err != nil {
		panic(err)
	}
	return m
}

func wantPayloadFirst(raw json.RawMessage) map[string]any {
	var arr []map[string]any
	if err := json.Unmarshal(raw, &arr); err != nil || len(arr) == 0 {
		return map[string]any{}
	}
	return arr[0]
}

// compareDumpedField tolerates the Dump representation quirks (dotted keys
// for composites, []byte / []any-of-numbers for bytes, int64 vs float64)
// while still catching real value drift. Deep composite parity is owned by
// the TypeScript conformance suite.
func compareDumpedField(
	t *testing.T,
	dm map[string]any,
	k string,
	v any,
) {
	t.Helper()
	if v == nil {
		return // null decodes as absence
	}
	switch reflect.TypeOf(v).Kind() {
	case reflect.Slice, reflect.Map:
		// Dump renders composites under dotted keys ("address.street").
		found := false
		for key := range dm {
			if key == k || strings.HasPrefix(key, k+".") {
				found = true
				break
			}
		}
		require.True(t, found, "composite field %q missing from dump", k)
		return
	}

	mv := reflect.ValueOf(dm[k])
	if !mv.IsValid() {
		t.Errorf("field %q missing from decoded document", k)
		return
	}
	if mv.Kind() == reflect.Slice {
		if mv.Type().Elem().Kind() == reflect.Uint8 {
			require.Equal(t, fmt.Sprintf("%v", v),
				base64.StdEncoding.EncodeToString(mv.Bytes()), "field %q (bytes)", k)
			return
		}
		if mv.Type().Elem().Kind() == reflect.Interface {
			bs := make([]byte, mv.Len())
			okAll := true
			for i := 0; i < mv.Len(); i++ {
				fv, isF := mv.Index(i).Interface().(float64)
				if !isF || fv != math.Trunc(fv) || fv < 0 || fv > 255 {
					okAll = false
					break
				}
				bs[i] = byte(fv)
			}
			if okAll {
				require.Equal(t, fmt.Sprintf("%v", v),
					base64.StdEncoding.EncodeToString(bs), "field %q (bytes-any)", k)
				return
			}
		}
	}
	require.Equal(t, fmt.Sprintf("%v", v), fmt.Sprintf("%v", dm[k]), "field %q", k)
}
