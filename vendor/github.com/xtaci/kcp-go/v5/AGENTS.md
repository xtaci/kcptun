# kcp-go Project Guide for AI Agents

## 1. Project Overview
**kcp-go** is a production-grade, reliable UDP library for Go. It implements the KCP protocol, providing a stream-based interface (like TCP) over UDP with low latency, high throughput, and packet-level control. It is widely used in gaming, streaming, and network acceleration (e.g., kcptun).

**Key Features:**
- **Reliable ARQ:** Automatic Repeat reQuest for reliable delivery.
- **FEC (Forward Error Correction):** Reed-Solomon codes to recover lost packets without retransmission.
- **Encryption:** Packet-level encryption (AES, TEA, Salsa20, etc.).
- **Performance:** Optimized for high concurrency and low memory footprint using buffer pools and platform-specific syscalls (sendmmsg/recvmmsg).

## 2. Architecture

The project is layered as follows:

1.  **Application Layer:** Uses `net.Conn` interface (`UDPSession`) to Read/Write streams.
2.  **Session Layer (`sess.go`):** Manages the connection lifecycle, encryption/decryption, and FEC encoding/decoding. It acts as the glue between the raw UDP socket and the KCP protocol state.
3.  **Protocol Layer (`kcp.go`):** The core KCP state machine. Handles sequence numbers, retransmission queues, flow control, and congestion control.
4.  **Transport Layer:** Raw UDP sockets (`net.PacketConn`).

### Data Pipeline

**Outgoing (Write):**
`App Stream` -> `UDPSession.Write` -> `KCP.Send` -> `KCP.flush` -> `UDPSession.output` -> `chPostProcessing` -> `postProcess` (FEC Encode -> Encrypt) -> `UDP Socket`

**Incoming (Read):**
`UDP Socket` -> `readLoop` -> `packetInput` (Decrypt -> FEC Decode) -> `KCP.Input` -> `UDPSession.Read` -> `App Stream`

## 3. Key Files & Components

| File | Component | Description |
| :--- | :--- | :--- |
| **`kcp.go`** | **Core Protocol** | Implements the KCP protocol logic (ARQ, RTO calculation, window management). Pure logic, no I/O. |
| **`sess.go`** | **Session Management** | Defines `UDPSession` (implements `net.Conn`) and `Listener`. Handles the "plumbing" of data. |
| **`fec.go`** | **FEC** | Forward Error Correction implementation using Reed-Solomon codes. |
| **`crypt.go`** | **Encryption** | Block ciphers and stream ciphers wrapper for packet encryption. |
| **`tx.go`** | **Transmission** | Platform-specific packet transmission logic (e.g., `tx_linux.go` uses `sendmmsg`). |
| **`readloop.go`** | **Reception** | Platform-specific packet reception loops (e.g., `readloop_linux.go` uses `recvmmsg`). |
| **`timedsched.go`** | **Scheduler** | A global timer scheduler to drive `KCP.update` for all sessions, reducing goroutine overhead. |
| **`bufferpool.go`** | **Memory Management** | `sync.Pool` wrappers to minimize GC pressure during high-throughput data transfer. |

## 4. Core Concepts

### KCP Protocol
- **Conversation ID (`conv`):** Uniquely identifies a session.
- **Command (`cmd`):** PUSH (data), ACK (acknowledgment), WASK (window probe), WINS (window size).
- **Fast Retransmit:** Retransmits lost packets faster than RTO if out-of-order packets are received.
- **No Delay:** Configurable mode to minimize RTO and disable congestion control for lowest latency.

### FEC (Forward Error Correction)
- Splits data into shards and generates parity shards.
- Allows recovery of `N` lost packets if `N <= parity_shards`.
- Increases bandwidth usage but reduces latency caused by retransmissions.

### Concurrency Model
- **Per-Session Goroutines:**
    - `readLoop`: Reads from the UDP socket (for clients).
    - `postProcess`: Handles encryption and FEC encoding before sending.
- **Global Scheduler:** `SystemTimedSched` (in `timedsched.go`) manages the `update` tick for all sessions to avoid creating a ticker goroutine per session.
- **Locking:** `UDPSession` uses a `sync.Mutex` to protect KCP state.

## 5. Development Guidelines for AI

1.  **Context Awareness:** When modifying `kcp.go`, remember it is a pure state machine. Do not introduce I/O operations directly into `KCP` methods.
2.  **Performance:** Be mindful of memory allocations. Use `defaultBufferPool` (`bufferpool.go`) for temporary byte slices.
3.  **Platform Compatibility:** If modifying network I/O, check `platform_*.go`, `tx_*.go`, and `readloop_*.go` to ensure cross-platform compatibility (Linux, BSD, Windows, Generic).
4.  **Testing:**
    - Use `kcp_test.go` for protocol logic tests.
    - Use `sess_test.go` for integration tests involving `UDPSession`.
    - Use `examples/` to verify end-to-end functionality.
5.  **Encryption:** New encryption methods should implement the `BlockCrypt` or `aeadCrypt` interfaces in `crypt.go`.

## 6. Common Tasks

- **Tuning Parameters:** Look at `SetNoDelay`, `SetWindowSize`, `SetMtu` in `sess.go`.
- **Debugging:** `KCP` struct has `reserved` fields and logging constants (`IKCP_LOG_*`) that can be enabled for tracing.
- **Adding Metrics:** `snmp.go` contains the `Snmp` struct for global statistics. Update this struct to add new metrics.
