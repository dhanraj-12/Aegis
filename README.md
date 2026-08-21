# Aegis

Aegis is a high-performance, decentralized distributed rate limiter built in Go. 

Traditional rate limiters often rely on centralized data stores (like Redis), which can introduce latency and become bottlenecks in the critical path of API gateways. Aegis eliminates this single point of failure by evaluating rate limits entirely in-memory and sharing state across nodes through peer-to-peer communication.

## Core Concepts

* **Decentralized State:** Pairs Conflict-free Replicated Data Types (CRDTs) with an asynchronous background UDP gossip protocol to aggregate rate limit data across the cluster.
* **Extreme Performance:** Designed for sub-millisecond, in-memory rate-limit evaluation, ensuring your API gateway is never blocked by a database lookup.
* **Eventual Consistency:** Nodes efficiently broadcast their local time-bucketed counts to peers in the background, merging state seamlessly without distributed locking.

---
*Note: Aegis is currently in active development. Features, architecture, and deployment instructions will be expanded as the project evolves.*
