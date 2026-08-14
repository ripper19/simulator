# ADR-011: Redis choice

## Status
Accepted

## Context
Transient/distributed coordination (heartbeats, idempotency claims, rate
limiting) needs a fast, shared store that is not the primary persistent state.

## Decision
Use Redis (protocol-compatible server) via `go-redis/v9` for transient
coordination only.

## Consequences
- Heartbeats use TTL keys; idempotency uses `SETNX` claims (released on failure);
  rate limiting uses an atomic Lua `INCR`+`EXPIRE` fixed window.
- Redis is intentionally NOT the source of truth: durable state stays in
  PostgreSQL; losing Redis degrades coordination but not data.
- Rate limiting and idempotency fail open on Redis errors (availability over
  strictness), documented.
