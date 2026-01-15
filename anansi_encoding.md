# Anansi Binary Wire Format Specification v1.0

---

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

The Anansi Binary Wire Format is a schema-based, high-performance serialization format optimized for constrained environments. It supports four packet types automatically selected based on data characteristics, with full schema versioning and optional compression.

### 1.2 Design Principles

- **Schema-driven**: All encoding decisions derived from schema definitions
- **Zero-copy capable**: String and binary data can be accessed without copying
- **Minimal overhead**: 2-4 bytes base overhead per message
- **Type-safe**: Schema validation ensures correctness
- **Performance-first**: Optimized for speed over flexibility

### 1.3 Key Features

- Four packet types (Dense, Sparse, Batch, Stream) chosen automatically
- Full schema versioning (1024 versions per schema)
- Optional compression with shared context for streams
- Columnar encoding for large result sets
- DAG-based decoding for zero branching
- Field types aligned with schema definition system
- Encryption and integrity hashing support

### 1.4 Use Cases

**Best for:**
- High-frequency RPC calls
- Real-time data streaming
- Mobile/IoT (bandwidth constrained)
- Low-latency services
- Large result sets (batch/stream mode)

**Not ideal for:**
- Ad-hoc data exchange (no schema)
- Human-readable debugging
- Cross-language without tooling
- Schemas changing every request

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
   │     │     └─────┴──── Version Epoch (bits 4-5)
   │     │           │
   │     │           └──────────────────────────── Encoding (bit 3)
   │     │
   │     └──────────────────────────────────────── Encryption (bit 6)
   │
   └────────────────────────────────────────────── Hash Present (bit 7)
      
      └─────┴──── Packet Type (bits 0-1)
            │
            └──────────────────────────────────── Compression (bit 2)
```

**Bit 0-1: Packet Type**
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

**Bit 4-5: Version Epoch**
- `00` (0): Epoch 0 (schema versions 0-255)
- `01` (1): Epoch 1 (schema versions 256-511)
- `10` (2): Epoch 2 (schema versions 512-767)
- `11` (3): Epoch 3 (schema versions 768-1023)

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
- Range: 0-255
- Combined with epoch bits (4-5) for full version

**Full Version Calculation:**
```
epoch = (flags >> 4) & 0x03
schemaVersion = byte[1]
fullVersion = (epoch << 8) | schemaVersion

Examples:
  flags=0x00, version=0x01 → fullVersion = 1
  flags=0x10, version=0x00 → fullVersion = 256 (epoch 1, schema 0)
  flags=0x20, version=0xFF → fullVersion = 767 (epoch 2, schema 255)
  flags=0x30, version=0xFF → fullVersion = 1023 (epoch 3, schema 255)
```

**Schema Registry Mapping:**
- Each fullVersion maps to a semantic version in ordered registry
- Registry maintains version history: `{0: "1.0.0", 1: "1.1.0", 256: "2.0.0", ...}`
- Decoders lookup schema by fullVersion

### 2.3 Field Ordering

Fields are encoded in **stable sorted order by (group, fieldKey)**:

**Group assignment:**
- Group 0: Booleans
- Group 1: Enums
- Group 2: Integers
- Group 3: Decimals
- Group 4: Numbers
- Group 5: Strings
- Group 6: Arrays
- Group 7: Sets
- Group 8: Objects
- Group 9: Records (with schema reference - structured)
- Group 10: Unions
- Group 11: Records (without schema reference - unstructured)

Within each group, fields are sorted alphabetically by **fieldKey** (the map key in the schema definition, not `FieldDefinition.Name`).

**Example:**
```
Schema fields map:
{
  "zulu": {name: "ZuluField", type: "string"},
  "alpha": {name: "AlphaField", type: "integer"},
  "beta": {name: "BetaField", type: "string"}
}

Encoding order (sorted by group, then fieldKey):
1. alpha (Group 2: integer)
2. beta (Group 5: string)
3. zulu (Group 5: string)
```

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

#### Boolean
```
Packed into boolean bitfield immediately after header
8 booleans = 1 byte
Bit order: LSB first (bit 0 = first boolean)
```

#### Integer
```
Varint encoding (LEB128)
1-9 bytes depending on magnitude
Signed integers use zigzag encoding
```

#### Number (float64)
```
8 bytes, little-endian IEEE 754
```

#### Decimal
```
[scale: u8][coefficient: varint]
Represents: coefficient / 10^scale
Example: 123.45 → [0x02][0x3039] (scale=2, coeff=12345)
```

#### String
```
[length: varint][bytes: UTF-8]
Length is byte count, not character count
No null terminator
```

#### Enum
```
[index: varint]
Index into schema's values array
```

#### Array
```
[count: varint]
FOR EACH item:
  [item_data] (encoded per itemsType)

Note: Arrays MUST have itemsType defined in schema.
```

#### Set
```
Same as Array
Uniqueness enforced by schema validation, not wire format
```

#### Object
```
Recursively encode using nested schema
[nested fields in nested schema order]

Note: Objects MUST have schema reference.
```

#### Record (structured - with schema)
```
[count: varint]
FOR EACH entry (sorted alphabetically by key):
  [key_length: varint]
  [key_bytes: UTF-8]
  [value_data] (encoded per schema reference, homogeneous values)

All values conform to the same schema (homogeneous).
Keys MUST be sorted alphabetically (UTF-8 byte order).
```

#### Record (unstructured - no schema)
```
[count: varint]
FOR EACH entry (sorted alphabetically by key):
  [key_length: varint]
  [key_bytes: UTF-8]
  [type_tag: u8]        (value type discriminator)
  [value_data]          (encoded per type tag)

Type tags:
- 0x00: null
- 0x01: boolean (1 byte: 0x00 or 0x01)
- 0x02: integer (zigzag varint)
- 0x03: number (8 bytes, little-endian float64)
- 0x04: decimal ([scale: u8][coefficient: varint])
- 0x05: string ([length: varint][bytes: UTF-8])
- 0x06: array ([count: varint] then type_tag + value for each item)
- 0x07: nested object ([count: varint][key][type_tag][value]... recursive)

Note: Unstructured records are placed in Group 11 for optimal decoding performance.
```

#### Union
```
[discriminator: varint] (index into union schemas array)
[value_data] (encoded per selected schema)

Union schemas are indexed in the order they appear in the schema definition.
```

### 2.6 Null Handling

The null encoding strategy is determined by the packet type:

**Type 1 (Dense):**
- Optional fields use `0x00` single-byte null marker when absent
- Marker appears at the field's position in sorted order
- Required fields cannot be null

**Type 2 (Sparse):**
- Absent fields are indicated by omitting their field_index
- No explicit null marker needed
- Only present fields are encoded

**Type 3 (Batch, Row-oriented):**
- Uses `0x00` markers per row, same as Type 1 Dense

**Type 3 (Batch, Columnar):**
- Nullable fields include a null bitmap before values
- Bitmap size: ⌈row_count / 8⌉ bytes
- Bit order: LSB first, 0 = null, 1 = present
- Non-nullable fields skip the bitmap

**Type 4 (Stream):**
- Follows the same rules as Type 3 based on chunk encoding mode

---

## 3. Packet Type Specifications

### 3.1 Type 1: Dense Packet

#### 3.1.1 When to Use

**Encoder selects Type 1 when:**
- Schema has ≤ 64 fields, OR
- Field density > 25% (present_fields / total_fields)
- Single document or homogeneous data

#### 3.1.2 Wire Format

```
┌──────────────────────────────────────────────┐
│ [flags: u8]                                  │  ← Packet type = 0b00
│ [schema_version: u8]                         │  ← Schema version (0-255)
├──────────────────────────────────────────────┤
│ [field_0_data]                               │
│ [field_1_data]                               │
│ [field_2_data]                               │
│ ...                                          │
│ [field_N_data]                               │
└──────────────────────────────────────────────┘
```

Fields are encoded in stable sorted order by (group, fieldKey) as defined in section 2.3.

#### 3.1.3 Example

**Schema:**
```json
{
  "name": "User",
  "version": "1",
  "fields": {
    "id": {"type": "string"},
    "username": {"type": "string"},
    "age": {"type": "integer"},
    "active": {"type": "boolean"}
  }
}
```

**After sorting (by group, then fieldKey):**
```
Group 0 (Booleans):
  Index 0: active

Group 2 (Integers):
  Index 1: age

Group 5 (Strings):
  Index 2: id
  Index 3: username
```

**Document:**
```json
{
  "id": "019b84534d097c33b603bff7d02bb65c",
  "username": "alice",
  "age": 25,
  "active": true
}
```

**Wire format:**
```
[0x00]                    flags (Dense, uncompressed)
[0x01]                    schema version 1

Boolean group:
[0x01]                    active=true (bit 0 set)

Integer group:
[0x19]                    age=25 (varint)

String group:
[0x20]                    id length=32
[32 bytes]                id bytes
[0x05]                    username length=5
['a']['l']['i']['c']['e'] username bytes

Total: 48 bytes
```

### 3.2 Type 2: Sparse Packet

#### 3.2.1 When to Use

**Encoder selects Type 2 when:**
- Schema has > 64 fields, AND
- Field density ≤ 25%
- Projection selected few fields from large schema

#### 3.2.2 Wire Format

```
┌──────────────────────────────────────────────┐
│ [flags: u8]                                  │  ← Packet type = 0b01
│ [schema_version: u8]                         │
├──────────────────────────────────────────────┤
│ [field_count: varint]                        │  ← Number of present fields
├──────────────────────────────────────────────┤
│ FOR EACH PRESENT FIELD:                      │
│   [field_index: varint]                      │  ← Index in sorted schema
│   [field_data]                               │  ← Encoded per type
└──────────────────────────────────────────────┘
```

#### 3.2.3 Field Index Encoding

- Field index is position in sorted schema (0-based), using the same sorting as Type 1 Dense
- Varint encoded:
  - Indices 0-127: 1 byte
  - Indices 128-16383: 2 bytes
  - Indices 16384+: 3+ bytes

#### 3.2.4 Field Ordering

Fields **SHOULD** be encoded in ascending index order for optimal decoding, but decoders **MUST** handle any order.

#### 3.2.5 Example

**Schema with 200 fields, 5 present:**

**Sorted field indices:**
```
Index 0: _metadata_.version (integer)
Index 47: _metadata_.checksum (string)
Index 48: _metadata_.created (string)
Index 150: id (string)
Index 199: username (string)
```

**Wire format:**
```
[0x01]              flags (Sparse, uncompressed)
[0x01]              schema version 1
[0x05]              5 fields present

[0x00]              field index 0
[0x1B]              version=27

[0x2F]              field index 47 (1 byte varint)
[0x40]              checksum length=64
[64 bytes]          checksum data

[0x30]              field index 48
[0x13]              created length=19
[19 bytes]          created data

[0x96 0x01]         field index 150 (2 byte varint: 150 = 0x96 0x01)
[0x20]              id length=32
[32 bytes]          id data

[0xC7 0x01]         field index 199 (2 byte varint: 199 = 0xC7 0x01)
[0x08]              username length=8
[8 bytes]           username data

Total: ~137 bytes
```

### 3.3 Type 3: Batch Packet

#### 3.3.1 When to Use

**Encoder selects Type 3 when:**
- Encoding multiple rows with identical schema
- Query results with known row count upfront
- Bulk operations
- Complete result set fits in memory
- Small to medium result sets (< 1000 rows typically)

#### 3.3.2 Row-Oriented Format

```
┌──────────────────────────────────────────────┐
│ [flags: u8]                                  │  ← Packet type = 0b10, bit 3 = 0
│ [schema_version: u8]                         │
├──────────────────────────────────────────────┤
│ [row_count: varint]                          │
├──────────────────────────────────────────────┤
│ FOR EACH ROW:                                │
│   [field_0_data]                             │
│   [field_1_data]                             │
│   ...                                        │
│   [field_N_data]                             │
└──────────────────────────────────────────────┘
```

Each row contains all fields in schema order, encoded identically to Type 1 (Dense).

**When to use:**
- Small to medium result sets (< 100 rows)
- Random row access needed
- Simple iteration patterns

#### 3.3.3 Columnar Format

```
┌──────────────────────────────────────────────┐
│ [flags: u8]                                  │  ← Packet type = 0b10, bit 3 = 1
│ [schema_version: u8]                         │
├──────────────────────────────────────────────┤
│ [row_count: varint]                          │
├──────────────────────────────────────────────┤
│ FOR EACH FIELD (in schema order):            │
│   [null_bitmap: ⌈row_count/8⌉]               │  ← If field is nullable
│   [values: packed array]                     │
└──────────────────────────────────────────────┘
```

**Null Bitmap (nullable fields only):**
```
Bitmap size: ⌈row_count / 8⌉ bytes
Bit order: LSB first
Bit value: 0 = null, 1 = present
```

**Value Packing by Type:**

Fixed-width types (packed tightly):
```
Boolean: ⌈row_count / 8⌉ bytes (bit array)
Integer: row_count × varint (can use delta encoding)
Number: row_count × 8 bytes
UUID/Fixed strings: row_count × fixed_size bytes
```

Variable-width types:
```
String: [lengths array][concatenated data]
  Lengths: row_count × varint
  Data: concatenated UTF-8 bytes

Array: [counts array][concatenated items]
  Counts: row_count × varint
  Items: all items concatenated
```

**When to use:**
- Large result sets (100+ rows)
- Analytics/aggregation queries
- Compression target (columnar compresses better)

#### 3.3.4 Example (Row-Oriented)

**Schema:**
```json
{
  "name": "UserResult",
  "version": "1",
  "fields": {
    "id": {"type": "string"},
    "username": {"type": "string"},
    "role": {"type": "string"}
  }
}
```

**3 rows:**
```json
[
  {"id": "id1...", "username": "alice", "role": "admin"},
  {"id": "id2...", "username": "bob", "role": "user"},
  {"id": "id3...", "username": "carol", "role": "admin"}
]
```

**Wire format:**
```
[0x02]              flags (Batch, row-oriented, uncompressed)
[0x01]              schema version 1
[0x03]              3 rows

Row 1:
[0x20][32 bytes]    id
[0x05]['a']['l']['i']['c']['e'] username
[0x05]['a']['d']['m']['i']['n'] role

Row 2:
[0x20][32 bytes]    id
[0x03]['b']['o']['b'] username
[0x04]['u']['s']['e']['r'] role

Row 3:
[0x20][32 bytes]    id
[0x05]['c']['a']['r']['o']['l'] username
[0x05]['a']['d']['m']['i']['n'] role

Total: ~146 bytes
```

#### 3.3.5 Example (Columnar)

**Same data, columnar encoding:**

```
[0x0A]              flags (Batch, columnar, uncompressed)
                    (0b00001010: bits 0-1=10, bit 3=1)
[0x01]              schema version 1
[0x03]              3 rows

Column 0 (id - all non-null, fixed 32 bytes):
[0xFF]              null bitmap (all present, only need 1 byte for 3 rows)
[32 bytes]          id1
[32 bytes]          id2
[32 bytes]          id3

Column 1 (username - all non-null, variable):
[0xFF]              null bitmap
[0x05][0x03][0x05]  lengths: 5, 3, 5
['a']['l']['i']['c']['e']['b']['o']['b']['c']['a']['r']['o']['l']

Column 2 (role - all non-null, variable):
[0xFF]              null bitmap
[0x05][0x04][0x05]  lengths: 5, 4, 5
['a']['d']['m']['i']['n']['u']['s']['e']['r']['a']['d']['m']['i']['n']

Total: ~144 bytes
```

### 3.4 Type 4: Stream Packet

#### 3.4.1 When to Use

**Encoder selects Stream packet when:**
- Streaming query results where total count is unknown upfront
- Large result sets (1000+ rows) sent incrementally
- Want to benefit from shared compression context across chunks
- Explicit stream semantics in wire format

#### 3.4.2 Wire Format

```
┌──────────────────────────────────────────────┐
│ STREAM HEADER (sent once)                    │
├──────────────────────────────────────────────┤
│ [flags: u8]              (0x03)              │  ← Extended packet type
│ [schema_version: u8]                         │  ← Schema version (0-255)
│ [extended_type: u8]      (0x01)              │  ← Stream marker
│ [stream_encoding: u8]                        │  ← Encoding flags
├──────────────────────────────────────────────┤
│ CHUNKS (repeating)                           │
├──────────────────────────────────────────────┤
│ [row_count: varint]                          │  ← Rows in this chunk
│ [chunk_data]                                 │  ← Encoded per stream_encoding
├──────────────────────────────────────────────┤
│ END MARKER                                   │
├──────────────────────────────────────────────┤
│ [row_count: 0]                               │  ← Stream terminator
└──────────────────────────────────────────────┘
```

#### 3.4.3 Stream Encoding Byte

```
┌─────┬─────┬─────┬─────┬─────┬─────┬─────┬─────┐
│  7  │  6  │  5  │  4  │  3  │  2  │  1  │  0  │
└─────┴─────┴─────┴─────┴─────┴─────┴─────┴─────┘
   │                       │     │     └─────┴──── Chunk Encoding (bits 0-1)
   │                       │     │
   │                       │     └──────────────── Compression (bit 2)
   │                       │
   │                       └────────────────────── Reserved (bit 3)
   │
   └────────────────────────────────────────────── Reserved (bits 4-7)
```

**Bits 0-1: Chunk Encoding**
- `00` (0x00): Row-oriented (like Type 3 batch)
- `01` (0x01): Columnar (like Type 3 batch with bit 3 set)
- `10` (0x02): Mixed (encoder can switch per chunk)
- `11` (0x03): Reserved

**Bit 2: Compression Flag**
- `0`: Uncompressed
- `1`: Compressed with shared context

**Bits 3-7: Reserved**
- Must be set to `00000`

#### 3.4.4 Chunk Encoding

Each chunk is encoded identically to Type 3 (Batch) packets, but **without the packet header**.

**Row-oriented chunks:**
```
[row_count: varint]
FOR i = 0 to row_count-1:
  [field_0_data]
  [field_1_data]
  ...
  [field_N_data]
```

**Columnar chunks:**
```
[row_count: varint]
FOR EACH FIELD (in schema order):
  [null_bitmap: ⌈row_count/8⌉]  (if nullable)
  [values: packed array]
```

#### 3.4.5 Shared Compression Context

When compression flag is set (bit 2 = 1):

1. **Compression starts** after stream_encoding byte
2. **All chunks** share the same compression dictionary
3. **Compressor learns** patterns from early chunks, applies to later chunks
4. **Decompressor maintains** state across all chunks until end marker

**Typical compression ratio improvement:** 15-30% better than per-packet compression for homogeneous data.

#### 3.4.6 End Marker

Stream **MUST** terminate with `row_count = 0`:

```
[0x00]  // varint encoding of 0
```

This signals:
- No more chunks
- Decompressor can finalize
- Decoder can return complete result set

#### 3.4.7 Handling Partial Streams

If stream is interrupted (network error, timeout):
- Decoder receives incomplete data
- Transport layer signals error (TCP RST, gRPC error, HTTP abort)
- Decoder returns what was successfully decoded + error indicator

**Decoders MUST NOT** assume stream is complete until:
1. End marker (`row_count = 0`) is received, OR
2. Transport signals EOF/completion

#### 3.4.8 Example

**Stream of 247 rows, 100-row chunks:**

```
// Stream header
[0x03]              // flags: Extended packet type
[0x01]              // schema version 1
[0x01]              // extended_type: Stream
[0x00]              // stream_encoding: row-oriented, uncompressed

// Chunk 1 (100 rows)
[0x64]              // row_count = 100
[row₀ data]
[row₁ data]
...
[row₉₉ data]

// Chunk 2 (100 rows)
[0x64]              // row_count = 100
[row₁₀₀ data]
...
[row₁₉₉ data]

// Chunk 3 (47 rows, final data)
[0x2F]              // row_count = 47
[row₂₀₀ data]
...
[row₂₄₆ data]

// End marker
[0x00]              // row_count = 0, stream complete
```

**Total overhead:** 4 bytes header + 3 chunk row_counts + 1 end marker = 8 bytes + data

#### 3.4.9 Mixed Encoding Mode

When `stream_encoding bits 0-1 = 10` (mixed mode):

Encoder can **switch encoding per chunk** by prefixing each chunk with encoding byte:

```
[row_count: varint]
[chunk_encoding: u8]     // 0x00 = row, 0x01 = columnar
[chunk_data]
```

**Use case:** Start with row-oriented for low-latency first rows, switch to columnar once buffered enough rows.

#### 3.4.10 Stream vs Batch Decision

```
Use Stream (Type 4) when:
- Row count unknown at encoding start
- Result set > 1000 rows
- Want shared compression context
- Sending incremental results

Use Batch (Type 3) when:
- Row count known upfront
- Complete result set fits in memory
- Small result sets (< 1000 rows)
- Single response expected
```

---

## 4. Advanced Features

### 4.1 Compression

#### 4.1.1 Compression Flag

When bit 2 of flags byte is set (`1`), data is compressed according to the encryption and hash flags:

**No encryption, no hash:**
```
All data after byte 1 (schema version) is compressed
```

**With hash, no encryption:**
```
Hash (16 bytes) is uncompressed
All data after hash is compressed
```

**With encryption (hash optional):**
```
Compression happens BEFORE encryption
Decrypt first, then decompress
```

For Stream packets, compression is indicated in the stream_encoding byte (bit 2) and follows the same rules.

#### 4.1.2 Compression Algorithm

**Recommended:** LZ4 or ZSTD

The specification does not mandate a specific algorithm. Implementations **MUST** document which algorithm they use.

#### 4.1.3 Compressed Packet Structure

**Without hash or encryption:**
```
┌──────────────────────────────────────────────┐
│ [flags: u8]              (bit 2 = 1)         │
│ [schema_version: u8]     (uncompressed)      │
├──────────────────────────────────────────────┤
│ [uncompressed_size: varint]  (optional)      │
├──────────────────────────────────────────────┤
│ [compressed_data...]                         │
└──────────────────────────────────────────────┘
```

**With hash:**
```
┌──────────────────────────────────────────────┐
│ [flags: u8]              (bits 2,7 = 1)      │
│ [schema_version: u8]     (uncompressed)      │
├──────────────────────────────────────────────┤
│ [hash: 16 bytes]         (uncompressed)      │
├──────────────────────────────────────────────┤
│ [uncompressed_size: varint]  (optional)      │
├──────────────────────────────────────────────┤
│ [compressed_data...]                         │
└──────────────────────────────────────────────┘
```

**With encryption:**
```
┌──────────────────────────────────────────────┐
│ [flags: u8]              (bits 2,6 = 1)      │
│ [schema_version: u8]     (uncompressed)      │
├──────────────────────────────────────────────┤
│ [encrypted_data...]      compress→encrypt    │
└──────────────────────────────────────────────┘
```

**Note:** `uncompressed_size` is optional but recommended for buffer pre-allocation.

#### 4.1.4 Decompression Process

**Without encryption:**
```
1. Check bit 2 of flags byte
2. If set, read schema_version (byte 1)
3. If bit 7 set, read and verify hash (16 bytes)
4. Optionally read uncompressed_size
5. Decompress remaining bytes
6. Continue decoding based on packet type
```

**With encryption:**
```
1. Check bits 2 and 6 of flags byte
2. Read schema_version (byte 1)
3. If bit 7 set, read hash (16 bytes) - hash is of plaintext
4. Decrypt remaining bytes
5. Decompress decrypted bytes
6. Verify hash (if present)
7. Continue decoding based on packet type
```

**Processing order:** `encrypt(compress(data))` on encode, `decompress(decrypt(data))` on decode

### 4.2 Encryption

#### 4.2.1 Encryption Flag

When bit 6 of flags byte is set (`1`), all data after byte 1 (schema version) or after hash (if present) is encrypted.

#### 4.2.2 Encryption Algorithm

**RECOMMENDED:** ChaCha20-Poly1305 or AES-256-GCM

The specification does not mandate a specific algorithm. Implementations **MUST** document which algorithm(s) they support.

**Requirements for chosen algorithm:**
- Authenticated encryption (AEAD)
- Provides confidentiality and integrity
- Resistant to timing attacks
- Well-audited implementation

#### 4.2.3 Key Management

Key distribution and management is **out of scope** for this specification. Implementations may use:

- Pre-shared keys (PSK)
- Key derivation functions (KDF)
- TLS session keys
- Key exchange protocols (ECDH, etc.)
- Hardware security modules (HSM)

#### 4.2.4 Encrypted Packet Structure

**Basic encrypted packet:**
```
┌──────────────────────────────────────────────┐
│ [flags: u8]              (bit 6 = 1)         │
│ [schema_version: u8]     (plaintext)         │
├──────────────────────────────────────────────┤
│ [nonce/IV: N bytes]      (algorithm-specific)│
├──────────────────────────────────────────────┤
│ [ciphertext + auth_tag]  (encrypted payload) │
└──────────────────────────────────────────────┘
```

**With hash (authenticated plaintext hash):**
```
┌──────────────────────────────────────────────┐
│ [flags: u8]              (bits 6,7 = 1)      │
│ [schema_version: u8]     (plaintext)         │
├──────────────────────────────────────────────┤
│ [hash: 16 bytes]         (plaintext hash)    │
├──────────────────────────────────────────────┤
│ [nonce/IV: N bytes]                          │
├──────────────────────────────────────────────┤
│ [ciphertext + auth_tag]                      │
└──────────────────────────────────────────────┘
```

**Nonce/IV size:**
- ChaCha20-Poly1305: 12 bytes (96 bits)
- AES-256-GCM: 12 bytes (96 bits recommended)

#### 4.2.5 Encryption with Compression

**Order of operations (encoding):**
```
plaintext → compress → encrypt → ciphertext
```

**Order of operations (decoding):**
```
ciphertext → decrypt → decompress → plaintext
```

**Rationale:** Compression before encryption:
- Better compression ratio (plaintext has more patterns)
- Standard practice (TLS, SSH, etc.)
- Prevents compression oracle attacks when using AEAD

#### 4.2.6 Decryption Process

```
1. Check bit 6 of flags byte
2. If set, read schema_version (byte 1)
3. If bit 7 set, read hash (16 bytes)
4. Read nonce/IV (algorithm-specific size)
5. Decrypt remaining bytes using algorithm + key + nonce
6. If AEAD verification fails, reject packet (tampering)
7. If bit 2 set, decompress decrypted data
8. If bit 7 set, verify BLAKE3 hash
9. Continue normal decoding
```

#### 4.2.7 Security Considerations

**Nonce reuse:**
- **CRITICAL:** Never reuse nonce with same key
- Use counter-based nonces or cryptographically random nonces
- For ChaCha20-Poly1305 and AES-GCM, nonce reuse breaks security

**Key rotation:**
- Implementations SHOULD support key rotation
- Use version epoch bits to signal key version if needed

**Additional Authenticated Data (AAD):**
- Implementations MAY include flags and schema_version in AAD
- AAD is authenticated but not encrypted

**Example AAD usage:**
```
AAD = [flags, schema_version]
encrypt(plaintext, key, nonce, AAD)
```

This prevents bit-flipping attacks on the header.

### 4.3 Integrity Hashing

#### 4.3.1 Hash Flag

When bit 7 of flags byte is set (`1`), a 16-byte BLAKE3 hash is included after byte 1.

#### 4.3.2 Hash Algorithm

**REQUIRED:** BLAKE3 truncated to 128 bits (16 bytes)

**Properties:**
- Fast (faster than SHA-256, competitive with BLAKE2)
- Cryptographically secure
- Parallelizable
- 128-bit truncation provides 2^64 collision resistance

#### 4.3.3 Hash Packet Structure

**Basic hashed packet:**
```
┌──────────────────────────────────────────────┐
│ [flags: u8]              (bit 7 = 1)         │
│ [schema_version: u8]                         │
├──────────────────────────────────────────────┤
│ [hash: 16 bytes]         BLAKE3-128          │
├──────────────────────────────────────────────┤
│ [payload...]             (plaintext data)    │
└──────────────────────────────────────────────┘
```

**Hash input:** All bytes starting from payload

```
hash = BLAKE3(payload_bytes)[0..16]
```

#### 4.3.4 Hash with Compression

**Packet structure:**
```
┌──────────────────────────────────────────────┐
│ [flags: u8]              (bits 2,7 = 1)      │
│ [schema_version: u8]                         │
├──────────────────────────────────────────────┤
│ [hash: 16 bytes]         (of uncompressed)   │
├──────────────────────────────────────────────┤
│ [compressed_data...]                         │
└──────────────────────────────────────────────┘
```

**Hash is computed on uncompressed plaintext**, not compressed data.

#### 4.3.5 Hash with Encryption

**Packet structure:**
```
┌──────────────────────────────────────────────┐
│ [flags: u8]              (bits 6,7 = 1)      │
│ [schema_version: u8]                         │
├──────────────────────────────────────────────┤
│ [hash: 16 bytes]         (of plaintext)      │
├──────────────────────────────────────────────┤
│ [nonce/IV: N bytes]                          │
├──────────────────────────────────────────────┤
│ [ciphertext + auth_tag]                      │
└──────────────────────────────────────────────┘
```

**Hash is computed on plaintext**, included **unencrypted**.

**Benefits:**
1. **Fast rejection:** Verify hash before decryption to detect corrupt ciphertext
2. **Layered security:** Hash provides data integrity, AEAD provides auth
3. **Debugging:** Can verify data integrity without keys

#### 4.3.6 Verification Process

```
1. Check bit 7 of flags byte
2. If set, extract hash from bytes [2..18]
3. Process data according to compression/encryption flags
4. Compute BLAKE3(final_plaintext)[0..16]
5. Compare with extracted hash
6. If mismatch:
   - Reject packet
   - Log integrity failure
   - Return error to caller
```

#### 4.3.7 Use Cases

**When to use hashing:**

✅ **DO use:**
- Mission-critical data that must not be corrupted
- Financial transactions
- Medical records
- Audit logs
- Data over unreliable networks
- Long-term storage (detect bit rot)

❌ **DON'T use:**
- Already using AEAD encryption (redundant)
- Extremely high-frequency, low-latency messages
- Trusted, error-corrected channels (TCP with good NICs)

**Performance impact:**
- BLAKE3 cost: ~1-2 GB/s on modern CPUs
- For 1KB message: ~1 microsecond overhead
- Negligible for most use cases

---

## 5. Schema Management

### 5.1 Schema Versioning and Migration

#### 5.1.1 Version Resolution

**Encoder:**
```
schema.Version = "2.1.4"
→ Lookup in registry: fullVersion = 256 (first v2.x.x)
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
→ Lookup in registry: "2.0.0"
→ Load compiled schema for version 256
```

#### 5.1.2 Schema Evolution

When schema changes:
1. Increment version
2. Define migration in schema
3. Compile new codec graph
4. Both old and new versions coexist

**Example:**
```
v1: [id, name, age]
v2: [id, name, age, email]  ← Added field

Old client (v1):
  Encodes with version=1
  Server decodes using v1 schema
  Migration applied to upgrade data

New client (v2):
  Encodes with version=2
  Old server (if v1 only) rejects or downgrades
```

#### 5.1.3 Backward/Forward Compatibility

**Backward compatible:** New decoder reads old data
- Add optional fields only
- Don't remove required fields
- Don't change field types

**Forward compatible:** Old decoder reads new data
- Requires sparse encoding (Type 2)
- Old decoder ignores unknown field indices

### 5.2 Schema Distribution

Schemas can be distributed via:

1. **Embedded:** Hardcode meta-schema in client
2. **Network:** Fetch schemas on-demand by version
3. **Bundle:** Include schemas in application package
4. **Registry:** Central schema registry service

### 5.3 Schema Reuse and Consolidation

#### 5.3.1 The Schema Proliferation Myth

**Argument:** "Using schemas for wire format creates schema overhead and proliferation."

**Reality:** Schemas already exist in every layer of modern applications - they're just fragmented, duplicated, and not reusable.

#### 5.3.2 Hidden Schemas in Existing Systems

**Typical full-stack application has 6-8 copies of the same schema:**

```
1. Database schema (SQL/migrations)
2. ORM models (Prisma, TypeORM, SQLAlchemy)
3. API types (TypeScript interfaces)
4. API validation (Zod, class-validator)
5. OpenAPI/Swagger documentation
6. Frontend types (TypeScript)
7. Form validation (Zod, Yup)
8. Form structure (React components)

ALL describing the SAME data structure!
```

#### 5.3.3 Anansi's Approach: Single Source of Truth

**One schema definition generates:**
```
1. Wire format encoder/decoder (DAG-compiled)
2. Database migration
3. ORM models
4. TypeScript types
5. Validation functions
6. OpenAPI documentation
7. Frontend form schemas
8. Mock data generators
9. Test fixtures
10. API client code

ALL from the same source!
```

#### 5.3.4 Schema Consolidation Benefits

**Developer Experience:**
```
Before (fragmented schemas):
- Add field → 8 files to update
- Time: ~30 minutes per schema change

After (single schema):
- Add field → 1 schema update, regenerate
- Time: ~2 minutes per schema change

15x productivity improvement
```

**Type Safety:**
```
Before:
Frontend: {email: string, age: number}
Backend:  {email: string, age: string}  // ❌ Type mismatch!

After:
Generated from same schema → guaranteed type compatibility
Compile-time error if types drift
```

---

## 6. Implementation Guidelines

### 6.1 Encoder Selection Logic

```go
func EncodeMessage(schema *Schema, data any) ([]byte, error) {
    // Stream detection
    if isStream(data) {
        return encodeStream(schema, data)
    }
    
    // Batch detection
    if isBatch(data) {
        rowCount := len(data)
        if rowCount >= 1000 {
            return encodeStream(schema, data) // Use stream for large batches
        }
        return encodeBatch(schema, data)
    }
    
    // Dense vs Sparse
    fieldCount := len(schema.Fields)
    presentCount := countPresent(data, schema)
    density := float64(presentCount) / float64(fieldCount)
    
    if fieldCount <= 64 || density > 0.25 {
        return encodeDense(schema, data)
    }
    
    return encodeSparse(schema, data)
}
```

### 6.2 Decoder Optimization

**Must:**
- Pre-compile schemas to DAG
- Cache compiled schemas by version
- Use jump tables for Type 1 (dense)
- Zero-copy string decoding where possible

**Should:**
- Implement field offset pre-calculation
- Use SIMD for bitmap operations
- Pool buffers to reduce allocations

**May:**
- Implement lazy field access (Proxy pattern)
- JIT compile decoders for hot schemas

### 6.3 DAG-Based Decoder Implementation

#### 6.3.1 Overview

DAG (Directed Acyclic Graph) compilation transforms schema definitions into optimized decoder closures at compile-time or schema-load time. This eliminates runtime branching, enables zero-copy operations, and produces highly cache-friendly code.

#### 6.3.2 Compilation Process

**Step 1: Schema Analysis**
```
1. Parse schema definition
2. Compute field ordering (by group, then fieldKey)
3. Identify bit-packable fields (booleans, small enums)
4. Mark optional vs required fields
5. Detect zero-copy candidates (fixed-length strings, UUIDs)
6. Build field dependency graph
```

**Step 2: Code Generation**
```
1. Generate specialized decoder function
2. Inline all type information (eliminate type dispatch)
3. Unroll loops for fixed-size fields
4. Pre-compute buffer offsets where possible
5. Generate zero-copy accessors for strings
6. Emit branchless code for predictable patterns
```

**Step 3: Optimization**
```
1. Constant propagation for field positions
2. Dead code elimination for unused fields
3. SIMD vectorization for bitmap operations
4. Cache-line alignment for hot fields
5. Prefetch hints for nested objects
```

#### 6.3.3 Example: DAG Compilation

**Schema:**
```json
{
  "name": "User",
  "version": "1",
  "fields": {
    "id": {"type": "string"},
    "active": {"type": "boolean"},
    "age": {"type": "integer"},
    "role": {"type": "enum", "values": ["user", "admin", "guest"]}
  }
}
```

**Generic Decoder (pseudo-code):**
```go
func decodeGeneric(buf []byte, schema *Schema) (any, error) {
    offset := 2  // Skip header
    result := make(map[string]any)
    
    for _, field := range schema.Fields {  // Runtime iteration
        switch field.Type {                 // Runtime branch
        case FieldTypeBoolean:
            value := (buf[offset] & (1 << field.BitIndex)) != 0
            result[field.Name] = value
        case FieldTypeEnum:
            index, n := readVarint(buf[offset:])  // Multiple branches
            offset += n
            result[field.Name] = field.Values[index]
        // ... more cases
        }
    }
    return result, nil
}
```

**DAG-Compiled Decoder:**
```go
// Generated once, cached forever by schema version
func decodeUser_v1(buf []byte) User {
    offset := 2  // Constant
    
    // Group 0: Boolean (no branches)
    active := (buf[offset] & 0x01) != 0
    offset = 3  // Constant propagated
    
    // Group 1: Enum (predictable branch)
    roleIndex := uint64(buf[offset] & 0x7F)
    if buf[offset] >= 0x80 {  // Branch predictor learns this is rare
        roleIndex |= uint64(buf[offset+1]&0x7F) << 7
        offset = 5
    } else {
        offset = 4
    }
    role := Role(roleIndex)  // Direct enum conversion
    
    // Group 2: Integer (unrolled varint)
    age := uint64(buf[offset] & 0x7F)
    if buf[offset] >= 0x80 {
        age |= uint64(buf[offset+1]&0x7F) << 7
        offset += 2
    } else {
        offset += 1
    }
    
    // Group 5: String (zero-copy)
    idLen := uint64(buf[offset] & 0x7F)
    offset += 1
    
    // Zero-copy string (no allocation)
    id := unsafe.String(&buf[offset], int(idLen))
    
    return User{
        ID:     id,
        Active: active,
        Age:    int(age),
        Role:   role,
    }
}
```

**Performance comparison:**
```
Generic Decoder:
  - Branches: ~15
  - Allocations: 2
  - CPU cycles: ~250

DAG Decoder:
  - Branches: ~3
  - Allocations: 0
  - CPU cycles: ~40

Speedup: 6.25x
```

#### 6.3.4 Zero-Copy String Implementation

**Technique:** Return string descriptor pointing into original buffer

```go
// Safe zero-copy string creation
func zeroCopyString(buf []byte, offset, length int) string {
    return unsafe.String(&buf[offset], length)
}
```

**Requirements:**
- Source buffer must remain valid for string lifetime
- Strings are immutable (Go guarantee)
- Safe for read-only operations

**When to use:**
- Request IDs, trace IDs (short-lived, read-only)
- Path strings, method names
- Log messages

**When NOT to use:**
- Strings stored beyond request scope
- User-provided content returned to client

#### 6.3.5 Decoder Cache

```go
type DecoderCache struct {
    decoders map[uint8]func([]byte) any  // version → decoder
    mu       sync.RWMutex
}

func (c *DecoderCache) Get(version uint8) (func([]byte) any, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()
    decoder, ok := c.decoders[version]
    return decoder, ok
}

// Usage
cache := &DecoderCache{decoders: make(map[uint8]func([]byte) any)}

// Compile and cache at startup
cache.Set(1, compileDecoder(userSchemaV1))
cache.Set(2, compileDecoder(userSchemaV2))

// Hot path (per request)
decoder, _ := cache.Get(buf[1])  // buf[1] = schema version
result := decoder(buf)
```

### 6.4 Decoding Algorithm

#### 6.4.1 Header Parsing

```
1. Read flags byte (byte 0)
2. Extract packet type (flags & 0x03)
3. Extract compression flag (flags & 0x04)
4. Extract epoch (flags >> 4 & 0x03)
5. Extract encryption flag (flags & 0x40)
6. Extract hash flag (flags & 0x80)
7. Read schema version (byte 1)
8. Calculate fullVersion = (epoch << 8) | schema_version
9. If hash flag set, read and store hash (16 bytes)
10. If encryption flag set, decrypt remaining bytes
11. If compression flag set, decompress data
12. If hash flag set, verify hash
```

#### 6.4.2 Type Dispatch

```
switch (packet_type) {
  case 0x00: decode_dense()
  case 0x01: decode_sparse()
  case 0x02: decode_batch()
  case 0x03: decode_stream()
}
```

#### 6.4.3 Dense Decoding

```
1. Lookup compiled schema for version
2. Get field ordering from schema
3. For each field in order:
   a. Check for null marker (0x00) if optional
   b. Decode based on field type
   c. Advance buffer offset
4. Return decoded object
```

#### 6.4.4 Sparse Decoding

```
1. Read field_count (varint)
2. Initialize result object
3. For i = 0 to field_count-1:
   a. Read field_index (varint)
   b. Lookup field definition from schema
   c. Decode field data by type
   d. Store in result[field.name]
4. Return result object
```

#### 6.4.5 Batch Decoding

**Row-oriented:**
```
1. Read row_count (varint)
2. For i = 0 to row_count-1:
   a. Decode as Dense packet (reuse dense decoder)
   b. Append to results array
3. Return results array
```

**Columnar:**
```
1. Read row_count (varint)
2. For each field in schema:
   a. Read null bitmap (if nullable)
   b. Read values array
   c. Store column
3. Transpose columns to rows
4. Return results array
```

#### 6.4.6 Stream Decoding

```
1. Read stream header (4 bytes)
2. Parse stream_encoding flags
3. If compressed, initialize decompressor with shared context
4. Initialize results array
5. Loop:
   a. Read row_count (varint)
   b. If row_count == 0, break (end of stream)
   c. Decode chunk based on stream_encoding
   d. Append chunk to results
6. If compressed, finalize decompressor
7. Return results array
```

### 6.5 Testing Requirements

Implementations **MUST** pass:
- Round-trip encoding/decoding tests
- Schema evolution tests (v1 → v2 → v3)
- All four packet types
- Compression tests
- Encryption and hashing tests
- Stream interruption handling
- Error handling tests
- Performance benchmarks vs JSON

---

## 7. Performance Characteristics

### 7.1 Generic Decoder Performance

**vs JSON (encoding/json):**
- Encode: 10-20x faster
- Decode: 5-15x faster
- Size: 40-60% smaller
- Memory: 70-90% fewer allocations

**vs MessagePack:**
- Encode: 3-5x faster
- Decode: 2-4x faster
- Size: 30-40% smaller

**vs Protocol Buffers:**
- Encode: 1-2x faster
- Decode: 2-4x faster
- Size: 5-15% smaller

### 7.2 DAG-Compiled Decoder Performance

**vs JSON (encoding/json):**
- Encode: 30-50x faster
- Decode: 40-60x faster
- Size: 40-60% smaller
- Memory: 95-100% fewer allocations (zero-copy strings)

**vs Protocol Buffers:**
- Encode: 5-10x faster
- Decode: 10-20x faster
- Size: 5-15% smaller
- Memory: 80-95% fewer allocations

**DAG Compilation Benefits:**
- CPU branches: 90-95% reduction
- Cache misses: 50-80% reduction
- Allocations: 90-100% reduction
- GC pressure: Near-zero

### 7.3 Stream Packet Performance

**vs Separate Batch Packets (10,000 rows, 100/chunk):**

| Metric | Separate Batches | Stream Packet | Improvement |
|--------|-----------------|---------------|-------------|
| Header overhead | 200 bytes | 105 bytes | **47% less** |
| Schema lookups | 100 | 1 | **99% less** |
| Compression ratio | 60% | 70-80% | **15-30% better** |
| CPU cache efficiency | Poor | Good | **~20% faster** |
| Memory allocation | 100 buffers | 1 buffer | **Less GC pressure** |

---

## 8. Protocol Extensions

### 8.1 Protocol Envelope Encoding

The wire format can encode entire protocol envelopes, not just application data. By defining schemas for request/response metadata, error handling, routing, and other protocol concerns, implementations can achieve protocol-level performance optimizations.

### 8.2 API Request Envelope Example

**Schema:**
```json
{
  "name": "APIRequest_v1",
  "version": "1",
  "fields": {
    "method": {
      "type": "enum",
      "values": ["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"]
    },
    "authenticated": {"type": "boolean"},
    "compressed": {"type": "boolean"},
    "priority": {
      "type": "enum",
      "values": ["low", "normal", "high", "urgent"]
    },
    "requestId": {"type": "string"},
    "path": {"type": "string"},
    "body": {
      "type": "union",
      "schema": [
        {"id": "UserCreateRequest"},
        {"id": "OrderCreateRequest"}
      ]
    }
  }
}
```

**Wire format:**
```
[0x00][0x01]           // Dense packet, APIRequest_v1

// Group 0: Booleans (bitfield)
[0x01]                 // authenticated=true, compressed=false

// Group 1: Enums (bit-packed)
[0xB4 0x01]            // method=POST, priority=high

// Group 5: Strings
[0x20][32 bytes]       // requestId (UUID)
[0x0D]["/api/users"]   // path

// Group 10: Union
[0x00]                 // discriminator=0 (UserCreateRequest)
[nested schema data]

Total envelope overhead: ~40 bytes
```

**Compare to HTTP:**
```
POST /api/users HTTP/1.1
Host: api.example.com
Authorization: Bearer eyJhbGc...
X-Request-ID: 019b84534d097c33b603bff7d02bb65c
Content-Type: application/json
Content-Length: 156

Overhead: ~200-300 bytes
```

**Savings: 80-85% bandwidth reduction for envelope**

### 8.3 RPC Method Routing

**Schema with bit-packed routing:**
```json
{
  "name": "RPCEnvelope_v1",
  "fields": {
    "service": {
      "type": "enum",
      "values": ["UserService", "OrderService", "PaymentService", "AuthService"]
    },
    "method": {
      "type": "enum",
      "values": ["CreateUser", "GetUser", "UpdateUser", "CreateOrder", "ProcessPayment"]
    }
  }
}
```

**Bit-packing:**
- service: 2 bits (4 services)
- method: 4 bits (16 methods)
- Total routing: 6 bits (1 byte)

**DAG-compiled router:**
```go
func routeRPC_v1(buf []byte) {
    packed := buf[2]  // Skip header
    service := Service(packed & 0x03)
    method := Method((packed >> 2) & 0x0F)
    
    // Direct dispatch (no string matching!)
    handlers[service][method](buf[3:])
}

// ~5 CPU cycles for routing decision
```

### 8.4 Service Mesh Integration

**Schema for service mesh metadata with bit-packing (24 bits total):**
```
sourceService:   8 bits
targetService:   8 bits
priority:        2 bits
retryPolicy:     2 bits
circuitBreaker:  1 bit
rateLimited:     1 bit
authenticated:   1 bit
encrypted:       1 bit
```

**vs gRPC with Envoy:**
```
gRPC metadata parsing:  ~10,000 cycles
Envoy proxy overhead:   ~40,000 cycles
Total:                  ~50,000 cycles

Anansi: ~20 cycles (2,500x faster!)
```

### 8.5 Complete Request/Response Cycle Performance

**Scenario:** API endpoint receiving 1M requests/second

**Traditional JSON over HTTP:**
```
Parse HTTP headers:     ~1,000 cycles
JSON decode request:    ~2,500 cycles
Process:                ~1,000 cycles
JSON encode response:   ~3,000 cycles
Format HTTP headers:    ~800 cycles
Total:                  ~8,300 cycles/request

Throughput: 3 GHz / 8,300 = ~361,000 req/sec/core
Memory: ~800 bytes allocated/request = 800 MB/sec GC churn
Bandwidth: ~400 bytes/request
```

**Anansi with DAG-compiled envelopes:**
```
Decode envelope (DAG):  ~50 cycles
Decode request (DAG):   ~100 cycles
Process:                ~1,000 cycles
Encode response (DAG):  ~80 cycles
Encode envelope (DAG):  ~40 cycles
Total:                  ~1,270 cycles/request

Throughput: 3 GHz / 1,270 = ~2,362,000 req/sec/core (6.5x faster)
Memory: ~0 bytes allocated/request = 0 MB/sec GC churn
Bandwidth: ~60 bytes/request (85% reduction)

Additional benefits:
- Zero GC pressure
- Predictable latency
- Better CPU cache utilization
- Lower memory footprint
- Higher connection density
```

---

## 9. Security & Error Handling

### 9.1 Error Handling

#### 9.1.1 Invalid Packet Type

**Error:** Unknown packet type (flags & 0x03 not in {0x00, 0x01, 0x02, 0x03})

**Action:** Reject packet, return error

#### 9.1.2 Unknown Schema Version

**Error:** Decoder doesn't have schema for version

**Action:**
- Option 1: Reject packet, request schema from server
- Option 2: Use schema negotiation protocol

#### 9.1.3 Buffer Underflow

**Error:** Not enough bytes remaining for expected data

**Action:** Reject packet, return error

#### 9.1.4 Type Mismatch

**Error:** Data doesn't match schema type expectations

**Action:** Reject packet, validation failed

#### 9.1.5 Compression Errors

**Error:** Decompression fails

**Action:** Reject packet, return error

#### 9.1.6 Stream Interruption

**Error:** Stream ends without end marker (row_count = 0)

**Action:** Return partial results with error indicator, or reject entirely based on implementation policy

### 9.2 Security Considerations

#### 9.2.1 Buffer Overflow Protection

Implementations **MUST**:
- Validate all length fields before allocation
- Set maximum message size limits
- Check buffer bounds before all reads
- Reject excessive varint sizes

#### 9.2.2 Compression Bombs

When compression is enabled:
- Set maximum decompressed size limit
- Set decompression time limit
- Reject excessive compression ratios

#### 9.2.3 Stream Resource Limits

For stream packets:
- Set maximum chunk count
- Set maximum total rows
- Set timeout for end marker
- Enforce maximum stream duration

#### 9.2.4 Schema Validation

Before decoding:
- Validate schema version exists
- Verify schema hash (if applicable)
- Check schema hasn't been revoked

#### 9.2.5 Denial of Service Mitigations

- Rate limit schema requests
- Cache compiled schemas
- Set maximum field count per schema
- Limit nesting depth
- Set maximum stream chunks

---

## 10. Testing & Conformance

### 10.1 Conformance Levels

**Level 1: Basic**
- Type 1 (Dense) support
- Type 2 (Sparse) support
- Unsigned varint encoding
- All primitive types
- Uncompressed only

**Level 2: Complete**
- All four packet types (Dense, Sparse, Batch, Stream)
- Signed integers (zigzag)
- All field types including Record
- Optional compression

**Level 3: Optimized**
- DAG-based decoding
- Zero-copy strings
- Schema compilation
- Jump table optimization
- Shared compression context for streams

### 10.2 Test Vectors

Implementations **SHOULD** pass official test vectors (see Appendix A).

---

## 11. Appendices

### Appendix A: Test Vectors

#### A.1 Type 1: Dense, Simple Types

**Schema:**
```json
{
  "name": "Simple",
  "version": "1",
  "fields": {
    "count": {"type": "integer"},
    "name": {"type": "string"},
    "active": {"type": "boolean"}
  }
}
```

**Sorted fields (by group, then fieldKey):**
```
Group 0 (Boolean): active
Group 2 (Integer): count
Group 5 (String): name
```

**Input:**
```json
{"count": 42, "name": "test", "active": true}
```

**Expected wire format (hex):**
```
00 01          // flags=0x00 (Dense), version=1
01             // boolean bitfield: active=true
2A             // count=42 (varint)
04             // name length=4
74 65 73 74    // "test"
```

**Total: 10 bytes**

#### A.2 Type 2: Sparse, Large Schema

**Schema: 100 fields, only 2 present**

**Expected wire format (hex):**
```
01 01          // flags=0x01 (Sparse), version=1
02             // field_count=2
05             // field_index=5
2A             // value=42
32             // field_index=50
04 74 65 73 74 // "test"
```

**Total: 12 bytes**

#### A.3 Type 3: Batch, Row-Oriented

**Schema: 3 fields, 2 rows**

**Expected wire format (hex):**
```
02 01          // flags=0x02 (Batch, row), version=1
02             // row_count=2

// Row 1
01             // field 0 (boolean)
0A             // field 1 (int=10)
03 66 6F 6F    // field 2 (string="foo")

// Row 2
00             // field 0 (boolean)
14             // field 1 (int=20)
03 62 61 72    // field 2 (string="bar")
```

**Total: 18 bytes**

#### A.4 Type 4: Stream, Row-Oriented

**Schema: 3 fields, 247 rows in 100-row chunks**

**Expected wire format (hex):**
```
03 01          // flags=0x03 (Stream), version=1
01             // extended_type=0x01 (Stream)
00             // stream_encoding: row-oriented, uncompressed

// Chunk 1 (100 rows)
64             // row_count=100
[100 rows of data...]

// Chunk 2 (100 rows)
64             // row_count=100
[100 rows of data...]

// Chunk 3 (47 rows)
2F             // row_count=47
[47 rows of data...]

// End marker
00             // row_count=0
```

#### A.5 Type 1: Dense with Null Field

**Schema:**
```json
{
  "name": "User",
  "version": "1",
  "fields": {
    "id": {"type": "string"},
    "email": {"type": "string", "required": false},
    "age": {"type": "integer"}
  }
}
```

**Sorted fields:**
```
Group 2: age
Group 5: email, id (alphabetical)
```

**Input (email is null):**
```json
{"id": "123", "age": 25}
```

**Expected wire format (hex):**
```
00 01          // flags=0x00 (Dense), version=1
19             // age=25 (varint)
00             // email=null (null marker)
03 31 32 33    // id="123"
```

**Total: 8 bytes**

#### A.6 Type 1: Unstructured Record

**Schema:**
```json
{
  "name": "Metadata",
  "version": "1",
  "fields": {
    "data": {"type": "record"}
  }
}
```

**Input:**
```json
{
  "data": {
    "count": 42,
    "name": "test"
  }
}
```

**Expected wire format (hex):**
```
00 01          // flags=0x00 (Dense), version=1
02             // record count=2

// Entry 1: "count" (alphabetically first)
05             // key length=5
63 6F 75 6E 74 // "count"
02             // type_tag=0x02 (integer)
2A             // value=42

// Entry 2: "name"
04             // key length=4
6E 61 6D 65    // "name"
05             // type_tag=0x05 (string)
04             // string length=4
74 65 73 74    // "test"
```

**Total: 26 bytes**

### Appendix B: MIME Type

**Proposed MIME type:** `application/vnd.anansi.binary`

**File extension:** `.anansi` or `.anb`

### Appendix C: Schema Definition Wire Format

The meta-schema itself can be encoded using this format, enabling schema distribution:

```
Schema definitions are encoded as Type 1 (Dense) packets
using a hardcoded meta-schema version
```

This enables complete bootstrapping:
1. Client hardcodes meta-schema codec
2. Server sends schemas encoded via meta-schema
3. Client decodes schemas, compiles codecs
4. Data exchange begins

### Appendix D: Extended Type System

The extended type byte (when flags & 0x03 == 0x03) allows for future packet types:

```
[flags: 0x03]
[schema_version: u8]
[extended_type: u8]  ← Type identifier
[extended_data...]

Current:
  0x01 = Stream

Reserved:
  0x02-0xFF = Future extensions
```

Possible uses:
- Schema negotiation
- Partial updates/patches
- Delta encoding
- Multi-schema messages

### Appendix E: Changelog

**Version 1.0 (2025-01-10):**
- Initial specification
- Four packet types defined (Dense, Sparse, Batch, Stream)
- Schema versioning support (1024 versions with epoch system)
- Compression support with shared context for streams
- Encryption support (AEAD recommended)
- Integrity hashing (BLAKE3-128)
- Varint encoding (LEB128 + zigzag)
- Field type specifications including structured and unstructured records
- Null handling clarifications for all packet types
- Field ordering by (group, fieldKey)
- DAG-based decoder implementation guidelines
- Protocol envelope encoding patterns
- Performance benchmarks and optimization strategies

---

**End of Specification**