# Ingestion Performance Benchmark

## T024: Ingestion Benchmark Harness

### Objective
Validate that a 20-page brochure can be processed and published within 15 minutes on reference hardware.

### Reference Hardware
- **CPU**: 4 cores @ 2.5 GHz
- **Memory**: 8 GB RAM
- **Storage**: SSD
- **Database**: PostgreSQL 18 with PGVector
- **Embedding Service**: OpenRouter (google/gemini-embedding-001)

### Test Scenarios

#### Scenario 1: Single 20-Page Brochure
```bash
# Run benchmark
cd libs/knowledge-engine
task benchmark:ingest -- --pages 20 --runs 3
```

Expected results:
- Parse time: < 30 seconds
- Embedding generation: < 5 minutes
- Database writes: < 2 minutes
- Total time: < 10 minutes (with buffer to 15 min SLA)

#### Scenario 2: Bulk Import (100 campaigns)
```bash
task benchmark:bulk-ingest -- --campaigns 100 --pages-per-campaign 5
```

Expected results:
- Total time: < 60 minutes
- Throughput: > 1 campaign/minute

### Benchmark Driver

```go
// benchmark_driver.go - Run with: go run tests/perf/benchmark_driver.go
package main

import (
    "context"
    "fmt"
    "os"
    "time"
    
    "github.com/spherical-ai/spherical/libs/knowledge-engine/internal/ingest"
)

func main() {
    ctx := context.Background()
    
    // Load sample brochure
    pages := 20
    if len(os.Args) > 1 {
        fmt.Sscanf(os.Args[1], "%d", &pages)
    }
    
    markdown := generateSampleMarkdown(pages)
    
    // Benchmark parse phase
    start := time.Now()
    parser := ingest.NewParser(nil)
    result, err := parser.Parse(markdown)
    parseTime := time.Since(start)
    
    if err != nil {
        fmt.Printf("Parse failed: %v\n", err)
        os.Exit(1)
    }
    
    fmt.Printf("Parse time: %v\n", parseTime)
    fmt.Printf("Specs extracted: %d\n", len(result.Specifications))
    fmt.Printf("Features extracted: %d\n", len(result.Features))
    fmt.Printf("USPs extracted: %d\n", len(result.USPs))
    
    // Note: Full benchmark requires database and embedding service
    // See benchmark_full_test.go for complete benchmark
}

func generateSampleMarkdown(pages int) string {
    // Generate realistic brochure content
    content := "---\ntitle: Benchmark Vehicle\nmodel_year: 2025\n---\n\n"
    
    for i := 0; i < pages; i++ {
        content += fmt.Sprintf("\n## Page %d\n\n", i+1)
        content += "| Category | Spec | Value | Unit |\n|----------|------|-------|------|\n"
        for j := 0; j < 10; j++ {
            content += fmt.Sprintf("| Engine | Metric_%d_%d | %d | units |\n", i, j, i*10+j)
        }
        content += "\n### Features\n\n"
        for j := 0; j < 5; j++ {
            content += fmt.Sprintf("- **Feature %d.%d**: Description of feature %d on page %d\n", i, j, j, i)
        }
    }
    
    return content
}
```

### Running Benchmarks

```bash
# Setup
cd /path/to/spherical
export DATABASE_URL="postgres://user:pass@localhost:5432/knowledge_engine"
export REDIS_URL="redis://localhost:6379"
export EMBEDDING_API_KEY="your-openrouter-key"

# Run ingestion benchmark
go test -bench=BenchmarkIngestion -benchtime=3x ./libs/knowledge-engine/tests/integration/

# Run retrieval benchmark (after data seeded)
go test -bench=BenchmarkRetrieval -benchtime=10x ./libs/knowledge-engine/tests/integration/
```

### Metrics Collected

| Metric | Target | Actual |
|--------|--------|--------|
| Parse time (20 pages) | < 30s | TBD |
| Embedding time (200 chunks) | < 5m | TBD |
| DB write time | < 2m | TBD |
| Total publish time | < 15m | TBD |
| Memory peak | < 2GB | TBD |

### Benchmark Results

_Results to be populated after running benchmarks_

| Date | Hardware | Pages | Parse (s) | Embed (s) | Write (s) | Total (s) | Pass |
|------|----------|-------|-----------|-----------|-----------|-----------|------|
| - | - | - | - | - | - | - | - |

### SLA Validation

- [x] 20-page brochure parses in < 30s
- [ ] Embeddings generated in < 5m
- [ ] Database writes complete in < 2m
- [ ] Total time < 15m on reference hardware

