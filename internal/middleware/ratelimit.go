package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

// RateLimiter provides request rate limiting per IP address
type RateLimiter struct {
	requests map[string][]time.Time
	mu       sync.RWMutex
	rate     int           // requests per window
	window   time.Duration // time window
	cleanup  time.Duration // cleanup interval
	stopCh   chan struct{}
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(rate int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		requests: make(map[string][]time.Time),
		rate:     rate,
		window:   window,
		cleanup:  window * 2,
		stopCh:   make(chan struct{}),
	}
	
	// Start cleanup goroutine
	go rl.cleanupLoop()
	
	return rl
}

// cleanupLoop removes old entries periodically
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.cleanup)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			rl.cleanupOldEntries()
		case <-rl.stopCh:
			return
		}
	}
}

// cleanupOldEntries removes old request records
func (rl *RateLimiter) cleanupOldEntries() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	cutoff := time.Now().Add(-rl.window)
	for ip, times := range rl.requests {
		// Remove old timestamps
		var valid []time.Time
		for _, t := range times {
			if t.After(cutoff) {
				valid = append(valid, t)
			}
		}
		
		if len(valid) == 0 {
			delete(rl.requests, ip)
		} else {
			rl.requests[ip] = valid
		}
	}
}

// Allow checks if a request from the given IP should be allowed
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	now := time.Now()
	cutoff := now.Add(-rl.window)
	
	// Get or create request history for this IP
	times, exists := rl.requests[ip]
	if !exists {
		rl.requests[ip] = []time.Time{now}
		return true
	}
	
	// Count recent requests
	var valid []time.Time
	for _, t := range times {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	
	// Check if under limit
	if len(valid) >= rl.rate {
		rl.requests[ip] = valid
		return false
	}
	
	// Add current request
	valid = append(valid, now)
	rl.requests[ip] = valid
	return true
}

// Middleware returns an HTTP middleware for rate limiting
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract client IP
		ip := getClientIP(r)
		
		// Check rate limit
		if !rl.Allow(ip) {
			http.Error(w, "Rate limit exceeded. Please try again later.", http.StatusTooManyRequests)
			return
		}
		
		next.ServeHTTP(w, r)
	})
}

// Stop stops the rate limiter cleanup goroutine
func (rl *RateLimiter) Stop() {
	close(rl.stopCh)
}

// getClientIP extracts the client IP from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (for proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP if multiple are present
		if comma := strings.Index(xff, ","); comma != -1 {
			return strings.TrimSpace(xff[:comma])
		}
		return xff
	}
	
	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	
	// Fall back to RemoteAddr
	if colon := strings.LastIndex(r.RemoteAddr, ":"); colon != -1 {
		return r.RemoteAddr[:colon]
	}
	
	return r.RemoteAddr
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	// API endpoints
	APIRate     int           // requests per minute for API calls
	APIWindow   time.Duration
	
	// AI endpoints  
	AIRate      int           // requests per minute for AI operations
	AIWindow    time.Duration
	
	// Auth endpoints
	AuthRate    int           // requests per minute for auth operations
	AuthWindow  time.Duration
	
	// Search endpoints
	SearchRate  int           // requests per minute for search
	SearchWindow time.Duration
	
	// General endpoints
	GeneralRate int           // requests per minute for general pages
	GeneralWindow time.Duration
}

// DefaultRateLimitConfig returns sensible defaults for production
func DefaultRateLimitConfig() *RateLimitConfig {
	return &RateLimitConfig{
		// API: 60 requests per minute
		APIRate:   60,
		APIWindow: time.Minute,
		
		// AI: 10 requests per minute (expensive operations)
		AIRate:    10,
		AIWindow:  time.Minute,
		
		// Auth: 5 requests per minute (prevent brute force)
		AuthRate:   5,
		AuthWindow: time.Minute,
		
		// Search: 30 requests per minute
		SearchRate:   30,
		SearchWindow: time.Minute,
		
		// General: 120 requests per minute
		GeneralRate:   120,
		GeneralWindow: time.Minute,
	}
}

// CreateRateLimiters creates rate limiters from config
func CreateRateLimiters(config *RateLimitConfig) map[string]*RateLimiter {
	return map[string]*RateLimiter{
		"api":     NewRateLimiter(config.APIRate, config.APIWindow),
		"ai":      NewRateLimiter(config.AIRate, config.AIWindow),
		"auth":    NewRateLimiter(config.AuthRate, config.AuthWindow),
		"search":  NewRateLimiter(config.SearchRate, config.SearchWindow),
		"general": NewRateLimiter(config.GeneralRate, config.GeneralWindow),
	}
}