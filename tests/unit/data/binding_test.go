package data_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStructBinder_To(t *testing.T) {
	type Address struct {
		Street string `doc:"street"`
		City   string `doc:"city"`
	}

	type User struct {
		ID        int       `doc:"user_id"`
		Name      string    `doc:"full_name,omitempty"`
		Active    bool      `doc:"is_active"`
		CreatedAt time.Time `doc:"created_at"`
		Address   Address   `doc:"address"`
		Tags      []string  `doc:"tags"`
	}

	doc, err := data.NewDocument(map[string]any{
		"user_id":    123,
		"full_name":  "John Doe",
		"is_active":  true,
		"created_at": "2023-10-27T10:00:00Z",
		"address": map[string]any{
			"street": "123 Main St",
			"city":   "Anytown",
		},
		"tags": []any{"go", "developer"},
	})
	require.NoError(t, err)

	var user User
	err = doc.BindTo(&user)
	require.NoError(t, err)

	require.Equal(t, 123, user.ID)
	require.Equal(t, "John Doe", user.Name)
	require.Equal(t, true, user.Active)
	require.Equal(t, "123 Main St", user.Address.Street)
	require.Equal(t, "Anytown", user.Address.City)
	require.Equal(t, []string{"go", "developer"}, user.Tags)

	// Test with context cancellation
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = doc.BindToWithContext(ctx, &user)
	require.Error(t, err)
}

func TestBindTo_Generic(t *testing.T) {
	type Product struct {
		SKU   string  `doc:"sku"`
		Price float64 `doc:"price"`
	}

	doc, err := data.NewDocument(map[string]any{
		"sku":   "ABC-123",
		"price": 99.99,
	})
	require.NoError(t, err)

	var product Product
	err = doc.BindTo(&product)
	require.NoError(t, err)
	require.Equal(t, "ABC-123", product.SKU)
	require.Equal(t, 99.99, product.Price)
}

func TestBindTo_PathTag(t *testing.T) {
	type Profile struct {
		DisplayName string `anansi:"payload.name"`
		Age         int    `anansi:"payload.age"`
	}

	doc := data.MustNewDocument(map[string]any{
		"payload": map[string]any{
			"name": "Alice",
			"age":  30,
		},
	})

	var profile Profile
	err := doc.BindTo(&profile)
	require.NoError(t, err)
	assert.Equal(t, "Alice", profile.DisplayName)
	assert.Equal(t, 30, profile.Age)
}

func TestNewDocumentFromStruct_PathTag(t *testing.T) {
	type Profile struct {
		DisplayName string `anansi:"payload.name"`
		Age         int    `anansi:"payload.age"`
	}

	profile := Profile{
		DisplayName: "Bob",
		Age:         25,
	}

	doc, err := data.NewDocumentFromStruct(profile)
	require.NoError(t, err)

	// Document should have nested structure
	payload, err := doc.Get("payload")
	require.NoError(t, err)
	payloadMap, ok := payload.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Bob", payloadMap["name"])
	assert.Equal(t, 25, payloadMap["age"])
}

func TestNewDocumentFromStruct_PathTag_Nested(t *testing.T) {
	type Config struct {
		Host string `anansi:"database.host"`
		Port int    `anansi:"database.port"`
	}

	config := Config{Host: "localhost", Port: 5432}

	doc, err := data.NewDocumentFromStruct(config)
	require.NoError(t, err)

	db, err := doc.Get("database")
	require.NoError(t, err)
	dbMap, ok := db.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "localhost", dbMap["host"])
	assert.Equal(t, 5432, dbMap["port"])
}

func TestRoundTrip_PathTag(t *testing.T) {
	type Server struct {
		Host string `anansi:"connection.host"`
		Port int    `anansi:"connection.port"`
	}

	original := Server{Host: "example.com", Port: 8080}

	doc, err := data.NewDocumentFromStruct(original)
	require.NoError(t, err)

	var restored Server
	err = doc.BindTo(&restored)
	require.NoError(t, err)

	assert.Equal(t, original.Host, restored.Host)
	assert.Equal(t, original.Port, restored.Port)
}

func TestBindTo_PathTag_MissingField(t *testing.T) {
	type Config struct {
		Host string `anansi:"database.host"`
		Port int    `anansi:"database.port"`
	}

	doc := data.MustNewDocument(map[string]any{
		"database": map[string]any{
			"host": "localhost",
		},
	})

	var cfg Config
	err := doc.BindTo(&cfg)
	require.NoError(t, err)
	assert.Equal(t, "localhost", cfg.Host)
	assert.Equal(t, 0, cfg.Port) // Port missing — left at zero value
}

func TestBindTo_PathTag_NonMapIntermediate(t *testing.T) {
	type Config struct {
		Host string `anansi:"database.host"`
	}

	doc := data.MustNewDocument(map[string]any{
		"database": "not_a_map",
	})

	var cfg Config
	err := doc.BindTo(&cfg)
	require.NoError(t, err)
	// Path can't be traversed — field stays at zero value
	assert.Empty(t, cfg.Host)
}

func TestBindTo_PathTag_DeepPath(t *testing.T) {
	type Deep struct {
		Value string `anansi:"a.b.c"`
	}

	doc := data.MustNewDocument(map[string]any{
		"a": map[string]any{
			"b": map[string]any{
				"c": "found",
			},
		},
	})

	var d Deep
	err := doc.BindTo(&d)
	require.NoError(t, err)
	assert.Equal(t, "found", d.Value)
}

func TestNewDocumentFromStruct_PathTag_DeepPath(t *testing.T) {
	type Deep struct {
		Value string `anansi:"x.y.z"`
	}

	d := Deep{Value: "deep_value"}
	doc, err := data.NewDocumentFromStruct(d)
	require.NoError(t, err)

	x, err := doc.Get("x")
	require.NoError(t, err)
	xMap, ok := x.(map[string]any)
	require.True(t, ok)
	yMap, ok := xMap["y"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "deep_value", yMap["z"])
}

func TestRoundTrip_DeepPath(t *testing.T) {
	type Deep struct {
		Value string `anansi:"x.y.z"`
		Num   int    `anansi:"x.y.num"`
	}

	original := Deep{Value: "test", Num: 42}

	doc, err := data.NewDocumentFromStruct(original)
	require.NoError(t, err)

	var restored Deep
	err = doc.BindTo(&restored)
	require.NoError(t, err)

	assert.Equal(t, original, restored)
}

func TestPathTag_WithDocumentModel(t *testing.T) {
	type User struct {
		data.DocumentModel
		Name  string `anansi:"profile.name"`
		Email string `anansi:"profile.email"`
	}

	user := data.New(&User{Name: "Charlie", Email: "charlie@example.com"})

	doc, err := user.Document()
	require.NoError(t, err)

	// Bind back
	var restored User
	err = doc.BindTo(&restored)
	require.NoError(t, err)
	assert.Equal(t, "Charlie", restored.Name)
	assert.Equal(t, "charlie@example.com", restored.Email)
}

func TestPathTag_OmitEmpty(t *testing.T) {
	type Config struct {
		Host string `anansi:"db.host,omitempty"`
		Port int    `anansi:"db.port"`
	}

	config := Config{Host: "", Port: 0}

	doc, err := data.NewDocumentFromStruct(config)
	require.NoError(t, err)

	// Port is zero but has no omitempty — should be present
	// Host is empty and has omitempty — should be absent
	db, err := doc.Get("db")
	require.NoError(t, err)
	dbMap, ok := db.(map[string]any)
	require.True(t, ok)

	_, hasHost := dbMap["host"]
	assert.False(t, hasHost, "omitempty field should be omitted")

	port, hasPort := dbMap["port"]
	assert.True(t, hasPort, "non-omitempty field should be present")
	assert.Equal(t, 0, port)
}

func TestSchemaFrom_PathTag(t *testing.T) {
	type Profile struct {
		Name string `anansi:"payload.name"`
		Age  int    `anansi:"payload.age"`
	}

	schemaJSON, err := data.SchemaFrom[Profile]()
	require.NoError(t, err)

	var schema map[string]any
	err = json.Unmarshal(schemaJSON, &schema)
	require.NoError(t, err)

	// Top-level should have a "payload" field of type "object"
	fields, ok := schema["fields"].(map[string]any)
	require.True(t, ok)

	var payloadField map[string]any
	for _, f := range fields {
		fm := f.(map[string]any)
		if fm["name"] == "payload" {
			payloadField = fm
			break
		}
	}
	require.NotNil(t, payloadField, "should have a 'payload' field")
	assert.Equal(t, "object", payloadField["type"])

	// payload field should reference a nested schema
	payloadSchema, ok := payloadField["schema"].(map[string]any)
	require.True(t, ok)
	payloadSchemaID, ok := payloadSchema["id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, payloadSchemaID)

	// The referenced schema should exist in schemas
	schemas, ok := schema["schemas"].(map[string]any)
	require.True(t, ok)

	payloadSchemaDef, ok := schemas[payloadSchemaID].(map[string]any)
	require.True(t, ok, "referenced schema must exist in schemas")
	assert.Equal(t, "payload", payloadSchemaDef["name"])

	// The nested schema should contain name and age fields
	nestedFields, ok := payloadSchemaDef["fields"].(map[string]any)
	require.True(t, ok)

	var nameFound, ageFound bool
	for _, f := range nestedFields {
		fm := f.(map[string]any)
		switch fm["name"] {
		case "name":
			nameFound = true
			assert.Equal(t, "string", fm["type"])
		case "age":
			ageFound = true
			assert.Equal(t, "integer", fm["type"])
		}
	}
	assert.True(t, nameFound, "should have 'name' field")
	assert.True(t, ageFound, "should have 'age' field")
}

func TestSchemaFrom_DeepPath(t *testing.T) {
	type Deep struct {
		Value string `anansi:"a.b.c,required"`
	}

	schemaJSON, err := data.SchemaFrom[Deep]()
	require.NoError(t, err)

	var schema map[string]any
	err = json.Unmarshal(schemaJSON, &schema)
	require.NoError(t, err)

	fields, ok := schema["fields"].(map[string]any)
	require.True(t, ok)

	// Top-level has "a" as object referencing synthetic schema
	var aField map[string]any
	for _, f := range fields {
		fm := f.(map[string]any)
		if fm["name"] == "a" {
			aField = fm
			break
		}
	}
	require.NotNil(t, aField)
	assert.Equal(t, "object", aField["type"])

	aSchema := aField["schema"].(map[string]any)
	aSchemaID := aSchema["id"].(string)

	schemas, ok := schema["schemas"].(map[string]any)
	require.True(t, ok)

	aSchemaDef := schemas[aSchemaID].(map[string]any)
	assert.Equal(t, "a", aSchemaDef["name"])

	// "a" schema has "b" as object
	aFields := aSchemaDef["fields"].(map[string]any)
	var bField map[string]any
	for _, f := range aFields {
		fm := f.(map[string]any)
		if fm["name"] == "b" {
			bField = fm
			break
		}
	}
	require.NotNil(t, bField)
	assert.Equal(t, "object", bField["type"])

	bSchema := bField["schema"].(map[string]any)
	bSchemaID := bSchema["id"].(string)

	bSchemaDef := schemas[bSchemaID].(map[string]any)
	assert.Equal(t, "b", bSchemaDef["name"])

	// "b" schema has "c" as string
	bFields := bSchemaDef["fields"].(map[string]any)
	var cField map[string]any
	for _, f := range bFields {
		fm := f.(map[string]any)
		if fm["name"] == "c" {
			cField = fm
			break
		}
	}
	require.NotNil(t, cField)
	assert.Equal(t, "string", cField["type"])
}

func TestSchemaFrom_PathTag_WithFlatField(t *testing.T) {
	type Mixed struct {
		Name    string `anansi:"name"`
		City    string `anansi:"address.city"`
		Country string `anansi:"address.country"`
	}

	schemaJSON, err := data.SchemaFrom[Mixed]()
	require.NoError(t, err)

	var schema map[string]any
	err = json.Unmarshal(schemaJSON, &schema)
	require.NoError(t, err)

	fields, ok := schema["fields"].(map[string]any)
	require.True(t, ok)

	var nameField, addressField map[string]any
	for _, f := range fields {
		fm := f.(map[string]any)
		switch fm["name"] {
		case "name":
			nameField = fm
		case "address":
			addressField = fm
		}
	}
	require.NotNil(t, nameField, "flat 'name' field should exist")
	assert.Equal(t, "string", nameField["type"])

	require.NotNil(t, addressField, "dotted 'address.city' should produce 'address' field")
	assert.Equal(t, "object", addressField["type"])
}

type Contact struct {
	PrimaryAddress string `anansi:"primary.address"`
	PrimaryEmail   string `anansi:"primary.email"`
}

type UserWithNestedPath struct {
	Contact Contact `anansi:"contact"`
}

func TestBindTo_NestedStructWithPathTags(t *testing.T) {
	doc := data.MustNewDocument(map[string]any{
		"contact": map[string]any{
			"primary": map[string]any{
				"address": "123 Main St",
				"email":   "user@example.com",
			},
		},
	})

	var u UserWithNestedPath
	err := doc.BindTo(&u)
	require.NoError(t, err)
	assert.Equal(t, "123 Main St", u.Contact.PrimaryAddress)
	assert.Equal(t, "user@example.com", u.Contact.PrimaryEmail)
}

func TestNewDocumentFromStruct_NestedStructWithPathTags(t *testing.T) {
	u := UserWithNestedPath{
		Contact: Contact{
			PrimaryAddress: "456 Oak Ave",
			PrimaryEmail:   "test@example.com",
		},
	}

	doc, err := data.NewDocumentFromStruct(u)
	require.NoError(t, err)

	contact, err := doc.Get("contact")
	require.NoError(t, err)
	contactMap, ok := contact.(map[string]any)
	require.True(t, ok)

	primary, ok := contactMap["primary"].(map[string]any)
	require.True(t, ok, "contact should have nested 'primary' map")
	assert.Equal(t, "456 Oak Ave", primary["address"])
	assert.Equal(t, "test@example.com", primary["email"])
}

func TestRoundTrip_NestedStructWithPathTags(t *testing.T) {
	original := UserWithNestedPath{
		Contact: Contact{
			PrimaryAddress: "789 Pine St",
			PrimaryEmail:   "roundtrip@example.com",
		},
	}

	doc, err := data.NewDocumentFromStruct(original)
	require.NoError(t, err)

	var restored UserWithNestedPath
	err = doc.BindTo(&restored)
	require.NoError(t, err)

	assert.Equal(t, original.Contact.PrimaryAddress, restored.Contact.PrimaryAddress)
	assert.Equal(t, original.Contact.PrimaryEmail, restored.Contact.PrimaryEmail)
}

func TestSchemaFrom_NestedStructWithPathTags(t *testing.T) {
	schemaJSON, err := data.SchemaFrom[UserWithNestedPath]()
	require.NoError(t, err)

	var schema map[string]any
	err = json.Unmarshal(schemaJSON, &schema)
	require.NoError(t, err)

	fields, ok := schema["fields"].(map[string]any)
	require.True(t, ok)

	// Top-level: "contact" is an object
	var contactField map[string]any
	for _, f := range fields {
		fm := f.(map[string]any)
		if fm["name"] == "contact" {
			contactField = fm
			break
		}
	}
	require.NotNil(t, contactField)
	assert.Equal(t, "object", contactField["type"])

	contactSchema := contactField["schema"].(map[string]any)
	contactSchemaID := contactSchema["id"].(string)

	schemas, ok := schema["schemas"].(map[string]any)
	require.True(t, ok)

	contactSchemaDef := schemas[contactSchemaID].(map[string]any)
	assert.Equal(t, "Contact", contactSchemaDef["name"])

	// Contact schema has "primary" as object
	contactFields := contactSchemaDef["fields"].(map[string]any)
	var primaryField map[string]any
	for _, f := range contactFields {
		fm := f.(map[string]any)
		if fm["name"] == "primary" {
			primaryField = fm
			break
		}
	}
	require.NotNil(t, primaryField, "Contact schema should have 'primary' field")
	assert.Equal(t, "object", primaryField["type"])

	primarySchema := primaryField["schema"].(map[string]any)
	primarySchemaID := primarySchema["id"].(string)

	primarySchemaDef := schemas[primarySchemaID].(map[string]any)
	assert.Equal(t, "primary", primarySchemaDef["name"])

	// Primary schema has "address" and "email"
	primaryFields := primarySchemaDef["fields"].(map[string]any)
	var addressFound, emailFound bool
	for _, f := range primaryFields {
		fm := f.(map[string]any)
		switch fm["name"] {
		case "address":
			addressFound = true
			assert.Equal(t, "string", fm["type"])
		case "email":
			emailFound = true
			assert.Equal(t, "string", fm["type"])
		}
	}
	assert.True(t, addressFound, "primary schema should have 'address'")
	assert.True(t, emailFound, "primary schema should have 'email'")
}

func TestBindTo_SetsParentForDocument(t *testing.T) {
	type Item struct {
		data.DocumentModel
		Name string `anansi:"name"`
	}

	doc := data.MustNewDocument(map[string]any{
		"name": "Widget",
	})

	var item Item
	err := doc.BindTo(&item)
	require.NoError(t, err)
	assert.Equal(t, "Widget", item.Name)

	// Document() should work because parent is set
	docOut, err := item.Document()
	require.NoError(t, err)
	assert.Equal(t, doc.ID(), docOut.ID())

	name, err := docOut.Get("name")
	require.NoError(t, err)
	assert.Equal(t, "Widget", name)
}

func TestBindTo_SetsParentForPatch(t *testing.T) {
	type Item struct {
		data.DocumentModel
		Name  string `anansi:"name"`
		Price int    `anansi:"price,omitempty"`
	}

	doc := data.MustNewDocument(map[string]any{
		"name": "Widget",
		"price": 100,
	})

	var item Item
	err := doc.BindTo(&item)
	require.NoError(t, err)

	// Patch() should work because parent is set
	patch, err := item.Patch()
	require.NoError(t, err)

	// Patch should contain the fields (zero-valued Price is omitted by omitempty)
	pName, err := patch.Get("name")
	require.NoError(t, err)
	assert.Equal(t, "Widget", pName)
}

func TestBindTo_SetsParentOnNestedStruct(t *testing.T) {
	type Address struct {
		data.DocumentModel
		Street string `anansi:"street"`
	}

	type User struct {
		data.DocumentModel
		Name    string  `anansi:"name"`
		Address Address `anansi:"address"`
	}

	doc := data.MustNewDocument(map[string]any{
		"name": "Alice",
		"address": map[string]any{
			"street": "123 Main St",
		},
	})

	var user User
	err := doc.BindTo(&user)
	require.NoError(t, err)

	// Top-level Document() works
	userDoc, err := user.Document()
	require.NoError(t, err)
	assert.Equal(t, doc.ID(), userDoc.ID())

	// Nested Address Document() works because parent was set recursively
	addrDoc, err := user.Address.Document()
	require.NoError(t, err)
	street, err := addrDoc.Get("street")
	require.NoError(t, err)
	assert.Equal(t, "123 Main St", street)
}
