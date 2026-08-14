# ADR-009: Message broker choice

## Status
Accepted

## Context
Job dispatch and worker communication need a broker. Candidates: RabbitMQ,
Kafka, NATS, Redis streams.

## Decision
Use RabbitMQ, behind the `Broker` interface.

## Consequences
- Native job-queue semantics (acks/nacks/requeue, QoS, dead-letter exchanges)
  fit the retry/DLQ/idempotency requirements without custom offset management.
- Light enough to run locally (sandboxed) and in Docker for the compose stack.
- The `Broker` interface keeps Kafka/NATS as a drop-in replacement if the
  workload grows to high-throughput log-style streams, which the current
  job-dispatch pattern does not need.
