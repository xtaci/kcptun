module github.com/xtaci/kcptun

require (
	github.com/fatih/color v1.19.0
	github.com/golang/snappy v1.0.0
	github.com/pkg/errors v0.9.1
	github.com/urfave/cli v1.22.17
	github.com/xtaci/kcp-go/v5 v5.6.71
	github.com/xtaci/qpp v1.1.25
	github.com/xtaci/smux v1.5.57
	github.com/xtaci/tcpraw v1.2.32
	golang.org/x/crypto v0.49.0
)

require (
	github.com/coreos/go-iptables v0.8.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.7 // indirect
	github.com/google/gopacket v1.1.19 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/klauspost/reedsolomon v1.13.3 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/tjfoc/gmsm v1.4.1 // indirect
	golang.org/x/net v0.52.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/time v0.15.0 // indirect
)

// replace github.com/xtaci/smux => /home/xtaci/go/src/github.com/xtaci/smux
//replace github.com/xtaci/kcp-go/v5 => /home/xtaci/go/src/github.com/xtaci/kcp-go

go 1.25.0
