# AGENTS.md - smux Project Context

## Project Overview
**smux** (Simple Multiplexing) is a multiplexing library for Golang. It allows multiple logical streams to share a single underlying connection (like TCP or KCP). It is designed for reliability and ordering, and is a core component of [kcp-go](https://github.com/xtaci/kcp-go) and [kcptun](https://github.com/xtaci/kcptun).

## Key Features
- **Multiplexing**: Multiple streams over one connection.
- **Flow Control**: Token bucket controlled receiving and per-stream sliding window (protocol v2+).
- **Memory Efficiency**: Shared receive buffer among streams to control overall memory usage.
- **Low Overhead**: Minimized header (8 bytes).
- **Traffic Shaping**: Built-in fair queue traffic shaping.

## Architecture & Core Components

### 1. Session (`session.go`)
The `Session` struct is the main manager for a multiplexed connection.
- Manages the underlying `io.ReadWriteCloser`.
- Handles the creation and acceptance of streams.
- Manages the shared receive buffer and token bucket.
- **Key Methods**: `Client`, `Server`, `OpenStream`, `AcceptStream`.

### 2. Stream (`stream.go`)
The `Stream` struct represents a logical stream within a session.
- Implements `net.Conn` interface (Read, Write, Close, etc.).
- Handles data buffering and flow control.
- **Key Methods**: `Read`, `Write`, `Close`.

### 3. Frame (`frame.go`)
Defines the wire format for data transmission.
- **Header Format** (8 bytes):
  - `VERSION` (1 byte): Protocol version (1 or 2).
  - `CMD` (1 byte): Command type (`SYN`, `FIN`, `PSH`, `NOP`, `UPD`).
  - `LENGTH` (2 bytes): Payload length.
  - `STREAMID` (4 bytes): Stream identifier.
- **Commands**:
  - `cmdSYN`: Stream open.
  - `cmdFIN`: Stream close (EOF).
  - `cmdPSH`: Data push.
  - `cmdNOP`: No operation (keep-alive).
  - `cmdUPD`: Window update (v2 only).

### 4. Configuration (`mux.go`)
The `Config` struct allows tuning the session.
- **Key Fields**: `Version`, `KeepAliveInterval`, `MaxFrameSize`, `MaxReceiveBuffer`, `MaxStreamBuffer`.

### 5. Traffic Shaping (`shaper.go`)
Implements traffic shaping logic to ensure fair bandwidth usage among streams.

## Development Guidelines

### Testing
- Run all tests:
  ```bash
  go test -v .
  ```
- Run benchmarks:
  ```bash
  go test -v -run=^$ -bench .
  ```

### Coding Conventions
- Follow standard Go coding conventions (formatting, naming, etc.).
- Ensure backward compatibility when modifying protocol-related code.
- Pay attention to concurrency and locking, as `Session` and `Stream` are heavily concurrent.

## Common Tasks

### Creating a Client Session
```go
conn, _ := net.Dial(...)
session, _ := smux.Client(conn, nil) // nil for default config
stream, _ := session.OpenStream()
```

### Creating a Server Session
```go
conn, _ := listener.Accept()
session, _ := smux.Server(conn, nil)
stream, _ := session.AcceptStream()
```

### Protocol Versions
- **Version 1**: Basic multiplexing.
- **Version 2**: Adds `cmdUPD` for flow control (window updates).

## File Structure
- `alloc.go`: Memory allocation utilities.
- `frame.go`: Frame definition and parsing.
- `mux.go`: Configuration and entry points.
- `session.go`: Session logic.
- `stream.go`: Stream logic.
- `shaper.go`: Traffic shaping logic.
- `*_test.go`: Tests for respective components.
