package ir

import (
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/document"
)

func TestConstraintEnforcement_Simulation(t *testing.T) {
	// 1. Define a Predicate that checks if a value is within a range.
	rangeCheck := func(data *document.DataContainer, fields []document.DataPoint, args any) bool {
		params := args.(map[string]any)
		min := params["min"].(float64)
		max := params["max"].(float64)

		for _, dp := range fields {
			// Retrieve the value from the DataContainer using the absolute DataPoint ID.
			val, ok, _ := data.GetInt(dp)
			if !ok {
				return false
			}
			if float64(val) < min || float64(val) > max {
				return false
			}
		}
		return true
	}

	pm := PredicateMap{
		"range": rangeCheck,
	}

	// 2. Define a schema with a nested object and a root constraint using absolute paths.
	src := []byte(`{
	  "name": "Inventory",
	  "version": "1.0.0",
	  "fields": {
	    "019ca000-0001-7001-8001-000000000001": {
	      "name": "warehouse",
	      "type": "object",
	      "schema": { "id": "019ca000-0010-7010-9010-000000000010" }
	    }
	  },
	  "schemas": {
	    "019ca000-0010-7010-9010-000000000010": {
	      "name": "Warehouse",
	      "fields": {
	        "019ca000-0011-7011-9011-000000000011": { "name": "stock_level", "type": "integer" }
	      }
	    }
	  },
	  "constraints": {
	    "019ca000-0020-7020-9020-000000000020": {
	      "name": "valid_stock",
	      "predicate": "range",
	      "fields": ["warehouse.stock_level"],
	      "parameters": { "min": 0, "max": 100 }
	    }
	  }
	}`)

	cs, err := Compile(mustParse(src), pm)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	// 3. Create a mock DataContainer and populate it with data.
	// We use the same Address() ordinals used by the storage engine.
	data := document.NewDataContainer()
	
	stockDP, _ := cs.Address("warehouse.stock_level")
	
	// Simulation Case A: Valid Data (stock = 50)
	data.SetInt(stockDP, 50)

	// Execute enforcement (simulating a Validator)
	rt := cs.ResolvedConstraints
	if rt == nil {
		t.Fatal("no resolved constraints found")
	}

	// For simulation, we just check the first root constraint
	rc := rt.Roots[0].(ResolvedConstraint)
	if !rc.Predicate(data, rc.Fields, rc.Parameters) {
		t.Errorf("Constraint failed for valid data (stock=50)")
	}

	// Simulation Case B: Invalid Data (stock = 150)
	data.SetInt(stockDP, 150)
	if rc.Predicate(data, rc.Fields, rc.Parameters) {
		t.Errorf("Constraint passed for invalid data (stock=150)")
	}
	
	// Verification: The field in the ResolvedConstraint must match the absolute path.
	if rc.Fields[0] != stockDP {
		t.Errorf("Resolved field DataPoint mismatch:\nGot:  %v\nWant: %v", rc.Fields[0], stockDP)
	}
}
