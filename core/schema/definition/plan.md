# Development and Testing Plan for go-anansi/core/schema/definition

## High-Level Overview

This package defines the schema structures and provides mechanisms for resolving, validating, and ensuring the integrity of data against those schemas. It includes core components for defining fields, constraints, and their evaluation logic, aiming for robust and high-performance data validation.

## Current Status (Completed Tasks)

The following items from the initial plan have been addressed:

*   **`validator.go` Specific Tests (`MaxDepth`):**
    *   Test with `MaxDepth` set to `1` (allowing only root fields, no nesting) has been added and verified.
*   **`constraints.go` Specific Tests (JSON Serialization/Deserialization):**
    *   Test for `ConstraintUnion.UnmarshalJSON` handling `ConstraintGroup` with `operator` but no `rules` field (resulting in an empty `Rules` slice) has been added and verified.
*   **General Testing Methodologies (`Fuzz Testing`):**
    *   Fuzz testing for the top-level `Schema` object has been initiated (`FuzzSchema`) and has passed initial runs.
*   **Suggestions for Benchmarks (`Validation Performance`):**
    *   A basic benchmark for `Validator.Validate` using a simple schema and object (`BenchmarkValidator_Validate_SimpleSchema`) has been set up and run.

## Critique Summary and Proposed Improvements

A review of the system's design and current implementation highlights several areas for enhanced robustness and clarity. The core areas of potential failure include:

*   **Complexity Management:** The use of discriminated unions (`ConstraintUnion`) and multi-purpose structs (`NestedSchema`) introduces complexity that requires vigilant management to avoid subtle bugs.
*   **Field Resolution Robustness:** The `FieldResolver` is critical for transforming raw schema definitions into actionable `EffectiveField` structures. Incomplete or inconsistent resolution can lead to panics or "INTERNAL_ERROR" messages later in the validation pipeline. Crucially, active cycle detection during resolution is needed for more informative errors than generic `MaxDepth` exceeded messages. The strict requirement for typed array/set elements is a design decision that needs clear enforcement.
*   **Validation Thoroughness:** While core scenarios are covered, comprehensive testing of all type-specific edge cases and interaction logic is vital to prevent unexpected behavior.
*   **Performance:** The recursive nature of validation, especially with deep schemas and numerous constraints, makes performance a continuous concern.
*   **JSON Unmarshaling Reliability:** Robustness against malformed or unexpected JSON inputs for complex structures like constraints is paramount to prevent crashes or silent data corruption.

### General Strategies for Improvement ("The Cure"):

1.  **Exhaustive Unit and Integration Testing:** Systematically cover all identified edge cases, error conditions, and interaction scenarios for every component.
2.  **Proactive Error Reporting:** Implement mechanisms for detecting and reporting errors as early as possible in the pipeline (e.g., during schema resolution for circular references) with precise, actionable messages.
3.  **Performance Profiling and Optimization:** Regularly benchmark and profile critical paths to identify and alleviate performance bottlenecks.
4.  **Clear Design Enforcement:** Ensure that design principles (e.g., strictly typed arrays/sets) are consistently enforced through explicit error messages during schema definition/resolution, rather than relying on implicit validation failures.
5.  **Documentation of Complexities:** Clearly document the design rationale and expected behavior of complex structures and their processing.

## Revised / Expanded Testing Strategy

To enhance robustness, the testing strategy will focus on these areas:

### 1. `FieldResolver` and `ResolvedSchema` Specific Tests

*   **Circular Reference Detection:**
    *   Construct scenarios involving truly circular schema references (e.g., `SchemaA` referencing `SchemaB`, which references `SchemaA`) and verify that the `FieldResolver` correctly detects these cycles *during resolution* and returns a specific, informative error (e.g., `ErrCircularReference`), rather than waiting for `MaxDepth` in validation.
*   **Error Handling During Resolution:**
    *   Add tests for scenarios where `SchemaId` references are missing or invalid, ensuring `ErrSchemaNotFound` and other resolution-specific errors are returned.
    *   Test invalid nested schema definitions (e.g., `NestedSchema` having neither fields nor a type, as per `resolveNestedSchema`).

### 2. `validator.go` Specific Tests

*   **`MaxDepth` (Validation Phase):**
    *   Verify `MaxDepth` behavior when validating deeply nested objects, ensuring `MAX_DEPTH_EXCEEDED` is returned at the correct path within the validation phase for both self-referential schemas and intentionally deep valid structures.
*   **Type-Specific Edge Cases (`EffectiveField.Validate`):**
    *   **`Geometry`**: Test with empty geometry arrays (`[]any{}`), tuples containing non-numeric coordinates (e.g., `[]any{[]any{"a", 1}}`), and tuples with varying lengths within the same geometry.
    *   **`Enum`**: Test enums with `LiteralValue` of diverse types (e.g., integers, booleans) and `INVALID_ENUM_DEFINITION` (where `ef.EnumValues` is `nil` or empty).
    *   **`Array` and `Set`**: Test with invalid element types given a *defined* element schema. For `Set`, rigorously test `SET_DUPLICATE` with objects or complex types where string representation for comparison might be crucial.
    *   **`Object`**: Test `ef.ObjectFields == nil` (if `FieldResolver` allows this state for some reason).
    *   **`Record`**: Test `ef.ElementField == nil` (if `FieldResolver` allows this state for records without a value schema).
    *   **`Union`**: Test `INVALID_UNION_DEFINITION` (`len(ef.UnionAlternatives) == 0`). Verify behavior when multiple alternatives could legitimately match.
    *   **`Composite`**: Test `INVALID_COMPOSITE_DEFINITION` (`len(ef.CompositeComponents) == 0`). Create scenarios where `Required` fields interact with multiple components defining the same field.
*   **Helper Functions**: Add explicit unit tests for `isNumber`, `compareValues`, and `toFloat64` to cover various input types and edge conditions (e.g., `isNumber(nil)`, `compareValues(float64, int)`, `toFloat64("not_a_number")`, `NaN`, `Inf`).

### 3. `constraints.go` Specific Tests (JSON Serialization/Deserialization & Evaluation)

*   **`ConstraintUnion.UnmarshalJSON`**:
    *   Test for JSON inputs containing `predicate` without `fields` or `parameters`.
    *   Verify handling of JSON with unexpected or unknown fields, ensuring graceful error handling or ignoring.
*   **`Constraint.UnmarshalJSON` / `ConstraintUnion.MarshalJSON`**:
    *   Ensure `omitempty` behavior for `Parameters` is correct when `LiteralValue` is `IsZero()` but not `IsNull()`, or vice-versa.
    *   Test marshaling a `ConstraintGroup` with an empty `Rules` array.
*   **Predicate Evaluation:** Create tests for various predicates (mocked or real) with valid and invalid inputs, ensuring correct issue generation.

### 4. General Testing Methodologies

*   **Fuzz Testing:** Extend fuzz testing to `UnmarshalJSON` methods for `Constraint`, `ConstraintUnion`, and `LiteralValue` to discover vulnerabilities or panics with malformed JSON inputs.
*   **Property-Based Testing:** Apply property-based testing to `compareValues` to ensure its commutative and transitive properties hold for various numeric and string inputs.

## Revised / Expanded Benchmarking Strategy

Given the criticality and recursive nature of schema validation, performance is key. Benchmarks should identify and optimize bottlenecks, focusing on:

### 1. Validator Initialization (`NewValidator`)

*   **Scenario**: Benchmark the time taken to create a `Validator` instance for schemas of varying complexity (e.g., 10 fields, 100 fields, 1000 fields, schemas with deep nesting, schemas with many constraints).
*   **Focus**: Measure the overhead of `ResolvedSchema` creation and internal `resolvedFields` mapping.

### 2. Validation Performance (`Validator.Validate`, `ValidatePartial`, `ValidateLoose`)

*   **Scenario**:
    *   Validate small, medium, and large data objects against corresponding schemas.
    *   Measure the impact of `MaxDepth` settings on validation time.
    *   Compare performance across `Strict`, `PartialStrict`, and `Loose` modes for representative workloads.
    *   Evaluate schemas with a high number of constraints (e.g., 100+ rules, nested `ConstraintGroup`s) to identify performance degradation.
    *   Benchmark type-specific validations (e.g., `validateArray` with 10k elements, `validateObject` with 1k fields, `validateUnion` with 50 alternatives).

### 3. JSON Serialization/Deserialization (Schema/Constraint Structures)

*   **Scenario**: Benchmark `json.Marshal` and `json.Unmarshal` operations for `Schema` and `Constraint` objects.
*   **Focus**: This is especially important for structures with custom `UnmarshalJSON` and `MarshalJSON` implementations (`Constraint`, `ConstraintUnion`), where performance can be sensitive to the complexity of the data being serialized.

### 4. Path-Based Value Retrieval (`GetValueByPath`)

*   **Scenario**: Benchmark `GetValueByPath` with varying path depths (e.g., "field", "field.nested", "field.nested.deeply.nested") and against objects with different levels of nesting.
*   **Focus**: Identify any linear or exponential performance scaling with path length or object depth.

## Next Steps / Priorities

1.  **Prioritize `FieldResolver` Circular Reference Detection:** This is a critical architectural improvement that will enhance robustness and provide clearer error messages.
2.  **Continue `validator.go` Type-Specific Edge Cases:** Systematically work through the remaining `EffectiveField.Validate` edge cases.
3.  **Implement Helper Function Tests:** Add comprehensive tests for `isNumber`, `compareValues`, and `toFloat64`.
4.  **Expand Fuzz Testing:** Extend fuzzing to `Constraint` and `LiteralValue` unmarshaling.
5.  **Deep Dive into Benchmarking:** Begin more detailed benchmarking as outlined, focusing on interpreting results and identifying optimization targets.
