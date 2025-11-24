# Implementation Plan: PDF Specification Extractor

**Branch**: `001-pdf-spec-extractor` | **Date**: 2025-11-22 | **Spec**: [link](./spec.md)
**Input**: Feature specification from `/specs/001-pdf-spec-extractor/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

Create a Go library to extract product specifications from PDF documents. The library will convert PDF pages to high-quality JPG images, process them using OpenRouter (Gemini 2.5 Flash/Pro) via a vision-capable LLM, and output structured Markdown containing specs, features, and USPs. It must handle complex tables effectively, support sequential processing, and provide streaming feedback.

## Technical Context

**Language/Version**: Go 1.25.4+ (Latest Stable)
**Primary Dependencies**: 
- `github.com/gen2brain/go-fitz` (Latest, requires MuPDF 1.24.9)
- `net/http` (Standard lib)
- `github.com/joho/godotenv` (v1.5.1)
**Storage**: Filesystem (temporary images, output Markdown)
**Testing**: Go `testing` package
**Target Platform**: Cross-platform (Linux/macOS/Windows)
**Project Type**: Library (Single module)
**Performance Goals**: Process 20-page brochure < 2 mins
**Constraints**: Low memory footprint (sequential processing), handle API rate limits
**Scale/Scope**: < 1000 LOC estimated, focus on robustness and error handling

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- [x] **I. Test Driven Development**: Plan includes test strategy (contracts/unit).
- [x] **II. Library First Approach**: Feature is explicitly defined as a standalone Go library.
- [x] **III. CLI First Approach**: Feature will be exposed via CLI for testing/usage.
- [x] **IV. Integration Testing Without Mocks**: Plan includes real integration tests with OpenRouter (mocking only for cost/rate limit simulations if needed, but preferring real integration for contract verification).
- [x] **V. Real-Time Task List Updates**: Tasks will be tracked in `tasks.md`.
- [x] **VI. Go Programming Language**: Implementation is in Go.
- [x] **VII. Modular Architecture**: Library will be modular (PDF handling, API client, Extraction logic).
- [x] **VIII. Module Size Limits**: Modules kept small.
- [x] **IX. Enterprise Grade Security**: Env vars for secrets, standard HTTP security.
- [x] **X. Comprehensive Documentation**: Plan includes `quickstart.md` and contract docs.
- [x] **XI. Git Best Practices**: Feature branch `001-pdf-spec-extractor` used.

## Project Structure

### Documentation (this feature)

```text
specs/001-pdf-spec-extractor/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
pdf-extractor/           # Root of the new library (or similar name)
├── cmd/
│   └── pdf-extractor/   # CLI entrypoint
├── internal/
│   ├── pdf/             # PDF processing (conversion to image)
│   ├── llm/             # OpenRouter API client
│   └── extract/         # Core extraction logic
├── pkg/                 # Public API (if any)
├── go.mod
└── go.sum

tests/
├── integration/         # End-to-end tests with real PDF/API
└── unit/                # Unit tests for logic
```

**Structure Decision**: Standard Go project layout. `cmd/` for the CLI, `internal/` for the library logic (to enforce encapsulation unless `pkg/` is needed for external consumers, which it likely is for a library).

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| N/A | | |

