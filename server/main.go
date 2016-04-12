package main

import (
	"crypto/aes"
	"crypto/cipher"
	"io"
	"log"
	"net"
	"os"
	"time"

	"github.com/codegangsta/cli"
	"github.com/xtaci/kcp-go"
)

var iv = []byte{147, 243, 201, 109, 83, 207, 190, 153, 204, 106, 86, 122, 71, 135, 200, 20}

func main() {
	myApp := cli.NewApp()
	myApp.Name = "kcptun"
	myApp.Usage = "kcptun server"
	myApp.Version = "1.0"
	myApp.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "listen,l",
			Value: ":29900",
			Usage: "kcp server listen addr:",
		},
		cli.StringFlag{
			Name:  "target, t",
			Value: "127.0.0.1:12948",
			Usage: "target server addr",
		},
		cli.StringFlag{
			Name:  "key",
			Value: "it's a secrect",
			Usage: "key for communcation, must be the same as kcptun client",
		},
	}
	myApp.Action = func(c *cli.Context) {
		lis, err := kcp.ListenEncrypted(kcp.MODE_FAST, c.String("listen"), c.String("key"))
		if err != nil {
			log.Fatal(err)
		}

		log.Println("listening on ", lis.Addr())
		for {
			if conn, err := lis.Accept(); err == nil {
				conn.SetWindowSize(1024, 128)
				go handleClient(conn, c.String("target"), c.String("key"))
			} else {
				log.Println(err)
			}
		}
	}
	myApp.Run(os.Args)
}

type cipherConn struct {
	rw net.Conn
	rs cipher.Stream
	ws cipher.Stream
	rd time.Duration
}

func NewCipherConn(rw net.Conn, commkey []byte) *cipherConn {
	rblock, rerr := aes.NewCipher(commkey)
	checkError(rerr)
	wblock, werr := aes.NewCipher(commkey)
	checkError(werr)
	return &cipherConn{
		rw: rw,
		rs: cipher.NewCTR(rblock, iv),
		ws: cipher.NewCTR(wblock, iv),
	}
}

func (m *cipherConn) Read(b []byte) (n int, err error) {
	if n, err = m.rw.Read(b); n > 0 {
		m.rs.XORKeyStream(b[:n], b[:n])
		if m.rd != 0 {
			m.rw.SetReadDeadline(time.Now().Add(m.rd))
		}
	}
	return
}

func (m *cipherConn) Write(b []byte) (n int, err error) {
	m.ws.XORKeyStream(b, b)
	return m.rw.Write(b)
}

func (m *cipherConn) SetReadTimeout(rd time.Duration) {
	m.rd = rd
}

func handleClient(udp_conn net.Conn, target string, key string) {
	log.Println("stream open")
	defer udp_conn.Close()
	defer log.Println("stream closed")

	tcp_conn, err := net.Dial("tcp", target)
	if err != nil {
		log.Println(err)
		return
	}

	tcp_conn.(*net.TCPConn).SetNoDelay(false)
	defer tcp_conn.Close()

	commkey := make([]byte, 32)
	copy(commkey, []byte(key))

	c_udp_conn := NewCipherConn(udp_conn, commkey)
	c_tcp_conn := NewCipherConn(tcp_conn, commkey)

	sess_die := make(chan struct{})

	go func() {
		c_udp_conn.SetReadTimeout(2 * time.Minute)
		if _, err := io.Copy(c_tcp_conn, c_udp_conn); nil != err {
			log.Println(err)
		}

		select {
		case <-sess_die:
		default:
			close(sess_die)
		}
	}()

	go func() {

		if _, err := io.Copy(c_udp_conn, c_tcp_conn); nil != err {
			log.Println(err)
		}

		select {
		case <-sess_die:
		default:
			close(sess_die)
		}
	}()

	<-sess_die
}

func checkError(err error) {
	if err != nil {
		log.Println(err)
		os.Exit(-1)
	}
}
