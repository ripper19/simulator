import http from 'k6/http';
import { check } from 'k6';

// Load test for the simulator API lifecycle endpoints.
//
// Run:
//   k6 run -e BASE_URL=http://127.0.0.1:8080 tests/load/simulations.js
// Scale:
//   k6 run --vus 100  --duration 30s -e BASE_URL=... tests/load/simulations.js
//   k6 run --vus 1000 --duration 30s -e BASE_URL=... tests/load/simulations.js
//
// Note: creation is rate-limited (60/min per IP) when the API runs with
// REDIS_ADDR set. For raw-throughput runs, start the API without REDIS_ADDR
// (or set RATE_LIMIT high) — see tests/load/README.md.
//
// Optionally authenticate by exporting a bearer token:
//   k6 run -e BASE_URL=... -e TOKEN=<jwt> tests/load/simulations.js

const BASE = __ENV.BASE_URL || 'http://127.0.0.1:8080';
const TOKEN = __ENV.TOKEN || '';

function headers(json) {
  const h = {};
  if (json) h['Content-Type'] = 'application/json';
  if (TOKEN) h['Authorization'] = 'Bearer ' + TOKEN;
  return h;
}

export const options = {
  thresholds: {
    http_req_failed: ['rate<0.05'],
    http_req_duration: ['p(95)<2000'],
  },
};

export default function () {
  const body = JSON.stringify({
    model_id: 'counter',
    seed: Math.floor(Math.random() * 1e9),
    max_ticks: 1000,
    config: { n: 100 },
  });

  const create = http.post(`${BASE}/api/v1/simulations`, body, { headers: headers(true) });
  check(create, { 'create 2xx': (r) => r.status === 201 });

  let id = '';
  try { id = JSON.parse(create.body).id; } catch (_) {}
  if (!id) return;

  http.post(`${BASE}/api/v1/simulations/${id}/start`, null, { headers: headers(false) });
  http.get(`${BASE}/api/v1/simulations/${id}/state`, { headers: headers(false) });
  http.get(`${BASE}/api/v1/simulations/${id}/metrics`, { headers: headers(false) });
  http.get(`${BASE}/api/v1/models`, { headers: headers(false) });
  // Stop and delete so simulations do not accumulate for the server's lifetime.
  http.post(`${BASE}/api/v1/simulations/${id}/stop`, null, { headers: headers(false) });
  http.del(`${BASE}/api/v1/simulations/${id}`, null, { headers: headers(false) });
}
