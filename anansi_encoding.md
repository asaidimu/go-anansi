# Anansi Binary Wire Format Specification
**Version: 1.0**

## Table of Contents

1. [Introduction & Overview](#1-introduction--overview)
2. [Core Concepts](#2-core-concepts)
3. [Packet Type Specifications](#3-packet-type-specifications)
4. [Advanced Features](#4-advanced-features)
5. [Schema Management](#5-schema-management)
6. [Implementation Guidelines](#6-implementation-guidelines)
7. [Performance Characteristics](#7-performance-characteristics)
8. [Protocol Extensions](#8-protocol-extensions)
9. [Security & Error Handling](#9-security--error-handling)
10. [Testing & Conformance](#10-testing--conformance)
11. [Appendices](#11-appendices)

---

## 1. Introduction & Overview

### 1.1 Purpose

The Anansi Binary Wire Format is a high-performance serialization format specifically optimized for `Document` instances. It serializes the flattened, typed data structure defined by the Document Specification v2.0, supporting four packet types automatically selected based on data characteristics, with full schema versioning and optional compression.

This format is a physical manifestation of the Document Specification. It inherits the Document's type system, field identity scheme, and null semantics directly. It delegates all schema structure (recursion, nesting, validation) to the layer that constructs `Document` objects — the wire format sees only a flat set of typed `DataPoint`→value pairs.

### 1.2 Design Principles

- **Storage Inheritance** — This format is a direct physical expression of the Document Specification v2.0. All types, limits, and field identity come from there.
- **Self-Delineation** — Data boundaries are determined by the Schema + State Map, eliminating per-row delimiter bytes.
- **Native Null Semantics** — The `DataPoint` null bit is the authoritative source of null state. The wire format does not inject additional null encoding on top of it.

---

## 2. Core Concepts

### 2.1 Common Header Format

All packets begin with a 2-byte header:

```
Byte 0: Flags (8 bits)
Byte 1: Schema Version (8 bits)
```

#### 2.1.1 Flags Byte Layout

```
┌─────┬─────┬─────┬─────┬─────┬─────┬─────┬─────┐
│  7  │  6  │  5  │  4  │  3  │  2  │  1  │  0  │
└─────┴─────┴─────┴─────┴─────┴─────┴─────┴─────┘
   │     │     │     │     │     │     │     │
   │     │     │     │     │     │     └─────┴──── Packet Type (bits 0-1)
   │     │     │     │     │     │
   │     │     │     │     │     └──────────────── Compression (bit 2)
   │     │     │     │     │
   │     │     │     │     └────────────────────── Encoding (bit 3, Batch only)
   │     │     │     │
   │     │     └─────┴──────────────────────────── Version Epoch (bits 4-5)
   │     │
   │     └──────────────────────────────────────── Encryption (bit 6)
   │
   └────────────────────────────────────────────── Hash Present (bit 7)
```

**Bits 0-1: Packet Type**
- `00` (0x00): Dense (Type 1)
- `01` (0x01): Sparse (Type 2)
- `10` (0x02): Batch (Type 3)
- `11` (0x03): Stream (Type 4, Extended)

**Bit 2: Compression Flag**
- `0`: Uncompressed
- `1`: Compressed (all data after schema version/epoch is compressed)

**Bit 3: Encoding Mode** (Batch packets only)
- `0`: Row-oriented
- `1`: Columnar

**Bits 4-5: Version Epoch**
- `00` (0): Epoch 0 (schema versions 0–255)
- `01` (1): Epoch 1 (schema versions 256–511)
- `10` (2): Epoch 2 (schema versions 512–767)
- `11` (3): Epoch 3 (schema versions 768–1023)

Combined with schema version byte, provides 1024 total schema versions:
```
Full Version = (Epoch << 8) | SchemaVersion
```

**Bit 6: Encryption Flag**
- `0`: Unencrypted
- `1`: Encrypted (all data after byte 1 is encrypted)

**Bit 7: Hash Present Flag**
- `0`: No hash
- `1`: Hash present (16 bytes immediately after byte 1, before payload/encryption)

#### 2.1.2 Example Flag Values

```
Basic packets:
Dense, uncompressed:                    0x00 (0b00000000)
Sparse, uncompressed:                   0x01 (0b00000001)
Batch row, uncompressed:                0x02 (0b00000010)
Batch columnar, uncompressed:           0x0A (0b00001010)
Stream, uncompressed:                   0x03 (0b00000011)

With compression:
Dense, compressed:                      0x04 (0b00000100)
Batch row, compressed:                  0x06 (0b00000110)
Stream, compressed:                     0x07 (0b00000111)

With version epoch:
Dense, epoch 1 (versions 256-511):      0x10 (0b00010000)
Dense, epoch 2 (versions 512-767):      0x20 (0b00100000)
Dense, epoch 3 (versions 768-1023):     0x30 (0b00110000)

With encryption:
Dense, encrypted:                       0x40 (0b01000000)
Dense, compressed + encrypted:          0x44 (0b01000100)

With hash:
Dense, hashed:                          0x80 (0b10000000)
Dense, hashed + encrypted:              0xC0 (0b11000000)

Combined:
Dense, epoch 2, compressed, encrypted, hashed:
                                        0xE4 (0b11100100)
```

### 2.2 Schema Version and Epoch

**Schema Version Byte (Byte 1):**
- Range: 0–255
- Combined with epoch bits (4-5) for full version

**Full Version Calculation:**
```
epoch       = (flags >> 4) & 0x03
fullVersion = (epoch << 8) | byte[1]

Examples:
  flags=0x00, version=0x01 → fullVersion = 1
  flags=0x10, version=0x00 → fullVersion = 256 (epoch 1, schema 0)
  flags=0x20, version=0xFF → fullVersion = 767 (epoch 2, schema 255)
  flags=0x30, version=0xFF → fullVersion = 1023 (epoch 3, schema 255)
```

**Schema Registry Mapping:**
- Each full version maps to a semantic version in an ordered registry.
- Registry maintains version history: `{0: "1.0.0", 1: "1.1.0", 256: "2.0.0", ...}`
- Decoders look up schema by full version.
- A `Collection` is bound to a single full version. Documents from different versions MUST NOT be mixed in the same packet.

### 2.3 DataPoints and Field Ordering

Fields are encoded in **stable sorted order by their `DataPoint` value (ascending int32)**. The `DataPoint` encodes the `DataType` in bits 1–4, so ascending sort naturally groups fields by type first, then by ID within each type. This mirrors Document v2.0's `data [16]unsafe.Pointer` layout where each slot index corresponds to a `DataType` iota value.

**DataPoint Bit Layout:**
```
┌──────────┬────────────┬──────────────────────────────────┐
│ Null(1b) │  Type(4b)  │           ID(27b)                │
└──────────┴────────────┴──────────────────────────────────┘
     0          1–4                   5–31
```

**Note on the null bit in field ordering:** The null bit (bit 0) participates in the int32 sort. A DataPoint representing a null field (`null bit = 1`) sorts one position higher than the same DataPoint with a value (`null bit = 0`). Encoders MUST use the canonical (non-null) DataPoint for ordering purposes, then apply null state as determined by the Document's `positions` map, not by the DataPoint's own null bit. The DataPoint null bit is informational and used during sparse encoding; it does not affect field identity or sort position in schema definition.

### 2.4 Varint Encoding

#### 2.4.1 Unsigned Varint (LEB128)

```
- 7 bits per byte for value
- Bit 7 (MSB) = continuation bit
  - 1: more bytes follow
  - 0: last byte

Examples:
  0      → [0x00]
  127    → [0x7F]
  128    → [0x80 0x01]
  16383  → [0xFF 0x7F]
  16384  → [0x80 0x80 0x01]
```

#### 2.4.2 Signed Varint (Zigzag Encoding)

For signed integers, use zigzag encoding before varint:

```
zigzag(n) = (n << 1) ^ (n >> 63)  // for 64-bit

Examples:
  0   → 0   → [0x00]
  -1  → 1   → [0x01]
  1   → 2   → [0x02]
  -2  → 3   → [0x03]
```

#### 2.4.3 Maximum Size

Varint encoding supports up to 64-bit values (9 bytes maximum).

### 2.5 Field Type Encoding

The following encoding applies to each `DataType`. These map directly to the 16 types in the Document Specification.

#### TypeInt (int64)
```
Signed varint (zigzag + LEB128)
1–9 bytes depending on magnitude
```

#### TypeFloat (float64)
```
8 bytes, little-endian IEEE 754
```

#### TypeString (string)
```
[length: varint][bytes: UTF-8]
Length is byte count, not character count. No null terminator.
```

#### TypeBool (bool)
```
Dense Mode:  Packed into bitfields (8 bools per byte).
Sparse Mode: Single byte (0x00 = false, 0x01 = true).
```

#### TypeDecimal
```
[scale: u8][coefficient: signed varint]
Represents: coefficient / 10^scale
Example: 123.45 → [0x02][zigzag(12345)]
```

#### TypeGeometry ([][]float64)
```
[ring_count: varint]
FOR EACH RING:
  [point_count: varint]
  FOR EACH POINT:
    [x: float64 LE][y: float64 LE]
```

#### TypeRecord (*DataContainer)
```
A recursively encoded Document payload.
[payload_length: varint][anansi_packet_bytes]
The nested packet uses the same schema version as the parent
unless the schema defines an override.
```

#### TypeUnknown (any)
```
[type_tag: u8][encoded_value]
type_tag corresponds to the DataType iota value of the actual runtime type.
Used only when the schema genuinely cannot constrain the type at definition time.
```

#### TypeArrayInt ([]int64)
```
[count: varint]
FOR EACH ELEMENT: [signed varint]
```

#### TypeArrayFloat ([]float64)
```
[count: varint]
FOR EACH ELEMENT: [float64 LE]
```

#### TypeArrayString ([]string)
```
[count: varint]
FOR EACH ELEMENT: [length: varint][bytes: UTF-8]
```

#### TypeArrayBool ([]bool)
```
[count: varint][packed bits: ceil(count/8) bytes]
Bits packed LSB-first within each byte.
```

#### TypeArrayDecimal ([]Decimal)
```
[count: varint]
FOR EACH ELEMENT: [scale: u8][coefficient: signed varint]
```

#### TypeArrayObject ([]*DataContainer)
```
[count: varint]
FOR EACH ELEMENT: [payload_length: varint][anansi_packet_bytes]
```

#### TypeArrayUnknown ([]any)
```
[count: varint]
FOR EACH ELEMENT: [type_tag: u8][encoded_value]
```

#### TypeArray ([][]any)
```
[outer_count: varint]
FOR EACH INNER ARRAY: [count: varint] FOR EACH ELEMENT: [type_tag: u8][encoded_value]
```

### 2.6 Storage Model Inheritance

The Anansi Wire Format inherits directly from the Document Specification v2.0:

- **Type system:** Exactly 16 DataTypes, indexed 0–15 by their iota value.
- **Field identity:** 27-bit ID space per type — up to 134,217,727 distinct field identifiers per DataType.
- **Selector stability:** DataPoints are schema-derived and stable for the life of the full schema version.
- **No depth/offset concept:** Nesting is flattened into the 27-bit ID space at schema definition time. The wire format sees a flat set of DataPoint→value pairs regardless of logical nesting depth.

### 2.7 Null Handling

The Document Specification defines three field states. The wire format handles them as follows:

| State | Document condition | Dense encoding | Sparse encoding |
|---|---|---|---|
| **Has Value** | `positions[point] >= 0` | State map `10`, write value | Write DataPoint (null bit = 0), write value |
| **Null** | `positions[point] == -1` | State map `01`, skip value | Write DataPoint (null bit = 1), no value follows |
| **Not Set** | key absent from `positions` | State map `00`, skip | Skip entirely |

**Null bit in sparse packets:** In sparse encoding, the DataPoint written to the wire has its null bit (bit 0) set to `1` to signal null, and to `0` for a value-bearing field. The decoder recovers the canonical DataPoint by masking off the null bit: `canonicalPoint = DataPoint(wire & ^1)`. This is a read of the DataPoint's own null bit — no external shift-and-OR mutation is applied.

**Dense state map:** 2 bits per schema field. Values:
- `00` — Not Set. Skip.
- `01` — Null. Skip value bytes.
- `10` — Has Value. Read value bytes.
- `11` — Reserved.

---

## 3. Packet Type Specifications

### 3.1 Type 1: Dense Packet

#### 3.1.1 Eligibility

A schema is eligible for Dense encoding if it is **non-recursive**: all fields eventually terminate in concrete DataTypes with no self-referential `TypeContainer` cycles. Schemas with `TypeContainer` fields that reference the same schema version are recursive and MUST use Sparse encoding.

Eligible example: a struct with TypeInt, TypeString, TypeContainer (where the nested schema is a different, finite schema).
Ineligible example: a `Node` schema with a `TypeContainer` field that also uses the `Node` schema.

#### 3.1.2 State Map

Since field count *N* is finite and known from the schema, the state map is a fixed-length bitstream of `2 × N` bits, one 2-bit entry per schema field in DataPoint-ascending order.

```
State map bit pairs:
  00 = Not Set
  01 = Null
  10 = Has Value
  11 = Reserved
```

The state map is byte-aligned (padded with `00` bits to the next byte boundary if needed).

#### 3.1.3 Structural Layout

```
┌───────────────────────────────┐
│ Header (2 bytes)              │  flags, schema version
├───────────────────────────────┤
│ State Map (bitstream)         │  2 bits per schema field, byte-aligned
├───────────────────────────────┤
│ TypeInt Value Block           │  all fields where state=10 and Type=TypeInt
├───────────────────────────────┤
│ TypeFloat Value Block         │  all fields where state=10 and Type=TypeFloat
├───────────────────────────────┤
│ TypeString Value Block        │  ...
├───────────────────────────────┤
│ TypeBool Value Block          │
├───────────────────────────────┤
│ TypeDecimal Value Block       │
├───────────────────────────────┤
│ TypeGeometry Value Block      │
├───────────────────────────────┤
│ TypeContainer Value Block     │
├───────────────────────────────┤
│ TypeArray* Value Blocks       │  one block per array type, same ordering
└───────────────────────────────┘
```

Value blocks appear in DataType iota order (TypeUnknown=0 through TypeArray=15). Empty blocks occupy zero bytes. The decoder uses the schema field count per type to know exactly how many values to read from each block.

#### 3.1.4 Handling Recursive Schemas

If the schema is recursive (contains `TypeContainer` cycles), the encoder MUST switch to Sparse encoding. Dense encoding is undefined for recursive schemas.

---

### 3.2 Type 2: Sparse Packet

Used for recursive structures, patches, or low field density. Fields are written with their DataPoint identifier so the decoder can look up type and identity without a schema-ordered index.

#### 3.2.1 Null Encoding

In Sparse packets, the DataPoint's own null bit (bit 0) signals null:
- `null bit = 0`: field has a value; value bytes follow immediately.
- `null bit = 1`: field is null; no value bytes follow.

The canonical DataPoint for a null field is recovered by the decoder as:
```
canonicalPoint = DataPoint(wirePoint & ^DataPoint(1))
```

No additional shift or mutation is applied to the DataPoint — the null bit is used in place.

#### 3.2.2 Wire Format

```
┌──────────────────────────────────────────────┐
│ [flags: u8]                                  │  Packet type = 0b01
│ [schema_version: u8]                         │
├──────────────────────────────────────────────┤
│ [field_count: varint]                        │  Number of set fields (null + value)
├──────────────────────────────────────────────┤
│ FOR EACH SET FIELD (in DataPoint order):     │
│   [data_point: varint]                       │  Full int32(DataPoint), null bit set if null
│   [field_data]                               │  Encoded per type; absent if null bit = 1
└──────────────────────────────────────────────┘
```

`field_count` includes both value-bearing and null fields. Not-set fields are omitted entirely.

---

### 3.3 Type 3: Batch Packet

Transmits a fixed number of Documents. Inherits schema version from the header.

**Header after flags/version:**
- `record_count`: varint
- `batch_flags`: u8
  - Bit 0: Orientation (0 = Row-oriented, 1 = Columnar)
  - Bit 1: Density (0 = Dense, 1 = Sparse)

#### 3.3.1 Row-Oriented Dense Batch

Each record is self-contained. Optimized for materializing full Documents.

```
FOR EACH RECORD (0 to record_count-1):
  [State Map]    (2 bits per schema field, byte-aligned)
  [Value Blocks] (one per DataType, values only for fields with state=10)
```

Delineation: the schema determines the state map bit length. Once the state map is read, the decoder knows exactly which value blocks follow and their sizes.

#### 3.3.2 Row-Oriented Sparse Batch

Used when records have few fields set relative to a wide schema, or the schema is recursive.

```
FOR EACH RECORD (0 to record_count-1):
  [field_count: varint]
  FOR EACH SET FIELD:
    [data_point: varint]   (null bit set if null)
    [value]                (absent if null)
```

#### 3.3.3 Columnar Dense Batch

Optimized for scanning specific fields across many records. Fields are grouped by DataType.

```
FOR EACH DATATYPE (TypeUnknown=0 through TypeArray=15):
  IF this type has any schema fields:
    [State Map Column]   (2 bits × record_count for each field of this type)
    FOR EACH FIELD OF THIS TYPE:
      [Value Array]      (values for all records where state=10)
```

Fixed-width types (TypeInt, TypeFloat, TypeBool): raw bytes for all N records, no per-value length prefix.
Variable-width types (TypeString, TypeGeometry, TypeContainer, TypeArray*): length-prefixed per value.

---

### 3.4 Type 4: Stream Packet

A sequence of chunks. Allows a long-lived connection to pivot between Dense and Sparse batches as data distribution changes.

#### 3.4.1 Wire Format

```
┌──────────────────────────────────────────────┐
│ STREAM HEADER (sent once)                    │
├──────────────────────────────────────────────┤
│ [flags: u8]              (0x03)              │  Extended packet type
│ [schema_version: u8]                         │
│ [extended_type: u8]      (0x01)              │  Stream marker
│ [stream_encoding: u8]                        │  Encoding flags (see 3.4.3)
├──────────────────────────────────────────────┤
│ CHUNKS (repeating)                           │
├──────────────────────────────────────────────┤
│ [chunk_descriptor: u8]                       │  Density & orientation for this chunk
│ [row_count: varint]                          │  Rows in this chunk
│ [chunk_data]                                 │  Encoded per chunk_descriptor
├──────────────────────────────────────────────┤
│ END MARKER                                   │
├──────────────────────────────────────────────┤
│ [row_count: 0]                               │  Stream terminator
└──────────────────────────────────────────────┘
```

#### 3.4.2 Stream Encoding Byte

```
┌─────┬─────┬─────┬─────┬─────┬─────┬─────┬─────┐
│  7  │  6  │  5  │  4  │  3  │  2  │  1  │  0  │
└─────┴─────┴─────┴─────┴─────┴─────┴─────┴─────┘
   │                       │     │     └─────┴──── Chunk Encoding (bits 0-1)
   │                       │     └──────────────── Compression (bit 2)
   │                       └────────────────────── Reserved (bit 3)
   └────────────────────────────────────────────── Reserved (bits 4-7)
```

**Bits 0-1: Chunk Encoding**
- `00`: Reserved
- `01`: Columnar
- `10`: Reserved
- `11`: Reserved

**Bit 2: Compression — shared context across all chunks**

#### 3.4.3 Shared Compression Context

When compression bit is set in `stream_encoding`, all chunks share one compression dictionary. The compressor learns patterns from early chunks and applies them to later ones. Typical improvement: 15–30% better ratio vs per-packet compression for homogeneous data.

#### 3.4.4 End Marker

Stream MUST terminate with `row_count = 0`.

Decoders MUST NOT assume a stream is complete until:
1. End marker (`row_count = 0`) is received, OR
2. Transport signals EOF/completion.

---

### 3.5 Delineation and Navigation

Anansi is a calculative format. To find the end of a record:

1. **Dense:** Read the `2 × N` bit State Map (N = schema field count). Sum the sizes of all fields with state `10`, using the per-type encoding rules in section 2.5.
2. **Sparse:** Read `field_count`. For each field, read the DataPoint (varint). If null bit = 0, read the value per the type encoded in DataPoint bits 1–4.
3. **Variable-length fields:** TypeString, TypeGeometry, TypeContainer, and all TypeArray* variants are length-prefixed with a varint.

---

## 4. Advanced Features

### 4.1 Compression

#### 4.1.1 Compression Flag

When bit 2 of the flags byte is set, data is compressed per the rules below.

**No encryption, no hash:**
```
All data after byte 1 (schema version) is compressed.
```

**With hash, no encryption:**
```
Hash (16 bytes) is uncompressed.
All data after hash is compressed.
```

**With encryption:**
```
Compression happens BEFORE encryption.
Decode order: decrypt → decompress.
```

For Stream packets, compression is indicated in the `stream_encoding` byte (bit 2) and follows the same rules.

#### 4.1.2 Compression Algorithm

**Recommended:** LZ4 or ZSTD.

The specification does not mandate a specific algorithm. Implementations MUST document which algorithm they use.

#### 4.1.3 Compressed Packet Structures

**Without hash or encryption:**
```
[flags: u8]               (bit 2 = 1)
[schema_version: u8]      (uncompressed)
[uncompressed_size: varint] (optional, for buffer pre-allocation)
[compressed_data...]
```

**With hash:**
```
[flags: u8]               (bits 2, 7 = 1)
[schema_version: u8]      (uncompressed)
[hash: 16 bytes]          (uncompressed)
[uncompressed_size: varint]
[compressed_data...]
```

**With encryption:**
```
[flags: u8]               (bits 2, 6 = 1)
[schema_version: u8]      (uncompressed)
[encrypted_data...]       (compress → encrypt)
```

#### 4.1.4 Decompression Process

**Without encryption:**
```
1. Check bit 2 of flags byte.
2. Read schema_version (byte 1).
3. If bit 7 set, read and verify hash (16 bytes).
4. Optionally read uncompressed_size.
5. Decompress remaining bytes.
6. Continue decoding based on packet type.
```

**With encryption:**
```
1. Check bits 2 and 6 of flags byte.
2. Read schema_version (byte 1).
3. If bit 7 set, read hash (16 bytes) — hash is of plaintext.
4. Decrypt remaining bytes.
5. Decompress decrypted bytes.
6. Verify hash (if present).
7. Continue decoding based on packet type.
```

---

### 4.2 Encryption

#### 4.2.1 Algorithm

**RECOMMENDED:** ChaCha20-Poly1305 or AES-256-GCM.

Requirements: authenticated encryption (AEAD), resistant to timing attacks, well-audited. Implementations MUST document which algorithm(s) they support. Key management is out of scope.

#### 4.2.2 Encrypted Packet Structure

**Basic:**
```
[flags: u8]               (bit 6 = 1)
[schema_version: u8]      (plaintext)
[nonce/IV: N bytes]       (algorithm-specific; 12 bytes for ChaCha20/AES-GCM)
[ciphertext + auth_tag]
```

**With hash:**
```
[flags: u8]               (bits 6, 7 = 1)
[schema_version: u8]      (plaintext)
[hash: 16 bytes]          (plaintext hash of unencrypted payload)
[nonce/IV: N bytes]
[ciphertext + auth_tag]
```

**Order of operations:**
- Encode: `plaintext → compress → encrypt → ciphertext`
- Decode: `ciphertext → decrypt → decompress → plaintext`

**Nonce reuse:** CRITICAL — never reuse a nonce with the same key. Use counter-based or cryptographically random nonces.

---

### 4.3 Integrity Hashing

#### 4.3.1 Algorithm

**REQUIRED:** BLAKE3 truncated to 128 bits (16 bytes).

#### 4.3.2 Hash Packet Structure

**Basic:**
```
[flags: u8]               (bit 7 = 1)
[schema_version: u8]
[hash: 16 bytes]          BLAKE3(payload)[0..16]
[payload...]
```

Hash is computed on uncompressed plaintext, not on compressed or encrypted bytes.

#### 4.3.3 Verification Process

```
1. Check bit 7 of flags byte.
2. If set, extract hash from bytes [2..18].
3. Process data per compression/encryption flags.
4. Compute BLAKE3(final_plaintext)[0..16].
5. Compare with extracted hash. If mismatch: reject packet, log, return error.
```

---

## 5. Schema Management

### 5.1 Schema Versioning and Migration

#### 5.1.1 Version Resolution

**Encoder:**
```
schema.Version = "2.1.4"
→ Lookup in registry: fullVersion = 256
→ epoch = fullVersion >> 8 = 1
→ schemaVersion = fullVersion & 0xFF = 0
→ Set flags bits 4-5 = 01
→ Set byte 1 = 0x00
```

**Decoder:**
```
flags = 0x10 (bits 4-5 = 01)
schemaVersion = 0x00
→ epoch = (flags >> 4) & 0x03 = 1
→ fullVersion = (epoch << 8) | schemaVersion = 256
→ Lookup in registry: "2.1.4"
→ Load compiled schema for version 256
```

#### 5.1.2 Schema Evolution

When a schema changes:
1. Increment the full version.
2. Define any migration rules.
3. Recompile codec.
4. Old and new versions coexist in the registry.

DataPoints are stable within a schema version. Adding a field in a new version assigns it a new DataPoint ID that did not exist in older versions. Removing a field retires its DataPoint ID — it must never be reused within the same schema lineage.

#### 5.1.3 Backward / Forward Compatibility

**Backward compatible** (new decoder reads old data): Add optional fields only. Do not remove fields. Do not change a DataPoint's type.

**Forward compatible** (old decoder reads new data): Requires Sparse encoding. Old decoder ignores DataPoints it does not recognise.

### 5.2 Schema Distribution

Schemas can be distributed via: embedded (hardcoded), network (on-demand fetch by version), bundle (included in application package), or registry (central service).

### 5.3 Schema as Single Source of Truth

A single Anansi schema definition can generate: wire format encoder/decoder, database migration, ORM models, TypeScript types, validation functions, OpenAPI documentation, frontend form schemas, mock data generators, test fixtures, and API client code — replacing the 6–8 fragmented copies of the same definition found in typical full-stack applications.

---

## 6. Implementation Guidelines

### 6.1 Encoder Selection Logic

```go
func EncodeDocument(schema *Schema, doc *Document) ([]byte, error) {
    fieldCount := schema.FieldCount()
    presentCount := doc.Length()
    density := float64(presentCount) / float64(fieldCount)

    if schema.IsRecursive() {
        return encodeSparse(schema, doc)
    }
    if fieldCount <= 64 || density > 0.25 {
        return encodeDense(schema, doc)
    }
    return encodeSparse(schema, doc)
}

func EncodeBatch(schema *Schema, docs []*Document) ([]byte, error) {
    if len(docs) >= 1000 {
        return encodeStream(schema, docs)
    }
    return encodeBatch(schema, docs)
}
```

### 6.2 Decoder Optimizations

**Must:**
- Pre-compile schemas to DAG at startup; cache by full version.
- Use jump tables for Dense packet type dispatch.
- Use DataPoint int32 value as the direct `positions` map key — no conversion needed.

**Should:**
- Pre-calculate state map byte length from schema field count: `ceil(2 * N / 8)`.
- Use SIMD for state map bitfield operations on wide schemas.
- Pool decode buffers to eliminate per-request allocation.
- Use `doc.Walk` for zero-copy deserialization directly into the Document's typed slices.

**May:**
- Implement zero-copy string decoding (`unsafe.String` pointing into wire buffer for short-lived strings).
- JIT-compile decoders for hot schemas.

### 6.3 DAG-Based Decoder

Compiling a schema into a DAG eliminates runtime branching on the hot decoding path. Each node in the DAG corresponds to a DataType block in the wire format. At compile time, the decoder pre-computes the state map bit length, the expected value count per type block, and the size of each fixed-width field. At decode time, the DAG executes without schema lookups.

#### 6.3.1 Zero-Copy String Decoding

```go
// Safe zero-copy string: points into the wire buffer.
// Source buffer must outlive the string.
func zeroCopyString(buf []byte, offset, length int) string {
    return unsafe.String(&buf[offset], length)
}
```

Use for request-scoped strings (IDs, paths, method names). Do not use for strings that outlive the request or are returned to clients.

#### 6.3.2 Decoder Cache

```go
type DecoderCache struct {
    mu       sync.RWMutex
    decoders map[uint16]func([]byte, *Document) error // fullVersion → decoder
}

// Hot path (per request):
version := uint16((uint16(flags>>4)&0x03)<<8) | uint16(buf[1])
decoder, _ := cache.Get(version)
decoder(buf[2:], doc)
```

### 6.4 Decoding Algorithm

#### 6.4.1 Header Parsing

```
1.  Read flags byte (byte 0).
2.  Extract packet type  (flags & 0x03).
3.  Extract compression  (flags & 0x04).
4.  Extract epoch        ((flags >> 4) & 0x03).
5.  Extract encryption   (flags & 0x40).
6.  Extract hash flag    (flags & 0x80).
7.  Read schema version  (byte 1).
8.  fullVersion = (epoch << 8) | schemaVersion.
9.  If hash flag: read and store hash (bytes 2–17).
10. If encryption flag: decrypt remaining bytes.
11. If compression flag: decompress.
12. If hash flag: verify BLAKE3(plaintext)[0..16].
```

#### 6.4.2 Dense Decoding

```
1. Look up compiled schema for fullVersion.
2. Compute state map byte length: ceil(2 * N / 8).
3. Read state map bytes.
4. For each DataType (iota 0–15):
   a. For each schema field of this type, in DataPoint order:
      - Read 2-bit state from state map.
      - If state = 10 (Has Value): decode value, call doc.AppendXxx(point, value).
      - If state = 01 (Null): call doc.SetNull(point).
      - If state = 00 (Not Set): skip.
```

#### 6.4.3 Sparse Decoding

```
1. Read field_count (varint).
2. For i = 0 to field_count-1:
   a. Read wire DataPoint (varint).
   b. Extract null bit = wirePoint & 1.
   c. Recover canonical point = DataPoint(wirePoint & ^1).
   d. If null bit = 1: call doc.SetNull(canonicalPoint).
   e. Else: decode value by canonicalPoint.Type(), call doc.SetXxx(canonicalPoint, value).
```

#### 6.4.4 Batch Decoding

**Row-oriented:**
```
1. Read record_count (varint).
2. For i = 0 to record_count-1:
   a. Acquire doc from pool.
   b. Decode as Dense or Sparse per batch_flags.
   c. Process doc.
   d. Release doc to pool.
```

**Columnar:**
```
1. Read record_count (varint).
2. For each DataType block:
   a. Read state map column (2 bits × record_count per field).
   b. Read value array for this type.
   c. Distribute values across per-record Documents.
```

#### 6.4.5 Stream Decoding

```
1. Read stream header (4 bytes: flags, version, extended_type, stream_encoding).
2. Parse stream_encoding flags.
3. If compressed, initialize shared decompressor context.
4. Loop:
   a. Read chunk_descriptor (u8).
   b. Read row_count (varint).
   c. If row_count = 0: break (end of stream).
   d. Decode chunk per chunk_descriptor.
   e. Process decoded documents.
5. If compressed, finalize decompressor.
```

---

## 7. Performance Characteristics

| Operation | Complexity | Notes |
|---|---|---|
| Dense encode | O(N) | N = set fields; state map write is O(schema size) |
| Dense decode | O(N) | DAG-compiled: no runtime schema lookups |
| Sparse encode | O(M log M) | M = set fields; sort by DataPoint if not pre-sorted |
| Sparse decode | O(M) | One map insert per field via doc.SetXxx |
| Batch encode (row) | O(R × N) | R = records, N = fields per record |
| Batch decode (columnar) | O(R × N) | Better cache performance for wide schemas |
| Field access after decode | O(1) | Document.GetXxx is one map lookup |

**Key advantages over JSON:**
- No string key allocation or hashing per field.
- No interface boxing per value.
- Typed slices in DataContainer eliminate GC pressure proportional to field count.
- Zero allocations on decode when using a pooled Document and pre-warmed capacity.

---

## 8. Protocol Extensions

### 8.1 Protocol Envelope Encoding

By defining Anansi schemas for request/response metadata, routing, and error handling, implementations achieve protocol-level performance. The wire format encodes envelope fields the same way it encodes data fields — there is no special envelope type.

### 8.2 API Request Envelope Example

**Schema (expressed as field DataPoints after schema compilation):**

```
TypeBool fields:   authenticated, compressed
TypeInt  fields:   priority (enum as int), method (enum as int)
TypeString fields: requestId, path
TypeContainer:     body (nested Document, schema discriminated by method)
```

**Wire format (Dense):**
```
[0x00][0x01]            // Dense packet, schema version 1

// State map: 2 bits per field, 7 fields total → 2 bytes
[state_map: 2 bytes]

// TypeBool block (2 fields packed):
[0x01]                  // authenticated=true, compressed=false

// TypeInt block (2 fields):
[zigzag(method)]        // e.g. POST=1 → 0x02
[zigzag(priority)]      // e.g. high=2 → 0x04

// TypeString block (2 fields):
[0x20][32 bytes]        // requestId (UUID)
[0x0D]["/api/users"]    // path

// TypeContainer block (1 field):
[payload_length][nested anansi bytes]

Total envelope overhead: ~40 bytes vs ~200-300 bytes for HTTP headers
```

### 8.3 RPC Method Routing

With enums encoded as TypeInt fields and a DAG-compiled router, method dispatch requires approximately 5 CPU cycles — no string matching, no header parsing loop.

### 8.4 Service Mesh Integration

Schema fields for service mesh metadata (sourceService, targetService, priority, retryPolicy, circuitBreaker, etc.) encode to packed TypeInt and TypeBool fields. Total metadata: 3–4 bytes. Parsing: ~20 cycles.

---

## 9. Security & Error Handling

### 9.1 Error Handling

| Error | Action |
|---|---|
| Unknown packet type (`flags & 0x03` not in 0–3) | Reject, return error |
| Unknown schema version | Reject; optionally request schema from registry |
| Buffer underflow | Reject, return error |
| Type mismatch (DataPoint type ≠ value encoding) | Reject, validation failed |
| Decompression failure | Reject, return error |
| AEAD authentication failure | Reject immediately — possible tampering |
| Hash mismatch | Reject, log integrity failure |
| Stream ends without end marker | Return partial results + error, or reject per policy |
| DataPoint ID outside schema | Ignore (forward compatibility) or reject per policy |

### 9.2 Security Considerations

#### 9.2.1 Buffer Overflow Protection

Implementations MUST:
- Validate all length fields before allocation.
- Set a maximum message size limit.
- Check buffer bounds before every read.
- Reject excessive varint sizes (> 9 bytes for 64-bit values).

#### 9.2.2 Compression Bombs

When compression is enabled: set maximum decompressed size limit, set decompression time limit, reject excessive compression ratios.

#### 9.2.3 Stream Resource Limits

Set maximum chunk count, maximum total rows, timeout for end marker, maximum stream duration.

#### 9.2.4 Schema Validation

Before decoding: validate schema version exists, verify schema hash if applicable, check schema has not been revoked.

#### 9.2.5 Denial of Service Mitigations

Rate-limit schema requests. Cache compiled schemas. Set maximum DataPoint count per schema. Limit stream chunks and duration.

---

## 10. Testing & Conformance

### 10.1 Conformance Levels

**Level 1: Basic**
- Type 1 (Dense) and Type 2 (Sparse) support.
- All 16 DataTypes (TypeUnknown through TypeArray).
- Unsigned varint encoding.
- Uncompressed only.

**Level 2: Complete**
- All four packet types.
- Signed integers (zigzag).
- Compression support.
- TypeContainer recursive encoding/decoding.

**Level 3: Optimized**
- DAG-based decoding.
- Zero-copy string decoding.
- Schema compilation and caching.
- Jump table optimization.
- Shared compression context for streams.
- Direct Document pool integration.

### 10.2 Test Requirements

Implementations MUST pass:
- Round-trip encoding/decoding for all 16 DataTypes.
- State transitions: set → null → unset → set.
- Schema evolution tests (v1 → v2 → v3).
- All four packet types.
- Compression and encryption tests.
- Stream interruption handling.
- TypeContainer recursive encoding.
- Conformance with Document v2.0 `Walk`-based serialization pattern.

---

## 11. Appendices

### Appendix A: Test Vectors

#### A.1 Type 1: Dense, Scalar Types

**Schema (v1):** 3 fields — `count` (TypeInt, id=1), `name` (TypeString, id=2), `active` (TypeBool, id=3)

**DataPoints in schema (canonical, null bit=0):**
```
count:  DataPoint{TypeInt,    id=1} = 0x00000022  (TypeInt=1, id=1)
name:   DataPoint{TypeString, id=2} = 0x00000046  (TypeString=3, id=2)
active: DataPoint{TypeBool,   id=3} = 0x00000068  (TypeBool=4, id=3... see note)
```
*(Exact bit values depend on schema-assigned IDs; shown illustratively. Ordering is ascending int32.)*

**Input:** `{count: 42, name: "test", active: true}`

**State map:** 3 fields × 2 bits = 6 bits → 1 byte (padded to 8 bits)
```
Field 0 (count):  10  (Has Value)
Field 1 (name):   10  (Has Value)
Field 2 (active): 10  (Has Value)
Padded: 10 10 10 00 → 0b10101000 → 0xA8
```

**Expected wire format (hex):**
```
00 01       // flags=0x00 (Dense), version=1
A8          // state map: all 3 fields present
2A          // count=42 (TypeInt block, zigzag varint)
04          // name length=4 (TypeString block)
74 65 73 74 // "test"
01          // active=true (TypeBool block, 1 byte in sparse; packed bit in dense)
```

**Total: 9 bytes**

---

#### A.2 Type 2: Sparse, 2 of 100 Fields Present

**Input:** field at DataPoint 0x00000022 = 42, field at DataPoint 0x00000646 = "test"

**Expected wire format (hex):**
```
01 01       // flags=0x01 (Sparse), version=1
02          // field_count=2
22 00 00 00 // DataPoint (varint encoded int32) for count; null bit=0
2A          // value=42 (zigzag varint)
46 06 00 00 // DataPoint for name; null bit=0
04 74 65 73 74 // "test"
```

*(DataPoints varint-encoded as their int32 value.)*

**Total: 14 bytes**

---

#### A.3 Type 1: Dense with Null Field

**Schema (v1):** `id` (TypeString), `email` (TypeString, optional), `age` (TypeInt)

**Input:** `{id: "123", age: 25}` — email is null

**State map:** 3 fields; email=null (01), age=Has Value (10), id=Has Value (10)
```
Ordered by DataPoint ascending — assume age sorts before email, email before id:
age:   10
email: 01
id:    10
Packed: 10 01 10 00 → 0b10011000 → 0x98
```

**Expected wire format (hex):**
```
00 01       // flags=0x00 (Dense), version=1
98          // state map
19          // age=25 (TypeInt block, zigzag(25)=50=0x32... varint)
            // email: no bytes (state=01, null)
03 31 32 33 // id="123" (TypeString block)
```

**Total: 7 bytes**

---

#### A.4 Type 3: Batch, Row-Oriented

**Schema:** 3 fields: active (TypeBool), count (TypeInt), name (TypeString). 2 rows.

**Expected wire format (hex):**
```
02 01       // flags=0x02 (Batch, row-oriented), version=1
02          // record_count=2
00          // batch_flags: row-oriented (bit 0=0), dense (bit 1=0)

// Row 1: state map (all present) + values
FC          // state map: 10 11 11 00 → all 3 present (illustrative)
01          // active=true
0A          // count=10 (zigzag varint)
03 66 6F 6F // name="foo"

// Row 2
FC          // state map
00          // active=false
14          // count=20
03 62 61 72 // name="bar"
```

**Total: ~20 bytes**

---

#### A.5 Type 4: Stream, Row-Oriented

**Schema:** 3 fields, 247 rows in chunks of 100.

**Expected wire format (hex):**
```
03 01       // flags=0x03 (Stream), version=1
01          // extended_type=0x01 (Stream)
00          // stream_encoding: row-oriented, uncompressed

// Chunk 1
00          // chunk_descriptor: row-oriented dense
64          // row_count=100
[100 rows...]

// Chunk 2
00          // chunk_descriptor
64          // row_count=100
[100 rows...]

// Chunk 3
00          // chunk_descriptor
2F          // row_count=47
[47 rows...]

// End marker
00          // row_count=0
```

---

#### A.6 TypeContainer: Nested Document

**Schema (v1):** `user` (TypeContainer, schema=UserSchema v1), `score` (TypeInt)

**Input:** `{score: 99, user: {name: "Alice", age: 30}}`

**Expected wire format (hex):**
```
00 01           // flags=0x00 (Dense), version=1
[state map]     // 2 fields present

// TypeInt block:
63              // score=99 (zigzag varint)

// TypeContainer block:
[length varint] // byte length of nested packet
00 01           // nested Dense packet, UserSchema v1
[user state map + values]  // name="Alice", age=30 encoded per UserSchema
```

The nested packet is a fully valid Anansi packet encoded with the referenced sub-schema version.

---

### Appendix B: MIME Type

**Proposed MIME type:** `application/vnd.anansi.binary`
**File extension:** `.anansi` or `.anb`

---

### Appendix C: Schema Definition Wire Format

Schema definitions themselves can be encoded as Type 1 (Dense) packets using a hardcoded meta-schema version. This enables complete bootstrapping:

1. Client hardcodes meta-schema codec.
2. Server sends schemas encoded via meta-schema.
3. Client decodes schemas, compiles DAG decoders.
4. Data exchange begins.

---

### Appendix D: Extended Packet Types

When `flags & 0x03 == 0x03`, the `extended_type` byte selects the packet variant:

```
[flags: 0x03]
[schema_version: u8]
[extended_type: u8]
[extended_data...]

Current:
  0x01 = Stream

Reserved:
  0x02 = Schema negotiation
  0x03 = Partial update / patch
  0x04 = Delta encoding
  0x05 = Multi-schema message
  0x06–0xFF = Future extensions
```
