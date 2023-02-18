# tcpraw

[![GoDoc][1]][2] [![Build Status][3]][4] [![Go Report Card][5]][6] [![Coverage Statusd][7]][8] [![MIT licensed][9]][10] 

[1]: https://godoc.org/github.com/xtaci/tcpraw?status.svg
[2]: https://godoc.org/github.com/xtaci/tcpraw
[3]: https://travis-ci.org/xtaci/tcpraw.svg?branch=master
[4]: https://travis-ci.org/xtaci/tcpraw
[5]: https://goreportcard.com/badge/github.com/xtaci/tcpraw
[6]: https://goreportcard.com/report/github.com/xtaci/tcpraw
[7]: https://codecov.io/gh/xtaci/tcpraw/branch/master/graph/badge.svg
[8]: https://codecov.io/gh/xtaci/tcpraw
[9]: https://img.shields.io/badge/license-MIT-blue.svg
[10]: LICENSE



# Introduction

A packet-oriented connection by simulating TCP protocol

## Features

0. Tiny
1. Support IPv4 and IPv6.
2. Realistic sliding window, NAT friendly.
3. Pure golang without cgo, available on all architecture.

## Documentation

For complete documentation, see the associated [Godoc](https://godoc.org/github.com/xtaci/tcpraw).


## Benchmark

```
goos: linux
goarch: amd64
pkg: github.com/xtaci/tcpraw
BenchmarkEcho-2   	   20000	     93036 ns/op	  11.01 MB/s	    6200 B/op	      62 allocs/op
PASS
ok  	github.com/xtaci/tcpraw	2.758s
```

## Status

Stable

## Who is using this

https://github.com/xtaci/kcptun
