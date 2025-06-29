# Testing Strategy for go-anansi

This document outlines the testing strategy for the `go-anansi` library, emphasizing interface-focused testing to ensure maintainable, robust test suites.

## Core Testing Philosophy

**Test interfaces, not implementation details.** Focus on what the code does (behavior) rather than how it does it (implementation details).

## Structured Testing Framework

Follow this systematic approach for all test development:

### Phase 1: Identify & Understand Modules/Interfaces

Before writing any tests, complete this analysis:

1. **Map Public Interfaces**
   - Identify all exported functions, methods, and types
   - Document expected inputs and outputs
   - Note any side effects (file operations, network calls, state changes)
   - Ignore private/unexported functions and internal data structures

2. **Define Interface Contracts**
   - What should happen with valid inputs?
   - How should invalid inputs be handled?
   - What are the observable behaviors from a caller's perspective?
   - What guarantees does this interface provide?

3. **Identify Dependencies**
   - What external dependencies does this interface rely on?
   - Which dependencies should be replaced with test doubles vs. real implementations?

### Phase 2: Write Minimal Interface Tests

Create the smallest possible tests that verify each interface works:

1. **Happy Path Test**
   ```go
   func TestComponentName_HappyPath(t *testing.T) {
       // Arrange: Set up minimal valid input
       // Act: Call the public interface
       // Assert: Verify expected output/behavior
   }
   ```

2. **Error Path Test**
   ```go
   func TestComponentName_InvalidInput(t *testing.T) {
       // Arrange: Set up invalid input
       // Act: Call the public interface
       // Assert: Verify appropriate error handling
   }
   ```

3. **Side Effect Test** (if applicable)
   ```go
   func TestComponentName_SideEffects(t *testing.T) {
       // Arrange: Set up observeable state
       // Act: Call the public interface
       // Assert: Verify observable state changes
   }
   ```

### Phase 3: Expand Tests as Needed

Only after minimal tests pass, expand based on:

1. **Edge Cases Discovery**
   - Boundary conditions (empty inputs, maximum values)
   - Concurrent access scenarios
   - Resource exhaustion scenarios

2. **Business Logic Complexity**
   - Multiple valid input combinations
   - Complex state transitions
   - Integration scenarios

## Testing Guidelines by Layer

### 1. Unit Tests

**DO:**
- Test public APIs only (exported functions/methods)
- Use table-driven tests for multiple input scenarios
- Mock external dependencies (databases, network, filesystem)
- Assert on return values and observable side effects
- Test error conditions through the public interface

**DON'T:**
- Mock interfaces for which there are existing implementations
- Test private functions directly
- Mock internal method calls within the same unit
- Assert on internal state that isn't exposed through the interface
- Test implementation-specific details (algorithm steps, internal data structures)

**Example Structure:**
```go
func TestCollection_Store(t *testing.T) {
    tests := []struct {
        name    string
        input   Document
        want    error
        setup   func(*testing.T) Collection
    }{
        {
            name:  "valid document",
            input: Document{ID: "test", Content: "data"},
            want:  nil,
            setup: func(t *testing.T) Collection { return NewInMemoryCollection() },
        },
        // Additional test cases...
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            collection := tt.setup(t)
            got := collection.Store(tt.input)
            assert.Equal(t, tt.want, got)
        })
    }
}
```

### 2. Integration Tests

**Focus:** Test that modules work correctly when combined, using real implementations where practical.

**Structure:**
1. **Identify Integration Boundaries**
   - Where does `core/persistence` interact with `sqlite`?
   - What are the interface contracts between modules?

2. **Test Interface Contracts**
   - Verify data flows correctly between modules
   - Test that errors propagate appropriately
   - Validate that configuration changes affect behavior as expected

3. **Use Real Dependencies When Possible**
   - Use in-memory databases instead of mocks when feasible
   - Test with actual file systems (using temp directories)
   - Only mock when the real dependency is too slow or unreliable

### 3. End-to-End (E2E) / Example-Based Tests

**Purpose:** Validate complete user workflows through the library's public APIs.

**Approach:**
1. **Derive Tests from Examples**
   - Each example in `examples/` should have corresponding E2E tests
   - Tests should exercise the same workflows as the examples
   - Assert on the same outcomes users would observe

2. **Test Complete Scenarios**
   - Full CRUD operations through the library
   - Configuration changes affecting behavior
   - Error recovery scenarios

### 4. Benchmarking Tests

**Focus:** Performance characteristics of public interfaces under realistic conditions.

**Guidelines:**
- Benchmark public APIs, not internal functions
- Use realistic data sizes and patterns
- Include both single-threaded and concurrent benchmarks
- Test memory allocation patterns

## Test Organization

### File Structure
```
package/
├── component.go
├── component_test.go          # Unit tests for component
├── integration_test.go        # Integration tests (if needed)
└── testdata/                  # Test fixtures and data
    ├── valid_input.json
    └── expected_output.json
```

### Test Naming Conventions
- `TestComponentName_MethodName_Scenario`
- `TestComponentName_Integration_Scenario`
- `BenchmarkComponentName_MethodName`

## Common Anti-Patterns to Avoid

1. **Testing Implementation Details**
   ```go
   // BAD: Testing internal method calls
   func TestProcessor_Process(t *testing.T) {
       p := &Processor{}
       p.Process(data)
       // Don't assert on internal method calls or private fields
   }
   ```

2. **Over-Mocking**
   ```go
   // BAD: Mocking everything, including simple data structures
   mockData := &MockData{}
   mockData.On("GetValue").Return("test")
   ```

3. **Testing Multiple Units Together in Unit Tests**
   ```go
   // BAD: This is integration testing disguised as unit testing
   func TestProcessor_Process(t *testing.T) {
       db := NewRealDatabase() // Should be mocked in unit tests
       processor := NewProcessor(db)
       // ...
   }
   ```

## Test Quality Checklist

Before submitting tests, verify:

- [ ] Tests only call public (exported) APIs
- [ ] Tests assert on observable behaviors, not internal state
- [ ] External dependencies are appropriately mocked or stubbed
- [ ] Tests would pass even if internal implementation changed (as long as interface contract remains)
- [ ] Error conditions are tested through the public interface
- [ ] Tests have clear Arrange-Act-Assert structure
- [ ] Test names clearly describe the scenario being tested

## CI/CD Integration

All tests must:
- Pass in CI environment without external dependencies
- Complete within reasonable time limits (unit tests < 1s, integration tests < 30s)
- Provide clear failure messages when they fail
- Not depend on specific timing or system resources

**Next Steps:**
1. Begin with Phase 1 analysis for `core` packages
2. Implement minimal tests (Phase 2) for each identified interface  
3. Expand tests (Phase 3) based on complexity and risk assessment
4. Leverage `examples/` directory for E2E test scenarios
