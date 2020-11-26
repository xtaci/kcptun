<img src="smux.png" alt="smux" height="35px" />

[![GoDoc][1]][2] [![MIT licensed][3]][4] [![Build Status][5]][6] [![Go Report Card][7]][8] [![Coverage Statusd][9]][10] [![Sourcegraph][11]][12]

<img src="mux.jpg" alt="smux" height="120px" /> 

[1]: https://godoc.org/github.com/xtaci/smux?status.svg
[2]: https://godoc.org/github.com/xtaci/smux
[3]: https://img.shields.io/badge/license-MIT-blue.svg
[4]: LICENSE
[5]: https://travis-ci.org/xtaci/smux.svg?branch=master
[6]: https://travis-ci.org/xtaci/smux
[7]: https://goreportcard.com/badge/github.com/xtaci/smux
[8]: https://goreportcard.com/report/github.com/xtaci/smux
[9]: https://codecov.io/gh/xtaci/smux/branch/master/graph/badge.svg
[10]: https://codecov.io/gh/xtaci/smux
[11]: https://sourcegraph.com/github.com/xtaci/smux/-/badge.svg
[12]: https://sourcegraph.com/github.com/xtaci/smux?badge

## Introduction

Smux ( **S**imple **MU**ltiple**X**ing) is a multiplexing library for Golang. It relies on an underlying connection to provide reliability and ordering, such as TCP or [KCP](https://github.com/xtaci/kcp-go), and provides stream-oriented multiplexing. The original intention of this library is to power the connection management for [kcp-go](https://github.com/xtaci/kcp-go).

## Features

1. ***Token bucket*** controlled receiving, which provides smoother bandwidth graph(see picture below).
2. Session-wide receive buffer, shared among streams, **fully controlled** overall memory usage.
3. Minimized header(8Bytes), maximized payload. 
4. Well-tested on millions of devices in [kcptun](https://github.com/xtaci/kcptun).
5. Builtin fair queue traffic shaping.
6. Per-stream sliding window to control congestion.(protocol version 2+).

![smooth bandwidth curve](curve.jpg)

## Documentation

For complete documentation, see the associated [Godoc](https://godoc.org/github.com/xtaci/smux).

## Benchmark
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
VERSION(1B) | CMD(1B) | LENGTH(2B) | STREAMID(4B) | DATA(LENGTH)  

VALUES FOR LATEST VERSION:
VERSION:
    1/2
    
CMD:
    cmdSYN(0)
    cmdFIN(1)
    cmdPSH(2)
    cmdNOP(3)
    cmdUPD(4)	// only supported on version 2
    
STREAMID:
    client use odd numbers starts from 1
    server use even numbers starts from 0
    
cmdUPD:
    | CONSUMED(4B) | WINDOW(4B) |
```

## Usage

```go

func client() {
    // Get a TCP connection
    conn, err := net.Dial(...)
    if err != nil {
        panic(err)
    }

    // Setup client side of smux
    session, err := smux.Client(conn, nil)
    if err != nil {
        panic(err)
    }

    // Open a new stream
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

    // Setup server side of smux
    session, err := smux.Server(conn, nil)
    if err != nil {
        panic(err)
    }

    // Accept a stream
    stream, err := session.AcceptStream()
    if err != nil {
        panic(err)
    }

    // Listen for a message
    buf := make([]byte, 4)
    stream.Read(buf)
    stream.Close()
    session.Close()
}

```

## Status

Stable
