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

	sess_die := make(chan struct{})

	commkey := make([]byte, 32)
	copy(commkey, []byte(key))

	go func() {
		block, _ := aes.NewCipher(commkey)
		decoder := cipher.NewCTR(block, iv)

		udp_conn.SetReadDeadline(time.Now().Add(2 * time.Minute))

		_, err := CopyFilter(tcp_conn, udp_conn, func(buf []byte) []byte {
			decoder.XORKeyStream(buf, buf)
			udp_conn.SetReadDeadline(time.Now().Add(2 * time.Minute))
			return buf
		})

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
		block, _ := aes.NewCipher(commkey)
		encoder := cipher.NewCTR(block, iv)

		_, err := CopyFilter(udp_conn, tcp_conn, func(buf []byte) []byte {
			encoder.XORKeyStream(buf, buf)
			return buf
		})

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
