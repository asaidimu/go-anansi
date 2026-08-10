package bits

import (
	"fmt"
	"sync/atomic"
	"testing"
)

// benchSizes are the blob sizes exercised by most benchmarks.
var benchSizes = []int{16, 256, 4096}

// appendBatch is how many entries append-style benchmarks write before
// resetting the record. A fresh header avoids exhausting the 65536-slot
// capacity while keeping Clear (cheap) off the steady-state timing path.
const appendBatch = 20000

func benchSpec(b *testing.B, lengthBits uint8) HandleSpec {
	b.Helper()
	spec, err := NewHandleSpec(lengthBits)
	if err != nil {
		b.Fatalf("NewHandleSpec(%d): %v", lengthBits, err)
	}
	return spec
}

// benchRecord builds a Record for benchmarking. When assignIDs is true the
// OnSetID hook stamps each entry with a unique sequential ID.
func benchRecord(b *testing.B, lengthBits uint8, assignIDs bool) *Record {
	b.Helper()
	spec := benchSpec(b, lengthBits)
	cfg := Config{HandleSpec: spec}
	if assignIDs {
		var next uint64
		cfg.OnSetID = func(h Handle, width uint8) Handle {
			if next >= spec.MaxID() {
				b.Fatalf("OnSetID: ID space exhausted at %d (spec max %d)", next, spec.MaxID())
			}
			next++
			out, err := spec.SetID(h, next)
			if err != nil {
				b.Fatalf("SetID: %v", err)
			}
			return out
		}
	}
	return NewRecord(cfg)
}

func benchBlob(n int) []byte {
	blob := make([]byte, n)
	for i := range blob {
		blob[i] = byte(i)
	}
	return blob
}

// fill sets count blobs of the given size and returns their handles.
func fill(b *testing.B, r *Record, blob []byte, count int) []Handle {
	b.Helper()
	handles := make([]Handle, 0, count)
	for i := 0; i < count; i++ {
		h, err := r.Set(blob)
		if err != nil {
			b.Fatalf("Set: %v", err)
		}
		handles = append(handles, h)
	}
	return handles
}

func BenchmarkRecordSet(b *testing.B) {
	for _, n := range benchSizes {
		b.Run(fmt.Sprintf("%dB", n), func(b *testing.B) {
			blob := benchBlob(n)
			r := benchRecord(b, 32, false)
			defer r.Close()
			b.SetBytes(int64(n))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if i%appendBatch == 0 {
					r.Clear()
				}
				if _, err := r.Set(blob); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkRecordSetWithIDs(b *testing.B) {
	blob := benchBlob(256)
	// 16 length bits leave 32 bits of ID space, so the monotonic ID counter
	// cannot overflow for realistically-sized runs.
	r := benchRecord(b, 16, true)
	defer r.Close()
	b.SetBytes(int64(len(blob)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%appendBatch == 0 {
			r.Clear()
		}
		if _, err := r.Set(blob); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRecordChurn exercises the steady-state delete+reuse path: a fixed
// window of live entries is rotated, so every Set lands in a reclaimed slot
// and reuses a just-freed hole instead of appending to data.
func BenchmarkRecordChurn(b *testing.B) {
	blob := benchBlob(256)
	r := benchRecord(b, 16, true)
	defer r.Close()
	const window = 64
	live := fill(b, r, blob, window)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx := i % window
		r.Delete(live[idx])
		h, err := r.Set(blob)
		if err != nil {
			b.Fatal(err)
		}
		live[idx] = h
	}
}

func BenchmarkRecordDelete(b *testing.B) {
	blob := benchBlob(256)
	r := benchRecord(b, 16, true)
	defer r.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		h, err := r.Set(blob)
		if err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
		r.Delete(h)
	}
}

func BenchmarkRecordGet(b *testing.B) {
	for _, n := range benchSizes {
		b.Run(fmt.Sprintf("%dB", n), func(b *testing.B) {
			blob := benchBlob(n)
			r := benchRecord(b, 32, false)
			defer r.Close()
			handles := fill(b, r, blob, 256)
			b.SetBytes(int64(n))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if got := r.Get(handles[i&0xFF]); len(got) != n {
					b.Fatalf("Get length = %d, want %d", len(got), n)
				}
			}
		})
	}
}

// BenchmarkRecordConcurrentGet measures the read-side claim of the
// concurrency model: concurrent readers take RLock and never block each other.
func BenchmarkRecordConcurrentGet(b *testing.B) {
	blob := benchBlob(256)
	r := benchRecord(b, 32, false)
	defer r.Close()
	handles := fill(b, r, blob, 64)
	b.ReportAllocs()
	b.ResetTimer()

	var total atomic.Int64
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		var sum int64
		for pb.Next() {
			sum += int64(len(r.Get(handles[i&63])))
			i++
		}
		total.Add(sum)
	})
	if total.Load() == 0 {
		b.Fatal("Get returned nothing")
	}
}

func BenchmarkRecordHandle(b *testing.B) {
	blob := benchBlob(256)
	r := benchRecord(b, 16, true)
	defer r.Close()
	const count = 1024
	fill(b, r, blob, count)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := uint64(i%count) + 1
		if h, ok := r.Handle(id); !ok || h == 0 {
			b.Fatalf("Handle(%d) not found", id)
		}
	}
}

func BenchmarkRecordKeys(b *testing.B) {
	blob := benchBlob(256)
	r := benchRecord(b, 32, false)
	defer r.Close()
	const count = 1024
	fill(b, r, blob, count)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if keys := r.Keys(); len(keys) != count {
			b.Fatalf("Keys length = %d, want %d", len(keys), count)
		}
	}
}

func BenchmarkRecordClone(b *testing.B) {
	blob := benchBlob(256)
	r := benchRecord(b, 32, true)
	defer r.Close()
	const count = 1024
	fill(b, r, blob, count)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := r.Clone()
		if c.Length() != count {
			b.Fatalf("clone Length = %d, want %d", c.Length(), count)
		}
		c.Close()
	}
}

// BenchmarkRecordRead measures the zero-copy bulk view used for serialization.
func BenchmarkRecordRead(b *testing.B) {
	for _, n := range benchSizes {
		b.Run(fmt.Sprintf("%dB", n), func(b *testing.B) {
			blob := benchBlob(n)
			r := benchRecord(b, 32, true)
			defer r.Close()
			const count = 1024
			fill(b, r, blob, count)
			payload := len(blob) * count
			b.SetBytes(int64(payload))
			b.ReportAllocs()
			b.ResetTimer()

			var sink int
			for i := 0; i < b.N; i++ {
				r.Read(func(entries []Handle, data []byte) {
					sink += len(entries) + len(data)
				})
			}
			if sink == 0 {
				b.Fatal("Read saw no data")
			}
		})
	}
}

// BenchmarkRecordWrite measures bulk replacement: deriving each entry's byte
// position from handle lengths and rebuilding header, slotToEntry, and idIndex.
func BenchmarkRecordWrite(b *testing.B) {
	for _, n := range benchSizes {
		b.Run(fmt.Sprintf("%dB", n), func(b *testing.B) {
			blob := benchBlob(n)
			r := benchRecord(b, 32, true)
			defer r.Close()
			const count = 1024
			fill(b, r, blob, count)

			var entries []Handle
			var data []byte
			r.Read(func(e []Handle, d []byte) {
				entries = append([]Handle(nil), e...)
				data = append([]byte(nil), d...)
			})
			payload := len(blob) * count
			b.SetBytes(int64(payload))
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				if err := r.Write(func() ([]Handle, []byte) {
					return entries, data
				}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
