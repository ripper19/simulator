# Distributed execution

```mermaid
sequenceDiagram
    participant C as Client
    participant A as API
    participant B as Broker
    participant W as Worker
    participant D as PostgreSQL
    C->>A: POST /simulations (create)
    A->>D: persist record
    A->>B: publish run job
    B->>W: deliver job
    W->>W: run simulation (compiled-in model)
    W->>B: publish result (status + snapshot)
    A->>D: update status
    C->>A: GET /simulations/{id}/state
```

## Jobs

A `Job` carries a `RunJobPayload` (model ID/version, seed, mode, bounds,
config). The queue layer adds:

- **Retry** with exponential backoff up to `MaxAttempts`.
- **Dead-lettering** to a DLQ after the final attempt.
- **Idempotency**: a Redis `job:processed:<id>` claim (released on failure, kept
  24h on success) guarantees at-most-once execution under duplicate delivery.

## Workers

Workers register (metadata) and heartbeat (TTL) in Redis, then consume jobs
with RabbitMQ QoS=1 (one job at a time). They instantiate the model from the
compiled-in registry, so the same model runs in-process, as a local worker, or
across a cluster without changes.

## Failure handling

| Failure | Handling |
|---------|----------|
| Worker crash mid-job | message unacked → redelivered → retry/DLQ |
| Duplicate delivery | idempotency claim drops duplicates |
| Broker connection drop | reconnect with bounded exponential backoff |
| Redis unavailable | heartbeats/idempotency/rate-limit fail open |
| Simulation timeout/error | job result records `failed` status |
