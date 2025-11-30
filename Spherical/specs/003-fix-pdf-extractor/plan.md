# Implementation Plan: Fix PDF Extractor Output Quality

**Branch**: `003-fix-pdf-extractor` | **Date**: 2025-11-30 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/003-fix-pdf-extractor/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

This feature fixes four critical issues in the PDF specification extractor output:
1. Remove markdown codeblock delimiters from LLM output
2. Implement standard hierarchical nomenclature for specification categories
3. Extract variant and trim information from specification tables
4. Capture variant differentiation (checkboxes/symbols) in specification tables

The implementation involves updating the LLM prompt in `libs/pdf-extractor/internal/llm/client.go` and adding post-processing logic in `libs/pdf-extractor/internal/extract/service.go` to ensure clean markdown output compatible with the knowledge engine ingestion pipeline.

## Technical Context

**Language/Version**: Go 1.25.0  
**Primary Dependencies**: 
- Existing: `github.com/gen2brain/go-fitz` (MuPDF bindings for PDF conversion)
- OpenRouter API (via HTTP client for LLM)
- Standard library: `strings`, `regexp` for post-processing

**Storage**: N/A (in-memory processing, outputs to markdown files)  
**Testing**: Go standard testing (`testing` package), integration tests with real PDFs  
**Target Platform**: Linux/macOS (CLI tool and library)  
**Project Type**: Single project (library with CLI)  
**Performance Goals**: 
- Post-processing overhead: <10ms per page
- No degradation in extraction speed (LLM calls remain the bottleneck)
- Memory efficient: process pages sequentially, no full document buffering

**Constraints**: 
- Must maintain backward compatibility with existing extraction service interface
- Must not break existing tests
- LLM prompt changes must be carefully tested to avoid regressions
- Post-processing must be idempotent (safe to run multiple times)

**Scale/Scope**: 
- Single library modification (`libs/pdf-extractor`)
- ~200-300 lines of code changes (prompt updates + post-processing)
- 4-6 new test cases for edge cases
- Documentation updates for new prompt structure

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

### I. Test Driven Development (NON-NEGOTIABLE)
✅ **COMPLIANT**: All changes will follow TDD. Tests will be written first for:
- Post-processing codeblock removal
- Nomenclature standardization validation
- Variant extraction from tables
- Checkbox/symbol parsing

### II. Library First Approach
✅ **COMPLIANT**: Changes are within existing `libs/pdf-extractor` library. No new libraries required.

### III. CLI First Approach
✅ **COMPLIANT**: Existing CLI (`cmd/pdf-extractor`) will automatically benefit from fixes. No CLI changes needed.

### IV. Integration Testing Without Mocks
✅ **COMPLIANT**: Integration tests will use real PDFs and real OpenRouter API calls to verify end-to-end behavior.

### V. Real-Time Task List Updates
✅ **COMPLIANT**: Task list will be updated as implementation progresses.

### VI. Go Programming Language
✅ **COMPLIANT**: All code in Go, no exceptions.

### VII. Modular Architecture
✅ **COMPLIANT**: Changes maintain existing modular structure:
- `internal/llm/client.go` - LLM prompt updates
- `internal/extract/service.go` - Post-processing logic
- Clear separation of concerns maintained

### VIII. Module Size Limits
✅ **COMPLIANT**: Changes are additive and distributed across existing modules. No single module will exceed 500 lines.

### IX. Enterprise Grade Security and Architecture
✅ **COMPLIANT**: No security implications. Input validation already exists. No new attack surfaces.

### X. Comprehensive Documentation
⚠️ **ACTION REQUIRED**: Must document:
- New prompt structure and nomenclature guide
- Post-processing behavior and edge cases
- Variant extraction format in output markdown

### XI. Git Best Practices
✅ **COMPLIANT**: Feature branch `003-fix-pdf-extractor` already created. Will merge after tests pass.

**Overall Status**: ✅ **PASS** - All gates pass. Documentation updates required but not blocking.

## Project Structure

### Documentation (this feature)

```text
specs/003-fix-pdf-extractor/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
libs/pdf-extractor/
├── internal/
│   ├── llm/
│   │   └── client.go              # MODIFY: Update buildPrompt() with new instructions
│   └── extract/
│       └── service.go              # MODIFY: Add post-processing for codeblock removal
├── tests/
│   └── integration/
│       └── pdf_test.go            # ADD: Tests for new functionality
└── cmd/
    └── pdf-extractor/
        └── main.go                 # NO CHANGES: CLI automatically benefits
```

**Structure Decision**: Single project structure. Changes are contained within existing `libs/pdf-extractor` library. No new modules or services required.

## Complexity Tracking

> **No violations identified - all changes are additive improvements to existing functionality**

