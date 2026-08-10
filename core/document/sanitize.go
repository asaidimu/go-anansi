package document

import (
	"context"
	"fmt"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/sanitize"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// ============================================================================
// SANITIZATION
// ============================================================================

// Sanitize returns a sanitized copy of d. The original is unchanged.
// Scopes are resolved from d's embedded context plus any additional contexts.
//
// Container-backed documents are sanitized without leaving the typed
// container: the document is deep-copied once (Clone) and the copy's masked
// leaves are rewritten in place via a schema walk. No map is materialized and
// no container is rebuilt — the record-view path below is the only one that
// works on a map, because the record view already is one.
func (d *Document) Sanitize(ctx ...context.Context) (data.Documenter, error) {
	if d == nil {
		return nil, ErrNilDocument
	}
	sanitizer, err := d.sanitizerForContexts(ctx...)
	if err != nil {
		return nil, err
	}
	if sanitizer == nil {
		return d.Clone(), nil
	}

	if d.isRecord() {
		return newRecordView(sanitizer.SanitizeDocumentDeep(deepCloneMap(d.record)), d.ctx), nil
	}

	out, err := sanitizedContainerClone(d, sanitizer)
	if err != nil {
		return nil, err
	}
	if err := out.Hash(); err != nil {
		return nil, common.SystemErrorFrom(err).
			WithOperation("document.Sanitize").
			WithMessage("failed to hash sanitized document")
	}
	return out, nil
}

// SafeString returns a sanitized string representation suitable for logging.
func (d *Document) SafeString(ctx ...context.Context) string {
	sanitized, err := d.Sanitize(ctx...)
	if err != nil {
		return fmt.Sprintf("[SANITIZATION_ERROR: %v]", err)
	}
	return sanitized.String()
}

func (d *Document) sanitizerForContexts(ctx ...context.Context) (*sanitize.DocumentSanitizer, error) {
	return sanitize.Registry().GetForContext(d.Context(), ctx...)
}

// ============================================================================
// CONTAINER-LEVEL SANITIZATION
// ============================================================================

// sanitizedContainerClone deep-copies a container-backed document and masks
// every policy-matched leaf of the copy in place, mirroring the schema walk
// materializeSlot uses. The original document and container are untouched.
func sanitizedContainerClone(d *Document, ds *sanitize.DocumentSanitizer) (*Document, error) {
	out := d.Clone().(*Document)
	if err := sanitizeContainerLeaves(out.cs, out.c, out.slotIdx, out.prefix, ds, false); err != nil {
		out.Release()
		return nil, common.SystemErrorFrom(err).
			WithOperation("document.sanitizeContainer").
			WithMessage("failed to sanitize document")
	}
	return out, nil
}

// sanitizeContainerLeaves walks the object at schema slot slotIdx stored in
// container c (addressed under path) and rewrites leaves whose masking policy
// is not MaskPreserve. It follows the same field enumeration and address
// scheme as materializeSlot so each leaf knows its schema field name for
// policy lookup. inMetadata marks the _metadata_ subtree, whose reserved
// system fields are always preserved.
func sanitizeContainerLeaves(cs *definition.CompiledSchema, c *container.DataContainer, slotIdx uint8, path definition.ResolvedPath, ds *sanitize.DocumentSanitizer, inMetadata bool) error {
	if cs == nil || int(slotIdx) >= len(cs.Schemas) {
		return nil
	}
	slot := cs.Schemas[slotIdx]
	scratch := make(definition.ResolvedPath, len(path), len(path)+1)
	copy(scratch, path)
	for j := uint16(0); j < slot.FieldCount; j++ {
		abs := int(slot.FieldStart) + int(j)
		fd := cs.Descriptors[abs]
		name := cs.FieldsMeta[abs].Name
		fp := append(scratch, definition.NewResolvedStep(slotIdx, uint8(j)))

		childMeta := inMetadata || (len(path) == 0 && name == data.MetadataField)

		if !fd.Terminal() && fd.ChildSchemaIdx() != definition.FdNoChild {
			child := fd.ChildSchemaIdx()
			switch fd.DataType() {
			case container.TypeRecord:
				k := internalKey(fd)
				if m, ok, err := c.GetRecord(k); err == nil && ok && m != nil {
					if err := c.SetRecord(k, ds.SanitizeNestedMap(m)); err != nil {
						return err
					}
				}
			case container.TypeArrayObject:
				k := internalKey(fd)
				if children, ok, err := c.GetArrayObject(k); err == nil && ok {
					for _, ch := range children {
						if ch == nil {
							continue
						}
						if err := sanitizeContainerLeaves(cs, ch, child, fp, ds, childMeta); err != nil {
							return err
						}
					}
				}
			default:
				if err := sanitizeContainerLeaves(cs, c, child, fp, ds, childMeta); err != nil {
					return err
				}
			}
			continue
		}

		// Reserved system metadata fields are never masked, matching the
		// map pipeline's SanitizeMetadata.
		if childMeta && isReservedMetadataKey(name) {
			continue
		}

		k, err := computeLeafKey(cs, fd, fp)
		if err != nil {
			return err
		}
		if !c.IsSet(k) || c.IsNull(k) {
			continue
		}

		policy := ds.PolicyForField(name)
		if policy == sanitize.MaskPreserve || policy == "" {
			continue
		}

		v, _, err := getByType(c, fd.DataType(), k)
		if err != nil {
			return err
		}
		masked := ds.ApplyPolicyValue(name, v, policy)

		// A masked value is always a string (redact/hash/obscure). Writing a
		// string into a non-string slot is a type change the container cannot
		// represent; surface an error for those, matching the map pipeline's
		// failed FromMap rebuild for the same case.
		if s, ok := masked.(string); ok && fd.DataType() != container.TypeString && fd.DataType() != container.TypeBytes {
			return fmt.Errorf("document: cannot store sanitized value %q of field %q in %v slot", s, name, fd.DataType())
		}
		if err := setByType(c, fd.DataType(), k, masked); err != nil {
			return err
		}
	}
	return nil
}
