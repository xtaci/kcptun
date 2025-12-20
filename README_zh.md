# <img src="assets/logo.png" alt="kcptun" height="54px" /> 

[![Release][13]][14] [![Powered][17]][18] [![MIT licensed][11]][12] [![Build Status][3]][4] [![Go Report Card][5]][6] [![Downloads][15]][16] [![Docker][1]][2] 

[1]: https://img.shields.io/docker/pulls/xtaci/kcptun
[2]: https://hub.docker.com/r/xtaci/kcptun
[3]: https://img.shields.io/github/created-at/xtaci/kcptun
[4]: https://img.shields.io/github/created-at/xtaci/kcptun
[5]: https://goreportcard.com/badge/github.com/xtaci/kcptun
[6]: https://goreportcard.com/report/github.com/xtaci/kcptun
[11]: https://img.shields.io/github/license/xtaci/kcptun
[12]: LICENSE.md
[13]: https://img.shields.io/github/v/release/xtaci/kcptun?color=orange
[14]: https://github.com/xtaci/kcptun/releases/latest
[15]: https://img.shields.io/github/downloads/xtaci/kcptun/total.svg?maxAge=1800&color=orange
[16]: https://github.com/xtaci/kcptun/releases
[17]: https://img.shields.io/badge/KCP-Powered-blue.svg
[18]: https://github.com/skywind3000/kcp

<img src="assets/kcptun.png" alt="kcptun" height="300px"/>

> *免责声明：kcptun 仅维护一个官方网站 — [github.com/xtaci/kcptun](https://github.com/xtaci/kcptun)。任何非 [github.com/xtaci/kcptun](https://github.com/xtaci/kcptun) 的网站均未获得 xtaci 的认可。*

### 系统要求

| 目标 | 支持平台 | 推荐平台 |
| --- | --- | --- |
| 操作系统 | darwin freebsd linux windows | freebsd linux |
| 内存 | >32 MB | > 64 MB |
| CPU | 任意 | 带 AES-NI & AVX2 指令集的 amd64 处理器 |

*注意：如果您使用的是 KVM，请确保客户机操作系统支持 AES 指令集*
<img src="https://github.com/xtaci/kcptun/assets/2346725/9358e8e5-2a4a-4be9-9859-62f1aaa553b0" alt="cpuinfo" height="400px"/>

### 快速开始

下载安装脚本：

`curl -L  https://raw.githubusercontent.com/xtaci/kcptun/master/download.sh | sh`

增加服务器上的最大打开文件数：

`ulimit -n 65535`，或者将其写入 `~/.bashrc`。

建议 Linux 系统使用以下 [sysctl.conf](https://github.com/xtaci/kcptun/blob/master/dist/linux/sysctl_linux) 参数以改善 UDP 数据包处理性能：

```
net.core.rmem_max=26214400 // BDP - 带宽延迟积
net.core.rmem_default=26214400
net.core.wmem_max=26214400
net.core.wmem_default=26214400
net.core.netdev_max_backlog=2048 // 与 -rcvwnd 成比例
```
FreeBSD 相关的 sysctl 设置可以在这里找到：https://github.com/xtaci/kcptun/blob/master/dist/freebsd/sysctl_freebsd

您还可以通过添加参数来增加每个套接字的缓冲区大小（默认为 4MB）：
```
-sockbuf 16777217
```
对于**慢速处理器**，增加此缓冲区对于正确接收数据包**至关重要**。

从预编译的 [Releases](https://github.com/xtaci/kcptun/releases) 页面下载相应的二进制文件。

```
KCP 客户端: ./client_darwin_amd64 -r "KCP_SERVER_IP:4000" -l ":8388" -mode fast3 -nocomp -autoexpire 900 -sockbuf 16777217 -dscp 46
KCP 服务端: ./server_linux_amd64 -t "TARGET_IP:8388" -l ":4000" -mode fast3 -nocomp -sockbuf 16777217 -dscp 46
```
上述命令将建立一个端口转发通道，将 8388/tcp 端口转发如下：

> 应用程序 -> **KCP 客户端(8388/tcp) -> KCP 服务端(4000/udp)** -> 目标服务器(8388/tcp) 

从而通过隧道传输原始连接：

> 应用程序 -> 目标服务器(8388/tcp) 

**_或者直接使用这些完整的配置文件启动：_** [客户端](https://github.com/xtaci/kcptun/blob/master/dist/local.json.example) --> [服务端](https://github.com/xtaci/kcptun/blob/master/dist/server.json.example)

### 从源码构建

```
$ git clone https://github.com/xtaci/kcptun.git
$ cd kcptun
$ ./build-release.sh
$ cd build
```

所有预编译版本均使用 `build-release.sh` 脚本生成。

### 性能展示

<img src="assets/fast.png" alt="fast.com" height="256px" />  

![bandwidth](assets/bw.png)

![flame](assets/flame.png)

> 实际带宽图表参数： -mode fast3 -ds 10 -ps 3



### 基础调优指南

#### 提高吞吐量

> **问：我拥有高速网络链路。如何最大化带宽？**

> **答：** **同时且逐步**增加 KCP 客户端的 `-rcvwnd` 和 KCP 服务端的 `-sndwnd`。这两个值的最小值决定了链路的最大传输速率，公式为 `wnd * mtu / rtt`。然后通过下载内容测试您的连接，以验证是否满足您的要求。（MTU 可以使用 `-mtu` 参数调整。）

#### 降低延迟

> **问：我使用 kcptun 进行游戏，想要最小化延迟。**

> **答：** 延迟峰值通常表示丢包。您可以通过调整 `-mode` 参数来减少滞后。

> 例如：`-mode fast3`

> 嵌入模式的重传激进程度/响应速度：

> *fast3 > fast2 > fast > normal > default*

#### 队头阻塞 (HOLB)

由于流被多路复用到单个物理通道中，可能会发生队头阻塞。将 `-smuxbuf` 增加到一个较大的值（默认为 4MB）可以缓解此问题，但这会消耗更多内存。

对于版本 >= v20190924，您可以切换到 smux 版本 2。Smux v2 提供了限制每个流内存使用的选项。设置 `-smuxver 2` 以启用 smux v2，并调整 `-streambuf` 以控制每个流的内存消耗。例如：`-streambuf 2097152` 将每个流的内存使用限制为 2MB。限制接收端的流缓冲区会对发送端施加背压，防止链路缓冲区溢出。（`-smuxver` 设置**必须**在两端**完全相同**；默认为 1。）

#### 慢速设备

kcptun 使用 **Reed-Solomon 纠删码** 进行数据包恢复，这需要大量的计算资源。低端 ARM 设备在运行 kcptun 时可能会遇到性能问题。为了获得最佳性能，建议使用多核 x86 服务器 CPU，如 AMD Opteron。如果您必须使用 ARM 路由器，建议禁用 `FEC` 并使用 `salsa20` 进行加密。

### 专家调优指南

#### 概览

<p align="left"><img src="assets/layeredparams.png" alt="params" height="450px"/></p>

#### 用法

```bash 
$ ./client_freebsd_amd64 -h
NAME:
   kcptun - client(with SMUX)

USAGE:
   client_freebsd_amd64 [global options] command [command options] [arguments...]

VERSION:
   20251124

COMMANDS:
   help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --localaddr value, -l value      本地监听地址 (默认: ":12948")
   --remoteaddr value, -r value     kcp 服务器地址, 例如: "IP:29900" 单个端口, "IP:minport-maxport" 端口范围 (默认: "vps:29900")
   --key value                      客户端和服务端之间的预共享密钥 (默认: "it's a secrect") [$KCPTUN_KEY]
   --crypt value                    加密方式: aes, aes-128, aes-128-gcm, aes-192, salsa20, blowfish, twofish, cast5, 3des, tea, xtea, xor, sm4, none, null (默认: "aes")
   --mode value                     模式配置: fast3, fast2, fast, normal, manual (默认: "fast")
   --QPP                            启用量子置换密码本 (Quantum Permutation Pads, QPP)
   --QPPCount value                 用于 QPP 的素数密码本数量：使用的密码本越多，加密越安全。每个密码本需要 256 字节。(默认: 61)
   --conn value                     连接到服务器的 UDP 连接数 (默认: 1)
   --autoexpire value               设置单个 UDP 连接的自动过期时间（秒），0 为禁用 (默认: 0)
   --scavengettl value              设置过期连接可以存活多久（秒） (默认: 600)
   --mtu value                      设置 UDP 数据包的最大传输单元 (默认: 1350)
   --ratelimit value                设置单个 KCP 连接的最大发送速度（字节/秒），0 为禁用。也称为数据包平滑发送 (packet pacing)。(默认: 0)
   --sndwnd value                   设置发送窗口大小（数据包数量） (默认: 128)
   --rcvwnd value                   设置接收窗口大小（数据包数量） (默认: 512)
   --datashard value, --ds value    设置 Reed-Solomon 纠删码 - 数据分片 (默认: 10)
   --parityshard value, --ps value  设置 Reed-Solomon 纠删码 - 校验分片 (默认: 3)
   --dscp value                     设置 DSCP(6bit) (默认: 0)
   --nocomp                         禁用压缩
   --sockbuf value                  每个套接字的缓冲区大小（字节） (默认: 4194304)
   --smuxver value                  指定 smux 版本，可用 1,2 (默认: 2)
   --smuxbuf value                  总的解复用缓冲区大小（字节） (默认: 4194304)
   --framesize value                smux 最大帧大小 (默认: 8192)
   --streambuf value                每个流的接收缓冲区大小（字节），smux v2+ (默认: 2097152)
   --keepalive value                心跳间隔秒数 (默认: 10)
   --closewait value                关闭连接前等待的秒数 (默认: 0)
   --snmplog value                  将 snmp 收集到文件，支持 golang 时间格式，如: ./snmp-20060102.log
   --snmpperiod value               snmp 收集周期，单位秒 (默认: 60)
   --log value                      指定输出日志文件，默认输出到 stderr
   --quiet                          抑制 'stream open/close' 消息
   --tcp                            模拟 TCP 连接 (linux)
   -c value                         从 json 文件配置，这将覆盖 shell 命令中的配置
   --pprof                          在 :6060 上启动 pprof 分析服务器
   --help, -h                       显示帮助
   --version, -v                    打印版本

$ ./server_freebsd_amd64 -h
NAME:
   kcptun - server(with SMUX)

USAGE:
   server_freebsd_amd64 [global options] command [command options] [arguments...]

VERSION:
   20251124

COMMANDS:
   help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --listen value, -l value         kcp 服务器监听地址, 例如: "IP:29900" 单个端口, "IP:minport-maxport" 端口范围 (默认: ":29900")
   --target value, -t value         目标服务器地址, 或 path/to/unix_socket (默认: "127.0.0.1:12948")
   --key value                      客户端和服务端之间的预共享密钥 (默认: "it's a secrect") [$KCPTUN_KEY]
   --crypt value                    加密方式: aes, aes-128, aes-128-gcm, aes-192, salsa20, blowfish, twofish, cast5, 3des, tea, xtea, xor, sm4, none, null (默认: "aes")
   --QPP                            启用量子置换密码本 (Quantum Permutation Pads, QPP)
   --QPPCount value                 用于 QPP 的素数密码本数量：使用的密码本越多，加密越安全。每个密码本需要 256 字节。(默认: 61)
   --mode value                     模式配置: fast3, fast2, fast, normal, manual (默认: "fast")
   --mtu value                      设置 UDP 数据包的最大传输单元 (默认: 1350)
   --ratelimit value                设置单个 KCP 连接的最大发送速度（字节/秒），0 为禁用。也称为数据包平滑发送 (packet pacing)。(默认: 0)
   --sndwnd value                   设置发送窗口大小（数据包数量） (默认: 1024)
   --rcvwnd value                   设置接收窗口大小（数据包数量） (默认: 1024)
   --datashard value, --ds value    设置 Reed-Solomon 纠删码 - 数据分片 (默认: 10)
   --parityshard value, --ps value  设置 Reed-Solomon 纠删码 - 校验分片 (默认: 3)
   --dscp value                     设置 DSCP(6bit) (默认: 0)
   --nocomp                         禁用压缩
   --sockbuf value                  每个套接字的缓冲区大小（字节） (默认: 4194304)
   --smuxver value                  指定 smux 版本，可用 1,2 (默认: 2)
   --smuxbuf value                  总的解复用缓冲区大小（字节） (默认: 4194304)
   --framesize value                smux 最大帧大小 (默认: 8192)
   --streambuf value                每个流的接收缓冲区大小（字节），smux v2+ (默认: 2097152)
   --keepalive value                心跳间隔秒数 (默认: 10)
   --closewait value                关闭连接前等待的秒数 (默认: 30)
   --snmplog value                  将 snmp 收集到文件，支持 golang 时间格式，如: ./snmp-20060102.log
   --snmpperiod value               snmp 收集周期，单位秒 (默认: 60)
   --pprof                          在 :6060 上启动 pprof 分析服务器
   --log value                      指定输出日志文件，默认输出到 stderr
   --quiet                          抑制 'stream open/close' 消息
   --tcp                            模拟 TCP 连接 (linux)
   -c value                         从 json 文件配置，这将覆盖 shell 命令中的配置
   --help, -h                       显示帮助
   --version, -v                    打印版本
```

#### 多端口拨号

kcptun 支持多端口拨号，如下所示：

```
客户端: --remoteaddr IP:minport-maxport
服务端: --listen IP:minport-maxport

例如:
客户端: --remoteaddr IP:3000-4000
服务端: --listen 0.0.0.0:3000-4000
```
通过指定端口范围，kcptun 在建立每个新连接时会自动切换到该范围内的下一个随机端口。

#### 速率限制和平滑发送 (Pacing)
kcptun 引入了用户空间平滑发送机制，以实现更平滑的数据传输：https://github.com/xtaci/kcp-go/releases/tag/v5.6.36。

通过设置 `--ratelimit <value>`，您可以指定单个 KCP 连接的最大发送速度（以字节/秒为单位，包括 FEC 数据包）。将此值设置为 `0` 可禁用速率限制。启用速率限制可提高高速下的连接稳定性。（默认值：0）

此参数对于**限制非对称网络上的上传速度**特别有用。

#### 前向纠错 (FEC)

在编码理论中，[Reed–Solomon 码](https://en.wikipedia.org/wiki/Reed%E2%80%93Solomon_error_correction) 属于非二进制循环纠错码类。Reed–Solomon 码基于有限域上的单变量多项式。

它们可以检测和纠正多个符号错误。通过向数据添加 t 个校验符号，Reed–Solomon 码可以检测最多 t 个错误符号的任意组合，或纠正最多 ⌊t/2⌋ 个符号。作为一种纠删码，它可以纠正最多 t 个已知删除，或检测和纠正错误和删除的组合。此外，Reed–Solomon 码适用于纠正多突发位错误，因为 b + 1 个连续位错误的序列最多会影响两个大小为 b 的符号。t 的值由代码设计者确定，可以在很宽的范围内选择。

![FED](assets/FEC.png)

#### DSCP

区分服务 (DiffServ) 是一种计算机网络架构，它指定了一种简单、可扩展且粗粒度的机制，用于对网络流量进行分类和管理，并在现代 IP 网络上提供服务质量 (QoS)。例如，DiffServ 可用于为语音或流媒体等关键网络流量提供低延迟服务，同时为 Web 浏览或文件传输等非关键流量提供简单的尽力而为服务。

DiffServ 使用 IP 标头中 8 位区分服务字段 (DS 字段) 中的 6 位区分服务代码点 (DSCP) 进行数据包分类。DS 字段和 ECN 字段取代了过时的 IPv4 TOS 字段。

使用 ```-dscp value``` 设置每一端。这里有一些 [常用的 DSCP 值](https://en.wikipedia.org/wiki/Differentiated_services#Commonly_used_DSCP_values)。

#### 密码分析

kcptun 包含内置的数据包加密功能，由在 [密文反馈模式 (CFB)](https://en.wikipedia.org/wiki/Block_cipher_mode_of_operation#Cipher_Feedback_(CFB)) 下运行的各种块加密算法提供支持。对于每个要发送的数据包，加密过程从加密来自 [系统熵](https://en.wikipedia.org/wiki//dev/random) 的 [nonce](https://en.wikipedia.org/wiki/Cryptographic_nonce) 开始，确保加密相同的明文永远不会产生相同的密文。

数据包内容已完全加密，包括标头（FEC、KCP）、校验和及数据。请注意，无论您在上层使用哪种加密方法，如果您通过指定 `-crypt none` 禁用 kcptun 加密，传输将是不安全的，因为标头保持 ***明文***，使其容易受到篡改攻击，例如操纵 *滑动窗口大小*、*往返时间*、*FEC 属性* 和 *校验和*。建议使用 ```aes-128``` 进行最小程度的加密，因为现代 CPU 包含 [AES-NI](https://en.wikipedia.org/wiki/AES_instruction_set) 指令，性能甚至优于 `salsa20`（见下表）。

针对 kcptun 的其他可能攻击包括：

- [流量分析](https://en.wikipedia.org/wiki/Traffic_analysis) - 在数据交换期间可能会识别出特定网站的数据流模式。通过采用 [smux](https://github.com/xtaci/smux) 混合数据流并引入噪声，已缓解了此类窃听。目前尚未出现完美的解决方案；理论上，在更大规模的网络中改组/混合消息可能会进一步缓解此问题。

- [重放攻击](https://en.wikipedia.org/wiki/Replay_attack) - 由于 kcptun 中尚未集成非对称加密，因此可以在不同的机器上捕获并重放数据包。（注意：劫持会话和解密内容仍然是 *不可能的*）。因此，上层必须实现非对称加密系统或派生的 MAC 以保证真实性并防止重放攻击（确保每条消息仅处理一次）。只有通过使用私钥对请求进行签名或在初始身份验证后采用基于 HMAC 的机制，才能消除此漏洞。

重要：
1. `-crypt` 和 `-key` 在 KCP 客户端和 KCP 服务端上必须完全相同。
2. `-crypt xor` 不安全，容易受到 [已知明文攻击](https://en.wikipedia.org/wiki/Known-plaintext_attack)。除非您完全了解其含义，否则请勿使用此选项。（*密码分析说明：任何类型的 [计数器模式](https://en.wikipedia.org/wiki/Block_cipher_mode_of_operation#Counter_(CTR)) 对于数据包加密都是不安全的，因为计数器周期缩短会导致 IV/nonce 冲突。*）

kcptun 支持的加密算法基准测试：

```
BenchmarkSM4-4                 	   50000	     32087 ns/op	  93.49 MB/s	       0 B/op	       0 allocs/op
BenchmarkAES128-4              	  500000	      3274 ns/op	 916.15 MB/s	       0 B/op	       0 allocs/op
BenchmarkAES192-4              	  500000	      3587 ns/op	 836.34 MB/s	       0 B/op	       0 allocs/op
BenchmarkAES256-4              	  300000	      3828 ns/op	 783.60 MB/s	       0 B/op	       0 allocs/op
BenchmarkTEA-4                 	  100000	     15359 ns/op	 195.32 MB/s	       0 B/op	       0 allocs/op
BenchmarkXOR-4                 	20000000	        90.2 ns/op	33249.02 MB/s	       0 B/op	       0 allocs/op
BenchmarkBlowfish-4            	   50000	     26885 ns/op	 111.58 MB/s	       0 B/op	       0 allocs/op
BenchmarkNone-4                	30000000	        45.8 ns/op	65557.11 MB/s	       0 B/op	       0 allocs/op
BenchmarkCast5-4               	   50000	     34370 ns/op	  87.29 MB/s	       0 B/op	       0 allocs/op
Benchmark3DES-4                	   10000	    117893 ns/op	  25.45 MB/s	       0 B/op	       0 allocs/op
BenchmarkTwofish-4             	   50000	     33477 ns/op	  89.61 MB/s	       0 B/op	       0 allocs/op
BenchmarkXTEA-4                	   30000	     45825 ns/op	  65.47 MB/s	       0 B/op	       0 allocs/op
BenchmarkSalsa20-4             	  500000	      3282 ns/op	 913.90 MB/s	       0 B/op	       0 allocs/op
```

来自 openssl 的基准测试结果

```
$ openssl speed -evp aes-128-cfb
Doing aes-128-cfb for 3s on 16 size blocks: 157794127 aes-128-cfb's in 2.98s
Doing aes-128-cfb for 3s on 64 size blocks: 39614018 aes-128-cfb's in 2.98s
Doing aes-128-cfb for 3s on 256 size blocks: 9971090 aes-128-cfb's in 2.99s
Doing aes-128-cfb for 3s on 1024 size blocks: 2510877 aes-128-cfb's in 2.99s
Doing aes-128-cfb for 3s on 8192 size blocks: 310865 aes-128-cfb's in 2.98s
OpenSSL 1.0.2p  14 Aug 2018
built on: reproducible build, date unspecified
options:bn(64,64) rc4(ptr,int) des(idx,cisc,16,int) aes(partial) idea(int) blowfish(idx)
compiler: clang -I. -I.. -I../include  -fPIC -fno-common -DOPENSSL_PIC -DOPENSSL_THREADS -D_REENTRANT -DDSO_DLFCN -DHAVE_DLFCN_H -arch x86_64 -O3 -DL_ENDIAN -Wall -DOPENSSL_IA32_SSE2 -DOPENSSL_BN_ASM_MONT -DOPENSSL_BN_ASM_MONT5 -DOPENSSL_BN_ASM_GF2m -DSHA1_ASM -DSHA256_ASM -DSHA512_ASM -DMD5_ASM -DAES_ASM -DVPAES_ASM -DBSAES_ASM -DWHIRLPOOL_ASM -DGHASH_ASM -DECP_NISTZ256_ASM
The 'numbers' are in 1000s of bytes per second processed.
type             16 bytes     64 bytes    256 bytes   1024 bytes   8192 bytes
aes-128-cfb     847216.79k   850770.86k   853712.05k   859912.39k   854565.80k
```

kcptun 中的加密性能与 openssl 库一样快（如果不是更快的话）。

#### 抗量子计算 (Quantum Resistance)
抗量子计算，也称为量子安全、后量子或量子安全密码学，是指能够抵御量子计算机潜在破译尝试的加密算法。
从版本 v20240701 开始，kcptun 采用基于 [Kuang's Quantum Permutation Pad](https://epjquantumtechnology.springeropen.com/articles/10.1140/epjqt/s40507-022-00145-y) 的 [QPP](https://github.com/xtaci/qpp) 进行抗量子通信。

![da824f7919f70dd1dfa3be9d2302e4e0](https://github.com/xtaci/kcptun/assets/2346725/7894f5e3-6134-4582-a9fe-e78494d2e417)

要在 kcptun 中启用 QPP，请设置以下参数：
```
   --QPP                启用量子置换密码本 (Quantum Permutation Pads, QPP)
   --QPPCount value     用于 QPP 的素数密码本数量。更多的密码本提供更高的加密安全性。每个密码本需要 256 字节。(默认: 61)
```
您也可以在客户端和服务端的 JSON 配置文件中指定：
```json
     "qpp":true,
     "qpp-count":61,
```
这两个参数在两端必须完全相同。

1. 要实现**有效的抗量子性**，请在 `-key` 参数中指定至少 **211** 字节，并确保 `-QPPCount` 至少为 **7**。
2. 确保 `-QPPCount` 与 **8** **互素 (COPRIME)**（或者简单地将其设置为 **素数 (PRIME)**），例如：
```101, 103, 107, 109, 113, 127, 131, 137, 139, 149, 151, 157, 163, 167, 173, 179, 181, 191, 193, 197, 199... ```

#### 内存控制

路由器和移动设备容易受到内存限制。设置 GOGC 环境变量（例如 GOGC=20）将导致垃圾收集器更积极地回收内存。
参考：https://blog.golang.org/go15gc

主要内存分配使用 kcp-go 中的全局缓冲池 *xmit.Buf* 进行。当需要分配字节时，从该池中获取，并返回一个 *固定容量* 的 1500 字节缓冲区 (mtuLimit)。*rx 队列*、*tx 队列* 和 *fec 队列* 都从该池接收字节，并在使用后返回它们，以防止 *不必要的字节清零*。
池机制维护切片对象的 *高水位线*。这些来自池的 *在途* 对象在定期垃圾收集中存活，而池保留在空闲时将内存返回给运行时的能力。参数 `-sndwnd`、`-rcvwnd`、`-ds` 和 `-ps` 影响此 *高水位线*；较大的值会导致更大的内存消耗。

`-smuxbuf` 参数也会影响最大内存消耗，并在 *并发性* 和 *资源使用* 之间保持微妙的平衡。如果您有许多客户端要服务并且服务器功能强大，则可以增加此值（默认为 4MB）以提高并发性。相反，如果您在内存有限的嵌入式 SoC 系统上运行程序，则可以减小此值以仅服务 1-2 个客户端。（请注意，`-smuxbuf` 值与并发性不成正比；需要进行测试。）


#### 压缩

kcptun 内置了 snappy 算法用于压缩流：

> Snappy 是一个压缩/解压缩库。它的目标不是最大压缩率，
> 或与任何其他压缩库的兼容性；相反，
> 它的目标是非常高的速度和合理的压缩率。例如，
> 与 zlib 的最快模式相比，Snappy 对于大多数输入来说要快一个数量级，
> 但生成的压缩文件要大 20% 到 100%。

> 参考：http://google.github.io/snappy/

压缩可以节省 **明文** 数据的带宽，对于特定场景（如跨数据中心复制）特别有用。在跨大陆传输数据流之前压缩数据库管理系统中的 redolog 或类似 Kafka 的消息队列，可以显著提高速度。

压缩默认启用。您可以通过在 KCP 客户端和 KCP 服务端**同时**设置 ```-nocomp``` 来禁用它（该设置在两端**必须** **完全相同**）。

#### SNMP

```go
type Snmp struct {
    BytesSent        uint64 // 上层发送的字节数
    BytesReceived    uint64 // 上层接收的字节数
    MaxConn          uint64 // 达到的最大连接数
    ActiveOpens      uint64 // 累积的主动打开连接数
    PassiveOpens     uint64 // 累积的被动打开连接数
    CurrEstab        uint64 // 当前建立的连接数
    InErrs           uint64 // net.PacketConn 报告的 UDP 读取错误
    InCsumErrors     uint64 // CRC32 校验和错误
    KCPInErrors      uint64 // KCP 报告的数据包输入错误
    InPkts           uint64 // 传入数据包计数
    OutPkts          uint64 // 传出数据包计数
    InSegs           uint64 // 传入 KCP 段
    OutSegs          uint64 // 传出 KCP 段
    InBytes          uint64 // 接收的 UDP 字节数
    OutBytes         uint64 // 发送的 UDP 字节数
    RetransSegs      uint64 // 累积的重传段
    FastRetransSegs  uint64 // 累积的快速重传段
    EarlyRetransSegs uint64 // 累积的早期重传段
    LostSegs         uint64 // 推断为丢失的段数
    RepeatSegs       uint64 // 重复的段数
    FECRecovered     uint64 // 从 FEC 恢复的正确数据包
    FECErrs          uint64 // 从 FEC 恢复的错误数据包
    FECParityShards  uint64 // 接收到的 FEC 段
    FECShortShards   uint64 // 数据分片不足以恢复的数量
}
```

向 KCP 客户端或 KCP 服务端发送 `SIGUSR1` 信号会将 SNMP 信息转储到控制台，类似于 `/proc/net/snmp`。您可以使用此信息进行细粒度调优。

### 手动控制

https://github.com/skywind3000/kcp/blob/master/README.en.md#protocol-configuration

`-mode manual -nodelay 1 -interval 20 -resend 2 -nc 1`

可以使用如上所示的手动模式修改低级 KCP 配置。在进行**任何**手动调整之前，请确保您完全**理解**这些参数的含义。


### 必须相同的参数

这些参数在**两端** **必须** **完全相同**：

1. --key 和 --crypt
1. --QPP 和 --QPPCount 
1. --nocomp
1. --smuxver


### 配置示例

1. [本地](https://github.com/xtaci/kcptun/blob/master/dist/local.json.example)
1. [服务端](https://github.com/xtaci/kcptun/blob/master/dist/server.json.example)

### 参考资料

1. https://github.com/skywind3000/kcp -- KCP - A Fast and Reliable ARQ Protocol.
1. https://github.com/xtaci/kcp-go/ -- A Production-Grade Reliable-UDP Library for golang
1. https://github.com/klauspost/reedsolomon -- Reed-Solomon Erasure Coding in Go.
1. https://en.wikipedia.org/wiki/Differentiated_services -- DSCP.
1. http://google.github.io/snappy/ -- A fast compressor/decompressor.
1. https://www.backblaze.com/blog/reed-solomon/ -- Reed-Solomon Explained.
1. http://www.qualcomm.cn/products/raptorq -- RaptorQ Forward Error Correction Scheme for Object Delivery.
1. https://en.wikipedia.org/wiki/PBKDF2 -- Key stretching.
1. http://blog.appcanary.com/2016/encrypt-or-compress.html -- Should you encrypt or compress first?
1. https://github.com/hashicorp/yamux -- Connection multiplexing library.
1. https://tools.ietf.org/html/rfc6937 -- Proportional Rate Reduction for TCP.
1. https://tools.ietf.org/html/rfc5827 -- Early Retransmit for TCP and Stream Control Transmission Protocol (SCTP).
1. http://http2.github.io/ -- What is HTTP/2?
1. http://www.lartc.org/ -- Linux Advanced Routing & Traffic Control
1. https://en.wikipedia.org/wiki/Noisy-channel_coding_theorem -- Noisy channel coding theorem
1. https://zhuanlan.zhihu.com/p/53849089 -- kcptun开发小记

### 捐赠
点击 [这里](https://github.com/xtaci/xtaci/issues/2) 捐赠。


***（注意：kcptun没有任何社交网站的账号，请小心骗子。）***
