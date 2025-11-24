# PDF Specification Extractor – Functional Specification

- **Document Version**: 1.0  
- **Last Updated**: 2025-11-23  
- **Owner**: Spherical AI Platform Team  
- **Audience**: Product, Engineering, QA, and DevOps stakeholders

## 1. Overview

The PDF Specification Extractor ingests multi-page marketing or technical brochures in PDF form, converts each page into high-quality JPG images, and uses a vision-capable LLM (via OpenRouter) to extract structured specifications, tables, and unique selling points (USPs). The system is exposed both as a CLI executable and as an embeddable Go library with streaming progress events.

## 2. Goals & Non-Goals

- **Goals**
  - Produce auditable Markdown artifacts containing tabular specs, feature bullets, and USPs.
  - Provide real-time progress for CLI and library callers.
  - Remain resilient to PDF conversion or OpenRouter instability.
  - Keep resource usage predictable for long-running pipelines.
- **Non-Goals**
  - OCR of scanned PDFs (out of scope for V1).
  - Automatic translation/localization of extracted specs.

## 3. Stakeholders

- Product owner (feature prioritization, success criteria).
- Platform engineers (CLI + library implementers).
- QA / Validation (acceptance scenarios and regression suites).
- Solution integrators (embedder teams consuming Go API/events).

## 4. Interfaces & Deployment Modes

1. **CLI (`cmd/pdf-extractor`)**
   - Accepts a PDF path and optional flags (`--output`, `--jpg-quality`, `--model`, `--stream-json`).
   - Streams human-readable progress plus optional JSON events for piping into other tools.
2. **Go Library (`pkg/extractor`)**
   - `NewClient()` loads configuration from environment or explicit `extractor.Config`.
   - `Process(ctx, pdfPath)` returns a receive-only channel of typed events (`EventStart`, `EventPageProcessing`, `EventLLMStreaming`, `EventPageComplete`, `EventError`, `EventComplete`).
3. **Automation Pipelines**
   - May run sequential CLI invocations or embed the library inside job runners.

## 5. Assumptions & External Dependencies

1. **MuPDF 1.24.9** (via `go-fitz`) is installed and kept in sync with Go bindings.
2. **OpenRouter availability** with configured Gemini 2.5 Flash (default) or Gemini 2.5 Pro models.
3. **Network Stability**: outbound HTTPS access to OpenRouter with <200 ms median latency; mitigations documented when this assumption fails.
4. **Storage**: Temporary disk space (2× PDF size) is available for intermediate JPG and Markdown artifacts.

## 6. Use Cases & Scenarios

- **US1 – Marketing Brochure Extraction**
  - *Scenario 1*: Balanced marketing/spec pages → extractor produces Markdown per page.
  - *Scenario 2*: Mixed content with repeated tables → deduped output while keeping structure.
  - *Scenario 3*: Marketing-heavy pages lacking specs → extractor records placeholders (“No actionable specs detected”) and logs reasons instead of hallucinating.
- **US2 – Technical Datasheet Extraction**
  - *Scenario 4*: Conversion failure mid-document → partial Markdown retained, retries attempted, cleanup rules triggered.
- **US3 – Pipeline Streaming Consumer**
  - *Scenario 5*: Library embedder subscribes to streaming events and surfaces real-time progress bars with <750 ms average latency between PDF conversion finish and first LLM chunk.

## 7. Functional Requirements

- **FR-001 – PDF Validation**
  - Reject non-existent files, enforce `.pdf` extension, ensure readable permissions, and emit `EventError` with actionable hints.
- **FR-002 – Image Conversion Quality**
  - Default JPG quality = 90 (≥85).  
  - Configurable via hierarchy: CLI flag `--jpg-quality`, env var `JPG_QUALITY`, config struct field `Config.JPGQuality`, or per-page overrides via callback.  
  - Documentation states when to lower/raise quality (e.g., reduce to 80 for 200+ page docs to conserve disk).
- **FR-003 – Model Selection**
  - Default `google/gemini-2.5-flash-preview-09-2025`; override via env (`LLM_MODEL`), CLI flag `--model`, or `Config.Model`.
- **FR-004 – Relevant Content Filter**
  - Heuristics enumerated for “non-relevant information”: exclude brand slogans, lifestyle copy, disclaimers unless they include quantitative specs.  
  - Positive inclusion rules: keep items mentioning measurements/ratings, tables, feature bullets containing verifiable attributes.
- **FR-005 – USP Extraction**
  - Maintain separate USP section per page.  
  - Apply tone policy: keep trademarked marketing cues (e.g., “Thor’s Hammer headlights”) but append clarifying spec if known.  
  - Disallow duplicating USPs already represented in the specification table unless additional qualitative nuance exists.
- **FR-006 – Temporary Markdown File Handling**
  - Markdown accumulated per page in `./tmp/<pdf-name>/<timestamp>/`.  
  - Filename schema `<original>-page-<n>.md` and `combined.md`.  
  - Created with restrictive permissions (`0600`).  
  - Cleanup: remove tmp directory after successful completion; retain on failure with pointer in final log. Ownership: CLI/library is responsible for cleanup; pipeline operators need not run extra scripts.
- **FR-007 – Table Preservation**
  - Preserve merged cells by emitting Markdown with HTML row/col spans when necessary.  
  - Repeat header rows on each page section when multi-page tables exist.  
  - Column alignment rules documented (numeric right-aligned, text left).  
  - Provide fallback plain-text representation when table confidence <0.8 and log warning.
- **FR-008 – Secret Management**
  - `.env` loading supported but API keys never logged.  
  - Secrets redacted in debug logs (`OPENROUTER_API_KEY` → `sk-****`).
- **FR-009 – Logging & Security**
  - Logs exclude PDF contents, redact personally identifiable/custom data in streamed events, and rotate log files when CLI `--log-file` is used.
- **FR-010 – OpenRouter Retry & Backoff**
  - Applies to 429/5xx responses.  
  - Max 5 attempts, exponential backoff with jitter: base=1 s, multiplier=2, jitter ±250 ms, cap=16 s.  
  - Emits `EventRetry` with attempt count; after final failure, returns structured error containing HTTP status, response snippet, and correlation ID.
- **FR-011 – Output Surfaces**
  - CLI writes Markdown file (default `<pdf>-specs.md`), optionally stream aggregated JSON summary when `--summary-json <path>` is provided.  
  - Library returns final `EventComplete` payload containing Markdown string, aggregated tables, and USP slice.
- **FR-012 – Partial Output & Recovery**
  - If conversion fails mid-run, completed pages stay persisted in tmp directory, final Markdown includes “Partial Output” banner, and CLI exit code = 2.  
  - Retry logic limited to failing page; rest of pipeline kept intact.
- **FR-013 – Streaming UX**
  - CLI displays spinner + percentage updates; latency between page completion and CLI output <1 s.  
  - Library exposes channel events with typed payloads; consumer guide included.
- **FR-014 – Interface Responsibilities**
  - CLI responsible for human-readable progress, file IO, and cleanup.  
  - Library responsible for event emission, returning aggregated structs, and letting embedders decide on persistence.  
  - Streaming consumers can enable both CLI spinner and JSON events without duplicate work.
- **FR-015 – Sequential Processing & Performance**
  - Process pages sequentially to cap RSS at 500 MB.  
  - Must process 20-page brochure in <2 minutes on reference hardware (M3 Pro, 32 GB) assuming OpenRouter latency <500 ms per chunk.  
  - Queue overflow policy documented for >100 pages (warn operator and continue sequentially).

## 8. Non-Functional Requirements

- **NFR-001 – Memory Footprint**: RSS <500 MB, tmp storage <2× PDF size.
- **NFR-002 – Observability**: Structured logs with correlation IDs per run.
- **NFR-003 – Security**: `.env` access limited to local filesystem; instructions for rotating API keys every 90 days.
- **NFR-004 – Reliability**: SLA 99% success across nightly regression set; retries + resume behavior documented.

## 9. Success Criteria

- **SC-001 – Spec Coverage**: ≥95% of verifiable specs captured vs human baseline sample of 20 documents; sampling instructions defined in QA plan.
- **SC-002 – Table Fidelity**: 100% of tables preserved (structure & values) for baseline set of 10 docs; QA records diff artifacts.
- **SC-003 – Performance**: 20-page Volvo XC90 brochure processed in <2 minutes end-to-end (excluding network spikes >1 s). Benchmark environment documented.
- **SC-004 – Streaming Latency**: Average <750 ms from `EventPageComplete` to first `EventLLMStreaming` chunk.
- **SC-005 – Streaming Reliability**: No dropped events in 10 sequential runs; CLI spinner and library events stay consistent.

## 10. Acceptance Scenarios

1. **Scenario A – Happy Path CLI**: CLI run on brochure with mix of tables/USPs completes successfully, generates Markdown file, and deletes tmp directory.
2. **Scenario B – Marketing-Heavy Pages**: Document with 3 consecutive marketing-only pages records “No actionable specs” stubs while keeping USPs defined in FR-005.
3. **Scenario C – Conversion Failure**: Page 12 conversion fails; system retries page only, emits `EventRetry`, persists partial file, exits with code 2, logs cleanup pointer as per FR-012.
4. **Scenario D – Streaming Consumer**: Library embedder verifies event order and latency meets SC-004; CLI spinner optional.
5. **Scenario E – Rate Limit Storm**: OpenRouter returns 429 for first 3 attempts; system demonstrates FR-010 jittered retries and final success/failure handling.

## 11. Traceability Matrix

| Requirement | Verification Method |
|-------------|---------------------|
| FR-001, FR-002 | Integration tests `tests/integration/pdf_test.go` |
| FR-004–FR-007 | QA sampling using Volvo XC90 brochures |
| FR-010 | `internal/llm/retry.go` unit tests + perf logs |
| FR-013–FR-014 | Streaming tests `tests/integration/stream_test.go` |
| SC-001–SC-005 | Manual QA checklist + automated benchmarks |
