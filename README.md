# <img src="logo.png" alt="kcptun" height="60px" /> 
[![Release][13]][14] [![Powered][17]][18] [![MIT licensed][11]][12] [![Build Status][3]][4] [![Go Report Card][5]][6] [![Downloads][15]][16] [![Gitter][19]][20] [![Docker][1]][2]
[1]: https://images.microbadger.com/badges/image/xtaci/kcptun.svg
[2]: https://microbadger.com/images/xtaci/kcptun
[3]: https://travis-ci.org/xtaci/kcptun.svg?branch=master
[4]: https://travis-ci.org/xtaci/kcptun
[5]: https://goreportcard.com/badge/github.com/xtaci/kcptun
[6]: https://goreportcard.com/report/github.com/xtaci/kcptun
[7]: https://img.shields.io/badge/license-MIT-blue.svg
[8]: https://raw.githubusercontent.com/xtaci/kcptun/master/LICENSE.md
[11]: https://img.shields.io/badge/license-MIT-blue.svg
[12]: LICENSE.md
[13]: https://img.shields.io/github/release/xtaci/kcptun.svg
[14]: https://github.com/xtaci/kcptun/releases/latest
[15]: https://img.shields.io/github/downloads/xtaci/kcptun/total.svg?maxAge=1800
[16]: https://github.com/xtaci/kcptun/releases
[17]: https://img.shields.io/badge/KCP-Powered-blue.svg
[18]: https://github.com/skywind3000/kcp
[19]: https://badges.gitter.im/xtaci/kcptun.svg
[20]: https://gitter.im/xtaci/kcptun?utm_source=badge&utm_medium=badge&utm_campaign=pr-badge

***[kcp-go](https://github.com/xtaci/kcp-go)协议测试小工具 :zap: [官方下载地址](https://github.com/xtaci/kcptun/releases/latest):zap:***

![kcptun](kcptun.png)
[English Readme](README.en.md)
### *快速设定* :lollipop:
```
服务器: ./server_linux_amd64 -t "127.0.0.1:8388" -l ":4000" -mode fast2  // 转发到服务器的本地8388端口
客户端: ./client_darwin_amd64 -r "服务器IP地址:4000" -l ":8388" -mode fast2    // 监听客户端的本地8388端口
注: 服务器端需要有服务监听8388端口
```
Windows用户可以关注GUI客户端管理工具　https://github.com/dfdragon/kcptun_gclient


### *速度对比* :lollipop:
<img src="fast.png" alt="fast.com" height="256px" />       
* 测速网站: https://fast.com
* 接入: 100M ADSL
* WIFI: 5GHz TL-WDR3320

### *使用方法* :lollipop:
在Mac OS X El Capitan下的帮助输出: 
```
$ ./client_darwin_amd64 -h
NAME:
   kcptun - kcptun client

USAGE:
   client_darwin_amd64 [global options] command [command options] [arguments...]

VERSION:
   20160820

COMMANDS:
     help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --localaddr value, -l value   local listen address (default: ":12948")
   --remoteaddr value, -r value  kcp server address (default: "vps:29900")
   --key value                   pre-shared secret for client and server (default: "it's a secrect") [$KCPTUN_KEY]
   --crypt value                 aes, aes-128, aes-192, salsa20, blowfish, twofish, cast5, 3des, tea, xtea, xor, none (default: "aes")
   --mode value                  profiles: fast3, fast2, fast, normal (default: "fast")
   --conn value                  set num of UDP connections to server (default: 1)
   --mtu value                   set maximum transmission unit of UDP packets (default: 1350)
   --sndwnd value                set send window size(num of packets) (default: 128)
   --rcvwnd value                set receive window size(num of packets) (default: 1024)
   --datashard value             set reed-solomon erasure coding - datashard (default: 10)
   --parityshard value           set reed-solomon erasure coding - parityshard (default: 3)
   --dscp value                  set DSCP(6bit) (default: 0)
   --nocomp                      disable compression
   --help, -h                    show help
   --version, -v                 print the version


$ ./server_darwin_amd64 -h
NAME:
   kcptun - kcptun server

USAGE:
   server_darwin_amd64 [global options] command [command options] [arguments...]

VERSION:
   20160820

COMMANDS:
     help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --listen value, -l value  kcp server listen address (default: ":29900")
   --target value, -t value  target server address (default: "127.0.0.1:12948")
   --key value               pre-shared secret for client and server (default: "it's a secrect") [$KCPTUN_KEY]
   --crypt value             aes, aes-128, aes-192, salsa20, blowfish, twofish, cast5, 3des, tea, xtea, xor, none (default: "aes")
   --mode value              profiles: fast3, fast2, fast, normal (default: "fast")
   --mtu value               set maximum transmission unit of UDP packets (default: 1350)
   --sndwnd value            set send window size(num of packets) (default: 1024)
   --rcvwnd value            set receive window size(num of packets) (default: 1024)
   --datashard value         set reed-solomon erasure coding - datashard (default: 10)
   --parityshard value       set reed-solomon erasure coding - parityshard (default: 3)
   --dscp value              set DSCP(6bit) (default: 0)
   --nocomp                  disable compression
   --help, -h                show help
   --version, -v             print the version
```

### *参数调整* :lollipop: 
***两端参数必须一致的有:***
* datashard
* parityshard
* nocomp
* key
* crypt

其余为两边可独立设定的参数

*简易自我调优方法*：
> 第一步：同时在两端逐步增大client rcvwnd和server sndwnd;        
> 第二步：尝试下载，观察如果带宽利用率（服务器＋客户端两端都要观察）接近物理带宽则停止，否则跳转到第一步。

***注意：产生大量重传时，一定是窗口偏大了***

*带宽计算公式*：
```
在不丢包的情况下，有最大-rcvwnd 个数据包在网络上正在向你传输，以平均数据包大小avgsize计算，在任意时刻，有：     

		network_cap = rcvwnd*avgsize

数据流向你，这个值再除以ping值(rtt)，等于最大带宽使用量。

		max_bandwidth = network_cap/rtt = rcvwnd*avgsize/rtt
		
举例，设rcvwnd = 1024, avgsize = 1KB, rtt = 400ms，则：

		max_bandwidth = 1024 * 1KB / 400ms = 2.5MB/s ~= 25Mbps
		
（注：以上计算不包括前向纠错的数据量）

前向纠错是最大带宽量的一个固定比例增加：

		max_bandwidth_fec = max_bandwidth*(datashard+parityshard)/datashard

举例，设datashard = 10 , partiyshard = 3，则：

		max_bandwidth_fec = max_bandwidth * (10 + 3) /10 = 1.3*max_bandwidth ＝ 1.3 * 25Mbps = 32.5Mbps
```

### *安全* :lollipop: 
无论你上层如何加密，如果```-crypt none```，那么协议头部都是***明文***的，建议至少采用```-crypt aes-128```加密。

注意: ```-crypt xor``` 也是不安全的，除非你知道你在做什么。

### *内存控制* :lollipop: 
路由器，手机等嵌入式设备通常对内存用量敏感，通过调节环境变量GOGC（例如GOGC=20)后启动client，可以降低内存使用。      
参考：https://blog.golang.org/go15gc

### *流量控制* :lollipop: 
***必要性: 针对流量敏感的服务器，做双保险。***      

> 基本原则: SERVER的发送速率不能超过ADSL下行带宽，否则只会浪费您的服务器带宽。  

在server通过linux tc，可以限制服务器发送带宽。   
举例:  用linux tc限制server发送带宽为32mbit/s: 
```
root@kcptun:~# cat tc.sh
tc qdisc del dev eth0 root
tc qdisc add dev eth0 root handle 1: htb
tc class add dev eth0 parent 1: classid 1:1 htb rate 32mbit
tc filter add dev eth0 protocol ip parent 1:0 prio 1 handle 10 fw flowid 1:1
iptables -t mangle -A POSTROUTING -o eth0  -j MARK --set-mark 10
root@kcptun:~#
```
其中eth0为网卡，有些服务器为ens3，有些为p2p1，通过ifconfig查询修改。


### *DSCP* :lollipop: 
DSCP差分服务代码点（Differentiated Services Code Point），IETF于1998年12月发布了Diff-Serv（Differentiated Service）的QoS分类标准。它在每个数据包IP头部的服务类别TOS标识字节中，利用已使用的6比特和未使用的2比特，通过编码值来区分优先级。     
常用DSCP值可以参考[Wikipedia DSCP](https://en.wikipedia.org/wiki/Differentiated_services#Commonly_used_DSCP_values)，至于有没有用，完全取决于数据包经过的设备。 

通过 ```-dscp ``` 参数指定dscp值，两端可分别设定。

### *前向纠错* :lollipop: 
前向纠错采用Reed Solomon纠删码, 它的基本原理如下： 给定n个数据块d1, d2,…, dn，n和一个正整数m， RS根据n个数据块生成m个校验块， c1, c2,…, cm。 对于任意的n和m， 从n个原始数据块和m 个校验块中任取n块就能解码出原始数据， 即RS最多容忍m个数据块或者校验块同时丢失。

![reed-solomon](rs.png)

通过参数```-datashard 10 -parityshard 3``` 在两端同时设定。

### *Snappy数据流压缩* :lollipop: 
> Snappy is a compression/decompression library. It does not aim for maximum
> compression, or compatibility with any other compression library; instead,
> it aims for very high speeds and reasonable compression. For instance,
> compared to the fastest mode of zlib, Snappy is an order of magnitude faster
> for most inputs, but the resulting compressed files are anywhere from 20% to
> 100% bigger.

> Reference: http://google.github.io/snappy/

通过参数 ```-nocomp``` 在两端同时设定以关闭压缩。
> 提示: 关闭压缩可能会降低延迟。

### *内置模式* :lollipop: 
响应速度:     
*fast3 >* ***[fast2]*** *> fast > normal > default*        
有效载荷比:     
*default > normal > fast >* ***[fast2]*** *> fast3*       
中间mode参数比较均衡，总之就是越快越浪费带宽，推荐模式 ***fast2***         
更高级的 ***手动档*** 需要理解KCP协议，并通过 ***隐藏参数*** 调整，例如:
```
 -mode manual -nodelay 1 -resend 2 -nc 1 -interval 20
```
高丢包率的网络建议采用fast2, 低丢包率的网络，建议采用normal。

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

使用```kill -SIGUSR1 pid``` 可以在控制台打印出SNMP信息，通常用于精细调整***当前链路的有效载荷比***。        
观察```RetransSegs,FastRetransSegs,LostSegs,OutSegs```这几者的数值比例，用于参考调整```-mode manual,fec```的参数。        

### *故障排除* :lollipop:
> Q: 客户端和服务器端***皆无*** ```stream opened```信息。       
> A: 连接客户端程序的端口设置错误。     

> Q: 客户端有 ```stream opened```信息，服务器端没有。     
> A: 连接服务器的端口设置错误，或者被防火墙拦截。     

> Q: 客户端服务器***皆有*** ```stream opened```信息，但无法通信。      
> A: 上层软件的设定错误。     

### *免责申明* :warning:
用户以各种方式使用本软件（包括但不限于修改使用、直接使用、通过第三方使用）的过程中，不得以任何方式利用本软件直接或间接从事违反中国法律、以及社会公德的行为。软件的使用者需对自身行为负责，因使用软件引发的一切纠纷，由使用者承担全部法律及连带责任。作者不承担任何法律及连带责任。       

对免责声明的解释、修改及更新权均属于作者本人所有。

### *捐赠* :dollar:
![donate](donate.png)          

对该项目的捐款将用于[gonet/2](http://gonet2.github.io/)游戏服务器框架的研发。     

```特别感谢: 郑H立, 南D风, Li, 七q, 凌J，昶，Les*ables, Ky*n, 噼**啦, *斌, 小苍** 等，名字已做特殊处理。```

### *参考资料* :paperclip:
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
13. http://www.lartc.org/LARTC-zh_CN.GB2312.pdf -- Linux Advanced Routing & Traffic Control
