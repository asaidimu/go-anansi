// AnansiCodec — the friendly facade over the functional wire API.
//
// Binds one compiled schema (+ optional default transforms/version) into a
// reusable object. Construct once per schema (or per endpoint/collection if
// you prefer finer-grained caching) and hold it for the lifetime of your
// app; instances are immutable and safe to share concurrently.

import type { ManifestField, LinkResult } from "./schema/link.ts";
import { link, buildManifest } from "./schema/link.ts";
import { parseSchema, type SchemaDefinition } from "./schema/types.ts";
import {
  encodeDocument, decodeDocument,
  encodeBatchRows, decodeBatch,
} from "./wire/packet.ts";
import type { EncodeKind } from "./wire/packet.ts";
import {
  encodeAnansiPacket, decodeAnansiPacket,
  encodeAnansiBatchRows, encodeAnansiBatchColumnar, decodeAnansiBatch,
} from "./wire/transforms.ts";
import type { DecodeTransforms, EncodeTransforms } from "./wire/transforms.ts";

export interface AnansiCodecOptions extends EncodeTransforms {
  /** Schema version stamped into outgoing packets (0–1023). */
  fullVersion?: number;
  /** Force Dense/Sparse instead of the density heuristic. */
  kind?: EncodeKind;
  /** Key used to decrypt incoming packets (if the server encrypts). */
  decryptionKey?: Uint8Array;
}

export class AnansiCodec {
  private constructor(
    readonly linked: LinkResult,
    readonly fields: ManifestField[],
    readonly fullVersion: number,
    private readonly kind: EncodeKind,
    private readonly enc: EncodeTransforms,
    private readonly dec: DecodeTransforms,
  ) {}

  /**
   * Compile a schema definition and bind it to a codec instance.
   *
   * ```ts
   * const codec = await AnansiCodec.create(schemaJSON);            // plain
   * const codec = await AnansiCodec.create(schemaJSON, {
   *   fullVersion: 7,
   *   transforms: { compression: true, integrity: true },
   * });
   * ```
   *
   * Instances are immutable; cache one per schema version (or per endpoint)
   * and share it freely across requests.
   */
  static async create(
    schema: string | SchemaDefinition | Record<string, unknown>,
    opts: AnansiCodecOptions = {},
  ): Promise<AnansiCodec> {
    const parsed = parseSchema(schema);
    const linked = link(parsed);
    return new AnansiCodec(
      linked,
      buildManifest(linked),
      opts.fullVersion ?? 0,
      opts.kind ?? "auto",
      {
        compression: opts.compression,
        integrity: opts.integrity,
        encryptionKey: opts.encryptionKey,
      },
      { decryptionKey: opts.decryptionKey },
    );
  }

  /** Encode a single document (Dense/Sparse per configured strategy). */
  async encode(doc: Record<string, unknown>): Promise<Uint8Array> {
    const hasTransforms =
      this.enc.compression || this.enc.integrity || !!this.enc.encryptionKey;
    if (hasTransforms) {
      return encodeAnansiPacket(this.fields, doc, this.fullVersion, {
        ...this.enc,
        kind: this.kind,
      });
    }
    return encodeDocument(this.fields, doc, this.fullVersion, this.kind);
  }

  /** Decode a single-document packet. Accepts plain and transformed frames. */
  async decode(data: Uint8Array): Promise<{ version: number; doc: Record<string, unknown> }> {
    return decodeAnansiPacket(data, this.fields, this.dec);
  }

  /** Encode many documents as one row-oriented batch. */
  async encodeBatch(docs: Record<string, unknown>[]): Promise<Uint8Array> {
    const hasTransforms =
      this.enc.compression || this.enc.integrity || !!this.enc.encryptionKey;
    if (hasTransforms) {
      return encodeAnansiBatchRows(this.fields, docs, this.fullVersion, this.enc);
    }
    return encodeBatchRows(this.fields, docs, this.fullVersion);
  }

  /** Decode any batch packet (row dense/sparse, columnar, transformed). */
  async decodeBatch(
    data: Uint8Array,
  ): Promise<{ version: number; docs: Record<string, unknown>[] }> {
    return decodeAnansiBatch(data, this.fields, this.dec);
  }

  /**
   * Columnar batch encode. Note: columnar output with transforms enabled is
   * pending in TypeScript — with transforms unset this emits plain packets.
   */
  async encodeColumnar(docs: Record<string, unknown>[]): Promise<Uint8Array> {
    const hasTransforms =
      this.enc.compression || this.enc.integrity || !!this.enc.encryptionKey;
    if (hasTransforms) {
      return encodeAnansiBatchColumnar(this.fields, docs, this.fullVersion, this.enc);
    }
    // Plain path delegates through transforms module too — identical bytes,
    // but keeps a single code path for future transform enablement.
    return encodeAnansiBatchColumnar(this.fields, docs, this.fullVersion, {});
  }
}
