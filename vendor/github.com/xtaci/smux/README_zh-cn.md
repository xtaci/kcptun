<img src="assets/smux.png" alt="smux" height="35px" />

[![GoDoc][1]][2] [![MIT licensed][3]][4] [![Build Status][5]][6] [![Go Report Card][7]][8] [![Coverage Statusd][9]][10] [![Sourcegraph][11]][12]

<img src="assets/mux.jpg" alt="smux" height="120px" />

[1]: https://godoc.org/github.com/xtaci/smux?status.svg
[2]: https://godoc.org/github.com/xtaci/smux
[3]: https://img.shields.io/badge/license-MIT-blue.svg
[4]: LICENSE
[5]: https://img.shields.io/github/created-at/xtaci/smux
[6]: https://img.shields.io/github/created-at/xtaci/smux
[7]: https://goreportcard.com/badge/github.com/xtaci/smux
[8]: https://goreportcard.com/report/github.com/xtaci/smux
[9]: https://codecov.io/gh/xtaci/smux/branch/master/graph/badge.svg
[10]: https://codecov.io/gh/xtaci/smux
[11]: https://sourcegraph.com/github.com/xtaci/smux/-/badge.svg
[12]: https://sourcegraph.com/github.com/xtaci/smux?badge

[English](README.md) | [中文](README_zh-cn.md)

## 简介

Smux（**S**imple **MU**ltiple**X**ing）是一个用 Golang 实现的多路复用库，让多个有序、可靠的逻辑流共享同一条底层连接（如 TCP 或 [KCP](https://github.com/xtaci/kcp-go)）。它最初为 [kcp-go](https://github.com/xtaci/kcp-go) 设计，用于在复杂网络环境中维持长连接时的精细流量控制和资源管理。

## 特性

1. **令牌桶限速**：基于令牌桶的接收控制，输出带宽曲线更平滑（如下图）。
2. **全局缓冲共享**：会话级接收缓冲在各流之间复用，可精确限制整体内存占用。
3. **极简协议头**：8 字节帧头最大化有效载荷占比。
4. **大规模验证**：在 [kcptun](https://github.com/xtaci/kcptun) 中经数百万设备验证，稳定可靠。
5. **公平队列整形**：内建公平调度，避免单个流独占带宽。
6. **流级滑动窗口**：协议版本 2 起支持 per-stream 拥塞控制，进一步提升吞吐和延迟表现。

![smooth bandwidth curve](assets/curve.jpg)

## 架构

* **Session**：多路复用会话管理器，负责维护底层 `io.ReadWriteCloser`，创建或接受 `Stream`，同时调度共享接收缓冲和限速逻辑。
* **Stream**：会话中的逻辑连接，实现 `net.Conn` 接口，承担读写缓冲与流量控制。
* **Frame**：在线协议帧格式，定义指令、流 ID、长度等字段，用于在 Session 与 Stream 之间传输数据/控制信息。

## 帧内存分配器

`alloc.go` 针对 64 KB 以内的帧实现了分层分配器：17 个 `sync.Pool` 分别缓存 2^n 容量的切片，`msb()` 函数利用 De Bruijn 序列常数在 O(1) 时间内定位到最合适的池子。复用出来的切片不会被额外清零，新帧直接覆盖旧负载，从而避开运行时的 memclr 开销。

这样带来的系统收益包括：

1. 单次分配的浪费率 < 50%，在高并发下也能准确控制会话级缓冲占用。
2. 重复使用固定容量的切片显著降低 GC 压力，也避免了多次清零的额外成本。
3. 常数时间的桶选择避免了搜索或额外锁竞争，让高吞吐会话在大量流同时活跃时保持低尾延迟。

## 文档

更完整的 API 与实现细节可参考 [Godoc](https://godoc.org/github.com/xtaci/smux)。

## 基准测试 (Benchmark)
```
$ go test -v -run=^$ -bench .
goos: darwin
goarch: amd64
pkg: github.com/xtaci/smux
BenchmarkMSB-4               30000000            51.8 ns/op
BenchmarkAcceptClose-4          50000         36783 ns/op
BenchmarkConnSmux-4             30000         58335 ns/op   2246.88 MB/s    1208 B/op     19 allocs/op
BenchmarkConnTCP-4              50000         25579 ns/op   5124.04 MB/s       0 B/op      0 allocs/op
PASS
ok      github.com/xtaci/smux   7.811s
```

## 规范 (Specification)

```
+---------------+---------------+-------------------------------+
 |  VERSION (1B) |    CMD (1B)   |          LENGTH (2B)          |
 +---------------+---------------+-------------------------------+
 |                          STREAMID (4B)                        |
 +---------------------------------------------------------------+
 |                                                               |
 /                        DATA (Variable)                        /
 |                                                               |
 +---------------------------------------------------------------+

VALUES FOR LATEST VERSION:
VERSION:
    1/2

CMD:
    cmdSYN(0)
    cmdFIN(1)
    cmdPSH(2)
    cmdNOP(3)
    cmdUPD(4)    // 仅在版本 2 支持

STREAMID:
    客户端使用从 1 开始的奇数
    服务端使用从 0 开始的偶数

cmdUPD:
    | CONSUMED(4B) | WINDOW(4B) |
```

## 用法 (Usage)

```go

func client() {
    // 建立一条 TCP 连接
    conn, err := net.Dial(...)
    if err != nil {
        panic(err)
    }

    // 初始化 smux 客户端，会采用默认配置
    session, err := smux.Client(conn, nil)
    if err != nil {
        panic(err)
    }

    // 打开一个新的逻辑流
    stream, err := session.OpenStream()
    if err != nil {
        panic(err)
    }

    // Stream 满足 io.ReadWriteCloser，可直接读写
    stream.Write([]byte("ping"))
    stream.Close()
    session.Close()
}

func server() {
    // 接收传入的 TCP 连接
    conn, err := listener.Accept()
    if err != nil {
        panic(err)
    }

    // 使用 smux.Server 将连接升级为服务端会话
    session, err := smux.Server(conn, nil)
    if err != nil {
        panic(err)
    }

    // 阻塞等待客户端打开的流
    stream, err := session.AcceptStream()
    if err != nil {
        panic(err)
    }

    // 简单读取一条 4 字节消息
    buf := make([]byte, 4)
    stream.Read(buf)
    stream.Close()
    session.Close()
}

```

## 配置

`smux.Config` 提供了常用调优项：

* `Version`：协议版本（1 或 2）。
* `KeepAliveInterval`：发送 `cmdNOP` 以维持心跳的间隔。
* `KeepAliveTimeout`：在无数据时认为连接失效的超时时间。
* `MaxFrameSize`：单帧数据的最大长度。
* `MaxReceiveBuffer`：会话级共享接收缓冲的上限。
* `MaxStreamBuffer`：单个流本地缓冲的上限。

## 参考

* [hashicorp/yamux](https://github.com/hashicorp/yamux)
* [xtaci/kcp-go](https://github.com/xtaci/kcp-go)
* [xtaci/kcptun](https://github.com/xtaci/kcptun)
