package anansi

import (
	"bytes"
	"fmt"

	"github.com/asaidimu/go-anansi/v8/core/data/container"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// batchFlagColumnar and batchFlagSparse are the two bits of the Batch
// packet's own batch_flags byte (spec 3.3): bit 0 orientation, bit 1
// density. This codec only implements row-oriented batches (orientation
// bit always 0); see the package doc comment for scope.
const (
	batchFlagColumnar = 0x01
	batchFlagSparse   = 0x02
)

// EncodeBatchRows encodes docs (each bound to schema slot 0 of cs) as a
// single row-oriented Batch packet (spec 3.3.1/3.3.2). All records use the
// same per-record packing (Dense or Sparse), chosen once via the same
// density heuristic as SerializeAnansi, evaluated over the batch's average
// density so a batch of mixed-density documents doesn't thrash between
// per-record formats — record boundaries are self-delineating either way
// (a fixed-size state map for Dense, an explicit field_count for Sparse),
// so the choice only affects size/speed, not correctness.
func EncodeBatchRows(cs *definition.CompiledSchema, docs []*container.DataContainer, fullVersion uint16) ([]byte, error) {
	epoch, version, err := schemaVersionByte(fullVersion)
	if err != nil {
		return nil, err
	}
	h := header{Version: version}
	h.Flags = Flags(epoch)<<flagEpochShift | Flags(PacketBatch)

	fields, err := collectWireFields(cs, rootSlot, nil)
	if err != nil {
		return nil, err
	}

	useSparse := false
	if len(fields) > 0 && len(docs) > 0 {
		var totalPresent, totalPossible int
		for _, doc := range docs {
			for _, wf := range fields {
				if fieldStateOf(doc, wf.key) != stateNotSet {
					totalPresent++
				}
			}
			totalPossible += len(fields)
		}
		density := float64(totalPresent) / float64(totalPossible)
		useSparse = len(fields) > 64 && density <= 0.25
	}

	buf := bytes.NewBuffer(nil)
	buf.WriteByte(byte(h.Flags))
	buf.WriteByte(h.Version)
	writeUvarintTo(buf, uint64(len(docs)))

	batchFlags := byte(0)
	if useSparse {
		batchFlags |= batchFlagSparse
	}
	buf.WriteByte(batchFlags)

	for i, doc := range docs {
		if useSparse {
			if err := encodeSparseBody(buf, cs, h, doc, fields); err != nil {
				return nil, fmt.Errorf("anansi: encode batch record %d: %w", i, err)
			}
		} else {
			if err := encodeDenseBody(buf, cs, h, doc, fields); err != nil {
				return nil, fmt.Errorf("anansi: encode batch record %d: %w", i, err)
			}
		}
	}
	return buf.Bytes(), nil
}

// DecodeBatchRows decodes a row-oriented Batch packet produced by
// EncodeBatchRows into freshly allocated (or pool-sourced, if pool is
// non-nil) *container.DataContainer values bound to schema slot 0 of cs.
func DecodeBatchRows(cs *definition.CompiledSchema, data []byte, pool *container.Pool) ([]*container.DataContainer, uint16, error) {
	r := newByteReader(data)
	h, err := readHeader(r)
	if err != nil {
		return nil, 0, err
	}
	if h.Flags.PacketType() != PacketBatch {
		return nil, 0, fmt.Errorf("anansi: DecodeBatchRows: packet type is %s, not Batch", h.Flags.PacketType())
	}
	if h.Flags.Compressed() || h.Flags.Encrypted() {
		return nil, 0, fmt.Errorf("anansi: compressed/encrypted batch packets are not supported by this codec")
	}
	if h.Flags.HashPresent() {
		if _, err := r.readN(16); err != nil {
			return nil, 0, fmt.Errorf("anansi: read batch packet hash: %w", err)
		}
	}

	recordCount, err := r.readUvarint()
	if err != nil {
		return nil, 0, fmt.Errorf("anansi: read batch record count: %w", err)
	}
	batchFlags, err := r.readByte()
	if err != nil {
		return nil, 0, fmt.Errorf("anansi: read batch flags: %w", err)
	}
	if batchFlags&batchFlagColumnar != 0 {
		return nil, 0, fmt.Errorf("anansi: columnar batch packets are not supported by this codec")
	}
	useSparse := batchFlags&batchFlagSparse != 0

	fields, err := collectWireFields(cs, rootSlot, nil)
	if err != nil {
		return nil, 0, err
	}

	docs := make([]*container.DataContainer, 0, recordCount)
	for i := uint64(0); i < recordCount; i++ {
		var doc *container.DataContainer
		if pool != nil {
			doc = pool.Get()
		} else {
			doc = container.NewDataContainer()
		}
		if useSparse {
			if err := decodeSparseBody(r, cs, h, doc, fields, pool); err != nil {
				return nil, 0, fmt.Errorf("anansi: decode batch record %d: %w", i, err)
			}
		} else {
			if err := decodeDenseBody(r, cs, h, doc, fields, pool); err != nil {
				return nil, 0, fmt.Errorf("anansi: decode batch record %d: %w", i, err)
			}
		}
		docs = append(docs, doc)
	}
	return docs, h.FullVersion(), nil
}
