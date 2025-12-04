# How to Query the Knowledge Engine Database

## Overview

The knowledge-engine CLI provides a `query` command to search for structured specifications and semantic content from ingested product brochures.

## Command Structure

```bash
go run ./cmd/knowledge-engine-cli query [OPTIONS]
```

## Location

Navigate to the knowledge-engine directory first:

```bash
cd /Users/seemant/Documents/Projects/spherical/Spherical/libs/knowledge-engine
```

## Required Parameters

### `--tenant` (Required)
The tenant name or UUID that owns the product data.

**Example:**
```bash
--tenant maruti-suzuki
```

### `--question` (Required)
The natural language question you want to answer. Enclose in quotes if it contains spaces.

**Example:**
```bash
--question "What is the fuel efficiency?"
```

## Optional Parameters

### `--products`
One or more product names or UUIDs (comma-separated for multiple products).

**Examples:**
```bash
--products wagon-r-2025
--products wagon-r-2025,camry-2025
```

### `--config`
Path to the configuration file. Defaults to environment variables if not specified.

**Example:**
```bash
--config configs/dev.yaml
```

### `--max-chunks`
Maximum number of semantic chunks to return. Default is 6.

**Example:**
```bash
--max-chunks 10
```

### `--intent`
Hint the query intent to help routing:
- `spec_lookup`: Looking for specifications
- `usp_lookup`: Looking for unique selling points
- `comparison`: Comparing products

**Example:**
```bash
--intent usp_lookup
```

### `--json`
Output results in JSON format instead of human-readable text.

**Example:**
```bash
--json
```

### `--verbose`
Enable detailed debug logging.

**Example:**
```bash
--verbose
```

## Complete Examples

### Example 1: Basic Query

```bash
cd /Users/seemant/Documents/Projects/spherical/Spherical/libs/knowledge-engine

go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --products wagon-r-2025 \
  --question "What is the fuel efficiency of the Wagon R?"
```

### Example 2: Query with JSON Output

```bash
go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --products wagon-r-2025 \
  --question "What safety features are available?" \
  --json
```

### Example 3: Query with Intent Hint

```bash
go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --products wagon-r-2025 \
  --question "What are the unique selling points?" \
  --intent usp_lookup \
  --max-chunks 10
```

### Example 4: Query Multiple Products

```bash
go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --products wagon-r-2025,camry-2025 \
  --question "Compare the fuel efficiency"
```

### Example 5: Verbose Debug Mode

```bash
go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --products wagon-r-2025 \
  --question "What are the dimensions?" \
  --verbose
```

## Output Format

### Human-Readable Output (Default)

```
Intent: spec_lookup (latency: 6ms)

Structured Facts:
  • Engine > Fuel Efficiency / Fuel Efficiency: 25.19 km/l (conf: 0.95)
  • Safety / Airbags: 6 (conf: 0.90)

Semantic Chunks:
  1. [usp] Enhanced safety comes standard with 6 airbags... (dist: 0.123)
  2. [spec_row] The 1.0L engine offers 25.19 km/l... (dist: 0.234)
```

### JSON Output (with `--json` flag)

```json
{
  "intent": "spec_lookup",
  "latencyMs": 6,
  "structuredFacts": [
    {
      "category": "Engine > Fuel Efficiency",
      "name": "Fuel Efficiency",
      "value": "25.19",
      "unit": "km/l",
      "confidence": 0.95
    }
  ],
  "semanticChunks": [
    {
      "chunkType": "usp",
      "text": "Enhanced safety comes standard...",
      "distance": 0.123
    }
  ]
}
```

## Understanding the Results

### Structured Facts
- **Category**: Hierarchical category path (e.g., "Engine > Fuel Efficiency")
- **Name**: Specification name
- **Value**: The actual value
- **Unit**: Unit of measurement
- **Confidence**: Extraction confidence (0.0-1.0)

### Semantic Chunks
- **ChunkType**: Type of content (`spec_row`, `feature_block`, `usp`, `faq`, `comparison`, `global`)
- **Text**: Excerpt from the source document
- **Distance**: Similarity score (lower = more relevant)

## Common Query Patterns

### Finding Specifications
```bash
--question "What is the [specification name]?"
--intent spec_lookup
```

### Finding Features
```bash
--question "What features does it have?"
```

### Finding USPs
```bash
--question "What are the unique selling points?"
--intent usp_lookup
```

### Comparing Products
```bash
--question "Compare [spec] between these products"
--intent comparison
--products product1,product2
```

## Troubleshooting

### Command Not Found
Make sure you're in the correct directory:
```bash
cd /Users/seemant/Documents/Projects/spherical/Spherical/libs/knowledge-engine
```

### No Results Returned

1. **Check if campaign is published:**
   ```bash
   go run ./cmd/knowledge-engine-cli publish \
     --tenant maruti-suzuki \
     --campaign wagon-r-2025-india
   ```

2. **Verify data exists:**
   ```bash
   sqlite3 /tmp/knowledge-engine.db \
     "SELECT COUNT(*) FROM knowledge_chunks WHERE tenant_id = 'efc6ed22-b61c-5af1-a85b-11b36af6069c';"
   ```

3. **Check database path in config:**
   ```bash
   grep -A 2 "sqlite:" configs/dev.yaml
   ```

### Getting Help

View all available options:
```bash
go run ./cmd/knowledge-engine-cli query --help
```

View all CLI commands:
```bash
go run ./cmd/knowledge-engine-cli --help
```

## Quick Reference

```bash
# Minimal query
go run ./cmd/knowledge-engine-cli query \
  --tenant <name> \
  --question "<question>"

# Full query with all options
go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant <name> \
  --products <product1>,<product2> \
  --question "<question>" \
  --intent spec_lookup \
  --max-chunks 10 \
  --json \
  --verbose
```

## Performance Notes

- **Simple queries**: <25ms (keyword-only, confidence ≥0.8)
- **Complex queries**: 100-300ms (requires vector search)
- Use `--max-chunks` to limit results and improve speed

## Next Steps

- See `QUERY_GUIDE.md` for detailed documentation
- See `QUICK_START_QUERY.md` for quick examples
- Check the main README.md for ingestion instructions

