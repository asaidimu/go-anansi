package ir_test

import (
	"fmt"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/schema/ir"
)

func BenchmarkCompile_Flat(b *testing.B) {
	ss := mustParse(flatSchema)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ir.Compile(ss, nil)
	}
}

func BenchmarkCompile_Nested(b *testing.B) {
	ss := mustParse(nestedObjectSchema)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ir.Compile(ss, nil)
	}
}

func BenchmarkCompile_DeepCycle(b *testing.B) {
	ss := mustParse(complexCycleSchema)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ir.Compile(ss, nil)
	}
}

func BenchmarkAddress_Flat(b *testing.B) {
	cs := mustCompile(flatSchema, nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cs.Address("name")
		_, _ = cs.Address("desc")
		_, _ = cs.Address("version")
	}
}

func BenchmarkAddress_Nested(b *testing.B) {
	cs := mustCompile(nestedObjectSchema, nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cs.Address("address.street")
		_, _ = cs.Address("address.city")
	}
}

func BenchmarkAddress_DeepCycle(b *testing.B) {
	cs := mustCompile(complexCycleSchema, nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cs.Address("start.next.next.value")
	}
}

func BenchmarkFullWalk_Nested(b *testing.B) {
	cs := mustCompile(nestedObjectSchema, nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ir.FullWalk(cs, 0, func(fd uint32) {})
	}
}

func BenchmarkTerminalWalk_Nested(b *testing.B) {
	cs := mustCompile(nestedObjectSchema, nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ir.TerminalWalk(cs, 0, func(fd uint32) {})
	}
}

func BenchmarkCompile_Large(b *testing.B) {
	src := generateLargeSchema(100, 10)
	ss, err := ir.Parse(src)
	if err != nil {
		b.Fatalf("Parse failed: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ir.Compile(ss, nil)
	}
}

func generateLargeSchema(numFields, numNestedSchemas int) []byte {
	fields := ""
	for i := 0; i < numFields; i++ {
		fields += fmt.Sprintf(`"019ca000-0000-7000-8000-%012x": { "name": "field%d", "type": "string" },`, i, i)
	}
	// Remove trailing comma
	if len(fields) > 0 {
		fields = fields[:len(fields)-1]
	}

	schemas := ""
	for i := 0; i < numNestedSchemas; i++ {
		nestedFields := ""
		for j := 0; j < 10; j++ {
			nestedFields += fmt.Sprintf(`"019ca000-0000-7000-9000-%06x%06x": { "name": "n%dfield%d", "type": "integer" },`, i, j, i, j)
		}
		if len(nestedFields) > 0 {
			nestedFields = nestedFields[:len(nestedFields)-1]
		}
		schemas += fmt.Sprintf(`"019ca000-0000-7000-a000-%012x": { "name": "Schema%d", "fields": { %s } },`, i, i, nestedFields)
	}
	if len(schemas) > 0 {
		schemas = schemas[:len(schemas)-1]
	}

	return []byte(fmt.Sprintf(`{
		"name": "LargeSchema",
		"version": "1.0.0",
		"fields": { %s },
		"schemas": { %s }
	}`, fields, schemas))
}
