package main

import (
	"crypto/aes"
	"crypto/cipher"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"time"

	"github.com/codegangsta/cli"
	"github.com/xtaci/kcp-go"
)

const (
	BUFSIZ = 65536
)

var (
	ch_buf chan []byte
	iv     []byte = []byte{147, 243, 201, 109, 83, 207, 190, 153, 204, 106, 86, 122, 71, 135, 200, 20}
)

func init() {
	ch_buf = make(chan []byte, 1024)
	go func() {
		for {
			ch_buf <- make([]byte, BUFSIZ)
		}
	}()

	rand.Seed(time.Now().UnixNano())
}

func main() {
	myApp := cli.NewApp()
	myApp.Name = "kcptun"
	myApp.Usage = "kcptun client"
	myApp.Version = "1.0"
	myApp.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "localaddr,l",
			Value: ":12948",
			Usage: "local listen addr:",
		},
		cli.StringFlag{
			Name:  "remoteaddr, r",
			Value: "vps:29900",
			Usage: "kcp server addr",
		},
		cli.StringFlag{
			Name:  "key",
			Value: "it's a secrect",
			Usage: "key for communcation, must be the same as kcptun server",
		},
	}
	myApp.Action = func(c *cli.Context) {
		addr, err := net.ResolveTCPAddr("tcp", c.String("localaddr"))
		checkError(err)
		listener, err := net.ListenTCP("tcp", addr)
		checkError(err)
		log.Println("listening on:", listener.Addr())
		for {
			conn, err := listener.AcceptTCP()
			if err != nil {
				log.Println("accept failed:", err)
				continue
			}
			go handleClient(conn, c.String("remoteaddr"), c.String("key"))
		}
	}
	myApp.Run(os.Args)
}

func CopyFilter(dst io.Writer, src io.Reader, filter func([]byte) []byte) (written int64, err error) {
	return copyBuffer(dst, src, nil, filter)
}

func copyBuffer(dst io.Writer, src io.Reader, buf []byte, filter func([]byte) []byte) (written int64, err error) {
	if 0 == len(buf) {
		buf = make([]byte, 32*1024)
	}
	if nil == filter {
		filter = func(data []byte) []byte { return data }
	}
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(filter(buf[:nr]))
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if nil != er {
			err = er
			break
		}
	}
	return written, err
}

func handleClient(tcp_conn *net.TCPConn, remote string, key string) {
	log.Println("stream opened")
	tcp_conn.SetNoDelay(false)
	defer tcp_conn.Close()

	var sendbytes, recvbytes int64
	defer func() { log.Println("stream closed.", "send: ", sendbytes, ", recv: ", recvbytes) }()

	udp_conn, err := kcp.DialEncrypted(kcp.MODE_FAST, remote, key)

	if err != nil {
		log.Println(err)
		return
	}

	udp_conn.SetWindowSize(128, 1024)
	defer udp_conn.Close()

	sess_die := make(chan struct{})

	commkey := make([]byte, 32)
	copy(commkey, []byte(key))

	go func() {
		block, err := aes.NewCipher(commkey)
		encoder := cipher.NewCTR(block, iv)

		sb, err := CopyFilter(udp_conn, tcp_conn, func(buf []byte) []byte {
			encoder.XORKeyStream(buf, buf)
			return buf
		})

		sendbytes += sb

		select {
		case <-sess_die:
		default:
			close(sess_die)
		}

		if nil != err {
			log.Println(err)
		}
	}()

	go func() {
		block, err := aes.NewCipher(commkey)
		decoder := cipher.NewCTR(block, iv)

		udp_conn.SetReadDeadline(time.Now().Add(2 * time.Minute))

		rb, err := CopyFilter(tcp_conn, udp_conn, func(buf []byte) []byte {
			decoder.XORKeyStream(buf, buf)
			udp_conn.SetReadDeadline(time.Now().Add(2 * time.Minute))
			return buf
		})

		recvbytes += rb

		select {
		case <-sess_die:
		default:
			close(sess_die)
		}

		if nil != err {
			log.Println(err)
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
