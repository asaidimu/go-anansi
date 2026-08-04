package document

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/data/container"
	cjson "github.com/asaidimu/go-anansi/v8/core/encoding/json"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// ============================================================================
// Row Scanning
// ============================================================================

// RowScan maps a query's result columns to container coordinates. It is built
// once per query — result columns are fixed — and reused for every row, so
// column resolution and address computation happen exactly once per query
// rather than once per row.
type RowScan struct {
	fields []fieldScan
	total  int // index of the query's total-count column, or -1
}

// TotalIndex returns the index of the query's internal total-count column, or
// -1 when the query did not project one.
func (s *RowScan) TotalIndex() int {
	if s == nil {
		return -1
	}
	return s.total
}

type fieldScan struct {
	col  int    // column index in the result row
	name string // root field name (table-prefix stripped)
	key  container.DataContainerKey
	dt   container.DataType
	json bool // FieldType.IsContainer() — column carries a serialized JSON fragment
}

// PlanRow resolves result columns into a RowScan against this pool's compiled
// schema. A column that cannot be resolved to a root field (an alias, a
// computed value, or the internal total-count column named by matchCol) is
// skipped rather than erroring, so any projection shape can be scanned; a
// skipped column is simply absent from the resulting document.
func (c *DocumentPool) PlanRow(columns []string, matchCol string) (*RowScan, error) {
	s := &RowScan{total: -1}
	if c == nil || c.cs == nil || len(c.cs.Schemas) == 0 {
		return s, nil
	}
	slot := c.cs.Schemas[0]
	for col, name := range columns {
		if name == matchCol {
			s.total = col
			continue
		}
		if i := strings.IndexByte(name, '.'); i >= 0 {
			name = name[i+1:]
		}
		j := -1
		for jj := uint16(0); jj < slot.FieldCount; jj++ {
			if c.cs.FieldsMeta[slot.FieldStart+jj].Name == name {
				j = int(jj)
				break
			}
		}
		if j < 0 {
			continue // unresolvable column → absence
		}
		abs := int(slot.FieldStart) + j
		fd := c.cs.Descriptors[abs]
		key, err := rootFieldKey(c.cs, fd, uint8(j))
		if err != nil {
			return nil, err
		}
		s.fields = append(s.fields, fieldScan{
			col:  col,
			name: name,
			key:  key,
			dt:   fd.DataType(),
			json: c.cs.FieldTypes[abs].IsContainer(),
		})
	}
	return s, nil
}

// ScanRow consumes one result row into a fresh pooled, schema-bound document.
// Values are parallel to the columns the plan was built with. NULL is absence
// (the slot is left untouched); scalar values land in their typed slot,
// JSON-fragment columns are decoded leniently at the field's coordinate. No ID,
// metadata defaults, or checksum are generated — read-back never invents
// values.
func (c *DocumentPool) ScanRow(ctx context.Context, s *RowScan, values []any) (*Document, error) {
	d, err := c.newDocument()
	if err != nil {
		return nil, err
	}
	if ctx != nil {
		d.ctx = ctx
	}
	if s == nil {
		return d, nil
	}
	for i := range s.fields {
		f := &s.fields[i]
		if f.col >= len(values) {
			continue
		}
		v := values[f.col]
		if v == nil {
			continue
		}
		if f.json {
			data, ok := fragmentBytes(v)
			if !ok {
				continue
			}
			if err := cjson.DecodeJSONField(c.cs, data, d.c, f.name, c.pool); err != nil {
				c.Release(d)
				return nil, err
			}
			continue
		}
		if err := setScalar(d.c, f.dt, f.key, v); err != nil {
			c.Release(d)
			return nil, err
		}
		if f.name == data.DocumentIDField {
			d.setID(asString(v))
		}
	}
	return d, nil
}

// rootFieldKey resolves a root-level field's single-step path to its
// DataContainerKey via the compiled schema's own address space — the schema is
// the sole source of address truth, mirroring the codec's computeLeafKey.
func rootFieldKey(cs *definition.CompiledSchema, fd definition.FieldDescriptor, fieldIdx uint8) (container.DataContainerKey, error) {
	path := definition.ResolvedPath{definition.NewResolvedStep(0, fieldIdx)}
	addr := cs.Address(path)
	if addr == 0 {
		return container.NewDataContainerKey(container.DataPoint(fd.DataPoint()), uint32(fd)), nil
	}
	dp, err := container.NewDataPoint(fd.DataType(), int32(addr))
	if err != nil {
		return 0, err
	}
	return container.NewDataContainerKey(dp, uint32(fd)), nil
}

// fragmentBytes extracts the serialized JSON fragment for a complex column.
func fragmentBytes(v any) ([]byte, bool) {
	switch t := v.(type) {
	case string:
		return []byte(t), true
	case []byte:
		return t, true
	}
	return nil, false
}

// setScalar writes a scalar SQLite value into the container slot for the
// field's compiled data type. SQLite returns INTEGER as int64 (booleans are
// 0/1), REAL as float64, TEXT as string, and BLOB as raw []byte; values are
// coerced so numeric enums stored as TEXT still bind to their slot.
func setScalar(doc *container.DataContainer, dt container.DataType, key container.DataContainerKey, v any) error {
	switch dt {
	case container.TypeInt:
		n, err := asInt64(v)
		if err != nil {
			return err
		}
		return doc.SetInt(key, n)
	case container.TypeFloat:
		f, err := asFloat64(v)
		if err != nil {
			return err
		}
		return doc.SetFloat(key, f)
	case container.TypeString:
		return doc.SetString(key, asString(v))
	case container.TypeBool:
		b, ok := boolFromValue(v)
		if !ok {
			return fmt.Errorf("document: expected boolean, got %T", v)
		}
		return doc.SetBool(key, b)
	case container.TypeBytes:
		b, ok := bytesFromValue(v)
		if !ok {
			return fmt.Errorf("document: expected bytes, got %T", v)
		}
		return doc.SetBytes(key, b)
	case container.TypeUnknown:
		return doc.SetUnknown(key, v)
	}
	return fmt.Errorf("document: unsupported scalar data type %d", dt)
}

func boolFromValue(v any) (bool, bool) {
	switch t := v.(type) {
	case bool:
		return t, true
	case int64:
		return t != 0, true
	case int:
		return t != 0, true
	case string:
		b, err := strconv.ParseBool(t)
		return b, err == nil
	}
	return false, false
}

func bytesFromValue(v any) ([]byte, bool) {
	switch t := v.(type) {
	case []byte:
		return t, true
	case string:
		return []byte(t), true
	}
	return nil, false
}
