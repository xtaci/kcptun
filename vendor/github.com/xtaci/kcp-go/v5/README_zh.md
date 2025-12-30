<img src="assets/kcp-go.png" alt="kcp-go" height="100px" />


[![GoDoc][1]][2] [![Powered][9]][10] [![MIT licensed][11]][12] [![Build Status][3]][4] [![Go Report Card][5]][6] [![Coverage Status][7]][8] [![Sourcegraph][13]][14]

[1]: https://godoc.org/github.com/xtaci/kcp-go?status.svg
[2]: https://pkg.go.dev/github.com/xtaci/kcp-go/v5
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

[English](README.md) | [中文](README_zh.md)


## 目录

- [简介](#简介)
- [特性](#特性)
- [文档](#文档)
- [KCP-GO 分层模型](#kcp-go-分层模型)
- [关键设计考量](#关键设计考量)
  - [1. 切片 (Slice) vs. 容器/链表 (Container/List)](#1-切片-slice-vs-容器链表-containerlist)
  - [2. 计时精度 vs. 系统调用 clock_gettime](#2-计时精度-vs-系统调用-clock_gettime)
  - [3. 内存管理](#3-内存管理)
  - [4. 信息安全](#4-信息安全)
  - [5. 报文时钟](#5-报文时钟)
  - [6. FEC 设计特性](#6-fec-设计特性)
- [协议规范](#协议规范)
- [性能](#性能)
- [典型火焰图](#典型火焰图)
- [连接终止](#连接终止)
- [常见问题 (FAQ)](#常见问题-faq)
- [谁在使用](#谁在使用)
- [示例](#示例)
- [相关链接](#相关链接)

## 简介

**kcp-go** 是面向 [Go 语言](https://golang.org/) 的 **可靠 UDP (Reliable-UDP)** 库，专注在不可靠网络之上提供低时延、稳健的流式传输能力。

它在 **UDP** 之上构建出具备 **平滑性、弹性、有序性、错误检测与匿名性** 的数据通道。凭借开源项目 [kcptun](https://github.com/xtaci/kcptun) 的大规模部署验证，从低端 MIPS 路由器到高性能服务器，已有数以百万计的设备在 **在线游戏、直播、文件同步、网络加速** 等场景中运行 kcp-go。

[最新发布](https://github.com/xtaci/kcp-go/releases)

## 特性

1. 面向 **低时延诉求** 的场景深度优化。
2. 采用 **缓存友好**、**内存友好** 的核心设计，性能余量充足。
3. 单台商用服务器即可轻松支撑 **5,000+ 并发连接**。
4. 完全兼容 [net.Conn](https://golang.org/pkg/net/#Conn) 与 [net.Listener](https://golang.org/pkg/net/#Listener)，可直接替代 [net.TCPConn](https://golang.org/pkg/net/#TCPConn) 使用。
5. 内建基于 [Reed-Solomon Codes](https://en.wikipedia.org/wiki/Reed%E2%80%93Solomon_error_correction) 的 [FEC (前向纠错)](https://en.wikipedia.org/wiki/Forward_error_correction)。
6. 提供数据包级加密，包括 [AES](https://en.wikipedia.org/wiki/Advanced_Encryption_Standard)、[TEA](https://en.wikipedia.org/wiki/Tiny_Encryption_Algorithm)、[3DES](https://en.wikipedia.org/wiki/Triple_DES)、[Blowfish](https://en.wikipedia.org/wiki/Blowfish_(cipher))、[Cast5](https://en.wikipedia.org/wiki/CAST-128)、[Salsa20](https://en.wikipedia.org/wiki/Salsa20) 等，统一运行在 [CFB 模式](https://en.wikipedia.org/wiki/Block_cipher_mode_of_operation#Cipher_Feedback_(CFB))，保障报文匿名性。
7. 支持 [AEAD](https://en.wikipedia.org/wiki/Authenticated_encryption) 数据包加密方案。
8. 服务端仅需维持 **固定数量的 goroutine**，显著降低 **上下文切换** 开销。
9. 与 [skywind3000](https://github.com/skywind3000) C 版本协议兼容，并在此基础上扩展多项能力。
10. 针对平台特性提供优化：在 Linux 上使用 [sendmmsg](http://man7.org/linux/man-pages/man2/sendmmsg.2.html) 与 [recvmmsg](http://man7.org/linux/man-pages/man2/recvmmsg.2.html)。

## 文档

有关完整文档，请参阅关联的 [Godoc](https://pkg.go.dev/github.com/xtaci/kcp-go/v5)。


### KCP-GO 分层模型

<img src="assets/layermodel.jpg" alt="layer-model" />

## 关键设计考量

下面几条原则支撑了 kcp-go 的性能与可靠性取舍：

### 1. 切片 (Slice) vs. 容器/链表 (Container/List)

`kcp.flush()` 每隔 20 毫秒都会扫描发送队列，确认是否需要触发重传。

我们对顺序遍历 *slice* 与 *链表* 的成本做了基准测试（代码见 [这里](https://gist.github.com/xtaci/ac2f13f0108494d874b25551134e4c9c)）：

```
BenchmarkLoopSlice-4   	2000000000	         0.39 ns/op
BenchmarkLoopList-4    	100000000	        54.6 ns/op
```

链表节点分散在内存中，极易出现 **cache miss**；slice 则具备更好的 **局部性 (locality)**。以 5,000 条连接、窗口 32、间隔 20 毫秒为例：

- 使用 slice，每次 `kcp.flush()` 仅消耗 6 微秒（约 0.03% CPU）。
- 换成链表，耗时立刻攀升至 8.7 毫秒（约 43.5% CPU）。

因此，发送缓冲区必须使用 slice，而非链表。

### 2. 计时精度 vs. 系统调用 clock_gettime

RTT 估算离不开精准计时；误差一大，KCP 就会发生无谓的重传。然而调用 `time.Now()` 本身要消耗约 42 个 CPU 周期（4 GHz CPU 上约 10.5 ns，在 2.7 GHz MacBook Pro 上约 15.6 ns）。

`time.Now()` 的基准测试详见 [这里](https://gist.github.com/xtaci/f01503b9167f9b520b8896682b67e14d)：

```
BenchmarkNow-4         	100000000	        15.6 ns/op
```

kcp-go 通过缓存“当前时间”降低调用频次：`kcp.output()` 每次返回前更新一次时间戳，单次 `kcp.flush()` 内只读取一次系统时间。以 5,000 条连接为例，固定成本约为 5000 × 15.6 ns = 78 μs。若吞吐达到 10 MB/s（MTU 1400），`kcp.output()` 每秒被调用约 7,500 次，`time.Now()` 的额外开销仅 117 μs。

### 3. 内存管理

核心分配来自全局缓冲池 `xmit.Buf`。当需要新的缓冲区时，直接从池中取出固定容量（1500 字节，即 mtuLimit）的切片；RX、TX 以及 FEC 队列都共用这一池子，用完立即归还，避免重复清零。这样既能维持活跃对象的高水位线，确保传输过程不会频繁触发 GC，又能在空闲期把内存退回运行时。

### 4. 信息安全

kcp-go 内置多种块加密算法，并统一运行在 [CFB 模式](https://en.wikipedia.org/wiki/Block_cipher_mode_of_operation#Cipher_Feedback_(CFB)) 下。每个数据包都会先对取自 [系统熵](https://en.wikipedia.org/wiki//dev/random) 的 [nonce](https://en.wikipedia.org/wiki/Cryptographic_nonce) 进行加密，再进入正文加密流程，即便明文相同也不会生成重复密文。

密文覆盖了所有报文字段（FEC/KCP 头、校验和、载荷），从而实现真正的匿名传输。务必注意：一旦关闭底层加密，即使上层还有 TLS/HTTPS加密，头部仍会裸露，攻击者可以通过篡改 *滑动窗口*、*RTT*、*FEC 参数* 或 *校验和* 来破坏会话。推荐至少启用 `AES-128`加密——借助现代 CPU 的 [AES-NI](https://en.wikipedia.org/wiki/AES_instruction_set) 指令，它的性能甚至优于 `salsa20`（详见基准表）。

kcp-go 仍需警惕的攻击面包括：

- **[流量分析](https://en.wikipedia.org/wiki/Traffic_analysis)：** 数据流模式可能暴露访问行为。通过 [smux](https://github.com/xtaci/smux) 做多路复用并注入噪声，可在一定程度上打散特征；理论上，跨更大范围的网络混洗能够进一步缓解。
- **[重放攻击](https://en.wikipedia.org/wiki/Replay_attack)：** 协议尚未内建非对称认证，攻击者可捕获报文并在其他主机重放。虽然无法借此解密内容或劫持会话，但仍建议在上层使用带签名的非对称体系（如 HTTPS/OpenSSL/LibreSSL）保证“只处理一次”。

总之，kcp-go 的加密设计目标在于**防止篡改**，而非抵御**主动攻击**。对于高安全性诉求的场景，务必在应用层叠加成熟的加密与认证机制。

### 5. 报文时钟

1. **FastACK 即刻释放**：只要触发 FastACK，就立刻发出去，而不是等待固定 interval。
2. **ACK 集齐即发**：累计到一个 MTU 的 ACK 包立刻发送，在高速链路上相当于提供更高频率的“时钟信号”。实测单向吞吐可提升约 6 倍——如果一个 batch 1.5 ms 就能处理完，却仍然以 10 ms 的周期发送，吞吐只剩 1/6。
3. **Pacing 时钟**：为避免大 `snd_wnd` 下瞬时把大量数据塞进内核、引爆拥塞，用户态实现了 Pacing。虽然实现难度高，但 echo 测试已能稳定在 100 MB/s 以上。
4. **数据结构保持短小**：例如 `snd_buf` 使用 ringbuffer 来保持 cache coherency，队列越短，遍历成本越低。高速网络中应根据 BDP 适当减小 buffer，避免结构本身成为延迟源。需注意：当前 KCP 的 RTO 计算为 O(n)，若想降到 O(1) 必须重构。

归根结底，传输系统里没有什么比“时钟”更重要。

### 6. FEC 设计特性

- Reed-Solomon 编解码被织入 `postProcess`/`packetInput` 链路，生成与消费冗余分片都在同一条流水线上完成，不额外拉起 goroutine，也没有锁竞争。
- 单个会话可按需调节数据/冗余比例，用 ~20–30% 的带宽溢价换更平滑的尾部时延，尤其适合长距离或高丢包链路。
- 冗余分片直接复用缓冲池切片，避免反复申请与清零，哪怕是多 Gbps 传输也能让 GC 曲线保持平稳。
- 解码坚持“凑够即还原”的单趟策略，分片到齐立刻重建原始报文并推送进 `KCP.Input`，乱序与重传风暴由此大幅收敛。
- 与加密层叠加后，FEC 头部同样被遮蔽，调度/整形设备难以窥探恢复节奏，自然也更难主动打压吞吐。

## 协议规范

下图展示了完整帧格式，便于与 Wireshark 等工具对照：

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

## 性能

以下为不同平台的基准测试，包含加密、FEC、echo 等典型场景，方便横向对比：
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


## 典型火焰图

下图为 kcptun 运行时采集的典型 CPU 火焰图，可用来定位热点函数和锁竞争：
![Flame Graph in kcptun](assets/flame.png)



## 连接终止

KCP 协议 **没有** 类似 TCP 的 **SYN/FIN/RST** 控制报文，因此 keepalive/heartbeat 必须由应用层负责。实战中可以在会话之上叠加 **多路复用** 协议，例如 [smux](https://github.com/xtaci/smux)（内置 keepalive），具体做法可参考 [kcptun](https://github.com/xtaci/kcptun)。

## 常见问题 (FAQ)

以下围绕部署、FEC 与安全性列出最常见的疑问：

**Q: 我的服务器正在处理 >5K 连接，CPU 利用率非常高。**

**A:** 建议把 kcp-go 前移到独立的 `agent`/`gate` 节点。这样一来既能分担 CPU，又能提升 RTT 采样精度，从而优化重传。还可以通过 `SetNoDelay` 拉长 update `interval`（如 `conn.SetNoDelay(1, 40, 1, 1)`）来进一步降载，但要权衡可能的性能回落。

**Q: 我应该何时启用 FEC？**

**A:** 远距离链路上，丢包一旦出现就会造成巨额时延，FEC 可以在无需等待重传的情况下补齐数据。现实网络的路由路径非常多变，单靠 RTT 做丢包检测往往失灵；RTT 样本的离散会迫使 RTO 拉长，进而拖慢整体吞吐。因此跨洲/跨境等长链路强烈建议开启 FEC。

**Q: 我应该启用加密吗？**

**A:** 必须启用。即便业务层已经有 TLS/HTTPS，KCP 报文头部仍会裸露，只有开启底层加密才能防止篡改与流量分析。

## 谁在使用？

1. https://github.com/xtaci/kcptun -- 基于 KCP over UDP 的安全隧道。
2. https://github.com/getlantern/lantern -- Lantern 提供快速访问开放互联网的服务。
3. https://github.com/smallnest/rpcx -- 基于 net/rpc 的 RPC 服务框架，类似于阿里巴巴 Dubbo 和微博 Motan。
4. https://github.com/gonet2/agent -- 带有流多路复用的游戏网关。
5. https://github.com/syncthing/syncthing -- 开源持续文件同步。

### 寻找 C++ 客户端？
1. https://github.com/xtaci/libkcp -- 用于 iOS/Android 的 C++ FEC 增强 KCP 会话库

## 示例

1. [简单示例](https://github.com/xtaci/kcp-go/tree/master/examples)
2. [kcptun 客户端](https://github.com/xtaci/kcptun/blob/master/client/main.go)
3. [kcptun 服务端](https://github.com/xtaci/kcptun/blob/master/server/main.go)

## 相关链接

1. https://github.com/xtaci/smux/ -- 内存占用极少的 golang 流多路复用库
1. **https://github.com/xtaci/libkcp -- 用于 iOS/Android 的 C++ FEC 增强 KCP 会话库**
1. https://github.com/skywind3000/kcp -- 快速可靠的 ARQ 协议
1. https://github.com/klauspost/reedsolomon -- Go 语言实现的 Reed-Solomon 纠删码
