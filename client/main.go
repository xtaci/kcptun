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
			handleClient(conn, c.String("remoteaddr"), c.String("key"))
		}
	}
	myApp.Run(os.Args)
}

func peer(sess_die chan struct{}, remote string, key string) (net.Conn, <-chan []byte) {
	conn, err := kcp.Dial(kcp.MODE_FAST, remote)
	if err != nil {
		panic(err)
	}
	if err != nil {
		log.Println(err)
		return nil, nil
	}
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
			conn.SetReadDeadline(time.Now().Add(2 * time.Minute))
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
	return conn, ch
}

func client(conn net.Conn, sess_die chan struct{}, key string) <-chan []byte {
	ch := make(chan []byte, 128)
	go func() {
		defer func() {
			close(ch)
		}()
		// encoder
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
	return ch
}

func handleClient(conn *net.TCPConn, remote string, key string) {
	log.Println("stream opened")
	defer log.Println("stream closed")
	sess_die := make(chan struct{})
	defer func() {
		close(sess_die)
		conn.Close()
	}()

	conn_peer, ch_peer := peer(sess_die, remote, key)
	ch_client := client(conn, sess_die, key)
	if conn_peer == nil {
		return
	}
	defer conn_peer.Close()

	for {
		select {
		case bts, ok := <-ch_peer:
			if !ok {
				return
			}
			if _, err := conn.Write(bts); err != nil {
				log.Println(err)
				return
			}
		case bts, ok := <-ch_client:
			if !ok {
				return
			}
			if _, err := conn_peer.Write(bts); err != nil {
				log.Println(err)
				return
			}
		}
	}
}

func checkError(err error) {
	if err != nil {
		log.Println(err)
		os.Exit(-1)
	}
}
