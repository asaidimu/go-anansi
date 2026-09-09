package document

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/data/container"
	cjson "github.com/asaidimu/go-anansi/v8/core/encoding/json"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

const scannerScalarsSchemaJSON = `{
  "version": "1.0.0",
  "name": "scannerscalars",
  "fields": {
    "id":     { "name": "id",     "type": "string" },
    "active": { "name": "active", "type": "boolean" },
    "age":    { "name": "age",    "type": "integer" },
    "score":  { "name": "score",  "type": "number" },
    "payload": { "name": "payload", "type": "bytes" }
  }
}`

// rowForDoc materializes a document into the shape a single-table SELECT row
// would have: one column per root field of the enriched schema, scalar values
// in their SQLite representation (booleans as 0/1), and container-type fields
// as their serialized JSON fragment. Absent fields become NULL.
func rowForDoc(t *testing.T, col *DocumentPool, d *Document) ([]string, []any) {
	t.Helper()
	cs := col.CompiledSchema()
	slot := cs.Schemas[0]
	var columns []string
	var values []any
	for j := uint16(0); j < slot.FieldCount; j++ {
		abs := int(slot.FieldStart) + int(j)
		name := cs.FieldsMeta[abs].Name
		ft := cs.FieldTypes[abs]
		columns = append(columns, name)
		if ft.IsContainer() {
			if _, err := d.Get(name); err != nil {
				values = append(values, nil) // absent field → NULL column
				continue
			}
			b, err := d.SerializeField(name)
			if err != nil {
				values = append(values, nil)
				continue
			}
			values = append(values, string(b))
			continue
		}
		if name == data.DocumentIDField {
			values = append(values, d.ID())
			continue
		}
		v, err := d.Get(name)
		if err != nil {
			values = append(values, nil)
			continue
		}
		switch tv := v.(type) {
		case bool:
			if tv {
				values = append(values, int64(1))
			} else {
				values = append(values, int64(0))
			}
		default:
			values = append(values, v)
		}
	}
	return columns, values
}

func TestDocument_ScanRowRoundTrip(t *testing.T) {
	col := newTestCollection(t)
	d := testDocument(t)

	columns, values := rowForDoc(t, col, d)
	plan, err := col.PlanRow(columns, "__matches__")
	require.NoError(t, err)
	require.Equal(t, -1, plan.TotalIndex())

	scanned, err := col.ScanRow(context.Background(), plan, values)
	require.NoError(t, err)

	orig, err := cjson.SerializeJSON(col.CompiledSchema(), d.c)
	require.NoError(t, err)
	got, err := cjson.SerializeJSON(col.CompiledSchema(), scanned.c)
	require.NoError(t, err)
	require.JSONEq(t, string(orig), string(got))

	require.Equal(t, d.ID(), scanned.ID())
}

func TestDocument_ScanRowScalars(t *testing.T) {
	col := newScannerScalarsCollection(t)
	d, err := col.FromMap(map[string]any{
		"id":      "scalar-1",
		"active":  true,
		"age":     31,
		"score":   9.5,
		"payload": []byte("raw-blob"),
	})
	require.NoError(t, err)

	columns, values := rowForDoc(t, col, d)
	plan, err := col.PlanRow(columns, "__matches__")
	require.NoError(t, err)

	scanned, err := col.ScanRow(context.Background(), plan, values)
	require.NoError(t, err)

	// The scanned document carries the same identity the row was built from.
	require.Equal(t, d.ID(), scanned.ID())
	got, err := scanned.Get("active")
	require.NoError(t, err)
	require.Equal(t, true, got)
	got, err = scanned.Get("age")
	require.NoError(t, err)
	require.Equal(t, int64(31), got)
	got, err = scanned.Get("score")
	require.NoError(t, err)
	require.Equal(t, 9.5, got)
	got, err = scanned.Get("payload")
	require.NoError(t, err)
	require.Equal(t, []byte("raw-blob"), got)
}

func TestDocument_ScanRowNullIsAbsence(t *testing.T) {
	col := newScannerScalarsCollection(t)
	d, err := col.FromMap(map[string]any{"id": "sparse"})
	require.NoError(t, err)

	columns, values := rowForDoc(t, col, d)
	plan, err := col.PlanRow(columns, "__matches__")
	require.NoError(t, err)

	scanned, err := col.ScanRow(context.Background(), plan, values)
	require.NoError(t, err)

	// Fields never written stay absent: no value and no error.
	_, err = scanned.Get("payload")
	require.Error(t, err)
	_, err = scanned.Get("age")
	require.Error(t, err)
}

func TestDocument_PlanRowTotalColumn(t *testing.T) {
	col := newTestCollection(t)
	plan, err := col.PlanRow([]string{"testdoc.id", "__matches__", "alias"}, "__matches__")
	require.NoError(t, err)
	require.Equal(t, 1, plan.TotalIndex())
	// Unresolvable columns ("alias") are skipped → absence.
	if len(plan.fields) != 1 {
		t.Fatalf("expected 1 resolvable field, got %d", len(plan.fields))
	}
	require.Equal(t, "id", plan.fields[0].name)
}

func newScannerScalarsCollection(t *testing.T) *DocumentPool {
	t.Helper()
	s, err := definition.FromJSON([]byte(scannerScalarsSchemaJSON))
	require.NoError(t, err)
	col, err := NewDocumentPool(s)
	require.NoError(t, err)
	return col
}

// The enriched root schema is field-ordered as _id_, _metadata_, then the
// remaining root fields alphabetically. Locate "age" by name rather than index.
func TestDocument_ScanRowRootFieldKey(t *testing.T) {
	col := newScannerScalarsCollection(t)
	cs := col.CompiledSchema()
	slot := cs.Schemas[0]

	var abs int
	for j := uint16(0); j < slot.FieldCount; j++ {
		if cs.FieldsMeta[slot.FieldStart+j].Name == "age" {
			abs = int(slot.FieldStart) + int(j)
			break
		}
	}
	if abs == 0 {
		t.Fatal("field age not found")
	}

	key, err := rootFieldKey(cs, cs.Descriptors[abs], uint8(abs))
	require.NoError(t, err)
	require.Equal(t, cs.Descriptors[abs].DataType(), key.Type())

	// The key is addressable: a value written with the scanner's key is
	// retrievable through the regular container API.
	doc := container.NewDataContainer()
	require.NoError(t, doc.SetInt(key, 7))
	got, ok, err := doc.GetInt(key)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, int64(7), got)
}
