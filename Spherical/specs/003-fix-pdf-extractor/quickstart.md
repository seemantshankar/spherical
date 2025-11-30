# Quickstart: Fix PDF Extractor Output Quality

**Feature**: 003-fix-pdf-extractor  
**Date**: 2025-11-30

## Overview

This feature fixes four critical issues in the PDF specification extractor:
1. Removes markdown codeblock delimiters from output
2. Implements standard hierarchical nomenclature for categories
3. Extracts variant and trim information
4. Captures variant differentiation (checkboxes/symbols)

## Prerequisites

- Go 1.25.0 or later
- Existing `libs/pdf-extractor` library
- OpenRouter API key configured
- Test PDF files for validation

## Implementation Steps

### 1. Update LLM Prompt

**File**: `libs/pdf-extractor/internal/llm/client.go`

**Function**: `buildPrompt()`

**Changes**:
- Add explicit instruction: "NEVER output markdown codeblock delimiters (```) anywhere in your response"
- Add standard hierarchical nomenclature guide with variable depth (2-4 levels) and examples
- Add variant extraction instructions (from table headers and text mentions)
- Add checkbox/symbol parsing instructions (✓, ✗, ●, ○, etc.)
- Specify 5-column table format: Category | Specification | Value | Key Features | Variant Availability
- Add instructions for handling ambiguous variant boundaries (mark as "Unknown")

**Example Addition**:
```go
CRITICAL OUTPUT FORMAT RULES:
- NEVER output markdown codeblock delimiters (```) anywhere in your response
- Use standard hierarchical category notation with variable depth (2-4 levels):
  * "Interior > Seats > Upholstery > Material" (4 levels for complex features)
  * "Engine > Torque" (2 levels for simple features)
- Use 5-column table format: Category | Specification | Value | Key Features | Variant Availability
- Extract variant names from table headers and text mentions
- Parse checkbox/symbol indicators (✓, ✗, ●, ○) for variant availability
- For features available in all variants, use "Standard" in Variant Availability column
- For ambiguous variant boundaries, use "Unknown" in Variant Availability column
```

### 2. Add Post-Processing Function

**File**: `libs/pdf-extractor/internal/extract/service.go`

**Function**: `cleanCodeblocks(markdown string) string`

**Implementation**:
```go
var codeblockPattern = regexp.MustCompile(`(?s)```(?:markdown)?\s*\n?(.*?)\n?```\s*`)

func cleanCodeblocks(markdown string) string {
    // Remove all markdown codeblock delimiters, preserve content
    cleaned := codeblockPattern.ReplaceAllString(markdown, "$1")
    return cleaned
}
```

**Integration**: Call in `extractPage()` after collecting markdown, before deduplication:
```go
pageMarkdown, err := s.extractPage(ctx, image, eventCh)
if err != nil {
    // handle error
}

// Remove codeblocks
cleanMarkdown := cleanCodeblocks(pageMarkdown)

// Deduplicate
finalMarkdown := deduplicator.DeduplicateMarkdown(cleanMarkdown)
```

### 3. Write Tests

**File**: `libs/pdf-extractor/tests/integration/pdf_test.go`

**Test Cases**:
1. Test codeblock removal from LLM output
2. Test standard nomenclature mapping
3. Test variant extraction from tables
4. Test checkbox/symbol parsing
5. Test edge cases (empty codeblocks, merged cells, etc.)

**Example Test**:
```go
func TestCodeblockRemoval(t *testing.T) {
    input := "```markdown\n## Specifications\n| Category | Spec | Value |\n```"
    expected := "## Specifications\n| Category | Spec | Value |"
    result := cleanCodeblocks(input)
    assert.Equal(t, expected, result)
}
```

### 4. Update Documentation

**Files to Update**:
- `libs/pdf-extractor/README.md`: Document new output format with variant information
- Inline comments: Document new prompt structure and post-processing

## Testing

### Unit Tests
```bash
cd libs/pdf-extractor
go test ./internal/extract -v -run TestCodeblockRemoval
```

### Integration Tests
```bash
cd libs/pdf-extractor
go test ./tests/integration -v -run TestVariantExtraction
```

### Manual Testing
```bash
# Extract a PDF with variants (e.g., Skoda Kodiaq)
./cmd/pdf-extractor/pdf-extractor --input testdata/kodiaq.pdf --output kodiaq-output.md

# Verify:
# 1. No codeblock delimiters in output
# 2. Standard nomenclature used (e.g., "Interior > Seats > Upholstery")
# 3. Variant names extracted (e.g., "Lounge", "Sportline", "Selection L&K")
# 4. Variant availability shown in 5th column (e.g., "Lounge: ✓, Sportline: ✓, Selection L&K: ✗" or "Standard")
```

## Verification Checklist

- [ ] No codeblock delimiters (```) in output markdown
- [ ] Categories use hierarchical notation with appropriate depth (2-4 levels, e.g., "Interior > Seats > Upholstery")
- [ ] Output uses 5-column table format: Category | Specification | Value | Key Features | Variant Availability
- [ ] Variant names extracted from table headers
- [ ] Variant-exclusive features tagged (e.g., "Exclusive to: Selection L&K" in Variant Availability column)
- [ ] Features available in all variants marked as "Standard" in Variant Availability column
- [ ] Checkbox/symbol indicators parsed and included in Variant Availability column
- [ ] All existing tests pass
- [ ] New tests pass
- [ ] Integration tests with real PDFs pass

## Rollback Plan

If issues are discovered:
1. Revert prompt changes in `client.go`
2. Remove post-processing function from `service.go`
3. Revert test changes
4. All changes are in feature branch, can be abandoned if needed

## Next Steps

After implementation:
1. Run full test suite
2. Test with multiple PDFs (including Skoda Kodiaq)
3. Verify ingestion engine compatibility
4. Update knowledge engine parser if needed to handle variant information
5. Merge to main after approval

