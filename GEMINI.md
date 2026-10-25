# GEMINI.md - Aegis Development Guide & Agent Instructions

## 1. Project Overview
**Aegis** is a high-performance, decentralized distributed rate limiter and load shedding proxy built in Go.
It eliminates centralized databases (such as Redis) from the critical path of API gateways by pairing Conflict-free Replicated Data Types (CRDTs) with an asynchronous background UDP gossip protocol and local TCP Vegas adaptive concurrency limiting.

* **Core Goal:** Sub-millisecond, in-memory rate-limit evaluation and adaptive load shedding with eventual consistency across distributed nodes.
* **Primary Language:** Go (Golang)
* **Serialization:** Protocol Buffers (Protobuf)
* **Transport:** UDP for peer gossip, HTTP for reverse proxy routing.
* **Testing & Verification:** Chaos testing via Toxiproxy and throughput benchmarking via Vegeta.

---

## 2. Architecture & Algorithms

### Rate Limiting & Concurrency Control
* **Velocity Rate Limiting:** Sliding Window Counter built on time-bucketed Grow-Only Counters (G-Counters).
* **State Convergence:** The CRDT merge function must be idempotent, commutative, and monotonic:
  $$\text{Merged}[i] = \max(\text{Local}[i], \text{Remote}[i])$$
* **Adaptive Concurrency Limiting (Load Shedding):** Local TCP Vegas gradient queue-delay algorithm:
  $$\text{QueueSize} = \text{Limit} \times \left(1 - \frac{RTT_{\min}}{RTT_{\text{curr}}}\right)$$
* **Modular Protection Modes:** Configurable via `Protection.Mode` as `RateLimitOnly` (429), `ConcurrencyOnly` (503), or `Strict` (both).

### Dynamic Key Extraction Hierarchy
To avoid the Shared IP / NAT problem, extract request identifiers in the following strict order:
1. `X-API-Key` header
2. `Authorization` header (Bearer Token / JWT)
3. `CF-Connecting-IP` / `X-Forwarded-For` (First IP in comma-separated list)
4. Fallback: Raw `http.Request.RemoteAddr` (parsed via `net.SplitHostPort` for IPv4 & IPv6)

---

## 3. Repository Layout & Package Responsibilities

```text
aegis/
├── api/
│   └── proto/          # Protocol buffer definitions (.proto) and generated Go files (*.pb.go)
├── cmd/
│   └── proxy/          # Application entrypoint (main.go), lifecycle management, and signal trapping
├── pkg/
│   ├── config/         # Configuration schema, YAML loader, and validation (pkg/config)
│   ├── crdt/           # G-Counter and Sliding Window implementation with eviction (pkg/crdt)
│   ├── gossip/         # UDP listener, periodic broadcaster, peer discovery, and state merging (pkg/gossip)
│   └── shield/         # Guard interface, TCP Vegas adaptive concurrency, and middleware (pkg/shield)
├── config.yaml         # Default runtime configuration
├── go.mod              # Go module definition
├── go.sum              # Go checksums
├── Makefile            # Automation commands for protoc, tests, and builds
├── README.md           # Project introduction and overview
└── GEMINI.md           # AI Agent Context & Instruction Manual
```

---

## 4. Development Standards & Coding Rules

### Concurrency & Memory Safety
* Always protect shared CRDT counters with `sync.RWMutex` or `sync/atomic` primitives.
* Keep write-locks (`Lock()`) as short as possible. Use read-locks (`RLock()`) for local limit evaluation and gossip broadcasts.
* Never block HTTP request evaluation on UDP network calls. Gossip broadcasts must execute strictly in background goroutines.

### Networking & Serialization
* Use Protocol Buffers for all UDP payloads to minimize packet overhead.
* Handle UDP datagram truncation gracefully by capping buffer sizes (e.g., standard safe MTU limit of 1400 bytes to avoid IP fragmentation) or dynamic chunking.
* Support dynamic peer bootstrapping via the `Peers` configuration list and Kubernetes DNS resolution.

### Error Handling & Dependencies
* Follow standard Go error wrapping: `fmt.Errorf("context: %w", err)`.
* Minimize third-party dependencies. Rely on Go's standard library (`net`, `net/http/httputil`, `sync`) whenever possible.
* Approved external packages: `go.yaml.in/yaml/v4`, `google.golang.org/protobuf`.

---

## 5. Build, Test, & Tooling Commands

### Protobuf Generation
```bash
protoc --go_out=. --go_opt=paths=source_relative api/proto/state.proto
```

### Unit & Race Detection Tests
```bash
go test -v -race ./pkg/...
```

### Local Execution via Makefile
```bash
make test
make build
make run
```

### Cross-Compilation (Bare-Metal Binaries)
```bash
# Linux AMD64
GOOS=linux GOARCH=amd64 go build -o bin/aegis-linux-amd64 cmd/proxy/main.go

# macOS ARM64
GOOS=darwin GOARCH=arm64 go build -o bin/aegis-darwin-arm64 cmd/proxy/main.go
```

---

## 6. Phased Roadmap

- [x] **Phase 1A: Setup & Config** - Project scaffolding, YAML parser, validation, and CLI flags (`pkg/config`).
- [x] **Phase 1B: Protobuf Schema** - Define `state.proto` with G-Counter vector clocks and compile Go stubs.
- [x] **Phase 1C: CRDT Engine** - Thread-safe G-Counter, Sliding Window, memory eviction, and race tests (`pkg/crdt`).
- [x] **Phase 1D: UDP Gossip** - Outbound broadcast ticker and inbound UDP listener with state merging (`pkg/gossip`).
- [x] **Phase 1E: Protection Shield** - Universal Guard pattern, TCP Vegas adaptive concurrency, and 429/503 middleware (`pkg/shield`).
- [ ] **Phase 2: Containerization** - Multi-stage distroless `Dockerfile` and local multi-node `docker-compose.yaml`.
- [ ] **Phase 3: Chaos & Benchmarking** - Toxiproxy partition tests and Vegeta latency reports ($p_{50}, p_{99}$).
- [ ] **Phase 4: Cloud-Native Delivery** - Kubernetes manifests (Sidecar & Gateway modes) and Helm chart packaging.

---

## 7. Instructions for AI Coding Agents
* Always reference the specific file path when creating or modifying code.
* Ensure all Go code compiles without unresolved imports before providing code blocks.
* Do not introduce breaking changes to the Protobuf schema once defined.
* Adhere strictly to the current phase checklist item before moving to subsequent architectural components.
