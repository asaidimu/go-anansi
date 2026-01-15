package document

import (
	"context"

	"github.com/asaidimu/go-anansi/v6/core/schema"
)

// A Document represents a flexible, schema-aware data structure.
//
// A Document consists of three distinct parts:
//
//  1. ID (string): Immutable system-generated identifier
//     - Access via: ID()
//     - Cannot be modified after creation
//
//  2. Data (map[string]any): User-managed fields
//     - Access via: Get(key), Set(key, value), GetString(key), etc.
//     - This is YOUR data - all Get/Set operations work on this
//
//  3. Metadata (map[string]any): System and custom metadata
//     - Access via: Metadata(), SetMetadataValue(key, value)
//     - Contains: version, timestamps, checksums, signatures
//     - Reserved fields are managed by the system

//  4. Schema: The schema definition that this document is based on

// TODO: Implement this class after dealing with the schema
type Document struct {
	id       string
	ctx      context.Context
	data     map[string]any
	metadata map[string]any
	schema   *schema.SchemaDefinition
	// Question, how do we pass around the schema?, and of what use is it here
	// It implies the existence of a validator but having each document hold a
	// copy of the validator might be counter productive and probably not memory
	// safe.
}
