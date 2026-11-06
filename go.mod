module github.com/xtaci/kcptun

require (
	github.com/fatih/color v1.19.0
	github.com/golang/snappy v1.0.0
	github.com/pkg/errors v0.9.1
	github.com/urfave/cli v1.22.17
	github.com/xtaci/kcp-go/v5 v5.7.4
	github.com/xtaci/qpp v1.1.25
	github.com/xtaci/smux v2.0.1+incompatible
	github.com/xtaci/tcpraw v1.2.32
	golang.org/x/crypto v0.58.0
)

require (
	github.com/coreos/go-iptables v0.8.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.7 // indirect
	github.com/google/gopacket v1.1.19 // indirect
	github.com/juju/ratelimit v1.0.2 // indirect
	github.com/klauspost/cpuid/v2 v2.4.2 // indirect
	github.com/klauspost/reedsolomon v1.14.2 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/mattn/go-colorable v0.1.16 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/pion/transport/v5 v5.1.4 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/tjfoc/gmsm v1.4.1 // indirect
	golang.org/x/net v0.62.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
)

// replace github.com/xtaci/smux => /home/xtaci/go/src/github.com/xtaci/smux
//replace github.com/xtaci/kcp-go/v5 => /home/xtaci/go/src/github.com/xtaci/kcp-go

go 1.26.0
