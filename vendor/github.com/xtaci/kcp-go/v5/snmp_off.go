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

//go:build kcp_no_snmp

package kcp

type Snmp struct{}

func (*Snmp) addBytesSent(delta int)           {}
func (*Snmp) addBytesReceived(delta int)       {}
func (*Snmp) increaseInPkts()                  {}
func (*Snmp) addOutPkts(delta int)             {}
func (*Snmp) increaseActiveOpens()             {}
func (*Snmp) increasePassiveOpens()            {}
func (*Snmp) increaseCurrEstab()               {}
func (*Snmp) decreaseCurrEstab()               {}
func (*Snmp) increaseInErrs()                  {}
func (*Snmp) increaseInCsumErrors()            {}
func (*Snmp) addKCPInErrors(delta uint64)      {}
func (*Snmp) addInSegs(delta uint64)           {}
func (*Snmp) increaseOutSegs()                 {}
func (*Snmp) addInBytes(delta int)             {}
func (*Snmp) addOutBytes(delta int)            {}
func (*Snmp) addRetransSegs(delta uint64)      {}
func (*Snmp) addFastRetransSegs(delta uint64)  {}
func (*Snmp) addEarlyRetransSegs(delta uint64) {}
func (*Snmp) addLostSegs(delta uint64)         {}
func (*Snmp) increaseRepeatSegs()              {}
func (*Snmp) increaseFECFullShardSet()         {}
func (*Snmp) addFECRecovered(delta int)        {}
func (*Snmp) increaseFECErrs()                 {}
func (*Snmp) increaseFECParityShards()         {}
func (*Snmp) increaseFECShardSet()             {}
func (*Snmp) storeFECShardSet(val int)         {}
func (*Snmp) storeFECShardMin(v uint32)        {}
func (*Snmp) storeRingBufferSndQueue(val int)  {}
func (*Snmp) storeRingBufferRcvQueue(val int)  {}
func (*Snmp) storeRingBufferSndBuffer(val int) {}
func (*Snmp) increaseOOBPackets()              {}

var DefaultSnmp = &Snmp{}
