# Data Model: Fix PDF Extractor Output Quality

**Feature**: 003-fix-pdf-extractor  
**Date**: 2025-11-30

## Overview

This feature modifies the output format of extracted specifications to include variant information and ensure clean markdown. The data model changes are primarily in the output format (markdown structure), not in internal data structures.

## Entities

### Extracted Specification (Enhanced)

**Purpose**: Represents a single specification row in the output markdown table.

**Fields**:

- `Category` (string): Hierarchical category path with variable depth (2-4 levels) based on semantic meaning (e.g., "Interior > Seats > Upholstery" or "Engine > Torque")
- `Specification` (string): Specification name (e.g., "Upholstery Material")
- `Value` (string): Specification value (e.g., "Leather", "169 kW")
- `Key Features` (string): Descriptive text, marketing highlights, or benefits
- `Variant Availability` (string): Information about which variants include this feature (e.g., "Lounge: ✓, Sportline: ✓, Selection L&K: ✗", "Standard", "Exclusive to: Selection L&K", or "Unknown")

**Format in Markdown Table** (5-column format):

```markdown
| Category | Specification | Value | Key Features | Variant Availability |
|----------|---------------|-------|--------------|----------------------|
| Interior > Seats | Upholstery | Leather | Premium leather upholstery | Lounge: ✓, Sportline: ✓, Selection L&K: ✓ |
| Interior > Seats | Upholstery | Fabric | Standard fabric upholstery | Standard |
| Engine | Power Output | 169 kW | High power output | Lounge: ✓, Sportline: ✓, Selection L&K: ✓ |
| Safety | Panoramic Sunroof | Yes | Available in all variants | Standard |
| Interior | Massage Seats | Yes | Premium comfort feature | Exclusive to: Selection L&K |
| Interior | Ambiguous Feature | TBD | Feature with unclear variant boundaries | Unknown |
```

**Validation Rules**:

- Category MUST use hierarchical notation with variable depth (2-4 levels): `Category > Subcategory > Detail` or `Category > Detail`
- Category MUST use standard nomenclature (see research.md for standard categories)
- Category depth MUST be consistent for similar features across brochures
- Value MUST NOT include variant availability (variant info goes in separate column)
- Variant Availability MUST use one of:
  - "Standard" for features available in all variants
  - "Exclusive to: [VariantName]" for features exclusive to one variant
  - "Lounge: ✓, Sportline: ✓, Selection L&K: ✗" format for multi-variant differentiation
  - "Unknown" for ambiguous cases where variant boundaries cannot be clearly identified

### Variant/Trim

**Purpose**: Represents a specific model configuration that may have unique features.

**Fields**:

- `Name` (string): Variant name as it appears in brochure (e.g., "Lounge", "Sportline", "Selection L&K")
- `Source` (string): Where variant was identified ("table_header", "text_mention", "feature_exclusive")

**Extraction Rules**:

- Extract from table column headers when present
- Extract from text mentions: "Exclusive to [Variant]", "Available only in [Variant]"
- Maintain variant names as-is (no normalization in V1)

**Example**:

- Variant: "Selection L&K" (from table header)
- Variant: "Lounge" (from text: "Exclusive to Lounge")

### Variant Availability

**Purpose**: Information about which variants include a specific feature.

**Representation**:

- **Format 1**: Symbol-based: `"Lounge: ✓, Sportline: ✓, Selection L&K: ✗"`
- **Format 2**: List-based: `"Available in: Lounge, Sportline, Selection L&K"`
- **Format 3**: Standard: `"Standard"` (when available in all variants)
- **Format 4**: Exclusive: `"Exclusive to: Selection L&K"` (when only one variant)
- **Format 5**: Ambiguous: `"Unknown"` (when variant boundaries cannot be clearly identified)

**Location in Output**:

- Dedicated 5th column "Variant Availability" in specifications table
- Always present (may be "Standard" or empty for single-trim models)

## Data Flow

### Current Flow (Before Changes)

```
PDF → Images → LLM → Markdown → Deduplication → Output
```

### New Flow (After Changes)

```
PDF → Images → LLM (with updated prompt) → Markdown → Codeblock Removal → Deduplication → Output
```

### Processing Steps

1. **LLM Extraction** (`internal/llm/client.go`):
   - Updated prompt includes:
     - Standard hierarchical nomenclature guide with variable depth (2-4 levels)
     - Variant extraction instructions (from table headers and text mentions)
     - Checkbox/symbol parsing instructions (✓, ✗, ●, ○, etc.)
     - 5-column table format: Category | Specification | Value | Key Features | Variant Availability
     - Explicit "no codeblocks" instruction
     - Instructions for handling ambiguous variant boundaries (mark as "Unknown")

2. **Codeblock Removal** (`internal/extract/service.go`):
   - Post-process markdown to remove any codeblock delimiters
   - Function: `cleanCodeblocks(markdown string) string`
   - Uses regex: `(?s)```(?:markdown)?\s*\n?(.*?)\n?```\s*`
   - Called after LLM extraction, before deduplication

3. **Deduplication** (existing):
   - Removes redundant specifications
   - No changes required

4. **Output**:
   - Clean markdown with standard nomenclature
   - Variant information included in dedicated 5th column where applicable
   - No codeblock delimiters

## State Transitions

N/A - This feature modifies output format, not internal state management.

## Relationships

- **Specification → Variant**: Many-to-many relationship
  - One specification can be available in multiple variants
  - One variant can have multiple specifications
  - Represented in markdown via Variant Availability column

## Validation Rules

### Category Nomenclature

- MUST use hierarchical format: `Category > Subcategory > Detail`
- MUST use standard categories (see research.md)
- MUST map brochure-specific terms to standard categories semantically
- MUST use variable depth (2-4 levels) based on semantic meaning
- Similar features MUST use consistent depth across brochures

### Variant Information

- Variant names extracted from table headers MUST be included in output
- Variant-exclusive features MUST be tagged with variant name in Variant Availability column
- Features available in all variants MUST be marked as "Standard" in Variant Availability column
- Checkbox/symbol indicators MUST be parsed and included in Variant Availability column
- Ambiguous cases MUST be marked as "Unknown" in Variant Availability column

### Codeblock Removal

- Output MUST contain zero markdown codeblock delimiters (```)
- Empty codeblocks MUST be removed (output empty string)
- Content inside codeblocks MUST be preserved (delimiters removed)

## Edge Cases

1. **No Variant Information**: Single trim models - Variant Availability column may be empty or "Standard"
2. **Non-Standard Symbols**: LLM should describe what it sees if symbols are unrecognized
3. **Merged Table Cells**: Create separate rows for each variant
4. **Multi-Page Tables**: Maintain variant context across pages
5. **Inconsistent Variant Names**: Use names as they appear (no normalization in V1)
6. **Empty Codeblocks**: Remove entirely, output empty string
7. **Nested Codeblocks**: Remove outer, preserve inner content (shouldn't occur)
8. **Ambiguous Variant Boundaries**: Mark as "Unknown" in Variant Availability column, continue processing

## Migration Notes

- **Format Change**: Output structure changes from 4-column to 5-column table (adds Variant Availability column)
- **Backward Compatibility**: Ingestion engine must handle both old format (4 columns) and new format (5 columns)
- **New Format**: New extractions will include variant information when available
- **No Breaking Changes**: Existing markdown files remain valid, new format is additive
