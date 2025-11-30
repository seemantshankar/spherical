# Research: Fix PDF Extractor Output Quality

**Feature**: 003-fix-pdf-extractor  
**Date**: 2025-11-30  
**Status**: Complete

## Research Tasks

### 1. Codeblock Removal Strategy

**Task**: Determine the most effective approach to remove markdown codeblock delimiters from LLM output

**Findings**:

- **Decision**: Multi-pass approach: first attempt LLM prompt fix (FR-001), then regex-based fallback removal (FR-002)
- **Rationale**:
  - Prompt-level prevention is primary defense but LLMs can be inconsistent
  - Post-processing provides safety net for edge cases
  - Defensive programming ensures 100% success rate (SC-001 requirement)
- **Implementation**:
  - Add explicit instruction in prompt: "NEVER output markdown codeblock delimiters (```) anywhere in your response"
  - Post-process using regex: `(?s)```(?:markdown)?\s*\n?(.*?)\n?```\s*` to remove any codeblocks
  - Handle edge cases: empty codeblocks, nested codeblocks, codeblocks in table cells
- **Alternatives Considered**:
  - Prompt-only: Rejected - LLMs can be inconsistent, need safety net
  - Post-processing only: Rejected - Better to prevent at source, reduces processing overhead
  - LLM model switching: Rejected - Current model works, just needs better instructions

### 2. Standard Hierarchical Nomenclature

**Task**: Define standard hierarchical category structure for automobile specifications

**Findings**:

- **Decision**: Use hierarchical path notation with variable depth (2-4 levels) based on semantic meaning
- **Rationale**:
  - Matches user expectation (spec mentions "Interior > seats > upholstery")
  - Enables consistent querying across different brochures
  - Semantic understanding allows mapping brochure-specific terms to standards
  - Variable depth allows appropriate granularity: deeper for complex features (3-4 levels), shallower for simple features (2 levels)
- **Standard Categories** (with examples):

  ```text
  Engine > [Type, Power, Torque, Displacement, Fuel Efficiency]
  Exterior > [Design, Dimensions, Lighting, Wheels, Colors]
  Interior > Seats > [Upholstery > Material, Adjustment, Heating]
  Interior > [Display, Climate Control, Audio]
  Safety > [Airbags, Driver Assistance, Braking, Stability]
  Performance > [Drive Modes, Transmission, Suspension]
  Dimensions > [Length, Width, Height, Wheelbase, Weight]
  ```

- **Depth Guidelines**:
  - 2 levels: Simple features (e.g., "Engine > Torque")
  - 3-4 levels: Complex features requiring granularity (e.g., "Interior > Seats > Upholstery > Material")
  - Similar features MUST use consistent depth across brochures
- **Implementation**:
  - Add comprehensive nomenclature guide to prompt with examples
  - Instruct LLM to use semantic understanding to map brochure terms to standard categories
  - Provide mapping examples: "Cabin Experience" → "Interior > Comfort", "DRL" → "Exterior > Lighting > DRL"
  - Allow LLM to infer additional categories when needed
- **Alternatives Considered**:
  - Flat categories: Rejected - Loses hierarchy, harder to query
  - Brochure-specific terms: Rejected - Inconsistent across models, breaks user requirement
  - Fixed depth: Rejected - Some features need more granularity than others
  - Taxonomy file: Considered but rejected for V1 - can be added later if needed

### 3. Variant Extraction from Tables

**Task**: Determine how to extract variant names and associate them with features from specification tables

**Findings**:

- **Decision**: Extract variant names from table headers and text mentions, associate with features via dedicated Variant Availability column
- **Rationale**:
  - Variant names appear in table column headers (e.g., "Lounge", "Sportline", "Selection L&K")
  - Text mentions like "Exclusive to L&K" provide additional context
  - Checkbox/symbol patterns indicate feature availability per variant
  - Dedicated column keeps variant data structured and queryable
- **Output Format**:
  - **Decision**: Add dedicated "Variant Availability" column as 5th column (Category | Specification | Value | Key Features | Variant Availability)
  - **Rationale**:
    - Keeps variant data structured and queryable
    - Aligns with existing table format structure
    - Makes it easy to filter/query by variant
    - Clear separation of concerns
  - **Format Examples**:
    - Multiple variants: "Lounge: ✓, Sportline: ✓, Selection L&K: ✗"
    - All variants: "Standard" (single word, per clarification)
    - Exclusive: "Exclusive to: Selection L&K"
    - Ambiguous: "Unknown" (when variant boundaries cannot be clearly identified)
- **Implementation**:
  - Prompt instruction: "Identify all variant/trim names from table headers and feature descriptions"
  - Extract variant names from column headers when table structure is detected
  - Parse text mentions: "Exclusive to [Variant]", "Available only in [Variant]"
  - For each specification row, include variant availability in dedicated 5th column
- **Alternatives Considered**:
  - Include in "Value" column: Rejected - Mixes value with availability metadata
  - Include in "Key Features" column: Rejected - Mixes concerns, harder to query
  - Separate rows per variant: Rejected - Too verbose, breaks existing structure

### 4. Checkbox/Symbol Parsing for Variant Differentiation

**Task**: Determine how to capture checkbox/symbol indicators showing feature availability per variant

**Findings**:

- **Decision**: Parse common symbols (✓, ✗, ●, ○, ☑, ☐, etc.) and map to availability status
- **Rationale**:
  - Brochures use various symbols: checkmarks, crosses, filled/empty circles, checkboxes
  - Users need to know which variants have which features (explicitly called "extremely important")
  - This is "one of the most asked questions by users"
- **Common Symbols Identified**:
  - ✓, ✔, ☑, ☒ (checkmarks) → Available
  - ✗, ✘, ☐ (crosses/empty) → Not Available
  - ●, ○ (filled/empty circles) → Available/Not Available
  - Standard/Base/All variants → Available in all variants
- **Implementation**:
  - Prompt instruction: "When parsing specification tables, pay close attention to checkboxes, checkmarks (✓), crosses (✗), filled circles (●), and empty circles (○) that indicate feature availability per variant"
  - Format output in Variant Availability column: "Lounge: ✓, Sportline: ✓, Selection L&K: ✗" or "Available in: Lounge, Sportline"
  - For features available in all variants: "Standard" (single word, per clarification)
  - For exclusive features: "Exclusive to: Selection L&K"
  - For ambiguous cases: "Unknown" (when variant boundaries cannot be clearly identified)
- **Edge Cases**:
  - Non-standard symbols: LLM should describe what it sees (e.g., "Available in Lounge and Sportline (indicated by filled square)")
  - Merged cells: Create separate rows for each variant
  - Multi-page tables: Maintain variant context across pages
- **Alternatives Considered**:
  - Ignore symbols, only use text: Rejected - Loses critical information
  - Binary availability only: Rejected - Need to show which variants have feature
  - Complex availability matrix: Rejected - Too complex for markdown output format

### 5. Post-Processing Implementation

**Task**: Determine where and how to implement post-processing for codeblock removal

**Findings**:

- **Decision**: Add post-processing function in `extract/service.go` after LLM extraction, before deduplication
- **Rationale**:
  - Service layer is appropriate for output sanitization
  - Must run before deduplication to ensure clean input
  - Can reuse existing deduplication infrastructure pattern
- **Implementation Location**:
  - Function: `cleanCodeblocks(markdown string) string` in `extract/service.go`
  - Called in `extractPage()` after collecting markdown from LLM, before deduplication
  - Use regex with multiline support to handle codeblocks spanning multiple lines
- **Regex Pattern**:

  ```go
  var codeblockPattern = regexp.MustCompile(`(?s)```(?:markdown)?\s*\n?(.*?)\n?```\s*`)
  ```

  - Captures content inside codeblocks
  - Handles optional "markdown" language tag
  - Preserves content, removes delimiters
- **Edge Cases**:
  - Empty codeblocks: ````markdown\n\n```` → empty string
  - Nested codeblocks: Remove outer, preserve inner (shouldn't occur but handle gracefully)
  - Codeblocks in table cells: Remove delimiters, preserve content
- **Alternatives Considered**:
  - LLM client layer: Rejected - Mixes concerns, client should only handle API communication
  - Deduplication layer: Rejected - Deduplication is for content, not format
  - Separate sanitization service: Rejected - Over-engineering for simple regex operation

### 6. Ambiguous Variant Boundary Handling

**Task**: Determine how to handle cases where variant boundaries cannot be clearly identified

**Findings**:

- **Decision**: Extract what can be identified with confidence, mark ambiguous cases as "Unknown", and continue processing
- **Rationale**:
  - Preserves extractable data rather than losing entire table
  - Avoids false positives by marking uncertain cases
  - Allows downstream validation while maintaining pipeline throughput
  - Better than halting or skipping entire tables
- **Implementation**:
  - Prompt instruction: "When variant boundaries are ambiguous, extract what you can identify with confidence and mark ambiguous cases as 'Unknown' in the Variant Availability column"
  - Continue processing rather than halting or skipping
- **Alternatives Considered**:
  - Skip entire table: Rejected - Loses all data, too aggressive
  - Use heuristics/fallback: Rejected - Risk of false positives
  - Return error and halt: Rejected - Breaks pipeline, loses other extractable data

### 7. Testing Strategy

**Task**: Determine test approach for new functionality

**Findings**:

- **Decision**: Integration tests with real PDFs + unit tests for post-processing
- **Rationale**:
  - Integration tests verify end-to-end behavior with real LLM responses
  - Unit tests for post-processing ensure regex correctness
  - Follows Constitution Principle IV (Integration Testing Without Mocks)
- **Test Cases**:
  1. PDF with codeblock-wrapped output → verify clean markdown
  2. PDF with variant tables → verify variant extraction
  3. PDF with checkbox indicators → verify availability parsing
  4. PDF with non-standard nomenclature → verify standard mapping
  5. Edge cases: empty codeblocks, merged cells, multi-page tables, ambiguous boundaries
- **Test Data**:
  - Use existing test PDFs
  - Add Skoda Kodiaq PDF (mentioned in spec) for variant testing
  - Create synthetic markdown samples for unit tests
- **Alternatives Considered**:
  - Mock LLM responses: Rejected - Violates Constitution Principle IV
  - Only integration tests: Rejected - Need fast unit tests for regex logic
  - Only unit tests: Rejected - Need to verify LLM prompt effectiveness

## Summary

All research tasks completed. Key decisions:

1. **Codeblock removal**: Multi-pass approach (prompt + regex fallback)
2. **Nomenclature**: Hierarchical path notation with variable depth (2-4 levels) based on semantic meaning
3. **Variant extraction**: From table headers and text mentions, in dedicated 5th column
4. **Checkbox parsing**: Parse common symbols, format as availability list in Variant Availability column
5. **Post-processing**: Service layer, before deduplication
6. **Ambiguous handling**: Extract with confidence, mark as "Unknown", continue processing
7. **Testing**: Integration + unit tests, real PDFs, no mocks

No blocking issues identified. Ready for Phase 1 design.
