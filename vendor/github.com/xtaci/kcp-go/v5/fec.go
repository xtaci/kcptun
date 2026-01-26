// The MIT License (MIT)
//
// Copyright (c) 2015 xtaci
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

// THE GENERALIZED REED-SOLOMON FEC SCHEME
//
// Encoding:
// -----------
// Message:         | M1 | M2 | M3 | M4 |
// Generate Parity: | P1 | P2 |
// Encoded Codeword:| M1 | M2 | M3 | M4 | P1 | P2 |
//
// Decoding with Erasures:
// ------------------------
// Received:        | M1 | ?? | M3 | M4 | P1 | ?? |
// Erasures:        |    | E1 |    |    |    | E2 |
// Syndromes:       S1, S2, ...
// Error Locator:   Λ(x) = ...
// Correct Erasures:Determine values for E1 (M2) and E2 (P2).
// Corrected:       | M1 | M2 | M3 | M4 | P1 | P2 |

package kcp

import (
	"container/heap"
	"encoding/binary"
	"sync/atomic"
	"time"

	"github.com/klauspost/reedsolomon"
)

const (
	fecHeaderSize      = 6
	fecHeaderSizePlus2 = fecHeaderSize + 2 // plus 2B data size
	typeData           = 0xf1
	typeParity         = 0xf2
	typeOOB            = 0xf3
	maxShardSets       = 3
)

// fecPacket is a decoded FEC packet
type fecPacket []byte

func (bts fecPacket) seqid() uint32 { return binary.LittleEndian.Uint32(bts) }
func (bts fecPacket) flag() uint16  { return binary.LittleEndian.Uint16(bts[4:]) }
func (bts fecPacket) data() []byte  { return bts[6:] }

// shardHeap holds a corelated set of datashards from the peers
type shardHeap struct {
	elements []fecPacket
	marks    map[uint32]struct{} // to avoid duplicates
}

func newShardHeap() *shardHeap {
	h := &shardHeap{
		marks: make(map[uint32]struct{}),
	}
	heap.Init(h)
	return h
}

func (h *shardHeap) Len() int { return len(h.elements) }

func (h *shardHeap) Less(i, j int) bool {
	return _itimediff(h.elements[j].seqid(), h.elements[i].seqid()) > 0
}

func (h *shardHeap) Swap(i, j int) { h.elements[i], h.elements[j] = h.elements[j], h.elements[i] }
func (h *shardHeap) Push(x any) {
	h.elements = append(h.elements, x.(fecPacket))
	h.marks[x.(fecPacket).seqid()] = struct{}{}
}

func (h *shardHeap) Pop() any {
	n := len(h.elements)
	x := h.elements[n-1]
	h.elements = h.elements[0 : n-1]
	delete(h.marks, x.seqid())
	return x
}

func (h *shardHeap) Has(sn uint32) bool {
	_, exists := h.marks[sn]
	return exists
}

// fecDecoder for decoding incoming packets
type fecDecoder struct {
	dataShards   int
	parityShards int
	shardSize    int
	shardSet     map[uint32]*shardHeap // shardMap[initial shard id] = shardHeap
	paws         uint32                // Protect Against Wrapped Sequence numbers

	// record the latest recovered shard id
	// the shards smaller than this one will be discarded
	newestShardId uint32

	// caches
	decodeCache [][]byte
	flagCache   []bool

	// RS decoder
	codec reedsolomon.Encoder

	// auto tune fec parameter
	autoTune   autoTune
	shouldTune bool
}

func newFECDecoder(dataShards, parityShards int) *fecDecoder {
	if dataShards <= 0 || parityShards <= 0 {
		return nil
	}

	if dataShards+parityShards > 256 {
		return nil
	}

	dec := new(fecDecoder)
	dec.dataShards = dataShards
	dec.parityShards = parityShards
	dec.shardSize = dataShards + parityShards
	dec.shardSet = make(map[uint32]*shardHeap)
	dec.paws = 0xffffffff / uint32(dec.shardSize) * uint32(dec.shardSize)
	codec, err := reedsolomon.New(dataShards, parityShards)
	if err != nil {
		return nil
	}
	dec.codec = codec
	dec.decodeCache = make([][]byte, dec.shardSize)
	dec.flagCache = make([]bool, dec.shardSize)
	return dec
}

// decode a fec packet
func (dec *fecDecoder) decode(in fecPacket) (recovered [][]byte) {
	// Sample the packet type for auto-tuning
	if in.flag() == typeData {
		dec.autoTune.Sample(true, in.seqid())
	} else {
		dec.autoTune.Sample(false, in.seqid())
	}

	// check seqid < paws to avoid invalid packets
	if in.seqid() >= dec.paws {
		return nil
	}

	// check if the packet type matches the current FEC parameters
	if in.seqid()%uint32(dec.shardSize) < uint32(dec.dataShards) {
		if in.flag() != typeData { // expect typeData
			dec.shouldTune = true
		}
	} else {
		if in.flag() != typeParity {
			dec.shouldTune = true
		}
	}

	// perform auto-tuning if the decoder is out of sync.
	if dec.shouldTune {
		autoDS := dec.autoTune.FindPeriod(true)
		autoPS := dec.autoTune.FindPeriod(false)

		// validate the auto-tuned parameters
		if autoDS > 0 && autoPS > 0 && autoDS+autoPS < 256 {
			if autoDS != dec.dataShards || autoPS != dec.parityShards {
				// apply the new FEC parameters
				dec.dataShards = autoDS
				dec.parityShards = autoPS
				dec.shardSize = autoDS + autoPS
				// recycle old shards before creating new shardSet
				for _, shard := range dec.shardSet {
					for _, pkt := range shard.elements {
						defaultBufferPool.Put(pkt)
					}
				}
				dec.shardSet = make(map[uint32]*shardHeap) // empty the shard set
				codec, err := reedsolomon.New(autoDS, autoPS)
				if err != nil {
					return nil
				}
				dec.codec = codec
				dec.decodeCache = make([][]byte, dec.shardSize)
				dec.flagCache = make([]bool, dec.shardSize)
				dec.paws = 0xffffffff / uint32(dec.shardSize) * uint32(dec.shardSize)
				//log.Println("autotune to :", dec.dataShards, dec.parityShards)
			}
			// reset shouldTune flag regardless of whether parameters changed
			// to avoid permanent blocking when detected parameters match current ones
			dec.shouldTune = false
		}
		return nil
	}

	// get the shard heap for this shard id
	shardId := dec.getShardId(in.seqid())
	shard, ok := dec.shardSet[shardId]
	if !ok {
		shard = newShardHeap()
		dec.shardSet[shardId] = shard
		atomic.AddUint64(&DefaultSnmp.FECShardSet, 1)
	}

	// ignore duplicate packets
	if shard.Has(in.seqid()) {
		return nil
	}

	// update statistics for parity shards
	if in.flag() == typeParity {
		atomic.AddUint64(&DefaultSnmp.FECParityShards, 1)
	}

	// push the packet into the shard heap
	pkt := fecPacket(defaultBufferPool.Get()[:len(in)])
	copy(pkt, in)
	shard.Push(pkt)

	// try to recover data if we have enough shards
	if shard.Len() >= dec.dataShards {
		var numDataShard, maxlen int

		// prepare the decode cache
		shards := dec.decodeCache
		shardsflag := dec.flagCache
		for k := range dec.decodeCache {
			shards[k] = nil
			shardsflag[k] = false
		}

		// pop all shards from the heap and fill into the decode cache
		var pkts []fecPacket
		for shard.Len() > 0 {
			pkt := shard.Pop().(fecPacket)
			pkts = append(pkts, pkt)
			seqid := pkt.seqid()
			shards[seqid%uint32(dec.shardSize)] = pkt.data()
			shardsflag[seqid%uint32(dec.shardSize)] = true
			if pkt.flag() == typeData {
				numDataShard++
			}
			if len(pkt.data()) > maxlen {
				maxlen = len(pkt.data())
			}
		}

		// case 1: all data shards are present
		if numDataShard == dec.dataShards {
			atomic.AddUint64(&DefaultSnmp.FECFullShardSet, 1)
		} else { // case 2: some data shards are missing, try to recover
			// fill '0' into the tail of each shard to make them equal-sized
			var newBuffers [][]byte
			for k := range shards {
				if shards[k] != nil {
					dlen := len(shards[k])
					shards[k] = shards[k][:maxlen]
					clear(shards[k][dlen:])
				} else if k < dec.dataShards {
					// prepare memory for the data recovery
					shards[k] = defaultBufferPool.Get()[:0]
					newBuffers = append(newBuffers, shards[k])
				}
			}

			// Reed-Solomon Erasure Code Decoding
			if err := dec.codec.ReconstructData(shards); err == nil {
				for k := range shards[:dec.dataShards] {
					if !shardsflag[k] {
						recovered = append(recovered, shards[k])
					}
				}
			} else {
				// recovery failed, record the error
				atomic.AddUint64(&DefaultSnmp.FECErrs, 1)
				// recycle new buffers if failed
				// NOTE: if success, these buffers are returned in 'recovered' and will be recycled by the caller
				for _, buf := range newBuffers {
					defaultBufferPool.Put(buf)
				}
			}

			// record the number of recovered packets
			atomic.AddUint64(&DefaultSnmp.FECRecovered, uint64(len(recovered)))
		}

		// recycle the packets
		for _, pkt := range pkts {
			defaultBufferPool.Put(pkt)
		}
	}

	// update the newest shard id
	if _itimediff(shardId*uint32(dec.shardSize), dec.newestShardId*uint32(dec.shardSize)) > 0 {
		dec.newestShardId = shardId
		atomic.StoreUint64(&DefaultSnmp.FECShardMin, uint64(dec.newestShardId))
	}

	// try to discard shard sets that are too old
	dec.discardShards()

	return
}

// getShardId calculates the shard id based on the sequence id
func (dec *fecDecoder) getShardId(seqid uint32) uint32 {
	return seqid / uint32(dec.shardSize)
}

// discardShards removes shards that are too old from the shardSet
func (dec *fecDecoder) discardShards() {
	for shardId, shard := range dec.shardSet {
		// discard shards that are too old
		if _itimediff(dec.newestShardId*uint32(dec.shardSize), shardId*uint32(dec.shardSize)) > maxShardSets*int32(dec.shardSize) {
			//println("flushing shard", shardId, "minShardId", dec.minShardId, _itimediff(dec.minShardId, shardId))
			for _, pkt := range shard.elements {
				defaultBufferPool.Put(pkt)
			}
			delete(dec.shardSet, shardId)
		}
	}

	atomic.StoreUint64(&DefaultSnmp.FECShardSet, uint64(len(dec.shardSet)))
}

type (
	// fecEncoder for encoding outgoing packets
	fecEncoder struct {
		dataShards   int
		parityShards int
		shardSize    int
		paws         uint32 // Protect Against Wrapped Sequence numbers
		next         uint32 // next seqid

		shardCount int // count the number of datashards collected
		maxSize    int // track maximum data length in datashard

		headerOffset  int // FEC header offset
		payloadOffset int // FEC payload offset

		// caches
		shardCache     [][]byte
		encodeCache    [][]byte
		tsLatestPacket int64

		// RS encoder
		codec reedsolomon.Encoder
	}
)

func newFECEncoder(dataShards, parityShards, offset int) *fecEncoder {
	if dataShards <= 0 || parityShards <= 0 {
		return nil
	}
	enc := new(fecEncoder)
	enc.dataShards = dataShards
	enc.parityShards = parityShards
	enc.shardSize = dataShards + parityShards
	enc.paws = 0xffffffff / uint32(enc.shardSize) * uint32(enc.shardSize)
	enc.headerOffset = offset
	enc.payloadOffset = enc.headerOffset + fecHeaderSize

	codec, err := reedsolomon.New(dataShards, parityShards)
	if err != nil {
		return nil
	}
	enc.codec = codec

	// caches
	enc.encodeCache = make([][]byte, enc.shardSize)
	enc.shardCache = make([][]byte, enc.shardSize)
	for k := range enc.shardCache {
		enc.shardCache[k] = make([]byte, mtuLimit)
	}
	return enc
}

// encodes the packet, outputs parity shards if we have collected quorum datashards
// notice: the contents of 'ps' will be re-written in successive calling
func (enc *fecEncoder) encode(b []byte, rto uint32) (ps [][]byte) {
	// The header format:
	// | FEC SEQID(4B) | FEC TYPE(2B) | SIZE (2B) | PAYLOAD(SIZE-2) |
	// |<-headerOffset                |<-payloadOffset
	enc.sealData(b[enc.headerOffset:])
	binary.LittleEndian.PutUint16(b[enc.payloadOffset:], uint16(len(b[enc.payloadOffset:])))

	// copy data from payloadOffset to fec shard cache
	sz := len(b)
	enc.shardCache[enc.shardCount] = enc.shardCache[enc.shardCount][:sz]
	copy(enc.shardCache[enc.shardCount][enc.payloadOffset:], b[enc.payloadOffset:])
	enc.shardCount++

	// track max datashard length
	if sz > enc.maxSize {
		enc.maxSize = sz
	}

	// Generation of Reed-Solomon Erasure Code when we have enough datashards
	now := time.Now().UnixMilli()
	if enc.shardCount == enc.dataShards {
		// Generate the parity shards if we collect enough datashards,
		// the continuity is determined by the time interval between
		// the latest 2 data packets.
		//
		// If the interval is larger than rto, we consider the data is non-continuous,
		// thus we skip this parity generation to avoid useless parity packets.
		//
		// Note that, even we skip this parity generation, we still need to
		// increase the seqid to keep the monotonic increasing property.
		// This is important for the receiver to detect lost packets.
		// see fecEncoder.skipParity()
		// also note that the rto is in milliseconds.
		// see kcp.UDPSession.rto()
		//
		if now-enc.tsLatestPacket < int64(rto) {
			// clear the tail of each datashard to make them equal-sized
			for i := 0; i < enc.dataShards; i++ {
				shard := enc.shardCache[i]
				slen := len(shard)
				clear(shard[slen:enc.maxSize])
			}

			// construct equal-sized slice with stripped header
			cache := enc.encodeCache
			for k := range cache {
				cache[k] = enc.shardCache[k][enc.payloadOffset:enc.maxSize]
			}

			// Reed-Solomon Erasure Code Encoding
			if err := enc.codec.Encode(cache); err == nil {
				ps = enc.shardCache[enc.dataShards:]
				for k := range ps {
					enc.sealParity(ps[k][enc.headerOffset:]) // NOTE(x): seal parity will increase the seqid by 1
					ps[k] = ps[k][:enc.maxSize]
				}
			} else {
				// encoding failed, record the error but keep the seqid monotonic increasing
				atomic.AddUint64(&DefaultSnmp.FECErrs, 1)
				enc.skipParity()
			}
		} else {
			// Non-continuous data detected, skip this parity generation.
			// Through we do not send non-continuous parity shard, we still need to increase seqid.
			enc.skipParity()
		}

		// reset shard count and max size
		enc.shardCount = 0
		enc.maxSize = 0
	}

	// record the time of the latest data packet
	enc.tsLatestPacket = now

	return
}

// sealData and sealParity write the sequence number and type into the FEC header
func (enc *fecEncoder) sealData(data []byte) {
	binary.LittleEndian.PutUint32(data, enc.next)
	binary.LittleEndian.PutUint16(data[4:], typeData)
	enc.next = (enc.next + 1) % enc.paws
}

func (enc *fecEncoder) sealParity(data []byte) {
	binary.LittleEndian.PutUint32(data, enc.next)
	binary.LittleEndian.PutUint16(data[4:], typeParity)
	enc.next = (enc.next + 1) % enc.paws
}

// encodeOOB encodes an out-of-band packet
func (enc *fecEncoder) encodeOOB(b []byte) {
	enc.sealOOB(b[enc.headerOffset:])
	binary.LittleEndian.PutUint16(b[enc.payloadOffset:], uint16(len(b[enc.payloadOffset:])))
}

// sealOOB seals an out-of-band packet
func (enc *fecEncoder) sealOOB(data []byte) {
	binary.LittleEndian.PutUint32(data, uint32(0xffffffff)) // use max uint32 as OOB seqid
	binary.LittleEndian.PutUint16(data[4:], typeOOB)
}

// skipParity skips the whole parity block by advancing the seqid
func (enc *fecEncoder) skipParity() {
	enc.next = (enc.next + uint32(enc.parityShards)) % enc.paws
}
