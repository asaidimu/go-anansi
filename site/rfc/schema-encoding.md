---
title: Schema binary encoding
description: "Proposes a binary encoding for the compiled schema IR."
rfcStatus: draft
rfcId: schema-encoding
---
**Version:** 1.2  
**Status:** Final  
**Scope:** Compiled Schema Serialization, Registry Storage, and Runtime Dispatch

**Changes since 1.1:** Section 8 (Metadata Records) now has a concrete,
byte-exact layout (previously described only conceptually). The Global
String Table (Section 3) is no longer optional — inline string storage has
been removed in favour of always-interned strings, which both compacts
repeated names/descriptions across a schema and keeps the fixed-width
metadata records self-contained. `FieldMeta` and `SchemaMeta` no longer
persist `Path`, `Parts`, or `Default` — see §6.6 for why.

---

## 1. Overview

The Anansi Schema Binary Encoding defines a compact, read‑optimised, and cache‑coherent binary representation of a `CompiledSchema`. The format is designed for **write‑once / append‑only** storage and **on‑demand random access** – the decoder pays only for the data it actually uses.

The entire schema payload is prefixed by a **fixed‑size 144‑byte header** (16 bytes of semantic/structural metadata + 128 bytes reserved for a navigation directory). This header is immediately followed by a sequence of variable‑length data sections. The absolute offset of every section is stored in the navigation directory, enabling constant‑time, zero‑parsing jumps to any required data.

---

## 2. Conventions and Notation

- **Byte Order:** Multi‑byte integers in **Word 0** and **Word 1** are stored in **big‑endian** order.  
  The **Fixed Navigation Directory** (offsets 16‑143) is stored in **little‑endian** order.  
  The endianness of all subsequent payload sections is determined by the `Endianness` flag in Word 1.
- **Reserved Fields:** Must be set to zero. Decoders **must** ignore reserved bits.
- **Alignment:** All sections are naturally aligned to their largest member (i.e., `uint64` sections are aligned to 8‑byte boundaries). The encoder must insert padding bytes (set to zero) between sections to maintain alignment.

---

## 3. Word 0 – Stable Handle (64 bits, Offset 0)

This word is the immutable, global identifier for the schema. It is used as a map key, a cache index, and a locator inside a continuous registry byte slice. It is never modified after the schema is published.

### 3.1. Bit Layout

```
 63  40 39       16 15        0
+------+----------+-----------+
| ID   |  Length  |  Offset   |
|(24b) |  (24b)   |  (16b)    |
+------+----------+-----------+
```

### 3.2. Field Definitions

| Field | Bits | Width | Description |
| :--- | :--- | :--- | :--- |
| **Offset** | 0 – 15 | 16 | Byte offset from the start of the registry’s contiguous byte slice to the **first byte of this schema’s payload** (i.e., the start of Word 0 itself). |
| **Length** | 16 – 39 | **24** | Total size of the schema payload in bytes, **including** all headers and trailing sections. Max value: 2²⁴ − 1 = 16,777,215 bytes (≈16 MiB). |
| **ID Block** | 40 – 63 | **24** | Immutable schema identity subdivided as follows: |

### 3.3. ID Block Sub‑Division (24 bits)

```
63  61 60  59 58  57 56       48 47       40
+------+------+---+----------+----------+
| Resv | Hdr  |Comp| Epoch    | Version  | Schema
| (3b) | Ext  |(1b)| (2b)     |  (8b)    |   ID
|      | (2b) |    |          |          |  (8b)
+------+------+---+----------+----------+
```

| Sub‑field | Bits (within ID block) | Bits (handle) | Width | Semantics |
| :--- | :--- | :--- | :--- | :--- |
| **Schema Id** | 0 – 7 | 40 – 47 | 8 | Base schema identifier (0–255). A family of related schemas shares the same Schema ID. |
| **Version** | 8 – 15 | 48 – 55 | 8 | Version number (0–255). |
| **Epoch** | 16 – 17 | 56 – 57 | 2 | Version epoch (0–3). Combined with `Version`, yields a space of 1024 distinct versions. |
| **Compression** | 18 | 58 | 1 | `0` = Payload (sections following the header) is uncompressed.<br>`1` = Payload is compressed (algorithm is indicated out‑of‑band or in a future extension). |
| **Header Extension** | 19 – 20 | 59 – 60 | 2 | Defines the total header size. <br>`0`: Word 0 only (not used). <br>`1`: Words 0 + 1 (default, 16 bytes). <br>`2`: Words 0 + 1 + 2 (24 bytes). <br>`3`: Words 0 + 1 + 2 + 3 (32 bytes). |
| **Reserved** | 21 – 23 | 61 – 63 | 3 | Must be zero. Reserved for future use. |

---

## 4. Word 1 – Dispatch Header (64 bits, Offset 8)

The decoder reads this word immediately after validating the handle. It contains **only** the structural flags required to begin parsing the byte stream.

### 4.1. Bit Layout

```
 63  56 55       32 31       8 7      0
+------+-----------+---------+--------+
| Resv | Checksum  | Resv    | Flags  |
| (8b) |  (24b)    | (24b)   | (8b)   |
+------+-----------+---------+--------+
```

### 4.2. Field Definitions

| Field | Bits | Width | Description |
| :--- | :--- | :--- | :--- |
| **Flags** | 0 – 7 | 8 | Structural decode directives (see §4.3). |
| **Reserved** | 8 – 31 | 24 | Must be zero. |
| **Checksum** | 32 – 55 | 24 | Optional integrity checksum (e.g., CRC‑24). If zero, checksum validation is disabled. If non‑zero, the decoder must verify the payload checksum before parsing. |
| **Reserved** | 56 – 63 | 8 | Must be zero. |

### 4.3. Flags Field – The 6 Essential Bits

Bits 0–5 contain the critical decoding directives. Bits 6–7 are reserved for future expansion.

| Bit | Flag | Semantics |
| :---: | :--- | :--- |
| 0 | **Endianness** | `0` = All multi‑byte integers in the **payload sections** (except the Navigation Directory) are little‑endian. <br>`1` = Big‑endian. |
| 1 | **Reserved (String Storage)** | As of v1.2, strings are always interned in the Global String Table (Section 3) and referenced by fixed‑width offsets from Metadata Records (Section 8) — inline string storage has been removed. This bit **must** be set to `1`. A decoder encountering `0` here is reading a pre‑1.2 payload written with inline strings and must not assume Section 8's v1.2 record layout applies. |
| 2 | **Offset Width** | `0` = Variable‑length internal pointers (e.g., within the String Table or Metadata Records) are **32‑bit**. <br>`1` = They are **64‑bit**. |
| 3 | **Has Cold Trailer** | `0` = The Cold Trailer section (Section 9) is entirely absent. <br>`1` = It is present (contains Defaults, Enums, Variants, Constraints, and Indexes). |
| 4 | **Reserved** | Must be `0`. |
| 5 | **Reserved** | Must be `0`. |

> **Critical Rule:** The **Fixed Navigation Directory** (offset 16) is **always** stored in little‑endian and uses fixed 64‑bit offsets, regardless of the `Endianness` or `Offset Width` flags. This ensures the directory is decodeable before the flags are applied.

---

## 5. Fixed Navigation Directory (128 bytes, Offset 16)

This directory contains absolute byte offsets (from the start of the payload) to the beginning of each major data section. Because it is exactly 128 bytes, it occupies exactly two cache lines.

> **Important:** All offsets in this directory are **64‑bit, little‑endian** integers.

| Byte Offset (payload‑relative) | Field | Width | Description |
| :--- | :--- | :--- | :--- |
| 16 – 23 | `StringTableOffset` | 8 | Absolute offset to Section 3 (Global String Table). |
| 24 – 31 | `SchemasOffset` | 8 | Absolute offset to Section 4 ([]SchemaSlot). |
| 32 – 39 | `DescriptorsOffset` | 8 | Absolute offset to Section 5 ([]FieldDescriptor). |
| 40 – 47 | `LocalOffsetsOffset` | 8 | Absolute offset to Section 6 ([]uint32 – LocalOffsets). |
| 48 – 55 | `FieldTypesOffset` | 8 | Absolute offset to Section 7 ([]FieldType). |
| 56 – 63 | `MetadataOffset` | 8 | Absolute offset to Section 8 (Metadata Records – FieldMeta / SchemaMeta). |
| 64 – 71 | `ColdTrailerOffset` | 8 | Absolute offset to Section 9 (Cold Trailer – Defaults, Enums, Variants, Constraints, Indexes). |
| 72 – 143 | **Reserved** | 72 | Must be zero. Reserved for future pointer additions (e.g., checksum footer, extension blobs). |

---

## 6. Payload Sections (Variable Offsets)

The sections may be placed in any order, provided they do not overlap and are correctly aligned. The decoder accesses them solely via the offsets stored in the Fixed Navigation Directory.

### 6.1. Section 3 – Global String Table (Always Present, as of v1.2)
- **Presence:** Always present. Every `Name`/`Description` reference in Section 8 points here; there is no inline-string fallback.
- **Format:** A contiguous byte array of length‑prefixed UTF‑8 strings. Each entry: `[ Length: uint16/uint32 (determined by Offset Width flag) ] [ Bytes ... ]`. The first string at offset 0 must have length 0 (reserved sentinel) — this is the value every `DescriptionOffset` of `0` points to, representing "no description".
- **Deduplication:** Encoders should intern identical strings once and reuse the offset — schemas commonly repeat short names/descriptions (e.g. `"id"`, `"name"`) across many fields, and deduplication is where the mandatory string table earns its keep over inline storage.

### 6.2. Section 4 – `[]SchemaSlot` (Fixed Size)
- **Record Size:** 8 bytes per slot (`uint16 FieldStart` + `uint16 FieldCount` + `uint32 Footprint`).
- **Access:** Directly castable to a slice of `SchemaSlot`. Used exclusively by `Address()`.

### 6.3. Section 5 – `[]FieldDescriptor` (Fixed Size)
- **Record Size:** 4 bytes per descriptor (packed flags).
- **Access:** Directly castable to a slice of `FieldDescriptor`. Used by `Address()` and `ResolveFieldStep()`.

### 6.4. Section 6 – `[]uint32 LocalOffsets` (Fixed Size)
- **Record Size:** 4 bytes per entry.
- **Access:** Parallel to Section 5. Used by `Address()` to compute the prefix‑sum address.

### 6.5. Section 7 – `[]FieldType` (Fixed Size)
- **Record Size:** 1 byte per entry.
- **Access:** Parallel to Section 5. Used for validation and default handling.

### 6.6. Section 8 – Metadata Records (Fixed Size, 24 Bytes per Record)

**Presence and layout:** Two fixed-width arrays, back to back, with no
separator or stored count — each array's element count is derived the same
way Sections 4/5's own counts are (the byte span between consecutive
Navigation Directory offsets, divided by record size):

1. `FieldMeta[]` — one record per entry in Section 5 (`[]FieldDescriptor`), same count, same absolute index. Starts at `MetadataOffset`.
2. `SchemaMeta[]` — one record per entry in Section 4 (`[]SchemaSlot`), same count, same slot index. Starts immediately after the last `FieldMeta` record.

Both record kinds share an identical 24-byte shape:

| Bytes | Field | Notes |
| :--- | :--- | :--- |
| 0 – 15 | `ID` | Raw 16-byte UUID (UUIDv7 for fields; `SchemaId` for schema slots). Stored as canonical UUID byte order, **not** affected by the `Endianness` flag — same treatment as string bytes in Section 3. All-zero for a `SchemaMeta` record with no source UUID: the root slot (identified by the top-level schema's own name/version, not a nested-schema id) and synthetic slots created for a field's inline composite (which has no `schema.json` entity of its own — see the field's `Variants`/child-schema linkage in the Cold Trailer to recover its constituent parts instead). |
| 16 – 23 | `Offsets` | Packed `uint64`, byte order per the `Endianness` flag (§4.3, bit 0). Layout below. |

**`Offsets` word (64 bits):**

```
 63          48 47          24 23           0
+--------------+--------------+--------------+
|   Reserved   | DescOffset   | NameOffset   |
|    (16b)     |    (24b)     |    (24b)     |
+--------------+--------------+--------------+
```

| Sub‑field | Bits | Width | Semantics |
| :--- | :--- | :--- | :--- |
| **NameOffset** | 0 – 23 | 24 | Offset into the Section 3 string table for this field/schema's name. 24 bits is sufficient because the string table can never exceed the schema's own 24-bit `Length` field (Word 0, §3.2) — the whole payload tops out at 16 MiB. |
| **DescOffset** | 24 – 47 | 24 | Offset into the string table for the description. `0` points at the reserved empty-string sentinel (§6.1) and means "no description" — this is the common case, since most fields don't carry one. |
| **Reserved** | 48 – 63 | 16 | Must be zero. Reserved for future per-record flags (e.g. a dedup/interning hint) without growing the record past 24 bytes. |

**Deliberately not stored:**
- **`Path` / `Parts`** — fully recoverable at read time by walking a `ResolvedPath` and looking up each step's `Name` (exactly what `joinPath()` does today). Persisting a precomputed dotted path for every field would just be storing a derivable string redundantly.
- **`Default`** — the runtime-consumed default value lives in the Cold Trailer's `Defaults` container (§6.7), keyed by `DataPoint`. The in-memory `FieldMeta.Default` is a construction-time-only convenience consumed once by `Link()`; there is no second reader, so there is no reason to persist it twice.

**Access:** Used by `ResolvePath()`, `PathString()`/`joinPath()`, and JSON reconstruction. Not touched by `Address()` or validation — this section is cold-path by design, per the operational priority in §8 (Design Rationale): validation and query planning run far more often than schema serialization, so names/descriptions stay out of the hot Sections 4–7 entirely.

### 6.7. Section 9 – Cold Trailer (Optional)
- **Presence:** Determined by the `HasColdTrailer` flag.
- **Content:** A concatenation of binary‑serialized `DataContainer` blobs for `Defaults` and `Enums`, followed by a serialized map for `Variants`, followed by `Constraints`, `Indexes`, `SchemaConstraints`, and `FieldRefConstraints`.
- **Access:** Only resolved when the caller explicitly invokes validation or default retrieval logic.

---

## 7. Decoder Flow – On‑Demand Execution

```
1. Read Word 0 (big‑endian) to locate the payload and validate the handle.
2. Read Word 1 (big‑endian) to extract Flags and Checksum.
3. (Optional) Verify Checksum if non‑zero.
4. If the caller invokes Address():
     a. Read `SchemasOffset` (payload[24:32]).
     b. Read `DescriptorsOffset` (payload[32:40]).
     c. Read `LocalOffsetsOffset` (payload[40:48]).
     d. Jump directly to these offsets. No other sections are touched.
5. If the caller invokes ResolvePath():
     a. Read `StringTableOffset` (payload[16:24]).
     b. Read `MetadataOffset` (payload[56:64]).
     c. Jump directly to these offsets. Skip Sections 4–7 and 9.
6. If the caller invokes GetDefaults() or GetConstraints():
     a. Read `ColdTrailerOffset` (payload[64:72]).
     b. Jump directly to it. Skip all other sections.
```

The decoder **never** parses, iterates, or allocates data structures for unused sections.

---

## 8. Design Rationale

| Principle | Implementation |
| :--- | :--- |
| **Write‑Once / Immutable** | The format is append‑only. Offsets are absolute and computed once at compile time. There are no free lists, relocation tables, or dynamic sizing. |
| **On‑Demand Reading** | No section directory iteration is required. The 128‑byte fixed directory provides absolute offsets for every major data block in constant time. |
| **Cache Locality** | The hot‑path arrays (Schemas, Descriptors, LocalOffsets) have their offsets stored contiguously in the directory (bytes 24–47). <br> `Address()` fetches these three pointers in a single cache line. |
| **Zero‑Copy Parsing** | Fixed‑size arrays (Sections 4–7) are directly memory‑mapped to Go slices using `unsafe.Pointer` or a safe byte‑slice cast, eliminating deserialization overhead. |
| **Stable Global Identity** | Word 0 encodes registry location, size, and version. The 24‑bit ID block provides ample room for identity sub‑fields (with 3 reserved bits) while the 16‑bit offset gives direct access to the payload. |
| **Extensibility** | The `Header Extension` and 72 bytes of reserved navigation space allow future additions (e.g., secondary string tables, index bloom filters) without breaking backward compatibility. The 16 reserved bits in every Section 8 `Offsets` word offer the same headroom for per-field/per-schema metadata flags without growing the record. |
| **Operational/Metadata Separation** | Sections 4–7 (`Schemas`, `Descriptors`, `LocalOffsets`, `FieldTypes`) hold everything validation and query planning touch on a per-document, per-field basis, and stay free of variable-length text. Names, descriptions, and schema/field UUIDs — needed only for path resolution, tooling, and JSON reconstruction, not for validating a document — live entirely in the cold Section 8/9 region. A decoder that only ever validates documents never touches the string table or the Cold Trailer. |

---

## 9. Implementation Notes for Encoders

1. **Calculate Offsets:** Build all sections in memory, compute their lengths, then write them sequentially into the output buffer. Record their absolute start offsets.
2. **Populate Navigation Directory:** Fill bytes 16–71 with the recorded offsets (little‑endian `uint64`).
3. **Write Header:** Write Word 0 (big‑endian) and Word 1 (big‑endian), ensuring the correct bit layout for Word 0 (Offset in bits 0‑15, Length in bits 16‑39, ID Block in bits 40‑63).
4. **Align Sections:** Ensure each section starts at an offset that is a multiple of its natural alignment (e.g., `uint64` sections at 8‑byte boundaries).
5. **Compute Length:** Set the `Length` field in Word 0 to the total number of bytes written, including the 144‑byte header.

---

## 10. ABNF Summary (Informative)

```
payload = header sections

header = word0 word1 navigation
word0   = 8OCTET                 ; big‑endian, stable handle (offset, length, id)
word1   = 8OCTET                 ; big‑endian, dispatch flags
navigation = 128OCTET            ; little‑endian, 64‑bit offsets

sections = string‑table         ; Section 3 (always present, v1.2+)
           schemas               ; Section 4
           descriptors           ; Section 5
           local‑offsets         ; Section 6
           field‑types           ; Section 7
           metadata‑records      ; Section 8 (fieldmeta[] ++ schemameta[])
           [ cold‑trailer ]      ; Section 9 (optional)
```

---

*This specification defines the canonical Anansi Schema binary encoding. All implementations MUST adhere to the bit‑level layouts and endianness rules outlined above to ensure interoperability across registries and runtimes.*