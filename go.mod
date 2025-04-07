module github.com/xtaci/kcptun

require (
	github.com/fatih/color v1.18.0
	github.com/golang/snappy v1.0.0
	github.com/pkg/errors v0.9.1
	github.com/urfave/cli v1.22.16
	github.com/xtaci/kcp-go/v5 v5.6.18
	github.com/xtaci/qpp v1.1.18
	github.com/xtaci/smux v1.5.34
	github.com/xtaci/tcpraw v1.2.31
	golang.org/x/crypto v0.37.0
)

require (
	github.com/coreos/go-iptables v0.7.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.5 // indirect
	github.com/google/gopacket v1.1.19 // indirect
	github.com/klauspost/cpuid/v2 v2.2.8 // indirect
	github.com/klauspost/reedsolomon v1.12.4 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/templexxx/cpu v0.1.1 // indirect
	github.com/templexxx/xorsimd v0.4.3 // indirect
	github.com/tjfoc/gmsm v1.4.1 // indirect
	golang.org/x/net v0.36.0 // indirect
	golang.org/x/sys v0.32.0 // indirect
)

//replace github.com/xtaci/tcpraw => /home/xtaci/tcpraw

go 1.22.3
toolchain go1.24.1
