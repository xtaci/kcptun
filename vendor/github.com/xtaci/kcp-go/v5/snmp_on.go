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

import "sync/atomic"

func (snmp *Snmp) addBytesSent(delta int) {
	atomic.AddUint64(&snmp.BytesSent, uint64(delta))
}

func (snmp *Snmp) addBytesReceived(delta int) {
	atomic.AddUint64(&snmp.BytesReceived, uint64(delta))
}

func (snmp *Snmp) increaseInPkts() {
	atomic.AddUint64(&snmp.InPkts, 1)
}

func (snmp *Snmp) addOutPkts(delta int) {
	atomic.AddUint64(&snmp.OutPkts, uint64(delta))
}

func (snmp *Snmp) increaseActiveOpens() {
	atomic.AddUint64(&snmp.ActiveOpens, 1)
}

func (snmp *Snmp) increasePassiveOpens() {
	atomic.AddUint64(&snmp.PassiveOpens, 1)
}

func (snmp *Snmp) increaseCurrEstab() {
	currestab := atomic.AddUint64(&DefaultSnmp.CurrEstab, 1)
	maxconn := atomic.LoadUint64(&DefaultSnmp.MaxConn)
	if currestab > maxconn {
		atomic.CompareAndSwapUint64(&DefaultSnmp.MaxConn, maxconn, currestab)
	}
}

func (snmp *Snmp) decreaseCurrEstab() {
	atomic.AddUint64(&snmp.CurrEstab, ^uint64(0))
}

func (snmp *Snmp) increaseInErrs() {
	atomic.AddUint64(&snmp.InErrs, 1)
}

func (snmp *Snmp) increaseInCsumErrors() {
	atomic.AddUint64(&snmp.InCsumErrors, 1)
}

func (snmp *Snmp) addKCPInErrors(delta uint64) {
	atomic.AddUint64(&snmp.KCPInErrors, delta)
}

func (snmp *Snmp) addInSegs(delta uint64) {
	atomic.AddUint64(&snmp.InSegs, delta)
}

func (snmp *Snmp) increaseOutSegs() {
	atomic.AddUint64(&snmp.OutSegs, 1)
}

func (snmp *Snmp) addInBytes(delta int) {
	atomic.AddUint64(&snmp.InBytes, uint64(delta))
}

func (snmp *Snmp) addOutBytes(delta int) {
	atomic.AddUint64(&snmp.OutBytes, uint64(delta))
}

func (snmp *Snmp) addRetransSegs(delta uint64) {
	atomic.AddUint64(&snmp.RetransSegs, delta)
}

func (snmp *Snmp) addFastRetransSegs(delta uint64) {
	atomic.AddUint64(&snmp.FastRetransSegs, delta)
}

func (snmp *Snmp) addEarlyRetransSegs(delta uint64) {
	atomic.AddUint64(&snmp.EarlyRetransSegs, delta)
}

func (snmp *Snmp) addLostSegs(delta uint64) {
	atomic.AddUint64(&snmp.LostSegs, delta)
}

func (snmp *Snmp) increaseRepeatSegs() {
	atomic.AddUint64(&snmp.RepeatSegs, 1)
}

func (snmp *Snmp) increaseFECFullShardSet() {
	atomic.AddUint64(&snmp.FECFullShardSet, 1)
}

func (snmp *Snmp) addFECRecovered(delta int) {
	atomic.AddUint64(&snmp.FECRecovered, uint64(delta))
}

func (snmp *Snmp) increaseFECErrs() {
	atomic.AddUint64(&snmp.FECErrs, 1)
}

func (snmp *Snmp) increaseFECParityShards() {
	atomic.AddUint64(&snmp.FECParityShards, 1)
}

func (snmp *Snmp) increaseFECShardSet() {
	atomic.AddUint64(&snmp.FECShardSet, 1)
}

func (snmp *Snmp) storeFECShardSet(val int) {
	atomic.StoreUint64(&snmp.FECShardSet, uint64(val))
}

func (snmp *Snmp) storeFECShardMin(v uint32) {
	atomic.StoreUint64(&snmp.FECShardMin, uint64(v))
}

func (snmp *Snmp) storeRingBufferSndQueue(val int) {
	atomic.StoreUint64(&snmp.RingBufferSndQueue, uint64(val))
}

func (snmp *Snmp) storeRingBufferRcvQueue(val int) {
	atomic.StoreUint64(&snmp.RingBufferRcvQueue, uint64(val))
}

func (snmp *Snmp) storeRingBufferSndBuffer(val int) {
	atomic.StoreUint64(&snmp.RingBufferSndBuffer, uint64(val))
}

func (snmp *Snmp) increaseOOBPackets() {
	atomic.AddUint64(&snmp.OOBPackets, 1)
}
