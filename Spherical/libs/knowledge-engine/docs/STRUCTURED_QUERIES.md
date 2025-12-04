# Structured Queries Documentation

## Overview

The Knowledge Engine supports structured spec requests from LLMs, enabling precise data retrieval with explicit availability status for each requested specification. This document describes how to use structured queries, interpret availability status, and integrate with LLM systems.

## Request Format

### Structured Request

A structured request includes a list of spec names that the LLM needs to answer a user's question:

```json
{
  "tenantId": "uuid",
  "productIds": ["uuid1", "uuid2"],
  "requestedSpecs": [
    "Fuel Economy",
    "Ground Clearance",
    "Engine Torque",
    "Suspension"
  ],
  "requestMode": "structured",
  "includeSummary": true
}
```

### Request Modes

- `natural_language`: Traditional question-based queries (default)
- `structured`: Process only the `requestedSpecs` list
- `hybrid`: Support both question and structured specs

### Spec Name Normalization

The system automatically normalizes spec names to handle synonyms and variations:

- **Fuel Economy**: Also accepts "Mileage", "Fuel Consumption", "Fuel Efficiency", "km/l", "kmpl", "mpg"
- **Engine Torque**: Also accepts "Torque", "Maximum Torque", "Peak Torque"
- **Ground Clearance**: Also accepts "Ground Clearance Height", "Minimum Ground Clearance", "Clearance"
- **Interior Comfort**: Also accepts "Comfort Features", "Seating Comfort"

See `spec_normalizer.go` for the complete list of supported synonyms.

## Response Format

### Availability Status

Each requested spec receives an availability status:

```json
{
  "specAvailability": [
    {
      "specName": "Fuel Economy",
      "status": "found",
      "confidence": 0.95,
      "alternativeNames": ["Mileage", "Fuel Consumption"],
      "matchedSpecs": [
        {
          "category": "Fuel Efficiency",
          "name": "Fuel Economy",
          "value": "25.49",
          "unit": "km/l",
          "confidence": 0.95
        }
      ],
      "matchedChunks": []
    },
    {
      "specName": "Ground Clearance",
      "status": "unavailable",
      "confidence": 0.0,
      "alternativeNames": [],
      "matchedSpecs": [],
      "matchedChunks": []
    }
  ],
  "overallConfidence": 0.475,
  "summary": "Found: Fuel Economy (25.49 km/l)\nUnavailable: Ground Clearance"
}
```

### Status Values

- **`found`**: Spec was found with sufficient confidence (≥60% by default)
- **`unavailable`**: Spec was not found in the knowledge base
- **`partial`**: Spec was found but with low confidence (30-60%)

### Confidence Scores

Confidence scores range from 0.0 to 1.0:

- **0.9-1.0**: Very high confidence, exact match
- **0.7-0.9**: High confidence, strong match
- **0.6-0.7**: Good confidence, acceptable match
- **0.3-0.6**: Low confidence, partial match (marked as "partial")
- **0.0-0.3**: Very low confidence or unavailable

### Overall Confidence

The `overallConfidence` field provides a weighted average of all found specs, useful for determining the quality of the entire response.

## API Endpoints

### REST API

**POST** `/api/v1/retrieval/query` or `/api/v1/retrieval/structured`

```bash
curl -X POST http://localhost:8085/api/v1/retrieval/structured \
  -H "Content-Type: application/json" \
  -d '{
    "tenantId": "your-tenant-id",
    "productIds": ["product-id"],
    "requestedSpecs": ["Fuel Economy", "Ground Clearance"],
    "requestMode": "structured",
    "includeSummary": true
  }'
```

### GraphQL

```graphql
query {
  retrieveKnowledge(input: {
    tenantId: "your-tenant-id"
    productIds: ["product-id"]
    requestedSpecs: ["Fuel Economy", "Ground Clearance"]
    requestMode: STRUCTURED
    includeSummary: true
  }) {
    specAvailability {
      specName
      status
      confidence
      alternativeNames
      matchedSpecs {
        category
        name
        value
        unit
      }
    }
    overallConfidence
    summary
  }
}
```

### gRPC

The gRPC service supports structured requests through the `RetrievalRequest` message:

```protobuf
message RetrievalRequest {
  string tenant_id = 1;
  repeated string product_ids = 2;
  repeated string requested_specs = 10;  // New field
  string request_mode = 11;              // New field
  bool include_summary = 12;             // New field
}
```

## LLM Integration Guide

### Step 1: User Query Analysis

When a user asks a question, the LLM should:

1. Identify the query type (spec lookup, comparison, USP lookup)
2. Determine which specs are needed to answer confidently
3. Generate a structured spec list

**Example:**

User: "Can I drive this car in the mountains?"

LLM Analysis:
- Query type: `spec_lookup`
- Required specs: `["Engine Specifications", "Engine Torque", "Suspension", "Ground Clearance", "Interior Comfort", "Fuel Tank Capacity", "Fuel Economy"]`

### Step 2: Structured Request

Send the structured request to the Knowledge Engine:

```json
{
  "tenantId": "...",
  "productIds": ["..."],
  "requestedSpecs": [
    "Engine Specifications",
    "Engine Torque",
    "Suspension",
    "Ground Clearance",
    "Interior Comfort",
    "Fuel Tank Capacity",
    "Fuel Economy"
  ],
  "requestMode": "structured"
}
```

### Step 3: Process Response

The Knowledge Engine returns:

```json
{
  "specAvailability": [
    {"specName": "Engine Torque", "status": "found", "confidence": 0.92, ...},
    {"specName": "Suspension", "status": "found", "confidence": 0.85, ...},
    {"specName": "Ground Clearance", "status": "unavailable", "confidence": 0.0, ...},
    {"specName": "Fuel Economy", "status": "found", "confidence": 0.88, ...}
  ],
  "overallConfidence": 0.66
}
```

### Step 4: Formulate Response

The LLM should:

1. Use found specs to answer the question
2. Explicitly mention unavailable specs: "Ground Clearance information is not available"
3. Use partial specs with caution, noting low confidence
4. Consider overall confidence when determining response certainty

**Example Response:**

"Based on the available information:
- Engine Torque: 221 Nm (good for mountain driving)
- Suspension: Independent front and rear (suitable for varied terrain)
- Fuel Economy: 25.49 km/l (efficient for long drives)
- Ground Clearance: Information not available

This vehicle appears suitable for mountain driving, though ground clearance data would help confirm."

## Best Practices

### 1. Spec Name Selection

- Use standard automotive terminology
- The system handles synonyms automatically
- Prefer full names over abbreviations when possible
- Examples: "Fuel Economy" over "MPG", "Engine Torque" over "Torque"

### 2. Batch Processing

- Request multiple specs in a single call for efficiency
- The system processes specs in parallel (up to 5 workers by default)
- Typical response time: <500ms for 10 specs (p95)

### 3. Confidence Interpretation

- **High confidence (≥0.7)**: Safe to use directly
- **Medium confidence (0.6-0.7)**: Use with caution, verify if possible
- **Low confidence (<0.6)**: Mark as "partial" or unavailable

### 4. Error Handling

- Always check `status` field, not just presence of data
- Handle "unavailable" gracefully in user-facing responses
- Use `alternativeNames` to inform users about synonyms found

### 5. Caching

- Structured requests are cached using normalized spec names
- Cache keys include sorted, normalized spec names for consistency
- Cache TTL: 5 minutes by default

## Configuration

### Router Configuration

```yaml
retrieval:
  min_availability_confidence: 0.6  # Threshold for "found" vs "partial"
  batch_processing_workers: 5       # Parallel workers for batch processing
  batch_processing_timeout: 30s     # Timeout for batch operations
  enable_summary_generation: false  # Enable NL summary generation
```

### Environment Variables

```bash
# Override defaults via environment
export RETRIEVAL_MIN_AVAILABILITY_CONFIDENCE=0.7
export RETRIEVAL_BATCH_PROCESSING_WORKERS=10
export RETRIEVAL_BATCH_PROCESSING_TIMEOUT=60s
```

## Troubleshooting

### No Results for Known Specs

1. Check spec name spelling and format
2. Verify synonyms are supported (see `spec_normalizer.go`)
3. Check product IDs and tenant ID are correct
4. Review confidence thresholds in configuration

### Low Confidence Scores

1. Verify data quality in knowledge base
2. Check if spec names match ingestion format
3. Review category aliases in `parser.go`
4. Consider adjusting `min_availability_confidence` threshold

### Performance Issues

1. Reduce `batch_processing_workers` if system is overloaded
2. Increase `batch_processing_timeout` for large batches
3. Check embedding service latency
4. Review cache hit rates

## Future Enhancements

1. **LLM-based Summary Generation**: Automatic natural language summaries
2. **Spec Name Learning**: ML-based normalization from user queries
3. **Cross-Make/Model Mapping**: Generic spec mapping (e.g., "Toyota Camry Ground Clearance" → "Ground Clearance")
4. **Spec Relationship Graph**: Understand related specs (e.g., "Fuel Economy" related to "Engine Efficiency")

## Support

For issues or questions:
- Review code: `internal/retrieval/`
- Check tests: `tests/integration/`
- See examples: `tests/contract/knowledge-engine/retrieval.http`

