package definition

import (
	"strconv"
	"sync"
)

// JSONBuilder builds JSON directly without intermediate allocations
type JSONBuilder struct {
	buf   []byte
	stack []byte // Stack of container types: '{' for object, '[' for array
	first []bool // Stack tracking if we need a comma before next element
}

var jsonBuilderPool = sync.Pool{
	New: func() any {
		return &JSONBuilder{
			buf:   make([]byte, 0, 8192), // Pre-allocate reasonable size
			stack: make([]byte, 0, 32),
			first: make([]bool, 0, 32),
		}
	},
}

func acquireJSONBuilder() *JSONBuilder {
	jb := jsonBuilderPool.Get().(*JSONBuilder)
	jb.buf = jb.buf[:0]
	jb.stack = jb.stack[:0]
	jb.first = jb.first[:0]
	return jb
}

func releaseJSONBuilder(jb *JSONBuilder) {
	jsonBuilderPool.Put(jb)
}

func (jb *JSONBuilder) Bytes() []byte {
	return jb.buf
}

func (jb *JSONBuilder) writeComma() {
	if len(jb.first) > 0 && !jb.first[len(jb.first)-1] {
		jb.buf = append(jb.buf, ',')
	}
	if len(jb.first) > 0 {
		jb.first[len(jb.first)-1] = false
	}
}

func (jb *JSONBuilder) startObject() {
	jb.buf = append(jb.buf, '{')
	jb.stack = append(jb.stack, '{')
	jb.first = append(jb.first, true)
}

func (jb *JSONBuilder) endObject() {
	jb.buf = append(jb.buf, '}')
	if len(jb.stack) > 0 {
		jb.stack = jb.stack[:len(jb.stack)-1]
		jb.first = jb.first[:len(jb.first)-1]
	}
	if len(jb.first) > 0 {
		jb.first[len(jb.first)-1] = false
	}
}

func (jb *JSONBuilder) startArray() {
	jb.buf = append(jb.buf, '[')
	jb.stack = append(jb.stack, '[')
	jb.first = append(jb.first, true)
}

func (jb *JSONBuilder) endArray() {
	jb.buf = append(jb.buf, ']')
	if len(jb.stack) > 0 {
		jb.stack = jb.stack[:len(jb.stack)-1]
		jb.first = jb.first[:len(jb.first)-1]
	}
	if len(jb.first) > 0 {
		jb.first[len(jb.first)-1] = false
	}
}

func (jb *JSONBuilder) writeKey(key string) {
	jb.writeComma()
	jb.buf = append(jb.buf, '"')
	jb.buf = append(jb.buf, key...)
	jb.buf = append(jb.buf, '"', ':')
}

func (jb *JSONBuilder) writeString(s string) {
	jb.buf = append(jb.buf, '"') // Start quote

	// Calculate required buffer size for escaped string
	escapeCount := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"', '\\', '\n', '\r', '\t':
			escapeCount++
		}
	}

	if escapeCount == 0 {
		// No escaping needed, append directly
		jb.buf = append(jb.buf, s...)
	} else {
		// Ensure enough capacity
		requiredLen := len(s) + escapeCount
		if cap(jb.buf)-len(jb.buf) < requiredLen {
			newBuf := make([]byte, len(jb.buf), len(jb.buf)+requiredLen+1) // +1 for closing quote
			copy(newBuf, jb.buf)
			jb.buf = newBuf
		}

		// Append escaped string
		for i := 0; i < len(s); i++ {
			c := s[i]
			switch c {
			case '"':
				jb.buf = append(jb.buf, '\\', '"')
			case '\\':
				jb.buf = append(jb.buf, '\\', '\\')
			case '\n':
				jb.buf = append(jb.buf, '\\', 'n')
			case '\r':
				jb.buf = append(jb.buf, '\\', 'r')
			case '\t':
				jb.buf = append(jb.buf, '\\', 't')
			default:
				jb.buf = append(jb.buf, c)
			}
		}
	}

	jb.buf = append(jb.buf, '"') // End quote
}

func (jb *JSONBuilder) writeBool(b bool) {
	if b {
		jb.buf = append(jb.buf, "true"...)
	} else {
		jb.buf = append(jb.buf, "false"...)
	}
}

func (jb *JSONBuilder) writeInt(i int64) {
	jb.buf = strconv.AppendInt(jb.buf, i, 10)
}

func (jb *JSONBuilder) writeFloat(f float64) {
	jb.buf = strconv.AppendFloat(jb.buf, f, 'g', -1, 64)
}

func (jb *JSONBuilder) writeNull() {
	jb.buf = append(jb.buf, "null"...)
}

// writeLiteralValue writes a LiteralValue in JSON format
func (jb *JSONBuilder) writeLiteralValue(lv LiteralValue) {
	if lv.IsZero() || lv.IsNull() {
		jb.writeNull()
		return
	}

	switch lv.kind {
	case LiteralTypeString:
		s, _ := LiteralValueAs[string](lv)
		jb.writeString(s)
	case LiteralTypeInteger:
		i, _ := LiteralValueAs[int64](lv)
		jb.writeInt(i)
	case LiteralTypeFloat:
		f, _ := LiteralValueAs[float64](lv)
		jb.writeFloat(f)
	case LiteralTypeBoolean:
		b, _ := LiteralValueAs[bool](lv)
		jb.writeBool(b)
	case LiteralTypeArray:
		arr, _ := LiteralValueAs[[]any](lv)
		jb.startArray()
		for _, elem := range arr {
			jb.writeComma()
			jb.writeLiteralValueFromAny(elem)
		}
		jb.endArray()
	case LiteralTypeObject:
		obj, _ := LiteralValueAs[map[string]any](lv)
		jb.startObject()
		for k, v := range obj {
			jb.writeKey(k)
			jb.writeLiteralValueFromAny(v)
		}
		jb.endObject()
	default:
		jb.writeNull()
	}
}

// writeLiteralValueFromAny writes any simple type value
func (jb *JSONBuilder) writeLiteralValueFromAny(v any) {
	if v == nil {
		jb.writeNull()
		return
	}

	switch val := v.(type) {
	case string:
		jb.writeString(val)
	case int64:
		jb.writeInt(val)
	case float64:
		jb.writeFloat(val)
	case bool:
		jb.writeBool(val)
	case []any:
		jb.startArray()
		for _, elem := range val {
			jb.writeComma()
			jb.writeLiteralValueFromAny(elem)
		}
		jb.endArray()
	case map[string]any:
		jb.startObject()
		for k, elem := range val {
			jb.writeKey(k)
			jb.writeLiteralValueFromAny(elem)
		}
		jb.endObject()
	default:
		jb.writeNull()
	}
}

// ToJSON converts a schema to JSON bytes using the walker
func (s *Schema) ToJSON() []byte {
	jb := acquireJSONBuilder()
	defer releaseJSONBuilder(jb)

	// Track depth to know when to close objects
	type contextInfo struct {
		nodeType NodeType
		depth    int
		inArray  bool
	}
	contextStack := make([]contextInfo, 0, 32)

	_, _ = s.Walk(jb, func(acc any, ctx *NodeContext) (any, error) {
		jb := acc.(*JSONBuilder)

		// Pop contexts that are shallower than current depth
		for len(contextStack) > 0 && contextStack[len(contextStack)-1].depth >= ctx.Depth {
			prev := contextStack[len(contextStack)-1]
			contextStack = contextStack[:len(contextStack)-1]

			switch prev.nodeType {
			case NodeTypeFieldsMap, NodeTypeSchemasMap, NodeTypeConstraintsMap,
				 NodeTypeIndexesMap, NodeTypeMetadataMap:
				jb.endObject()
			case NodeTypeField, NodeTypeNestedSchema, NodeTypeConstraint, NodeTypeIndex:
				jb.endObject()
			case NodeTypeConstraintGroup:
				// Close the rules array
				jb.endArray()
				jb.endObject()
			case NodeTypeIndexConditionGroup:
				// Close the conditions array
				jb.endArray()
				jb.endObject()
			}
		}

		switch ctx.Type {
		case NodeTypeSchema:
			schema := ctx.Value.(*Schema)
			jb.startObject()

			jb.writeKey("version")
			jb.writeString(schema.Version.String())

			jb.writeKey("name")
			jb.writeString(schema.Name)

			if schema.Description != "" {
				jb.writeKey("description")
				jb.writeString(schema.Description)
			}

		case NodeTypeFieldsMap:
			jb.writeKey("fields")
			jb.startObject()
			contextStack = append(contextStack, contextInfo{ctx.Type, ctx.Depth, false})

		case NodeTypeField:
			field := ctx.Value.(*Field)
			jb.writeKey(ctx.Key)
			jb.startObject()

			jb.writeKey("name")
			jb.writeString(string(field.Name))

			if field.Description != "" {
				jb.writeKey("description")
				jb.writeString(field.Description)
			}

			if field.Required {
				jb.writeKey("required")
				jb.writeBool(true)
			}

			if field.Deprecated {
				jb.writeKey("deprecated")
				jb.writeBool(true)
			}

			if field.Unique {
				jb.writeKey("unique")
				jb.writeBool(true)
			}

			if field.Type != 0 {
				jb.writeKey("type")
				jb.writeString(field.Type.String())
			}

			contextStack = append(contextStack, contextInfo{ctx.Type, ctx.Depth, false})

		case NodeTypeSchemasMap:
			jb.writeKey("schemas")
			jb.startObject()
			contextStack = append(contextStack, contextInfo{ctx.Type, ctx.Depth, false})

		case NodeTypeNestedSchema:
			ns := ctx.Value.(*NestedSchema)
			jb.writeKey(ctx.Key)
			jb.startObject()

			jb.writeKey("name")
			jb.writeString(ns.Name)

			if ns.Description != "" {
				jb.writeKey("description")
				jb.writeString(ns.Description)
			}

			if ns.Concrete {
				jb.writeKey("concrete")
				jb.writeBool(true)
			}

			if ns.Type != 0 {
				jb.writeKey("type")
				jb.writeString(ns.Type.String())
			}

			if len(ns.Values) > 0 {
				jb.writeKey("values")
				jb.startArray()
				for _, lv := range ns.Values {
					jb.writeComma()
					jb.writeLiteralValue(lv)
				}
				jb.endArray()
			}

			contextStack = append(contextStack, contextInfo{ctx.Type, ctx.Depth, false})

		case NodeTypeConstraintsMap:
			jb.writeKey("constraints")
			jb.startObject()
			contextStack = append(contextStack, contextInfo{ctx.Type, ctx.Depth, false})

		case NodeTypeConstraint:
			constraint := ctx.Value.(*Constraint)
			jb.writeKey(ctx.Key)
			jb.startObject()

			jb.writeKey("name")
			jb.writeString(constraint.Name)

			if constraint.Description != "" {
				jb.writeKey("description")
				jb.writeString(constraint.Description)
			}

			contextStack = append(contextStack, contextInfo{ctx.Type, ctx.Depth, false})

		case NodeTypeConstraintRule:
			rule := ctx.Value.(*ConstraintRule)

			jb.writeKey("predicate")
			jb.writeString(string(rule.Predicate))

			if len(rule.Fields) > 0 {
				jb.writeKey("fields")
				jb.startArray()
				for _, f := range rule.Fields {
					jb.writeString(string(f))
				}
				jb.endArray()
			}

			if !rule.Parameters.IsZero() && !rule.Parameters.IsNull() {
				jb.writeKey("parameters")
				jb.writeLiteralValue(rule.Parameters)
			}

		case NodeTypeConstraintGroup:
			group := ctx.Value.(*ConstraintGroup)

			jb.writeKey("operator")
			jb.writeString(group.Operator.String())

			jb.writeKey("rules")
			jb.startArray()

			contextStack = append(contextStack, contextInfo{ctx.Type, ctx.Depth, true})

			// Each rule in the group will be an object in the array
			for range group.Rules {
				jb.startObject()
			}

		case NodeTypeIndexesMap:
			jb.writeKey("indexes")
			jb.startObject()
			contextStack = append(contextStack, contextInfo{ctx.Type, ctx.Depth, false})

		case NodeTypeIndex:
			index := ctx.Value.(*Index)
			jb.writeKey(ctx.Key)
			jb.startObject()

			jb.writeKey("name")
			jb.writeString(index.Name)

			if index.Description != "" {
				jb.writeKey("description")
				jb.writeString(index.Description)
			}

			jb.writeKey("type")
			jb.writeString(index.Type.String())

			if len(index.Fields) > 0 {
				jb.writeKey("fields")
				jb.startArray()
				for _, f := range index.Fields {
					jb.writeString(string(f))
				}
				jb.endArray()
			}

			if index.Order != "" {
				jb.writeKey("order")
				jb.writeString(index.Order)
			}

			if index.Unique {
				jb.writeKey("unique")
				jb.writeBool(true)
			}

			contextStack = append(contextStack, contextInfo{ctx.Type, ctx.Depth, false})

		case NodeTypeIndexCondition:
			condition := ctx.Value.(*IndexCondition)
			jb.writeKey("condition")
			jb.startObject()

			jb.writeKey("field")
			jb.writeString(string(condition.Field))

			jb.writeKey("operator")
			jb.writeString(condition.Operator.String())

			if !condition.Value.IsZero() && !condition.Value.IsNull() {
				jb.writeKey("value")
				jb.writeLiteralValue(condition.Value)
			}

			jb.endObject()

		case NodeTypeIndexConditionGroup:
			group := ctx.Value.(*IndexConditionGroup)
			jb.writeKey("condition")
			jb.startObject()

			jb.writeKey("operator")
			jb.writeString(group.Operator.String())

			jb.writeKey("conditions")
			jb.startArray()

			contextStack = append(contextStack, contextInfo{ctx.Type, ctx.Depth, true})

		case NodeTypeFieldSchema:
			ref := ctx.Value.(FieldSchemaReference)
			jb.writeKey("schema")

			if ref.IsSingle() {
				sr, _ := FieldSchemaAs[SchemaReference](ref)
				jb.startObject()
				jb.writeKey("id")
				jb.writeString(string(sr.ID))

				// TODO: Handle indexes and constraints in schema references
				if len(sr.Indexes) > 0 {
					jb.writeKey("indexes")
					jb.startObject()
					jb.endObject()
				}
				if len(sr.Constraints) > 0 {
					jb.writeKey("constraints")
					jb.startObject()
					jb.endObject()
				}

				jb.endObject()
			} else if ref.IsMultiple() {
				refs, _ := FieldSchemaAs[[]SchemaReference](ref)
				jb.startArray()
				for _, sr := range refs {
					jb.writeComma()
					jb.startObject()
					jb.writeKey("id")
					jb.writeString(string(sr.ID))
					jb.endObject()
				}
				jb.endArray()
			}

		case NodeTypeValuesArray:
			values := ctx.Value.([]LiteralValue)
			jb.writeKey(ctx.Key)
			jb.startArray()
			for _, lv := range values {
				jb.writeComma()
				jb.writeLiteralValue(lv)
			}
			jb.endArray()

		case NodeTypeConstraintParameters:
			params := ctx.Value.(LiteralValue)
			jb.writeKey(ctx.Key)
			jb.writeLiteralValue(params)

		case NodeTypeConstraintFields:
			fields := ctx.Value.([]FieldName)
			jb.writeKey(ctx.Key)
			jb.startArray()
			for _, f := range fields {
				jb.writeComma()
				jb.writeString(string(f))
			}
			jb.endArray()

		case NodeTypeIndexFields:
			fields := ctx.Value.([]FieldId)
			jb.writeKey(ctx.Key)
			jb.startArray()
			for _, f := range fields {
				jb.writeComma()
				jb.writeString(string(f))
			}
			jb.endArray()

		case NodeTypeFieldDefault:
			def := ctx.Value.(LiteralValue)
			jb.writeKey(ctx.Key)
			jb.writeLiteralValue(def)

		case NodeTypeLiteralValue:
			lv := ctx.Value.(LiteralValue)
			if !lv.IsZero() && !lv.IsNull() {
				jb.writeKey(ctx.Key)
				jb.writeLiteralValue(lv)
			}


		case NodeTypeMetadataMap:
			metadata := ctx.Value.(map[string]any)
			jb.writeKey("metadata")
			jb.startObject()
			for k, v := range metadata {
				jb.writeKey(k)
				jb.writeLiteralValueFromAny(v)
			}
			jb.endObject()
		}

		return jb, nil
	})

	// Close any remaining open contexts
	for len(contextStack) > 0 {
		prev := contextStack[len(contextStack)-1]
		contextStack = contextStack[:len(contextStack)-1]

		switch prev.nodeType {
		case NodeTypeFieldsMap, NodeTypeSchemasMap, NodeTypeConstraintsMap,
			 NodeTypeIndexesMap, NodeTypeMetadataMap:
			jb.endObject()
		case NodeTypeField, NodeTypeNestedSchema, NodeTypeConstraint, NodeTypeIndex:
			jb.endObject()
		case NodeTypeConstraintGroup:
			jb.endArray()
			jb.endObject()
		case NodeTypeIndexConditionGroup:
			jb.endArray()
			jb.endObject()
		}
	}

	// Close root object
	jb.endObject()

	// Make a copy since we're releasing the builder back to the pool
	result := make([]byte, len(jb.buf))
	copy(result, jb.buf)
	return result
}
