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

The Anansi Binary Wire Format is a high-performance serialization format specifically optimized for `Document` instances. It serializes the flattened, primitive-typed data structure defined by the Document Specification, supporting four packet types automatically selected based on data characteristics, with full schema versioning and optional compression. This approach delegates the complexities of logical schema definitions (including recursion and validation) to the layer that constructs the `Document` object.

### 1.2 Design Principles
- **Storage Inheritance:** This format is a physical manifestation of the Document Storage Spec. It inherits all hard limits and types.
- **Self-Delineation:** Data boundaries are determined by the Schema + State Map, eliminating the need for per-row delimiter bytes.

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
   │     │     │     │     └────────────────────── Encoding (bit 3)
   │     │     │     │                        
   │     │     └─────┴──────────────────────────── Version Epoch (bits 4-5)
   │     │
   │     └──────────────────────────────────────── Encryption (bit 6)
   │
   └────────────────────────────────────────────── Hash Present (bit 7)
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
- Each full version maps to a semantic version in an ordered registry.
- Registry maintains version history: `{0: "1.0.0", 1: "1.1.0", 256: "2.0.0", ...}`
- Decoders lookup schema by full version

### 2.3 Field Selectors and Ordering

Fields are encoded in **stable sorted order by their `FieldSelector` value (ascending int32)**. The `FieldSelector` inherently contains type information (via its `Type` bits), which implicitly groups fields by their primitive storage type (`TypeInt`, `TypeFloat`, `TypeString`, `TypeBool`, `TypeBytes`).

This direct reliance on `FieldSelector` ensures consistency with the `Document` structure and removes any ambiguity regarding field order, including for nested or recursive logical schemas (as these are pre-resolved and flattened into unique `FieldSelector`s during `Document` construction).

**Field Selector Bit Layout**:
┌──────────────┬──────────┬───────────┬───────────┬───────────┐
│ Reserved(2b) │ Type(3b) │ Depth(9b) │ Offset(9b)│ Index(9b) │
└──────────────┴──────────┴───────────┴───────────┴───────────┘

**Example:**
Assume a `Document` contains fields corresponding to the following `FieldSelector`s, derived from a logical schema. The encoding order is simply by the ascending `int32` value of these selectors.

`FieldSelector` values (example):
- `0x00000001` (TypeBool, Depth 0, Offset 0, Index 1)
- `0x0000000A` (TypeInt, Depth 0, Offset 0, Index 2)
- `0x00000013` (TypeString, Depth 0, Offset 0, Index 3)
- `0x00000020` (TypeBytes, Depth 1, Offset 1, Index 1)

**Encoding order:**
1. `FieldSelector` 0x00000001 (Boolean field)
2. `FieldSelector` 0x0000000A (Integer field)
3. `FieldSelector` 0x00000013 (String field)
4. `FieldSelector` 0x00000020 (Bytes field, representing a nested object or array)

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
Dense Mode: Packed into bitfields (8 bools per byte).
Sparse Mode: Single byte (0x00 or 0x01).
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

#### Bytes
```
[length: varint][bytes: Raw]
Can contain raw binary, encoded arrays, or serializations of logical types not supported natively.
```

### 2.6 Storage Model Inheritance

The Anansi Wire Format is a physical manifestation of the `Document Storage Specification`. It implicitly inherits all hard limits of that model:
- **Max Depth:** 511 levels. Nesting in recursive or sparse packets cannot exceed this.
- **Max Fields:** 511 fields per level.
- **Selector Stability:** FieldSelectors are determined by the schema and must remain stable for the life of the `schema_version`.

### 2.7 Null Handling

The spec allows three states for a value. The wire format handles them as follows

|State|Document Logic|Dense Encoding|Sparse Encoding|
|-----|--------------|--------------|---------------|
|Has Value|positions[sel] >= 0|"Bitmask=1| Write Value"|Write [Selector] [Value]|
|Null|positions[sel] < 0|"Bitmask=1| Write Null Marker"|Write [Selector] [NullMarker]|
|Not Set|key not in positions|"Bitmask=0| Skip"|Skip|

## 3. Packet Type Specifications

### 3.1 Type 1: Dense Packet

#### 3.1.1. The Finite Eligibility Rule
A Schema is only eligible for Dense (Type 0x00) encoding if it is Non-Recursive.
- Eligible: A struct containing a fixed list of primitives and nested structs that eventually terminate in primitives.
- Ineligible: A Node struct containing a Node field, or a Map field with arbitrary keys.

#### 3.1.2. Streamlining the State Map

Since we are now guaranteed a finite, pre-known field count *N* for any Dense packet, we can provide a state map as a fixed-length header bitstream that leverages the stable sorting order of FieldSelectors.

We use 2 bits per field to represent any of the three states allowed for a value:
- `00` (Not Set): The field is absent. Skip.
- `01` (Null): The field is explicitly null. Skip
- `10` (Value): The field has data. Read from the value segment.
- `11` (Reserved):.

#### 3.1.3. Structural Layout

The values are appended in the strict order of their FieldSelector (Type → Depth → Offset → Index). Because the schema is finite, the decoder knows exactly how many "Value" states (10) it needs to read for each primitive type block.
┌───────────────────────────────┐
│ Header (2 Bytes)              │ Flags, Schema Version
├───────────────────────────────┤
│ State Map (Bitstream)         │ 2-bits per schema field
├───────────────────────────────┤
│ Int Value Block               │ All fields where state == 10 and Type == Int
├───────────────────────────────┤
│ Float Value Block             │ All fields where state == 10 and Type == Float
├───────────────────────────────┤
│ ... (String, Bool, Bytes)     │
└───────────────────────────────┘

#### 3.1.4. Handling Recursion
If the data structure is recursive (like a linked list or tree), the encoder must switch to the sparse packet type:

### 3.2 Type 2: Sparse Packet
Used for recursive structures, patches, or when field density is low. This format eliminates all bitmaps by encoding the state within the field identifier.
In Sparse packets, the `FieldSelector` is mutated to encode the "Null" state efficiently. 
- **Wire Value:** `(StorageSelector << 1) | NullBit`
- **NullBit:** `0` indicates the field is set; `1` indicates the field is explicitly null.
- **Decoding:** Parsers MUST right-shift the wire value by 1 bit to recover the actual `FieldSelector` used by the Document storage.

#### 3.2.1 Wire Format

```
┌──────────────────────────────────────────────┐
│ [flags: u8]                                  │  <- Packet type = 0b01
│ [schema_version: u8]                         │
├──────────────────────────────────────────────┤
│ [field_count: varint]                        │  <- Number of present fields
├──────────────────────────────────────────────┤
│ FOR EACH PRESENT FIELD:                      │
│   [field_selector: varint]                   │  <- FieldSelector 
│   [field_data]                               │  <- Encoded per type
└──────────────────────────────────────────────┘
```

### 3.3 Type 3: Batch Packet
The Batch packet transmits a fixed number of records. It inherits the Schema Version from the header. The layout transitions from Field-oriented (Columnar) to Record-oriented based on the BatchFlags.

**Header Structure**:

- `record_count`: varint
- `batch_flags`: u8
    - Bit 0: Orientation (0 = Row-Oriented, 1 = Columnar)
    - Bit 1: Density (0 = Dense, 1 = Sparse)

#### 3.3.1 Row-Oriented Dense Batch (The "Transaction" Layout) 
Optimized for materializing full Documents. Each record is self-contained.

```
FOR EACH RECORD (0 to record_count-1):
  [State Map] (2 bits per field in Schema)
  [Values Block] (Values only for fields with state '10')
```

*Delineation*: The parser uses the Schema to know the bit-length of the State Map. Once the State Map is read, it knows exactly which values follow and their lengths. The next bit/byte is immediately the start of the next record's State Map.

#### 3.3.2 Row-Oriented Sparse Batch (The "Outlier" Layout)
Used when records have very few fields set relative to a wide schema or are not dense suitable.

```
FOR EACH RECORD (0 to record_count-1):
  [field_count] (varint)
  FOR EACH FIELD (0 to field_count-1):
    [WireSelector] (varint: (StorageSelector << 1) | NullBit)
    [Value] (If NullBit == 0)
```

#### 3.3.3 Columnar Dense Batch (The "Analytics" Layout) 
Optimized for scanning specific fields across many records.

```
FOR EACH FIELD IN SCHEMA:
  [Field Data Block]
    IF Fixed-Width (Int/Float/Bool): 
       [Raw bytes for all N records]
    IF Variable-Width (String/Bytes):
       [Length-prefixed values for all N records]
```

### 3.4 Type 4: Stream Packet

#### 3.4.1 When to Use
Streams are sequences of "Chunks." This allows a long-lived connection to pivot between Dense and Sparse batches as the data distribution changes.

#### 3.4.2 Wire Format

```
┌──────────────────────────────────────────────┐
│ STREAM HEADER (sent once)                    │
├──────────────────────────────────────────────┤
│ [flags: u8]              (0x03)              │  <- Extended packet type
│ [schema_version: u8]                         │  <- Schema version (0-255)
│ [extended_type: u8]      (0x01)              │  <- Stream marker
│ [stream_encoding: u8]                        │  <- Encoding flags
├──────────────────────────────────────────────┤
│ CHUNKS (repeating)                           │
├──────────────────────────────────────────────┤
│ [row_count: varint]                          │  <- Rows in this chunk
│ [chunk_data]                                 │  <- Encoded per stream_encoding
├──────────────────────────────────────────────┤
│ END MARKER                                   │
├──────────────────────────────────────────────┤
│ [row_count: 0]                               │  <- Stream terminator
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
- `00` (0x00): Reserved
- `01` (0x01): Columnar (like Type 3 batch with bit 3 set)
- `10` (0x02): Reserved
- `11` (0x03): Reserved

**Bit 2: Compression Flag**
- `0`: Uncompressed
- `1`: Compressed with shared context

**Bits 3-7: Reserved**
- Must be set to `00000`


**Adaptive Chunking**

Every chunk repeats the Batch logic but adds a 1-byte Chunk Descriptor.
```
┌────────────────────────────────────────────────────────┐
│ [Chunk Descriptor: u8] (Density & Orientation bits)    │
│ [Chunk Row Count: varint]                              │
│ [Payload: Logic from Section 3.3.1 - 3.3.3]            │
└────────────────────────────────────────────────────────┘
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

### 3.5 Delineation and Navigation
Anansi is a "calculative" format. To find the end of a record:
1. **Dense:** Read the $N$-bit State Map (where $N = 2 \times \text{fields in schema}$). Sum the sizes of all fields marked '10'.
2. **Sparse:** Read the `field_count`. For each field, read the `WireSelector` (varint). If the LSB is 0, read the value based on the type associated with that selector in the schema.
3. **Variable Lengths:** All `TypeString` and `TypeBytes` are length-prefixed with a varint.

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
-> Lookup in registry: fullVersion = 256 (first v2.x.x)
-> epoch = fullVersion >> 8 = 1
-> schemaVersion = fullVersion & 0xFF = 0
-> Set flags bits 4-5 = 01
-> Set byte 1 = 0x00
```

**Decoder:**
```
flags = 0x10 (bits 4-5 = 01)
schemaVersion = 0x00

-> epoch = (flags >> 4) & 0x03 = 1
-> fullVersion = (epoch << 8) | schemaVersion = 256
-> Lookup in registry: "2.0.0"
-> Load compiled schema for version 256
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

Schemas already exist in every layer of modern applications - they're just fragmented, duplicated, and not reusable.

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

**One schema definition can generate:**
```
1. Wire format encoder/decoder
2. Database migration
3. ORM models
4. TypeScript types
5. Validation functions
6. OpenAPI documentation
7. Frontend form schemas
8. Mock data generators
9. Test fixtures
10. API client code
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
    
    if (fieldCount <= 64 || density > 0.25) && schema.IsFinite() {
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

Compiling a schema into a DAG driven decoder optimizes decoding by eliminating runtime branching.

#### 6.3.2 Zero-Copy String Implementation

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

#### 6.3.4 Decoder Cache

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


## 7. Protocol Extensions

### 7.1 Protocol Envelope Encoding

The wire format can encode entire protocol envelopes, not just application data. By defining schemas for request/response metadata, error handling, routing, and other protocol concerns, implementations can achieve protocol-level performance optimizations.

### 7.2 API Request Envelope Example

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
05             // field_selector=5
2A             // value=42
32             // field_selector=50
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
