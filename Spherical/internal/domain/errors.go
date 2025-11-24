package domain

import "fmt"

// Error types for domain-specific errors
type ErrorType string

const (
	ErrorTypeValidation ErrorType = "validation"
	ErrorTypeConversion ErrorType = "conversion"
	ErrorTypeExtraction ErrorType = "extraction"
	ErrorTypeAPI        ErrorType = "api"
	ErrorTypeConfig     ErrorType = "config"
	ErrorTypeIO         ErrorType = "io"
)

// DomainError represents a domain-specific error with context
type DomainError struct {
	Type    ErrorType
	Message string
	Err     error
}

func (e *DomainError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Type, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Type, e.Message)
}

func (e *DomainError) Unwrap() error {
	return e.Err
}

// NewError creates a new domain error
func NewError(errType ErrorType, message string, err error) *DomainError {
	return &DomainError{
		Type:    errType,
		Message: message,
		Err:     err,
	}
}

// Common error constructors
func ValidationError(message string, err error) *DomainError {
	return NewError(ErrorTypeValidation, message, err)
}

func ConversionError(message string, err error) *DomainError {
	return NewError(ErrorTypeConversion, message, err)
}

func ExtractionError(message string, err error) *DomainError {
	return NewError(ErrorTypeExtraction, message, err)
}

func APIError(message string, err error) *DomainError {
	return NewError(ErrorTypeAPI, message, err)
}

func ConfigError(message string, err error) *DomainError {
	return NewError(ErrorTypeConfig, message, err)
}

func IOError(message string, err error) *DomainError {
	return NewError(ErrorTypeIO, message, err)
}

