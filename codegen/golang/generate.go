package golang

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// ============================================================================
// Configuration types
// ============================================================================

// TagRule defines how a single tag (e.g. `json:"name,omitempty"`) is constructed.
type TagRule struct {
	Key       string // e.g., "json", "anansi", "db"
	Property  string // Property to extract from Field, e.g., "name", "type" (currently only "name" is supported)
	OmitEmpty bool   // Append ",omitempty" when the field is optional (!Required)
}

// TagConfig holds tag rules for field generation.
type TagConfig []TagRule

// TagConfigFromMap creates a TagConfig from a simple map like {"json": "name", "anansi": "name"}.
// Optionally specify keys that should include ",omitempty" for optional fields.
func TagConfigFromMap(m map[string]string, omitEmptyKeys ...string) TagConfig {
	omitMap := make(map[string]bool, len(omitEmptyKeys))
	for _, k := range omitEmptyKeys {
		omitMap[k] = true
	}

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	config := make(TagConfig, 0, len(keys))
	for _, k := range keys {
		config = append(config, TagRule{
			Key:       k,
			Property:  m[k],
			OmitEmpty: omitMap[k],
		})
	}
	return config
}

// DefaultTagConfig returns the default configuration used when no config is provided.
func DefaultTagConfig() TagConfig {
	return TagConfigFromMap(
		map[string]string{
			"json":   "name",
			"anansi": "name",
		},
		"json", // only json gets ",omitempty"
	)
}

// NameRule maps a regex pattern in a schema name to a Go type prefix.
// For example, Pattern=`^_(.+)_$` with Prefix=`System` converts `_user_`
// to `SystemUser` (the matched portion is stripped, then Prefix+camelCase).
type NameRule struct {
	Pattern *regexp.Regexp
	Prefix  string
}

// MustCompileRule is a convenience for compiling a regex at init time.
// It panics on invalid regex.
func MustCompileRule(pattern, prefix string) NameRule {
	return NameRule{Pattern: regexp.MustCompile(pattern), Prefix: prefix}
}

type GoGenerator struct {
	tagConfig   TagConfig
	scoped      bool
	nameRules   []NameRule
	packageName string
}

// ============================================================================
// Generator entry point
// ============================================================================

type GeneratorConfig struct {
	TagConfig      TagConfig
	ScopedPackages bool
	NameRules      []NameRule
	PackageName    string
}

func NewGoGenerator(config *GeneratorConfig) *GoGenerator {
	if config == nil {
		config = &GeneratorConfig{
			TagConfig: DefaultTagConfig(),
			NameRules: make([]NameRule, 0),
		}
	}
	return &GoGenerator{tagConfig: config.TagConfig, scoped: config.ScopedPackages, nameRules: config.NameRules, packageName: config.PackageName}
}

// goTypeName converts a schema name to a Go type name by:
//  1. Applying the first matching NameRule (strips matched portion, applies prefix)
//  2. Converting to CamelCase
func (g *GoGenerator) goTypeName(name string) string {
	for _, rule := range g.nameRules {
		if m := rule.Pattern.FindStringSubmatch(name); len(m) > 1 {
			// Join prefix and captured inner name with _ so camelCase
			// treats them as separate title-cased words.
			return toCamelCase(rule.Prefix + "_" + m[1])
		}
	}
	return toCamelCase(name)
}

func (g *GoGenerator) Generate(schemaBytes []byte) (string, error) {
	tagConfig := g.tagConfig

	var data map[string]any
	if err := json.Unmarshal(schemaBytes, &data); err != nil {
		return "", fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Extract root information
	rootName := getString(data, "name")
	if rootName == "" {
		return "", fmt.Errorf("schema missing 'name'")
	}
	rootFieldsRaw, ok := data["fields"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("root 'fields' missing or invalid")
	}
	rootFields := parseFields(rootFieldsRaw)

	// Extract nested schemas
	schemasRaw, ok := data["schemas"].(map[string]any)
	if !ok {
		schemasRaw = make(map[string]any)
	}
	schemaInfos := make(map[string]*SchemaInfo)
	for id, raw := range schemasRaw {
		info, err := parseSchemaInfo(id, raw)
		if err != nil {
			return "", fmt.Errorf("failed to parse schema %s: %w", id, err)
		}
		schemaInfos[id] = info
	}

	// Build type name registry: ID -> Go type name
	typeNames := make(map[string]string)
	for id, info := range schemaInfos {
		typeNames[id] = g.goTypeName(info.Name)
	}
	rootTypeName := g.goTypeName(rootName)

	// Maps to hold generated definitions
	structs := make(map[string][]StructField)  // struct name -> fields
	typeAliases := make(map[string]string)     // name -> underlying type expr
	enumDefs := make(map[string]EnumDef)       // name -> enum definition
	inlineEnumNames := make(map[string]string) // key -> generated type name for inline enum

	// Helper to generate inline enum types (unique name per parent+field)
	genInlineEnumName := func(parent, field string) string {
		key := parent + "_" + field + "_enum"
		if name, ok := inlineEnumNames[key]; ok {
			return name
		}
		name := toCamelCase(parent + "_" + field + "_enum")
		inlineEnumNames[key] = name
		return name
	}

	// Process all nested schemas
	for id, info := range schemaInfos {
		if info.IsSchema {
			fields, err := generateFields(info.Fields, typeNames, schemaInfos, structs, typeAliases, enumDefs, inlineEnumNames, genInlineEnumName, info.Name, tagConfig)
			if err != nil {
				return "", fmt.Errorf("failed to generate fields for schema %s: %w", id, err)
			}
			structs[typeNames[id]] = fields
		} else if info.IsEnum {
			underlying := mapPrimitiveTypeToGo(info.Type)
			if underlying == "" {
				return "", fmt.Errorf("enum %s has unsupported underlying type %q", id, info.Type)
			}
			enumDefs[typeNames[id]] = EnumDef{
				Underlying: underlying,
				Values:     info.Values,
			}
		} else {
			// Type mode (non-enum)
			switch info.Type {
			case "composite":
				refs, ok := info.SchemaRef.([]any)
				if !ok || len(refs) == 0 {
					return "", fmt.Errorf("composite schema %s missing or invalid schema refs", id)
				}
				var embeds []StructField
				for _, r := range refs {
					rmap, ok := r.(map[string]any)
					if !ok {
						return "", fmt.Errorf("composite ref is not a map")
					}
					refType, err := resolveSchemaReference(rmap, typeNames, schemaInfos)
					if err != nil {
						return "", err
					}
					embeds = append(embeds, StructField{
						Name: "", // empty name → embedded field
						Type: refType,
						Tags: "",
					})
				}
				structs[typeNames[id]] = embeds

			case "union":
				refs, ok := info.SchemaRef.([]any)
				if !ok || len(refs) == 0 {
					return "", fmt.Errorf("union schema %s missing or invalid schema refs", id)
				}
				var fields []StructField
				for _, r := range refs {
					rmap, ok := r.(map[string]any)
					if !ok {
						return "", fmt.Errorf("union ref is not a map")
					}
					refType, err := resolveSchemaReference(rmap, typeNames, schemaInfos)
					if err != nil {
						return "", err
					}
					fields = append(fields, StructField{
						Name: refType, // use the type name as field name
						Type: "*" + refType,
						Tags: fmt.Sprintf("`json:\"%s,omitempty\"`", toSnakeCase(refType)),
					})
				}
				structs[typeNames[id]] = fields

			default:
				// array, record, or primitive
				goType, err := resolveTypeMode(info.Type, info.SchemaRef, typeNames, schemaInfos, structs, typeAliases, enumDefs, inlineEnumNames, genInlineEnumName, info.Name)
				if err != nil {
					return "", fmt.Errorf("failed to resolve type mode for schema %s: %w", id, err)
				}
				typeAliases[typeNames[id]] = goType
			}
		}
	}

	// Process root schema
	rootFieldsGo, err := generateFields(rootFields, typeNames, schemaInfos, structs, typeAliases, enumDefs, inlineEnumNames, genInlineEnumName, rootName, tagConfig)
	if err != nil {
		return "", fmt.Errorf("failed to generate root fields: %w", err)
	}
	structs[rootTypeName] = rootFieldsGo

	// Build output
	var sb strings.Builder

	_, hasRootStruct := structs[rootTypeName]

	// Determine if any struct field uses decimal.Decimal
	needsDecimal := false
	for _, fields := range structs {
		for _, f := range fields {
			if f.Type == "decimal.Decimal" {
				needsDecimal = true
				break
			}
		}
		if needsDecimal {
			break
		}
	}

	// File header
	sb.WriteString("// Code generated by anansi. DO NOT EDIT.\n")
	sb.WriteString("//\n")
	sb.WriteString("// This file is auto-generated from the schema definition.\n")
	sb.WriteString("// Always run codegen alongside database migrations so that the\n")
	sb.WriteString("// generated models stay in sync with the schema on file.\n")
	sb.WriteString("//\n")
	sb.WriteString("// Extend model functionality in separate Go files (e.g. ")
	sb.WriteString(toSnakeCase(rootTypeName) + "_utils.go).\n")
	sb.WriteString("// This file is overwritten on each codegen run — never edit it directly.\n")
	if g.packageName != "" {
		sb.WriteString("//\n")
		sb.WriteString(fmt.Sprintf("// Package %s provides the %s model.\n", g.packageName, rootTypeName))
		sb.WriteString(fmt.Sprintf("package %s\n", g.packageName))
		sb.WriteString("\n")
	} else {
		sb.WriteString("\n")
	}

	// Imports
	sb.WriteString("import (\n")
	sb.WriteString("\t\"github.com/asaidimu/go-anansi/v8/core/data\"\n")
	if needsDecimal {
		sb.WriteString("\t\"github.com/asaidimu/go-anansi/v8/core/types/decimal\"\n")
	}
	if hasRootStruct {
		sb.WriteString("\t\"context\"\n")
		sb.WriteString("\t\"sync\"\n")
		sb.WriteString("\t\"github.com/asaidimu/go-anansi/v8/core/common\"\n")
		sb.WriteString("\t\"github.com/asaidimu/go-anansi/v8/core/persistence/base\"\n")
		sb.WriteString("\t\"github.com/asaidimu/go-anansi/v8/core/persistence/collection\"\n")
		sb.WriteString("\t\"go.uber.org/zap\"\n")
	}
	sb.WriteString(")\n\n")

	// Output type aliases (non-enum)
	for name, underlying := range typeAliases {
		fmt.Fprintf(&sb, "type %s = %s\n", name, underlying)
	}
	if len(typeAliases) > 0 {
		sb.WriteString("\n")
	}

	// Output enum types
	for name, enum := range enumDefs {
		fmt.Fprintf(&sb, "type %s %s\n\n", name, enum.Underlying)
		sb.WriteString("const (\n")
		for _, v := range enum.Values {
			valStr := formatLiteral(v)
			constName := toCamelCase(fmt.Sprintf("%s_%v", name, v))
			fmt.Fprintf(&sb, "    %s %s = %s\n", constName, name, valStr)
		}
		sb.WriteString(")\n\n")
	}

	// Output nested structs (everything except root, which gets the model treatment)
	for name, fields := range structs {
		if name == rootTypeName {
			continue
		}
		sort.Slice(fields, func(i, j int) bool {
			return typeSize(fields[i].Type) > typeSize(fields[j].Type)
		})
		sb.WriteString(fmt.Sprintf("type %s struct {\n", name))
		for _, f := range fields {
			sb.WriteString(fmt.Sprintf("    %s %s `%s`\n", f.Name, f.Type, strings.Trim(f.Tags, "`")))
		}
		sb.WriteString("}\n\n")
	}

	// Output root struct with DocumentModel embed
	if hasRootStruct {
		fields := structs[rootTypeName]
		sort.Slice(fields, func(i, j int) bool {
			return typeSize(fields[i].Type) > typeSize(fields[j].Type)
		})
		sb.WriteString(fmt.Sprintf("type %s struct {\n", rootTypeName))
		sb.WriteString("    data.DocumentModel\n")
		for _, f := range fields {
			sb.WriteString(fmt.Sprintf("    %s %s `%s`\n", f.Name, f.Type, strings.Trim(f.Tags, "`")))
		}
		sb.WriteString("}\n\n")

		// Constructor
		if g.scoped {
			sb.WriteString(fmt.Sprintf("// New creates and initializes a new %s\n", rootTypeName))
			sb.WriteString(fmt.Sprintf("func New(model %s) *%s {\n", rootTypeName, rootTypeName))
			sb.WriteString(fmt.Sprintf("    return data.New(&model)\n"))
			sb.WriteString("}\n\n")
		} else {
			sb.WriteString(fmt.Sprintf("// New%s creates and initializes a new %s\n", rootTypeName, rootTypeName))
			sb.WriteString(fmt.Sprintf("func New%s(model %s) *%s {\n", rootTypeName, rootTypeName, rootTypeName))
			sb.WriteString(fmt.Sprintf("    return data.New(&model)\n"))
			sb.WriteString("}\n\n")
		}

		// Typed collection wrapper
		collectionName := rootTypeName + "s"
		sb.WriteString(fmt.Sprintf("// %s is a type-safe collection for %s\n", collectionName, rootTypeName))
		sb.WriteString(fmt.Sprintf("type %s struct {\n", collectionName))
		sb.WriteString(fmt.Sprintf("    base.ModelCollection[%s, *%s]\n", rootTypeName, rootTypeName))
		sb.WriteString("}\n\n")

		// Singleton model access — unexported vars, exported functions.
		// In scoped mode (one model per package) the function prefix is
		// empty — InitModel / Model. In non-scoped mode the collection
		// name is used — InitUsersModel / UsersModel.
		var fnPrefix string
		var constName string
		var varName string
		if g.scoped {
			constName = "CollectionName"
			varName = "model"
		} else {
			fnPrefix = collectionName
			constName = collectionName + "CollectionName"
			varName = lowerFirst(collectionName) + "Model"
		}
		sb.WriteString(fmt.Sprintf("const %s = %q\n\n", constName, rootName))
		sb.WriteString(fmt.Sprintf("var (\n"))
		sb.WriteString(fmt.Sprintf("    %sMu sync.Mutex\n", varName))
		sb.WriteString(fmt.Sprintf("    %s   *%s\n", varName, collectionName))
		sb.WriteString(fmt.Sprintf(")\n\n"))

		sb.WriteString(fmt.Sprintf("// Init%sModel must be called once at startup to configure and\n", fnPrefix))
		sb.WriteString(fmt.Sprintf("// construct the %s model. Idempotent — subsequent calls return\n", rootTypeName))
		sb.WriteString(fmt.Sprintf("// the existing instance. Retry-safe: if the first call fails, the\n"))
		sb.WriteString(fmt.Sprintf("// caller can fix the underlying issue and call again.\n"))
		sb.WriteString(fmt.Sprintf("func Init%sModel(p base.Persistence, logger *zap.Logger, opts ...collection.ModelCollectionOptions[%s, *%s]) (*%s, error) {\n", fnPrefix, rootTypeName, rootTypeName, collectionName))
		sb.WriteString(fmt.Sprintf("    %sMu.Lock()\n", varName))
		sb.WriteString(fmt.Sprintf("    defer %sMu.Unlock()\n", varName))
		sb.WriteString(fmt.Sprintf("    if %s != nil {\n", varName))
		sb.WriteString(fmt.Sprintf("        return %s, nil\n", varName))
		sb.WriteString(fmt.Sprintf("    }\n"))
		sb.WriteString(fmt.Sprintf("    raw, err := p.Collection(context.Background(), %s)\n", constName))
		sb.WriteString(fmt.Sprintf("    if err != nil {\n"))
		sb.WriteString(fmt.Sprintf("        return nil, common.SystemErrorFrom(err, \"ERR_MODEL_INIT_FAILED\").\n"))
		sb.WriteString(fmt.Sprintf("            WithOperation(\"Init%sModel\").\n", fnPrefix))
		sb.WriteString(fmt.Sprintf("            WithPath(%q)\n", rootName))
		sb.WriteString(fmt.Sprintf("    }\n"))
		sb.WriteString(fmt.Sprintf("    mc, err := collection.NewModelCollection[%s, *%s](raw, logger, opts...)\n", rootTypeName, rootTypeName))
		sb.WriteString(fmt.Sprintf("    if err != nil {\n"))
		sb.WriteString(fmt.Sprintf("        return nil, common.SystemErrorFrom(err, \"ERR_MODEL_INIT_FAILED\").\n"))
		sb.WriteString(fmt.Sprintf("            WithOperation(\"Init%sModel\").\n", fnPrefix))
		sb.WriteString(fmt.Sprintf("            WithPath(%q)\n", rootName))
		sb.WriteString(fmt.Sprintf("    }\n"))
		sb.WriteString(fmt.Sprintf("    %s = &%s{ModelCollection: mc}\n", varName, collectionName))
		sb.WriteString(fmt.Sprintf("    return %s, nil\n", varName))
		sb.WriteString(fmt.Sprintf("}\n\n"))

		sb.WriteString(fmt.Sprintf("// %sModel returns the singleton %s model.\n", fnPrefix, rootTypeName))
		sb.WriteString(fmt.Sprintf("func %sModel() (*%s, error) {\n", fnPrefix, collectionName))
		sb.WriteString(fmt.Sprintf("    %sMu.Lock()\n", varName))
		sb.WriteString(fmt.Sprintf("    defer %sMu.Unlock()\n", varName))
		sb.WriteString(fmt.Sprintf("    if %s == nil {\n", varName))
		sb.WriteString(fmt.Sprintf("        return nil, common.NewSystemError(\"ERR_MODEL_NOT_INITIALIZED\",\n"))
		sb.WriteString(fmt.Sprintf("            \"%sModel not initialized — call Init%sModel first\").\n", fnPrefix, fnPrefix))
		sb.WriteString(fmt.Sprintf("            WithOperation(\"%sModel\")\n", fnPrefix))
		sb.WriteString(fmt.Sprintf("    }\n"))
		sb.WriteString(fmt.Sprintf("    return %s, nil\n", varName))
		sb.WriteString(fmt.Sprintf("}\n"))
	}

	return sb.String(), nil
}

// ============================================================================
// Internal types and helpers
// ============================================================================

type FieldDef struct {
	Name      string
	Type      string
	Required  bool
	Nullable  bool
	Default   any
	SchemaRef any // nil, map[string]any (named ref), []any (array ref), or map (inline)
}

type SchemaInfo struct {
	ID        string
	Name      string
	IsSchema  bool // has fields
	IsEnum    bool // has type and values
	Type      string
	Values    []any
	Fields    map[string]FieldDef
	SchemaRef any // only for Type mode with schema ref
}

type StructField struct {
	Name string
	Type string
	Tags string // e.g., `json:"name" schema:"..."`
}

type EnumDef struct {
	Underlying string
	Values     []any
}

// ----------------------------------------------------------------------------
// Parsing helpers
// ----------------------------------------------------------------------------

func getString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getBool(m map[string]any, key string, def bool) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return def
}

func parseFields(raw map[string]any) map[string]FieldDef {
	fields := make(map[string]FieldDef)
	for id, val := range raw {
		fm, ok := val.(map[string]any)
		if !ok {
			continue
		}
		fd := FieldDef{
			Name:      getString(fm, "name"),
			Type:      getString(fm, "type"),
			Required:  getBool(fm, "required", true),
			Nullable:  getBool(fm, "nullable", false),
			Default:   fm["default"],
			SchemaRef: fm["schema"],
		}
		fields[id] = fd
	}
	return fields
}

func parseSchemaInfo(id string, raw any) (*SchemaInfo, error) {
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("schema entry is not a map")
	}
	info := &SchemaInfo{ID: id, Name: getString(m, "name")}

	if fieldsRaw, ok := m["fields"].(map[string]any); ok && len(fieldsRaw) > 0 {
		info.IsSchema = true
		info.Fields = parseFields(fieldsRaw)
	}

	if typeStr := getString(m, "type"); typeStr != "" {
		info.Type = typeStr
		if values, ok := m["values"].([]any); ok && len(values) > 0 {
			info.IsEnum = true
			info.Values = values
		}
		if ref, ok := m["schema"]; ok {
			info.SchemaRef = ref
		}
	}

	if info.IsSchema && info.Type != "" {
		return nil, fmt.Errorf("schema %s has both 'fields' and 'type' (must be exclusive)", id)
	}
	if !info.IsSchema && info.Type == "" {
		return nil, fmt.Errorf("schema %s has neither 'fields' nor 'type'", id)
	}
	return info, nil
}

// ----------------------------------------------------------------------------
// Type resolution helpers
// ----------------------------------------------------------------------------

func mapPrimitiveTypeToGo(schemaType string) string {
	switch schemaType {
	case "string":
		return "string"
	case "number":
		return "float64"
	case "integer":
		return "int64"
	case "decimal":
		return "decimal.Decimal"
	case "boolean":
		return "bool"
	case "bytes":
		return "[]byte"
	case "unknown":
		return "any"
	case "geometry":
		return "[][]float64"
	default:
		return ""
	}
}

func resolveInlineTypeDescriptor(desc map[string]any, parent, field string, typeNames map[string]string, schemaInfos map[string]*SchemaInfo, structs map[string][]StructField, typeAliases map[string]string, enumDefs map[string]EnumDef, inlineEnumNames map[string]string, genInlineEnumName func(string, string) string, parentName string) (string, error) {
	typ := getString(desc, "type")
	if typ == "" {
		return "", fmt.Errorf("inline descriptor missing 'type'")
	}
	values, hasValues := desc["values"].([]any)

	if hasValues && len(values) > 0 {
		enumName := genInlineEnumName(parent, field)
		underlying := mapPrimitiveTypeToGo(typ)
		if underlying == "" {
			return "", fmt.Errorf("inline enum has unsupported underlying type %q", typ)
		}
		enumDefs[enumName] = EnumDef{
			Underlying: underlying,
			Values:     values,
		}
		return enumName, nil
	}

	goType := mapPrimitiveTypeToGo(typ)
	if goType == "" {
		return "", fmt.Errorf("inline type %q not supported", typ)
	}
	return goType, nil
}

func resolveSchemaReference(ref map[string]any, typeNames map[string]string, schemaInfos map[string]*SchemaInfo) (string, error) {
	id, ok := ref["id"].(string)
	if !ok || id == "" {
		return "", fmt.Errorf("schema reference missing 'id'")
	}
	name, exists := typeNames[id]
	if !exists {
		return "", fmt.Errorf("schema reference to unknown id %s", id)
	}
	return name, nil
}

func resolveTypeMode(schemaType string, schemaRef any, typeNames map[string]string, schemaInfos map[string]*SchemaInfo, structs map[string][]StructField, typeAliases map[string]string, enumDefs map[string]EnumDef, inlineEnumNames map[string]string, genInlineEnumName func(string, string) string, parentName string) (string, error) {
	switch schemaType {
	case "array":
		if schemaRef == nil {
			return "", fmt.Errorf("array type missing schema reference")
		}
		elemType, err := resolveReferenceOrInline(schemaRef, "", "", typeNames, schemaInfos, structs, typeAliases, enumDefs, inlineEnumNames, genInlineEnumName, parentName)
		if err != nil {
			return "", err
		}
		return "[]" + elemType, nil
	case "record":
		if schemaRef == nil {
			return "map[string]any", nil
		}
		valType, err := resolveReferenceOrInline(schemaRef, "", "", typeNames, schemaInfos, structs, typeAliases, enumDefs, inlineEnumNames, genInlineEnumName, parentName)
		if err != nil {
			return "", err
		}
		return "map[string]" + valType, nil
	default:
		goType := mapPrimitiveTypeToGo(schemaType)
		if goType == "" {
			return "", fmt.Errorf("unsupported Type mode type %q", schemaType)
		}
		return goType, nil
	}
}

func resolveReferenceOrInline(ref any, parent, field string, typeNames map[string]string, schemaInfos map[string]*SchemaInfo, structs map[string][]StructField, typeAliases map[string]string, enumDefs map[string]EnumDef, inlineEnumNames map[string]string, genInlineEnumName func(string, string) string, parentName string) (string, error) {
	if ref == nil {
		return "", nil
	}
	switch v := ref.(type) {
	case map[string]any:
		if _, hasID := v["id"]; hasID {
			return resolveSchemaReference(v, typeNames, schemaInfos)
		}
		if _, hasType := v["type"]; hasType {
			return resolveInlineTypeDescriptor(v, parent, field, typeNames, schemaInfos, structs, typeAliases, enumDefs, inlineEnumNames, genInlineEnumName, parentName)
		}
		return "", fmt.Errorf("invalid schema reference: neither 'id' nor 'type'")
	case []any:
		return "", fmt.Errorf("unexpected array of references for non-union/composite")
	default:
		return "", fmt.Errorf("unexpected schema reference type %T", ref)
	}
}

// ----------------------------------------------------------------------------
// Field generation with configurable tags
// ----------------------------------------------------------------------------

func generateFields(fields map[string]FieldDef, typeNames map[string]string, schemaInfos map[string]*SchemaInfo, structs map[string][]StructField, typeAliases map[string]string, enumDefs map[string]EnumDef, inlineEnumNames map[string]string, genInlineEnumName func(string, string) string, parentName string, tagConfig TagConfig) ([]StructField, error) {
	var result []StructField

	for _, fd := range fields {
		fieldName := fd.Name
		if fieldName == "" {
			continue
		}
		goFieldName := toCamelCase(fieldName)

		var goType string

		switch fd.Type {
		case "union":
			refs, ok := fd.SchemaRef.([]any)
			if !ok || len(refs) == 0 {
				return nil, fmt.Errorf("union field %s.%s: missing or invalid schema refs", parentName, fieldName)
			}
			containerName := genInlineEnumName(parentName, fieldName) + "Union"
			containerFields := []StructField{}
			for _, r := range refs {
				rmap, ok := r.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("union ref is not a map")
				}
				refType, err := resolveSchemaReference(rmap, typeNames, schemaInfos)
				if err != nil {
					return nil, err
				}
				containerFields = append(containerFields, StructField{
					Name: refType,
					Type: "*" + refType,
					Tags: fmt.Sprintf("json:\"%s,omitempty\" schema:\"%s,omitempty\"", toSnakeCase(refType), toSnakeCase(refType)),
				})
			}
			structs[containerName] = containerFields
			goType = containerName

		case "composite":
			refs, ok := fd.SchemaRef.([]any)
			if !ok || len(refs) == 0 {
				return nil, fmt.Errorf("composite field %s.%s: missing or invalid schema refs", parentName, fieldName)
			}
			containerName := genInlineEnumName(parentName, fieldName) + "Composite"
			containerFields := []StructField{}
			for _, r := range refs {
				rmap, ok := r.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("composite ref is not a map")
				}
				refType, err := resolveSchemaReference(rmap, typeNames, schemaInfos)
				if err != nil {
					return nil, err
				}
				containerFields = append(containerFields, StructField{
					Name: "",
					Type: refType,
					Tags: "",
				})
			}
			structs[containerName] = containerFields
			goType = containerName

		case "array":
			if fd.SchemaRef == nil {
				return nil, fmt.Errorf("array field %s.%s missing schema reference", parentName, fieldName)
			}
			elemType, err := resolveReferenceOrInline(fd.SchemaRef, parentName, fieldName, typeNames, schemaInfos, structs, typeAliases, enumDefs, inlineEnumNames, genInlineEnumName, parentName)
			if err != nil {
				return nil, err
			}
			if elemType == "" {
				return nil, fmt.Errorf("array field %s.%s resolved to empty type", parentName, fieldName)
			}
			goType = "[]" + elemType

		case "record":
			if fd.SchemaRef == nil {
				goType = "map[string]any"
			} else {
				valType, err := resolveReferenceOrInline(fd.SchemaRef, parentName, fieldName, typeNames, schemaInfos, structs, typeAliases, enumDefs, inlineEnumNames, genInlineEnumName, parentName)
				if err != nil {
					return nil, err
				}
				if valType == "" {
					return nil, fmt.Errorf("record field %s.%s resolved to empty value type", parentName, fieldName)
				}
				goType = "map[string]" + valType
			}

		case "object":
			refMap, ok := fd.SchemaRef.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("object field %s.%s missing schema reference", parentName, fieldName)
			}
			refType, err := resolveSchemaReference(refMap, typeNames, schemaInfos)
			if err != nil {
				return nil, err
			}
			goType = refType

		case "enum":
			if fd.SchemaRef == nil {
				return nil, fmt.Errorf("enum field %s.%s missing schema reference", parentName, fieldName)
			}
			enumType, err := resolveReferenceOrInline(fd.SchemaRef, parentName, fieldName, typeNames, schemaInfos, structs, typeAliases, enumDefs, inlineEnumNames, genInlineEnumName, parentName)
			if err != nil {
				return nil, err
			}
			goType = enumType

		default:
			goType = mapPrimitiveTypeToGo(fd.Type)
			if goType == "" {
				return nil, fmt.Errorf("unsupported field type %q for %s.%s", fd.Type, parentName, fieldName)
			}
		}

		// Decide pointer based on Required/Nullable
		if !fd.Required || fd.Nullable {
			if !isReferenceType(goType) {
				goType = "*" + goType
			}
		}

		// Build tags from configuration
		tags := buildTags(fd, fieldName, goFieldName, tagConfig)

		result = append(result, StructField{
			Name: goFieldName,
			Type: goType,
			Tags: tags,
		})
	}

	return result, nil
}

// isReferenceType returns true if the type is already a reference type (slice, map, interface).
func isReferenceType(goType string) bool {
	if strings.HasPrefix(goType, "[]") {
		return true
	}
	if strings.HasPrefix(goType, "map[") {
		return true
	}
	if goType == "any" {
		return true
	}
	return false
}

// buildTags constructs a space-separated string of struct tags from the TagConfig.
func buildTags(fd FieldDef, jsonName, goFieldName string, tagConfig TagConfig) string {
	var parts []string
	for _, rule := range tagConfig {
		if rule.Key == "anansi" {
			tagVal := buildSchemaTagValue(fd, jsonName)
			parts = append(parts, fmt.Sprintf(`anansi:"%s"`, tagVal))
		} else {
			// Simple tag: key:"fieldName" with optional omitempty.
			tagVal := jsonName
			if rule.OmitEmpty && !fd.Required {
				tagVal += ",omitempty"
			}
			parts = append(parts, fmt.Sprintf(`%s:"%s"`, rule.Key, tagVal))
		}
	}
	return strings.Join(parts, " ")
}

// buildSchemaTagValue creates the value for the "anansi" tag with full metadata.
func buildSchemaTagValue(fd FieldDef, fieldName string) string {
	// Start with field name
	tagVal := fieldName

	// Required flag
	if !fd.Required {
		tagVal += ",required=false"
	}
	// Nullable flag
	if fd.Nullable {
		tagVal += ",nullable=true"
	}
	// Type override if needed (only for specific types)
	switch fd.Type {
	case "union", "composite", "enum", "geometry", "bytes", "unknown":
		tagVal += fmt.Sprintf(",type=%s", fd.Type)
	}
	// Default value
	if fd.Default != nil {
		defaultStr := formatLiteral(fd.Default)
		tagVal += fmt.Sprintf(",default=%s", defaultStr)
	}
	return tagVal
}

// ----------------------------------------------------------------------------
// Naming and formatting helpers
// ----------------------------------------------------------------------------

var (
	snakeToCamel = regexp.MustCompile(`_([a-z])`)

	goAcronyms = map[string]string{
		"id":   "ID",
		"url":  "URL",
		"http": "HTTP",
		"json": "JSON",
		"api":  "API",
		"uuid": "UUID",
		"uri":  "URI",
		"html": "HTML",
		"xml":  "XML",
		"sql":  "SQL",
		"ftp":  "FTP",
		"ssh":  "SSH",
		"db":   "DB",
		"io":   "IO",
		"os":   "OS",
		"ui":   "UI",
	}
)

func lowerFirst(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func toCamelCase(s string) string {
	if s == "" {
		return ""
	}
	camel := snakeToCamel.ReplaceAllStringFunc(s, func(m string) string {
		return strings.ToUpper(m[1:])
	})
	if len(camel) > 0 {
		camel = strings.ToUpper(camel[:1]) + camel[1:]
	}
	return fixAcronyms(camel)
}

var wordBoundary = regexp.MustCompile(`[A-Z]`)

func fixAcronyms(s string) string {
	var words []string
	start := 0
	for _, idx := range wordBoundary.FindAllStringIndex(s[1:], -1) {
		i := idx[0] + 1
		words = append(words, s[start:i])
		start = i
	}
	words = append(words, s[start:])
	for i, w := range words {
		if acronym, ok := goAcronyms[strings.ToLower(w)]; ok {
			words[i] = acronym
		}
	}
	return strings.Join(words, "")
}

func typeSize(typ string) int {
	if strings.HasPrefix(typ, "*") {
		return 8
	}
	if strings.HasPrefix(typ, "[]") {
		return 24
	}
	if strings.HasPrefix(typ, "map[") {
		return 8
	}
	switch typ {
	case "bool":
		return 1
	case "int8", "uint8", "byte":
		return 1
	case "int16", "uint16":
		return 2
	case "int32", "uint32", "float32":
		return 4
	case "int", "uint", "int64", "uint64", "float64":
		return 8
	case "string":
		return 16
	case "any":
		return 16
	case "time.Time":
		return 24
	case "decimal.Decimal":
		return 16
	case "data.DocumentModel":
		return 40
	default:
		return 8
	}
}

func toSnakeCase(s string) string {
	var result []rune
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result = append(result, '_')
		}
		result = append(result, r)
	}
	return strings.ToLower(string(result))
}

func formatLiteral(v any) string {
	switch val := v.(type) {
	case string:
		return fmt.Sprintf("%q", val)
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%v", val)
	case bool:
		return fmt.Sprintf("%t", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}
