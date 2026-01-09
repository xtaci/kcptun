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
	"math/big"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"sync"

	"golang.org/x/crypto/pbkdf2"

	"github.com/fatih/color"
	"github.com/urfave/cli"
	kcp "github.com/xtaci/kcp-go/v5"
	"github.com/xtaci/kcptun/std"
	"github.com/xtaci/qpp"
	"github.com/xtaci/smux"
	"github.com/xtaci/tcpraw"
)

const (
	// SALT is used as the PBKDF2 salt while deriving the shared session key.
	SALT = "kcp-go"
	// maxSmuxVer guards against negotiating unsupported smux protocol versions.
	maxSmuxVer = 2
)

const (
	TGT_UNIX = iota
	TGT_TCP
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
	myApp.Usage = "server(with SMUX)"
	myApp.Version = VERSION
	myApp.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "listen,l",
			Value: ":29900",
			Usage: `kcp server listen address, eg: "IP:29900" for a single port, "IP:minport-maxport" for port range`,
		},
		cli.StringFlag{
			Name:  "target, t",
			Value: "127.0.0.1:12948",
			Usage: "target server address, or path/to/unix_socket",
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
		cli.BoolFlag{
			Name:  "QPP",
			Usage: "enable Quantum Permutation Pads(QPP)",
		},
		cli.IntFlag{
			Name:  "QPPCount",
			Value: 61,
			Usage: "the prime number of pads to use for QPP: The more pads you use, the more secure the encryption. Each pad requires 256 bytes.",
		},

		cli.StringFlag{
			Name:  "mode",
			Value: "fast",
			Usage: "profiles: fast3, fast2, fast, normal, manual",
		},
		cli.IntFlag{
			Name:  "mtu",
			Value: 1350,
			Usage: "set maximum transmission unit for UDP packets",
		},
		cli.IntFlag{
			Name:  "ratelimit",
			Value: 0,
			Usage: "set maximum outgoing speed (in bytes per second) for a single KCP connection, 0 to disable. Also known as packet pacing.",
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
			Value: 4194304, // socket buffer size in bytes
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
			Value: 10, // nat keepalive interval in seconds
			Usage: "seconds between heartbeats",
		},
		cli.IntFlag{
			Name:  "closewait",
			Value: 30,
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
		cli.BoolFlag{
			Name:  "pprof",
			Usage: "start profiling server on :6060",
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
			Value: "", // when set, the JSON file must exist on disk
			Usage: "config from json file, which will override the command from shell",
		},
	}
	myApp.Action = func(c *cli.Context) error {
		config := Config{}
		config.Listen = c.String("listen")
		config.Target = c.String("target")
		config.Key = c.String("key")
		config.Crypt = c.String("crypt")
		config.Mode = c.String("mode")
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
		config.Pprof = c.Bool("pprof")
		config.Quiet = c.Bool("quiet")
		config.TCP = c.Bool("tcp")
		config.QPP = c.Bool("QPP")
		config.QPPCount = c.Int("QPPCount")
		config.CloseWait = c.Int("closewait")

		if c.String("c") != "" {
			// Only JSON configuration files are supported at the moment.
			err := parseJSONConfig(&config, c.String("c"))
			checkError(err)
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
		log.Println("smux version:", config.SmuxVer)
		log.Println("listening on:", config.Listen)
		log.Println("target:", config.Target)
		log.Println("encryption:", config.Crypt)
		log.Println("nodelay parameters:", config.NoDelay, config.Interval, config.Resend, config.NoCongestion)
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
		log.Println("snmplog:", config.SnmpLog)
		log.Println("snmpperiod:", config.SnmpPeriod)
		log.Println("pprof:", config.Pprof)
		log.Println("quiet:", config.Quiet)
		log.Println("tcp:", config.TCP)

		if config.QPP {
			if config.QPPCount <= 0 {
				log.Fatal("QPPCount must be greater than 0 when QPP is enabled")
			}
			minSeedLength := qpp.QPPMinimumSeedLength(8)
			if len(config.Key) < minSeedLength {
				color.Red("QPP Warning: 'key' has size of %d bytes, required %d bytes at least", len(config.Key), minSeedLength)
			}

			minPads := qpp.QPPMinimumPads(8)
			if config.QPPCount < minPads {
				color.Red("QPP Warning: QPPCount %d, required %d at least", config.QPPCount, minPads)
			}

			if new(big.Int).GCD(nil, nil, big.NewInt(int64(config.QPPCount)), big.NewInt(8)).Int64() != 1 {
				color.Red("QPP Warning: QPPCount %d, choose a prime number for security", config.QPPCount)
			}
		}
		// Guard against negotiating unsupported smux protocol versions.
		if config.SmuxVer > maxSmuxVer {
			log.Fatal("unsupported smux version:", config.SmuxVer)
		}

		// Derive the shared session key from the pre-shared secret.
		log.Println("initiating key derivation")
		pass := pbkdf2.Key([]byte(config.Key), []byte(SALT), 4096, 32, sha1.New)
		log.Println("key derivation done")
		block, effectiveCrypt := std.SelectBlockCrypt(config.Crypt, pass)
		config.Crypt = effectiveCrypt

		// Start the SNMP logger if the feature is enabled.
		go std.SnmpLogger(config.SnmpLog, config.SnmpPeriod)

		// Start the pprof server if the feature is enabled.
		if config.Pprof {
			go http.ListenAndServe(":6060", nil)
		}

		// Instantiate a shared QPP pad if the feature is enabled.
		var _Q_ *qpp.QuantumPermutationPad
		if config.QPP {
			_Q_ = qpp.NewQPP([]byte(config.Key), uint16(config.QPPCount))
		}

		// Spawn an accept loop per listener and track each goroutine via WaitGroup.
		var wg sync.WaitGroup
		// loop accepts new KCP conversations on the provided listener and hands
		// each of them to handleMux in its own goroutine.
		loop := func(lis *kcp.Listener) {
			defer wg.Done()
			if err := lis.SetDSCP(config.DSCP); err != nil {
				log.Println("SetDSCP:", err)
			}
			if err := lis.SetReadBuffer(config.SockBuf); err != nil {
				log.Println("SetReadBuffer:", err)
			}
			if err := lis.SetWriteBuffer(config.SockBuf); err != nil {
				log.Println("SetWriteBuffer:", err)
			}

			for {
				if conn, err := lis.AcceptKCP(); err == nil {
					log.Println("remote address:", conn.RemoteAddr())
					conn.SetStreamMode(true)
					conn.SetWriteDelay(false)
					conn.SetNoDelay(config.NoDelay, config.Interval, config.Resend, config.NoCongestion)
					conn.SetMtu(config.MTU)
					conn.SetWindowSize(config.SndWnd, config.RcvWnd)
					conn.SetACKNoDelay(config.AckNodelay)
					conn.SetRateLimit(uint32(config.RateLimit))

					if config.NoComp {
						go handleMux(_Q_, conn, &config)
					} else {
						go handleMux(_Q_, std.NewCompStream(conn), &config)
					}
				} else {
					log.Printf("%+v", err)
				}
			}
		}

		// Parse the listen address which may contain a port range.
		mp, err := std.ParseMultiPort(config.Listen)
		if err != nil {
			log.Println(err)
			return err
		}

		// Create listeners for every port inside the configured range.
		for port := mp.MinPort; port <= mp.MaxPort; port++ {
			listenAddr := fmt.Sprintf("%v:%v", mp.Host, port)
			if config.TCP { // optional tcpraw dual stack
				if conn, err := tcpraw.Listen("tcp", listenAddr); err == nil {
					log.Printf("Listening on: %v/tcp", listenAddr)
					lis, err := kcp.ServeConn(block, config.DataShard, config.ParityShard, conn)
					checkError(err)
					wg.Add(1)
					go loop(lis)
				} else {
					log.Println(err)
				}
			}

			// Always stand up the UDP listener; this is the default transport.
			log.Printf("Listening on: %v/udp", listenAddr)
			lis, err := kcp.ListenWithOptions(listenAddr, block, config.DataShard, config.ParityShard)
			checkError(err)
			wg.Add(1)
			go loop(lis)
		}

		wg.Wait()
		return nil
	}
	myApp.Run(os.Args)
}

// handleMux terminates a KCP session, accepts smux streams, and forwards them
// to the configured TCP or UNIX target.
func handleMux(_Q_ *qpp.QuantumPermutationPad, conn net.Conn, config *Config) {
	// Determine whether the upstream target is TCP or a UNIX socket path.
	targetType := TGT_TCP
	if _, _, err := net.SplitHostPort(config.Target); err != nil {
		targetType = TGT_UNIX
	}
	log.Println("smux version:", config.SmuxVer, "on connection:", conn.LocalAddr(), "->", conn.RemoteAddr())

	smuxConfig, err := std.BuildSmuxConfig(std.SmuxConfigParams{
		Version:          config.SmuxVer,
		MaxReceiveBuffer: config.SmuxBuf,
		MaxStreamBuffer:  config.StreamBuf,
		MaxFrameSize:     config.FrameSize,
		KeepAliveSeconds: config.KeepAlive,
	})
	if err != nil {
		log.Println(err)
		conn.Close()
		return
	}

	// Create the smux server session.
	mux, err := smux.Server(conn, smuxConfig)
	if err != nil {
		log.Println(err)
		return
	}
	defer mux.Close()

	// Accept and handle smux streams until the session terminates.
	for {
		stream, err := mux.AcceptStream()
		if err != nil {
			log.Println(err)
			return
		}

		go func(p1 *smux.Stream) {
			var p2 net.Conn
			var err error

			switch targetType {
			case TGT_TCP:
				p2, err = net.Dial("tcp", config.Target)
				if err != nil {
					log.Println(err)
					p1.Close()
					return
				}
				handleClient(_Q_, []byte(config.Key), p1, p2, config.Quiet, config.CloseWait)
			case TGT_UNIX:
				p2, err = net.Dial("unix", config.Target)
				if err != nil {
					log.Println(err)
					p1.Close()
					return
				}
				handleClient(_Q_, []byte(config.Key), p1, p2, config.Quiet, config.CloseWait)
			}

		}(stream)
	}
}

// handleClient bridges the smux stream to the upstream target and optionally
// wraps it in a QPP layer for additional obfuscation.
func handleClient(_Q_ *qpp.QuantumPermutationPad, seed []byte, p1 *smux.Stream, p2 net.Conn, quiet bool, closeWait int) {
	logln := func(v ...any) {
		if !quiet {
			log.Println(v...)
		}
	}

	defer p1.Close()
	defer p2.Close()

	logln("stream opened", "in:", fmt.Sprint(p1.RemoteAddr(), "(", p1.ID(), ")"), "out:", p2.RemoteAddr())
	defer logln("stream closed", "in:", fmt.Sprint(p1.RemoteAddr(), "(", p1.ID(), ")"), "out:", p2.RemoteAddr())

	var s1, s2 io.ReadWriteCloser = p1, p2
	// Optionally wrap the smux side with QPP obfuscation.
	if _Q_ != nil {
		// Replace the smux side with a QPP-wrapped port.
		s1 = std.NewQPPPort(p1, _Q_, seed)
	}

	// Begin piping data bidirectionally between the upstream and downstream ends.
	err1, err2 := std.Pipe(s1, s2, closeWait)

	// Report non-EOF errors so operators can diagnose failing streams.
	if err1 != nil && err1 != io.EOF {
		logln("pipe:", err1, "in:", p1.RemoteAddr(), "out:", fmt.Sprint(p2.RemoteAddr(), "(", p2.RemoteAddr(), ")"))
	}
	if err2 != nil && err2 != io.EOF {
		logln("pipe:", err2, "in:", p1.RemoteAddr(), "out:", fmt.Sprint(p2.RemoteAddr(), "(", p2.RemoteAddr(), ")"))
	}
}

func checkError(err error) {
	if err != nil {
		log.Printf("%+v\n", err)
		os.Exit(-1)
	}
}
