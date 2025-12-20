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

[**中文说明**](README_zh.md)

<img src="assets/kcptun.png" alt="kcptun" height="300px"/>

> *Disclaimer: kcptun maintains a single website — [github.com/xtaci/kcptun](https://github.com/xtaci/kcptun). Any websites other than [github.com/xtaci/kcptun](https://github.com/xtaci/kcptun) are not endorsed by xtaci.*

### Requirements

| Target | Supported | Recommended |
| --- | --- | --- |
| System | darwin freebsd linux windows | freebsd linux |
| Memory | >32 MB | > 64 MB |
| CPU | ANY | amd64 with AES-NI & AVX2 |

*NOTE: If you are using KVM, ensure that the guest OS supports AES instructions*
<img src="https://github.com/xtaci/kcptun/assets/2346725/9358e8e5-2a4a-4be9-9859-62f1aaa553b0" alt="cpuinfo" height="400px"/>

### QuickStart

Download:

`curl -L  https://raw.githubusercontent.com/xtaci/kcptun/master/download.sh | sh`

Increase the number of open files on your server, as:

`ulimit -n 65535`, or write it in `~/.bashrc`.

Suggested [sysctl.conf](https://github.com/xtaci/kcptun/blob/master/dist/linux/sysctl_linux) parameters for Linux to improve UDP packet handling:

```
net.core.rmem_max=26214400 // BDP - Bandwidth Delay Product
net.core.rmem_default=26214400
net.core.wmem_max=26214400
net.core.wmem_default=26214400
net.core.netdev_max_backlog=2048 // Proportional to -rcvwnd
```
FreeBSD-related sysctl settings can be found here: https://github.com/xtaci/kcptun/blob/master/dist/freebsd/sysctl_freebsd

You can also increase the per-socket buffer by adding the parameter (default 4MB):
```
-sockbuf 16777217
```
For **slow processors**, increasing this buffer is **CRITICAL** for proper packet reception.

Download the appropriate binary from the precompiled [Releases](https://github.com/xtaci/kcptun/releases).

```
KCP Client: ./client_darwin_amd64 -r "KCP_SERVER_IP:4000" -l ":8388" -mode fast3 -nocomp -autoexpire 900 -sockbuf 16777217 -dscp 46
KCP Server: ./server_linux_amd64 -t "TARGET_IP:8388" -l ":4000" -mode fast3 -nocomp -sockbuf 16777217 -dscp 46
```
The above commands will establish a port forwarding channel for port 8388/tcp as follows:

> Application -> **KCP Client(8388/tcp) -> KCP Server(4000/udp)** -> Target Server(8388/tcp) 

which tunnels the original connection:

> Application -> Target Server(8388/tcp) 

**_OR START WITH THESE COMPLETE CONFIGURATION FILES:_** [client](https://github.com/xtaci/kcptun/blob/master/dist/local.json.example) --> [server](https://github.com/xtaci/kcptun/blob/master/dist/server.json.example)

### Building from source

```
$ git clone https://github.com/xtaci/kcptun.git
$ cd kcptun
$ ./build-release.sh
$ cd build
```

All precompiled releases are generated using the `build-release.sh` script.

### Performance

<img src="assets/fast.png" alt="fast.com" height="256px" />  

![bandwidth](assets/bw.png)

![flame](assets/flame.png)

> Practical bandwidth graph with parameters:  -mode fast3 -ds 10 -ps 3



### Basic Tuning Guide

#### To Improve Throughput

> **Q: I have a high-speed network link. How can I maximize bandwidth?**

> **A:** Increase `-rcvwnd` on the KCP Client and `-sndwnd` on the KCP Server **simultaneously and gradually**. The minimum of these values determines the maximum transfer rate of the link using the formula `wnd * mtu / rtt`. Then test your connection by downloading content to verify it meets your requirements. (The MTU can be adjusted using the `-mtu` parameter.)

#### To Improve Latency

> **Q: I'm using kcptun for gaming and want to minimize latency.**

> **A:** Latency spikes often indicate packet loss. You can reduce lag by adjusting the `-mode` parameter.

> For example: `-mode fast3`

> Retransmission aggressiveness/responsiveness for embedded modes:

> *fast3 > fast2 > fast > normal > default*

#### Head-of-Line Blocking (HOLB)

Since streams are multiplexed into a single physical channel, head-of-line blocking may occur. Increasing `-smuxbuf` to a larger value (default is 4MB) can mitigate this issue, though it will consume more memory.

For versions >= v20190924, you can switch to smux version 2. Smux v2 provides options to limit per-stream memory usage. Set `-smuxver 2` to enable smux v2, and adjust `-streambuf` to control per-stream memory consumption. For example: `-streambuf 2097152` limits per-stream memory usage to 2MB. Limiting the stream buffer on the receiver side applies back-pressure to the sender, preventing buffer overflow along the link. (The `-smuxver` setting **MUST** be **IDENTICAL** on both sides; the default is 1.)

#### Slow Devices

kcptun uses **Reed-Solomon Codes** for packet recovery, which requires substantial computational resources. Low-end ARM devices may experience performance issues with kcptun. For optimal performance, a multi-core x86 server CPU such as AMD Opteron is recommended. If you must use ARM routers, it's advisable to disable `FEC` and use `salsa20` for encryption.

### Expert Tuning Guide

#### Overview

<p align="left"><img src="assets/layeredparams.png" alt="params" height="450px"/></p>

#### Usage

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
   --localaddr value, -l value      local listen address (default: ":12948")
   --remoteaddr value, -r value     kcp server address, eg: "IP:29900" a for single port, "IP:minport-maxport" for port range (default: "vps:29900")
   --key value                      pre-shared secret between client and server (default: "it's a secrect") [$KCPTUN_KEY]
   --crypt value                    aes, aes-128, aes-128-gcm, aes-192, salsa20, blowfish, twofish, cast5, 3des, tea, xtea, xor, sm4, none, null (default: "aes")
   --mode value                     profiles: fast3, fast2, fast, normal, manual (default: "fast")
   --QPP                            enable Quantum Permutation Pads(QPP)
   --QPPCount value                 the prime number of pads to use for QPP: The more pads you use, the more secure the encryption. Each pad requires 256 bytes. (default: 61)
   --conn value                     set num of UDP connections to server (default: 1)
   --autoexpire value               set auto expiration time(in seconds) for a single UDP connection, 0 to disable (default: 0)
   --scavengettl value              set how long an expired connection can live (in seconds) (default: 600)
   --mtu value                      set maximum transmission unit for UDP packets (default: 1350)
   --ratelimit value                set maximum outgoing speed (in bytes per second) for a single KCP connection, 0 to disable. Also known as packet pacing. (default: 0)
   --sndwnd value                   set send window size(num of packets) (default: 128)
   --rcvwnd value                   set receive window size(num of packets) (default: 512)
   --datashard value, --ds value    set reed-solomon erasure coding - datashard (default: 10)
   --parityshard value, --ps value  set reed-solomon erasure coding - parityshard (default: 3)
   --dscp value                     set DSCP(6bit) (default: 0)
   --nocomp                         disable compression
   --sockbuf value                  per-socket buffer in bytes (default: 4194304)
   --smuxver value                  specify smux version, available 1,2 (default: 2)
   --smuxbuf value                  the overall de-mux buffer in bytes (default: 4194304)
   --framesize value                smux max frame size (default: 8192)
   --streambuf value                per stream receive buffer in bytes, smux v2+ (default: 2097152)
   --keepalive value                seconds between heartbeats (default: 10)
   --closewait value                the seconds to wait before tearing down a connection (default: 0)
   --snmplog value                  collect snmp to file, aware of timeformat in golang, like: ./snmp-20060102.log
   --snmpperiod value               snmp collect period, in seconds (default: 60)
   --log value                      specify a log file to output, default goes to stderr
   --quiet                          to suppress the 'stream open/close' messages
   --tcp                            to emulate a TCP connection(linux)
   -c value                         config from json file, which will override the command from shell
   --pprof                          start profiling server on :6060
   --help, -h                       show help
   --version, -v                    print the version

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
   --listen value, -l value         kcp server listen address, eg: "IP:29900" for a single port, "IP:minport-maxport" for port range (default: ":29900")
   --target value, -t value         target server address, or path/to/unix_socket (default: "127.0.0.1:12948")
   --key value                      pre-shared secret between client and server (default: "it's a secrect") [$KCPTUN_KEY]
   --crypt value                    aes, aes-128, aes-128-gcm, aes-192, salsa20, blowfish, twofish, cast5, 3des, tea, xtea, xor, sm4, none, null (default: "aes")
   --QPP                            enable Quantum Permutation Pads(QPP)
   --QPPCount value                 the prime number of pads to use for QPP: The more pads you use, the more secure the encryption. Each pad requires 256 bytes. (default: 61)
   --mode value                     profiles: fast3, fast2, fast, normal, manual (default: "fast")
   --mtu value                      set maximum transmission unit for UDP packets (default: 1350)
   --ratelimit value                set maximum outgoing speed (in bytes per second) for a single KCP connection, 0 to disable. Also known as packet pacing. (default: 0)
   --sndwnd value                   set send window size(num of packets) (default: 1024)
   --rcvwnd value                   set receive window size(num of packets) (default: 1024)
   --datashard value, --ds value    set reed-solomon erasure coding - datashard (default: 10)
   --parityshard value, --ps value  set reed-solomon erasure coding - parityshard (default: 3)
   --dscp value                     set DSCP(6bit) (default: 0)
   --nocomp                         disable compression
   --sockbuf value                  per-socket buffer in bytes (default: 4194304)
   --smuxver value                  specify smux version, available 1,2 (default: 2)
   --smuxbuf value                  the overall de-mux buffer in bytes (default: 4194304)
   --framesize value                smux max frame size (default: 8192)
   --streambuf value                per stream receive buffer in bytes, smux v2+ (default: 2097152)
   --keepalive value                seconds between heartbeats (default: 10)
   --closewait value                the seconds to wait before tearing down a connection (default: 30)
   --snmplog value                  collect snmp to file, aware of timeformat in golang, like: ./snmp-20060102.log
   --snmpperiod value               snmp collect period, in seconds (default: 60)
   --pprof                          start profiling server on :6060
   --log value                      specify a log file to output, default goes to stderr
   --quiet                          to suppress the 'stream open/close' messages
   --tcp                            to emulate a TCP connection(linux)
   -c value                         config from json file, which will override the command from shell
   --help, -h                       show help
   --version, -v                    print the version
```

#### Multiport Dialer

kcptun supports multi-port dialing as follows:

```
client: --remoteaddr IP:minport-maxport
server: --listen IP:minport-maxport

eg:
client: --remoteaddr IP:3000-4000
server: --listen 0.0.0.0:3000-4000
```
By specifying a port range, kcptun will automatically switch to the next random port within that range when establishing each new connection.

#### Rate Limit and Pacing

kcptun supports userspace packet pacing to smooth out data transmission.

**Why use it?**
Without pacing, KCP may send data in large bursts (micro-bursts). These sudden spikes can overflow the network interface card (NIC) buffers or the OS kernel's UDP buffer, causing **local packet drops** before the data even leaves your server. This is especially common on high-speed links or restricted environments.

**How to use:**
Use `--ratelimit <value>` to set the maximum outgoing speed (in bytes per second) for a single KCP connection.
- Example: `--ratelimit 1048576` limits the speed to 1MB/s.
- Default: `0` (unlimited).

**Benefits:**
1. **Prevents Kernel Drops**: Reduces the risk of `ENOBUFS` errors and kernel-level packet drops.
2. **Smoother Traffic**: Creates a more consistent flow of packets, which is friendlier to intermediate routers and reduces jitter.
3. **Bandwidth Control**: Useful for limiting upload speed on asymmetric networks (e.g., ADSL/Cable).

#### Forward Error Correction

kcptun uses [Reed-Solomon Codes](https://en.wikipedia.org/wiki/Reed%E2%80%93Solomon_error_correction) to recover lost packets, which significantly improves data throughput on lossy networks.

You can configure the FEC parameters using the following flags:
- `--datashard, -ds`: Number of data shards (default: 10).
- `--parityshard, -ps`: Number of parity shards (default: 3).

**How it works:**
For every `datashard` packets sent, `parityshard` redundant packets are generated and sent. This allows the receiver to recover the original data even if up to `parityshard` packets are lost within the group of `datashard + parityshard` packets.

**Overhead:**
The bandwidth overhead can be calculated as: `parityshard / datashard`.
For the default setting (10 data, 3 parity), the overhead is 30%.

**Configuration Guide:**
1. **AutoTune**: The receiver automatically detects and adapts to the sender's FEC parameters (DataShard/ParityShard), so you can adjust them on one side without restarting the other.
2. **Tuning**:
   - Increase `-parityshard` to improve reliability on highly lossy networks, at the cost of higher bandwidth usage.
   - Decrease `-parityshard` to reduce bandwidth overhead if the network quality is good.
3. **Disable FEC**: Set `--parityshard 0` to disable Forward Error Correction. This saves CPU and bandwidth but reduces reliability on unstable networks.

![FEC](assets/FEC.png)

#### DSCP

Differentiated Services (DiffServ) is a computer networking architecture that specifies a simple, scalable, and coarse-grained mechanism for classifying and managing network traffic and providing Quality of Service (QoS) on modern IP networks. DiffServ can, for example, be used to provide low-latency service to critical network traffic such as voice or streaming media while providing simple best-effort service to non-critical traffic such as web browsing or file transfers.

DiffServ uses a 6-bit differentiated services code point (DSCP) in the 8-bit differentiated services field (DS field) in the IP header for packet classification purposes. The DS field and ECN field replace the outdated IPv4 TOS field.

Set each side with ```-dscp value```. Here are some [commonly used DSCP values](https://en.wikipedia.org/wiki/Differentiated_services#Commonly_used_DSCP_values).

#### Cryptoanalysis

kcptun includes built-in packet encryption powered by various block encryption algorithms operating in [Cipher Feedback Mode](https://en.wikipedia.org/wiki/Block_cipher_mode_of_operation#Cipher_Feedback_(CFB)). For each packet to be sent, the encryption process begins by encrypting a [nonce](https://en.wikipedia.org/wiki/Cryptographic_nonce) from the [system entropy](https://en.wikipedia.org/wiki//dev/random), ensuring that encrypting identical plaintexts never produces identical ciphertexts.

Packet contents are fully encrypted, including headers (FEC, KCP), checksums, and data. Note that regardless of which encryption method you use in your upper layer, if you disable kcptun encryption by specifying `-crypt none`, the transmission will be insecure because the header remains ***PLAINTEXT***, making it susceptible to tampering attacks such as manipulation of the *sliding window size*, *round-trip time*, *FEC properties*, and *checksums*. ```aes-128``` is recommended for minimal encryption since modern CPUs include [AES-NI](https://en.wikipedia.org/wiki/AES_instruction_set) instructions and perform even better than `salsa20` (see the table below).

Other possible attacks against kcptun include: 

- [Traffic analysis](https://en.wikipedia.org/wiki/Traffic_analysis) - data flow patterns from specific websites may be identifiable during data exchange. This type of eavesdropping has been mitigated by adapting [smux](https://github.com/xtaci/smux) to mix data streams and introduce noise. A perfect solution has not yet emerged; theoretically, shuffling/mixing messages across a larger-scale network may further mitigate this problem. 

- [Replay attack](https://en.wikipedia.org/wiki/Replay_attack) - since asymmetric encryption has not been integrated into kcptun, capturing and replaying packets on a different machine is possible. (Note: hijacking sessions and decrypting contents remains *impossible*). Therefore, upper layers must implement an asymmetric cryptosystem or a derived MAC to guarantee authenticity and prevent replay attacks (ensuring each message is processed exactly once). This vulnerability can only be eliminated by signing requests with private keys or employing an HMAC-based mechanism following initial authentication.

Important:
1. `-crypt` and `-key` must be identical on both the KCP Client and KCP Server.
2. `-crypt xor` is insecure and vulnerable to [known-plaintext attacks](https://en.wikipedia.org/wiki/Known-plaintext_attack). Do not use this unless you fully understand the implications. (*Cryptanalysis note: any type of [counter mode](https://en.wikipedia.org/wiki/Block_cipher_mode_of_operation#Counter_(CTR)) is insecure for packet encryption due to shortened counter periods that lead to IV/nonce collisions.*)

Benchmarks for crypto algorithms supported by kcptun:

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

Benchmark result from openssl

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

The encryption performance in kcptun is as fast as in openssl library(if not faster).

#### Quantum Resistance
Quantum Resistance, also known as quantum-secure, post-quantum, or quantum-safe cryptography, refers to cryptographic algorithms that can withstand potential code-breaking attempts by quantum computers.
Starting with version v20240701, kcptun adopts [QPP](https://github.com/xtaci/qpp) based on [Kuang's Quantum Permutation Pad](https://epjquantumtechnology.springeropen.com/articles/10.1140/epjqt/s40507-022-00145-y) for quantum-resistant communication.

![da824f7919f70dd1dfa3be9d2302e4e0](https://github.com/xtaci/kcptun/assets/2346725/7894f5e3-6134-4582-a9fe-e78494d2e417)

To enable QPP in kcptun, set the following parameters:
```
   --QPP                Enable Quantum Permutation Pads (QPP)
   --QPPCount value     The prime number of pads to use for QPP. More pads provide greater encryption security. Each pad requires 256 bytes. (default: 61)
```
You can also specify
```json
     "qpp":true,
     "qpp-count":61,
```
in your client and server-side JSON configuration files. These two parameters must be identical on both sides.

1. To achieve **effective quantum resistance**, specify at least **211** bytes in the `-key` parameter and ensure `-QPPCount` is at least **7**.
2. Ensure that `-QPPCount` is **COPRIME (互素)** to **8** (or simply set it to a **PRIME** number) such as: 
```101, 103, 107, 109, 113, 127, 131, 137, 139, 149, 151, 157, 163, 167, 173, 179, 181, 191, 193, 197, 199... ```

#### Memory Control

Routers and mobile devices are susceptible to memory constraints. Setting the GOGC environment variable (e.g., GOGC=20) will cause the garbage collector to recycle memory more aggressively.
Reference: https://blog.golang.org/go15gc

Primary memory allocation is performed using a global buffer pool *xmit.Buf* in kcp-go. When bytes need to be allocated, they are obtained from this pool, and a *fixed-capacity* 1500-byte buffer (mtuLimit) is returned. The *rx queue*, *tx queue*, and *fec queue* all receive bytes from this pool and return them after use to prevent *unnecessary zeroing* of bytes. 
The pool mechanism maintains a *high watermark* for slice objects. These *in-flight* objects from the pool survive periodic garbage collection, while the pool retains the ability to return memory to the runtime when idle. The parameters `-sndwnd`, `-rcvwnd`, `-ds`, and `-ps` affect this *high watermark*; larger values result in greater memory consumption.

The `-smuxbuf` parameter also affects maximum memory consumption and maintains a delicate balance between *concurrency* and *resource usage*. You can increase this value (default 4MB) to boost concurrency if you have many clients to serve and a powerful server. Conversely, you can decrease this value to serve only 1-2 clients if you're running the program on an embedded SoC system with limited memory. (Note that the `-smuxbuf` value is not directly proportional to concurrency; testing is required.)


#### Compression

kcptun has builtin snappy algorithms for compressing streams:

> Snappy is a compression/decompression library. It does not aim for maximum
> compression, or compatibility with any other compression library; instead,
> it aims for very high speeds and reasonable compression. For instance,
> compared to the fastest mode of zlib, Snappy is an order of magnitude faster
> for most inputs, but the resulting compressed files are anywhere from 20% to
> 100% bigger.

> Reference: http://google.github.io/snappy/

Compression can save bandwidth for **PLAINTEXT** data and is particularly useful for specific scenarios such as cross-datacenter replication. Compressing redologs in database management systems or Kafka-like message queues before transferring data streams across continents can significantly improve speed.

Compression is enabled by default. You can disable it by setting ```-nocomp``` on **BOTH** the KCP Client and KCP Server (the setting **MUST** be **IDENTICAL** on both sides).

#### SNMP

```go
type Snmp struct {
    BytesSent        uint64 // bytes sent from upper level
    BytesReceived    uint64 // bytes received to upper level
    MaxConn          uint64 // max number of connections ever reached
    ActiveOpens      uint64 // accumulated active open connections
    PassiveOpens     uint64 // accumulated passive open connections
    CurrEstab        uint64 // current number of established connections
    InErrs           uint64 // UDP read errors reported from net.PacketConn
    InCsumErrors     uint64 // checksum errors from CRC32
    KCPInErrors      uint64 // packet input errors reported from KCP
    InPkts           uint64 // incoming packets count
    OutPkts          uint64 // outgoing packets count
    InSegs           uint64 // incoming KCP segments
    OutSegs          uint64 // outgoing KCP segments
    InBytes          uint64 // UDP bytes received
    OutBytes         uint64 // UDP bytes sent
    RetransSegs      uint64 // accumulated retransmitted segments
    FastRetransSegs  uint64 // accumulated fast retransmitted segments
    EarlyRetransSegs uint64 // accumulated early retransmitted segments
    LostSegs         uint64 // number of segs inferred as lost
    RepeatSegs       uint64 // number of segs duplicated
    FECRecovered     uint64 // correct packets recovered from FEC
    FECErrs          uint64 // incorrect packets recovered from FEC
    FECParityShards  uint64 // FEC segments received
    FECShortShards   uint64 // number of data shards that's not enough for recovery
}
```

Sending a `SIGUSR1` signal to the KCP Client or KCP Server will dump SNMP information to the console, similar to `/proc/net/snmp`. You can use this information for fine-grained tuning.

### Manual Control

https://github.com/skywind3000/kcp/blob/master/README.en.md#protocol-configuration

`-mode manual -nodelay 1 -interval 20 -resend 2 -nc 1`

Low-level KCP configuration can be modified using manual mode as shown above. Make sure you fully **UNDERSTAND** what these parameters mean before making **ANY** manual adjustments.


### Identical Parameters

These parameters **MUST** be **IDENTICAL** on **BOTH** sides:

1. --key and --crypt
1. --QPP and --QPPCount 
1. --nocomp
1. --smuxver


### Example Configurations

1. [Local](https://github.com/xtaci/kcptun/blob/master/dist/local.json.example)
1. [Server](https://github.com/xtaci/kcptun/blob/master/dist/server.json.example)

### References

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

### Donation
Click [here](https://github.com/xtaci/xtaci/issues/2) to donate.


***（注意：kcptun没有任何社交网站的账号，请小心骗子。）***
