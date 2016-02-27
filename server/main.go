package main

import (
	"crypto/rc4"
	"log"
	"net"
	"time"

	"github.com/xtaci/kcp-go"
)

const (
	_port     = ":29900"          // change this to bind ip
	_endpoint = "localhost:12948" // endpoint address
	_key_recv = "KS7893685"       // change both key for client & server
	_key_send = "KR3411865"
)

func main() {
	lis, err := kcp.Listen(kcp.MODE_FAST, _port)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("listening on ", lis.Addr())
	for {
		if conn, err := lis.Accept(); err == nil {
			handleClient(conn)
		} else {
			log.Println(err)
		}
	}
}

func peer(conn net.Conn, sess_die chan struct{}) chan []byte {
	ch := make(chan []byte, 128)
	go func() {
		defer func() {
			close(ch)
		}()

		decoder, err := rc4.NewCipher([]byte(_key_recv))
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
	return ch
}

func endpoint(sess_die chan struct{}) (net.Conn, <-chan []byte) {
	conn, err := net.Dial("udp", _endpoint)
	if err != nil {
		log.Println(err)
		return nil, nil
	}

	ch := make(chan []byte, 128)
	go func() {
		defer func() {
			close(ch)
		}()

		encoder, err := rc4.NewCipher([]byte(_key_send))
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

func handleClient(conn net.Conn) {
	log.Println("stream open")
	defer log.Println("stream close")
	sess_die := make(chan struct{})
	defer func() {
		close(sess_die)
		conn.Close()
	}()

	////
	ch_peer := peer(conn, sess_die)
	conn_ep, ch_ep := endpoint(sess_die)
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
