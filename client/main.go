package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
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
	// sec params
	snonce []byte
	cnonce []byte
	pass   []byte
	key    []byte
	crypt  string
	// Finished message
	SFINISHED = []byte("SFINISHED")
	CFINISHED = []byte("CFINISHED")
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
			Value: "localhost:29900",
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
		cli.BoolFlag{
			Name:  "nocomp",
			Usage: "disable compression",
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
	}
	myApp.Action = func(c *cli.Context) error {
		log.Println("version:", VERSION)
		addr, err := net.ResolveTCPAddr("tcp", c.String("localaddr"))
		checkError(err)
		listener, err := net.ListenTCP("tcp", addr)
		checkError(err)

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

		crypt := c.String("crypt")

		remoteaddr := c.String("remoteaddr")
		datashard, parityshard := c.Int("datashard"), c.Int("parityshard")
		mtu, sndwnd, rcvwnd := c.Int("mtu"), c.Int("sndwnd"), c.Int("rcvwnd")
		nocomp, acknodelay := c.Bool("nocomp"), c.Bool("acknodelay")
		dscp, sockbuf, keepalive, conn := c.Int("dscp"), c.Int("sockbuf"), c.Int("keepalive"), c.Int("conn")

		log.Println("listening on:", listener.Addr())
		log.Println("encryption:", crypt)
		log.Println("nodelay parameters:", nodelay, interval, resend, nc)
		log.Println("remote address:", remoteaddr)
		log.Println("sndwnd:", sndwnd, "rcvwnd:", rcvwnd)
		log.Println("compression:", !nocomp)
		log.Println("mtu:", mtu)
		log.Println("datashard:", datashard, "parityshard:", parityshard)
		log.Println("acknodelay:", acknodelay)
		log.Println("dscp:", dscp)
		log.Println("sockbuf:", sockbuf)
		log.Println("keepalive:", keepalive)
		log.Println("conn:", conn)

		config := &yamux.Config{
			AcceptBacklog:          256,
			EnableKeepAlive:        true,
			KeepAliveInterval:      30 * time.Second,
			ConnectionWriteTimeout: 30 * time.Second,
			MaxStreamWindowSize:    uint32(sockbuf),
			LogOutput:              os.Stderr,
		}
		key = []byte(c.String("key"))
		crypt = c.String("crypt")
		createConn := func() (*yamux.Session, *kcp.UDPSession) {
			kcpconn, err := kcp.DialWithOptions(remoteaddr, nil, datashard, parityshard)
			checkError(err)
			kcpconn.SetStreamMode(true)
			kcpconn.SetNoDelay(nodelay, interval, resend, nc)
			kcpconn.SetWindowSize(sndwnd, rcvwnd)
			kcpconn.SetMtu(mtu)
			kcpconn.SetACKNoDelay(acknodelay)
			kcpconn.SetKeepAlive(keepalive)

			if err := kcpconn.SetDSCP(dscp); err != nil {
				log.Println("SetDSCP:", err)
			}
			if err := kcpconn.SetReadBuffer(sockbuf); err != nil {
				log.Println("SetReadBuffer:", err)
			}
			if err := kcpconn.SetWriteBuffer(sockbuf); err != nil {
				log.Println("SetWriteBuffer:", err)
			}

			// stream multiplex
			var session *yamux.Session
			if nocomp {
				session, err = yamux.Client(kcpconn, config)
			} else {
				session, err = yamux.Client(newCompStream(kcpconn), config)
			}
			checkError(err)
			return session, kcpconn
		}

		numconn := uint16(conn)
		var muxes []*yamux.Session
		var sessions []*kcp.UDPSession
		for i := uint16(0); i < numconn; i++ {
			mux, kcpSession := createConn()
			muxes = append(muxes, mux)
			sessions = append(sessions, kcpSession)
		}

		rr := uint16(0)
		for {
			p1, err := listener.AcceptTCP()
			if err := p1.SetReadBuffer(sockbuf); err != nil {
				log.Println("TCP SetReadBuffer:", err)
			}
			if err := p1.SetWriteBuffer(sockbuf); err != nil {
				log.Println("TCP SetWriteBuffer:", err)
			}
			checkError(err)
			mux := muxes[rr%numconn]
			session := sessions[rr%numconn]
			p2, err := mux.Open()
			if err != nil { // yamux failure
				log.Println(err)
				p1.Close()
				muxes[rr%numconn], sessions[rr%numconn] = createConn()
				mux.Close()
				continue
			}
			if !session.Established() {
				handshake(p2, session)
			}
			go handleClient(p1, p2)
			rr++
		}
	}
	myApp.Run(os.Args)
}

func handshake(conn io.ReadWriteCloser, session *kcp.UDPSession) {
	// handshake
	cnonce := make([]byte, 16)
	snonce := make([]byte, 16)
	rand.Read(cnonce)
	conn.Write(cnonce)
	log.Printf("cnonce: %x", cnonce)

	conn.Read(snonce)
	log.Printf("snonce: %x", snonce)

	pass := pbkdf2.Key(key, append(snonce, cnonce...), 4096, 32, sha1.New)
	log.Printf("pass: %x", pass)
	var block kcp.BlockCrypt
	switch crypt {
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
		crypt = "aes"
		block, _ = kcp.NewAESBlockCrypt(pass)
	}
	mac := hmac.New(sha256.New, pass)
	mac.Write(CFINISHED)
	cfinished := mac.Sum(nil)
	log.Printf("Writing cfinished...")
	conn.Write(cfinished)
	log.Printf("Write cfinished: %x", cfinished)

	sfinished := make([]byte, sha256.Size)
	log.Printf("Reading sfinished...")
	conn.Read(sfinished)
	log.Printf("Read sfinished: %x", sfinished)

	mac.Reset()
	mac.Write(SFINISHED)
	if !hmac.Equal(mac.Sum(nil), sfinished) {
		log.Println("hmac wrong")
		return
	}

	session.SetBlock(block)
	log.Printf("SetBlock: %x", block)
}
