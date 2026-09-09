package typescript

import (
	"fmt"
	"sort"
	"strings"

	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

type TSGenerator struct {
	schema *definition.Schema
}

func NewTSGenerator(schema *definition.Schema) *TSGenerator {
	return &TSGenerator{schema: schema}
}

func (g *TSGenerator) Generate() string {
	lines := []string{"// Auto-generated from anansi schema — do not edit\n"}

	for _, id := range g.sortedSchemaIDs() {
		if s := g.nestedTypeDecl(g.schema.Schemas[id]); s != "" {
			lines = append(lines, s)
		}
	}

	if len(g.schema.Fields) > 0 {
		lines = append(lines, g.interfaceDecl(g.schema.Name, g.schema.Fields, g.schema.Description))
	}

	return strings.Join(lines, "\n\n") + "\n"
}

// GenerateCombined generates a single TypeScript file containing types for all
// given schemas. Nested schemas are merged into one registry so cross-schema
// references resolve correctly.
func GenerateCombined(schemas []*definition.Schema) string {
	merged := &definition.Schema{
		Schemas: make(map[definition.SchemaId]definition.NestedSchema),
	}
	for _, s := range schemas {
		for id, ns := range s.Schemas {
			merged.Schemas[id] = ns
		}
	}

	gen := &TSGenerator{schema: merged}
	parts := []string{"// Auto-generated from anansi schemas — do not edit\n"}

	// Generate all nested schema types from the merged registry
	for _, id := range gen.sortedSchemaIDs() {
		if s := gen.nestedTypeDecl(merged.Schemas[id]); s != "" {
			parts = append(parts, s)
		}
	}

	// Generate each top-level schema's interface
	for _, s := range schemas {
		if len(s.Fields) > 0 {
			g := &TSGenerator{schema: &definition.Schema{
				BaseSchema: definition.BaseSchema{
					Name:        s.Name,
					Description: s.Description,
					Fields:      s.Fields,
				},
				Schemas: merged.Schemas,
			}}
			parts = append(parts, g.interfaceDecl(s.Name, s.Fields, s.Description))
		}
	}

	return strings.Join(parts, "\n\n") + "\n"
}

func (g *TSGenerator) sortedSchemaIDs() []definition.SchemaId {
	ids := make([]definition.SchemaId, 0, len(g.schema.Schemas))
	for id := range g.schema.Schemas {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return g.schema.Schemas[ids[i]].Name < g.schema.Schemas[ids[j]].Name
	})
	return ids
}

func (g *TSGenerator) nestedTypeDecl(ns definition.NestedSchema) string {
	name := ns.Name
	if name == "" {
		return ""
	}

	if len(ns.Values) > 0 {
		d := g.docComment(ns.Description)
		if d != "" {
			return d + "\nexport type " + name + " = " + g.enumValues(ns.Values) + ";"
		}
		return fmt.Sprintf("export type %s = %s;", name, g.enumValues(ns.Values))
	}
	if len(ns.Fields) > 0 {
		return g.interfaceDecl(name, ns.Fields, ns.Description)
	}
	if ns.Type != 0 {
		d := g.docComment(ns.Description)
		if d != "" {
			return d + "\nexport type " + name + " = " + g.resolveFieldProperties(ns.FieldProperties) + ";"
		}
		return fmt.Sprintf("export type %s = %s;", name, g.resolveFieldProperties(ns.FieldProperties))
	}
	return ""
}

func (g *TSGenerator) enumValues(values []definition.LiteralValue) string {
	parts := make([]string, 0, len(values))
	for _, v := range values {
		parts = append(parts, g.formatLiteral(v))
	}
	return strings.Join(parts, " | ")
}

func (g *TSGenerator) docComment(desc string) string {
	if desc == "" {
		return ""
	}
	return fmt.Sprintf("/** %s */", desc)
}

func (g *TSGenerator) fieldDocComment(desc string) string {
	if desc == "" {
		return ""
	}
	return fmt.Sprintf("  /** %s */\n", desc)
}

func (g *TSGenerator) interfaceDecl(name string, fields map[definition.FieldId]definition.Field, desc ...string) string {
	sorted := g.sortedFields(fields)
	var buf strings.Builder

	if len(desc) > 0 && desc[0] != "" {
		buf.WriteString(g.docComment(desc[0]))
		buf.WriteString("\n")
	}
	fmt.Fprintf(&buf, "export interface %s {\n", name)
	for _, f := range sorted {
		if f.Description != "" {
			buf.WriteString(g.fieldDocComment(f.Description))
		}
		tsType := g.fieldType(f)
		if f.Required {
			fmt.Fprintf(&buf, "  %s: %s;\n", f.Name, tsType)
		} else {
			fmt.Fprintf(&buf, "  %s?: %s;\n", f.Name, tsType)
		}
	}
	buf.WriteString("}")
	return buf.String()
}

func (g *TSGenerator) sortedFields(fields map[definition.FieldId]definition.Field) []definition.Field {
	names := make([]string, 0, len(fields))
	byName := make(map[string]definition.Field, len(fields))
	for _, f := range fields {
		n := string(f.Name)
		names = append(names, n)
		byName[n] = f
	}
	sort.Strings(names)
	out := make([]definition.Field, len(names))
	for i, n := range names {
		out[i] = byName[n]
	}
	return out
}

func (g *TSGenerator) fieldType(f definition.Field) string {
	switch f.Type {
	case definition.FieldTypeString:
		return "string"
	case definition.FieldTypeNumber, definition.FieldTypeInteger, definition.FieldTypeDecimal:
		return "number"
	case definition.FieldTypeBoolean:
		return "boolean"
	case definition.FieldTypeBytes:
		return "string"
	case definition.FieldTypeUnknown:
		return "unknown"
	case definition.FieldTypeGeometry:
		return "unknown"
	default:
		return g.complexFieldType(f)
	}
}

func (g *TSGenerator) complexFieldType(f definition.Field) string {
	if f.Schema.IsZero() {
		return "unknown"
	}

	switch f.Type {
	case definition.FieldTypeEnum:
		return g.resolveSingleRef(f.Schema)
	case definition.FieldTypeObject:
		return g.resolveSingleRef(f.Schema)
	case definition.FieldTypeArray:
		item := g.resolveSingleRef(f.Schema)
		return item + "[]"
	case definition.FieldTypeRecord:
		item := g.resolveSingleRef(f.Schema)
		return fmt.Sprintf("Record<string, %s>", item)
	case definition.FieldTypeUnion, definition.FieldTypeComposite:
		types := g.resolveMultiRefs(f.Schema)
		sep := " | "
		if f.Type == definition.FieldTypeComposite {
			sep = " & "
		}
		return joinUnique(types, sep)
	default:
		return "unknown"
	}
}

func (g *TSGenerator) resolveFieldProperties(fp definition.FieldProperties) string {
	switch fp.Type {
	case definition.FieldTypeString:
		return "string"
	case definition.FieldTypeNumber, definition.FieldTypeInteger, definition.FieldTypeDecimal:
		return "number"
	case definition.FieldTypeBoolean:
		return "boolean"
	case definition.FieldTypeBytes:
		return "string"
	case definition.FieldTypeUnknown:
		return "unknown"
	case definition.FieldTypeArray:
		item := g.resolveSingleRef(fp.Schema)
		return item + "[]"
	case definition.FieldTypeRecord:
		item := g.resolveSingleRef(fp.Schema)
		return fmt.Sprintf("Record<string, %s>", item)
	case definition.FieldTypeUnion:
		return joinUnique(g.resolveMultiRefs(fp.Schema), " | ")
	case definition.FieldTypeComposite:
		return joinUnique(g.resolveMultiRefs(fp.Schema), " & ")
	case definition.FieldTypeObject:
		return g.resolveSingleRef(fp.Schema)
	case definition.FieldTypeEnum:
		return g.resolveSingleRef(fp.Schema)
	default:
		return "unknown"
	}
}

func (g *TSGenerator) resolveSingleRef(fr definition.FieldSchemaReference) string {
	if fr.IsZero() {
		return "unknown"
	}
	if !fr.IsSingle() {
		return "unknown"
	}
	ref, err := definition.FieldSchemaAs[definition.SchemaReference](fr)
	if err != nil {
		return "unknown"
	}
	if ref.IsInline() {
		return g.inlineType(ref.Type)
	}
	if ns, ok := g.schema.Schemas[ref.ID]; ok {
		return ns.Name
	}
	return "unknown"
}

func (g *TSGenerator) resolveMultiRefs(fr definition.FieldSchemaReference) []string {
	if fr.IsZero() || !fr.IsMultiple() {
		return nil
	}
	refs, err := definition.FieldSchemaAs[[]definition.SchemaReference](fr)
	if err != nil {
		return nil
	}
	out := make([]string, len(refs))
	for i, ref := range refs {
		if ref.IsInline() {
			out[i] = g.inlineType(ref.Type)
		} else if ns, ok := g.schema.Schemas[ref.ID]; ok {
			out[i] = ns.Name
		} else {
			out[i] = "unknown"
		}
	}
	return out
}

func (g *TSGenerator) inlineType(t definition.FieldType) string {
	switch t {
	case definition.FieldTypeString:
		return "string"
	case definition.FieldTypeNumber, definition.FieldTypeInteger, definition.FieldTypeDecimal:
		return "number"
	case definition.FieldTypeBoolean:
		return "boolean"
	case definition.FieldTypeBytes:
		return "string"
	default:
		return "unknown"
	}
}

func (g *TSGenerator) formatLiteral(lv definition.LiteralValue) string {
	typ, err := lv.Type()
	if err != nil {
		return "unknown"
	}
	switch typ {
	case definition.LiteralTypeString:
		return fmt.Sprintf("%q", lv.Value().(string))
	case definition.LiteralTypeInteger:
		return fmt.Sprintf("%d", lv.Value().(int64))
	case definition.LiteralTypeFloat:
		return fmt.Sprintf("%v", lv.Value().(float64))
	case definition.LiteralTypeBoolean:
		return fmt.Sprintf("%t", lv.Value().(bool))
	default:
		return "unknown"
	}
}

func joinUnique(parts []string, sep string) string {
	if len(parts) == 0 {
		return "unknown"
	}
	seen := make(map[string]bool, len(parts))
	filtered := make([]string, 0, len(parts))
	for _, p := range parts {
		if !seen[p] {
			seen[p] = true
			filtered = append(filtered, p)
		}
	}
	return strings.Join(filtered, sep)
}
