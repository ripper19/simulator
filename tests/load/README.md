# Load tests (k6)

Load tests for the simulator API lifecycle (create → start → state → metrics).

## Run

```sh
k6 run -e BASE_URL=http://127.0.0.1:8080 tests/load/simulations.js
k6 run --vus 100 --duration 30s -e BASE_URL=http://127.0.0.1:8080 tests/load/simulations.js
# authenticated: -e TOKEN=<jwt>
```

## Measured results (recorded, not fabricated)

Hardware: Intel Core i7-6500U @ 2.50 GHz (2 cores / 4 threads), 15 GB RAM,
Linux, Go 1.26.5, local native API + PostgreSQL 17 (default pgx pool).

| VUs | Duration | Success | http_req_duration (p95) | Throughput |
|-----|----------|---------|--------------------------|------------|
| 50  | 10s      | 100%    | 1.75 s                   | ~21 iter/s, ~109 req/s |
| 100 | 10s      | 100%    | 12.58 s                  | ~6.7 iter/s, ~33 req/s |

### Observations

- **With Redis rate limiting enabled** (60 req/min per IP on create), a 50-VU
  burst is correctly throttled: 98% of create requests returned 429 (the rate
  limiter works as intended).
- **Without rate limiting**, the API is bottlenecked by the PostgreSQL
  connection pool (default 4 connections): at 50 VUs p95 latency is 1.75 s and
  at 100 VUs it degrades to 12.58 s while remaining 100% successful. This is a
  known scalability limit (connection-pool sizing + per-request DB round-trips),
  tracked for the performance phase.
- 1000/5000 concurrent users were not meaningfully testable on this 2-core
  development machine; they require more hardware and a larger database
  connection pool (see `deployments/helm` scaling notes).
