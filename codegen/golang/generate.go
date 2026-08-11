package golang

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
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
	mode        GenerationMode
}

// reservedMetadataSchemaName is the system nested schema injected by schema
// normalization and migration generation to carry the per-document integrity
// envelope (version, updated, checksum, signature, created).
const reservedMetadataSchemaName = "_metadata_"

// ============================================================================
// Generation modes
// ============================================================================

// GenerationMode is a bitmask selecting which layers of generated output to
// emit. Layers are cumulative: each higher layer implies everything below it,
// so the flags express "emit at least up to this layer".
//
// A zero value is valid and selects ModeFull (the default).
type GenerationMode uint32

const (
	// ModeStructs emits type declarations only (aliases, enums, structs). The
	// root schema is emitted like any other schema — no DocumentModel embed,
	// no constructor. Suitable for plain DTOs that don't touch persistence.
	ModeStructs GenerationMode = 1 << iota

	// ModeModel emits ModeStructs plus the root model treatment: the root
	// struct embeds document.DocumentModel and a New constructor is emitted.
	ModeModel

	// ModeCollection emits ModeModel plus the typed collection wrapper, the
	// collection-name constant, the singleton instance, and the Init/accessor
	// functions. This is the full persistence stack.
	ModeCollection

	// ModeFull is the default: every layer is emitted.
	ModeFull = ModeStructs | ModeModel | ModeCollection
)

// emitsStructs reports whether type declarations are included in the mode.
func (m GenerationMode) emitsStructs() bool {
	return m&(ModeStructs|ModeModel|ModeCollection) != 0
}

// emitsRootModel reports whether the root struct gets a DocumentModel embed.
func (m GenerationMode) emitsRootModel() bool {
	return m&(ModeModel|ModeCollection) != 0
}

// emitsCollection reports whether the collection scaffold is included.
func (m GenerationMode) emitsCollection() bool {
	return m&ModeCollection != 0
}

// String returns the canonical string form of the mode: the highest layer
// requested ("structs", "model", or "full"), or "" for an empty mode.
func (m GenerationMode) String() string {
	switch {
	case m == 0:
		return ""
	case m.emitsCollection():
		return "full"
	case m.emitsRootModel():
		return "model"
	case m.emitsStructs():
		return "structs"
	default:
		return ""
	}
}

// ParseGenerationMode parses a mode from its string form. Accepted tokens are
// "structs", "model", "collection", and "full"; multiple tokens may be
// comma-separated (e.g. "structs,collection"). Each token expands to include
// the layers it implies.
func ParseGenerationMode(s string) (GenerationMode, error) {
	trimmed := strings.ToLower(strings.TrimSpace(s))
	if trimmed == "" {
		return 0, fmt.Errorf("generation mode is empty (valid: structs, model, collection, full)")
	}
	if trimmed == "full" {
		return ModeFull, nil
	}

	var mode GenerationMode
	for _, tok := range strings.Split(trimmed, ",") {
		switch strings.TrimSpace(tok) {
		case "full":
			mode |= ModeFull
		case "collection":
			mode |= ModeStructs | ModeModel | ModeCollection
		case "model":
			mode |= ModeStructs | ModeModel
		case "structs":
			mode |= ModeStructs
		default:
			return 0, fmt.Errorf("unknown generation mode %q (valid: structs, model, collection, full)", tok)
		}
	}
	return mode, nil
}

// ============================================================================
// Generator entry point
// ============================================================================

type GeneratorConfig struct {
	TagConfig      TagConfig
	ScopedPackages bool
	NameRules      []NameRule
	PackageName    string
	// Mode selects which layers of output to emit. Zero selects ModeFull.
	Mode GenerationMode
}

func NewGoGenerator(config *GeneratorConfig) *GoGenerator {
	if config == nil {
		config = &GeneratorConfig{
			TagConfig: DefaultTagConfig(),
			NameRules: make([]NameRule, 0),
		}
	}
	mode := config.Mode
	if mode == 0 {
		mode = ModeFull
	}
	return &GoGenerator{tagConfig: config.TagConfig, scoped: config.ScopedPackages, nameRules: config.NameRules, packageName: config.PackageName, mode: mode}
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
	rootTypeName := g.goTypeName(rootName)
	typeNames := make(map[string]string)
	for id, info := range schemaInfos {
		if info.Name == reservedMetadataSchemaName {
			// The reserved _metadata_ nested schema gets a per-collection type
			// name. Multiple schemas in one package each declare their own
			// metadata struct (XxxMetadata), so a shared name would collide —
			// and per-schema structs keep any per-schema custom tags intact.
			typeNames[id] = rootTypeName + "Metadata"
			continue
		}
		typeNames[id] = g.goTypeName(info.Name)
	}

	// Maps to hold generated definitions
	structs := make(map[string][]StructField) // struct name -> fields
	typeAliases := make(map[string]string)    // name -> underlying type expr
	enumDefs := make(map[string]EnumDef)      // name -> enum definition
	inlineNames := make(map[string]string)    // key -> generated type name for inline container

	// Helper to generate inline container types (unique name per parent+field).
	// kind is the container kind: "enum", "union", or "composite".
	genInlineName := func(parent, field, kind string) string {
		key := parent + "_" + field + "_" + kind
		if name, ok := inlineNames[key]; ok {
			return name
		}
		name := toCamelCase(parent + "_" + field + "_" + kind)
		inlineNames[key] = name
		return name
	}

	// Process all nested schemas
	for id, info := range schemaInfos {
		if info.IsSchema {
			fields, _, err := generateFields(info.Fields, typeNames, schemaInfos, structs, typeAliases, enumDefs, inlineNames, genInlineName, info.Name, tagConfig)
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
						Tags: buildVariantTags(toSnakeCase(refType), tagConfig),
					})
				}
				structs[typeNames[id]] = fields

			default:
				// array, record, or primitive
				goType, err := resolveTypeMode(info.Type, info.SchemaRef, typeNames, schemaInfos, structs, typeAliases, enumDefs, inlineNames, genInlineName, info.Name)
				if err != nil {
					return "", fmt.Errorf("failed to resolve type mode for schema %s: %w", id, err)
				}
				typeAliases[typeNames[id]] = goType
			}
		}
	}

	// Process root schema
	rootFieldsGo, rootIDFieldName, err := generateFields(rootFields, typeNames, schemaInfos, structs, typeAliases, enumDefs, inlineNames, genInlineName, rootName, tagConfig)
	if err != nil {
		return "", fmt.Errorf("failed to generate root fields: %w", err)
	}
	structs[rootTypeName] = rootFieldsGo

	// Detect whether the root schema already declares the reserved system
	// fields. When they are absent the generated root model embeds
	// document.DocumentModel and additionally emits shadow ID/Metadata fields;
	// when present they are emitted as ordinary schema fields instead.
	rootHasSystemFields := false
	for _, fd := range rootFields {
		if fd.Name == "_id_" || fd.Name == reservedMetadataSchemaName {
			rootHasSystemFields = true
			break
		}
	}

	// Resolve projections declared under metadata.projections. Resolution and
	// validation run in every mode so that schema errors surface regardless of
	// which layers are emitted; only render decides whether they appear.
	projections := make(map[string][]StructField)
	projectionSpecs, err := parseProjections(data)
	if err != nil {
		return "", err
	}
	for _, projName := range sortedKeys(projectionSpecs) {
		projGoName := g.goTypeName(projName)
		if projGoName == rootTypeName {
			return "", fmt.Errorf("projection %s conflicts with the root model type %s", projName, projGoName)
		}
		if _, ok := structs[projGoName]; ok {
			return "", fmt.Errorf("projection %s produces a type name (%s) that collides with an existing type", projName, projGoName)
		}
		projFields, projTags, err := resolveProjection(projName, projectionSpecs[projName], rootFields)
		if err != nil {
			return "", err
		}
		// Reuse the root as parentName so inline containers (enum/union/
		// composite/record) are shared with the root struct instead of being
		// regenerated per projection.
		projFieldsGo, _, err := generateFields(projFields, typeNames, schemaInfos, structs, typeAliases, enumDefs, inlineNames, genInlineName, rootName, tagConfig)
		if err != nil {
			return "", fmt.Errorf("failed to generate fields for projection %s: %w", projName, err)
		}
		if err := applyProjectionTags(projFieldsGo, projFields, projTags); err != nil {
			return "", fmt.Errorf("projection %s: %w", projName, err)
		}
		projections[projGoName] = projFieldsGo
	}

	// Build output
	return g.render(rootName, rootTypeName, structs, typeAliases, enumDefs, projections, rootHasSystemFields, rootIDFieldName)
}

// ============================================================================
// Output rendering
// ============================================================================

// render assembles the generated source from the parsed type definitions. The
// generation mode controls which layers are emitted; all type-generation work
// happens before this point and is shared across modes. Output is deterministic:
// every map is emitted in sorted key order.
func (g *GoGenerator) render(rootSchemaName, rootTypeName string, structs map[string][]StructField, typeAliases map[string]string, enumDefs map[string]EnumDef, projections map[string][]StructField, rootHasSystemFields bool, rootIDFieldName string) (string, error) {
	mode := g.mode
	emitRootModel := mode.emitsRootModel()
	emitCollection := mode.emitsCollection()

	// Determine which imports the emitted layers reference.
	needsDecimal := false
	for _, typ := range typeAliases {
		if strings.Contains(typ, "decimal.Decimal") {
			needsDecimal = true
			break
		}
	}
	if !needsDecimal {
		for _, fields := range structs {
			for _, f := range fields {
				if strings.Contains(f.Type, "decimal.Decimal") {
					needsDecimal = true
					break
				}
			}
			if needsDecimal {
				break
			}
		}
	}
	if !needsDecimal {
		for _, fields := range projections {
			for _, f := range fields {
				if strings.Contains(f.Type, "decimal.Decimal") {
					needsDecimal = true
					break
				}
			}
			if needsDecimal {
				break
			}
		}
	}

	var imports []string
	if emitRootModel {
		imports = append(imports, "\"github.com/asaidimu/go-anansi/v8/core/document\"")
	}
	if needsDecimal {
		imports = append(imports, "\"github.com/asaidimu/go-anansi/v8/core/types/decimal\"")
	}
	if emitCollection {
		imports = append(imports,
			"\"context\"",
			"\"sync\"",
			"\"github.com/asaidimu/go-anansi/v8/core/common\"",
			"\"github.com/asaidimu/go-anansi/v8/core/persistence/base\"",
			"\"github.com/asaidimu/go-anansi/v8/core/persistence/collection\"",
			"\"go.uber.org/zap\"",
		)
	}

	var sb strings.Builder

	// File header
	sb.WriteString("// Code generated by anansi. DO NOT EDIT.\n")
	sb.WriteString("//\n")
	sb.WriteString("// This file is auto-generated from the schema definition.\n")
	sb.WriteString("// Always run codegen alongside database migrations so that the\n")
	sb.WriteString("// generated models stay in sync with the schema on file.\n")
	sb.WriteString("//\n")
	sb.WriteString("// Add custom methods to the ")
	sb.WriteString(rootTypeName)
	sb.WriteString(" model in separate Go files,\n")
	sb.WriteString("// using filenames that reflect their purpose (e.g., ")
	sb.WriteString(toSnakeCase(rootTypeName))
	sb.WriteString("_validation.go,\n")
	sb.WriteString("// ")
	sb.WriteString(toSnakeCase(rootTypeName))
	sb.WriteString("_serialization.go). Avoid throwing all logic into a\n")
	sb.WriteString("// single ")
	sb.WriteString(toSnakeCase(rootTypeName))
	sb.WriteString("_utils.go file.\n")
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
	if len(imports) > 0 {
		sb.WriteString("import (\n")
		for _, imp := range imports {
			sb.WriteString("\t")
			sb.WriteString(imp)
			sb.WriteString("\n")
		}
		sb.WriteString(")\n\n")
	}

	// Type aliases (non-enum)
	for _, name := range sortedKeys(typeAliases) {
		fmt.Fprintf(&sb, "type %s = %s\n", name, typeAliases[name])
	}
	if len(typeAliases) > 0 {
		sb.WriteString("\n")
	}

	// Enum types
	for _, name := range sortedKeys(enumDefs) {
		enum := enumDefs[name]
		fmt.Fprintf(&sb, "type %s %s\n\n", name, enum.Underlying)
		sb.WriteString("const (\n")
		for _, v := range enum.Values {
			valStr := formatLiteral(v)
			constName := toCamelCase(fmt.Sprintf("%s_%v", name, v))
			fmt.Fprintf(&sb, "    %s %s = %s\n", constName, name, valStr)
		}
		sb.WriteString(")\n\n")
	}

	// Structs. The root struct is emitted in sorted order like any other schema
	// unless it receives the model treatment, in which case it is emitted last
	// with the DocumentModel embed.
	for _, name := range sortedKeys(structs) {
		if name == rootTypeName && emitRootModel {
			continue
		}
		emitStruct(&sb, name, structs[name], false, true)
	}
	if emitRootModel {
		// When the schema does not declare the system fields, the root model
		// embeds document.DocumentModel and emits shadow ID/Metadata fields so
		// the struct exposes the system fields with canonical (and
		// customizable) tags. When the schema declares them, they are already
		// emitted as ordinary fields and nothing extra is added.
		rootFields := structs[rootTypeName]
		shadowIDName := ""
		if !rootHasSystemFields {
			rootFields, shadowIDName = appendSystemShadows(rootFields)
		}
		emitStruct(&sb, rootTypeName, rootFields, true, false)

		// GetID returns the document identifier carried by the _id_ field —
		// either the shadow (schema without system fields) or the ordinary
		// schema field named _id_ (enriched schema). It shadows the promoted
		// DocumentModel.GetID so the struct's own field is authoritative.
		idName := rootIDFieldName
		if idName == "" {
			idName = shadowIDName
			if idName == "" {
				idName = "ID"
			}
		}
		fmt.Fprintf(&sb, "// GetID returns the document identifier for %s.\n", rootTypeName)
		fmt.Fprintf(&sb, "func (m *%s) GetID() string {\n    return m.%s\n}\n\n", rootTypeName, idName)

		// Constructor
		if g.scoped {
			sb.WriteString(fmt.Sprintf("// New creates and initializes a new %s\n", rootTypeName))
			sb.WriteString(fmt.Sprintf("func New(model %s) *%s {\n", rootTypeName, rootTypeName))
			sb.WriteString(fmt.Sprintf("    return document.New(&model)\n"))
			sb.WriteString("}\n\n")
		} else {
			sb.WriteString(fmt.Sprintf("// New%s creates and initializes a new %s\n", rootTypeName, rootTypeName))
			sb.WriteString(fmt.Sprintf("func New%s(model %s) *%s {\n", rootTypeName, rootTypeName, rootTypeName))
			sb.WriteString(fmt.Sprintf("    return document.New(&model)\n"))
			sb.WriteString("}\n\n")
		}

		if emitCollection {
			emitCollectionScaffold(&sb, g.scoped, rootTypeName, rootSchemaName)
		}

		// Projections share the model treatment (DocumentModel embed).
		for _, name := range sortedKeys(projections) {
			emitStruct(&sb, name, projections[name], true, true)
		}
	}

	return sb.String(), nil
}

// emitStruct writes a single struct declaration. Fields are sorted by size so
// that padding is minimized. embedModel embeds document.DocumentModel first.
func emitStruct(sb *strings.Builder, name string, fields []StructField, embedModel bool, omitModelTags bool) {
	sort.Slice(fields, func(i, j int) bool {
		return typeSize(fields[i].Type) > typeSize(fields[j].Type)
	})
	fmt.Fprintf(sb, "type %s struct {\n", name)
	if embedModel {
		if omitModelTags {
			sb.WriteString("    document.DocumentModel `json:\"-\" anansi:\"-\"`\n")
		} else {
			sb.WriteString("    document.DocumentModel\n")
		}
	}
	for _, f := range fields {
		tagStr := strings.Trim(f.Tags, "`")
		if tagStr == "" {
			fmt.Fprintf(sb, "    %s %s\n", f.Name, f.Type)
			continue
		}
		fmt.Fprintf(sb, "    %s %s `%s`\n", f.Name, f.Type, tagStr)
	}
	sb.WriteString("}\n\n")
}

// appendSystemShadows appends shadow fields for the reserved _id_/_metadata_
// system fields to the root model's field list. The shadows mirror the
// embedded document.DocumentModel's fields with canonical tags, so the
// generated model exposes the system fields as ordinary struct fields (and
// document.New keeps them in sync with the embed). If the schema already
// declares a Go field named ID or Metadata, the shadow is renamed to
// ModelID/ModelMetadata to avoid a declaration collision. Returns the
// augmented field list and the chosen _id_ shadow's Go field name.
func appendSystemShadows(fields []StructField) ([]StructField, string) {
	used := make(map[string]bool, len(fields))
	for _, f := range fields {
		used[f.Name] = true
	}

	idName := "ID"
	if used[idName] {
		idName = "ModelID"
	}
	metaName := "Metadata"
	if used[metaName] {
		metaName = "ModelMetadata"
	}

	shadows := append(append([]StructField(nil), fields...),
		StructField{Name: idName, Type: "string", Tags: `json:"_id_,omitempty" anansi:"_id_,required=true,omitempty"`},
		StructField{Name: metaName, Type: "map[string]any", Tags: `json:"_metadata_,omitempty" anansi:"_metadata_,required=true,omitempty"`},
	)
	return shadows, idName
}

// emitCollectionScaffold writes the typed collection wrapper, the collection
// name constant, the singleton instance, and the Init/accessor functions.
// rootTypeName is the Go name of the model type; rootSchemaName is the raw
// schema/collection name used as the collection identifier.
func emitCollectionScaffold(sb *strings.Builder, scoped bool, rootTypeName, rootSchemaName string) {
	collectionName := rootTypeName + "s"
	sb.WriteString(fmt.Sprintf("// %s is a type-safe collection for %s\n", collectionName, rootTypeName))
	sb.WriteString(fmt.Sprintf("type %s struct {\n", collectionName))
	sb.WriteString(fmt.Sprintf("    *collection.ModelCollection[*%s]\n", rootTypeName))
	sb.WriteString("}\n\n")

	// Singleton model access — unexported vars, exported functions.
	// In scoped mode (one model per package) the function prefix is
	// empty — InitModel / Model. In non-scoped mode the collection
	// name is used — InitUsersModel / UsersModel.
	var fnPrefix string
	var constName string
	var varName string
	if scoped {
		constName = "CollectionName"
		varName = "model"
	} else {
		fnPrefix = collectionName
		constName = collectionName + "CollectionName"
		varName = lowerFirst(collectionName) + "Model"
	}
	sb.WriteString(fmt.Sprintf("const %s = %q\n\n", constName, rootSchemaName))
	sb.WriteString(fmt.Sprintf("var (\n"))
	sb.WriteString(fmt.Sprintf("    %sMu sync.Mutex\n", varName))
	sb.WriteString(fmt.Sprintf("    %s   *%s\n", varName, collectionName))
	sb.WriteString(fmt.Sprintf(")\n\n"))

	sb.WriteString(fmt.Sprintf("// Init%sModel must be called once at startup to configure and\n", fnPrefix))
	sb.WriteString(fmt.Sprintf("// construct the %s model. Idempotent — subsequent calls return\n", rootTypeName))
	sb.WriteString(fmt.Sprintf("// the existing instance. Retry-safe: if the first call fails, the\n"))
	sb.WriteString(fmt.Sprintf("// caller can fix the underlying issue and call again.\n"))
	sb.WriteString(fmt.Sprintf("func Init%sModel(p base.Persistence, logger *zap.Logger, opts ...collection.ModelCollectionOptions[*%s]) (*%s, error) {\n", fnPrefix, rootTypeName, collectionName))
	sb.WriteString(fmt.Sprintf("    %sMu.Lock()\n", varName))
	sb.WriteString(fmt.Sprintf("    defer %sMu.Unlock()\n", varName))
	sb.WriteString(fmt.Sprintf("    if %s != nil {\n", varName))
	sb.WriteString(fmt.Sprintf("        return %s, nil\n", varName))
	sb.WriteString(fmt.Sprintf("    }\n"))
	sb.WriteString(fmt.Sprintf("    raw, err := p.Collection(context.Background(), %s)\n", constName))
	sb.WriteString(fmt.Sprintf("    if err != nil {\n"))
	sb.WriteString(fmt.Sprintf("        return nil, common.SystemErrorFrom(err, \"ERR_MODEL_INIT_FAILED\").\n"))
	sb.WriteString(fmt.Sprintf("            WithOperation(\"Init%sModel\").\n", fnPrefix))
	sb.WriteString(fmt.Sprintf("            WithPath(%q)\n", rootSchemaName))
	sb.WriteString(fmt.Sprintf("    }\n"))
	sb.WriteString(fmt.Sprintf("    mc, err := collection.NewModelCollection[*%s](raw, logger, opts...)\n", rootTypeName))
	sb.WriteString(fmt.Sprintf("    if err != nil {\n"))
	sb.WriteString(fmt.Sprintf("        return nil, common.SystemErrorFrom(err, \"ERR_MODEL_INIT_FAILED\").\n"))
	sb.WriteString(fmt.Sprintf("            WithOperation(\"Init%sModel\").\n", fnPrefix))
	sb.WriteString(fmt.Sprintf("            WithPath(%q)\n", rootSchemaName))
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
	sb.WriteString(fmt.Sprintf("}\n\n"))

	// DangerouslyReset%[1]sModel clears the cached singleton so Init%[1]sModel
	// can be called again — for example after rebuilding the persistence layer
	// in tests. It closes the previous instance's managed cache, if any.
	sb.WriteString(fmt.Sprintf("// Deprecated: DangerouslyReset%sModel clears the cached singleton so\n", fnPrefix))
	sb.WriteString(fmt.Sprintf("// Init%sModel can be called again — for example after rebuilding the\n", fnPrefix))
	sb.WriteString(fmt.Sprintf("// persistence layer in tests. It closes the previous instance's managed\n"))
	sb.WriteString(fmt.Sprintf("// cache, if any. Never use in production code.\n"))
	sb.WriteString(fmt.Sprintf("func DangerouslyReset%sModel() {\n", fnPrefix))
	sb.WriteString(fmt.Sprintf("    %sMu.Lock()\n", varName))
	sb.WriteString(fmt.Sprintf("    defer %sMu.Unlock()\n", varName))
	sb.WriteString(fmt.Sprintf("    if %s == nil {\n", varName))
	sb.WriteString(fmt.Sprintf("        return\n"))
	sb.WriteString(fmt.Sprintf("    }\n"))
	sb.WriteString(fmt.Sprintf("    _ = %s.Close()\n", varName))
	sb.WriteString(fmt.Sprintf("    %s = nil\n", varName))
	sb.WriteString(fmt.Sprintf("}\n"))
}

// sortedKeys returns the keys of a string-keyed map in ascending order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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
			Required:  getBool(fm, "required", false),
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
// Projections
// ----------------------------------------------------------------------------

// projectionSpec is the parsed form of a single projection declaration.
type projectionSpec struct {
	Include  []string
	Exclude  []string
	Required []string
	Optional []string
	// Tags maps a field name to custom struct tags (tag key -> template).
	// Templates may reference field properties via {prop} placeholders.
	Tags map[string]map[string]string
}

// parseProjections extracts the projections declared under
// metadata.projections. Each projection maps a name to a field DSL:
//
//	"fields": {
//	    "include":  ["total", "status"],   // whitelist (default: all fields)
//	    "exclude":  ["internal"],          // removed from the final set
//	    "required": ["total"],             // force required=true
//	    "optional": ["status"],            // force required=false
//	    "tags": {
//	        "total": { "input": "arguments.{name}" }
//	    }
//	}
func parseProjections(data map[string]any) (map[string]projectionSpec, error) {
	metadata, ok := data["metadata"].(map[string]any)
	if !ok {
		return nil, nil
	}
	rawProjections, ok := metadata["projections"].(map[string]any)
	if !ok || len(rawProjections) == 0 {
		return nil, nil
	}

	projections := make(map[string]projectionSpec, len(rawProjections))
	for name, raw := range rawProjections {
		pm, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("projection %s must be an object", name)
		}
		fieldsRaw, ok := pm["fields"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("projection %s missing 'fields'", name)
		}

		var spec projectionSpec
		var err error
		if spec.Include, err = parseProjectionFieldList(fieldsRaw, "include"); err != nil {
			return nil, fmt.Errorf("projection %s: %w", name, err)
		}
		if spec.Exclude, err = parseProjectionFieldList(fieldsRaw, "exclude"); err != nil {
			return nil, fmt.Errorf("projection %s: %w", name, err)
		}
		if spec.Required, err = parseProjectionFieldList(fieldsRaw, "required"); err != nil {
			return nil, fmt.Errorf("projection %s: %w", name, err)
		}
		if spec.Optional, err = parseProjectionFieldList(fieldsRaw, "optional"); err != nil {
			return nil, fmt.Errorf("projection %s: %w", name, err)
		}
		if spec.Tags, err = parseProjectionTags(fieldsRaw); err != nil {
			return nil, fmt.Errorf("projection %s: %w", name, err)
		}
		projections[name] = spec
	}
	return projections, nil
}

func parseProjectionTags(fields map[string]any) (map[string]map[string]string, error) {
	rawTags, ok := fields["tags"].(map[string]any)
	if !ok {
		return nil, nil
	}
	tags := make(map[string]map[string]string, len(rawTags))
	for fieldName, raw := range rawTags {
		tagMap, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("tags for field %q must be an object", fieldName)
		}
		entry := make(map[string]string, len(tagMap))
		for key, val := range tagMap {
			sv, ok := val.(string)
			if !ok {
				return nil, fmt.Errorf("tag %q on field %q must be a string", key, fieldName)
			}
			entry[key] = sv
		}
		tags[fieldName] = entry
	}
	return tags, nil
}

func parseProjectionFieldList(fields map[string]any, key string) ([]string, error) {
	raw, ok := fields[key]
	if !ok {
		return nil, nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("'%s' must be an array of field names", key)
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		s, ok := e.(string)
		if !ok {
			return nil, fmt.Errorf("'%s' entries must be strings", key)
		}
		out = append(out, s)
	}
	return out, nil
}

// resolveProjection computes the effective field set for a projection by
// applying the include/exclude/required/optional DSL over the root fields.
// It fails fast on any self-inconsistent or unknown reference. The returned
// tags map is keyed by field name; every key is guaranteed to be part of the
// final field set.
func resolveProjection(name string, spec projectionSpec, rootFields map[string]FieldDef) (map[string]FieldDef, map[string]map[string]string, error) {
	byName := make(map[string]FieldDef, len(rootFields))
	for _, fd := range rootFields {
		if fd.Name != "" {
			byName[fd.Name] = fd
		}
	}

	// Every referenced field must exist in the root schema.
	for _, list := range [][]string{spec.Include, spec.Exclude, spec.Required, spec.Optional} {
		for _, f := range list {
			if _, ok := byName[f]; !ok {
				return nil, nil, fmt.Errorf("projection %s references unknown field %q", name, f)
			}
		}
	}

	// A field cannot be both included and excluded.
	excluded := make(map[string]bool, len(spec.Exclude))
	for _, f := range spec.Exclude {
		if slices.Contains(spec.Include, f) {
			return nil, nil, fmt.Errorf("projection %s: field %q cannot be both included and excluded", name, f)
		}
		excluded[f] = true
	}

	// A field cannot be both required and optional.
	required := make(map[string]bool, len(spec.Required))
	for _, f := range spec.Required {
		if slices.Contains(spec.Optional, f) {
			return nil, nil, fmt.Errorf("projection %s: field %q cannot be both required and optional", name, f)
		}
		required[f] = true
	}
	optional := make(map[string]bool, len(spec.Optional))
	for _, f := range spec.Optional {
		optional[f] = true
	}

	// Membership: whitelist if given, otherwise all root fields, minus excludes.
	var names []string
	if len(spec.Include) > 0 {
		names = spec.Include
	} else {
		names = make([]string, 0, len(byName))
		for n := range byName {
			names = append(names, n)
		}
		sort.Strings(names)
	}

	result := make(map[string]FieldDef, len(names))
	for _, n := range names {
		if excluded[n] {
			continue
		}
		fd := byName[n]
		if required[n] {
			fd.Required = true
		}
		if optional[n] {
			fd.Required = false
		}
		result[n] = fd
	}

	// required/optional fields must survive the membership filter.
	for _, f := range spec.Required {
		if _, ok := result[f]; !ok {
			return nil, nil, fmt.Errorf("projection %s: required field %q is not part of the projection", name, f)
		}
	}
	for _, f := range spec.Optional {
		if _, ok := result[f]; !ok {
			return nil, nil, fmt.Errorf("projection %s: optional field %q is not part of the projection", name, f)
		}
	}

	// Tagged fields must survive the membership filter.
	for f := range spec.Tags {
		if _, ok := result[f]; !ok {
			return nil, nil, fmt.Errorf("projection %s: tags reference field %q which is not part of the projection", name, f)
		}
	}

	return result, spec.Tags, nil
}

// applyProjectionTags appends the custom tags declared on a projection to the
// matching generated struct fields. Placeholders in tag values are expanded
// from the field's resolved properties.
func applyProjectionTags(fields []StructField, defs map[string]FieldDef, tags map[string]map[string]string) error {
	if len(tags) == 0 {
		return nil
	}

	goToName := make(map[string]string, len(defs))
	for schemaName := range defs {
		switch schemaName {
		case "_id_":
			goToName["ID"] = schemaName
			goToName["ModelID"] = schemaName
		case "_metadata_":
			goToName["Metadata"] = schemaName
			goToName["ModelMetadata"] = schemaName
		default:
			goToName[toCamelCase(schemaName)] = schemaName
		}
	}

	for i := range fields {
		sf := &fields[i]
		schemaName, ok := goToName[sf.Name]
		if !ok {
			continue
		}
		fieldTags, ok := tags[schemaName]
		if !ok {
			continue
		}
		fd := defs[schemaName]

		var parts []string
		for _, key := range sortedKeys(fieldTags) {
			value, err := expandFieldTemplate(fieldTags[key], fd, schemaName)
			if err != nil {
				return fmt.Errorf("projection tag %q on field %q: %w", key, schemaName, err)
			}
			parts = append(parts, fmt.Sprintf(`%s:%q`, key, value))
		}
		if sf.Tags != "" {
			sf.Tags += " " + strings.Join(parts, " ")
		} else {
			sf.Tags = strings.Join(parts, " ")
		}
	}
	return nil
}

// expandFieldTemplate substitutes {prop} placeholders with the field's
// resolved properties. Unknown properties are an error so typos surface at
// codegen time rather than silently producing malformed tags.
func expandFieldTemplate(tmpl string, fd FieldDef, schemaName string) (string, error) {
	if !strings.Contains(tmpl, "{") {
		return tmpl, nil
	}

	props := map[string]string{
		"name":     schemaName,
		"type":     fd.Type,
		"required": strconv.FormatBool(fd.Required),
		"nullable": strconv.FormatBool(fd.Nullable),
		"goName":   toCamelCase(schemaName),
		"default":  "",
	}
	if fd.Default != nil {
		props["default"] = strings.Trim(formatLiteral(fd.Default), `"`)
	}

	tokenRe := regexp.MustCompile(`\{([a-zA-Z0-9_]+)\}`)
	var firstErr error
	out := tokenRe.ReplaceAllStringFunc(tmpl, func(m string) string {
		prop := m[1 : len(m)-1]
		v, ok := props[prop]
		if !ok {
			if firstErr == nil {
				firstErr = fmt.Errorf("unknown field property {%s}", prop)
			}
			return m
		}
		return v
	})
	return out, firstErr
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

func resolveInlineTypeDescriptor(desc map[string]any, parent, field string, typeNames map[string]string, schemaInfos map[string]*SchemaInfo, structs map[string][]StructField, typeAliases map[string]string, enumDefs map[string]EnumDef, inlineNames map[string]string, genInlineName func(string, string, string) string, parentName string) (string, error) {
	typ := getString(desc, "type")
	if typ == "" {
		return "", fmt.Errorf("inline descriptor missing 'type'")
	}
	values, hasValues := desc["values"].([]any)

	if hasValues && len(values) > 0 {
		enumName := genInlineName(parent, field, "enum")
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

func resolveTypeMode(schemaType string, schemaRef any, typeNames map[string]string, schemaInfos map[string]*SchemaInfo, structs map[string][]StructField, typeAliases map[string]string, enumDefs map[string]EnumDef, inlineNames map[string]string, genInlineName func(string, string, string) string, parentName string) (string, error) {
	switch schemaType {
	case "array":
		if schemaRef == nil {
			return "", fmt.Errorf("array type missing schema reference")
		}
		elemType, err := resolveReferenceOrInline(schemaRef, "", "", typeNames, schemaInfos, structs, typeAliases, enumDefs, inlineNames, genInlineName, parentName)
		if err != nil {
			return "", err
		}
		return "[]" + elemType, nil
	case "record":
		if schemaRef == nil {
			return "map[string]any", nil
		}
		valType, err := resolveReferenceOrInline(schemaRef, "", "", typeNames, schemaInfos, structs, typeAliases, enumDefs, inlineNames, genInlineName, parentName)
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

func resolveReferenceOrInline(ref any, parent, field string, typeNames map[string]string, schemaInfos map[string]*SchemaInfo, structs map[string][]StructField, typeAliases map[string]string, enumDefs map[string]EnumDef, inlineNames map[string]string, genInlineName func(string, string, string) string, parentName string) (string, error) {
	if ref == nil {
		return "", nil
	}
	switch v := ref.(type) {
	case map[string]any:
		if _, hasID := v["id"]; hasID {
			return resolveSchemaReference(v, typeNames, schemaInfos)
		}
		if _, hasType := v["type"]; hasType {
			return resolveInlineTypeDescriptor(v, parent, field, typeNames, schemaInfos, structs, typeAliases, enumDefs, inlineNames, genInlineName, parentName)
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

func generateFields(fields map[string]FieldDef, typeNames map[string]string, schemaInfos map[string]*SchemaInfo, structs map[string][]StructField, typeAliases map[string]string, enumDefs map[string]EnumDef, inlineNames map[string]string, genInlineName func(string, string, string) string, parentName string, tagConfig TagConfig) ([]StructField, string, error) {
	var result []StructField

	// Iterate deterministically: field names ascending, falling back to the
	// map key (the field ID) as a tie-breaker for duplicate names.
	type fieldEntry struct {
		id string
		fd FieldDef
	}
	entries := make([]fieldEntry, 0, len(fields))
	for id, fd := range fields {
		entries = append(entries, fieldEntry{id: id, fd: fd})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].fd.Name != entries[j].fd.Name {
			return entries[i].fd.Name < entries[j].fd.Name
		}
		return entries[i].id < entries[j].id
	})

	// Resolve candidate Go names and detect collisions before emitting.
	// System fields (_id_, _metadata_) claim canonical names (ID, Metadata);
	// when a regular field already holds a canonical name (e.g. a schema field
	// named "id" or "metadata"), the system field yields and is renamed
	// ModelID/ModelMetadata, mirroring the pre-normalize shadow fallback.
	// Any remaining duplicate regular-field names are an error.
	names := make([]string, len(entries))
	used := make(map[string]int, len(entries))
	for i, e := range entries {
		goName := toCamelCase(e.fd.Name)
		switch e.fd.Name {
		case "_id_":
			goName = "ID"
		case "_metadata_":
			goName = "Metadata"
		}
		names[i] = goName
		used[goName]++
	}

	resolveSystemName := func(canonical, modelName string) string {
		if used[canonical] <= 1 {
			return canonical
		}
		name := modelName
		for i := 2; used[name] > 0; i++ {
			name = fmt.Sprintf("%s%d", modelName, i)
		}
		return name
	}
	idName := resolveSystemName("ID", "ModelID")
	metaName := resolveSystemName("Metadata", "ModelMetadata")

	resolved := make([]string, len(entries))
	seen := make(map[string]bool, len(entries))
	idFieldName := ""
	for i, e := range entries {
		fd := e.fd
		goName := names[i]
		switch fd.Name {
		case "_id_":
			goName = idName
			idFieldName = goName
		case "_metadata_":
			goName = metaName
		default:
			if seen[goName] {
				return nil, "", fmt.Errorf("field %s.%s produces duplicate Go field name %q", parentName, fd.Name, goName)
			}
		}
		seen[goName] = true
		resolved[i] = goName
	}

	for i, e := range entries {
		fd := e.fd
		fieldName := fd.Name
		if fieldName == "" {
			continue
		}
		goFieldName := resolved[i]

		var goType string

		switch fd.Type {
		case "union":
			refs, ok := fd.SchemaRef.([]any)
			if !ok || len(refs) == 0 {
				return nil, "", fmt.Errorf("union field %s.%s: missing or invalid schema refs", parentName, fieldName)
			}
			containerName := genInlineName(parentName, fieldName, "union")
			containerFields := []StructField{}
			for _, r := range refs {
				rmap, ok := r.(map[string]any)
				if !ok {
					return nil, "", fmt.Errorf("union ref is not a map")
				}
				refType, err := resolveSchemaReference(rmap, typeNames, schemaInfos)
				if err != nil {
					return nil, "", err
				}
				containerFields = append(containerFields, StructField{
					Name: refType,
					Type: "*" + refType,
					Tags: buildVariantTags(toSnakeCase(refType), tagConfig),
				})
			}
			structs[containerName] = containerFields
			goType = containerName

		case "composite":
			refs, ok := fd.SchemaRef.([]any)
			if !ok || len(refs) == 0 {
				return nil, "", fmt.Errorf("composite field %s.%s: missing or invalid schema refs", parentName, fieldName)
			}
			containerName := genInlineName(parentName, fieldName, "composite")
			containerFields := []StructField{}
			for _, r := range refs {
				rmap, ok := r.(map[string]any)
				if !ok {
					return nil, "", fmt.Errorf("composite ref is not a map")
				}
				refType, err := resolveSchemaReference(rmap, typeNames, schemaInfos)
				if err != nil {
					return nil, "", err
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
				return nil, "", fmt.Errorf("array field %s.%s missing schema reference", parentName, fieldName)
			}
			elemType, err := resolveReferenceOrInline(fd.SchemaRef, parentName, fieldName, typeNames, schemaInfos, structs, typeAliases, enumDefs, inlineNames, genInlineName, parentName)
			if err != nil {
				return nil, "", err
			}
			if elemType == "" {
				return nil, "", fmt.Errorf("array field %s.%s resolved to empty type", parentName, fieldName)
			}
			goType = "[]" + elemType

		case "record":
			if fd.SchemaRef == nil {
				goType = "map[string]any"
			} else {
				valType, err := resolveReferenceOrInline(fd.SchemaRef, parentName, fieldName, typeNames, schemaInfos, structs, typeAliases, enumDefs, inlineNames, genInlineName, parentName)
				if err != nil {
					return nil, "", err
				}
				if valType == "" {
					return nil, "", fmt.Errorf("record field %s.%s resolved to empty value type", parentName, fieldName)
				}
				goType = "map[string]" + valType
			}

		case "object":
			refMap, ok := fd.SchemaRef.(map[string]any)
			if !ok {
				return nil, "", fmt.Errorf("object field %s.%s missing schema reference", parentName, fieldName)
			}
			refType, err := resolveSchemaReference(refMap, typeNames, schemaInfos)
			if err != nil {
				return nil, "", err
			}
			goType = refType

		case "enum":
			if fd.SchemaRef == nil {
				return nil, "", fmt.Errorf("enum field %s.%s missing schema reference", parentName, fieldName)
			}
			enumType, err := resolveReferenceOrInline(fd.SchemaRef, parentName, fieldName, typeNames, schemaInfos, structs, typeAliases, enumDefs, inlineNames, genInlineName, parentName)
			if err != nil {
				return nil, "", err
			}
			goType = enumType

		default:
			goType = mapPrimitiveTypeToGo(fd.Type)
			if goType == "" {
				return nil, "", fmt.Errorf("unsupported field type %q for %s.%s", fd.Type, parentName, fieldName)
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

	return result, idFieldName, nil
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

// buildVariantTags constructs struct tags for union container variant fields.
// Variants are always optional (at most one is set), so omitempty is applied
// where configured. The anansi key is skipped: variant fields are not
// independently DTO-extracted — the containing field carries the type=union
// annotation.
func buildVariantTags(name string, tagConfig TagConfig) string {
	var parts []string
	for _, rule := range tagConfig {
		if rule.Key == "anansi" {
			continue
		}
		tagVal := name
		if rule.OmitEmpty {
			tagVal += ",omitempty"
		}
		parts = append(parts, fmt.Sprintf(`%s:"%s"`, rule.Key, tagVal))
	}
	return strings.Join(parts, " ")
}

// buildSchemaTagValue creates the value for the "anansi" tag with full metadata.
func buildSchemaTagValue(fd FieldDef, fieldName string) string {
	// Start with field name
	tagVal := fieldName

	// Required flag
	if fd.Required {
		tagVal += ",required=true"
	} else {
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
		tagVal += fmt.Sprintf(",default=%s", strings.Trim(defaultStr, `"`))
	}
	// Optional fields are omitempty so nil/zero values are skipped when a
	// struct is converted to a document for create (the binding layer's
	// structFieldValues honors omitempty in the anansi tag). Without it,
	// create of a nil optional object field fails conversion.
	if !fd.Required {
		tagVal += ",omitempty"
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
	case "document.DocumentModel":
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
