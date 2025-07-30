package types

import "github.com/asaidimu/go-anansi/v6/core/schema"

// Issue represents a validation or operational issue.
type Issue struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Path        string `json:"path,omitempty"`
	Severity    string `json:"severity,omitempty"`
	Description string `json:"description,omitempty"`
}

// ValidationResult represents the result of a validation operation.
type ValidationResult struct {
	Valid  bool    `json:"valid"`
	Issues []Issue `json:"issues"`
}

// NodeResult holds the outcome of a single node's execution.
type NodeResult struct {
	Success bool
	Value   any
	Issues  []Issue
}

// ValidationContext holds the state during a validation traversal.
type ValidationContext struct {
	RootData    any
	Data        any
	Results     map[string]*NodeResult
	FunctionMap *schema.FunctionMap
	Path        *string
}

// ValidationGraph represents the compiled set of validation operations.
type ValidationGraph struct {
	Nodes        map[string]ValidationNode
	Dependencies map[string][]string
}

// ValidationNode represents a single validation operation in the graph.
type ValidationNode interface {
	Execute(ctx *ValidationContext) *NodeResult
	GetDependencies() []string
	GetID() string
	GetPath() string
}

type DocumentValidator interface {
	Validate(document map[string]any, loose bool) ([]Issue, bool)
}

type ValidatorFactory = func(sc *schema.SchemaDefinition) (DocumentValidator, error)
