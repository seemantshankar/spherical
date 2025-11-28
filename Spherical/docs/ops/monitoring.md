# Knowledge Engine Monitoring & Observability

## T061: Monitoring Dashboards and Alert Rules

### Overview

The Knowledge Engine uses OpenTelemetry for distributed tracing, Prometheus for metrics, and structured logging for operational visibility.

### Metrics

#### Application Metrics

| Metric | Type | Description | Labels |
|--------|------|-------------|--------|
| `ke_retrieval_requests_total` | Counter | Total retrieval requests | `tenant_id`, `intent`, `status` |
| `ke_retrieval_latency_seconds` | Histogram | Retrieval response latency | `tenant_id`, `intent` |
| `ke_ingestion_jobs_total` | Counter | Total ingestion jobs | `tenant_id`, `status` |
| `ke_ingestion_duration_seconds` | Histogram | Ingestion job duration | `tenant_id` |
| `ke_cache_hits_total` | Counter | Cache hits | `cache_type` |
| `ke_cache_misses_total` | Counter | Cache misses | `cache_type` |
| `ke_embedding_requests_total` | Counter | Embedding API calls | `model` |
| `ke_embedding_latency_seconds` | Histogram | Embedding generation latency | `model` |
| `ke_comparison_queries_total` | Counter | Comparison queries | `tenant_id` |
| `ke_drift_alerts_total` | Counter | Drift alerts generated | `alert_type` |

#### Infrastructure Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `pg_connections_active` | Gauge | Active PostgreSQL connections |
| `pg_queries_slow_total` | Counter | Slow queries (> 100ms) |
| `redis_memory_used_bytes` | Gauge | Redis memory usage |
| `redis_keyspace_hits_total` | Counter | Redis cache hits |

### Prometheus Configuration

```yaml
# prometheus.yml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: 'knowledge-engine-api'
    static_configs:
      - targets: ['localhost:8085']
    metrics_path: /metrics

  - job_name: 'postgres'
    static_configs:
      - targets: ['localhost:9187']  # postgres_exporter

  - job_name: 'redis'
    static_configs:
      - targets: ['localhost:9121']  # redis_exporter
```

### Alert Rules

```yaml
# alerting_rules.yml
groups:
  - name: knowledge_engine
    rules:
      # High latency alert
      - alert: RetrievalHighLatency
        expr: histogram_quantile(0.95, rate(ke_retrieval_latency_seconds_bucket[5m])) > 0.5
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High retrieval latency"
          description: "p95 latency > 500ms for 5 minutes"

      # Error rate alert
      - alert: RetrievalHighErrorRate
        expr: rate(ke_retrieval_requests_total{status="error"}[5m]) / rate(ke_retrieval_requests_total[5m]) > 0.01
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "High error rate"
          description: "Error rate > 1% for 5 minutes"

      # Ingestion failure alert
      - alert: IngestionFailures
        expr: increase(ke_ingestion_jobs_total{status="failed"}[1h]) > 5
        for: 1m
        labels:
          severity: warning
        annotations:
          summary: "Multiple ingestion failures"
          description: "More than 5 ingestion failures in the last hour"

      # Cache hit rate alert
      - alert: LowCacheHitRate
        expr: rate(ke_cache_hits_total[5m]) / (rate(ke_cache_hits_total[5m]) + rate(ke_cache_misses_total[5m])) < 0.5
        for: 15m
        labels:
          severity: warning
        annotations:
          summary: "Low cache hit rate"
          description: "Cache hit rate < 50% for 15 minutes"

      # Database connection alert
      - alert: HighDBConnections
        expr: pg_connections_active > 50
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High database connections"
          description: "More than 50 active connections"

      # Drift alert backlog
      - alert: DriftAlertBacklog
        expr: ke_drift_alerts_total{status="open"} > 10
        for: 1h
        labels:
          severity: warning
        annotations:
          summary: "Drift alert backlog"
          description: "More than 10 unresolved drift alerts"

      # Embedding service latency
      - alert: EmbeddingHighLatency
        expr: histogram_quantile(0.95, rate(ke_embedding_latency_seconds_bucket[5m])) > 2
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High embedding latency"
          description: "p95 embedding latency > 2s"
```

### Grafana Dashboards

#### Dashboard: Knowledge Engine Overview

**Panels:**

1. **Request Rate** (Graph)
   - Query: `rate(ke_retrieval_requests_total[5m])`
   - Grouped by: intent

2. **Latency Distribution** (Heatmap)
   - Query: `histogram_quantile(0.5, rate(ke_retrieval_latency_seconds_bucket[5m]))`
   - Query: `histogram_quantile(0.95, rate(ke_retrieval_latency_seconds_bucket[5m]))`
   - Query: `histogram_quantile(0.99, rate(ke_retrieval_latency_seconds_bucket[5m]))`

3. **Error Rate** (Graph)
   - Query: `rate(ke_retrieval_requests_total{status="error"}[5m])`

4. **Cache Performance** (Stat)
   - Query: `sum(rate(ke_cache_hits_total[5m])) / (sum(rate(ke_cache_hits_total[5m])) + sum(rate(ke_cache_misses_total[5m])))`

5. **Ingestion Jobs** (Table)
   - Query: `ke_ingestion_jobs_total by (status)`

6. **Active Drift Alerts** (Stat)
   - Query: `ke_drift_alerts_total{status="open"}`

#### Dashboard: Ingestion Pipeline

**Panels:**

1. **Jobs in Progress** (Gauge)
2. **Job Duration** (Histogram)
3. **Documents Processed** (Counter)
4. **Embedding Queue Depth** (Gauge)
5. **Errors by Stage** (Bar chart)

#### Dashboard: Database Performance

**Panels:**

1. **Query Latency** (Heatmap)
2. **Connection Pool Usage** (Gauge)
3. **Slow Queries** (Counter)
4. **Table Sizes** (Bar chart)
5. **Index Usage** (Table)

### Structured Logging

```json
{
  "timestamp": "2025-11-28T10:30:00Z",
  "level": "info",
  "service": "knowledge-engine-api",
  "trace_id": "abc123",
  "span_id": "def456",
  "tenant_id": "00000000-0000-0000-0000-000000000001",
  "event": "retrieval_query",
  "intent": "spec_lookup",
  "latency_ms": 45,
  "result_count": 3,
  "cache_hit": true
}
```

### OpenTelemetry Configuration

```yaml
# otel-collector-config.yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

processors:
  batch:
    timeout: 1s
    send_batch_size: 1024

exporters:
  jaeger:
    endpoint: jaeger:14250
    tls:
      insecure: true
  prometheus:
    endpoint: 0.0.0.0:8889

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [batch]
      exporters: [jaeger]
    metrics:
      receivers: [otlp]
      processors: [batch]
      exporters: [prometheus]
```

### Health Checks

```bash
# API health
curl http://localhost:8085/health

# Readiness (checks DB, Redis, etc.)
curl http://localhost:8085/ready

# Liveness
curl http://localhost:8085/live
```

### Runbooks

#### High Latency Investigation

1. Check cache hit rate in Grafana
2. Review slow query log in PostgreSQL
3. Check embedding service latency
4. Review recent deployments
5. Scale horizontally if load-related

#### Ingestion Failure Investigation

1. Check job error payload in database
2. Review source document validity
3. Check embedding API quotas
4. Verify storage connectivity
5. Re-run failed job with debug logging

#### Drift Alert Triage

1. Identify affected campaigns
2. Compare document hashes
3. Schedule re-ingestion if needed
4. Resolve alert after verification

