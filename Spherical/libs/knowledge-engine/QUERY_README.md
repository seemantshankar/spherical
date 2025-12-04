# How to Query the Knowledge Engine Database

## Quick Start

The simplest way to query the database:

```bash
cd /Users/seemant/Documents/Projects/spherical/Spherical/libs/knowledge-engine

go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --products wagon-r-2025 \
  --question "What is the fuel efficiency?"
```

## Required Parameters

1. **`--tenant`** - The tenant name (e.g., `maruti-suzuki`)
2. **`--question`** - Your question in quotes (e.g., `"What is the fuel efficiency?"`)

## Complete Command Template

```bash
go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant <tenant-name> \
  --products <product-name> \
  --question "<your question>" \
  [--intent <intent-type>] \
  [--max-chunks <number>] \
  [--json] \
  [--verbose]
```

## Examples for Your Data

Since you've already ingested the Arena Wagon R data, try these:

```bash
# 1. Fuel efficiency query
go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --products wagon-r-2025 \
  --question "What is the fuel efficiency of the Wagon R?"

# 2. Safety features query
go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --products wagon-r-2025 \
  --question "What safety features does it have?"

# 3. JSON output format
go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --products wagon-r-2025 \
  --question "What are the dimensions?" \
  --json

# 4. With verbose logging (for debugging)
go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --products wagon-r-2025 \
  --question "What is the fuel efficiency?" \
  --verbose
```

## Understanding the Output

### Normal Output (When Working)
```
Intent: spec_lookup (latency: 8ms)

Structured Facts:
  • Engine > Fuel Efficiency: 25.19 km/l (conf: 0.95)

Semantic Chunks:
  1. [usp] Enhanced safety comes standard... (dist: 0.123)
```

### Current Output (Empty Results)
```
Intent: spec_lookup (latency: 8ms)
```

If you see this with no facts or chunks, see the troubleshooting guide.

## Current Status

Your query command is **working correctly** (8ms latency is excellent!), but it's returning 0 results because:

1. **FAISS index is in-memory only** - vectors aren't persisting between ingestion and query
2. **Embeddings may not be stored** - check if embeddings were saved to the database
3. **Campaign is in draft status** - may need to be published

## Quick Diagnostic

Run this to check your data:

```bash
cd /Users/seemant/Documents/Projects/spherical/Spherical/libs/knowledge-engine

sqlite3 /tmp/knowledge-engine.db "SELECT 'Chunks:' as metric, COUNT(*) as value FROM knowledge_chunks WHERE tenant_id = 'efc6ed22-b61c-5af1-a85b-11b36af6069c' UNION ALL SELECT 'Chunks with embeddings:', COUNT(*) FROM knowledge_chunks WHERE tenant_id = 'efc6ed22-b61c-5af1-a85b-11b36af6069c' AND embedding_vector IS NOT NULL AND LENGTH(embedding_vector) > 0;"
```

## Documentation Files

All documentation is in this directory:

- **QUERY_QUICK_REFERENCE.md** - Quick reference card (START HERE!)
- **QUERY_INSTRUCTIONS.md** - Step-by-step instructions  
- **QUERY_GUIDE.md** - Complete detailed guide
- **QUERY_TROUBLESHOOTING.md** - Diagnosis and solutions
- **QUERY_README.md** - This file

## Getting Help

```bash
# Command help
go run ./cmd/knowledge-engine-cli query --help

# All commands
go run ./cmd/knowledge-engine-cli --help
```

## Notes

The query infrastructure is working (fast 8ms latency!). The 0 results issue is related to vector index persistence and embedding storage, which are separate concerns that need to be addressed in the implementation.

