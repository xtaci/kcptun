<img src="assets/kcp-go.png" alt="kcp-go" height="50px" />


[![GoDoc][1]][2] [![Powered][9]][10] [![MIT licensed][11]][12] [![Build Status][3]][4] [![Go Report Card][5]][6] [![Coverage Statusd][7]][8]


[1]: https://godoc.org/github.com/xtaci/kcp-go?status.svg
[2]: https://pkg.go.dev/github.com/xtaci/kcp-go
[3]: https://img.shields.io/github/created-at/xtaci/kcp-go
[4]: https://img.shields.io/github/created-at/xtaci/kcp-go
[5]: https://goreportcard.com/badge/github.com/xtaci/kcp-go
[6]: https://goreportcard.com/report/github.com/xtaci/kcp-go
[7]: https://codecov.io/gh/xtaci/kcp-go/branch/master/graph/badge.svg
[8]: https://codecov.io/gh/xtaci/kcp-go
[9]: https://img.shields.io/badge/KCP-Powered-blue.svg
[10]: https://github.com/skywind3000/kcp
[11]: https://img.shields.io/badge/license-MIT-blue.svg
[12]: LICENSE
[13]: https://sourcegraph.com/github.com/xtaci/kcp-go/-/badge.svg
[14]: https://sourcegraph.com/github.com/xtaci/kcp-go?badge

## Introduction

**kcp-go** is a **Reliable-UDP** library for [golang](https://golang.org/). 

This library is designed to provide **smooth, resilient, ordered, error-checked and anonymous** delivery of streams over **UDP** packets. It has been battle-tested with the open-source project [kcptun](https://github.com/xtaci/kcptun). Millions of devices, ranging from low-end MIPS routers to high-end servers, have deployed kcp-go-powered programs in various applications, including **online games, live broadcasting, file synchronization, and network acceleration**.

[Lastest Release](https://github.com/xtaci/kcp-go/releases)

## Features

1. Designed for **latency-sensitive** scenarios.
2. **Cache-friendly** and **memory-optimized** design, offering extremely **high performance** core.
3. Handles **>5K concurrent connections** on a single commodity server.
4. Compatible with [net.Conn](https://golang.org/pkg/net/#Conn) and [net.Listener](https://golang.org/pkg/net/#Listener), serving as a drop-in replacement for [net.TCPConn](https://golang.org/pkg/net/#TCPConn).
5. [FEC (Forward Error Correction)](https://en.wikipedia.org/wiki/Forward_error_correction) support with [Reed-Solomon Codes](https://en.wikipedia.org/wiki/Reed%E2%80%93Solomon_error_correction).
6. Packet-level encryption support with [AES](https://en.wikipedia.org/wiki/Advanced_Encryption_Standard), [TEA](https://en.wikipedia.org/wiki/Tiny_Encryption_Algorithm), [3DES](https://en.wikipedia.org/wiki/Triple_DES), [Blowfish](https://en.wikipedia.org/wiki/Blowfish_(cipher)), [Cast5](https://en.wikipedia.org/wiki/CAST-128), [Salsa20](https://en.wikipedia.org/wiki/Salsa20), etc., in [CFB](https://en.wikipedia.org/wiki/Block_cipher_mode_of_operation#Cipher_Feedback_(CFB)) mode, generating completely anonymous packets.
7. Only **a fixed number of goroutines** are created for the entire server application, with costs in **context switching** between goroutines taken into consideration.
8. Compatible with [skywind3000's](https://github.com/skywind3000) C version with various improvements.
9. Platform-dependent optimizations: [sendmmsg](http://man7.org/linux/man-pages/man2/sendmmsg.2.html) and [recvmmsg](http://man7.org/linux/man-pages/man2/recvmmsg.2.html) exploited for Linux.

## Documentation

For complete documentation, see the associated [Godoc](https://godoc.org/github.com/xtaci/kcp-go).

## Specification

<img src="assets/frame.png" alt="Frame Format" height="109px" />

```
NONCE:
  16bytes cryptographically secure random number, nonce changes for every packet.
  
CRC32:
  CRC-32 checksum of data using the IEEE polynomial
 
FEC TYPE:
  typeData = 0xF1
  typeParity = 0xF2
  
FEC SEQID:
  monotonically increasing in range: [0, (0xffffffff/shardSize) * shardSize - 1]
  
SIZE:
  The size of KCP frame plus 2

KCP Header
+------------------+
| conv      uint32 |
+------------------+
| cmd       uint8  |
+------------------+
| frg       uint8  |
+------------------+
| wnd      uint16  |
+------------------+
| ts       uint32  |
+------------------+
| sn       uint32  |
+------------------+
| una      uint32  |
+------------------+
| rto      uint32  |
+------------------+
| xmit     uint32  |
+------------------+
| resendts uint32  |
+------------------+
| fastack  uint32  |
+------------------+
| acked    uint32  |
+------------------+
| data     []byte  |
+------------------+
```

### Layer-Model of KCP-GO
```
+-----------------+
| SESSION         |
+-----------------+
| KCP(ARQ)        |
+-----------------+
| FEC(OPTIONAL)   |
+-----------------+
| CRYPTO(OPTIONAL)|
+-----------------+
| UDP(PACKET)     |
+-----------------+
| IP              |
+-----------------+
| LINK            |
+-----------------+
| PHY             |
+-----------------+
```

### Looing for a C++ client?
1. https://github.com/xtaci/libkcp -- FEC enhanced KCP session library for iOS/Android in C++

## Examples

1. [simple examples](https://github.com/xtaci/kcp-go/tree/master/examples)
2. [kcptun client](https://github.com/xtaci/kcptun/blob/master/client/main.go)
3. [kcptun server](https://github.com/xtaci/kcptun/blob/master/server/main.go)

## Benchmark
```
===
Model Name:	MacBook Pro
Model Identifier:	MacBookPro14,1
Processor Name:	Intel Core i5
Processor Speed:	3.1 GHz
Number of Processors:	1
Total Number of Cores:	2
L2 Cache (per Core):	256 KB
L3 Cache:	4 MB
Memory:	8 GB
===

$ go test -v -run=^$ -bench .
beginning tests, encryption:salsa20, fec:10/3
goos: darwin
goarch: amd64
pkg: github.com/xtaci/kcp-go
BenchmarkSM4-4                 	   50000	     32180 ns/op	  93.23 MB/s	       0 B/op	       0 allocs/op
BenchmarkAES128-4              	  500000	      3285 ns/op	 913.21 MB/s	       0 B/op	       0 allocs/op
BenchmarkAES192-4              	  300000	      3623 ns/op	 827.85 MB/s	       0 B/op	       0 allocs/op
BenchmarkAES256-4              	  300000	      3874 ns/op	 774.20 MB/s	       0 B/op	       0 allocs/op
BenchmarkTEA-4                 	  100000	     15384 ns/op	 195.00 MB/s	       0 B/op	       0 allocs/op
BenchmarkXOR-4                 	20000000	        89.9 ns/op	33372.00 MB/s	       0 B/op	       0 allocs/op
BenchmarkBlowfish-4            	   50000	     26927 ns/op	 111.41 MB/s	       0 B/op	       0 allocs/op
BenchmarkNone-4                	30000000	        45.7 ns/op	65597.94 MB/s	       0 B/op	       0 allocs/op
BenchmarkCast5-4               	   50000	     34258 ns/op	  87.57 MB/s	       0 B/op	       0 allocs/op
Benchmark3DES-4                	   10000	    117149 ns/op	  25.61 MB/s	       0 B/op	       0 allocs/op
BenchmarkTwofish-4             	   50000	     33538 ns/op	  89.45 MB/s	       0 B/op	       0 allocs/op
BenchmarkXTEA-4                	   30000	     45666 ns/op	  65.69 MB/s	       0 B/op	       0 allocs/op
BenchmarkSalsa20-4             	  500000	      3308 ns/op	 906.76 MB/s	       0 B/op	       0 allocs/op
BenchmarkCRC32-4               	20000000	        65.2 ns/op	15712.43 MB/s
BenchmarkCsprngSystem-4        	 1000000	      1150 ns/op	  13.91 MB/s
BenchmarkCsprngMD5-4           	10000000	       145 ns/op	 110.26 MB/s
BenchmarkCsprngSHA1-4          	10000000	       158 ns/op	 126.54 MB/s
BenchmarkCsprngNonceMD5-4      	10000000	       153 ns/op	 104.22 MB/s
BenchmarkCsprngNonceAES128-4   	100000000	        19.1 ns/op	 837.81 MB/s
BenchmarkFECDecode-4           	 1000000	      1119 ns/op	1339.61 MB/s	    1606 B/op	       2 allocs/op
BenchmarkFECEncode-4           	 2000000	       832 ns/op	1801.83 MB/s	      17 B/op	       0 allocs/op
BenchmarkFlush-4               	 5000000	       272 ns/op	       0 B/op	       0 allocs/op
BenchmarkEchoSpeed4K-4         	    5000	    259617 ns/op	  15.78 MB/s	    5451 B/op	     149 allocs/op
BenchmarkEchoSpeed64K-4        	    1000	   1706084 ns/op	  38.41 MB/s	   56002 B/op	    1604 allocs/op
BenchmarkEchoSpeed512K-4       	     100	  14345505 ns/op	  36.55 MB/s	  482597 B/op	   13045 allocs/op
BenchmarkEchoSpeed1M-4         	      30	  34859104 ns/op	  30.08 MB/s	 1143773 B/op	   27186 allocs/op
BenchmarkSinkSpeed4K-4         	   50000	     31369 ns/op	 130.57 MB/s	    1566 B/op	      30 allocs/op
BenchmarkSinkSpeed64K-4        	    5000	    329065 ns/op	 199.16 MB/s	   21529 B/op	     453 allocs/op
BenchmarkSinkSpeed256K-4       	     500	   2373354 ns/op	 220.91 MB/s	  166332 B/op	    3554 allocs/op
BenchmarkSinkSpeed1M-4         	     300	   5117927 ns/op	 204.88 MB/s	  310378 B/op	    6988 allocs/op
PASS
ok  	github.com/xtaci/kcp-go	50.349s
```

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


## Typical Flame Graph
![Flame Graph in kcptun](assets/flame.png)

## Key Design Considerations

### 1. Slice vs. Container/List

`kcp.flush()` loops through the send queue for retransmission checking every 20 ms.

I wrote a benchmark comparing sequential loops through a *slice* and a *container/list* [here](https://github.com/xtaci/notes/blob/master/golang/benchmark2/cachemiss_test.go):

```
BenchmarkLoopSlice-4   	2000000000	         0.39 ns/op
BenchmarkLoopList-4    	100000000	        54.6 ns/op
```

The list structure introduces **heavy cache misses** compared to the slice, which has better **locality**. For 5,000 connections with a 32-window size and a 20 ms interval, using a slice costs 6 μs (0.03% CPU) per `kcp.flush()`, while using a list costs 8.7 ms (43.5% CPU).

### 2. Timing Accuracy vs. Syscall clock_gettime

Timing is **critical** to the **RTT estimator**. Inaccurate timing leads to false retransmissions in KCP, but calling `time.Now()` costs 42 cycles (10.5 ns on a 4 GHz CPU, 15.6 ns on my MacBook Pro 2.7 GHz).

The benchmark for `time.Now()` is [here](https://github.com/xtaci/notes/blob/master/golang/benchmark2/syscall_test.go):

```
BenchmarkNow-4         	100000000	        15.6 ns/op
```

In kcp-go, after each `kcp.output()` function call, the current clock time is updated upon return. For a single `kcp.flush()` operation, the current time is queried from the system once. For 5,000 connections, this costs 5000 * 15.6 ns = 78 μs (a fixed cost when no packet needs to be sent). For 10 MB/s data transfer with a 1400 MTU, `kcp.output()` is called around 7500 times, costing 117 μs for `time.Now()` every second.

### 3. Memory Management

Primary memory allocation is done from a global buffer pool, `xmit.Buf`. In kcp-go, when we need to allocate some bytes, we get them from that pool, which returns a fixed-capacity 1500 bytes (mtuLimit). The rx queue, tx queue, and fec queue all receive bytes from this pool and return them after use to prevent unnecessary zeroing of bytes. The pool mechanism maintains a high watermark for slice objects, allowing these in-flight objects to survive periodic garbage collection, while also being able to return memory to the runtime when idle.

### 4. Information Security

kcp-go is shipped with built-in packet encryption powered by various block encryption algorithms and works in [Cipher Feedback Mode](https://en.wikipedia.org/wiki/Block_cipher_mode_of_operation#Cipher_Feedback_(CFB)). For each packet to be sent, the encryption process starts by encrypting a [nonce](https://en.wikipedia.org/wiki/Cryptographic_nonce) from the [system entropy](https://en.wikipedia.org/wiki//dev/random), ensuring that encryption of the same plaintext never results in the same ciphertext.

The contents of the packets are completely anonymous with encryption, including the headers (FEC, KCP), checksums, and contents. Note that no matter which encryption method you choose at the upper layer, if you disable encryption, the transmission will be insecure, as the header is ***plaintext*** and susceptible to tampering, such as jamming the *sliding window size*, *round-trip time*, *FEC properties*, and *checksums*. `AES-128` is suggested for minimal encryption, as modern CPUs come with [AES-NI](https://en.wikipedia.org/wiki/AES_instruction_set) instructions and perform better than `salsa20` (check the table above).

Other possible attacks on kcp-go include:

- **[Traffic analysis](https://en.wikipedia.org/wiki/Traffic_analysis):** Data flow on specific websites may have patterns while exchanging data. This type of eavesdropping has been mitigated by adopting [smux](https://github.com/xtaci/smux) to mix data streams and introduce noise. A perfect solution has not yet appeared, but theoretically, shuffling/mixing messages on a larger scale network may mitigate this problem.
- **[Replay attack](https://en.wikipedia.org/wiki/Replay_attack):** Since asymmetrical encryption has not been introduced into kcp-go, capturing packets and replaying them on a different machine is possible. Note that hijacking the session and decrypting the contents is still *impossible*. Upper layers should use an asymmetrical encryption system to guarantee the authenticity of each message (to process each message exactly once), such as HTTPS/OpenSSL/LibreSSL. Signing requests with private keys can eliminate this type of attack.

## Connection Termination

Control messages like **SYN/FIN/RST** in TCP **are not defined** in KCP. You need a **keepalive/heartbeat mechanism** at the application level. A real-world example is to use a **multiplexing** protocol over the session, such as [smux](https://github.com/xtaci/smux) (which has an embedded keepalive mechanism). See [kcptun](https://github.com/xtaci/kcptun) for an example.

## FAQ

**Q: I'm handling >5K connections on my server, and the CPU utilization is so high.**

**A:** A standalone `agent` or `gate` server for running kcp-go is suggested, not only to reduce CPU utilization but also to improve the **precision** of RTT measurements (timing), which indirectly affects retransmission. Increasing the update `interval` with `SetNoDelay`, such as `conn.SetNoDelay(1, 40, 1, 1)`, will dramatically reduce system load but may lower performance.

**Q: When should I enable FEC?**

**A:** Forward error correction is critical for long-distance transmission because packet loss incurs a huge time penalty. In the complex packet routing networks of the modern world, round-trip time-based loss checks are not always efficient. The significant deviation of RTT samples over long distances usually leads to a larger RTO value in typical RTT estimators, which slows down the transmission.

**Q: Should I enable encryption?**

**A:** Yes, for the security of the protocol, even if the upper layer has encryption.

## Who is using this?

1. https://github.com/xtaci/kcptun -- A Secure Tunnel Based On KCP over UDP.
2. https://github.com/getlantern/lantern -- Lantern delivers fast access to the open Internet. 
3. https://github.com/smallnest/rpcx -- A RPC service framework based on net/rpc like alibaba Dubbo and weibo Motan.
4. https://github.com/gonet2/agent -- A gateway for games with stream multiplexing.
5. https://github.com/syncthing/syncthing -- Open Source Continuous File Synchronization.

## Links

1. https://github.com/xtaci/smux/ -- A Stream Multiplexing Library for golang with least memory
1. **https://github.com/xtaci/libkcp -- FEC enhanced KCP session library for iOS/Android in C++**
1. https://github.com/skywind3000/kcp -- A Fast and Reliable ARQ Protocol
1. https://github.com/klauspost/reedsolomon -- Reed-Solomon Erasure Coding in Go
