package schemagen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
	"github.com/asaidimu/go-anansi/v8/core/schema/meta"
)

// metadataFile is the declarative source of truth for user-defined metadata.
// Provider field and dependency-schema keys may be names or canonical UUID v7
// IDs; canonicalizeMetadata assigns stable UUID v7 IDs to name keys.
type metadataFile struct {
	Name      string                         `json:"name,omitempty"`
	Providers map[string]metadataProviderDef `json:"providers,omitempty"`
	Schemas   map[string]definition.NestedSchema `json:"schemas,omitempty"`
}

type metadataProviderDef struct {
	Description string                      `json:"description,omitempty"`
	Fields      map[string]definition.Field `json:"fields"`
}

// Metadata is the parsed, canonicalized form of the metadata file.
type Metadata struct {
	Name      string
	Providers []MetadataProvider          // sorted by name
	Deps      []*definition.NestedSchema // canonical dependency schemas, sorted by name
}

type MetadataProvider struct {
	Name        string
	Description string
	Fields      map[definition.FieldId]definition.Field
	Schema      *definition.NestedSchema
	Ident       string // Go identifier derived from Name
}

// exportedIdent converts a provider name into an exported Go identifier, e.g.
// "my_provider" -> "My_provider", "trace" -> "Trace".
func exportedIdent(name string) string {
	ident := SafeIdent(name)
	if ident == "" {
		return ident
	}
	r := []rune(ident)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// LoadMetadata parses, canonicalizes, and validates the declarative metadata
// file. The returned Metadata is a pure function of the file's content, so it
// can be used to enrich schemas in a stable (idempotent) way. When the file is
// absent, an empty Metadata carrying only the base schema name is returned.
// The second return value reports whether the canonical form differs from the
// on-disk bytes (the caller writes the canonical form back when !dryRun).
func LoadMetadata(path string) (*Metadata, bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Metadata{Name: data.MetadataField}, false, nil
		}
		return nil, false, fmt.Errorf("read metadata %s: %w", path, err)
	}

	var mf metadataFile
	if err := json.Unmarshal(raw, &mf); err != nil {
		return nil, false, fmt.Errorf("parse metadata %s: %w", path, err)
	}

	if err := canonicalizeMetadata(&mf); err != nil {
		return nil, false, err
	}

	md := &Metadata{Name: mf.Name}
	for _, pname := range sortedKeys(mf.Providers) {
		p := mf.Providers[pname]
		fields := make(map[definition.FieldId]definition.Field, len(p.Fields))
		for fid, f := range p.Fields {
			fields[definition.FieldId(fid)] = f
		}
		md.Providers = append(md.Providers, MetadataProvider{
			Name:        pname,
			Description: p.Description,
			Fields:      fields,
			Schema: &definition.NestedSchema{
				BaseSchema: definition.BaseSchema{Name: pname, Fields: fields},
			},
			Ident: exportedIdent(pname),
		})
	}
	for _, sname := range sortedKeys(mf.Schemas) {
		ns := mf.Schemas[sname]
		md.Deps = append(md.Deps, &ns)
	}

	if err := validateMetadata(md); err != nil {
		return nil, false, err
	}

	out, err := json.MarshalIndent(mf, "", "  ")
	if err != nil {
		return nil, false, fmt.Errorf("marshal metadata %s: %w", path, err)
	}
	out = append(out, '\n')

	return md, !bytes.Equal(out, raw), nil
}

// canonicalizeMetadata rewrites mf in place into its canonical form: field IDs
// and dependency-schema keys are UUID v7 (name keys are assigned stable IDs),
// and schema references point at dependency names (the form the persistence
// registry keys dependencies by at runtime).
func canonicalizeMetadata(mf *metadataFile) error {
	if mf.Name == "" {
		mf.Name = data.MetadataField
	}
	if mf.Name != data.MetadataField {
		return fmt.Errorf("metadata: file name must be %q, got %q", data.MetadataField, mf.Name)
	}

	// Build a synthetic schema combining every provider's fields and every
	// declared dependency. meta.NormalizeSchema canonicalizes name-keyed fields
	// and dependency inner fields to UUID v7 and rewrites schema references,
	// mirroring how schema files are normalized.
	synthetic := &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Fields: make(map[definition.FieldId]definition.Field),
		},
		Schemas: make(map[definition.SchemaId]definition.NestedSchema),
	}

	owners := make(map[string]string) // field name -> provider name
	for _, pname := range sortedKeys(mf.Providers) {
		p := mf.Providers[pname]
		for _, fkey := range sortedKeys(p.Fields) {
			f := p.Fields[fkey]
			if f.Name == "" {
				f.Name = definition.FieldName(fkey)
			}
			if looksLikeUUID(fkey) && f.Name == definition.FieldName(fkey) {
				return fmt.Errorf("metadata: provider %q field %q has a UUID key but no name; declare a name", pname, fkey)
			}
			fname := string(f.Name)
			if owner, ok := owners[fname]; ok {
				return fmt.Errorf("metadata: duplicate field name %q declared in providers %q and %q", fname, owner, pname)
			}
			owners[fname] = pname
			synthetic.Fields[definition.FieldId(fkey)] = f
		}
	}

	for _, skey := range sortedKeys(mf.Schemas) {
		ns := mf.Schemas[skey]
		if ns.Name == "" {
			ns.Name = skey
		}
		if ns.Name == data.MetadataField {
			return fmt.Errorf("metadata: dependency schema cannot be named %q", data.MetadataField)
		}
		if ns.Name != skey {
			return fmt.Errorf("metadata: dependency schema key %q must match its name %q", skey, ns.Name)
		}
		synthetic.Schemas[definition.SchemaId(skey)] = ns
	}

	meta.NormalizeSchema(synthetic)

	// Partition the canonical fields back to their owning provider.
	canonProviders := make(map[string]metadataProviderDef, len(mf.Providers))
	for _, pname := range sortedKeys(mf.Providers) {
		canonProviders[pname] = metadataProviderDef{
			Description: mf.Providers[pname].Description,
			Fields:      make(map[string]definition.Field),
		}
	}
	for fid, f := range synthetic.Fields {
		pname, ok := owners[string(f.Name)]
		if !ok {
			return fmt.Errorf("metadata: internal error: field %q has no owning provider", f.Name)
		}
		p := canonProviders[pname]
		p.Fields[string(fid)] = f
		canonProviders[pname] = p
	}
	mf.Providers = canonProviders

	// Re-key dependency schemas by name (runtime convention) while keeping
	// their canonicalized inner fields, and point metadata field references at
	// the dependency names.
	canonSchemas := make(map[string]definition.NestedSchema, len(synthetic.Schemas))
	nameByID := make(map[string]string, len(synthetic.Schemas))
	for sid, ns := range synthetic.Schemas {
		nameByID[string(sid)] = ns.Name
		canonSchemas[ns.Name] = ns
	}
	mf.Schemas = canonSchemas

	for pname := range mf.Providers {
		p := mf.Providers[pname]
		for fid, f := range p.Fields {
			f = rewriteDepRefs(f, nameByID)
			p.Fields[fid] = f
		}
		mf.Providers[pname] = p
	}

	return nil
}

// rewriteDepRefs rewrites any single/multiple schema reference whose ID matches
// a dependency's canonical (transient) UUID back to the dependency name.
func rewriteDepRefs(f definition.Field, nameByID map[string]string) definition.Field {
	if f.Schema.IsZero() {
		return f
	}
	if f.Schema.IsSingle() {
		sr, _ := definition.FieldSchemaAs[definition.SchemaReference](f.Schema)
		if name, ok := nameByID[string(sr.ID)]; ok {
			sr.ID = definition.SchemaId(name)
			f.Schema = definition.NewSchemaReference(sr)
		}
	} else if f.Schema.IsMultiple() {
		refs, _ := definition.FieldSchemaAs[[]definition.SchemaReference](f.Schema)
		changed := false
		for i, sr := range refs {
			if name, ok := nameByID[string(sr.ID)]; ok {
				refs[i].ID = definition.SchemaId(name)
				changed = true
			}
		}
		if changed {
			f.Schema = definition.NewSchemaReference(refs)
		}
	}
	return f
}

func validateMetadata(md *Metadata) error {
	base := data.DefaultMetadataSchema()
	baseNames := make(map[string]bool, len(base.Fields))
	for _, f := range base.Fields {
		baseNames[string(f.Name)] = true
	}

	idents := make(map[string]string)
	for _, p := range md.Providers {
		if other, ok := idents[p.Ident]; ok {
			return fmt.Errorf("metadata: providers %q and %q both map to identifier %q; use distinct provider names", other, p.Name, p.Ident)
		}
		idents[p.Ident] = p.Name

		for fid, f := range p.Fields {
			if baseNames[string(f.Name)] {
				return fmt.Errorf("metadata: provider %q field %q uses reserved metadata field name %q", p.Name, f.Name, f.Name)
			}
			if _, clash := base.Fields[fid]; clash {
				return fmt.Errorf("metadata: provider %q field %q collides with a base metadata field ID", p.Name, f.Name)
			}
		}
	}

	seen := make(map[string]bool)
	for _, dep := range md.Deps {
		if seen[dep.Name] {
			return fmt.Errorf("metadata: duplicate dependency schema name %q", dep.Name)
		}
		seen[dep.Name] = true
	}
	return nil
}

// MergedSchema mirrors data.GetMetadataSchema: the base framework metadata
// fields plus every provider's declared fields.
func (m *Metadata) MergedSchema() *definition.NestedSchema {
	merged := data.DefaultMetadataSchema()
	if merged.Fields == nil {
		merged.Fields = make(map[definition.FieldId]definition.Field)
	}
	for _, p := range m.Providers {
		for fid, f := range p.Fields {
			merged.Fields[fid] = f
		}
	}
	return merged
}

// Dependencies returns the declared dependency schemas, deduplicated by name.
func (m *Metadata) Dependencies() []*definition.NestedSchema {
	seen := make(map[string]bool, len(m.Deps))
	deps := make([]*definition.NestedSchema, 0, len(m.Deps))
	for _, dep := range m.Deps {
		if !seen[dep.Name] {
			seen[dep.Name] = true
			deps = append(deps, dep)
		}
	}
	return deps
}

// writeMetadataFile writes the canonical form of m back to path (with a .bak
// backup), matching the canonical bytes LoadMetadata would produce.
func writeMetadataFile(path string, m *Metadata) error {
	mf := metadataFile{
		Name:      m.Name,
		Providers: make(map[string]metadataProviderDef, len(m.Providers)),
		Schemas:   make(map[string]definition.NestedSchema, len(m.Deps)),
	}
	for _, p := range m.Providers {
		fields := make(map[string]definition.Field, len(p.Fields))
		for fid, f := range p.Fields {
			fields[string(fid)] = f
		}
		mf.Providers[p.Name] = metadataProviderDef{Description: p.Description, Fields: fields}
	}
	for _, dep := range m.Deps {
		mf.Schemas[dep.Name] = *dep
	}
	out, err := json.MarshalIndent(mf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	out = append(out, '\n')

	if err := backupFile(path); err != nil {
		return fmt.Errorf("backup metadata %s: %w", path, err)
	}
	if err := os.WriteFile(path, out, 0644); err != nil {
		return fmt.Errorf("write metadata %s: %w", path, err)
	}
	return nil
}

// metadataFileExists reports whether the declarative metadata file is present.
func metadataFileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// metadataOutDir resolves the directory where generated metadata files land.
func metadataOutDir(cfg *Config) string {
	if cfg.Metadata.OutDir != "" {
		return cfg.Metadata.OutDir
	}
	return cfg.Schema.MigrationsDir
}

// GenerateMetadataFiles renders metadata.go (always) and providers.go (only
// when missing) into the configured output directory. It returns the list of
// files written.
func GenerateMetadataFiles(cfg *Config, m *Metadata, dryRun bool) ([]string, error) {
	outDir := metadataOutDir(cfg)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return nil, fmt.Errorf("create metadata output dir: %w", err)
	}
	pkg := SafeIdent(filepath.Base(strings.TrimRight(outDir, "/")))
	if pkg == "." || pkg == "" {
		pkg = "migrations"
	}

	var written []string
	metadataPath := filepath.Join(outDir, "metadata.go")
	if dryRun {
		fmt.Printf("  would generate: %s\n", metadataPath)
	} else if err := os.WriteFile(metadataPath, []byte(renderMetadataGo(pkg, m)), 0644); err != nil {
		return nil, fmt.Errorf("write %s: %w", metadataPath, err)
	} else {
		fmt.Printf("  generated: %s\n", metadataPath)
	}
	written = append(written, metadataPath)

	providersPath := filepath.Join(outDir, "providers.go")
	_, err := os.Stat(providersPath)
	exists := err == nil
	if dryRun && !exists {
		fmt.Printf("  would generate: %s\n", providersPath)
		written = append(written, providersPath)
		return written, nil
	}
	if exists {
		fmt.Printf("  skipped: %s (already exists, write-once)\n", providersPath)
		return written, nil
	}
	if err := os.WriteFile(providersPath, []byte(renderProvidersGo(pkg, m)), 0644); err != nil {
		return nil, fmt.Errorf("write %s: %w", providersPath, err)
	}
	fmt.Printf("  generated: %s\n", providersPath)
	written = append(written, providersPath)
	return written, nil
}

func renderMetadataGo(pkg string, m *Metadata) string {
	mergedJSON, err := json.MarshalIndent(m.MergedSchema(), "", "  ")
	if err != nil {
		panic(err)
	}
	depsJSON, err := json.MarshalIndent(m.Dependencies(), "", "  ")
	if err != nil {
		panic(err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "// Code generated by anansi. DO NOT EDIT.\n// Source: metadata.schema.json\n\npackage %s\n\n", pkg)
	fmt.Fprintf(&b, `import (
	"encoding/json"
	"fmt"

	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

var metadata_schema_json = %s

// MetadataSchema returns the merged _metadata_ schema: the framework's base
// fields plus every field declared in metadata.schema.json.
func MetadataSchema() *definition.NestedSchema {
	var ns definition.NestedSchema
	if err := json.Unmarshal(metadata_schema_json, &ns); err != nil {
		panic("invalid embedded metadata schema: " + err.Error())
	}
	return &ns
}

var metadata_dependencies_json = %s

// MetadataDependencies returns the dependency nested schemas referenced by
// metadata fields, deduplicated by name.
func MetadataDependencies() []*definition.NestedSchema {
	var deps []*definition.NestedSchema
	if err := json.Unmarshal(metadata_dependencies_json, &deps); err != nil {
		panic("invalid embedded metadata dependencies: " + err.Error())
	}
	return deps
}
`, jsonLiteral(mergedJSON), jsonLiteral(depsJSON))

	for _, p := range m.Providers {
		pj, err := json.MarshalIndent(p.Schema, "", "  ")
		if err != nil {
			panic(err)
		}
		fmt.Fprintf(&b, "var %s_schema_json = %s\n\n", strings.ToLower(p.Ident), jsonLiteral(pj))
		fmt.Fprintf(&b, "// %sSchema returns the schema of the %q provider fields.\n", p.Ident, p.Name)
		fmt.Fprintf(&b, "func %sSchema() *definition.NestedSchema {\n", p.Ident)
		b.WriteString("\tvar ns definition.NestedSchema\n")
		fmt.Fprintf(&b, "\tif err := json.Unmarshal(%s_schema_json, &ns); err != nil {\n", strings.ToLower(p.Ident))
		b.WriteString("\t\tpanic(\"invalid embedded metadata schema: \" + err.Error())\n\t}\n")
		b.WriteString("\treturn &ns\n}\n\n")
	}

	b.WriteString("// ValidateMetadataSchema reports whether the runtime document factory's merged\n")
	b.WriteString("// metadata schema (data.GetMetadataSchema) matches the schema generated from\n")
	b.WriteString("// metadata.schema.json. Wire every generated provider schema into\n")
	b.WriteString("// DocumentFactoryConfig.Providers for them to agree.\n")
	b.WriteString("func ValidateMetadataSchema() error {\n")
	b.WriteString("\tgot, _ := data.GetMetadataSchema()\n")
	b.WriteString("\twant := MetadataSchema()\n")
	b.WriteString("\tgotFields := metadataFieldMap(got)\n")
	b.WriteString("\twantFields := metadataFieldMap(want)\n")
	b.WriteString("\tfor name, f := range wantFields {\n")
	b.WriteString("\t\trf, ok := gotFields[name]\n")
	b.WriteString("\t\tif !ok {\n")
	b.WriteString("\t\t\treturn fmt.Errorf(\"metadata: field %q declared in metadata.schema.json is missing from the runtime factory; wire the generated provider into DocumentFactoryConfig.Providers\", name)\n")
	b.WriteString("\t\t}\n")
	b.WriteString("\t\tif rf.Type != f.Type {\n")
	b.WriteString("\t\t\treturn fmt.Errorf(\"metadata: field %q type mismatch: runtime %s vs metadata.schema.json %s\", name, rf.Type, f.Type)\n")
	b.WriteString("\t\t}\n")
	b.WriteString("\t}\n")
	b.WriteString("\treturn nil\n")
	b.WriteString("}\n\n")
	b.WriteString("func metadataFieldMap(ns *definition.NestedSchema) map[string]definition.Field {\n")
	b.WriteString("\tout := make(map[string]definition.Field, len(ns.Fields))\n")
	b.WriteString("\tfor _, f := range ns.Fields {\n")
	b.WriteString("\t\tout[string(f.Name)] = f\n")
	b.WriteString("\t}\n")
	b.WriteString("\treturn out\n")
	b.WriteString("}\n")

	return b.String()
}

func renderProvidersGo(pkg string, m *Metadata) string {
	var b strings.Builder
	b.WriteString("// Code generated by anansi. DO NOT EDIT.\n")
	b.WriteString("// Source: metadata.schema.json\n")
	b.WriteString("// This file contains provider stubs. It is written once; edit the bodies to\n")
	b.WriteString("// produce the metadata declared for each provider.\n\n")
	fmt.Fprintf(&b, "package %s\n\n", pkg)
	b.WriteString(`import (
	"context"

	"github.com/asaidimu/go-anansi/v8/core/data"
)

`)
	for _, p := range m.Providers {
		var fieldNames []string
		for _, f := range p.Fields {
			fieldNames = append(fieldNames, string(f.Name))
		}
		sort.Strings(fieldNames)
		fmt.Fprintf(&b, "// %sProvider fills the %q metadata fields (%s).\n", p.Ident, p.Name, strings.Join(fieldNames, ", "))
		fmt.Fprintf(&b, "func %sProvider(ctx context.Context, doc data.Documenter) (map[string]any, error) {\n", p.Ident)
		b.WriteString("\t// TODO: implement\n")
		b.WriteString("\treturn nil, nil\n")
		b.WriteString("}\n\n")
	}
	return b.String()
}

func looksLikeUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < 36; i++ {
		c := s[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !isHexChar(c) {
				return false
			}
		}
	}
	return true
}

func isHexChar(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
