package main

import (
	"fmt"

	. "github.com/asaidimu/go-anansi/v6/core/document"
)

func main() {
	// Smoke test: DataPoint round-trip
	p, err := NewDataPoint(TypeInt, 42)
	if err != nil {
		panic(err)
	}
	if p.Type() != TypeInt || p.ID() != 42 || p.IsNull() {
		panic("DataPoint round-trip failed")
	}

	// Smoke test: TypeRecord
	rp, _ := NewDataPoint(TypeRecord, 1)
	dc := NewDataContainer()
	rec := map[string]*DataContainer{
		"alice": NewDataContainer(),
	}
	if err := dc.SetRecord(rp, rec); err != nil {
		panic(err)
	}
	got, set, err := dc.GetRecord(rp)
	if err != nil || !set || len(got) != 1 {
		panic("TypeRecord round-trip failed")
	}

	// Smoke test: enum stored as TypeInt ordinal
	ep, _ := NewDataPoint(TypeInt, 99)
	if err := dc.SetInt(ep, 2); err != nil { // ordinal 2 = "archived"
		panic(err)
	}
	oval, _, _ := dc.GetInt(ep)
	if oval != 2 {
		panic("enum ordinal round-trip failed")
	}

	// Smoke test: Clear resets without deallocation
	dc.Clear()
	if dc.Length() != 0 {
		panic("Clear failed")
	}

	// Smoke test: null semantics
	np, _ := NewDataPoint(TypeString, 7)
	doc := NewDocument()
	doc.SetString(np, "hello")
	doc.SetNull(np)
	if !doc.IsNull(np) || doc.HasValue(np) {
		panic("null semantics failed")
	}
	doc.Unset(np)
	if doc.IsSet(np) {
		panic("unset failed")
	}

	fmt.Println("all checks passed")
}


