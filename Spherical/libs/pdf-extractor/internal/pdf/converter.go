package pdf

import (
	"context"
	"fmt"
	"image/jpeg"
	"os"
	"path/filepath"

	"github.com/gen2brain/go-fitz"
	"github.com/spherical/pdf-extractor/internal/domain"
)

// Converter implements PDF to image conversion using go-fitz
type Converter struct {
	doc       *fitz.Document
	tempFiles []string
	tempDir   string
}

// NewConverter creates a new PDF converter instance
func NewConverter() *Converter {
	return &Converter{
		tempFiles: make([]string, 0),
	}
}

// Convert converts a PDF file to a series of high-quality JPG images
func (c *Converter) Convert(ctx context.Context, pdfPath string, quality int) ([]domain.PageImage, error) {
	// Validate input
	validator := NewValidator()
	if err := validator.ValidatePDFPath(pdfPath); err != nil {
		return nil, err
	}
	if err := validator.ValidateQuality(quality); err != nil {
		return nil, err
	}

	// Open PDF document
	doc, err := fitz.New(pdfPath)
	if err != nil {
		return nil, domain.ConversionError("Failed to open PDF", err)
	}
	c.doc = doc

	// Create temporary directory for images
	tempDir, err := os.MkdirTemp("", "pdf-extractor-*")
	if err != nil {
		return nil, domain.IOError("Failed to create temp directory", err)
	}
	c.tempDir = tempDir

	// Get page count
	pageCount := doc.NumPage()
	if pageCount == 0 {
		return nil, domain.ValidationError("PDF has no pages", nil)
	}

	// Convert each page
	images := make([]domain.PageImage, 0, pageCount)

	for pageNum := 0; pageNum < pageCount; pageNum++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		img, err := doc.Image(pageNum)
		if err != nil {
			return nil, domain.ConversionError(fmt.Sprintf("Failed to convert page %d", pageNum+1), err)
		}

		// Save as JPG
		outputPath := filepath.Join(tempDir, fmt.Sprintf("page_%03d.jpg", pageNum+1))
		outputFile, err := os.Create(outputPath)
		if err != nil {
			return nil, domain.IOError(fmt.Sprintf("Failed to create output file for page %d", pageNum+1), err)
		}

		// Encode with specified quality
		opts := &jpeg.Options{Quality: quality}
		err = jpeg.Encode(outputFile, img, opts)
		outputFile.Close()
		if err != nil {
			return nil, domain.ConversionError(fmt.Sprintf("Failed to encode page %d as JPG", pageNum+1), err)
		}

		// Track temp file for cleanup
		c.tempFiles = append(c.tempFiles, outputPath)

		// Get image dimensions
		bounds := img.Bounds()
		pageImage := domain.PageImage{
			PageNumber: pageNum + 1,
			ImagePath:  outputPath,
			Width:      bounds.Dx(),
			Height:     bounds.Dy(),
		}

		images = append(images, pageImage)
	}

	return images, nil
}

// Cleanup removes temporary files and closes the PDF document
func (c *Converter) Cleanup() error {
	var errs []error

	// Close document
	if c.doc != nil {
		c.doc.Close()
		c.doc = nil
	}

	// Remove temporary directory
	if c.tempDir != "" {
		err := os.RemoveAll(c.tempDir)
		if err != nil {
			errs = append(errs, err)
		}
		c.tempDir = ""
	}

	c.tempFiles = nil

	if len(errs) > 0 {
		return fmt.Errorf("cleanup errors: %v", errs)
	}

	return nil
}
