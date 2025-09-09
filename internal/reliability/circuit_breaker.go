package reliability

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// State represents the circuit breaker state
type State int

const (
	StateClosed State = iota
	StateOpen
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreaker implements the circuit breaker pattern for AI operations
type CircuitBreaker struct {
	name            string
	maxFailures     int
	resetTimeout    time.Duration
	halfOpenMax     int
	onStateChange   func(from, to State)
	
	mu              sync.RWMutex
	state           State
	failures        int
	lastFailureTime time.Time
	successCount    int
	totalRequests   int64
	totalFailures   int64
	totalSuccesses  int64
}

// Config for circuit breaker
type Config struct {
	Name          string
	MaxFailures   int
	ResetTimeout  time.Duration
	HalfOpenMax   int
	OnStateChange func(from, to State)
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(config Config) *CircuitBreaker {
	if config.MaxFailures == 0 {
		config.MaxFailures = 5
	}
	if config.ResetTimeout == 0 {
		config.ResetTimeout = 60 * time.Second
	}
	if config.HalfOpenMax == 0 {
		config.HalfOpenMax = 3
	}
	
	return &CircuitBreaker{
		name:          config.Name,
		maxFailures:   config.MaxFailures,
		resetTimeout:  config.ResetTimeout,
		halfOpenMax:   config.HalfOpenMax,
		onStateChange: config.OnStateChange,
		state:         StateClosed,
	}
}

// Execute runs a function with circuit breaker protection
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func() error) error {
	if err := cb.canExecute(); err != nil {
		return err
	}
	
	// Execute the function
	err := fn()
	
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	cb.totalRequests++
	
	if err != nil {
		cb.onFailure()
		cb.totalFailures++
		return fmt.Errorf("circuit breaker %s: %w", cb.name, err)
	}
	
	cb.onSuccess()
	cb.totalSuccesses++
	return nil
}

// ExecuteWithFallback runs a function with circuit breaker and fallback
func (cb *CircuitBreaker) ExecuteWithFallback(ctx context.Context, fn func() error, fallback func() error) error {
	if err := cb.Execute(ctx, fn); err != nil {
		log.Printf("CircuitBreaker %s: Primary function failed, using fallback: %v", cb.name, err)
		if fallback != nil {
			return fallback()
		}
		return err
	}
	return nil
}

// canExecute checks if the circuit breaker allows execution
func (cb *CircuitBreaker) canExecute() error {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	
	switch cb.state {
	case StateClosed:
		return nil
		
	case StateOpen:
		// Check if we should transition to half-open
		if time.Since(cb.lastFailureTime) > cb.resetTimeout {
			cb.mu.RUnlock()
			cb.mu.Lock()
			if cb.state == StateOpen { // Double-check with write lock
				cb.setState(StateHalfOpen)
				cb.successCount = 0
			}
			cb.mu.Unlock()
			cb.mu.RLock()
			return nil
		}
		return fmt.Errorf("circuit breaker %s is open", cb.name)
		
	case StateHalfOpen:
		return nil
		
	default:
		return fmt.Errorf("circuit breaker %s in unknown state", cb.name)
	}
}

// onSuccess handles successful execution
func (cb *CircuitBreaker) onSuccess() {
	switch cb.state {
	case StateClosed:
		cb.failures = 0
		
	case StateHalfOpen:
		cb.successCount++
		if cb.successCount >= cb.halfOpenMax {
			cb.setState(StateClosed)
			cb.failures = 0
			cb.successCount = 0
		}
		
	case StateOpen:
		// Shouldn't happen, but reset if it does
		cb.setState(StateHalfOpen)
		cb.successCount = 1
	}
}

// onFailure handles failed execution
func (cb *CircuitBreaker) onFailure() {
	cb.failures++
	cb.lastFailureTime = time.Now()
	
	switch cb.state {
	case StateClosed:
		if cb.failures >= cb.maxFailures {
			cb.setState(StateOpen)
		}
		
	case StateHalfOpen:
		cb.setState(StateOpen)
		
	case StateOpen:
		// Already open, just update failure time
	}
}

// setState changes the circuit breaker state
func (cb *CircuitBreaker) setState(newState State) {
	if cb.state == newState {
		return
	}
	
	oldState := cb.state
	cb.state = newState
	
	log.Printf("CircuitBreaker %s: State changed from %s to %s", cb.name, oldState, newState)
	
	if cb.onStateChange != nil {
		cb.onStateChange(oldState, newState)
	}
}

// GetState returns the current state
func (cb *CircuitBreaker) GetState() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// GetStats returns circuit breaker statistics
func (cb *CircuitBreaker) GetStats() map[string]interface{} {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	
	successRate := float64(0)
	if cb.totalRequests > 0 {
		successRate = float64(cb.totalSuccesses) / float64(cb.totalRequests) * 100
	}
	
	return map[string]interface{}{
		"name":            cb.name,
		"state":           cb.state.String(),
		"failures":        cb.failures,
		"totalRequests":   cb.totalRequests,
		"totalSuccesses":  cb.totalSuccesses,
		"totalFailures":   cb.totalFailures,
		"successRate":     fmt.Sprintf("%.2f%%", successRate),
		"lastFailureTime": cb.lastFailureTime,
	}
}

// Reset manually resets the circuit breaker
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	cb.setState(StateClosed)
	cb.failures = 0
	cb.successCount = 0
}

// CircuitBreakerManager manages multiple circuit breakers
type CircuitBreakerManager struct {
	breakers map[string]*CircuitBreaker
	mu       sync.RWMutex
}

// NewCircuitBreakerManager creates a new manager
func NewCircuitBreakerManager() *CircuitBreakerManager {
	return &CircuitBreakerManager{
		breakers: make(map[string]*CircuitBreaker),
	}
}

// GetOrCreate gets an existing circuit breaker or creates a new one
func (m *CircuitBreakerManager) GetOrCreate(name string, config Config) *CircuitBreaker {
	m.mu.RLock()
	if cb, exists := m.breakers[name]; exists {
		m.mu.RUnlock()
		return cb
	}
	m.mu.RUnlock()
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Double-check after acquiring write lock
	if cb, exists := m.breakers[name]; exists {
		return cb
	}
	
	config.Name = name
	cb := NewCircuitBreaker(config)
	m.breakers[name] = cb
	return cb
}

// Get returns a circuit breaker by name
func (m *CircuitBreakerManager) Get(name string) (*CircuitBreaker, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cb, exists := m.breakers[name]
	return cb, exists
}

// GetAll returns all circuit breakers
func (m *CircuitBreakerManager) GetAll() map[string]*CircuitBreaker {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	result := make(map[string]*CircuitBreaker)
	for k, v := range m.breakers {
		result[k] = v
	}
	return result
}

// GetStats returns statistics for all circuit breakers
func (m *CircuitBreakerManager) GetStats() []map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	stats := make([]map[string]interface{}, 0, len(m.breakers))
	for _, cb := range m.breakers {
		stats = append(stats, cb.GetStats())
	}
	return stats
}

// ResetAll resets all circuit breakers
func (m *CircuitBreakerManager) ResetAll() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	for _, cb := range m.breakers {
		cb.Reset()
	}
}

// Global circuit breaker manager
var Manager = NewCircuitBreakerManager()