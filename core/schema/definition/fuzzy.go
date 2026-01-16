package definition

import (
	"math/rand"
)


type FuzzyConfig struct {
	MaxDepth            int     // hard recursion limit
	ContinueProbability float64 // 0.0–1.0 chance to recurse deeper at each level
	ErrorRate           float64 // 0.0–1.0 overall chance to inject an error per field/nested
}

const (
	DefaultMaxDepth            = 12
	DefaultContinueProbability = 0.30
	DefaultErrorRate           = 0.10
)

// GenerateFuzzyData generates a map[string]any matching the schema (mostly valid, tunable invalidity)
func GenerateFuzzyData(s *Schema, cfg FuzzyConfig) map[string]any {
	if cfg.MaxDepth <= 0 {
		cfg.MaxDepth = DefaultMaxDepth
	}
	if cfg.ContinueProbability < 0 || cfg.ContinueProbability > 1 {
		cfg.ContinueProbability = DefaultContinueProbability
	}
	if cfg.ErrorRate < 0 || cfg.ErrorRate > 1 {
		cfg.ErrorRate = DefaultErrorRate
	}

	state := &genState{
		schema:              s,
		cfg:                 cfg,
		visited:             make(map[SchemaId]int),
		path:                []string{},
		depth:               0,
	}
	data := make(map[string]any)
	for _, field := range s.Fields {
		key := string(field.Name)
		state.path = []string{key}
		val := state.generateField(field, 0)
		if val != nil || field.Required { // always include required, even if nil (invalid)
			data[key] = val
		}
	}
	return data
}

type genState struct {
	schema  *Schema
	cfg     FuzzyConfig
	visited map[SchemaId]int
	path    []string
	depth   int
}

func (st *genState) generateField(f Field, depth int) any {
	st.depth = depth
	st.path = append(st.path, string(f.Name))
	defer func() { st.path = st.path[:len(st.path)-1] }()

	injectError := rand.Float64() < st.cfg.ErrorRate

	if injectError && f.Required {
		// Popular negative test: missing required field
		if rand.Float64() < 0.4 { // tunable bias
			return nil // omitted → violation
		}
	}

	if !f.Default.IsZero() && !f.Default.IsNull() && !injectError {
		return f.Default.Value()
	}

	if injectError {
		return st.injectErrorForField(f)
	}

	// Normal valid generation
	switch f.Type {
	case FieldTypeString:
		return randomString(8 + rand.Intn(12))
	case FieldTypeNumber, FieldTypeDecimal:
		return rand.Float64() * 9999
	case FieldTypeInteger:
		return rand.Intn(10000)
	case FieldTypeBoolean:
		return rand.Float64() < 0.5
	case FieldTypeArray, FieldTypeSet:
		if f.Schema.IsZero() {
			return []any{}
		}
		ref, _ := FieldSchemaAs[SchemaReference](f.Schema)
		ns, ok := st.schema.Schemas[ref.ID]
		if !ok {
			return []any{}
		}
		count := 1 + rand.Intn(4) // small array
		arr := make([]any, count)
		for i := 0; i < count; i++ {
			arr[i] = st.generateNested(ns, depth+1)
		}
		return arr
	case FieldTypeObject, FieldTypeRecord:
		if f.Schema.IsZero() {
			return map[string]any{}
		}
		ref, _ := FieldSchemaAs[SchemaReference](f.Schema)
		ns, ok := st.schema.Schemas[ref.ID]
		if !ok {
			return map[string]any{}
		}
		return st.generateNested(ns, depth+1)
	case FieldTypeEnum:
		if f.Schema.IsZero() {
			return randomString(6)
		}
		ref, _ := FieldSchemaAs[SchemaReference](f.Schema)
		ns, ok := st.schema.Schemas[ref.ID]
		if !ok || len(ns.Values) == 0 {
			return randomString(6)
		}
		return ns.Values[rand.Intn(len(ns.Values))].Value()
	// ... handle union/composite/geometry similarly
	default:
		return nil
	}
}

func (st *genState) generateNested(ns NestedSchema, depth int) any {
	if depth >= st.cfg.MaxDepth {
		return st.leafFallback(ns)
	}

	shouldRecurse := rand.Float64() < st.cfg.ContinueProbability
	if !shouldRecurse {
		return st.leafFallback(ns)
	}

	// Normal nested object generation (similar to before)
	if len(ns.Fields) > 0 {
		obj := make(map[string]any)
		for _, f := range ns.Fields {
			key := string(f.Name)
			obj[key] = st.generateField(f, depth+1)
		}
		// Occasionally add extra unknown field (error case, but outside per-field decision)
		if rand.Float64() < st.cfg.ErrorRate*0.5 {
			obj[randomString(6)] = randomString(10)
		}
		return obj
	}

	// Enum / primitive fallback / etc.
	// ...
	return st.leafFallback(ns)
}

func (st *genState) leafFallback(ns NestedSchema) any {
	switch ns.Type {
	case FieldTypeObject, FieldTypeRecord:
		return map[string]any{"_leaf": true}
	case FieldTypeArray, FieldTypeSet:
		return []any{}
	case FieldTypeString:
		return randomString(5)
	default:
		return nil
	}
}

func (st *genState) injectErrorForField(f Field) any {
	switch rand.Intn(5) { // simple switch between error kinds
	case 0: // wrong type
		switch f.Type {
		case FieldTypeString:
			return rand.Intn(9999)
		case FieldTypeInteger, FieldTypeNumber:
			return randomString(8)
		case FieldTypeBoolean:
			return randomString(5)
		default:
			return nil
		}
	case 1: // invalid enum
		if f.Type == FieldTypeEnum || (f.Schema.IsZero() == false) {
			return "INVALID_" + randomString(8)
		}
		return randomString(12)
	case 2: // empty / too small array
		if f.Type == FieldTypeArray || f.Type == FieldTypeSet {
			return []any{}
		}
		return nil
	case 3: // negative / zero for likely positive numeric
		if f.Type == FieldTypeInteger || f.Type == FieldTypeNumber {
			return -rand.Intn(1000) - 1
		}
		return nil
	default:
		return nil // null / missing handled outside
	}
}

func randomString(n int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}
