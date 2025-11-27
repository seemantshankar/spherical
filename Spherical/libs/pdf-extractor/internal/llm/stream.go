package llm

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
)

// StreamParser handles parsing of Server-Sent Events (SSE) streams
type StreamParser struct {
	scanner *bufio.Scanner
}

// NewStreamParser creates a new stream parser
func NewStreamParser(reader io.Reader) *StreamParser {
	return &StreamParser{
		scanner: bufio.NewScanner(reader),
	}
}

// StreamChunk represents a single chunk from the stream
type StreamChunk struct {
	Content      string
	FinishReason string
	Done         bool
}

// Next reads the next chunk from the stream
func (p *StreamParser) Next() (*StreamChunk, error) {
	for p.scanner.Scan() {
		line := p.scanner.Text()

		// Skip non-data lines
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		// Extract data
		data := strings.TrimPrefix(line, "data: ")

		// Check for end marker
		if data == "[DONE]" {
			return &StreamChunk{Done: true}, nil
		}

		// Parse JSON
		var resp Response
		if err := json.Unmarshal([]byte(data), &resp); err != nil {
			// Skip invalid JSON lines
			continue
		}

		// Extract content
		if len(resp.Choices) > 0 {
			choice := resp.Choices[0]
			return &StreamChunk{
				Content:      choice.Delta.Content,
				FinishReason: choice.FinishReason,
				Done:         choice.FinishReason != "",
			}, nil
		}
	}

	// Check for scanner errors
	if err := p.scanner.Err(); err != nil {
		return nil, err
	}

	// End of stream
	return &StreamChunk{Done: true}, nil
}

// ParseAll reads all chunks from the stream and sends them to a channel
func (p *StreamParser) ParseAll(resultCh chan<- string) error {
	for {
		chunk, err := p.Next()
		if err != nil {
			return err
		}

		// Send content if present (even if this is the final chunk)
		if chunk.Content != "" {
			resultCh <- chunk.Content
		}

		// Break after sending content if this is the final chunk
		if chunk.Done {
			break
		}
	}

	return nil
}

