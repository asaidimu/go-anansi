package faker

import (
	"fmt"
	"math/rand"

	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
	"github.com/brianvoe/gofakeit/v7"
)

type FakeGenerator struct {
	schema   *definition.Schema
	rng      *rand.Rand
	maxDepth int
	seeded   bool
}

func NewFakeGenerator(schema *definition.Schema, seed int64) *FakeGenerator {
	return &FakeGenerator{
		schema:   schema,
		rng:      rand.New(rand.NewSource(seed)),
		maxDepth: 5,
	}
}

func (g *FakeGenerator) Generate() map[string]any {
	if !g.seeded {
		gofakeit.Seed(g.rng.Int63())
		g.seeded = true
	}
	return g.generateObject(g.schema.Fields, 0)
}

func (g *FakeGenerator) generateObject(fields map[definition.FieldId]definition.Field, depth int) map[string]any {
	out := make(map[string]any, len(fields))
	for _, f := range fields {
		out[string(f.Name)] = g.generateField(f, depth)
	}
	return out
}

func (g *FakeGenerator) generateField(f definition.Field, depth int) any {
	switch f.Type {
	case definition.FieldTypeString:
		return g.fakeString(f)
	case definition.FieldTypeNumber:
		return g.fakeNumber()
	case definition.FieldTypeInteger:
		return g.fakeInteger()
	case definition.FieldTypeDecimal:
		return g.fakeDecimal()
	case definition.FieldTypeBoolean:
		return gofakeit.Bool()
	case definition.FieldTypeBytes:
		return gofakeit.UUID()
	case definition.FieldTypeEnum:
		return g.fakeEnum(f)
	case definition.FieldTypeObject:
		return g.fakeObject(f, depth)
	case definition.FieldTypeArray:
		return g.fakeArray(f, depth)
	case definition.FieldTypeRecord:
		return g.fakeRecord(f, depth)
	case definition.FieldTypeUnion:
		return g.fakeUnion(f, depth)
	case definition.FieldTypeComposite:
		return g.fakeComposite(f, depth)
	case definition.FieldTypeGeometry:
		return g.fakeGeometry()
	default:
		return nil
	}
}

func (g *FakeGenerator) fakeString(f definition.Field) string {
	return g.fieldBasedString(string(f.Name))
}

func (g *FakeGenerator) fieldBasedString(name string) string {
	switch name {
	case "id", "ID", "uuid", "UUID", "uid", "UID":
		return gofakeit.UUID()
	case "email", "Email", "mail", "Mail":
		return gofakeit.Email()
	case "name", "Name", "full_name", "FullName":
		return gofakeit.Name()
	case "username", "Username", "user", "User", "handle", "Handle":
		return gofakeit.Username()
	case "first_name", "FirstName", "firstname", "first":
		return gofakeit.FirstName()
	case "last_name", "LastName", "lastname", "last", "surname", "Surname":
		return gofakeit.LastName()
	case "url", "URL", "uri", "URI", "link", "Link", "website", "Website":
		return gofakeit.URL()
	case "phone", "Phone", "tel", "Tel", "telephone", "Telephone", "mobile", "Mobile":
		return gofakeit.Phone()
	case "address", "Address", "street", "Street":
		return gofakeit.Street()
	case "city", "City":
		return gofakeit.City()
	case "state", "State", "province", "Province", "region", "Region":
		return gofakeit.State()
	case "country", "Country":
		return gofakeit.Country()
	case "zip", "Zip", "zipcode", "Zipcode", "postal_code", "PostalCode", "postcode":
		return gofakeit.Zip()
	case "description", "Description", "desc", "Desc", "summary", "Summary", "bio", "Bio":
		return gofakeit.Sentence(10)
	case "password", "Password", "token", "Token", "secret", "Secret":
		return gofakeit.Password(true, true, true, false, false, 24)
	case "color", "Color", "colour", "Colour", "hex", "Hex":
		return gofakeit.HexColor()
	case "ip", "IP", "ip_address", "IPAddress", "ipv4", "IPv4":
		return gofakeit.IPv4Address()
	case "ipv6", "IPv6":
		return gofakeit.IPv6Address()
	case "status", "Status":
		return g.randomChoice("active", "inactive", "pending", "archived")
	case "type", "Type", "kind", "Kind", "category", "Category":
		return g.randomChoice("standard", "premium", "basic", "enterprise")
	case "role", "Role":
		return g.randomChoice("admin", "user", "moderator", "viewer")
	case "lang", "Lang", "language", "Language", "locale", "Locale":
		return gofakeit.Language()
	case "job", "Job", "occupation", "Occupation", "title", "Title":
		return gofakeit.JobTitle()
	case "company", "Company", "org", "Org", "organization", "Organization":
		return gofakeit.Company()
	case "date", "Date", "created_at", "CreatedAt", "updated_at", "UpdatedAt":
		return gofakeit.Date().Format("2006-01-02T15:04:05Z")
	case "time", "Time", "timestamp", "Timestamp":
		return gofakeit.Date().Format("2006-01-02T15:04:05Z")
	case "avatar", "Avatar", "image", "Image", "img", "Img", "photo", "Photo":
		return gofakeit.URL() + "/img/" + gofakeit.UUID()
	case "slug", "Slug":
		return gofakeit.Username()
	default:
		return gofakeit.Word()
	}
}

func (g *FakeGenerator) fakeNumber() float64 {
	return gofakeit.Price(0.01, 9999.99)
}

func (g *FakeGenerator) fakeInteger() int {
	return gofakeit.Number(0, 99999)
}

func (g *FakeGenerator) fakeDecimal() float64 {
	return gofakeit.Price(0.01, 999999.99)
}

func (g *FakeGenerator) fakeEnum(f definition.Field) any {
	if f.Schema.IsZero() {
		return gofakeit.Word()
	}
	if !f.Schema.IsSingle() {
		return gofakeit.Word()
	}

	ref, err := definition.FieldSchemaAs[definition.SchemaReference](f.Schema)
	if err != nil {
		return gofakeit.Word()
	}

	if ref.IsInline() {
		if len(ref.Values) > 0 {
			return g.pickLiteral(ref.Values)
		}
		return gofakeit.Word()
	}

	ns, ok := g.schema.Schemas[ref.ID]
	if !ok || len(ns.Values) == 0 {
		return gofakeit.Word()
	}

	return g.pickLiteral(ns.Values)
}

func (g *FakeGenerator) fakeObject(f definition.Field, depth int) any {
	if depth >= g.maxDepth {
		return nil
	}
	if f.Schema.IsZero() || !f.Schema.IsSingle() {
		return nil
	}

	ref, err := definition.FieldSchemaAs[definition.SchemaReference](f.Schema)
	if err != nil {
		return nil
	}
	if ref.IsInline() {
		return nil
	}

	ns, ok := g.schema.Schemas[ref.ID]
	if !ok {
		return nil
	}

	return g.generateObject(ns.Fields, depth+1)
}

func (g *FakeGenerator) fakeArray(f definition.Field, depth int) any {
	if depth >= g.maxDepth {
		return nil
	}

	count := gofakeit.Number(1, 3)

	if f.Schema.IsZero() {
		out := make([]any, count)
		for i := range out {
			out[i] = gofakeit.Word()
		}
		return out
	}
	if !f.Schema.IsSingle() {
		out := make([]any, count)
		for i := range out {
			out[i] = gofakeit.Word()
		}
		return out
	}

	ref, err := definition.FieldSchemaAs[definition.SchemaReference](f.Schema)
	if err != nil {
		out := make([]any, count)
		for i := range out {
			out[i] = gofakeit.Word()
		}
		return out
	}

	itemType := g.resolveItemType(ref)
	out := make([]any, count)
	for i := range out {
		out[i] = g.generateFromType(itemType, depth+1)
	}
	return out
}

func (g *FakeGenerator) fakeRecord(f definition.Field, depth int) any {
	if depth >= g.maxDepth {
		return nil
	}

	count := gofakeit.Number(1, 3)
	out := make(map[string]any, count)

	if f.Schema.IsZero() || !f.Schema.IsSingle() {
		for i := 0; i < count; i++ {
			out[fmt.Sprintf("key_%d", i)] = gofakeit.Word()
		}
		return out
	}

	ref, err := definition.FieldSchemaAs[definition.SchemaReference](f.Schema)
	if err != nil {
		return out
	}

	itemType := g.resolveItemType(ref)
	for i := 0; i < count; i++ {
		out[gofakeit.Word()] = g.generateFromType(itemType, depth+1)
	}
	return out
}

func (g *FakeGenerator) fakeUnion(f definition.Field, depth int) any {
	if depth >= g.maxDepth {
		return nil
	}
	if f.Schema.IsZero() || !f.Schema.IsMultiple() {
		return nil
	}

	refs, err := definition.FieldSchemaAs[[]definition.SchemaReference](f.Schema)
	if err != nil || len(refs) == 0 {
		return nil
	}

	ref := refs[g.rng.Intn(len(refs))]
	if ref.IsInline() {
		return g.generateFromType(ref.Type, depth+1)
	}

	ns, ok := g.schema.Schemas[ref.ID]
	if !ok {
		return nil
	}

	return g.generateFromNested(ns, depth+1)
}

func (g *FakeGenerator) fakeComposite(f definition.Field, depth int) any {
	if depth >= g.maxDepth {
		return nil
	}
	if f.Schema.IsZero() || !f.Schema.IsMultiple() {
		return nil
	}

	refs, err := definition.FieldSchemaAs[[]definition.SchemaReference](f.Schema)
	if err != nil {
		return nil
	}

	merged := make(map[string]any)
	for _, ref := range refs {
		ns, ok := g.schema.Schemas[ref.ID]
		if !ok {
			continue
		}
		if obj := g.generateObject(ns.Fields, depth+1); obj != nil {
			for k, v := range obj {
				merged[k] = v
			}
		}
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}

func (g *FakeGenerator) fakeGeometry() any {
	return map[string]any{
		"type":        "Point",
		"coordinates": []float64{gofakeit.Longitude(), gofakeit.Latitude()},
	}
}

func (g *FakeGenerator) resolveItemType(ref definition.SchemaReference) definition.FieldType {
	if ref.IsInline() && ref.Type != 0 {
		return ref.Type
	}
	if ns, ok := g.schema.Schemas[ref.ID]; ok {
		if ns.Type != 0 {
			return ns.Type
		}
		if len(ns.Fields) > 0 {
			return definition.FieldTypeObject
		}
	}
	return definition.FieldTypeUnknown
}

func (g *FakeGenerator) generateFromType(t definition.FieldType, depth int) any {
	switch t {
	case definition.FieldTypeString:
		return gofakeit.Word()
	case definition.FieldTypeNumber:
		return g.fakeNumber()
	case definition.FieldTypeInteger:
		return g.fakeInteger()
	case definition.FieldTypeDecimal:
		return g.fakeDecimal()
	case definition.FieldTypeBoolean:
		return gofakeit.Bool()
	case definition.FieldTypeObject:
		return nil
	default:
		return gofakeit.Word()
	}
}

func (g *FakeGenerator) generateFromNested(ns definition.NestedSchema, depth int) any {
	if len(ns.Values) > 0 {
		return g.pickLiteral(ns.Values)
	}
	if len(ns.Fields) > 0 {
		return g.generateObject(ns.Fields, depth)
	}
	return nil
}

func (g *FakeGenerator) pickLiteral(values []definition.LiteralValue) any {
	if len(values) == 0 {
		return nil
	}
	lv := values[g.rng.Intn(len(values))]
	return lv.Value()
}

func (g *FakeGenerator) randomChoice(options ...string) string {
	return options[g.rng.Intn(len(options))]
}
