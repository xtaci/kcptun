<img src="assets/kcp-go.png" alt="kcp-go" height="100px" />


[![GoDoc][1]][2] [![Created At][3]][4] [![MIT licensed][11]][12] [![Coverage Status][7]][8]

[1]: https://godoc.org/github.com/xtaci/kcp-go?status.svg
[2]: https://pkg.go.dev/github.com/xtaci/kcp-go/v5
[3]: https://img.shields.io/github/created-at/xtaci/kcp-go
[4]: https://github.com/xtaci/kcp-go
[7]: https://codecov.io/gh/xtaci/kcp-go/branch/master/graph/badge.svg
[8]: https://codecov.io/gh/xtaci/kcp-go
[11]: https://img.shields.io/badge/license-MIT-blue.svg
[12]: LICENSE


## Table of Contents

- [Introduction](#introduction)
- [Quick Start](#quick-start)
- [Features](#features)
- [Documentation](#documentation)
- [Layer-Model of KCP-GO](#layer-model-of-kcp-go)
- [Design and Tuning Notes](#design-and-tuning-notes)
  - [1. Slice vs. Container/List](#1-slice-vs-containerlist)
  - [2. Timing Accuracy and RTT Estimation](#2-timing-accuracy-and-rtt-estimation)
  - [3. Memory Management](#3-memory-management)
  - [4. Per-session Outbound Rate Limiting](#4-per-session-outbound-rate-limiting)
  - [5. Information Security](#5-information-security)
  - [6. Packet Clocking](#6-packet-clocking)
  - [7. FEC Design Characteristics](#7-fec-design-characteristics)
- [Specification](#specification)
- [Performance](#performance)
- [Typical Flame Graph](#typical-flame-graph)
- [Connection Termination and Multiplexing](#connection-termination-and-multiplexing)
- [FAQ](#faq)
- [Who is using this?](#who-is-using-this)
- [Examples](#examples)
- [Links](#links)

## Introduction

**kcp-go** is a **Reliable-UDP** library for [golang](https://golang.org/).

This library provides a reliable, ordered byte-stream transport over **UDP**. Optional FEC and packet encryption are available. Encryption can hide packet contents, but random per-packet nonces do not make network traffic anonymous: endpoints, packet sizes, timing, and traffic volume may still be observable. It has been deployed across a range of devices and applications, including **online games, live broadcasting, file synchronization, and network acceleration**.

[Latest Release](https://github.com/xtaci/kcp-go/releases)

## Quick Start

`kcp-go` implements `net.Conn` and `net.Listener`, so applications can use familiar stream I/O. These standalone examples show a basic echo server and client; they use no packet encryption or FEC.

Echo server:

```go
package main

import (
	"io"
	"log"
	"net"

	"github.com/xtaci/kcp-go/v5"
)

func main() {
	ln, err := kcp.Listen("127.0.0.1:12345")
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go func(conn net.Conn) {
			defer conn.Close()
			if _, err := io.Copy(conn, conn); err != nil {
				log.Println(err)
			}
		}(conn)
	}
}
```

Echo client:

```go
package main

import (
	"fmt"
	"io"
	"log"

	"github.com/xtaci/kcp-go/v5"
)

func main() {
	conn, err := kcp.Dial("127.0.0.1:12345")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	payload := []byte("hello over KCP")
	if _, err := conn.Write(payload); err != nil {
		log.Fatal(err)
	}
	reply := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, reply); err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(reply))
}
```

For optional packet encryption or FEC, use `ListenWithOptions` and `DialWithOptions`; both peers must use matching encryption keys and FEC shard settings. See [Godoc](https://pkg.go.dev/github.com/xtaci/kcp-go/v5) for configuration details.

## Features

- Reliable, ordered byte-stream delivery over UDP with configurable latency/throughput trade-offs.
- `net.Conn` and `net.Listener` APIs; the familiar interface does not make the transport TCP.
- Optional Reed-Solomon FEC to recover some packet loss, with configurable data/parity shard counts.
- Optional packet encryption, including AEAD for packet-level integrity and authentication.
- A shared timer scheduler and batched, platform-specific UDP I/O to support many sessions efficiently.
- Designed for high concurrency; capacity depends on workload, configuration, and hardware.
- Based on [skywind3000's KCP](https://github.com/skywind3000/kcp), with additional features and optimizations.

## Documentation

For complete documentation, see the associated [Godoc](https://pkg.go.dev/github.com/xtaci/kcp-go/v5).


### Layer-Model of KCP-GO

<img src="assets/layermodel.jpg" alt="layer-model" />

## Design and Tuning Notes

### 1. Slice vs. Container/List

The KCP flush path scans the in-flight send buffer to find segments that need transmission or retransmission. The ring-buffer representation supports sequential traversal with good locality; the scan cost depends on the number of in-flight segments and how often the session is updated.

I wrote a benchmark comparing sequential loops through a *slice* and a *container/list* [here](https://gist.github.com/xtaci/ac2f13f0108494d874b25551134e4c9c):

```
BenchmarkLoopSlice-4   	2000000000	         0.39 ns/op
BenchmarkLoopList-4    	100000000	        54.6 ns/op
```

The list structure introduces **heavy cache misses** compared to the slice, which offers better **locality**. In the linked benchmark, for 5,000 connections with a 32-window size and a 20 ms update interval, using a slice costs 6 μs (0.03% CPU) per `kcp.flush()`, whereas using a list costs 8.7 ms (43.5% CPU). These figures are benchmark-specific; actual cost depends on workload and hardware.

### 2. Timing Accuracy and RTT Estimation

Timing is **critical** for the **RTT estimator**. Inaccurate timing can cause premature or delayed retransmissions. KCP uses `currentMs()`, which derives elapsed monotonic time from a package reference point. The `time.Now()` numbers below are measurements from a historical benchmark on the listed hardware, not portable performance guarantees.

The benchmark for `time.Now()` is [here](https://gist.github.com/xtaci/f01503b9167f9b520b8896682b67e14d):

```
BenchmarkNow-4         	100000000	        15.6 ns/op
```

The number of clock reads varies with the flush path: KCP samples time while checking retransmission deadlines and when preparing transmissions. The `time.Now()` benchmark above is historical and should not be treated as a measurement of current `currentMs()` cost or as a per-flush cost estimate.

### 3. Memory Management

Many packet buffers are obtained from the package-wide `defaultBufferPool`, which provides buffers with a fixed capacity of 1500 bytes (`mtuLimit`). Buffers are returned to the pool when they are no longer needed, reducing repeated allocations. The pool uses `sync.Pool`, so the Go runtime may discard pooled buffers during garbage collection; reuse and retention are not guaranteed.

### 4. Per-session Application Write Rate Limiting

`UDPSession.SetRateLimit(bytesPerSecond)` configures an independent token-bucket rate limit for application data submitted through `Write` and `WriteBuffers`. The limit is measured in application bytes per second. It does not count KCP/FEC/encryption overhead, FEC parity, retransmissions, `SendOOB` traffic, or UDP/IP headers. Rate limiting is disabled by default, and passing `0` disables it. A throttled write waits for tokens while remaining interruptible by its write deadline, socket write errors, and session closure.

The bucket allows an initial burst of up to `maxBatchSize * maxPacketSize` bytes (currently 64 × 1472 = 94,208 bytes), then limits the average rate at which writes are accepted. Writes larger than the burst capacity are admitted in rate-limited chunks. This controls application write throughput rather than the exact rate of UDP packets on the wire.

```go
sess.SetRateLimit(256 * 1024) // limit this session to 256 KiB/s
sess.SetRateLimit(0)           // disable rate limiting
```

### 5. Information Security

Applications commonly use TLS or a similar protocol to protect application data. That protection does not extend to the KCP and FEC headers added below the application layer. kcp-go's packet protection is a separate layer: it can encrypt and, when using AEAD, authenticate each UDP payload, including those transport headers.

Packet integrity depends on the selected mode. When a non-AEAD `BlockCrypt` is configured, kcp-go adds a CRC-32 checksum; if an encryption cipher is enabled, the checksum is carried within the encrypted packet. After CFB decryption, the receiver verifies this checksum and drops packets corrupted by accidental bit errors. This is only an anti-corruption check, not cryptographic anti-tamper protection: CRC-32 is unkeyed, and its linearity combined with the malleability of modes such as CFB means an active attacker may be able to modify packets without detection. Do not rely on non-AEAD modes for cryptographic integrity or authentication. AEAD modes authenticate each packet's encrypted contents and are recommended when protection against deliberate packet modification is required. AEAD packet authentication proves possession of the configured key; it does not by itself establish a peer's identity, replace secure key management, or guarantee application-level replay protection.

Packet encryption protects packet contents, including FEC and KCP headers, but does not hide traffic metadata such as endpoints, packet sizes, timing, or traffic volume. A random nonce for each packet helps prevent repeated encryption inputs from producing repeated ciphertext, but it does not conceal this metadata or provide anonymity. Without packet encryption, packet contents are transmitted in plaintext. Algorithm performance varies by hardware and workload, so benchmark the available options in your deployment rather than assuming one cipher is always faster.

Other possible attacks on kcp-go include:

- **[Traffic analysis](https://en.wikipedia.org/wiki/Traffic_analysis):** Packet sizes, timing, and endpoints can reveal communication patterns even when packet contents are encrypted. Multiplexing streams with [smux](https://github.com/xtaci/smux) does not eliminate traffic analysis.
- **[Replay attack](https://en.wikipedia.org/wiki/Replay_attack):** Packet encryption does not by itself establish peer identity or guarantee application messages are processed only once. Applications that need these properties should use an authenticated protocol and implement appropriate replay protection.

### 6. Packet Clocking

1. **Immediate flush on window progress or fast retransmit**: Flush pending data when acknowledgments advance the send window or trigger fast retransmission, without waiting for the next periodic update.
2. **ACK clocking**: Flush ACKs when the pending ACK list reaches approximately one MTU's worth of ACK segments, rather than waiting for the periodic update.
   In high-speed networks, this acts as a higher-frequency "clock signal," potentially boosting unidirectional transmission speed by approximately 6x. For instance, if a batch takes only 1.5ms to process on a high-speed link but still adheres to a fixed 10ms transmission cycle, the actual throughput would be limited to 1/6 of the potential.
3. **Packet batching**: The session packet-processing path batches outgoing packets and uses platform-specific batched I/O where available. This can reduce syscall overhead; it is not a packet pacing mechanism.
4. **Data structures**: The `snd_buf` ring buffer supports cache-friendly sequential traversal. Retransmission checks are O(n) in the number of in-flight segments, so very large windows can increase per-update work.

Ultimately, nothing is more critical in a transmission system than the clock (real-time performance).

### 7. FEC Design Characteristics

- Reed-Solomon encoding and decoding are performed in the outgoing and incoming packet-processing paths.
- Data/parity ratios are configurable per session. More parity can recover more packet loss, at the cost of additional bandwidth; the overhead depends on the configured ratio.
- Encoder and decoder buffers are reused where possible to reduce allocations; actual memory use depends on traffic and configuration.
- When enough shards arrive, the decoder attempts to reconstruct missing data packets and passes recovered packets to KCP for normal processing.
- When packet encryption is enabled, FEC headers are included in the encrypted packet. AEAD authenticates packet contents; non-AEAD modes with CRC-32 do not provide cryptographic authentication.

## Specification

<img src="assets/frame.png" alt="Frame Format" height="109px" />

```
NONCE:
  Per-packet nonce; its size depends on the encryption mode (16 bytes for
  the legacy mode, or the configured AEAD nonce size).
  
CRC32:
  IEEE CRC-32 checksum used by non-AEAD modes; AEAD uses an authentication tag.
 
FEC TYPE:
  typeData = 0xF1
  typeParity = 0xF2
  
FEC SEQID:
  Increments modulo (0xffffffff/shardSize) * shardSize; valid IDs are in
  [0, (0xffffffff/shardSize) * shardSize - 1].
  
SIZE:
  Number of bytes from the 2-byte SIZE field through the end of the packet:
  the KCP frame plus 2 bytes. The payload after the SIZE field is SIZE-2 bytes.

KCP Header
+------------------------------+
|           conv (u32)         |
+-------+-------+--------------+
|  cmd  |  frag |     wnd      |
|  u8   |  u8   |     u16      |
+------------------------------+
|           ts   (u32)         |
+------------------------------+
|           sn   (u32)         |
+------------------------------+
|           una  (u32)         |
+------------------------------+
|           data (bytes)       |
+------------------------------+
```

## Performance

The results below are historical and hardware-specific, not performance guarantees. Run `go test -run='^$' -bench .` on your target system for comparable measurements. Expand a section to see the raw output.

<details>
<summary>Linux amd64 — AMD Ryzen 9 5950X</summary>

```
2025/11/26 11:12:51 beginning tests, encryption:salsa20, fec:10/3
goos: linux
goarch: amd64
pkg: github.com/xtaci/kcp-go/v5
cpu: AMD Ryzen 9 5950X 16-Core Processor
BenchmarkSM4
BenchmarkSM4-32                            56077             21672 ns/op         138.43 MB/s           0 B/op          0 allocs/op
BenchmarkAES128
BenchmarkAES128-32                        525854              2228 ns/op        1346.69 MB/s           0 B/op          0 allocs/op
BenchmarkAES192
BenchmarkAES192-32                        473692              2429 ns/op        1234.95 MB/s           0 B/op          0 allocs/op
BenchmarkAES256
BenchmarkAES256-32                        427497              2725 ns/op        1101.06 MB/s           0 B/op          0 allocs/op
BenchmarkTEA
BenchmarkTEA-32                           149976              8085 ns/op         371.06 MB/s           0 B/op          0 allocs/op
BenchmarkXOR
BenchmarkXOR-32                         12333190                92.35 ns/op     32485.16 MB/s          0 B/op          0 allocs/op
BenchmarkBlowfish
BenchmarkBlowfish-32                       70762             16983 ns/op         176.65 MB/s           0 B/op          0 allocs/op
BenchmarkNone
BenchmarkNone-32                        47325206                24.49 ns/op     122482.39 MB/s         0 B/op          0 allocs/op
BenchmarkCast5
BenchmarkCast5-32                          66837             18035 ns/op         166.35 MB/s           0 B/op          0 allocs/op
Benchmark3DES
Benchmark3DES-32                           18402             64349 ns/op          46.62 MB/s           0 B/op          0 allocs/op
BenchmarkTwofish
BenchmarkTwofish-32                        56440             21380 ns/op         140.32 MB/s           0 B/op          0 allocs/op
BenchmarkXTEA
BenchmarkXTEA-32                           45616             26124 ns/op         114.84 MB/s           0 B/op          0 allocs/op
BenchmarkSalsa20
BenchmarkSalsa20-32                       525685              2199 ns/op        1363.97 MB/s           0 B/op          0 allocs/op
BenchmarkCRC32
BenchmarkCRC32-32                       19418395                59.05 ns/op     17341.83 MB/s
BenchmarkCsprngSystem
BenchmarkCsprngSystem-32                 2912889               404.3 ns/op        39.58 MB/s
BenchmarkCsprngMD5
BenchmarkCsprngMD5-32                   15063580                79.23 ns/op      201.95 MB/s
BenchmarkCsprngSHA1
BenchmarkCsprngSHA1-32                  20186407                60.04 ns/op      333.08 MB/s
BenchmarkCsprngNonceMD5
BenchmarkCsprngNonceMD5-32              13863704                85.11 ns/op      187.98 MB/s
BenchmarkCsprngNonceAES128
BenchmarkCsprngNonceAES128-32           97239751                12.56 ns/op     1274.09 MB/s
BenchmarkFECDecode
BenchmarkFECDecode-32                    1808791               679.1 ns/op      2208.94 MB/s        1641 B/op          3 allocs/op
BenchmarkFECEncode
BenchmarkFECEncode-32                    6671982               181.4 ns/op      8270.76 MB/s           2 B/op          0 allocs/op
BenchmarkFlush
BenchmarkFlush-32                         322982              3809 ns/op               0 B/op          0 allocs/op
BenchmarkDebugLog
BenchmarkDebugLog-32                    1000000000               0.2146 ns/op
BenchmarkEchoSpeed4K
BenchmarkEchoSpeed4K-32                    35583             32875 ns/op         124.59 MB/s       18223 B/op        148 allocs/op
BenchmarkEchoSpeed64K
BenchmarkEchoSpeed64K-32                    1995            510301 ns/op         128.43 MB/s      284233 B/op       2297 allocs/op
BenchmarkEchoSpeed512K
BenchmarkEchoSpeed512K-32                    259           4058131 ns/op         129.19 MB/s     2243058 B/op      18148 allocs/op
BenchmarkEchoSpeed1M
BenchmarkEchoSpeed1M-32                      145           8561996 ns/op         122.47 MB/s     4464227 B/op      36009 allocs/op
BenchmarkSinkSpeed4K
BenchmarkSinkSpeed4K-32                   194648             42136 ns/op          97.21 MB/s        2073 B/op         50 allocs/op
BenchmarkSinkSpeed64K
BenchmarkSinkSpeed64K-32                   10000            113038 ns/op         579.77 MB/s       29242 B/op        741 allocs/op
BenchmarkSinkSpeed256K
BenchmarkSinkSpeed256K-32                   1555            843724 ns/op         621.40 MB/s      229558 B/op       5850 allocs/op
BenchmarkSinkSpeed1M
BenchmarkSinkSpeed1M-32                      667           1783214 ns/op         588.03 MB/s      462691 B/op      11694 allocs/op
PASS
ok      github.com/xtaci/kcp-go/v5      49.978s
```
</details>

<details>
<summary>macOS arm64 — Apple M4 Pro</summary>

```
macOS 26.6.2
Apple M4 Pro
12 CPUs, 24 GiB memory
Go 1.27.3

$ go test -run='^$' -bench . -benchmem -count=1
goos: darwin
goarch: arm64
pkg: github.com/xtaci/kcp-go/v5
cpu: Apple M4 Pro
BenchmarkSM4-12                       	   51745	     21525 ns/op	 136.77 MB/s	       0 B/op	       0 allocs/op
BenchmarkAES128-12                    	  766851	      1597 ns/op	1843.20 MB/s	       0 B/op	       0 allocs/op
BenchmarkAES192-12                    	  711027	      1714 ns/op	1717.21 MB/s	       0 B/op	       0 allocs/op
BenchmarkAES256-12                    	  660265	      1835 ns/op	1604.65 MB/s	       0 B/op	       0 allocs/op
BenchmarkTEA-12                       	  160390	      7446 ns/op	 395.40 MB/s	       0 B/op	       0 allocs/op
BenchmarkXOR-12                       	27940386	        43.37 ns/op	67884.26 MB/s	       0 B/op	       0 allocs/op
BenchmarkBlowfish-12                  	   82837	     14560 ns/op	 202.20 MB/s	       0 B/op	       0 allocs/op
BenchmarkNone-12                      	47629758	        26.09 ns/op	112842.22 MB/s	       0 B/op	       0 allocs/op
BenchmarkCast5-12                     	   82333	     14530 ns/op	 202.62 MB/s	       0 B/op	       0 allocs/op
Benchmark3DES-12                      	   20512	     57895 ns/op	  50.85 MB/s	       0 B/op	       0 allocs/op
BenchmarkTwofish-12                   	   62769	     19188 ns/op	 153.43 MB/s	       0 B/op	       0 allocs/op
BenchmarkXTEA-12                      	   47636	     25590 ns/op	 115.04 MB/s	       0 B/op	       0 allocs/op
BenchmarkSalsa20-12                   	  303675	      3944 ns/op	 746.49 MB/s	       0 B/op	       0 allocs/op
BenchmarkCRC32-12                     	14890838	        80.91 ns/op	12656.36 MB/s
BenchmarkCFB_AES_128_CRC32-12         	 1213918	       988.2 ns/op	1416.79 MB/s	       0 B/op	       0 allocs/op
BenchmarkAEAD_AES_128_GCM-12          	 6249505	       194.4 ns/op	7203.41 MB/s	       0 B/op	       0 allocs/op
BenchmarkCFB_Salsa20_CRC32-12         	  607309	      1970 ns/op	 710.74 MB/s	       0 B/op	       0 allocs/op
BenchmarkAEAD_Chacha20_Poly1035-12    	 1000000	      1124 ns/op	1245.71 MB/s	       0 B/op	       0 allocs/op
BenchmarkCsprngSystem-12              	 6860886	       175.9 ns/op	  90.98 MB/s	       0 B/op	       0 allocs/op
BenchmarkCsprngAES128-12              	100000000	        10.75 ns/op	1488.95 MB/s	       0 B/op	       0 allocs/op
BenchmarkCsprngChacha8-12             	133251962	         9.005 ns/op	 888.41 MB/s	       0 B/op	       0 allocs/op
BenchmarkFECDecode-12                 	 5951834	       198.6 ns/op	7412.60 MB/s	     290 B/op	       2 allocs/op
BenchmarkFECEncode-12                 	 6632745	       178.9 ns/op	8228.76 MB/s	       2 B/op	       0 allocs/op
BenchmarkFlush-12                     	  572144	      1966 ns/op	       0 B/op	       0 allocs/op
BenchmarkDebugLog-12                  	752248714	         1.594 ns/op	       0 B/op	       0 allocs/op
BenchmarkEchoSpeed4K-12               	   15751	     73867 ns/op	  55.45 MB/s	    6054 B/op	     132 allocs/op
BenchmarkEchoSpeed64K-12              	    1231	    990333 ns/op	  66.18 MB/s	   82028 B/op	    1788 allocs/op
BenchmarkEchoSpeed512K-12             	     153	   7894735 ns/op	  66.41 MB/s	  663806 B/op	   14314 allocs/op
BenchmarkEchoSpeed1M-12               	     100	  15527693 ns/op	  67.53 MB/s	 1388181 B/op	   28534 allocs/op
BenchmarkSinkSpeed4K-12               	   73106	     17769 ns/op	 230.51 MB/s	    1537 B/op	      34 allocs/op
BenchmarkSinkSpeed64K-12              	    4190	    279547 ns/op	 234.44 MB/s	   24571 B/op	     552 allocs/op
BenchmarkSinkSpeed256K-12             	     583	   2224163 ns/op	 235.72 MB/s	  193812 B/op	    4387 allocs/op
BenchmarkSinkSpeed1M-12               	     235	   4496335 ns/op	 233.21 MB/s	  409976 B/op	    8730 allocs/op
BenchmarkTimedFuncHeap-12             	22539919	        53.40 ns/op	       0 B/op	       0 allocs/op
PASS
ok  	github.com/xtaci/kcp-go/v5	47.161s
```
</details>

<details>
<summary>Linux arm — Raspberry Pi 4</summary>

```
=== Raspberry Pi 4 ===

➜  kcp-go git:(master) cat /proc/cpuinfo
processor	: 0
model name	: ARMv7 Processor rev 3 (v7l)
BogoMIPS	: 108.00
Features	: half thumb fastmult vfp edsp neon vfpv3 tls vfpv4 idiva idivt vfpd32 lpae evtstrm crc32
CPU implementer	: 0x41
CPU architecture: 7
CPU variant	: 0x0
CPU part	: 0xd08
CPU revision	: 3

➜  kcp-go git:(master)  go test -run=^$ -bench .
2020/01/05 19:25:13 beginning tests, encryption:salsa20, fec:10/3
goos: linux
goarch: arm
pkg: github.com/xtaci/kcp-go/v5
BenchmarkSM4-4                     20000             86475 ns/op          34.69 MB/s           0 B/op          0 allocs/op
BenchmarkAES128-4                  20000             62254 ns/op          48.19 MB/s           0 B/op          0 allocs/op
BenchmarkAES192-4                  20000             71802 ns/op          41.78 MB/s           0 B/op          0 allocs/op
BenchmarkAES256-4                  20000             80570 ns/op          37.23 MB/s           0 B/op          0 allocs/op
BenchmarkTEA-4                     50000             37343 ns/op          80.34 MB/s           0 B/op          0 allocs/op
BenchmarkXOR-4                    100000             22266 ns/op         134.73 MB/s           0 B/op          0 allocs/op
BenchmarkBlowfish-4                20000             66123 ns/op          45.37 MB/s           0 B/op          0 allocs/op
BenchmarkNone-4                  3000000               518 ns/op        5786.77 MB/s           0 B/op          0 allocs/op
BenchmarkCast5-4                   20000             76705 ns/op          39.11 MB/s           0 B/op          0 allocs/op
Benchmark3DES-4                     5000            418868 ns/op           7.16 MB/s           0 B/op          0 allocs/op
BenchmarkTwofish-4                  5000            326896 ns/op           9.18 MB/s           0 B/op          0 allocs/op
BenchmarkXTEA-4                    10000            114418 ns/op          26.22 MB/s           0 B/op          0 allocs/op
BenchmarkSalsa20-4                 50000             36736 ns/op          81.66 MB/s           0 B/op          0 allocs/op
BenchmarkCRC32-4                 1000000              1735 ns/op         589.98 MB/s
BenchmarkCsprngSystem-4          1000000              2179 ns/op           7.34 MB/s
BenchmarkCsprngMD5-4             2000000               811 ns/op          19.71 MB/s
BenchmarkCsprngSHA1-4            2000000               862 ns/op          23.19 MB/s
BenchmarkCsprngNonceMD5-4        2000000               878 ns/op          18.22 MB/s
BenchmarkCsprngNonceAES128-4     5000000               326 ns/op          48.97 MB/s
BenchmarkFECDecode-4              200000              9081 ns/op         165.16 MB/s         140 B/op          1 allocs/op
BenchmarkFECEncode-4              100000             12039 ns/op         124.59 MB/s          11 B/op          0 allocs/op
BenchmarkFlush-4                  100000             21704 ns/op               0 B/op          0 allocs/op
BenchmarkEchoSpeed4K-4              2000            981182 ns/op           4.17 MB/s       12384 B/op        424 allocs/op
BenchmarkEchoSpeed64K-4              100          10503324 ns/op           6.24 MB/s      123616 B/op       3779 allocs/op
BenchmarkEchoSpeed512K-4              20         138633802 ns/op           3.78 MB/s     1606584 B/op      29233 allocs/op
BenchmarkEchoSpeed1M-4                 5         372903568 ns/op           2.81 MB/s     4080504 B/op      63600 allocs/op
BenchmarkSinkSpeed4K-4             10000            121239 ns/op          33.78 MB/s        4647 B/op        104 allocs/op
BenchmarkSinkSpeed64K-4             1000           1587906 ns/op          41.27 MB/s       50914 B/op       1115 allocs/op
BenchmarkSinkSpeed256K-4             100          16277830 ns/op          32.21 MB/s      453027 B/op       9296 allocs/op
BenchmarkSinkSpeed1M-4               100          31040703 ns/op          33.78 MB/s      898097 B/op      18932 allocs/op
PASS
ok      github.com/xtaci/kcp-go/v5      64.151s
```
</details>


## Typical Flame Graph
![Typical flame graph](assets/flame.png)



## Connection Termination and Multiplexing

KCP does not define TCP-style connection setup and teardown messages such as **SYN/FIN/RST**. Use an application-level keepalive/heartbeat when needed. [smux](https://github.com/xtaci/smux) can multiplex logical streams over a KCP session and provides its own keepalive mechanism.

Multiplexing also affects congestion behavior. KCP's congestion control is maintained per session, so logical streams carried by one smux connection share that session's congestion window and loss response. Opening a separate KCP session for every logical stream creates independent congestion controllers; over the same bottleneck, these sessions may compete with one another and collectively use more bandwidth. smux itself is not a network congestion controller: it multiplexes streams over the KCP transport.

The trade-off is that KCP delivers an ordered byte stream. Packet loss can therefore delay delivery to all smux streams sharing that session, even if some streams' data arrived successfully. Multiplex streams over one session when they should share transport and congestion state; use separate sessions when isolation is more important, while accounting for their independent congestion behavior.

## FAQ

**Q: I'm handling thousands of connections on my server, and CPU utilization is high.**

**A:** Profile the workload and consider separating KCP traffic into a dedicated service. If the session is currently using a short update interval (for example, 10 ms), increasing it (for example, to 40 ms) may reduce scheduling and update overhead, at the cost of response latency. The default KCP interval is 100 ms, so 40 ms is not an increase over the default. `SetNoDelay` takes `(nodelay, intervalMs, fastResend, disableCongestionControl)`; for example, `conn.SetNoDelay(1, 40, 1, 0)` keeps congestion control enabled. Disabling congestion control can increase congestion on shared networks and should only be done when appropriate for the deployment.

**Q: When should I enable FEC?**

**A:** FEC can recover some packet losses without waiting for retransmission, which may reduce latency on lossy links. It adds bandwidth and processing overhead and cannot recover losses beyond the configured parity capacity. Benchmark with representative loss patterns and tune the data/parity ratio for your workload.

**Q: Should I enable encryption?**

**A:** If you need protection for KCP/FEC headers in addition to application data, configure packet encryption. Prefer AEAD when UDP-packet integrity and authentication are required. If the application already uses TLS, packet encryption is not a substitute for TLS; whether to enable the additional layer depends on your threat model and overhead requirements.

## Who is using this?

1. https://github.com/getlantern/lantern -- Lantern delivers fast access to the open Internet.
2. https://github.com/smallnest/rpcx -- An RPC service framework based on net/rpc, similar to Alibaba Dubbo and Weibo Motan.
3. https://github.com/gonet2/agent -- A gateway for games with stream multiplexing.
4. https://github.com/syncthing/syncthing -- Open Source Continuous File Synchronization.
5. https://github.com/hanselime/paqet -- A bidirectional packet-level proxy built using raw sockets and KCP.

### Looking for a C++ client?
1. https://github.com/xtaci/libkcp -- FEC enhanced KCP session library for iOS/Android in C++

## Examples

1. [simple examples](https://github.com/xtaci/kcp-go/tree/master/examples)

## Links

1. https://github.com/xtaci/smux/ -- A Stream Multiplexing Library for golang with least memory
1. **https://github.com/xtaci/libkcp -- FEC enhanced KCP session library for iOS/Android in C++**
1. https://github.com/skywind3000/kcp -- A Fast and Reliable ARQ Protocol
1. https://github.com/klauspost/reedsolomon -- Reed-Solomon Erasure Coding in Go
