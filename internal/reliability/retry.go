package reliability

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/rand"
	"time"
)

// RetryConfig configures retry behavior
type RetryConfig struct {
	MaxAttempts     int
	InitialDelay    time.Duration
	MaxDelay        time.Duration
	Multiplier      float64
	Jitter          bool
	RetryableErrors func(error) bool
}

// DefaultRetryConfig returns a sensible default configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		Jitter:       true,
		RetryableErrors: func(err error) bool {
			// By default, retry all errors
			return true
		},
	}
}

// Retry executes a function with retry logic
func Retry(ctx context.Context, config RetryConfig, fn func() error) error {
	var lastErr error
	delay := config.InitialDelay

	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		// Check context before attempting
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Execute the function
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if config.RetryableErrors != nil && !config.RetryableErrors(err) {
			return fmt.Errorf("non-retryable error: %w", err)
		}

		// Don't retry if this was the last attempt
		if attempt == config.MaxAttempts {
			break
		}

		// Calculate next delay
		if config.Jitter {
			// Add jitter to prevent thundering herd
			jitter := time.Duration(rand.Float64() * float64(delay) * 0.3)
			delay = delay + jitter
		}

		// Cap at max delay
		if delay > config.MaxDelay {
			delay = config.MaxDelay
		}

		log.Printf("Retry: Attempt %d/%d failed, retrying in %v: %v",
			attempt, config.MaxAttempts, delay, err)

		// Wait with context cancellation
		select {
		case <-time.After(delay):
			// Continue to next attempt
		case <-ctx.Done():
			return ctx.Err()
		}

		// Exponential backoff for next iteration
		delay = time.Duration(float64(delay) * config.Multiplier)
	}

	return fmt.Errorf("max attempts (%d) exceeded: %w", config.MaxAttempts, lastErr)
}

// RetryWithBackoff is a convenience function with exponential backoff
func RetryWithBackoff(ctx context.Context, fn func() error) error {
	return Retry(ctx, DefaultRetryConfig(), fn)
}

// RetryOperation represents a retryable operation with state
type RetryOperation struct {
	Name        string
	Config      RetryConfig
	Attempts    int
	LastError   error
	LastAttempt time.Time
	StartTime   time.Time
	EndTime     time.Time
	Success     bool
}

// Execute runs the operation with retry logic
func (op *RetryOperation) Execute(ctx context.Context, fn func() error) error {
	op.StartTime = time.Now()
	op.Attempts = 0
	op.Success = false

	err := Retry(ctx, op.Config, func() error {
		op.Attempts++
		op.LastAttempt = time.Now()
		err := fn()
		if err != nil {
			op.LastError = err
			return err
		}
		return nil
	})

	op.EndTime = time.Now()

	if err == nil {
		op.Success = true
		op.LastError = nil
	} else {
		op.LastError = err
	}

	return err
}

// GetStats returns statistics about the operation
func (op *RetryOperation) GetStats() map[string]any {
	duration := op.EndTime.Sub(op.StartTime)

	return map[string]any{
		"name":        op.Name,
		"attempts":    op.Attempts,
		"success":     op.Success,
		"lastError":   op.LastError,
		"lastAttempt": op.LastAttempt,
		"duration":    duration.String(),
		"startTime":   op.StartTime,
		"endTime":     op.EndTime,
	}
}

// ExponentialBackoff calculates exponential backoff delay
func ExponentialBackoff(attempt int, baseDelay time.Duration, maxDelay time.Duration) time.Duration {
	if attempt <= 0 {
		return baseDelay
	}

	delay := time.Duration(math.Pow(2, float64(attempt-1))) * baseDelay
	if delay > maxDelay {
		return maxDelay
	}
	return delay
}

// LinearBackoff calculates linear backoff delay
func LinearBackoff(attempt int, baseDelay time.Duration, maxDelay time.Duration) time.Duration {
	delay := time.Duration(attempt) * baseDelay
	if delay > maxDelay {
		return maxDelay
	}
	return delay
}

// FibonacciBackoff calculates Fibonacci backoff delay
func FibonacciBackoff(attempt int, baseDelay time.Duration, maxDelay time.Duration) time.Duration {
	if attempt <= 0 {
		return baseDelay
	}

	// Calculate Fibonacci number for attempt
	fib := fibonacci(attempt)
	delay := time.Duration(fib) * baseDelay

	if delay > maxDelay {
		return maxDelay
	}
	return delay
}

// fibonacci calculates the nth Fibonacci number
func fibonacci(n int) int {
	if n <= 1 {
		return n
	}
	a, b := 0, 1
	for i := 2; i <= n; i++ {
		a, b = b, a+b
	}
	return b
}

// RetryableError wraps an error to indicate it should be retried
type RetryableError struct {
	Err error
}

func (e RetryableError) Error() string {
	return fmt.Sprintf("retryable: %v", e.Err)
}

func (e RetryableError) Unwrap() error {
	return e.Err
}

// PermanentError wraps an error to indicate it should not be retried
type PermanentError struct {
	Err error
}

func (e PermanentError) Error() string {
	return fmt.Sprintf("permanent: %v", e.Err)
}

func (e PermanentError) Unwrap() error {
	return e.Err
}

// IsRetryable checks if an error should be retried
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Check for explicit retryable error
	if _, ok := err.(RetryableError); ok {
		return true
	}

	// Check for explicit permanent error
	if _, ok := err.(PermanentError); ok {
		return false
	}

	// Check for context errors (not retryable)
	if err == context.Canceled || err == context.DeadlineExceeded {
		return false
	}

	// Default to retryable for other errors
	return true
}
