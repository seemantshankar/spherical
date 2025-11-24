# Feature Specification: PDF Specification Extractor

**Feature Branch**: `001-pdf-spec-extractor`
**Created**: 2025-11-22
**Status**: Draft
**Input**: User description: "I want to create a Go library that takes in a document in the form of a PDF and performs the following steps... 1. Convert each page to a high quality (>85%) JPG image. 2. Send the images (Page by Page) to an **Openrouter** vision LLM (google/gemini-2.5-flash-preview-09-2025). Create a .env file and add it to gitignore where you will store all the credentials. Create a template and I will fill in the secrets myself. 3. The documents would be brochures / Spec Sheets / User Manuals, etc. for products that the platform has to sell to users, therefore, we need to extract all the specifications, features and unique selling propositions. We need to ensure that we only extract the relevant information and remove anything that isnt relevant to the product. 4. The extracted information should be stored in a temporary markdown file to be processed later. The markdown file is not a final product ready decision. I want to create this file to check that the extraction and cleansning is happening properly and no vital information is missed. 5. I want to pay special attention to tables in the brochures as many LLMs do not extract data within complex tables effectively. Please suggest best practices (or more capable LLMs) that specialise in extracting information from tables. Please help me expand these requriements to ensure we are not missing any edge conditions that may break the system in production."

## Clarifications

### Session 2025-11-22
- Q: Model Selection Strategy → A: Option B: Hybrid/Smart (Flash default, Pro optional via config)
- Q: Page Processing Strategy → A: Option A: Sequential (process pages one by one for stability)

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Extract Product Data from PDF (Priority: P1)

A developer or system administrator needs to extract structured product data (specs, features, USPs) from a PDF brochure so that it can be reviewed for accuracy before ingestion into the platform.

**Why this priority**: This is the core functionality of the library. Without this, the tool provides no value.

**Independent Test**: Can be fully tested by providing a sample PDF brochure and verifying that the generated Markdown file contains accurate specifications, features, and USPs, and that the structure of tables is preserved.

**Acceptance Scenarios**:

1. **Given** a valid PDF brochure with text, images, and tables, **When** the extraction function is run, **Then** a Markdown file is generated containing all product specifications, features, and USPs found in the document.
2. **Given** a PDF with a complex specification table, **When** processed, **Then** the Markdown output contains a Markdown table that accurately reflects the rows, columns, and data of the source table.
3. **Given** a PDF page with mixed content (marketing fluff + technical specs), **When** processed, **Then** the output contains the technical specs and excludes the generic marketing copy (unless it counts as a USP).
4. **Given** the environment is missing the `.env` file, **When** the library is initialized, **Then** it returns a clear error indicating missing configuration.
5. **Given** a long extraction process, **When** running, **Then** the user receives real-time streaming updates (e.g., via logs or callback) to confirm the system is active and working.

---

### User Story 2 - Handle Edge Cases and Errors (Priority: P2)

The system must be robust enough to handle common failure modes like corrupt files, network issues, or API limits without crashing or leaving the system in an undefined state.

**Why this priority**: Essential for production reliability.

**Independent Test**: Can be tested by simulating API errors, providing invalid files, or using large documents.

**Acceptance Scenarios**:

1. **Given** a corrupted or non-PDF file, **When** provided as input, **Then** the system returns a specific error message (e.g., "Invalid PDF format").
2. **Given** a large PDF (e.g., >50 pages), **When** processed, **Then** the system processes pages sequentially or with rate limiting to avoid 429 errors from the API.
3. **Given** an API rate limit response (429) from OpenRouter, **When** encountered, **Then** the system waits and retries with exponential backoff before failing.
4. **Given** a PDF with a blank page, **When** processed, **Then** the system skips the page or produces an empty entry for that page without crashing.

---

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The library MUST accept a file path to a local PDF document as input.
- **FR-002**: The system MUST convert each page of the PDF into a JPG image with a quality setting of at least 85%.
- **FR-003**: The system MUST send each converted page image to the OpenRouter API using the `google/gemini-2.5-flash-preview-09-2025` model by default.
- **FR-003a**: The system MUST allow configuration (via environment variable `LLM_MODEL`) to override the default model with a more capable one (e.g., `google/gemini-2.5-pro`) for better table extraction accuracy if needed.
- **FR-004**: The system MUST extract three specific categories of information:
    1. **Specifications**: Technical data, dimensions, power requirements, etc.
    2. **Features**: Functional capabilities of the product.
    3. **Unique Selling Propositions (USPs)**: Key differentiators highlighted in the document.
- **FR-005**: The system MUST filter out non-relevant information (e.g., generic legal disclaimers, unrelated marketing filler) that does not fall into the above categories.
- **FR-006**: The system MUST output the extracted information into a single Markdown file.
- **FR-007**: The system MUST employ prompting strategies specifically designed to preserve the structure of complex tables in the Markdown output (e.g., asking for Markdown table format explicitly, handling merged cells by duplicating values or logical splitting).
- **FR-008**: The library MUST load API credentials from a `.env` file in the project root.
- **FR-009**: The library MUST ignore the `.env` file in version control (via `.gitignore`) and provide a `.env.template` with placeholder values.
- **FR-010**: The system MUST implement error handling for OpenRouter API failures, including retries for transient errors and rate limits (HTTP 429).
- **FR-011**: The system MUST validate that the input file exists and is a valid PDF before attempting processing.
- **FR-012**: The system MUST clean up any temporary image files created during the process after the operation completes (success or failure).
- **FR-013**: The system MUST enable response streaming (OpenRouter API `stream: true`) to provide real-time feedback on the generation process.
- **FR-014**: The system MUST expose a mechanism (e.g., channel, callback, or real-time file writing) to allow callers to consume the streaming output as it is generated.
- **FR-015**: The system MUST process pages sequentially (one at a time) to ensure predictable resource usage and simplified error recovery.

### Key Entities

- **Document**: The source PDF file.
- **Page**: A single page of the document, converted to an image.
- **ExtractionResult**: The structured data (Specs, Features, USPs) extracted from a single page or the whole document.
- **Output**: The final Markdown file.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of tables visible in the source PDF are represented as Markdown tables in the output with correct row/column alignment.
- **SC-002**: Extracted Markdown contains at least 95% of the technical specifications present in the source text (verified by manual sampling).
- **SC-003**: System successfully processes a 20-page brochure in under 2 minutes (assuming standard API latency).
- **SC-004**: System handles API rate limits without crashing (verified by simulation).
- **SC-005**: Streaming output events are received within 2 seconds of API start for each page.
