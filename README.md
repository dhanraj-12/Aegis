# Aegis

Aegis is a high-performance, decentralized distributed rate limiter built in Go.

Traditional rate limiters often rely on centralized data stores (like Redis), which can introduce latency and become bottlenecks in the critical path of API gateways.
Aegis eliminates this single point of failure by evaluating rate limits entirely in-memory and sharing state across nodes through peer-to-peer communication.

## Core Concepts

* **Decentralized State:** Pairs Conflict-free Replicated Data Types (CRDTs) with an asynchronous background UDP gossip protocol to aggregate rate limit data across the cluster.
* **Extreme Performance:** Designed for sub-millisecond, in-memory rate-limit evaluation, ensuring your API gateway is never blocked by a database lookup.
* **Eventual Consistency:** Nodes efficiently broadcast their local time-bucketed counts to peers in the background, merging state seamlessly without distributed locking.

## How It Works

Aegis uses a **Sliding Window Counter** built on top of time-bucketed G-Counters (a type of CRDT).
Each node maintains its own local counters and periodically gossips its state to peers over UDP using Protocol Buffers.
When a rate-limit check happens, the sliding window algorithm calculates a weighted estimate from the current and previous time buckets to decide whether to allow or reject (HTTP 429) the request.

## Project Status

Aegis is in active development.
The following milestones have been completed:

- [x] Project scaffolding, YAML config loader, and entrypoint (`cmd/proxy`)
- [x] Protobuf schema for G-Counter vector clocks and gossip payloads (`api/proto/state.proto`)
- [x] Thread-safe G-Counter and Sliding Window rate limiter (`pkg/crdt`)

Currently working towards:

- [x] UDP gossip protocol for peer state synchronization (`pkg/gossip`)
- [ ] Reverse proxy with HTTP 429 enforcement (`pkg/proxy`)

---
*See [GEMINI.md](GEMINI.md) for the full development guide, architecture details, and coding standards.*
