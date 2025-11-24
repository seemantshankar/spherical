# Research Findings: PDF Specification Extractor

**Date**: 2025-11-22
**Branch**: `001-pdf-spec-extractor`

## Decisions & Rationale

### 1. PDF to Image Conversion Library

**Decision**: `github.com/gen2brain/go-fitz` (via CGO) or `github.com/Mindinventory/Golang-PDF-to-Image-Converter` (if Pure Go is critical).
*Refinement*: Given the Constitution's preference for standalone Go libraries and the context of this being a library itself, using CGO (`go-fitz`) is powerful but complicates cross-compilation. However, `go-fitz` is widely regarded as the most robust because it wraps MuPDF, the gold standard for rendering. `pdf-to-jpg` is a CLI tool, not a library we can easily import. `ConvertAPI` and `Aspose` are commercial/external dependencies which violate the goal of a self-contained library (unless we want to vend another API).
**Selected**: `github.com/gen2brain/go-fitz`
**Rationale**: High-quality rendering is the #1 requirement ("high quality (>85%) JPG"). MuPDF (wrapped by `go-fitz`) provides the best rendering fidelity of any open-source tool. While it requires CGO, the trade-off for quality is worth it.
**Alternatives Considered**:
- `ConvertAPI`/`Aspose`: Rejected due to commercial cost/external API dependency for a core local function.
- `Mindinventory/Golang-PDF-to-Image-Converter`: A strong second choice, but `go-fitz` is more battle-tested for rendering fidelity.
- `unidoc/unipdf`: Rejected due to "less optimal" image conversion feedback.

### 2. Image Processing
**Decision**: Standard `image/jpeg` package in Go.
**Rationale**: Simple, standard library support for encoding images with specific quality settings (`jpeg.Options{Quality: 85}`).

### 3. OpenRouter Client
**Decision**: Standard `net/http` with custom struct for request/response.
**Rationale**: The OpenRouter API is compatible with OpenAI's chat/completions format (mostly), but since we need specific "vision" payload structures and beta "responses" endpoints (as per user's search result), a lightweight, typed client using `net/http` is preferred over a heavy external SDK. It ensures we own the exact JSON shape (critical for `stream: true` handling).

### 4. Streaming Handling
**Decision**: Server-Sent Events (SSE) parser.
**Rationale**: The API returns `data: {...}` lines. We need a simple loop reading from `response.Body` line-by-line to parse these events and emit them to the caller.

### 5. Configuration
**Decision**: `github.com/joho/godotenv`
**Rationale**: Standard Go community choice for loading `.env` files.

## Best Practices for Table Extraction (Research from Prompt Engineering)
**Findings**:
- **Format**: Explicitly request "Markdown table" format in the system prompt.
- **Structure**: Use "Chain of Thought" or specific instructions: "Identify the table structure first, then extract row by row."
- **Model**: Gemini models are multimodal and generally good at this, but giving a "one-shot" example in the prompt of how to handle a complex merged cell (e.g., "repeat the value" or "use <br>") helps significantly.
- **Cleanliness**: Instruct the model to "Exclude any rows that are purely decorative or empty."

## Resolved Clarifications
- **PDF Lib**: `go-fitz` chosen for quality.
- **Model**: `google/gemini-2.5-flash-preview-09-2025` (default) + `google/gemini-2.5-pro` (opt-in).
- **Processing**: Sequential (page-by-page).

