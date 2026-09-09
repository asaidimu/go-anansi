// Varint codecs — LEB128 unsigned + zigzag signed (spec 2.4).

export function putUvarint(buf: number[], v: number): void {
  v = v >>> 0;
  while (v >= 0x80) {
    buf.push((v & 0x7f) | 0x80);
    v >>>= 7;
  }
  buf.push(v);
}

export function putVarint(buf: number[], v: number): void {
  if (!Number.isSafeInteger(v)) {
    throw new Error(`anansi: integer ${v} exceeds Number.MAX_SAFE_INTEGER`);
  }
  putVarintBig(buf, BigInt(v));
}

export function putUvarintBig(buf: number[], v: bigint): void {
  while (v >= 0x80n) {
    buf.push(Number(v & 0x7fn) | 0x80);
    v >>= 7n;
  }
  buf.push(Number(v));
}

export function putVarintBig(buf: number[], v: bigint): void {
  putUvarintBig(buf, (v << 1n) ^ (v >> 63n));
}

export class Reader {
  constructor(public data: Uint8Array, public pos = 0) {}
  eof(): boolean { return this.pos >= this.data.length; }
  remaining(): number { return this.data.length - this.pos; }

  byte(): number {
    if (this.eof()) throw new Error("anansi: unexpected end of buffer");
    return this.data[this.pos++];
  }

  take(n: number): Uint8Array {
    if (n < 0 || this.pos + n > this.data.length) {
      throw new Error(`anansi: unexpected end of buffer (need ${n}, have ${this.remaining()})`);
    }
    const out = this.data.subarray(this.pos, this.pos + n);
    this.pos += n;
    return out;
  }

  uvarint(): number {
    let x = 0, s = 0;
    for (let i = 0; i < 10; i++) {
      const b = this.byte();
      if (b < 0x80) {
        if (i === 9 && b > 1) throw new Error("anansi: varint overflow");
        return (x | (b << s)) >>> 0;
      }
      x |= (b & 0x7f) << s;
      s += 7;
    }
    throw new Error("anansi: varint overflow");
  }

  uvarintBig(): bigint {
    let x = 0n, s = 0n, i = 0;
    while (i < 10) {
      const b = this.byte();
      x |= BigInt(b & 0x7f) << s;
      if (b < 0x80) return x;
      s += 7n; i++;
    }
    throw new Error("anansi: varint overflow");
  }

  varint(): number {
    const z = this.uvarintBig();
    const v = (z >> 1n) ^ -(z & 1n);
    const n = Number(v);
    if (!Number.isSafeInteger(n)) {
      throw new Error(`anansi: decoded integer ${n} exceeds Number.MAX_SAFE_INTEGER`);
    }
    return n;
  }
}
