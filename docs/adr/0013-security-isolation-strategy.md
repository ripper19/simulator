# ADR-013: Security/isolation strategy

## Status
Accepted

## Context
The platform exposes a network API with auth and runs arbitrary user code
(models). It must not leak data or let a model compromise the host.

## Decision
- Transport/API hardening: JWT access/refresh tokens (argon2id, kind-tagged),
  RBAC (`USER`/`ADMIN`), per-user ownership on every simulation route, Redis
  rate limiting on expensive endpoints, request-size limits, timeouts, and
  structured errors (401/403/404/409/429).
- Secrets never live in committed code: local dev uses a gitignored `.env`;
  compose/Helm/CI use placeholders or env substitution.
- Trusted, compiled-in models run in-process (fast, deterministic). Arbitrary
  user-defined models are out of scope for in-process execution.

## Consequences
- No in-process execution of untrusted models; the safe extension path for those
  is a separate, resource-limited gRPC model worker (future work).
- Rate limiting and idempotency fail open on Redis outages, trading strictness
  for availability (documented).
- Ownership is enforced in a single middleware, so IDOR is prevented uniformly.
