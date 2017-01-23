# kcptun Project Guide for AI Agents

## 1. Project Overview
**kcptun** is a high-performance network accelerator based on KCP with N:M multiplexing and FEC (Forward Error Correction). It is designed to relay TCP traffic over KCP (UDP), which can significantly improve throughput on lossy networks.

- **Core Protocol**: KCP (Reliable UDP).
- **Language**: Go.
- **License**: MIT.

## 2. Architecture
The project follows a Client-Server architecture:

- **Client (`client/`)**:
  - Listens on a local TCP port.
  - Encapsulates TCP traffic into KCP packets.
  - Sends KCP packets to the Server via UDP.
  - Handles encryption/decryption and multiplexing (SMUX).

- **Server (`server/`)**:
  - Listens on a UDP port for KCP traffic.
  - Decapsulates KCP packets back to TCP.
  - Forwards TCP traffic to the target server.
  - Handles encryption/decryption and multiplexing.

**Data Flow**:
`Application -> KCP Client (TCP Listen) -> [KCP/UDP Relay] -> KCP Server (UDP Listen) -> Target Server (TCP Connect)`

## 3. Directory Structure

- **`client/`**: Contains the source code for the client application.
  - `main.go`: Entry point, flag parsing, and main loop.
  - `config.go`: Configuration handling.
  - `dial.go`: Logic for dialing connections, including multiport dialer.
- **`server/`**: Contains the source code for the server application.
  - `main.go`: Entry point, flag parsing, and main loop.
  - `config.go`: Configuration handling.
- **`std/`**: Shared standard utilities used by both client and server.
  - `copy.go`: Memory-optimized `io.Copy` implementation.
  - `snmp.go`: SNMP statistics logging.
  - `comp.go`: Compression helpers.
  - `qpp.go`: Quantum Permutation Pads helpers.
- **`vendor/`**: Vendored dependencies.
- **`assets/`**: Images and other static assets.
- **Root**:
  - `build-release.sh`: Script to build releases for multiple platforms.
  - `Dockerfile`: Docker build configuration.
  - `go.mod`: Go module definition.

## 4. Key Dependencies
- **`github.com/xtaci/kcp-go/v5`**: The core KCP implementation in Go.
- **`github.com/xtaci/smux`**: Stream multiplexing library (allows multiple TCP streams over one KCP session).
- **`github.com/xtaci/tcpraw`**: Raw TCP packet emulation.
- **`github.com/urfave/cli`**: Command-line interface library for parsing flags.
- **`golang.org/x/crypto`**: Cryptographic primitives.

## 5. Build & Run

### Building
To build the project, you can use the provided script or standard Go commands.

**Using Script:**
```bash
./build-release.sh
```
This will generate binaries in the `build/` directory for a wide range of platforms including:
- **Darwin (macOS)**: amd64, arm64
- **Linux**: 386, amd64, arm (v5, v6, v7), arm64, mips, mipsle, loong64
- **Windows**: 386, amd64, arm64
- **FreeBSD**: amd64

**Using Go:**
```bash
go build -o client_bin ./client
go build -o server_bin ./server
```

**Using Docker:**
The project includes a `Dockerfile` for building a lightweight Alpine-based image.
```bash
docker build -t kcptun .
```
The image exposes ports `29900/udp` (Server) and `12948` (Client).

### Running
**Client:**
```bash
./client_bin -r "KCP_SERVER_IP:4000" -l ":8388" -mode fast3
```

**Server:**
```bash
./server_bin -t "TARGET_IP:8388" -l ":4000" -mode fast3
```

## 6. Configuration & Tuning
The application is highly configurable via command-line flags.

- **Mode (`-mode`)**: Controls KCP parameters (fast3, fast2, fast, normal, manual).
- **Crypt (`-crypt`)**: Encryption method (aes, salsa20, none, etc.).
- **MTU (`-mtu`)**: Maximum Transmission Unit.
- **SndWnd/RcvWnd**: Send and receive window sizes.
- **DataShard/ParityShard**: FEC settings.

**Performance Tips:**
- Increase system file descriptors (`ulimit -n`).
- Tune kernel UDP buffer sizes (`sysctl` parameters like `net.core.rmem_max`).
- Use `-nocomp` to disable compression if CPU is a bottleneck or data is already compressed.

## 7. Development Notes
- **Testing**: Run tests using `go test ./...`.
- **Vendoring**: Dependencies are managed in `vendor/`. When adding new dependencies, ensure they are vendored correctly.
- **Cross-Platform**: The project supports Linux, macOS, Windows, FreeBSD, and ARM architectures.
