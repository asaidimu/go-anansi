package ir_test

import (
	"testing"

)

func TestIndexOrdinalResolution(t *testing.T) {
	cs := mustCompile(indexedSchema, nil)

	// indexedSchema:
	// root has name(0) and one index on name.
	// FrontSize = 1.
	// Index ordinal = FrontSize + 1 = 2.

	key := uint16(0)<<8 | uint16(0)
	ri := cs.ResolvedIndexes[key]

	if ri.Fields[0].DataPoint().ID() != 1 {
		t.Errorf("Index field ID: got %d, want 1", ri.Fields[0].DataPoint().ID())
	}
}
