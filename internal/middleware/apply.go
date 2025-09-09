package middleware

import (
	"fmt"
	"net/http"
	"strings"
)

// RouteRateLimiter applies different rate limits based on route patterns
type RouteRateLimiter struct {
	limiters map[string]*RateLimiter
}

// NewRouteRateLimiter creates a new route-based rate limiter
func NewRouteRateLimiter(limiters map[string]*RateLimiter) *RouteRateLimiter {
	return &RouteRateLimiter{
		limiters: limiters,
	}
}

// Middleware returns the rate limiting middleware
func (rrl *RouteRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Determine which rate limiter to use based on path
		limiter := rrl.getLimiterForPath(r.URL.Path)
		
		// Apply rate limiting
		if limiter != nil {
			ip := getClientIP(r)
			if !limiter.Allow(ip) {
				// Set rate limit headers
				w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", limiter.rate))
				w.Header().Set("X-RateLimit-Window", limiter.window.String())
				w.Header().Set("Retry-After", fmt.Sprintf("%.0f", limiter.window.Seconds()))
				
				// Return 429 Too Many Requests
				http.Error(w, "Rate limit exceeded. Please try again later.", http.StatusTooManyRequests)
				return
			}
		}
		
		next.ServeHTTP(w, r)
	})
}

// getLimiterForPath determines which rate limiter to use for a given path
func (rrl *RouteRateLimiter) getLimiterForPath(path string) *RateLimiter {
	// AI endpoints - most restrictive
	if strings.HasPrefix(path, "/ai/") {
		return rrl.limiters["ai"]
	}
	
	// Auth endpoints - prevent brute force
	if strings.HasPrefix(path, "/_auth/") || 
	   strings.HasPrefix(path, "/signin") || 
	   strings.HasPrefix(path, "/signup") ||
	   strings.HasPrefix(path, "/reset") {
		return rrl.limiters["auth"]
	}
	
	// API endpoints
	if strings.HasPrefix(path, "/api/") ||
	   strings.Contains(path, "/webhook") {
		return rrl.limiters["api"]
	}
	
	// Search endpoints
	if strings.Contains(path, "/search") {
		return rrl.limiters["search"]
	}
	
	// Static assets - no rate limiting
	if strings.HasPrefix(path, "/static/") ||
	   strings.HasPrefix(path, "/assets/") ||
	   strings.HasSuffix(path, ".css") ||
	   strings.HasSuffix(path, ".js") ||
	   strings.HasSuffix(path, ".png") ||
	   strings.HasSuffix(path, ".jpg") ||
	   strings.HasSuffix(path, ".ico") {
		return nil
	}
	
	// General endpoints
	return rrl.limiters["general"]
}

// ApplyRateLimiting wraps an http.Handler with rate limiting
func ApplyRateLimiting(handler http.Handler, config *RateLimitConfig) http.Handler {
	// Create rate limiters
	limiters := CreateRateLimiters(config)
	
	// Create route rate limiter
	routeLimiter := NewRouteRateLimiter(limiters)
	
	// Apply middleware
	return routeLimiter.Middleware(handler)
}

// RateLimitedHandler wraps a handler function with rate limiting
func RateLimitedHandler(handler http.HandlerFunc, limiter *RateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := getClientIP(r)
		if !limiter.Allow(ip) {
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		handler(w, r)
	}
}