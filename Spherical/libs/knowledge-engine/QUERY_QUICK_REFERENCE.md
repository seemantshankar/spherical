# Query CLI - Quick Reference Card

## Basic Command Structure

```bash
cd /Users/seemant/Documents/Projects/spherical/Spherical/libs/knowledge-engine

go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant <tenant-name> \
  --products <product-name> \
  --question "<your question>"
```

## Minimal Required Command

```bash
go run ./cmd/knowledge-engine-cli query \
  --tenant maruti-suzuki \
  --question "What is the fuel efficiency?"
```

## Complete Example (With All Options)

```bash
go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --products wagon-r-2025 \
  --question "What is the fuel efficiency?" \
  --intent spec_lookup \
  --max-chunks 6 \
  --json \
  --verbose
```

## Parameters

| Parameter | Required | Description | Example |
|-----------|----------|-------------|---------|
| `--tenant` | ✅ Yes | Tenant name or UUID | `maruti-suzuki` |
| `--question` | ✅ Yes | Your question in quotes | `"What is the fuel efficiency?"` |
| `--config` | No | Config file path | `configs/dev.yaml` |
| `--products` | No | Product name(s), comma-separated | `wagon-r-2025` or `wagon-r-2025,camry-2025` |
| `--intent` | No | Query intent hint | `spec_lookup`, `usp_lookup`, `comparison` |
| `--max-chunks` | No | Max results to return (default: 6) | `10` |
| `--json` | No | Output in JSON format | (flag only) |
| `--verbose` | No | Show debug logs | (flag only) |

## Quick Examples

### Example 1: Simple Query
```bash
go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --products wagon-r-2025 \
  --question "What is the fuel efficiency?"
```

### Example 2: JSON Output
```bash
go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --products wagon-r-2025 \
  --question "What safety features are available?" \
  --json
```

### Example 3: With Intent Hint
```bash
go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --products wagon-r-2025 \
  --question "What are the unique selling points?" \
  --intent usp_lookup
```

### Example 4: More Results
```bash
go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --products wagon-r-2025 \
  --question "Tell me about all features" \
  --max-chunks 20
```

### Example 5: Debug Mode
```bash
go run ./cmd/knowledge-engine-cli query \
  --config configs/dev.yaml \
  --tenant maruti-suzuki \
  --products wagon-r-2025 \
  --question "What is the fuel efficiency?" \
  --verbose
```

## Output Format

### Human-Readable (Default)
```
Intent: spec_lookup (latency: 8ms)

Structured Facts:
  • Engine > Fuel Efficiency: 25.19 km/l (conf: 0.95)

Semantic Chunks:
  1. [usp] Enhanced safety... (dist: 0.123)
```

### JSON Format (with `--json`)
```json
{
  "intent": "spec_lookup",
  "latencyMs": 8,
  "structuredFacts": [...],
  "semanticChunks": [...]
}
```

## Troubleshooting

### Query Returns 0 Results?

1. **Check if data exists**:
   ```bash
   sqlite3 /tmp/knowledge-engine.db \
     "SELECT COUNT(*) FROM knowledge_chunks WHERE tenant_id = (SELECT id FROM tenants WHERE name = 'maruti-suzuki');"
   ```

2. **Check campaign status**:
   ```bash
   sqlite3 /tmp/knowledge-engine.db \
     "SELECT status FROM campaign_variants WHERE tenant_id = (SELECT id FROM tenants WHERE name = 'maruti-suzuki');"
   ```

3. **Publish campaign if needed**:
   ```bash
   go run ./cmd/knowledge-engine-cli publish \
     --config configs/dev.yaml \
     --tenant maruti-suzuki \
     --campaign wagon-r-2025-india
   ```

4. **Run with verbose flag** to see detailed logs:
   ```bash
   go run ./cmd/knowledge-engine-cli query ... --verbose
   ```

See `QUERY_TROUBLESHOOTING.md` for detailed diagnosis.

## Getting Help

```bash
# Query command help
go run ./cmd/knowledge-engine-cli query --help

# All commands help
go run ./cmd/knowledge-engine-cli --help
```

## Common Questions

**Q: Why do I need `--config`?**  
A: It points to your database configuration. Default is `configs/dev.yaml`.

**Q: Can I query multiple products?**  
A: Yes, use comma-separated: `--products product1,product2`

**Q: What are the intent options?**  
A: `spec_lookup` (specifications), `usp_lookup` (selling points), `comparison` (compare products)

**Q: Why is latency so fast (8ms)?**  
A: Keyword queries are very fast. Vector search takes longer (100-300ms) but only runs when needed.

## Related Documentation

- `QUERY_GUIDE.md` - Complete detailed guide
- `QUERY_TROUBLESHOOTING.md` - Diagnosis and solutions
- `QUERY_INSTRUCTIONS.md` - Step-by-step instructions

