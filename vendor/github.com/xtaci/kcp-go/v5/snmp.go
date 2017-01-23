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

//go:build !kcp_no_snmp

package kcp

import (
	"strconv"
	"sync/atomic"
)

// Snmp contains network statistics.
type Snmp struct {
	BytesSent           uint64 // application bytes sent
	BytesReceived       uint64 // application bytes received
	MaxConn             uint64 // max number of connections ever reached
	ActiveOpens         uint64 // accumulated active open connections
	PassiveOpens        uint64 // accumulated passive open connections
	CurrEstab           uint64 // current number of established connections
	InErrs              uint64 // UDP read errors reported from net.PacketConn
	InCsumErrors        uint64 // checksum errors from CRC32
	KCPInErrors         uint64 // packet input errors reported from KCP
	InPkts              uint64 // incoming packets count
	OutPkts             uint64 // outgoing packets count
	InSegs              uint64 // incoming KCP segments
	OutSegs             uint64 // outgoing KCP segments
	InBytes             uint64 // UDP bytes received
	OutBytes            uint64 // UDP bytes sent
	RetransSegs         uint64 // accumulated retransmitted segments
	FastRetransSegs     uint64 // accumulated fast retransmitted segments
	EarlyRetransSegs    uint64 // accumulated early retransmitted segments
	LostSegs            uint64 // number of segs inferred as lost
	RepeatSegs          uint64 // number of segs duplicated
	FECFullShardSet     uint64 // number of shard sets received with all data shards present
	FECRecovered        uint64 // number of packets recovered by FEC
	FECErrs             uint64 // number of FEC encoding or recovery errors
	FECParityShards     uint64 // number of FEC parity shards received
	FECShardSet         uint64 // number of shard sets that are not yet complete
	FECShardMin         uint64 // minimum shard ID among active FEC shard sets
	RingBufferSndQueue  uint64 // Len of segments in send queue ring buffer
	RingBufferRcvQueue  uint64 // Len of segments in receive queue ring buffer
	RingBufferSndBuffer uint64 // Len of segments in send buffer ring buffer
	OOBPackets          uint64 // number of OOB packets received
}

func newSnmp() *Snmp {
	return new(Snmp)
}

// Header returns the names of all fields.
func (s *Snmp) Header() []string {
	return []string{
		"BytesSent",
		"BytesReceived",
		"MaxConn",
		"ActiveOpens",
		"PassiveOpens",
		"CurrEstab",
		"InErrs",
		"InCsumErrors",
		"KCPInErrors",
		"InPkts",
		"OutPkts",
		"InSegs",
		"OutSegs",
		"InBytes",
		"OutBytes",
		"RetransSegs",
		"FastRetransSegs",
		"EarlyRetransSegs",
		"LostSegs",
		"RepeatSegs",
		"FECFullShards",
		"FECParityShards",
		"FECErrs",
		"FECRecovered",
		"FECShardSet",
		"FECShardMin",
		"RingBufferSndQueue",
		"RingBufferRcvQueue",
		"RingBufferSndBuffer",
		"OOBPackets",
	}
}

// ToSlice returns the current SNMP information as a slice.
func (s *Snmp) ToSlice() []string {
	snmp := s.Copy()
	return []string{
		strconv.FormatUint(snmp.BytesSent, 10),
		strconv.FormatUint(snmp.BytesReceived, 10),
		strconv.FormatUint(snmp.MaxConn, 10),
		strconv.FormatUint(snmp.ActiveOpens, 10),
		strconv.FormatUint(snmp.PassiveOpens, 10),
		strconv.FormatUint(snmp.CurrEstab, 10),
		strconv.FormatUint(snmp.InErrs, 10),
		strconv.FormatUint(snmp.InCsumErrors, 10),
		strconv.FormatUint(snmp.KCPInErrors, 10),
		strconv.FormatUint(snmp.InPkts, 10),
		strconv.FormatUint(snmp.OutPkts, 10),
		strconv.FormatUint(snmp.InSegs, 10),
		strconv.FormatUint(snmp.OutSegs, 10),
		strconv.FormatUint(snmp.InBytes, 10),
		strconv.FormatUint(snmp.OutBytes, 10),
		strconv.FormatUint(snmp.RetransSegs, 10),
		strconv.FormatUint(snmp.FastRetransSegs, 10),
		strconv.FormatUint(snmp.EarlyRetransSegs, 10),
		strconv.FormatUint(snmp.LostSegs, 10),
		strconv.FormatUint(snmp.RepeatSegs, 10),
		strconv.FormatUint(snmp.FECFullShardSet, 10),
		strconv.FormatUint(snmp.FECParityShards, 10),
		strconv.FormatUint(snmp.FECErrs, 10),
		strconv.FormatUint(snmp.FECRecovered, 10),
		strconv.FormatUint(snmp.FECShardSet, 10),
		strconv.FormatUint(snmp.FECShardMin, 10),
		strconv.FormatUint(snmp.RingBufferSndQueue, 10),
		strconv.FormatUint(snmp.RingBufferRcvQueue, 10),
		strconv.FormatUint(snmp.RingBufferSndBuffer, 10),
		strconv.FormatUint(snmp.OOBPackets, 10),
	}
}

// Copy returns a copy of the current SNMP snapshot.
func (s *Snmp) Copy() *Snmp {
	d := newSnmp()
	d.BytesSent = atomic.LoadUint64(&s.BytesSent)
	d.BytesReceived = atomic.LoadUint64(&s.BytesReceived)
	d.MaxConn = atomic.LoadUint64(&s.MaxConn)
	d.ActiveOpens = atomic.LoadUint64(&s.ActiveOpens)
	d.PassiveOpens = atomic.LoadUint64(&s.PassiveOpens)
	d.CurrEstab = atomic.LoadUint64(&s.CurrEstab)
	d.InErrs = atomic.LoadUint64(&s.InErrs)
	d.InCsumErrors = atomic.LoadUint64(&s.InCsumErrors)
	d.KCPInErrors = atomic.LoadUint64(&s.KCPInErrors)
	d.InPkts = atomic.LoadUint64(&s.InPkts)
	d.OutPkts = atomic.LoadUint64(&s.OutPkts)
	d.InSegs = atomic.LoadUint64(&s.InSegs)
	d.OutSegs = atomic.LoadUint64(&s.OutSegs)
	d.InBytes = atomic.LoadUint64(&s.InBytes)
	d.OutBytes = atomic.LoadUint64(&s.OutBytes)
	d.RetransSegs = atomic.LoadUint64(&s.RetransSegs)
	d.FastRetransSegs = atomic.LoadUint64(&s.FastRetransSegs)
	d.EarlyRetransSegs = atomic.LoadUint64(&s.EarlyRetransSegs)
	d.LostSegs = atomic.LoadUint64(&s.LostSegs)
	d.RepeatSegs = atomic.LoadUint64(&s.RepeatSegs)
	d.FECFullShardSet = atomic.LoadUint64(&s.FECFullShardSet)
	d.FECParityShards = atomic.LoadUint64(&s.FECParityShards)
	d.FECErrs = atomic.LoadUint64(&s.FECErrs)
	d.FECRecovered = atomic.LoadUint64(&s.FECRecovered)
	d.FECShardSet = atomic.LoadUint64(&s.FECShardSet)
	d.FECShardMin = atomic.LoadUint64(&s.FECShardMin)
	d.RingBufferSndQueue = atomic.LoadUint64(&s.RingBufferSndQueue)
	d.RingBufferRcvQueue = atomic.LoadUint64(&s.RingBufferRcvQueue)
	d.RingBufferSndBuffer = atomic.LoadUint64(&s.RingBufferSndBuffer)
	d.OOBPackets = atomic.LoadUint64(&s.OOBPackets)
	return d
}

// Reset sets all statistics to zero.
func (s *Snmp) Reset() {
	atomic.StoreUint64(&s.BytesSent, 0)
	atomic.StoreUint64(&s.BytesReceived, 0)
	atomic.StoreUint64(&s.MaxConn, 0)
	atomic.StoreUint64(&s.ActiveOpens, 0)
	atomic.StoreUint64(&s.PassiveOpens, 0)
	atomic.StoreUint64(&s.CurrEstab, 0)
	atomic.StoreUint64(&s.InErrs, 0)
	atomic.StoreUint64(&s.InCsumErrors, 0)
	atomic.StoreUint64(&s.KCPInErrors, 0)
	atomic.StoreUint64(&s.InPkts, 0)
	atomic.StoreUint64(&s.OutPkts, 0)
	atomic.StoreUint64(&s.InSegs, 0)
	atomic.StoreUint64(&s.OutSegs, 0)
	atomic.StoreUint64(&s.InBytes, 0)
	atomic.StoreUint64(&s.OutBytes, 0)
	atomic.StoreUint64(&s.RetransSegs, 0)
	atomic.StoreUint64(&s.FastRetransSegs, 0)
	atomic.StoreUint64(&s.EarlyRetransSegs, 0)
	atomic.StoreUint64(&s.LostSegs, 0)
	atomic.StoreUint64(&s.RepeatSegs, 0)
	atomic.StoreUint64(&s.FECFullShardSet, 0)
	atomic.StoreUint64(&s.FECParityShards, 0)
	atomic.StoreUint64(&s.FECErrs, 0)
	atomic.StoreUint64(&s.FECRecovered, 0)
	atomic.StoreUint64(&s.FECShardSet, 0)
	atomic.StoreUint64(&s.FECShardMin, 0)
	atomic.StoreUint64(&s.RingBufferSndQueue, 0)
	atomic.StoreUint64(&s.RingBufferRcvQueue, 0)
	atomic.StoreUint64(&s.RingBufferSndBuffer, 0)
	atomic.StoreUint64(&s.OOBPackets, 0)
}

// DefaultSnmp is the global KCP connection statistics collector
var DefaultSnmp *Snmp = newSnmp()
