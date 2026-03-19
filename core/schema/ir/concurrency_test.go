package ir_test

import (
	"sync"
	"testing"

)

func TestAddress_ConcurrencySafe(t *testing.T) {
	cs := mustCompile(cycleSchema, nil)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_, _ = cs.Address("node.next.value")
			}
		}()
	}
	wg.Wait()
}
