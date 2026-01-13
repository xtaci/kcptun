// The MIT License (MIT)
//
// # Copyright (c) 2016 xtaci
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package main

import (
	"crypto/sha1"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"

	"golang.org/x/crypto/pbkdf2"

	"github.com/fatih/color"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	kcp "github.com/xtaci/kcp-go/v5"
	"github.com/xtaci/kcptun/std"
	"github.com/xtaci/qpp"
	"github.com/xtaci/smux"
)

const (
	// SALT is used as the PBKDF2 salt while deriving the shared session key.
	SALT = "kcp-go"
	// maxSmuxVer guards against negotiating unsupported smux protocol versions.
	maxSmuxVer = 2
	// scavengePeriod defines how frequently expired sessions are purged.
	scavengePeriod = 5
)

// VERSION is populated via build flags when packaging official binaries.
var VERSION = "SELFBUILD"

func main() {
	if VERSION == "SELFBUILD" {
		// Enable timestamps + file:line to simplify debugging self-built binaries.
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	}

	myApp := cli.NewApp()
	myApp.Name = "kcptun"
	myApp.Usage = "client(with SMUX)"
	myApp.Version = VERSION
	myApp.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "localaddr,l",
			Value: ":12948",
			Usage: "local listen address",
		},
		cli.StringFlag{
			Name:  "remoteaddr, r",
			Value: "vps:29900",
			Usage: `kcp server address, eg: "IP:29900" a for single port, "IP:minport-maxport" for port range`,
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
			Usage: "aes, aes-128, aes-128-gcm, aes-192, salsa20, blowfish, twofish, cast5, 3des, tea, xtea, xor, sm4, none, null",
		},
		cli.StringFlag{
			Name:  "mode",
			Value: "fast",
			Usage: "profiles: fast3, fast2, fast, normal, manual",
		},
		cli.BoolFlag{
			Name:  "QPP",
			Usage: "enable Quantum Permutation Pads(QPP)",
		},
		cli.IntFlag{
			Name:  "QPPCount",
			Value: 61,
			Usage: "the prime number of pads to use for QPP: The more pads you use, the more secure the encryption. Each pad requires 256 bytes.",
		},
		cli.IntFlag{
			Name:  "conn",
			Value: 1,
			Usage: "set num of UDP connections to server",
		},
		cli.IntFlag{
			Name:  "autoexpire",
			Value: 0,
			Usage: "set auto expiration time(in seconds) for a single UDP connection, 0 to disable",
		},
		cli.IntFlag{
			Name:  "scavengettl",
			Value: 600,
			Usage: "set how long an expired connection can live (in seconds)",
		},
		cli.IntFlag{
			Name:  "mtu",
			Value: 1350,
			Usage: "set maximum transmission unit for UDP packets",
		},
		cli.IntFlag{
			Name:  "ratelimit",
			Value: 0,
			Usage: "set maximum outgoing speed (in bytes per second) for a single KCP connection, 0 to disable. Also known as packet pacing",
		},
		cli.IntFlag{
			Name:  "sndwnd",
			Value: 128,
			Usage: "set send window size(num of packets)",
		},
		cli.IntFlag{
			Name:  "rcvwnd",
			Value: 512,
			Usage: "set receive window size(num of packets)",
		},
		cli.IntFlag{
			Name:  "datashard,ds",
			Value: 10,
			Usage: "set reed-solomon erasure coding - datashard",
		},
		cli.IntFlag{
			Name:  "parityshard,ps",
			Value: 3,
			Usage: "set reed-solomon erasure coding - parityshard",
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
		cli.BoolFlag{
			Name:   "acknodelay",
			Usage:  "flush ack immediately when a packet is received",
			Hidden: true,
		},
		cli.IntFlag{
			Name:   "nodelay",
			Value:  0,
			Hidden: true,
		},
		cli.IntFlag{
			Name:   "interval",
			Value:  50,
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
			Name:  "sockbuf",
			Value: 4194304, // default socket buffer size in bytes
			Usage: "per-socket buffer in bytes",
		},
		cli.IntFlag{
			Name:  "smuxver",
			Value: 2,
			Usage: "specify smux version, available 1,2",
		},
		cli.IntFlag{
			Name:  "smuxbuf",
			Value: 4194304,
			Usage: "the overall de-mux buffer in bytes",
		},
		cli.IntFlag{
			Name:  "framesize",
			Value: 8192,
			Usage: "smux max frame size",
		},
		cli.IntFlag{
			Name:  "streambuf",
			Value: 2097152,
			Usage: "per stream receive buffer in bytes, smux v2+",
		},
		cli.IntFlag{
			Name:  "keepalive",
			Value: 10, // NAT keepalive interval in seconds
			Usage: "seconds between heartbeats",
		},
		cli.IntFlag{
			Name:  "closewait",
			Value: 0,
			Usage: "the seconds to wait before tearing down a connection",
		},
		cli.StringFlag{
			Name:  "snmplog",
			Value: "",
			Usage: "collect snmp to file, aware of timeformat in golang, like: ./snmp-20060102.log",
		},
		cli.IntFlag{
			Name:  "snmpperiod",
			Value: 60,
			Usage: "snmp collect period, in seconds",
		},
		cli.StringFlag{
			Name:  "log",
			Value: "",
			Usage: "specify a log file to output, default goes to stderr",
		},
		cli.BoolFlag{
			Name:  "quiet",
			Usage: "to suppress the 'stream open/close' messages",
		},
		cli.BoolFlag{
			Name:  "tcp",
			Usage: "to emulate a TCP connection(linux)",
		},
		cli.StringFlag{
			Name:  "c",
			Value: "", // when set, the referenced JSON file must exist on disk
			Usage: "config from json file, which will override the command from shell",
		},
		cli.BoolFlag{
			Name:  "pprof",
			Usage: "start profiling server on :6060",
		},
	}
	myApp.Action = func(c *cli.Context) error {
		config := Config{}
		config.LocalAddr = c.String("localaddr")
		config.RemoteAddr = c.String("remoteaddr")
		config.Key = c.String("key")
		config.Crypt = c.String("crypt")
		config.Mode = c.String("mode")
		config.Conn = c.Int("conn")
		config.AutoExpire = c.Int("autoexpire")
		config.ScavengeTTL = c.Int("scavengettl")
		config.MTU = c.Int("mtu")
		config.RateLimit = c.Int("ratelimit")
		config.SndWnd = c.Int("sndwnd")
		config.RcvWnd = c.Int("rcvwnd")
		config.DataShard = c.Int("datashard")
		config.ParityShard = c.Int("parityshard")
		config.DSCP = c.Int("dscp")
		config.NoComp = c.Bool("nocomp")
		config.AckNodelay = c.Bool("acknodelay")
		config.NoDelay = c.Int("nodelay")
		config.Interval = c.Int("interval")
		config.Resend = c.Int("resend")
		config.NoCongestion = c.Int("nc")
		config.SockBuf = c.Int("sockbuf")
		config.SmuxBuf = c.Int("smuxbuf")
		config.FrameSize = c.Int("framesize")
		config.StreamBuf = c.Int("streambuf")
		config.SmuxVer = c.Int("smuxver")
		config.KeepAlive = c.Int("keepalive")
		config.Log = c.String("log")
		config.SnmpLog = c.String("snmplog")
		config.SnmpPeriod = c.Int("snmpperiod")
		config.Quiet = c.Bool("quiet")
		config.TCP = c.Bool("tcp")
		config.Pprof = c.Bool("pprof")
		config.QPP = c.Bool("QPP")
		config.QPPCount = c.Int("QPPCount")
		config.CloseWait = c.Int("closewait")

		if c.String("c") != "" {
			err := parseJSONConfig(&config, c.String("c"))
			checkError(err)
		}

		if config.Conn <= 0 {
			log.Fatal("conn must be greater than 0")
		}

		if config.RateLimit < 0 {
			log.Printf("ratelimit %d is negative, falling back to 0", config.RateLimit)
			config.RateLimit = 0
		}

		// Redirect logs when the user supplied a dedicated log file.
		if config.Log != "" {
			f, err := os.OpenFile(config.Log, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
			checkError(err)
			defer f.Close()
			log.SetOutput(f)
		}

		switch config.Mode {
		case "normal":
			config.NoDelay, config.Interval, config.Resend, config.NoCongestion = 0, 40, 2, 1
		case "fast":
			config.NoDelay, config.Interval, config.Resend, config.NoCongestion = 0, 30, 2, 1
		case "fast2":
			config.NoDelay, config.Interval, config.Resend, config.NoCongestion = 1, 20, 2, 1
		case "fast3":
			config.NoDelay, config.Interval, config.Resend, config.NoCongestion = 1, 10, 2, 1
		}

		log.Println("version:", VERSION)
		var listener net.Listener
		var isUnix bool
		if _, _, err := net.SplitHostPort(config.LocalAddr); err != nil {
			isUnix = true
		}
		if isUnix {
			addr, err := net.ResolveUnixAddr("unix", config.LocalAddr)
			checkError(err)
			listener, err = net.ListenUnix("unix", addr)
			checkError(err)
		} else {
			addr, err := net.ResolveTCPAddr("tcp", config.LocalAddr)
			checkError(err)
			listener, err = net.ListenTCP("tcp", addr)
			checkError(err)
		}

		log.Println("smux version:", config.SmuxVer)
		log.Println("listening on:", listener.Addr())
		log.Println("encryption:", config.Crypt)
		log.Println("QPP:", config.QPP)
		log.Println("QPP Count:", config.QPPCount)
		log.Println("nodelay parameters:", config.NoDelay, config.Interval, config.Resend, config.NoCongestion)
		log.Println("remote address:", config.RemoteAddr)
		log.Println("sndwnd:", config.SndWnd, "rcvwnd:", config.RcvWnd)
		log.Println("compression:", !config.NoComp)
		log.Println("mtu:", config.MTU)
		log.Println("ratelimit:", config.RateLimit)
		log.Println("datashard:", config.DataShard, "parityshard:", config.ParityShard)
		log.Println("acknodelay:", config.AckNodelay)
		log.Println("dscp:", config.DSCP)
		log.Println("sockbuf:", config.SockBuf)
		log.Println("smuxbuf:", config.SmuxBuf)
		log.Println("framesize:", config.FrameSize)
		log.Println("streambuf:", config.StreamBuf)
		log.Println("keepalive:", config.KeepAlive)
		log.Println("conn:", config.Conn)
		log.Println("autoexpire:", config.AutoExpire)
		log.Println("scavengettl:", config.ScavengeTTL)
		log.Println("snmplog:", config.SnmpLog)
		log.Println("snmpperiod:", config.SnmpPeriod)
		log.Println("quiet:", config.Quiet)
		log.Println("tcp:", config.TCP)
		log.Println("pprof:", config.Pprof)

		// Validate QPP parameters so we can warn about unsafe combinations early.
		if config.QPP {
			suggestions, err := std.ValidateQPPParams(config.QPPCount, config.Key)
			if err != nil {
				log.Fatal(err)
			}
			for _, msg := range suggestions {
				color.Red(msg)
			}
		}

		// Ensure scavenger TTL does not exceed the auto-expire window.
		if config.AutoExpire != 0 && config.ScavengeTTL > config.AutoExpire {
			color.Red("WARNING: scavengettl is bigger than autoexpire, connections may race hard to use bandwidth.")
			color.Red("Try limiting scavengettl to a smaller value.")
		}

		// Guard against negotiating unsupported smux protocol versions.
		if config.SmuxVer > maxSmuxVer {
			log.Fatal("unsupported smux version:", config.SmuxVer)
		}

		// Derive the shared encryption key and prepare the block cipher.
		log.Println("initiating key derivation")
		pass := pbkdf2.Key([]byte(config.Key), []byte(SALT), 4096, 32, sha1.New)
		log.Println("key derivation done")
		block, effectiveCrypt := std.SelectBlockCrypt(config.Crypt, pass)
		config.Crypt = effectiveCrypt

		// Continuously export SNMP counters when requested.
		go std.SnmpLogger(config.SnmpLog, config.SnmpPeriod)

		// Optionally expose Go's net/http/pprof handlers on :6060.
		if config.Pprof {
			go http.ListenAndServe(":6060", nil)
		}

		// Launch the session scavenger only when auto-expiration is enabled.
		chScavenger := make(chan timedSession, 128)
		if config.AutoExpire > 0 {
			go scavenger(chScavenger, &config)
		}

		// Accept TCP/UNIX clients and multiplex them across the UDP tunnels.
		numconn := uint16(config.Conn)
		muxes := make([]timedSession, numconn)
		// rr tracks which pre-established session should carry the next client so
		// short-lived TCP dials do not hammer the same UDP tunnel.
		rr := uint16(0)

		// Instantiate a shared QPP pad if the feature is enabled.
		var _Q_ *qpp.QuantumPermutationPad
		if config.QPP {
			_Q_ = qpp.NewQPP([]byte(config.Key), uint16(config.QPPCount))
		}

		for {
			p1, err := listener.Accept()
			if err != nil {
				log.Fatalf("%+v", err)
			}
			idx := rr % numconn

			// Refresh the selected session if it is missing, closed, or past its TTL.
			if muxes[idx].session == nil || muxes[idx].session.IsClosed() ||
				(config.AutoExpire > 0 && time.Now().After(muxes[idx].expiryDate)) {
				muxes[idx].session = waitConn(&config, block)
				muxes[idx].expiryDate = time.Now().Add(time.Duration(config.AutoExpire) * time.Second)
				if config.AutoExpire > 0 { // only track TTL when auto-expiration is enabled
					chScavenger <- muxes[idx]
				}
			}

			go handleClient(_Q_, []byte(config.Key), muxes[idx].session, p1, config.Quiet, config.CloseWait)
			rr++
		}
	}
	myApp.Run(os.Args)
}

// createConn establishes a fresh KCP connection with all tunables applied and
// then upgrades it into an smux session ready for multiplexing.
func createConn(config *Config, block kcp.BlockCrypt) (*smux.Session, error) {
	kcpconn, err := dial(config, block)
	if err != nil {
		return nil, errors.Wrap(err, "dial()")
	}
	kcpconn.SetStreamMode(true)
	kcpconn.SetWriteDelay(false)
	kcpconn.SetNoDelay(config.NoDelay, config.Interval, config.Resend, config.NoCongestion)
	kcpconn.SetWindowSize(config.SndWnd, config.RcvWnd)
	kcpconn.SetMtu(config.MTU)
	kcpconn.SetACKNoDelay(config.AckNodelay)
	kcpconn.SetRateLimit(uint32(config.RateLimit))

	if err := kcpconn.SetDSCP(config.DSCP); err != nil {
		log.Println("SetDSCP:", err)
	}
	if err := kcpconn.SetReadBuffer(config.SockBuf); err != nil {
		log.Println("SetReadBuffer:", err)
	}
	if err := kcpconn.SetWriteBuffer(config.SockBuf); err != nil {
		log.Println("SetWriteBuffer:", err)
	}
	log.Println("smux version:", config.SmuxVer, "on connection:", kcpconn.LocalAddr(), "->", kcpconn.RemoteAddr())
	smuxConfig, err := std.BuildSmuxConfig(std.SmuxConfigParams{
		Version:          config.SmuxVer,
		MaxReceiveBuffer: config.SmuxBuf,
		MaxStreamBuffer:  config.StreamBuf,
		MaxFrameSize:     config.FrameSize,
		KeepAliveSeconds: config.KeepAlive,
	})
	if err != nil {
		log.Fatalf("%+v", err)
	}

	var session *smux.Session
	if config.NoComp {
		session, err = smux.Client(kcpconn, smuxConfig)
	} else {
		session, err = smux.Client(std.NewCompStream(kcpconn), smuxConfig)
	}
	if err != nil {
		return nil, errors.Wrap(err, "createConn()")
	}
	return session, nil
}

// waitConn keeps dialing until a healthy smux session becomes available.
func waitConn(config *Config, block kcp.BlockCrypt) *smux.Session {
	for {
		session, err := createConn(config, block)
		if err == nil {
			return session
		}
		log.Println("re-connecting:", err)
		time.Sleep(time.Second)
	}
}

// handleClient tunnels a single accepted TCP/UNIX client through an smux
// stream and optionally wraps the stream in QPP for additional obfuscation.
func handleClient(_Q_ *qpp.QuantumPermutationPad, seed []byte, session *smux.Session, p1 net.Conn, quiet bool, closeWait int) {
	logln := func(v ...any) {
		if !quiet {
			log.Println(v...)
		}
	}

	// Transport layer: accept the inbound socket and clean it up on exit.
	defer p1.Close()
	p2, err := session.OpenStream()
	if err != nil {
		logln(err)
		return
	}
	defer p2.Close()

	logln("stream opened", "in:", p1.RemoteAddr(), "out:", fmt.Sprint(p2.RemoteAddr(), "(", p2.ID(), ")"))
	defer logln("stream closed", "in:", p1.RemoteAddr(), "out:", fmt.Sprint(p2.RemoteAddr(), "(", p2.ID(), ")"))

	var s1, s2 io.ReadWriteCloser = p1, p2
	// Optionally wrap the smux side with QPP obfuscation.
	if _Q_ != nil {
		// Replace the smux side with a QPP-wrapped port.
		s2 = std.NewQPPPort(p2, _Q_, seed)
	}

	// Begin piping data bidirectionally between the socket and the smux stream.
	err1, err2 := std.Pipe(s1, s2, closeWait)

	// Report non-EOF errors so operators can diagnose failing streams.
	if err1 != nil && err1 != io.EOF {
		logln("pipe:", err1, "in:", p1.RemoteAddr(), "out:", fmt.Sprint(p2.RemoteAddr(), "(", p2.ID(), ")"))
	}
	if err2 != nil && err2 != io.EOF {
		logln("pipe:", err2, "in:", p1.RemoteAddr(), "out:", fmt.Sprint(p2.RemoteAddr(), "(", p2.ID(), ")"))
	}
}

// checkError logs the supplied fatal error and terminates the process.
func checkError(err error) {
	if err != nil {
		log.Printf("%+v\n", err)
		os.Exit(-1)
	}
}

// timedSession annotates a smux session with its expiration deadline.
type timedSession struct {
	session    *smux.Session
	expiryDate time.Time
}

// scavenger tracks expiring sessions received on ch and closes them after the
// configured TTL elapses.
func scavenger(ch chan timedSession, config *Config) {
	ticker := time.NewTicker(scavengePeriod * time.Second)
	defer ticker.Stop()
	var sessionList []timedSession
	for {
		select {
		case item := <-ch:
			sessionList = append(sessionList, timedSession{
				item.session,
				item.expiryDate.Add(time.Duration(config.ScavengeTTL) * time.Second)})
		case <-ticker.C:
			var newList []timedSession
			for k := range sessionList {
				s := sessionList[k]
				if s.session.IsClosed() {
					log.Println("scavenger: session normally closed:", s.session.LocalAddr())
				} else if time.Now().After(s.expiryDate) {
					s.session.Close()
					log.Println("scavenger: session closed due to ttl:", s.session.LocalAddr())
				} else {
					newList = append(newList, sessionList[k])
				}
			}
			sessionList = newList
		}
	}
}
