package main

import (
	"crypto/sha1"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"runtime"
	"time"

	"golang.org/x/crypto/pbkdf2"

	"github.com/golang/snappy"
	"github.com/urfave/cli"
	kcp "github.com/xtaci/kcp-go"
	"github.com/xtaci/yamux"
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
		log.Printf("%+v\n", err)
		os.Exit(-1)
	}
}

func main() {
	rand.Seed(int64(time.Now().Nanosecond()))
	if VERSION == "SELFBUILD" {
		// add more log flags for debugging
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	}
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
			Usage:  "pre-shared secret between client and server",
			EnvVar: "KCPTUN_KEY",
		},
		cli.StringFlag{
			Name:  "crypt",
			Value: "aes",
			Usage: "aes, aes-128, aes-192, salsa20, blowfish, twofish, cast5, 3des, tea, xtea, xor, none",
		},
		cli.StringFlag{
			Name:  "mode",
			Value: "fast",
			Usage: "profiles: fast3, fast2, fast, normal",
		},
		cli.IntFlag{
			Name:  "conn",
			Value: 1,
			Usage: "set num of UDP connections to server",
		},
		cli.IntFlag{
			Name:  "autoexpire",
			Value: 0,
			Usage: "set auto expiration time(in seconds) for a single UDP connection, 0 to disable",
		},
		cli.IntFlag{
			Name:  "mtu",
			Value: 1350,
			Usage: "set maximum transmission unit for UDP packets",
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
		cli.IntFlag{
			Name:  "dscp",
			Value: 0,
			Usage: "set DSCP(6bit)",
		},
		cli.BoolFlag{
			Name:  "nocomp",
			Usage: "disable compression",
		},
		cli.BoolFlag{
			Name:   "acknodelay",
			Usage:  "flush ack immediately when a packet is received",
			Hidden: true,
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
		cli.IntFlag{
			Name:   "sockbuf",
			Value:  4194304, // socket buffer size in bytes
			Hidden: true,
		},
		cli.IntFlag{
			Name:   "keepalive",
			Value:  10, // nat keepalive interval in seconds
			Hidden: true,
		},
		cli.StringFlag{
			Name: "c",
		},
	}
	myApp.Action = func(c *cli.Context) error {
		config := Config{}
		config.LocalAddr = c.String("localaddr")
		config.RemoteAddr = c.String("remoteaddr")
		config.Key = c.String("key")
		config.Crypt = c.String("crypt")
		config.Mode = c.String("mode")
		config.Conn = c.Int("conn")
		config.AutoExpire = c.Int("autoexpire")
		config.MTU = c.Int("mtu")
		config.SndWnd = c.Int("sndwnd")
		config.RcvWnd = c.Int("rcvwnd")
		config.DataShard = c.Int("datashard")
		config.ParityShard = c.Int("parityshard")
		config.DSCP = c.Int("dscp")
		config.NoComp = c.Bool("nocomp")
		config.AckNodelay = c.Bool("acknodelay")
		config.NoDelay = c.Int("nodelay")
		config.Interval = c.Int("interval")
		config.Resend = c.Int("resend")
		config.NoCongestion = c.Int("nc")
		config.SockBuf = c.Int("sockbuf")
		config.KeepAlive = c.Int("keepalive")

		//Now only support json config file
		parseJsonConfig(&config, c.String("c"))

		switch config.Mode {
		case "normal":
			config.NoDelay, config.Interval, config.Resend, config.NoCongestion = 0, 30, 2, 1
		case "fast":
			config.NoDelay, config.Interval, config.Resend, config.NoCongestion = 0, 20, 2, 1
		case "fast2":
			config.NoDelay, config.Interval, config.Resend, config.NoCongestion = 1, 20, 2, 1
		case "fast3":
			config.NoDelay, config.Interval, config.Resend, config.NoCongestion = 1, 10, 2, 1
		}

		log.Println("version:", VERSION)
		addr, err := net.ResolveTCPAddr("tcp", config.LocalAddr)
		checkError(err)
		listener, err := net.ListenTCP("tcp", addr)
		checkError(err)

		pass := pbkdf2.Key([]byte(config.Key), []byte(SALT), 4096, 32, sha1.New)
		var block kcp.BlockCrypt
		switch config.Crypt {
		case "tea":
			block, _ = kcp.NewTEABlockCrypt(pass[:16])
		case "xor":
			block, _ = kcp.NewSimpleXORBlockCrypt(pass)
		case "none":
			block, _ = kcp.NewNoneBlockCrypt(pass)
		case "aes-128":
			block, _ = kcp.NewAESBlockCrypt(pass[:16])
		case "aes-192":
			block, _ = kcp.NewAESBlockCrypt(pass[:24])
		case "blowfish":
			block, _ = kcp.NewBlowfishBlockCrypt(pass)
		case "twofish":
			block, _ = kcp.NewTwofishBlockCrypt(pass)
		case "cast5":
			block, _ = kcp.NewCast5BlockCrypt(pass[:16])
		case "3des":
			block, _ = kcp.NewTripleDESBlockCrypt(pass[:24])
		case "xtea":
			block, _ = kcp.NewXTEABlockCrypt(pass[:16])
		case "salsa20":
			block, _ = kcp.NewSalsa20BlockCrypt(pass)
		default:
			config.Crypt = "aes"
			block, _ = kcp.NewAESBlockCrypt(pass)
		}

		log.Println("listening on:", listener.Addr())
		log.Println("encryption:", config.Crypt)
		log.Println("nodelay parameters:", config.NoDelay, config.Interval, config.Resend, config.NoCongestion)
		log.Println("remote address:", config.RemoteAddr)
		log.Println("sndwnd:", config.SndWnd, "rcvwnd:", config.RcvWnd)
		log.Println("compression:", !config.NoComp)
		log.Println("mtu:", config.MTU)
		log.Println("datashard:", config.DataShard, "parityshard:", config.ParityShard)
		log.Println("acknodelay:", config.AckNodelay)
		log.Println("dscp:", config.DSCP)
		log.Println("sockbuf:", config.SockBuf)
		log.Println("keepalive:", config.KeepAlive)
		log.Println("conn:", config.Conn)
		log.Println("autoexpire:", config.AutoExpire)

		yconfig := &yamux.Config{
			AcceptBacklog:          256,
			EnableKeepAlive:        true,
			KeepAliveInterval:      30 * time.Second,
			ConnectionWriteTimeout: 10 * time.Second,
			MaxStreamWindowSize:    uint32(config.SockBuf),
			LogOutput:              os.Stderr,
		}
		createConn := func() *yamux.Session {
			kcpconn, err := kcp.DialWithOptions(config.RemoteAddr, block, config.DataShard, config.ParityShard)
			checkError(err)
			kcpconn.SetStreamMode(true)
			kcpconn.SetNoDelay(config.NoDelay, config.Interval, config.Resend, config.NoCongestion)
			kcpconn.SetWindowSize(config.SndWnd, config.RcvWnd)
			kcpconn.SetMtu(config.MTU)
			kcpconn.SetACKNoDelay(config.AckNodelay)
			kcpconn.SetKeepAlive(config.KeepAlive)

			if err := kcpconn.SetDSCP(config.DSCP); err != nil {
				log.Println("SetDSCP:", err)
			}
			if err := kcpconn.SetReadBuffer(config.SockBuf); err != nil {
				log.Println("SetReadBuffer:", err)
			}
			if err := kcpconn.SetWriteBuffer(config.SockBuf); err != nil {
				log.Println("SetWriteBuffer:", err)
			}

			// stream multiplex
			var session *yamux.Session
			if config.NoComp {
				session, err = yamux.Client(kcpconn, yconfig)
			} else {
				session, err = yamux.Client(newCompStream(kcpconn), yconfig)
			}
			checkError(err)
			runtime.SetFinalizer(session, func(s *yamux.Session) {
				s.Close()
			})
			return session
		}

		numconn := uint16(config.Conn)
		muxes := make([]struct {
			session *yamux.Session
			ttl     time.Time
		}, numconn)

		for k := range muxes {
			muxes[k].session = createConn()
			muxes[k].ttl = time.Now().Add(time.Duration(config.AutoExpire) * time.Second)
		}

		rr := uint16(0)
		for {
			p1, err := listener.AcceptTCP()
			if err := p1.SetReadBuffer(config.SockBuf); err != nil {
				log.Println("TCP SetReadBuffer:", err)
			}
			if err := p1.SetWriteBuffer(config.SockBuf); err != nil {
				log.Println("TCP SetWriteBuffer:", err)
			}
			checkError(err)
			idx := rr % numconn

		OPEN_P2:
			// do auto expiration
			if config.AutoExpire > 0 && time.Now().After(muxes[idx].ttl) {
				log.Println("autoexpired")
				muxes[idx].session = createConn()
				muxes[idx].ttl = time.Now().Add(time.Duration(config.AutoExpire) * time.Second)
			}

			// do session open
			p2, err := muxes[idx].session.Open()
			if err != nil { // yamux failure
				muxes[idx].session = createConn()
				muxes[idx].ttl = time.Now().Add(time.Duration(config.AutoExpire) * time.Second)
				goto OPEN_P2
			}
			go handleClient(p1, p2)
			rr++
		}
	}
	myApp.Run(os.Args)
}
