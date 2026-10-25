# Aegis

Aegis is a high-performance, decentralized distributed rate limiter and load shedding reverse proxy built in Go.

Traditional rate limiters often rely on centralized data stores (like Redis), which introduce latency and become bottlenecks in the critical path of API gateways.
Aegis eliminates this single point of failure by evaluating rate limits entirely in-memory and sharing state across nodes through peer-to-peer communication.
In addition, Aegis incorporates local adaptive concurrency limiting (TCP Vegas gradient) to shed load and prevent cascading backend worker exhaustion.

## Core Concepts

* **Decentralized State:** Pairs Conflict-free Replicated Data Types (CRDTs) with an asynchronous background UDP gossip protocol to aggregate rate limit data across the cluster.
* **Extreme Performance:** Designed for sub-millisecond, in-memory rate-limit evaluation, ensuring your API gateway is never blocked by a database lookup.
* **Dual Protection Shield:** Combines cluster-wide velocity rate limiting (requests/minute) with local TCP Vegas adaptive concurrency control (in-flight request load shedding).
* **Eventual Consistency:** Nodes efficiently broadcast their local time-bucketed counts to peers in the background, merging state seamlessly without distributed locking.

## How It Works

Aegis provides two complementary layers of protection orchestrated via a pluggable [`Guard`](file:///home/dj/Drive2/aegis/pkg/shield/shield.go#L17) interface:

1. **Velocity Rate Limiting (`pkg/crdt`):** Uses a **Sliding Window Counter** built on top of time-bucketed G-Counters (a CRDT).
   Each node maintains its own local counters and periodically gossips its state to peers over UDP using Protocol Buffers.
   When a rate-limit check happens, the sliding window algorithm calculates a weighted estimate from the current and previous time buckets to decide whether to allow or reject (`HTTP 429 Too Many Requests`).

2. **Adaptive Concurrency Limiting (`pkg/shield`):** Uses the **TCP Vegas gradient queue-delay algorithm** to dynamically measure upstream round-trip latency (RTT).
   If queue backlogs build up because backend dependencies are slow, Aegis dynamically sheds excess load (`HTTP 503 Service Unavailable`) before worker pools exhaust.

---

## Architecture & System Design

### 1. Traditional Approaches vs Decentralized Model

Rate limiters have historically fallen into two primary categories, both of which exhibit significant engineering trade-offs:

1. **Non-Distributed (Local In-Memory Counter):**
   Each node maintains its own independent in-memory counter.
   While latency is near-zero, limits are fragmented across the cluster.
   In a cluster of $N$ nodes behind a round-robin load balancer, an attacker can consume $N \times \text{Limit}$ requests before being throttled.
   Uneven traffic distribution causes inaccurate rate enforcement.

2. **Centralized Distributed (Redis / Memcached):**
   Gateway nodes query a centralized in-memory datastore synchronously on every incoming request.
   While global accuracy is achieved, every HTTP request pays a network round-trip penalty (typically 1-5ms+).
   The centralized store becomes a single point of failure (SPOF) and an operational bottleneck under high traffic spikes.

```mermaid
%%{init: {'theme': 'dark', 'themeVariables': { 'darkMode': true, 'background': '#09090b', 'mainBkg': '#18181b', 'primaryColor': '#18181b', 'primaryBorderColor': '#ffffff', 'primaryTextColor': '#ffffff', 'lineColor': '#ffffff', 'textColor': '#ffffff', 'nodeBorder': '#ffffff', 'clusterBkg': '#09090b', 'clusterBorder': '#ffffff', 'edgeLabelBackground': '#18181b' }}}%%
flowchart TD
    classDef dark fill:#18181b,stroke:#ffffff,stroke-width:1.5px,color:#ffffff;

    subgraph NonDist["Approach 1: Non-Distributed (In-Memory per Node)"]
        direction TB
        C1["Client Requests"]:::dark --> LB1["Load Balancer"]:::dark
        LB1 --> N1["Gateway Node 1 (Local Counter)"]:::dark
        LB1 --> N2["Gateway Node 2 (Local Counter)"]:::dark
        LB1 --> N3["Gateway Node 3 (Local Counter)"]:::dark
        N1 --> B1["Backend Service"]:::dark
        N2 --> B1
        N3 --> B1
    end

    subgraph CentralDist["Approach 2: Centralized Distributed (Redis / Datastore)"]
        direction TB
        C2["Client Requests"]:::dark --> LB2["Load Balancer"]:::dark
        LB2 --> GN1["Gateway Node 1"]:::dark
        LB2 --> GN2["Gateway Node 2"]:::dark
        LB2 --> GN3["Gateway Node 3"]:::dark
        GN1 <-->|"Sync TCP RTT (1-5ms+)"| REDIS[("Central Redis Cluster")]:::dark
        GN2 <-->|"Sync TCP RTT (1-5ms+)"| REDIS
        GN3 <-->|"Sync TCP RTT (1-5ms+)"| REDIS
        GN1 --> B2["Backend Service"]:::dark
        GN2 --> B2
        GN3 --> B2
    end

    style NonDist fill:#09090b,stroke:#ffffff,stroke-width:1.5px,color:#ffffff
    style CentralDist fill:#09090b,stroke:#ffffff,stroke-width:1.5px,color:#ffffff
    linkStyle default stroke:#ffffff,stroke-width:1.5px,color:#ffffff
```

---

### 2. Aegis High-Level Design (HLD)

Aegis decouples rate limit evaluation from cross-node synchronization:

* **The Critical Path (<1ms):** Requests are evaluated purely in-memory on the receiving node using local CRDT state and adaptive concurrency gates.
  No synchronous database queries or distributed locks block the HTTP proxy path.
* **The Background Path:** Nodes continuously gossip state deltas to peers asynchronously over UDP using Protocol Buffers.
* **Convergence:** The CRDT state convergence guarantees eventual consistency across all nodes without centralized coordination.

```mermaid
%%{init: {'theme': 'dark', 'themeVariables': { 'darkMode': true, 'background': '#09090b', 'mainBkg': '#18181b', 'primaryColor': '#18181b', 'primaryBorderColor': '#ffffff', 'primaryTextColor': '#ffffff', 'lineColor': '#ffffff', 'textColor': '#ffffff', 'nodeBorder': '#ffffff', 'clusterBkg': '#09090b', 'clusterBorder': '#ffffff', 'edgeLabelBackground': '#18181b' }}}%%
flowchart TD
    classDef dark fill:#18181b,stroke:#ffffff,stroke-width:1.5px,color:#ffffff;
    classDef innerDark fill:#27272a,stroke:#ffffff,stroke-width:1.5px,color:#ffffff;

    Clients["Client Requests (Web / Mobile / Third-Party)"]:::dark --> LB["Load Balancer / Ingress"]:::dark

    subgraph AegisCluster["Aegis Distributed Cluster (Decentralized P2P Mesh)"]
        direction LR

        subgraph Node1["Aegis Node 1"]
            direction TB
            Proxy1["Reverse Proxy (pkg/shield)"]:::innerDark
            CRDT1[("In-Memory CRDT Store")]:::innerDark
            Proxy1 --- CRDT1
        end

        subgraph Node2["Aegis Node 2"]
            direction TB
            Proxy2["Reverse Proxy (pkg/shield)"]:::innerDark
            CRDT2[("In-Memory CRDT Store")]:::innerDark
            Proxy2 --- CRDT2
        end

        subgraph Node3["Aegis Node 3"]
            direction TB
            Proxy3["Reverse Proxy (pkg/shield)"]:::innerDark
            CRDT3[("In-Memory CRDT Store")]:::innerDark
            Proxy3 --- CRDT3
        end

        Node1 <-->|"Async UDP Gossip (Protobuf)"| Node2
        Node2 <-->|"Async UDP Gossip (Protobuf)"| Node3
        Node3 <-->|"Async UDP Gossip (Protobuf)"| Node1
    end

    LB -->|"HTTP Request"| Proxy1
    LB -->|"HTTP Request"| Proxy2
    LB -->|"HTTP Request"| Proxy3

    Proxy1 -->|"Forward Allowed Traffic (<1ms)"| Backend["Upstream Backend Service"]:::dark
    Proxy2 -->|"Forward Allowed Traffic (<1ms)"| Backend
    Proxy3 -->|"Forward Allowed Traffic (<1ms)"| Backend

    Proxy1 -.->|"429 Rate Limit / 503 Concurrency Shed"| Clients
    Proxy2 -.->|"429 Rate Limit / 503 Concurrency Shed"| Clients
    Proxy3 -.->|"429 Rate Limit / 503 Concurrency Shed"| Clients

    style AegisCluster fill:#09090b,stroke:#ffffff,stroke-width:1.5px,color:#ffffff
    style Node1 fill:#18181b,stroke:#ffffff,stroke-width:1px,color:#ffffff
    style Node2 fill:#18181b,stroke:#ffffff,stroke-width:1px,color:#ffffff
    style Node3 fill:#18181b,stroke:#ffffff,stroke-width:1px,color:#ffffff
    linkStyle default stroke:#ffffff,stroke-width:1.5px,color:#ffffff
```

---

### 3. Aegis Low-Level Design (LLD)

Internally, an Aegis node executes a unified protection pipeline orchestrated by [`pkg/shield`](file:///home/dj/Drive2/aegis/pkg/shield):

1. **Synchronous Request Processing:**
   * Inbound HTTP requests pass through [`ProtectionMiddleware`](file:///home/dj/Drive2/aegis/pkg/shield/middleware.go#L62).
   * Identifier and limit are extracted using the 4-tier hierarchy:
     `X-API-Key` -> `Authorization: Bearer` -> `CF-Connecting-IP` / `X-Forwarded-For` -> `net.SplitHostPort(RemoteAddr)`.
   * [`Guard.Acquire`](file:///home/dj/Drive2/aegis/pkg/shield/shield.go#L20) evaluates protection rules based on configured `Protection.Mode`:
     * **Rate Limiting:** Checks sliding window counter across time buckets via [`crdt.Limiter`](file:///home/dj/Drive2/aegis/pkg/crdt/limiter.go).
       Rejects with `HTTP 429 Too Many Requests` if velocity threshold is exceeded.
     * **Concurrency Limiting:** Checks active in-flight count against the dynamic limit via [`AdaptiveConcurrencyGuard`](file:///home/dj/Drive2/aegis/pkg/shield/concurrency.go).
       Rejects with `HTTP 503 Service Unavailable` if concurrency ceiling is reached.
   * If permitted, the request is forwarded to the upstream backend.
   * The deferred `DoneFunc` records response status and elapsed round-trip latency (RTT) to dynamically adjust Vegas limits.

2. **Asynchronous UDP Gossip Engine:**
   * **Broadcaster:** Every 500ms, [`gossip.Engine`](file:///home/dj/Drive2/aegis/pkg/gossip/gossip.go#L14) snapshots active non-stale window counters, serializes them to Protobuf [`GossipPayload`](file:///home/dj/Drive2/aegis/api/proto/state.proto#L12), and sends UDP datagrams to configured peers.
   * **Listener:** Inbound UDP datagrams on `GossipPort` are decoded from Protobuf and merged into local state using the commutative max rule:
     $\text{local}[i] = \max(\text{local}[i], \text{remote}[i])$.

```mermaid
%%{init: {'theme': 'dark', 'themeVariables': { 'darkMode': true, 'background': '#09090b', 'mainBkg': '#18181b', 'primaryColor': '#18181b', 'primaryBorderColor': '#ffffff', 'primaryTextColor': '#ffffff', 'lineColor': '#ffffff', 'textColor': '#ffffff', 'nodeBorder': '#ffffff', 'clusterBkg': '#09090b', 'clusterBorder': '#ffffff', 'edgeLabelBackground': '#18181b' }}}%%
flowchart TD
    classDef dark fill:#18181b,stroke:#ffffff,stroke-width:1.5px,color:#ffffff;
    classDef innerDark fill:#27272a,stroke:#ffffff,stroke-width:1.5px,color:#ffffff;
    classDef highlight fill:#3f3f46,stroke:#ffffff,stroke-width:2px,color:#ffffff;

    Client["HTTP Client Request"]:::dark
    Resp429["HTTP 429 Too Many Requests"]:::dark
    Resp503["HTTP 503 Service Unavailable"]:::dark
    Upstream["Upstream Backend API"]:::dark

    subgraph AegisNode["Aegis Node Internals"]
        direction TB

        subgraph ShieldPipeline["1. Protection Shield Pipeline (pkg/shield)"]
            Extract["Extract Identifier (API Key, Auth, IP)"]:::innerDark
            
            subgraph ChainGuard["Guard Engine (pkg/shield)"]
                RateGuard["RateLimitGuard (crdt.Limiter)"]:::innerDark
                VegasGuard["AdaptiveConcurrencyGuard (TCP Vegas)"]:::innerDark
            end

            Decision{"Allowed?"}:::highlight
        end

        subgraph BackgroundPath["2. Gossip Engine (pkg/gossip) (Asynchronous, Background UDP)"]
            direction TB

            subgraph OutboundBroadcast["Outbound Broadcaster"]
                Ticker["Ticker (500ms)"]:::innerDark
                SnapShot["Limiter.SnapShot() (Active Windows)"]:::innerDark
                ProtoEncode["Protobuf Marshal (GossipPayload)"]:::innerDark
                UDPSend["UDP Socket Broadcast"]:::innerDark
            end

            subgraph InboundListen["Inbound Listener"]
                UDPListen["UDP Listener (:GossipPort)"]:::innerDark
                ProtoDecode["Protobuf Unmarshal"]:::innerDark
                MergeLogic["Limiter.MergeState: max(local, remote)"]:::innerDark
            end
        end
    end

    Client -->|"HTTP Request"| Extract
    Extract --> RateGuard
    RateGuard --> VegasGuard
    VegasGuard --> Decision
    Decision -->|"Rate Limit Exceeded"| Resp429
    Decision -->|"Concurrency Saturated"| Resp503
    Decision -->|"Yes (Increment In-Flight & Local Count)"| Upstream

    Upstream -.->|"Response RTT Latency"| VegasGuard

    Ticker -->|"Every 500ms"| SnapShot
    SnapShot --> ProtoEncode
    ProtoEncode --> UDPSend
    UDPSend -->|"UDP Datagram"| RemotePeers["Remote Aegis Peer Nodes"]:::dark

    RemotePeers -->|"UDP Datagram"| UDPListen
    UDPListen --> ProtoDecode
    ProtoDecode --> MergeLogic

    style AegisNode fill:#09090b,stroke:#ffffff,stroke-width:1.5px,color:#ffffff
    style ShieldPipeline fill:#18181b,stroke:#ffffff,stroke-width:1px,color:#ffffff
    style ChainGuard fill:#21262d,stroke:#ffffff,stroke-width:1px,color:#ffffff
    style BackgroundPath fill:#18181b,stroke:#ffffff,stroke-width:1px,color:#ffffff
    style OutboundBroadcast fill:#27272a,stroke:#ffffff,stroke-width:1px,color:#ffffff
    style InboundListen fill:#27272a,stroke:#ffffff,stroke-width:1px,color:#ffffff
    linkStyle default stroke:#ffffff,stroke-width:1.5px,color:#ffffff
```

---

## Project Status

Aegis is in active development.
The following milestones have been completed:

- [x] Project scaffolding, YAML config loader, and entrypoint ([`cmd/proxy`](file:///home/dj/Drive2/aegis/cmd/proxy/main.go))
- [x] Protobuf schema for G-Counter vector clocks and gossip payloads ([`api/proto/state.proto`](file:///home/dj/Drive2/aegis/api/proto/state.proto))
- [x] Thread-safe G-Counter and Sliding Window rate limiter with memory eviction ([`pkg/crdt`](file:///home/dj/Drive2/aegis/pkg/crdt))
- [x] UDP gossip protocol for peer state synchronization ([`pkg/gossip`](file:///home/dj/Drive2/aegis/pkg/gossip))
- [x] Universal Guard pattern, TCP Vegas adaptive concurrency, and 429/503 enforcement ([`pkg/shield`](file:///home/dj/Drive2/aegis/pkg/shield))
- [x] Automated unit and race detection test suites (`make test`)

Currently working towards:

- [ ] Phase 2: Containerization (Distroless Dockerfile, multi-node `docker-compose.yaml`)
- [ ] Phase 3: Chaos Testing (Toxiproxy partition tests, Vegeta latency benchmarks)
- [ ] Phase 4: Cloud-Native Delivery (Kubernetes manifests, Helm chart)

---
*See [GEMINI.md](file:///home/dj/Drive2/aegis/GEMINI.md) for the full development guide, architecture details, and coding standards.*
