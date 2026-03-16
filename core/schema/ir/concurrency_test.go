package ir

import (
	"sync"
	"testing"
)

func TestAddress_Concurrency(t *testing.T) {
	cs := mustCompile(cycleSchema, nil)

	paths := []string{
		"label",
		"node",
		"node.value",
		"node.next",
		"node.next.value",
		"node.next.next",
		"node.next.next.value",
	}

	const (
		numGoroutines = 10
		numIterations = 100
	)

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				for _, path := range paths {
					dp, err := cs.Address(path)
					if err != nil {
						t.Errorf("[G%d] Address(%s) failed: %v", id, path, err)
						return
					}
					if dp == 0 {
						t.Errorf("[G%d] Address(%s) returned zero DataPoint", id, path)
						return
					}
				}
			}
		}(i)
	}

	wg.Wait()
}

func TestCompile_Concurrency(t *testing.T) {
	// Although IR is immutable after compilation, we should ensure the
	// compiler itself doesn't have shared global state (it shouldn't).
	ss := mustParse(nestedObjectSchema)

	const numGoroutines = 5
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			cs, err := Compile(ss, nil)
			if err != nil {
				t.Errorf("[G%d] Compile failed: %v", id, err)
				return
			}
			if len(cs.Descriptors) == 0 {
				t.Errorf("[G%d] CompiledSchema has no descriptors", id)
			}
		}(i)
	}

	wg.Wait()
}
