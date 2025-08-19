package middleware

import (
	"net/http"
	"sync"
	"time"
)

// RateLimiter provides rate limiting functionality for HTTP endpoints
type RateLimiter struct {
	attempts map[string]*attemptInfo
	mu       sync.RWMutex
	maxAttempts int
	window      time.Duration
	blockDuration time.Duration
}

type attemptInfo struct {
	count     int
	firstTime time.Time
	blockedUntil time.Time
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(maxAttempts int, window time.Duration, blockDuration time.Duration) *RateLimiter {
	rl := &RateLimiter{
		attempts:      make(map[string]*attemptInfo),
		maxAttempts:   maxAttempts,
		window:        window,
		blockDuration: blockDuration,
	}
	
	// Start cleanup goroutine
	go rl.cleanup()
	
	return rl
}

// Middleware returns an HTTP middleware that enforces rate limiting
func (rl *RateLimiter) Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := rl.getClientIP(r)
		
		if !rl.Allow(ip) {
			http.Error(w, "Too many attempts. Please try again later.", http.StatusTooManyRequests)
			return
		}
		
		next(w, r)
	}
}

// Allow checks if a request from the given IP should be allowed
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	now := time.Now()
	info, exists := rl.attempts[ip]
	
	if !exists {
		// First attempt
		rl.attempts[ip] = &attemptInfo{
			count:     1,
			firstTime: now,
		}
		return true
	}
	
	// Check if blocked
	if !info.blockedUntil.IsZero() && now.Before(info.blockedUntil) {
		return false
	}
	
	// Check if window has expired
	if now.Sub(info.firstTime) > rl.window {
		// Reset window
		info.count = 1
		info.firstTime = now
		info.blockedUntil = time.Time{}
		return true
	}
	
	// Increment count
	info.count++
	
	// Check if limit exceeded
	if info.count > rl.maxAttempts {
		info.blockedUntil = now.Add(rl.blockDuration)
		return false
	}
	
	return true
}

// RecordFailure records a failed authentication attempt
func (rl *RateLimiter) RecordFailure(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	now := time.Now()
	info, exists := rl.attempts[ip]
	
	if !exists {
		rl.attempts[ip] = &attemptInfo{
			count:     1,
			firstTime: now,
		}
		return
	}
	
	// For failed attempts, we count more aggressively
	info.count += 2 // Failed attempts count double
}

// Reset clears the rate limit for a specific IP (e.g., after successful login)
func (rl *RateLimiter) Reset(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	delete(rl.attempts, ip)
}

// cleanup periodically removes old entries to prevent memory leaks
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		
		for ip, info := range rl.attempts {
			// Remove entries older than 1 hour
			if now.Sub(info.firstTime) > time.Hour {
				delete(rl.attempts, ip)
			}
		}
		
		rl.mu.Unlock()
	}
}

// getClientIP extracts the client IP from the request
func (rl *RateLimiter) getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (for proxies)
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		// Use the first IP in the chain
		if idx := len(forwarded) - 1; idx >= 0 {
			for i := idx; i >= 0; i-- {
				if forwarded[i] == ',' || forwarded[i] == ' ' {
					return forwarded[i+1:]
				}
			}
			return forwarded
		}
	}
	
	// Check X-Real-IP header
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return realIP
	}
	
	// Fall back to RemoteAddr
	return r.RemoteAddr
}

// GetStats returns current rate limiting statistics
func (rl *RateLimiter) GetStats() map[string]interface{} {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	
	blocked := 0
	total := len(rl.attempts)
	now := time.Now()
	
	for _, info := range rl.attempts {
		if !info.blockedUntil.IsZero() && now.Before(info.blockedUntil) {
			blocked++
		}
	}
	
	return map[string]interface{}{
		"total_tracked": total,
		"currently_blocked": blocked,
		"max_attempts": rl.maxAttempts,
		"window_seconds": rl.window.Seconds(),
		"block_duration_seconds": rl.blockDuration.Seconds(),
	}
}

// AuthRateLimiter is the global rate limiter for authentication endpoints
var AuthRateLimiter = NewRateLimiter(
	5,                    // max 5 attempts
	15 * time.Minute,     // within 15 minutes
	30 * time.Minute,     // block for 30 minutes
)

// SignupRateLimiter is a stricter rate limiter for signup to prevent spam
var SignupRateLimiter = NewRateLimiter(
	3,                    // max 3 signups
	time.Hour,            // within 1 hour
	time.Hour,            // block for 1 hour
)