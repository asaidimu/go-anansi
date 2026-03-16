package ir

import "strings"

// errors.go defines the error types and pass identifiers used throughout
// the compiler. All passes emit []CompileError. The top-level Compile
// function collects errors across all passes and returns a CompileErrors
// value if any pass produced errors.

// CompilePass identifies which compiler pass produced an error.
type CompilePass uint8

const (
	PassParse       CompilePass = iota + 1
	PassSchemaIndex
	PassFieldIndex
	PassDescriptor
	PassVariants
	PassOffsets
	PassStore
	PassMeta
	PassIndexes
	PassConstraints
	PassAddressSpace
)

func (p CompilePass) String() string {
	switch p {
	case PassParse:
		return "parse"
	case PassSchemaIndex:
		return "schema_index"
	case PassFieldIndex:
		return "field_index"
	case PassDescriptor:
		return "descriptor"
	case PassVariants:
		return "variants"
	case PassOffsets:
		return "offsets"
	case PassStore:
		return "store"
	case PassMeta:
		return "meta"
	case PassIndexes:
		return "indexes"
	case PassConstraints:
		return "constraints"
	case PassAddressSpace:
		return "address_space"
	default:
		return "unknown"
	}
}

// CompileError is a single compiler diagnostic. All fields except Pass and
// Message are optional context that may be empty.
type CompileError struct {
	Pass      CompilePass
	SchemaUUID string
	FieldUUID  string
	Message    string
}

func (e CompileError) Error() string {
	var b strings.Builder
	b.WriteString("[")
	b.WriteString(e.Pass.String())
	b.WriteString("]")
	if e.SchemaUUID != "" {
		b.WriteString(" schema=")
		b.WriteString(e.SchemaUUID)
	}
	if e.FieldUUID != "" {
		b.WriteString(" field=")
		b.WriteString(e.FieldUUID)
	}
	b.WriteString(": ")
	b.WriteString(e.Message)
	return b.String()
}

// CompileErrors is the multi-error returned by Compile when one or more passes
// fail. It implements the error interface so it can be returned as a plain error.
type CompileErrors []CompileError

func (ce CompileErrors) Error() string {
	if len(ce) == 0 {
		return "compile: no errors"
	}
	var b strings.Builder
	b.WriteString("compile failed with ")
	b.WriteString(itoa(len(ce)))
	b.WriteString(" error(s):\n")
	for i, e := range ce {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("  ")
		b.WriteString(e.Error())
	}
	return b.String()
}

// Errors returns the individual CompileError values for programmatic inspection.
func (ce CompileErrors) Errors() []CompileError {
	return []CompileError(ce)
}
