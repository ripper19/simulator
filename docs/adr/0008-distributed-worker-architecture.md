# ADR-008: Distributed worker architecture

## Status
Accepted

## Context
Simulation jobs must run asynchronously on a horizontally scalable worker pool,
while the same model also runs in-process without changes.

## Decision
- A `Broker` interface (Publish/Consume with ack/nack) abstracts the message
  layer; RabbitMQ is the production implementation and an in-memory broker
  serves local dev and tests.
- Jobs are JSON messages carrying a `RunJobPayload` (model, seed, mode, bounds,
  config). Workers consume jobs, instantiate the model from the compiled-in
  registry, run it, and publish a `Result` (with snapshot).
- Retry/DLQ/idempotency live in the queue layer: failures are re-published with
  exponential backoff up to `MaxAttempts`, then dead-lettered; a Redis
  `job:processed:<id>` claim (released on failure) ensures at-most-once
  execution even under duplicate delivery.
- Workers register and heartbeat via Redis; liveness is TTL-based.

## Consequences
- The same compiled-in model runs in-process (API), as a local worker, or across
  a worker cluster with no model changes.
- Workers are stateless and scale out; the broker/DB are the shared bottlenecks.
- Trade-off: workers hold a full model registry and run models in-process (not
  isolated); untrusted models are a P13/ADR-013 concern.
