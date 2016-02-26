package main

import (
	"crypto/rc4"
	"log"
	"net"
	"os"
	"os/exec"
	"time"

	"github.com/xtaci/kcp-go"
)

const (
	_port     = ":1194"     // change this to bind ip
	_server   = "vps:29900" // server address
	_key_send = "KS7893685" // change both key for client & server
	_key_recv = "KR3411865"
)

func main() {
	tcpAddr, err := net.ResolveTCPAddr("tcp4", _port)
	checkError(err)
	listener, err := net.ListenTCP("tcp", tcpAddr)
	checkError(err)
	log.Println("listening on:", listener.Addr())

	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			log.Println("accept failed:", err)
			continue
		}
		handleClient(conn)
	}
}

func peer(sess_die chan struct{}) (net.Conn, <-chan []byte) {
	conn, err := kcp.Dial(_server)
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

func client(conn *net.TCPConn, sess_die chan struct{}) <-chan []byte {
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

func handleClient(conn *net.TCPConn) {
	log.Println("stream opened")
	defer log.Println("stream closed")
	defer func() {
		cmd := exec.Command("killall", "-HUP", "openvpn")
		cmd.Run()
	}()

	sess_die := make(chan struct{})
	defer func() {
		close(sess_die)
		conn.Close()
	}()

	//
	conn_peer, ch_peer := peer(sess_die)
	ch_client := client(conn, sess_die)
	if conn_peer == nil {
		return
	}
	defer conn_peer.Close()

	force_re := time.After(6 * time.Hour)

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
		case <-force_re:
			return
		}
	}
}

func checkError(err error) {
	if err != nil {
		log.Println(err)
		os.Exit(-1)
	}
}
