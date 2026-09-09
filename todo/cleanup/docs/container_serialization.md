# Container Serialization
## Abstract principles of the schema-driven codec

**Version 1.0**
Companion to: *Schema Address Spec*, *Schema IR Specification*

---

# 1. Purpose

This document states the abstract principles of the schema-driven codec that
moves data between schema-shaped JSON and the in-memory `container.DataContainer`.
It is intentionally implementation-free: it captures the invariants any
container serialization path (decode, write-out, storage bindings) must
respect. Concrete guidance for a specific storage binding appears in the
relevant storage docs; this document is the shared mental model underneath.

---

# 2. The abstract container model

## 2.1 A document is a flat value space, not a tree

A document is stored as a **flat, typed value space** — a set of parallel
homogeneous stores, one per value type. Each stored value is located by a
**key** that encodes two facts:

- **the flat address** of the value's location in the schema's address space
  (§2.2), and
- **the identity** of the field descriptor (its declared type and name).

The schema, not the runtime, decides a value's location. The same field in
any document of a schema lives at the same key.

## 2.2 Shape determines storage

A field's storage depends only on its shape (§ covers three cases):

1. **Leaf values** (scalars, byte strings, geometries, records, typed arrays,
   untyped values) occupy a single slot located by their key.
2. **Arrays of objects** hold a sequence of elements; each element is its own
   independent **child document** sharing the element schema. These are the
   only structural values that introduce nested documents at runtime.
3. **Plain nested objects** (every other object-typed field, e.g. metadata)
   are stored **flattened** into the enclosing document's value space: each
   nested leaf is a value in the *same* document addressed by its deep flat
   address.

The third case is the load-bearing one: containment is an **addressing
relation, not an allocation relation**. Nesting an object costs no storage
dereference — it is purely a refinement of the flat address. Metadata is an
ordinary nested object field; it is not a special or retained structure.

---

# 3. The codec is one schema-directed walk

Encoding and decoding are the **same traversal** run forward and backward.
Given the schema and a coordinate in the value space, the codec decides:

- **decode**: which encoding value to consume next and where to place it;
- **encode**: which stored value to consume and how to emit it.

There is no parsing into an intermediate generic tree and no emission from an
intermediate generic map. The schema is the only navigator; JSON and the
value space are just the two endpoint encodings of the same positions.

## 3.1 A coordinate identifies a location uniquely

A coordinate is the flat address plus the field descriptor identity (§2.1).
Coordinates are stable per schema and shared across all documents. Resolving
a coordinate to a concrete slot is deterministic and can be **derived once
and reused** — the mapping from coordinate to location never varies, so it is
a single eager computation rather than a per-value cost. Implementations that
walk many documents of one schema must compute this mapping once.

## 3.2 Dispatch: structural vs. leaf

For any field there are exactly two ways to consume/produce its value:

- **Structural** — the value is a self-delimiting composite (an object, or
  an array of objects). The codec recurses into it following §2.2.
- **Leaf** — the value is a single typed literal placed in one slot.

The decision is made from the schema alone. The rule for a nested object is
to recurse at the object's own coordinate **within the same document**
(flattened §2.2); the rule for an array of objects is to allocate a child
document per element; the rule for a record is to treat it as an opaque leaf
value that is not schema-navigated.

---

# 4. Decode principles

1. **Consume one position at a time.** The decoder advances through the input
   and, for each position, visits the coordinate and stores the typed value.
2. **Absence and presence.** A field present in the input is written; a field
   absent from the input is simply not written. Names the schema does not
   bind are consumed (skipped) without interpretation.
3. **Validation is presence-based.** After an object is consumed, the fields
   the schema marks required must have been present, else the decode fails.
   Validation is about *which positions were visited*, not about the values.
4. **Decode never invents values.** Defaults are an application concern
   (identity/metadata completion), not a codec concern. A decoder that
   injected defaults would make serialize → decode → serialize
   non-identical, breaking round-trip stability.
5. **Lenient on read-back.** When a decoder is used to materialize stored
   fragments that may be partial or structurally absent (some positions not
   written), it must not reject absence the way full-document decode does.

---

# 5. Encode principles

1. **Recover the name from the key.** Every stored value yields its
   fully-qualified name from its key's address and identity; no bookkeeping
   beyond that is needed to name a value.
2. **Reproduce the shape from the addresses.** Flattened nested objects are
   re-grouped back into JSON objects by walking the address prefixes. A
   nested object is therefore rebuilt from the addresses its leaves reveal —
   containment is recovered from the flat model, not stored explicitly.
3. **Shape exclusion.** Because §2.2 says a structural field is either a
   value or a set of descendants, a position that is both is invalid. Encode
   treats the two as mutually exclusive shapes under one name.
4. **Values serialize once.** Each typed value is emitted by the writer
   appropriate to its type, in a form that binds to the same type on decode.

---

# 6. Storage-binding principles

A storage binding (e.g. a relational column) projects the same model onto
its own medium. The general rules:

1. **Scalar-typed fields** map to a native representation of the medium; the
   medium's returned form is bound back to the field's type on read.
2. **Byte-string fields** map to the medium's raw binary form; they are an
   opaque leaf, never routed through a lossy textual conversion on read.
3. **Structural fields** (objects, arrays of objects, records) map to a
   serialized self-delimiting text form produced by the encoder for that
   field's coordinate, and are decoded back on read by visiting that
   coordinate in a fresh document per the shape rules.
4. **NULL** maps to absence: the position is simply not written.
5. **Schema-derived identity and metadata are ordinary fields**; they lead
   the field order but are bound by exactly the same rules as any user field.

---

# 7. Invariants (checklist)

1. A document is a flat typed value space keyed by stable, schema-derived
   coordinates; no runtime tree is built for it.
2. Nesting is addressing, not allocation; only arrays of objects allocate
   child documents.
3. Encode and decode are one schema-directed walk; nothing materializes a
   generic intermediate tree.
4. The coordinate→location mapping is shared and computed once per schema.
5. Decode never injects defaults and never invents values; it only writes
   what it consumes.
6. Full-document decode validates presence of required fields; read-back of
   potentially-partial fragments must not.
7. A structural field holds a value *or* descendants, never both.
8. Schema-bound storage must recover a value's type from the schema, not the
   storage medium's returned form.