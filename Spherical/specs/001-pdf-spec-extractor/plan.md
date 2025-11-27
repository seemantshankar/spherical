# PDF Specification Extractor – Delivery Plan

- **Document Version**: 1.1  
- **Last Updated**: 2025-11-24  
- **Owner**: Delivery Lead – PDF Extractor

## 1. Summary

We will ship a cross-platform PDF extraction solution consisting of:

1. A Go library (`pkg/extractor`) that exposes a streaming API for pipelines.
2. A CLI wrapper (`cmd/pdf-extractor`) for operators and QA.
3. Support utilities (table fixers, USP enhancers) to prepare outputs.

Milestones:

| Milestone | Date | Exit Criteria |
|-----------|------|---------------|
| M1 – Core Extraction | 2025-11-28 | CLI renders JPGs ≥85 quality, LLM returns Markdown |
| M2 – Streaming & Events | 2025-12-02 | Library channel events documented and validated |
| M3 – Resilience & QA | 2025-12-06 | Retry logic, cleanup guarantees, SC-001…SC-005 met |
| M4 – Document Categorization | 2025-12-10 | Automatic categorization (Domain, Subdomain, Country Code, Model Year, Condition, Make, Model) detected and included in Markdown header, EventComplete payload, and summary JSON (FR-016) |

## 2. Project Structure

| Component | Responsibilities | Hand-off |
|-----------|------------------|----------|
| CLI | Argument parsing, env loading, writing Markdown files, surfacing progress spinners. | CLI owns temp directory lifecycle, output naming, and human logs. |
| Library API | PDF conversion orchestration, LLM calls, event fan-out, returning aggregated structs (`Markdown`, `Tables`, `USPs`, `DocumentMetadata`). | Library never persists files; embedders decide on storage. |
| Streaming UX | Shared event schema consumed by CLI (spinner) and library embedder callbacks. | Event ordering enforced: Start → PageProcessing → LLMStreaming → PageComplete → Complete/Error. |
| Temp Artifacts | `./tmp/<pdf>/...` tree shared between CLI/library but orchestrated inside `internal/extract`. | Cleanup triggered when CLI exit status = 0; non-zero leaves breadcrumbs for operators. |
| Document Categorization | LLM-based analysis of cover page (fallback to first page if cover blank/unreadable; continue to subsequent pages sequentially until clear categorization information is found) to extract Domain, Subdomain, Country Code, Model Year, Condition, Make, Model. Uses majority vote across pages for conflict resolution. Injects YAML frontmatter header into Markdown output. Confidence threshold >70% based on LLM response scores (heuristic fallback if unavailable). | Categorization metadata included in `EventComplete` payload and `--summary-json` output for programmatic access. Validation: ISO 3166-1 alpha-2 for country codes, predefined domain list, reasonable model year range (1900-2100). |

## 3. Technical Context

- **Runtime**: Go 1.25.4 with CGO enabled.
- **PDF Conversion**: `go-fitz` + MuPDF 1.24.9 (documented install steps). Sequential processing to cap memory <500 MB.
- **LLM Access**: OpenRouter (Gemini 2.5 Flash default). Config fallback to Gemini 2.5 Pro when higher accuracy needed; CLI flag `--model`.
- **Image Quality Control**: `--jpg-quality` flag propagates to converter; recommended range 80–95. Provide doc table mapping scenarios → quality (e.g., “low-contrast scans: 92”, “200+ pages: 82”).
- **Temporary Markdown Files**: Ownership, naming, retention spelled out to align FR-006/FR-012.
- **Document Categorization**: YAML frontmatter format (between `---` delimiters) for categorization header. LLM confidence scores (>70% threshold) with heuristic fallback. Cover page precedence with sequential page fallback (first page, then subsequent pages until clear categorization information is found). Majority vote conflict resolution across pages. Domain-specific validation (ISO 3166-1 alpha-2 country codes, predefined domains, reasonable model year range).

## 4. Dependencies & Compatibility

1. **MuPDF 1.24.9** – Verified via `pkg-config --modversion mupdf`. Upgrade guidance: run `brew upgrade mupdf` followed by regression `go test ./tests/integration/...`.
2. **OpenRouter Availability** – Document fallback script to queue jobs locally if API unreachable for >5 minutes.
3. **Gemini Model Access** – Requires enabling both Flash and Pro models; if Pro unavailable, degrade gracefully by raising warning before run.
4. **Disk Space** – Minimum free disk (2× PDF size + 200 MB headroom) asserted before run.

## 5. Constraints & Assumptions

- **Latency Assumption**: OpenRouter p95 latency ≤800 ms; when higher, CLI warns and suggests switching to streaming chunk size 256 tokens.
- **Rate Limit Assumption**: 60 RPM default; if sustained 429s for >2 minutes, run enters “degraded mode” (serialize jobs, extend backoff to 32 s) and surfaces instructions for operators.
- **Security**: Operators manage `.env` via secret store; instructions for rotating keys every 90 days and storing outside repos.
- **Environment Parity**: Benchmark hardware defined (Apple M3 Pro, macOS 15.1, 32 GB RAM). Linux parity validated via nightly CI run on c7g.2xlarge.

## 6. Risk Mitigation

| Risk | Mitigation |
|------|------------|
| MuPDF upgrade breaks CGO | Pin via `pkg-config` check, add CI guard. |
| OpenRouter outage | Queue jobs locally, allow offline dry-run using cached JPGs. |
| Large PDFs exceed temp storage | Pre-run disk check, configurable `TMPDIR`. |
| Marketing-heavy docs produce noise | Heuristic filters (FR-004/FR-005) plus QA sampling instructions. |

## 7. QA & Validation Plan

1. **Sampling Procedure** (SC-001/SC-002): QA reviews 20-document set weekly, records % specs captured and table fidelity.
2. **Performance Benchmark** (SC-003): Run `go test -v ./tests/integration/perf_test.go -run TestTwentyPageThroughput` on reference hardware; log environment details in `TEST_RESULTS_COMPARISON.md`.
3. **Streaming Validation** (SC-004/SC-005): Use `tests/integration/stream_test.go` with CLI `--stream-json` to ensure CLI + library share schema and no events dropped.
4. **Failure Scenarios**: Execute US2 Scenario 4 (forced conversion failure) and Scenario E (429 storm) each release candidate.

## 8. Work Breakdown

- **Task T1 – Converter Enhancements**: Ensure JPG quality overrides, disk checks, and logging (Owner: PDF team).
- **Task T2 – LLM Client**: Implement retry/backoff, streaming chunk aggregator (Owner: LLM team).
- **Task T3 – Temp Markdown Lifecycle**: Directory creation, retention policies, CLI flag to keep artifacts (Owner: Platform).
- **Task T4 – Streaming UX**: Align CLI spinner + JSON events, document event order (Owner: Developer Experience).
- **Task T5 – QA Harness**: Maintain benchmark data, sampling scripts (Owner: QA).
- **Task T6 – Document Categorization**: Implement automatic document categorization (Domain, Subdomain, Country Code, Model Year, Condition, Make, Model) detection via LLM analysis of cover page (fallback to first page, then continue sequentially to subsequent pages until clear categorization information is found). Use LLM confidence scores (>70% threshold) with heuristic fallback. Implement majority vote conflict resolution across pages. Inject YAML frontmatter header (between `---` delimiters) at top of Markdown output. Include validation helpers (ISO 3166-1 alpha-2 country codes, predefined domains, model year range 1900-2100). Include in `EventComplete` payload and `--summary-json` (Owner: LLM team, dependencies: T2).

## 9. Communication & Reporting

- Daily Slack check-in (#pdf-extractor) with progress + blockers.
- Twice-weekly demo of streaming UI updates.
- Release candidate sign-off requires completed requirements checklist (this document + `checklists/requirements-quality.md`).

## 10. Appendix – Configuration Reference

| Setting | Surface | Description |
|---------|---------|-------------|
| `--jpg-quality` / `JPG_QUALITY` | CLI flag / env | 80–95, default 90. |
| `--stream-json` | CLI flag | Emits JSON event stream for pipelines. |
| `TMPDIR` | Env | Override default temp root; CLI logs chosen path. |
| `--summary-json` | CLI flag | Writes aggregated struct for consumers who do not use Markdown. |


