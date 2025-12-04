# Quick Start: Querying the Knowledge Engine

## Basic Command

```bash
cd libs/knowledge-engine

go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant <tenant-name> \
  --products <product-name> \
  --question "<your question>"
```

## For Arena Wagon R (Already Ingested)

```bash
cd libs/knowledge-engine

# Example 1: Fuel efficiency
go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --products wagon-r-2025 \
  --question "What is the fuel efficiency of the Wagon R?"

# Example 2: Safety features  
go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --products wagon-r-2025 \
  --question "What safety features does the Wagon R have?"

# Example 3: JSON output
go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --products wagon-r-2025 \
  --question "What are the dimensions?" \
  --json
```

## Required Parameters

- `--tenant`: Tenant name or UUID (e.g., `maruti-suzuki`)
- `--question`: Your question in quotes

## Optional Parameters

- `--products`: Product name(s), comma-separated for multiple
- `--max-chunks`: Number of results (default: 6)
- `--intent`: `spec_lookup`, `usp_lookup`, or `comparison`
- `--json`: Get JSON output instead of human-readable
- `--verbose`: Show detailed debug logs

## Output Format

The query returns:
1. **Structured Facts**: Specific specifications with values
2. **Semantic Chunks**: Relevant text excerpts from documents

Both include confidence scores and source information.

See `QUERY_GUIDE.md` for detailed documentation.
