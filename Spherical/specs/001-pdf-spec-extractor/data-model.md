# Data Model: PDF Specification Extractor

## Entities

### 1. Document
Represents the source file being processed.

```go
type Document struct {
    FilePath string
    TotalPages int
    // Metadata could go here
}
```

### 2. PageImage
Represents a converted single page.

```go
type PageImage struct {
    PageNumber int
    ImagePath  string // Path to temp JPG file
    Width      int
    Height     int
}
```

### 3. ExtractionResult
The structured data extracted from a single page or combined document.

```go
type ExtractionResult struct {
    Specifications []Specification `json:"specifications"`
    Features       []string        `json:"features"`
    USPs           []string        `json:"usps"`
    RawMarkdown    string          `json:"raw_markdown"` // The full markdown output from LLM
}

type Specification struct {
    Category string `json:"category"` // e.g. "Dimensions", "Power"
    Name     string `json:"name"`
    Value    string `json:"value"`
}
```

### 4. ProcessingStats
Metadata about the execution.

```go
type ProcessingStats struct {
    TotalTime       time.Duration
    PagesProcessed  int
    SuccessfulPages int
    FailedPages     int
    Errors          []error
}
```

### 5. StreamEvent
The event structure emitted during streaming.

```go
type EventType string

const (
    EventStart          EventType = "start"
    EventPageProcessing EventType = "page_processing"
    EventLLMStreaming   EventType = "llm_streaming" // Chunk of text
    EventPageComplete   EventType = "page_complete"
    EventError          EventType = "error"
    EventComplete       EventType = "complete"
)

type StreamEvent struct {
    Type       EventType   `json:"type"`
    PageNumber int         `json:"page_number,omitempty"`
    Payload    interface{} `json:"payload,omitempty"` // Text chunk or status message
    Timestamp  time.Time   `json:"timestamp"`
}
```

## API Contracts (Internal Library Interfaces)

### 1. Converter Interface (PDF -> Images)

```go
type Converter interface {
    // Convert turns a PDF into a slice of image paths
    Convert(ctx context.Context, pdfPath string, quality int) ([]PageImage, error)
}
```

### 2. Extractor Interface (Images -> Data)

```go
type Extractor interface {
    // Extract processes a single page image and returns the result
    // StreamCallback is called with chunks of generated text if provided
    Extract(ctx context.Context, image PageImage, streamCh chan<- StreamEvent) (*ExtractionResult, error)
}
```

### 3. Pipeline (Orchestrator)

```go
type Pipeline struct {
    Converter Converter
    Extractor Extractor
}

func (p *Pipeline) Process(ctx context.Context, pdfPath string) (<-chan StreamEvent, error)
```

## Data Flow

1. **Input**: User provides `pdfPath`.
2. **Validation**: File exists, is PDF.
3. **Conversion**: `Converter` runs `go-fitz`, creates temp JPGs.
4. **Orchestration**: Loop through images:
   - Emit `EventPageProcessing`.
   - Call `Extractor.Extract(image)`.
   - `Extractor` calls OpenRouter API.
   - API streams chunks -> `EventLLMStreaming`.
   - Accumulate result.
   - Emit `EventPageComplete`.
5. **Aggregation**: Combine all `ExtractionResult`s (simple concatenation for Markdown).
6. **Output**: Final Markdown file + `EventComplete`.

