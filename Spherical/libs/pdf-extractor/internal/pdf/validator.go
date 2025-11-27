package pdf

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spherical/pdf-extractor/internal/domain"
)

// Validator provides input validation for PDF files
type Validator struct{}

// NewValidator creates a new validator instance
func NewValidator() *Validator {
	return &Validator{}
}

// ValidatePDFPath validates that a file path is valid and points to a PDF
func (v *Validator) ValidatePDFPath(path string) error {
	// Check if path is empty
	if strings.TrimSpace(path) == "" {
		return domain.ValidationError("file path cannot be empty", nil)
	}

	// Check if file exists
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return domain.ValidationError(fmt.Sprintf("file does not exist: %s", path), err)
		}
		return domain.ValidationError(fmt.Sprintf("cannot access file: %s", path), err)
	}

	// Check if it's a directory
	if info.IsDir() {
		return domain.ValidationError(fmt.Sprintf("path is a directory, not a file: %s", path), nil)
	}

	// Check file extension
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".pdf" {
		return domain.ValidationError(fmt.Sprintf("file is not a PDF (has extension %s)", ext), nil)
	}

	// Check file size (warn if very large, but don't reject)
	const maxSize = 100 * 1024 * 1024 // 100MB
	if info.Size() > maxSize {
		// Just a warning, not an error
		domain.DefaultLogger.Warn("PDF file is very large (%d MB), processing may take a while", info.Size()/(1024*1024))
	}

	// Check if file is readable
	file, err := os.Open(path)
	if err != nil {
		return domain.ValidationError(fmt.Sprintf("cannot open file: %s", path), err)
	}
	file.Close()

	return nil
}

// ValidateQuality validates image quality parameter
func (v *Validator) ValidateQuality(quality int) error {
	if quality < 1 || quality > 100 {
		return domain.ValidationError(fmt.Sprintf("quality must be between 1 and 100, got %d", quality), nil)
	}
	return nil
}




