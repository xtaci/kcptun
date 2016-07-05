# <img src="logo.png" alt="kcptun" height="60px" /> 
[![GoDoc][1]][2] [![Release][13]][14] [![Powered][17]][18] [![Build Status][3]][4] [![Go Report Card][5]][6] [![Downloads][15]][16] 
[1]: https://godoc.org/github.com/xtaci/kcptun?status.svg
[2]: https://godoc.org/github.com/xtaci/kcptun
[3]: https://travis-ci.org/xtaci/kcptun.svg?branch=master
[4]: https://travis-ci.org/xtaci/kcptun
[5]: https://goreportcard.com/badge/github.com/xtaci/kcptun
[6]: https://goreportcard.com/report/github.com/xtaci/kcptun
[7]: https://img.shields.io/badge/license-MIT-blue.svg
[8]: https://raw.githubusercontent.com/xtaci/kcptun/master/LICENSE.md
[9]: https://img.shields.io/github/stars/xtaci/kcptun.svg
[10]: https://github.com/xtaci/kcptun/stargazers
[11]: https://img.shields.io/github/forks/xtaci/kcptun.svg
[12]: https://github.com/xtaci/kcptun/network
[13]: https://img.shields.io/github/release/xtaci/kcptun.svg
[14]: https://github.com/xtaci/kcptun/releases/latest
[15]: https://img.shields.io/github/downloads/xtaci/kcptun/total.svg?maxAge=2592000
[16]: https://github.com/xtaci/kcptun/releases
[17]: https://img.shields.io/badge/KCP-Powered-blue.svg
[18]: https://github.com/skywind3000/kcp
[19]: https://img.shields.io/docker/pulls/xtaci/kcptun.svg?maxAge=2592000
[20]: https://hub.docker.com/r/xtaci/kcptun/

A tool for converting tcp stream into kcp+udp stream, :zap: ***[offical download address](https://github.com/xtaci/kcptun/releases/latest)***:zap:workflow：  

![kcptun](kcptun.png)

***kcptun is based on [kcp-go](https://github.com/xtaci/kcp-go), A remote port forwarder***   

### *Quickstart* :lollipop:
```
Server Side: ./server_linux_amd64 -t "127.0.0.1:1080" -l ":554" -mode fast2  // forwarding to local port 1080
Client Side: ./client_darwin_amd64 -r "SERVERIP:554" -l ":1080" -mode fast2  // listening on port 1080
```

### *Usage* :lollipop:
![client](client.png)
![server](server.png)

### *Applications* :lollipop:   
1. RealTime gaming.
2. Cross-ISP data exchange in PRC.
3. Other lossy network.

### *Parameters Recommended* :lollipop: 
```
Test Environment: China Telecom 100M ADSL(100mbps up/8mbps down)
SERVER:   -mtu 1400 -sndwnd 2048 -rcvwnd 2048 -mode fast2
CLIENT:   -mtu 1400 -sndwnd 256 -rcvwnd 2048 -mode fast2 -dscp 46
```

*How to optimize*：
> Step 1：Increase client rcvwnd & server sndwnd simultaneously in small step。
> Step 2：Try download something，if the bandwidth usage is close the limit stop, otherwise goto step 1.

### *DSCP* :lollipop: 
Differentiated services or DiffServ is a computer networking architecture that specifies a simple, scalable and coarse-grained mechanism for classifying and managing network traffic and providing quality of service (QoS) on modern IP networks. DiffServ can, for example, be used to provide low-latency to critical network traffic such as voice or streaming media while providing simple best-effort service to non-critical services such as web traffic or file transfers.

DiffServ uses a 6-bit differentiated services code point (DSCP) in the 8-bit differentiated services field (DS field) in the IP header for packet classification purposes. The DS field and ECN field replace the outdated IPv4 TOS field.[1]

### *Embeded Mode* :lollipop: 
Latency:     
*fast3 >* ***[fast2]*** *> fast > normal > default*        
Payload Ratio:     
*default > normal > fast >* ***[fast2]*** *> fast3*       
Parameters in middle is balanced for latency & payload ratio, the faster you get the more wasteful you are.
Manual control is supported with hidden parameters, you must understand KCP protocol before doing this.
```
 -mode manual -nodelay 1 -resend 2 -nc 1 -interval 20
```

### *Forward Error Correction* :lollipop: 
In coding theory, the Reed–Solomon code belongs to the class of non-binary cyclic error-correcting codes. The Reed–Solomon code is based on univariate polynomials over finite fields.

It is able to detect and correct multiple symbol errors. By adding t check symbols to the data, a Reed–Solomon code can detect any combination of up to t erroneous symbols, or correct up to ⌊t/2⌋ symbols. As an erasure code, it can correct up to t known erasures, or it can detect and correct combinations of errors and erasures. Furthermore, Reed–Solomon codes are suitable as multiple-burst bit-error correcting codes, since a sequence of b + 1 consecutive bit errors can affect at most two symbols of size b. The choice of t is up to the designer of the code, and may be selected within wide limits.

![reed-solomon](rs.png)

Adjust parameters of RS-Code with ```-datashard 10 -parityshard 3``` 

### *Snappy Stream Compression* :lollipop: 
> Snappy is a compression/decompression library. It does not aim for maximum
> compression, or compatibility with any other compression library; instead,
> it aims for very high speeds and reasonable compression. For instance,
> compared to the fastest mode of zlib, Snappy is an order of magnitude faster
> for most inputs, but the resulting compressed files are anywhere from 20% to
> 100% bigger.

Reference: http://google.github.io/snappy/

### *SNMP* :lollipop:
```go
// Snmp defines network statistics indicator
type Snmp struct {
	BytesSent        uint64 // payload bytes sent
	BytesReceived    uint64
	MaxConn          uint64
	ActiveOpens      uint64
	PassiveOpens     uint64
	CurrEstab        uint64
	InErrs           uint64
	InCsumErrors     uint64 // checksum errors
	InSegs           uint64
	OutSegs          uint64
	OutBytes         uint64 // udp bytes sent
	RetransSegs      uint64
	FastRetransSegs  uint64
	EarlyRetransSegs uint64
	LostSegs         uint64
	RepeatSegs       uint64
	FECRecovered     uint64
	FECErrs          uint64
	FECSegs          uint64 // fec segments received
}
```

Sending a signal by ```kill -SIGUSR1 pid``` will give SNMP information for KCP，useful for fine-grained adjustment.
Of which ```RetransSegs,FastRetransSegs,LostSegs,OutSegs``` is the most useful.

### *Performance* :lollipop:
```
root@vultr:~# iperf -s
------------------------------------------------------------
Server listening on TCP port 5001
TCP window size: 4.00 MByte (default)
------------------------------------------------------------
[  4] local 172.7.7.1 port 5001 connected with 172.7.7.2 port 55453
[ ID] Interval       Transfer     Bandwidth
[  4]  0.0-18.0 sec  5.50 MBytes  2.56 Mbits/sec     <-- connection via kcptun
[  5] local 45.32.xxx.xxx port 5001 connected with 218.88.xxx.xxx port 17220
[  5]  0.0-17.9 sec  2.12 MBytes   997 Kbits/sec     <-- direct connnection via tcp
```

### *Donations* :dollar:
![donate](donate.png)          

All donations to this project will be used on the R&D of [gonet/2](http://gonet2.github.io/).

### *References* :paperclip:
1. https://github.com/skywind3000/kcp -- KCP - A Fast and Reliable ARQ Protocol.
2. https://github.com/klauspost/reedsolomon -- Reed-Solomon Erasure Coding in Go.
3. https://en.wikipedia.org/wiki/Differentiated_services -- DSCP.
4. http://google.github.io/snappy/ -- A fast compressor/decompressor.
5. https://www.backblaze.com/blog/reed-solomon/ -- Reed-Solomon Explained.
6. http://www.qualcomm.cn/products/raptorq -- RaptorQ Forward Error Correction Scheme for Object Delivery.
7. https://en.wikipedia.org/wiki/PBKDF2 -- Key stretching.
8. http://blog.appcanary.com/2016/encrypt-or-compress.html -- Should you encrypt or compress first?
9. https://github.com/hashicorp/yamux -- Connection multiplexing library.
10. https://tools.ietf.org/html/rfc6937 -- Proportional Rate Reduction for TCP.
11. https://tools.ietf.org/html/rfc5827 -- Early Retransmit for TCP and Stream Control Transmission Protocol (SCTP).
12. http://http2.github.io/ -- What is HTTP/2?
