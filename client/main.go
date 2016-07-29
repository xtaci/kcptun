package main

import (
	"crypto/sha1"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"time"

	"golang.org/x/crypto/pbkdf2"

	"github.com/golang/snappy"
	"github.com/hashicorp/yamux"
	"github.com/urfave/cli"
	"github.com/xtaci/kcp-go"
)

var (
	// VERSION is injected by buildflags
	VERSION = "SELFBUILD"
	// SALT is use for pbkdf2 key expansion
	SALT = "kcp-go"
)

type compStream struct {
	conn net.Conn
	w    *snappy.Writer
	r    *snappy.Reader
}

func (c *compStream) Read(p []byte) (n int, err error) {
	return c.r.Read(p)
}

func (c *compStream) Write(p []byte) (n int, err error) {
	n, err = c.w.Write(p)
	err = c.w.Flush()
	return n, err
}

func (c *compStream) Close() error {
	return c.conn.Close()
}

func newCompStream(conn net.Conn) *compStream {
	c := new(compStream)
	c.conn = conn
	c.w = snappy.NewBufferedWriter(conn)
	c.r = snappy.NewReader(conn)
	return c
}

func handleClient(p1, p2 io.ReadWriteCloser) {
	log.Println("stream opened")
	defer log.Println("stream closed")
	defer p1.Close()
	defer p2.Close()

	// start tunnel
	p1die := make(chan struct{})
	go func() {
		io.Copy(p1, p2)
		close(p1die)
	}()

	p2die := make(chan struct{})
	go func() {
		io.Copy(p2, p1)
		close(p2die)
	}()

	// wait for tunnel termination
	select {
	case <-p1die:
	case <-p2die:
	}
}

func checkError(err error) {
	if err != nil {
		log.Println(err)
		os.Exit(-1)
	}
}

func main() {
	rand.Seed(int64(time.Now().Nanosecond()))
	myApp := cli.NewApp()
	myApp.Name = "kcptun"
	myApp.Usage = "kcptun client"
	myApp.Version = VERSION
	myApp.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "localaddr,l",
			Value: ":12948",
			Usage: "local listen address",
		},
		cli.StringFlag{
			Name:  "remoteaddr, r",
			Value: "vps:29900",
			Usage: "kcp server address",
		},
		cli.StringFlag{
			Name:   "key",
			Value:  "it's a secrect",
			Usage:  "key for communcation, must be the same as kcptun server",
			EnvVar: "KCPTUN_KEY",
		},
		cli.StringFlag{
			Name:  "crypt",
			Value: "aes",
			Usage: "methods for encryption: aes, tea, xor, none",
		},
		cli.StringFlag{
			Name:  "mode",
			Value: "fast",
			Usage: "mode for communication: fast3, fast2, fast, normal",
		},
		cli.IntFlag{
			Name:  "conn",
			Value: 1,
			Usage: "establish N physical connections as specified by 'conn' to server",
		},
		cli.IntFlag{
			Name:  "mtu",
			Value: 1350,
			Usage: "set MTU of UDP packets, suggest 'tracepath' to discover path mtu",
		},
		cli.IntFlag{
			Name:  "sndwnd",
			Value: 128,
			Usage: "set send window size(num of packets)",
		},
		cli.IntFlag{
			Name:  "rcvwnd",
			Value: 1024,
			Usage: "set receive window size(num of packets)",
		},
		cli.BoolFlag{
			Name:  "nocomp",
			Usage: "disable compression",
		},
		cli.IntFlag{
			Name:  "datashard",
			Value: 10,
			Usage: "set reed-solomon erasure coding - datashard",
		},
		cli.IntFlag{
			Name:  "parityshard",
			Value: 3,
			Usage: "set reed-solomon erasure coding - parityshard",
		},
		cli.BoolFlag{
			Name:   "acknodelay",
			Usage:  "flush ack immediately when a packet is received",
			Hidden: true,
		},
		cli.IntFlag{
			Name:  "dscp",
			Value: 0,
			Usage: "set DSCP(6bit)",
		},
		cli.IntFlag{
			Name:   "nodelay",
			Value:  0,
			Hidden: true,
		},
		cli.IntFlag{
			Name:   "interval",
			Value:  40,
			Hidden: true,
		},
		cli.IntFlag{
			Name:   "resend",
			Value:  0,
			Hidden: true,
		},
		cli.IntFlag{
			Name:   "nc",
			Value:  0,
			Hidden: true,
		},
	}
	myApp.Action = func(c *cli.Context) error {
		log.Println("version:", VERSION)
		addr, err := net.ResolveTCPAddr("tcp", c.String("localaddr"))
		checkError(err)
		listener, err := net.ListenTCP("tcp", addr)
		checkError(err)
		pass := pbkdf2.Key([]byte(c.String("key")), []byte(SALT), 4096, 32, sha1.New)

		// kcp server
		nodelay, interval, resend, nc := c.Int("nodelay"), c.Int("interval"), c.Int("resend"), c.Int("nc")

		switch c.String("mode") {
		case "normal":
			nodelay, interval, resend, nc = 0, 30, 2, 1
		case "fast":
			nodelay, interval, resend, nc = 0, 20, 2, 1
		case "fast2":
			nodelay, interval, resend, nc = 1, 20, 2, 1
		case "fast3":
			nodelay, interval, resend, nc = 1, 10, 2, 1
		}

		log.Println("listening on:", listener.Addr())
		log.Println("encryption:", c.String("crypt"))
		log.Println("nodelay parameters:", nodelay, interval, resend, nc)
		log.Println("remote address:", c.String("remoteaddr"))
		log.Println("sndwnd:", c.Int("sndwnd"), "rcvwnd:", c.Int("rcvwnd"))
		log.Println("compression:", !c.Bool("nocomp"))
		log.Println("mtu:", c.Int("mtu"))
		log.Println("datashard:", c.Int("datashard"), "parityshard:", c.Int("parityshard"))
		log.Println("acknodelay:", c.Bool("acknodelay"))
		log.Println("dscp:", c.Int("dscp"))
		log.Println("conn:", c.Int("conn"))

		createConn := func() *yamux.Session {
			var block kcp.BlockCrypt
			switch c.String("crypt") {
			case "tea":
				block, _ = kcp.NewTEABlockCrypt(pass[:16])
			case "xor":
				block, _ = kcp.NewSimpleXORBlockCrypt(pass)
			case "none":
				block, _ = kcp.NewNoneBlockCrypt(pass)
			default:
				block, _ = kcp.NewAESBlockCrypt(pass)
			}
			kcpconn, err := kcp.DialWithOptions(c.String("remoteaddr"), block, c.Int("datashard"), c.Int("parityshard"))
			checkError(err)
			kcpconn.SetStreamMode(true)
			kcpconn.SetNoDelay(nodelay, interval, resend, nc)
			kcpconn.SetWindowSize(c.Int("sndwnd"), c.Int("rcvwnd"))
			kcpconn.SetMtu(c.Int("mtu"))
			kcpconn.SetACKNoDelay(c.Bool("acknodelay"))
			kcpconn.SetDSCP(c.Int("dscp"))

			// stream multiplex
			config := &yamux.Config{
				AcceptBacklog:          256,
				EnableKeepAlive:        true,
				KeepAliveInterval:      30 * time.Second,
				ConnectionWriteTimeout: 30 * time.Second,
				MaxStreamWindowSize:    16777216,
				LogOutput:              os.Stderr,
			}
			var session *yamux.Session
			if c.Bool("nocomp") {
				session, err = yamux.Client(kcpconn, config)
			} else {
				session, err = yamux.Client(newCompStream(kcpconn), config)
			}
			checkError(err)
			return session
		}

		numconn := uint16(c.Int("conn"))
		var muxes []*yamux.Session
		for i := uint16(0); i < numconn; i++ {
			muxes = append(muxes, createConn())
		}

		rr := uint16(0)
		for {
			p1, err := listener.AcceptTCP()
			checkError(err)
			mux := muxes[rr%numconn]
			p2, err := mux.Open()
			if err != nil { // yamux failure
				log.Println(err)
				p1.Close()
				mux.Close()
				muxes[rr%numconn] = createConn()
				continue
			}
			go handleClient(p1, p2)
			rr++
		}
	}
	myApp.Run(os.Args)
}
