<img src="assets/smux.png" alt="smux" height="35px" />

[![GoDoc][1]][2] [![MIT licensed][3]][4] [![Created At][5]][6] [![Coverage][9]][10]

<img src="assets/mux.jpg" alt="smux" height="120px" /> 

[1]: https://godoc.org/github.com/xtaci/smux?status.svg
[2]: https://godoc.org/github.com/xtaci/smux
[3]: https://img.shields.io/badge/license-MIT-blue.svg
[4]: LICENSE
[5]: https://img.shields.io/github/created-at/xtaci/smux
[6]: https://github.com/xtaci/smux
[9]: https://codecov.io/gh/xtaci/smux/branch/master/graph/badge.svg
[10]: https://codecov.io/gh/xtaci/smux
[English](README.md)

## Introduction

Smux (**S**imple **MU**ltiple**X**ing) is a stream-multiplexing library for Go. It lets multiple logical streams share a single underlying connection, relying on that connection—such as TCP or [KCP](https://github.com/xtaci/kcp-go)—to provide reliable, ordered delivery. Smux was originally developed to manage connections in [kcp-go](https://github.com/xtaci/kcp-go).

## Features

1. Token-bucket-based receive control helps smooth bandwidth usage, as illustrated below.
2. A shared, session-wide receive buffer bounds memory usage across streams.
3. An 8-byte frame header leaves more space for payload.
4. Built-in fair-queue traffic shaping.
5. Per-stream sliding-window flow control in protocol version 2.

![smooth bandwidth curve](assets/curve.jpg)

## Architecture

* **Session**: Manages the underlying `io.ReadWriteCloser`, stream creation and acceptance, and the shared receive buffer.
* **Stream**: Represents a logical connection within a session and implements `net.Conn`, including buffering and flow control.
* **Frame**: Defines the wire format used to carry stream data and control messages.

## Frame Allocator

`alloc.go` implements a slab-style allocator for frames up to 64 KiB. Seventeen `sync.Pool` buckets hold slices with power-of-two capacities. A De Bruijn-based `msb()` lookup selects the smallest bucket that can satisfy a request in constant time. Reused buffers are not cleared when returned to the pool; incoming frame data overwrites the bytes that are read.

This design helps sessions by:

1. Limiting capacity waste to less than 50% for each allocation.
2. Reusing pre-sized slices to reduce allocation and garbage-collection overhead.
3. Selecting a capacity bucket in constant time.

## Documentation

For the complete API documentation, see [Godoc](https://godoc.org/github.com/xtaci/smux).

## Benchmarks

Run the benchmarks with:
```
$ go test -v -run=^$ -bench .
goos: darwin
goarch: amd64
pkg: github.com/xtaci/smux
BenchmarkMSB-4           	30000000	        51.8 ns/op
BenchmarkAcceptClose-4   	   50000	     36783 ns/op
BenchmarkConnSmux-4      	   30000	     58335 ns/op	2246.88 MB/s	    1208 B/op	      19 allocs/op
BenchmarkConnTCP-4       	   50000	     25579 ns/op	5124.04 MB/s	       0 B/op	       0 allocs/op
PASS
ok  	github.com/xtaci/smux	7.811s
```

## Specification

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

Protocol values:
VERSION:
    1/2
    
CMD:
    cmdSYN(0)
    cmdFIN(1)
    cmdPSH(2)
    cmdNOP(3)
    cmdUPD(4)	// only supported on version 2
    
STREAMID:
    client uses odd numbers, starting at 1
    server uses even numbers, starting at 0
    
cmdUPD:
    | CONSUMED(4B) | WINDOW(4B) |
```

## Usage

```go

func client() {
    // Establish a TCP connection
    conn, err := net.Dial(...)
    if err != nil {
        panic(err)
    }

    // Create a client session
    session, err := smux.Client(conn, nil)
    if err != nil {
        panic(err)
    }

    // Open a stream
    stream, err := session.OpenStream()
    if err != nil {
        panic(err)
    }

    // Stream implements io.ReadWriteCloser
    stream.Write([]byte("ping"))
    stream.Close()
    session.Close()
}

func server() {
    // Accept a TCP connection
    conn, err := listener.Accept()
    if err != nil {
        panic(err)
    }

    // Create a server session
    session, err := smux.Server(conn, nil)
    if err != nil {
        panic(err)
    }

    // Accept a stream
    stream, err := session.AcceptStream()
    if err != nil {
        panic(err)
    }

    // Read a message
    buf := make([]byte, 4)
    stream.Read(buf)
    stream.Close()
    session.Close()
}

```

## Configuration

Use `smux.Config` to tune session behavior:

* `Version`: Protocol version (1 or 2).
* `KeepAliveDisabled`: Disables keep-alive messages and timeout checks.
* `KeepAliveInterval`: Time between NOP keep-alive frames.
* `KeepAliveTimeout`: Maximum time without received data before the session is closed.
* `MaxFrameSize`: Maximum frame payload size.
* `MaxReceiveBuffer`: Maximum amount of buffered data across all streams in the session.
* `MaxStreamBuffer`: Per-stream receive-window size in protocol version 2.

## Reference

* [hashicorp/yamux](https://github.com/hashicorp/yamux)
* [xtaci/kcp-go](https://github.com/xtaci/kcp-go)
