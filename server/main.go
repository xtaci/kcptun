package main

import (
	"crypto/rc4"
	"log"
	"net"
	"os"
	"time"

	"github.com/codegangsta/cli"
	"github.com/xtaci/kcp-go"
)

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
		lis, err := kcp.Listen(kcp.MODE_FAST, c.String("listen"))
		if err != nil {
			log.Fatal(err)
		}

		log.Println("listening on ", lis.Addr())
		for {
			if conn, err := lis.Accept(); err == nil {
				handleClient(conn, c.String("target"), c.String("key"))
			} else {
				log.Println(err)
			}
		}
	}
	myApp.Run(os.Args)
}

func peer(conn net.Conn, sess_die chan struct{}, key string) chan []byte {
	ch := make(chan []byte, 128)
	go func() {
		defer func() {
			close(ch)
		}()

		decoder, err := rc4.NewCipher([]byte(key))
		if err != nil {
			log.Println(err)
			return
		}

		for {
			conn.SetReadDeadline(time.Now().Add(30 * time.Second))
			bts := make([]byte, 4096)
			n, err := conn.Read(bts)
			if err != nil {
				log.Println(err)
				return
			}
			bts = bts[:n]
			decoder.XORKeyStream(bts, bts)
			select {
			case ch <- bts:
			case <-sess_die:
				return
			}
		}
	}()
	return ch
}

func endpoint(sess_die chan struct{}, target string, key string) (net.Conn, <-chan []byte) {
	conn, err := net.Dial("tcp", target)
	if err != nil {
		log.Println(err)
		return nil, nil
	}

	ch := make(chan []byte, 128)
	go func() {
		defer func() {
			close(ch)
		}()

		encoder, err := rc4.NewCipher([]byte(key))
		if err != nil {
			log.Println(err)
			return
		}

		for {
			bts := make([]byte, 4096)
			n, err := conn.Read(bts)
			if err != nil {
				log.Println(err)
				return
			}

			bts = bts[:n]
			encoder.XORKeyStream(bts, bts)
			select {
			case ch <- bts:
			case <-sess_die:
				return
			}
		}
	}()
	return conn, ch
}

func handleClient(conn net.Conn, target string, key string) {
	log.Println("stream open")
	defer log.Println("stream close")
	sess_die := make(chan struct{})
	defer func() {
		close(sess_die)
		conn.Close()
	}()

	////
	ch_peer := peer(conn, sess_die, key)
	conn_ep, ch_ep := endpoint(sess_die, target, key)
	if conn_ep == nil {
		return
	}
	defer conn_ep.Close()

	for {
		select {
		case bts, ok := <-ch_peer:
			if !ok {
				return
			}
			if _, err := conn_ep.Write(bts); err != nil {
				log.Println(err)
				return
			}
		case bts, ok := <-ch_ep:
			if !ok {
				return
			}
			if _, err := conn.Write(bts); err != nil {
				log.Println(err)
				return
			}
		}
	}
}
