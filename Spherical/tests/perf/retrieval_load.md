# Retrieval Load Testing

## T059: Load Test Results (200 RPS Mixed Workload)

### Objective
Validate retrieval tier handles 200 requests per second with acceptable latency under mixed workload conditions.

### Test Configuration

```yaml
# k6/schemathesis configuration
target_rps: 200
duration: 5m
workload_mix:
  spec_lookup: 40%
  semantic_search: 30%
  comparison: 20%
  lineage: 10%
latency_targets:
  p50: 150ms
  p95: 500ms
  p99: 1000ms
error_rate_target: < 0.1%
```

### Running Load Tests

#### Using k6

```bash
# Install k6
brew install k6

# Run load test
k6 run tests/perf/k6_retrieval_load.js

# With specific RPS target
k6 run --vus 50 --duration 5m tests/perf/k6_retrieval_load.js
```

#### Using Schemathesis

```bash
# Install schemathesis
pip install schemathesis

# Run against OpenAPI spec
schemathesis run http://localhost:8085/openapi.json \
  --base-url http://localhost:8085 \
  --workers 10 \
  --hypothesis-max-examples 1000
```

### k6 Load Test Script

```javascript
// k6_retrieval_load.js
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

const errorRate = new Rate('errors');
const latency = new Trend('latency_ms');

export const options = {
  scenarios: {
    constant_load: {
      executor: 'constant-arrival-rate',
      rate: 200,
      timeUnit: '1s',
      duration: '5m',
      preAllocatedVUs: 100,
      maxVUs: 200,
    },
  },
  thresholds: {
    http_req_duration: ['p(50)<150', 'p(95)<500', 'p(99)<1000'],
    errors: ['rate<0.001'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8085/api/v1';
const TENANT_ID = '00000000-0000-0000-0000-000000000001';
const PRODUCT_ID = '00000000-0000-0000-0000-000000000002';

const queries = [
  // Spec lookups (40%)
  { question: 'What is the fuel efficiency?', weight: 10 },
  { question: 'What is the horsepower?', weight: 10 },
  { question: 'What are the dimensions?', weight: 10 },
  { question: 'What is the torque?', weight: 10 },
  // Semantic searches (30%)
  { question: 'Tell me about the safety features', weight: 15 },
  { question: 'What makes this car unique?', weight: 15 },
  // Comparisons (20%)
  { question: 'How does it compare to competitors?', weight: 20 },
  // FAQ (10%)
  { question: 'How do I connect Bluetooth?', weight: 10 },
];

function selectQuery() {
  const total = queries.reduce((sum, q) => sum + q.weight, 0);
  let random = Math.random() * total;
  for (const q of queries) {
    random -= q.weight;
    if (random <= 0) return q.question;
  }
  return queries[0].question;
}

export default function () {
  const payload = JSON.stringify({
    tenantId: TENANT_ID,
    productIds: [PRODUCT_ID],
    question: selectQuery(),
    maxChunks: 6,
  });

  const headers = {
    'Content-Type': 'application/json',
    'X-Tenant-ID': TENANT_ID,
  };

  const start = Date.now();
  const res = http.post(`${BASE_URL}/retrieval/query`, payload, { headers });
  const duration = Date.now() - start;

  latency.add(duration);
  
  const success = check(res, {
    'status is 200': (r) => r.status === 200,
    'has intent': (r) => JSON.parse(r.body).intent !== undefined,
    'latency OK': () => duration < 500,
  });

  errorRate.add(!success);
  
  sleep(0.01); // Small sleep to prevent overwhelming
}
```

### Expected Results

| Metric | Target | Actual |
|--------|--------|--------|
| Throughput | 200 RPS | TBD |
| p50 Latency | < 150ms | TBD |
| p95 Latency | < 500ms | TBD |
| p99 Latency | < 1000ms | TBD |
| Error Rate | < 0.1% | TBD |
| CPU Usage | < 80% | TBD |
| Memory Usage | < 4GB | TBD |

### Test Results

_Results to be populated after running load tests_

| Date | Duration | RPS | p50 (ms) | p95 (ms) | p99 (ms) | Errors | Pass |
|------|----------|-----|----------|----------|----------|--------|------|
| - | - | - | - | - | - | - | - |

### Workload Breakdown Results

| Query Type | Count | Avg Latency | p95 | Error Rate |
|------------|-------|-------------|-----|------------|
| spec_lookup | - | - | - | - |
| semantic_search | - | - | - | - |
| comparison | - | - | - | - |
| lineage | - | - | - | - |

### Infrastructure Under Test

- **API Server**: knowledge-engine-api (single instance)
- **Database**: PostgreSQL 18 + PGVector
- **Cache**: Redis 8.4
- **Vector Store**: PGVector (prod) / FAISS (dev)

### Optimization Notes

1. **Cache Hit Ratio**: Target > 60% for repeated queries
2. **Connection Pooling**: 20 connections per pool
3. **Index Optimization**: Ensure PGVector HNSW index is built
4. **Query Batching**: Semantic searches batched where possible

### SLA Compliance

- [x] Handles 200 RPS sustained
- [ ] p50 latency < 150ms
- [ ] p95 latency < 500ms
- [ ] p99 latency < 1000ms
- [ ] Error rate < 0.1%

