package main

import (
	"crypto/rc4"
	"log"
	"net"
	"os"
	"time"

	"github.com/xtaci/kcp-go"
)

const (
	_port     = ":12948"    // change this to bind ip
	_server   = "vps:29900" // server address
	_key_send = "KS7893685" // change both key for client & server
	_key_recv = "KR3411865"
)

func main() {
	udpaddr, err := net.ResolveUDPAddr("udp", _port)
	checkError(err)
	conn, err := net.ListenUDP("udp", udpaddr)
	checkError(err)
	log.Println("listening on:", conn.LocalAddr())
	handleClient(conn)
}

func peer(sess_die chan struct{}) (net.Conn, <-chan []byte) {
	conn, err := kcp.Dial(kcp.MODE_FAST, _server)
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
	return conn, ch
}

func client(conn net.Conn, sess_die chan struct{}) <-chan []byte {
	ch := make(chan []byte, 128)
	go func() {
		defer func() {
			close(ch)
		}()
		// encoder
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
	return ch
}

func handleClient(conn *net.UDPConn) {
	log.Println("stream opened")
	defer log.Println("stream closed")
	sess_die := make(chan struct{})
	defer func() {
		close(sess_die)
		conn.Close()
	}()

	//
	buf := make([]byte, 4096)
	n, from, err := conn.ReadFromUDP(buf)
	if err != nil {
		return
	}

	conn_peer, ch_peer := peer(sess_die)
	conn_peer.Write(buf[:n])
	ch_client := client(conn, sess_die)
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
			if _, err := conn.WriteTo(bts, from); err != nil {
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
