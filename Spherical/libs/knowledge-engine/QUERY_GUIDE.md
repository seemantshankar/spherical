# Knowledge Engine CLI - Query Guide

## Quick Start

Query the knowledge engine database to retrieve structured specifications and semantic chunks for your products.

## Prerequisites

1. **Database Setup**: Ensure you have a configured database (default: `/tmp/knowledge-engine.db` for SQLite)
2. **Config File**: Use the dev config file at `configs/dev.yaml` or set environment variables
3. **Tenant/Product Data**: Make sure you have ingested data using the `ingest` command

## Basic Query Syntax

```bash
cd libs/knowledge-engine

go run ./cmd/knowledge-engine-cli query \
  --tenant <tenant-name-or-uuid> \
  --products <product-name-or-uuid> \
  --question "<your question>"
```

## Required Flags

- `--tenant` (required): Tenant ID or name (e.g., `maruti-suzuki`)
- `--question` (required): The question you want to answer

## Optional Flags

- `--products`: Product ID(s) to query (can specify multiple)
- `--intent`: Intent hint (`spec_lookup`, `usp_lookup`, `comparison`)
- `--max-chunks`: Maximum semantic chunks to return (default: 6)
- `--config`: Path to config file (default: uses env vars)
- `--json`: Output results in JSON format
- `--verbose`: Enable verbose logging

## Examples

### Example 1: Basic Query - Fuel Efficiency

```bash
go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --products wagon-r-2025 \
  --question "What is the fuel efficiency of the Wagon R?"
```

**Output:**
```
Intent: spec_lookup (latency: 6ms)

Structured Facts:
  • Engine > Fuel Efficiency / Fuel Efficiency: 25.19 km/l (conf: 0.95)
  • Engine > Fuel Efficiency / Fuel Efficiency (CNG): 33.47 km/kg (conf: 0.95)

Semantic Chunks:
  1. [spec_row] The 1.0L Next-Gen K-Series engine offers 25.19 km/l fuel efficiency... (dist: 0.123)
  2. [usp] Drive smart and choose green with the advanced S-CNG technology... (dist: 0.245)
```

### Example 2: Safety Features Query

```bash
go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --products wagon-r-2025 \
  --question "What safety features does the Wagon R have?"
```

### Example 3: JSON Output Format

```bash
go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --products wagon-r-2025 \
  --question "What safety features does the Wagon R have?" \
  --json
```

**Output (JSON):**
```json
{
  "intent": "spec_lookup",
  "latencyMs": 2,
  "structuredFacts": [
    {
      "category": "Safety",
      "name": "Airbags",
      "value": "6",
      "unit": "",
      "confidence": 0.95
    }
  ],
  "semanticChunks": [
    {
      "chunkType": "usp",
      "text": "Protect your loved ones with 12+ safety features...",
      "distance": 0.234
    }
  ]
}
```

### Example 4: Query with Intent Hint

```bash
go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --products wagon-r-2025 \
  --question "Tell me about the unique selling points" \
  --intent usp_lookup \
  --max-chunks 10
```

### Example 5: Query Multiple Products

```bash
go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --products wagon-r-2025,camry-2025 \
  --question "Compare fuel efficiency between these vehicles"
```

## Understanding the Output

### Structured Facts
- **Category**: Specification category (e.g., "Engine", "Safety")
- **Name**: Specification name (e.g., "Fuel Efficiency", "Airbags")
- **Value**: The actual value (numeric or text)
- **Unit**: Unit of measurement (if applicable)
- **Confidence**: Extraction confidence score (0.0 - 1.0)

### Semantic Chunks
- **ChunkType**: Type of content (`spec_row`, `feature_block`, `usp`, `faq`, `comparison`, `global`)
- **Text**: Excerpt from the document
- **Distance**: Similarity distance (lower = more relevant)

### Query Intent
The system automatically classifies your query into one of these intents:
- `spec_lookup`: Looking for specific specifications
- `usp_lookup`: Looking for unique selling points
- `comparison`: Comparing products

## Advanced Usage

### Using Environment Variables Instead of Config File

```bash
export DATABASE_URL="sqlite:///tmp/knowledge-engine.db"
export OPENROUTER_API_KEY="sk-or-your-key"

go run ./cmd/knowledge-engine-cli query \
  --tenant maruti-suzuki \
  --products wagon-r-2025 \
  --question "What are the dimensions?"
```

### Verbose Mode for Debugging

```bash
go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --products wagon-r-2025 \
  --question "What is the fuel efficiency?" \
  --verbose
```

This will show detailed logs about:
- Keyword extraction
- Confidence scores
- Vector search results
- Filtering steps

## Troubleshooting

### No Results Returned (0 structured facts, 0 semantic chunks)

If queries return no results, check the following:

#### 1. Check Campaign Status
Ensure the campaign is published (not draft):
```bash
# Check current status
sqlite3 /tmp/knowledge-engine.db \
  "SELECT status FROM campaign_variants WHERE tenant_id = (SELECT id FROM tenants WHERE name = 'maruti-suzuki');"

# Publish the campaign
go run ./cmd/knowledge-engine-cli publish \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --campaign wagon-r-2025-india
```

#### 2. Verify Data Exists in Database
```bash
# Check chunks count
sqlite3 /tmp/knowledge-engine.db \
  "SELECT COUNT(*) as total_chunks FROM knowledge_chunks WHERE tenant_id = (SELECT id FROM tenants WHERE name = 'maruti-suzuki');"

# Check if embeddings were stored
sqlite3 /tmp/knowledge-engine.db \
  "SELECT COUNT(*) as chunks_with_embeddings FROM knowledge_chunks WHERE tenant_id = (SELECT id FROM tenants WHERE name = 'maruti-suzuki') AND embedding_vector IS NOT NULL AND LENGTH(embedding_vector) > 0;"
```

#### 3. Vector Store Issue (Most Common)
The FAISS adapter is **in-memory only** and doesn't persist between sessions. If you see:
- `Vector search returned no chunks`
- `results_count=0`

This means:
- **During ingestion**: Vectors were created and stored in the database, but the FAISS index is in-memory only
- **During query**: A new empty FAISS adapter is created, so it has no vectors to search

**Current Workaround**: The system should load vectors from the database into the FAISS adapter on startup, but this may not be fully implemented. Check if your ingestion successfully stored embeddings in the `knowledge_chunks.embedding_vector` column.

#### 4. Schema Issues
If you see warnings during ingestion like:
- `sql: converting argument $7 type: unsupported type []string`
- `sql: Scan error on column index 4`

These indicate schema mismatches that prevent data from being stored correctly. The chunks may have been created but specs/features weren't stored due to SQLite limitations.

#### 5. Check Database Path
Ensure you're using the correct database:
```bash
# Verify config points to correct database
grep -A 2 "sqlite:" configs/dev.yaml

# Check if database exists
ls -lh /tmp/knowledge-engine.db
```

#### 6. Verbose Debugging
Run with `--verbose` to see detailed logs:
```bash
go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --products wagon-r-2025 \
  --question "What is the fuel efficiency?" \
  --verbose
```

Look for:
- `Vector search completed query_dimension=768 results_count=0` → No vectors in index
- `Keyword search results keyword=fuel results=0` → No specs found in database

### Low Confidence Results

- Try rephrasing your question
- Use more specific terms
- Increase `--max-chunks` to see more results

### Performance Tips

- Simple keyword queries (confidence ≥0.8) return in <25ms
- Complex queries trigger vector search (may take 100-300ms)
- Use `--max-chunks` to limit results and improve speed

## Example Queries for Arena Wagon R

Based on the ingested data, here are some example queries you can try:

```bash
# Fuel efficiency
go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --products wagon-r-2025 \
  --question "What is the fuel efficiency?"

# Safety features
go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --products wagon-r-2025 \
  --question "What safety features are available?"

# Engine specifications
go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --products wagon-r-2025 \
  --question "What engine options are available?"

# Unique selling points
go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --products wagon-r-2025 \
  --question "What are the unique selling points?" \
  --intent usp_lookup

# Dimensions
go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --products wagon-r-2025 \
  --question "What are the vehicle dimensions?"
```

## Full Command Reference

For all available commands:

```bash
go run ./cmd/knowledge-engine-cli --help
```

For query-specific help:

```bash
go run ./cmd/knowledge-engine-cli query --help
```


## Understanding Query Output

When you see output like:
```
Intent: spec_lookup (latency: 8ms)
```

With no "Structured Facts" or "Semantic Chunks" sections, it means:

### Possible Causes:

1. **No Data in Database**: The ingestion may not have stored data correctly
2. **Vector Index Empty**: The FAISS adapter is in-memory and may not have loaded vectors from the database
3. **Campaign Not Published**: Some queries only return data from published campaigns
4. **Schema Issues**: SQLite type mismatches may have prevented data storage

### Expected Output Format:

When working correctly, you should see:

```
Intent: spec_lookup (latency: 6ms)

Structured Facts:
  • Engine > Fuel Efficiency / Fuel Efficiency: 25.19 km/l (conf: 0.95)

Semantic Chunks:
  1. [usp] Enhanced safety comes standard with 6 airbags... (dist: 0.123)
```

If you see empty results, check the troubleshooting section above.
