package anansijson

import (
	"encoding/binary"
	"math/bits"
)

// ============================================================================
// STRING INTERNING
// ============================================================================
//
// Decoded strings must be real copies — they may never alias the input
// buffer, which the caller is free to reuse after Unmarshal returns. But a
// fresh copy per string value dominates the decoder's allocation count. A
// direct-mapped cache turns repeated values (enum-like statuses, city names,
// SKUs, tags, or simply re-decoded payloads) into zero-allocation hits.
// Cached strings are owned by the cache, so the no-alias contract still
// holds; the cache retains at most 256 strings of at most 256 bytes, about
// 64 KiB per pooled parser.

type stringCache = [256]string

// makeString returns b as a string, reusing a cached copy when one matches.
// Values outside [2, 256] bytes bypass the cache: single bytes are interned
// by the runtime already, and longer strings are too valuable to retain.
// Lookups probe the hashed slot and its successor, so a pair of strings that
// collide on the same slot can coexist instead of evicting each other every
// call; the empty string marks a free slot (cached values are 2+ bytes).
func (ps *parser) makeString(b []byte) string {
	if len(b) < 2 || len(b) > 256 {
		return string(b)
	}

	var lo, hi uint32
	switch {
	case len(b) >= 8:
		lo = binary.LittleEndian.Uint32(b[:4]) ^ binary.LittleEndian.Uint32(b[4:8])
		hi = binary.LittleEndian.Uint32(b[len(b)-8:]) ^ binary.LittleEndian.Uint32(b[len(b)-4:])
	case len(b) >= 4:
		lo = binary.LittleEndian.Uint32(b[:4])
		hi = binary.LittleEndian.Uint32(b[len(b)-4:])
	default:
		lo = uint32(binary.LittleEndian.Uint16(b[:2]))
		hi = uint32(binary.LittleEndian.Uint16(b[len(b)-2:]))
	}

	i := hashString(lo, hi) % uint32(len(ps.cache))
	j := (i + 1) % uint32(len(ps.cache))
	if s := ps.cache[i]; s == string(b) {
		return s
	}
	if s := ps.cache[j]; s == string(b) {
		return s
	}
	s := string(b)
	switch {
	case ps.cache[i] == "":
		ps.cache[i] = s
	case ps.cache[j] == "":
		ps.cache[j] = s
	default:
		ps.cache[i] = s
	}
	return s
}

// hashString mixes the head and tail words of a string. The comparison in
// makeString is authoritative, so the hash only needs to spread 256 slots
// evenly, not resist collisions — but correlated inputs (strings sharing
// prefixes and suffixes, like repeated field values) still need the full
// avalanche finalizer to avoid slot clustering and cache thrashing.
func hashString(lo, hi uint32) uint32 {
	const (
		seed   = 0x9e3779b1
		prime2 = 0x85ebca77
		prime3 = 0xc2b2ae3d
		prime4 = 0x27d4eb2f
	)
	h := seed + uint32(8)
	h += lo * prime3
	h = bits.RotateLeft32(h, 17) * prime4
	h += hi * prime3
	h = bits.RotateLeft32(h, 17) * prime4
	h ^= h >> 15
	h *= prime2
	h ^= h >> 13
	h *= prime3
	h ^= h >> 16
	return h
}
