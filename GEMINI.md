# GEMINI.md - Aegis Development Guide & Agent Instructions

## 1. Project Overview
**Aegis** is a high-performance, decentralized distributed rate limiter built in Go[cite: 1]. It eliminates centralized databases (such as Redis) from the critical path of API gateways by pairing Conflict-free Replicated Data Types (CRDTs) with an asynchronous background UDP gossip protocol[cite: 1].

* **Core Goal:** Sub-millisecond, in-memory rate-limit evaluation with eventual consistency across distributed nodes[cite: 1].
* **Primary Language:** Go (Golang)[cite: 1]
* **Serialization:** Protocol Buffers (Protobuf)[cite: 1]
* **Transport:** UDP for peer gossip[cite: 1], HTTP for reverse proxy routing.
* **Testing & Verification:** Chaos testing via Toxiproxy and throughput benchmarking via Vegeta[cite: 1].

---

## 2. Architecture & Algorithms

### Rate Limiting Algorithm
* **Phase 1 Implementation:** Sliding Window Counter.
* **CRDT Model:** Time-Bucketed Grow-Only Counters (Map of G-Counters by time window)[cite: 1].
* **State Convergence:** The CRDT merge function must be idempotent, commutative, and monotonic:
  $$\text{Merged}[i] = \max(\text{Local}[i], \text{Remote}[i])$$

### Dynamic Key Extraction Hierarchy
To avoid the Shared IP / NAT problem, extract request identifiers in the following strict order:
1. `X-API-Key` header
2. `Authorization` header (Bearer Token / JWT)
3. `X-Forwarded-For` / `CF-Connecting-IP` (First IP in comma-separated list)
4. Fallback: Raw `http.Request.RemoteAddr`

---

## 3. Repository Layout & Package Responsibilities

```text
aegis/
├── api/
│   └── proto/          # Protocol buffer definitions (.proto) and generated Go files (*.pb.go)
├── cmd/
│   └── proxy/          # Application entrypoint (main.go), lifecycle management, and signal trapping
├── pkg/
│   ├── config/         # Configuration schema, YAML loader, and environment variable bindings
│   ├── crdt/           # G-Counter and Sliding Window implementation (thread-safe, sync.RWMutex)
│   ├── gossip/         # UDP listener, periodic broadcaster, peer discovery, and state merging
│   └── proxy/          # httputil.ReverseProxy handler, middleware, and 429 response enforcement
├── config.yaml         # Default runtime configuration
├── go.mod              # Go module definition
├── go.sum              # Go checksums
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

### Local Bare-Metal Execution
```bash
go run cmd/proxy/main.go
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

- [x] **Phase 1A: Setup & Config** — Project scaffolding, YAML parser (`pkg/config`), and baseline CLI flags.
- [x] **Phase 1B: Protobuf Schema** - Define `state.proto` with G-Counter vector clocks and compile Go stubs.
- [x] **Phase 1C: CRDT Engine** - Thread-safe G-Counter and Sliding Window logic with unit tests (`pkg/crdt`).
- [ ] **Phase 1D: UDP Gossip** — Outbound broadcast ticker and inbound UDP listener with state merging (`pkg/gossip`).
- [ ] **Phase 1E: Reverse Proxy** — `httputil.ReverseProxy` integration, middleware, and `HTTP 429` enforcement (`pkg/proxy`).
- [ ] **Phase 2: Containerization** — Multi-stage distroless `Dockerfile` and local multi-node `docker-compose.yaml`.
- [ ] **Phase 3: Chaos & Benchmarking** — Toxiproxy partition tests and Vegeta latency reports ($p_{50}, p_{99}$).
- [ ] **Phase 4: Cloud-Native Delivery** — Kubernetes manifests (Sidecar & Gateway modes) and Helm chart packaging.

---

## 7. Instructions for AI Coding Agents
* Always reference the specific file path when creating or modifying code.
* Ensure all Go code compiles without unresolved imports before providing code blocks.
* Do not introduce breaking changes to the Protobuf schema once defined.
* Adhere strictly to the current phase checklist item before moving to subsequent architectural components.
