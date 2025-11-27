package llm

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/spherical/pdf-extractor/internal/domain"
)

const (
	maxRetries     = 3
	initialBackoff = 1 * time.Second
	maxBackoff     = 30 * time.Second
)

// RetryConfig holds retry configuration
type RetryConfig struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
}

// DefaultRetryConfig returns the default retry configuration
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxRetries:     maxRetries,
		InitialBackoff: initialBackoff,
		MaxBackoff:     maxBackoff,
	}
}

// shouldRetry determines if an error is retryable
func shouldRetry(statusCode int) bool {
	switch statusCode {
	case http.StatusTooManyRequests: // 429
		return true
	case http.StatusInternalServerError: // 500
		return true
	case http.StatusBadGateway: // 502
		return true
	case http.StatusServiceUnavailable: // 503
		return true
	case http.StatusGatewayTimeout: // 504
		return true
	default:
		return false
	}
}

// calculateBackoff calculates exponential backoff duration
func calculateBackoff(attempt int, config *RetryConfig) time.Duration {
	// Exponential backoff: initialBackoff * 2^attempt
	backoff := float64(config.InitialBackoff) * math.Pow(2, float64(attempt))

	// Cap at maxBackoff
	if backoff > float64(config.MaxBackoff) {
		backoff = float64(config.MaxBackoff)
	}

	return time.Duration(backoff)
}

// retryWithBackoff wraps an HTTP request with retry logic
func (c *Client) retryWithBackoff(ctx context.Context, reqFunc func() (*http.Response, error)) (*http.Response, error) {
	config := DefaultRetryConfig()
	var lastErr error

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Execute request
		resp, err := reqFunc()

		// Success case
		if err == nil && resp.StatusCode == http.StatusOK {
			return resp, nil
		}

		// Store error for later
		if err != nil {
			lastErr = err
		} else if resp != nil {
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)

			// Check if we should retry
			if !shouldRetry(resp.StatusCode) {
				return resp, nil // Return non-retryable errors immediately
			}

			// Close response body before retry
			if resp.Body != nil {
				resp.Body.Close()
			}
		}

		// Don't wait after last attempt
		if attempt == config.MaxRetries {
			break
		}

		// Calculate backoff
		backoff := calculateBackoff(attempt, config)
		domain.DefaultLogger.Warn("Request failed (attempt %d/%d), retrying in %v: %v",
			attempt+1, config.MaxRetries, backoff, lastErr)

		// Wait with context cancellation support
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
			// Continue to next attempt
		}
	}

	return nil, domain.APIError(fmt.Sprintf("request failed after %d retries", config.MaxRetries), lastErr)
}




