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

// handle multiplex-ed connection
func handleMux(conn io.ReadWriteCloser, target string, config *yamux.Config, session *kcp.UDPSession) {
	// stream multiplex
	mux, err := yamux.Server(conn, config)
	if err != nil {
		log.Println(err)
		return
	}
	defer mux.Close()
	for {
		p1, err := mux.Accept()
		if err != nil {
			log.Println(err)
			return
		}
		sockbuf := int(config.MaxStreamWindowSize)
		p2, err := net.DialTimeout("tcp", target, 5*time.Second)
		if err != nil {
			log.Println(err)
			return
		}

		if err := p2.(*net.TCPConn).SetReadBuffer(sockbuf); err != nil {
			log.Println("TCP SetReadBuffer:", err)
		}
		if err := p2.(*net.TCPConn).SetWriteBuffer(sockbuf); err != nil {
			log.Println("TCP SetWriteBuffer:", err)
		}

		if !session.Established() {
			handshake(p1, session)
		}
		go handleClient(p1, p2)
	}
}

func handshake(conn io.ReadWriteCloser, session *kcp.UDPSession) {
	// handshake
	cnonce := make([]byte, 16)
	snonce := make([]byte, 16)
	conn.Read(cnonce)
	log.Printf("cnonce: %x", cnonce)

	rand.Read(snonce)
	pass = pbkdf2.Key(key, append(snonce, cnonce...), 4096, 32, sha1.New)
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
	conn.Write(snonce)
	log.Printf("snonce: %x", snonce)

	cfinished := make([]byte, sha256.Size)
	log.Printf("Reading cfinished...")
	conn.Read(cfinished)
	log.Printf("Read cfinished: %x", cfinished)

	mac := hmac.New(sha256.New, pass)
	mac.Write(CFINISHED)
	if !hmac.Equal(mac.Sum(nil), cfinished) {
		log.Fatalln("hmac wrong")
		return
	}

	mac.Reset()
	mac.Write(SFINISHED)
	sfinished := mac.Sum(nil)
	log.Printf("Writing sfinished...")
	conn.Write(sfinished)
	log.Printf("Write sfinished: %x", sfinished)

	// session.SetBlock(block)
	log.Printf("SetBlock: %x", block)
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

func main() {
	rand.Seed(int64(time.Now().Nanosecond()))
	if VERSION == "SELFBUILD" {
		// add more log flags for debugging
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	}
	myApp := cli.NewApp()
	myApp.Name = "kcptun"
	myApp.Usage = "kcptun server"
	myApp.Version = VERSION
	myApp.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "listen,l",
			Value: ":29900",
			Usage: "kcp server listen address",
		},
		cli.StringFlag{
			Name:  "target, t",
			Value: "127.0.0.1:12948",
			Usage: "target server address",
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
			Name:  "mtu",
			Value: 1350,
			Usage: "set maximum transmission unit for UDP packets",
		},
		cli.IntFlag{
			Name:  "sndwnd",
			Value: 1024,
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

		key = []byte(c.String("key"))
		crypt = c.String("crypt")

		datashard, parityshard := c.Int("datashard"), c.Int("parityshard")
		lis, err := kcp.ListenWithOptions(c.String("listen"), nil, datashard, parityshard)
		if err != nil {
			log.Fatal(err)
		}

		mtu, sndwnd, rcvwnd := c.Int("mtu"), c.Int("sndwnd"), c.Int("rcvwnd")
		nocomp, acknodelay := c.Bool("nocomp"), c.Bool("acknodelay")
		dscp, sockbuf, keepalive := c.Int("dscp"), c.Int("sockbuf"), c.Int("keepalive")
		target := c.String("target")

		log.Println("listening on ", lis.Addr())
		log.Println("encryption:", crypt)
		log.Println("nodelay parameters:", nodelay, interval, resend, nc)
		log.Println("sndwnd:", sndwnd, "rcvwnd:", rcvwnd)
		log.Println("compression:", !nocomp)
		log.Println("mtu:", mtu)
		log.Println("datashard:", datashard, "parityshard:", parityshard)
		log.Println("acknodelay:", acknodelay)
		log.Println("dscp:", dscp)
		log.Println("sockbuf:", sockbuf)
		log.Println("keepalive:", keepalive)

		if err := lis.SetDSCP(dscp); err != nil {
			log.Println("SetDSCP:", err)
		}
		if err := lis.SetReadBuffer(sockbuf); err != nil {
			log.Println("SetReadBuffer:", err)
		}
		if err := lis.SetWriteBuffer(sockbuf); err != nil {
			log.Println("SetWriteBuffer:", err)
		}
		config := &yamux.Config{
			AcceptBacklog:          256,
			EnableKeepAlive:        true,
			KeepAliveInterval:      30 * time.Second,
			ConnectionWriteTimeout: 30 * time.Second,
			MaxStreamWindowSize:    uint32(sockbuf),
			LogOutput:              os.Stderr,
		}
		for {
			if conn, err := lis.Accept(); err == nil {
				log.Println("remote address:", conn.RemoteAddr())
				conn.SetStreamMode(true)
				conn.SetNoDelay(nodelay, interval, resend, nc)
				conn.SetMtu(mtu)
				conn.SetWindowSize(sndwnd, rcvwnd)
				conn.SetACKNoDelay(acknodelay)
				conn.SetKeepAlive(keepalive)

				if nocomp {
					go handleMux(conn, target, config, conn)
				} else {
					go handleMux(newCompStream(conn), target, config, conn)
				}
			} else {
				log.Println(err)
			}
		}
	}
	myApp.Run(os.Args)
}
