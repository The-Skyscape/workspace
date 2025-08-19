package controllers

import (
	"log"
	"net/http"
	"workspace/middleware"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
)

// RateLimit is a factory function with the prefix and instance
func RateLimit() (string, *RateLimitController) {
	return "ratelimit", &RateLimitController{}
}

// RateLimitController wraps authentication endpoints with rate limiting
type RateLimitController struct {
	application.BaseController
}

// Setup registers rate-limited authentication endpoints
func (c *RateLimitController) Setup(app *application.App) {
	c.BaseController.Setup(app)
	
	auth := app.Use("auth").(*authentication.Controller)
	
	// Wrap signin endpoint with rate limiting
	http.HandleFunc("POST /signin", func(w http.ResponseWriter, r *http.Request) {
		ip := c.getClientIP(r)
		
		// Check rate limit
		if !middleware.AuthRateLimiter.Allow(ip) {
			log.Printf("RateLimit: Too many signin attempts from %s", ip)
			http.Error(w, "Too many login attempts. Please try again in 30 minutes.", http.StatusTooManyRequests)
			return
		}
		
		// Store original response writer to check status
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		
		// Call the original signin handler
		auth.HandleSignin(rw, r)
		
		// If signin failed (not 2xx or 3xx), record failure
		if rw.statusCode >= 400 {
			middleware.AuthRateLimiter.RecordFailure(ip)
			log.Printf("RateLimit: Failed signin attempt from %s", ip)
		} else {
			// Successful login - reset rate limit for this IP
			middleware.AuthRateLimiter.Reset(ip)
			log.Printf("RateLimit: Successful signin from %s - rate limit reset", ip)
		}
	})
	
	// Wrap signup endpoint with stricter rate limiting
	http.HandleFunc("POST /signup", func(w http.ResponseWriter, r *http.Request) {
		ip := c.getClientIP(r)
		
		// Check rate limit for signups
		if !middleware.SignupRateLimiter.Allow(ip) {
			log.Printf("RateLimit: Too many signup attempts from %s", ip)
			http.Error(w, "Too many signup attempts. Please try again in 1 hour.", http.StatusTooManyRequests)
			return
		}
		
		// Call the original signup handler
		auth.HandleSignup(w, r)
		
		log.Printf("RateLimit: Signup attempt from %s", ip)
	})
	
	log.Println("RateLimit: Authentication rate limiting enabled")
	log.Println("RateLimit: Max 5 signin attempts per 15 minutes, 30 minute block")
	log.Println("RateLimit: Max 3 signup attempts per hour")
}

// Handle prepares controller for request
func (c RateLimitController) Handle(req *http.Request) application.Controller {
	c.Request = req
	return &c
}

// GetRateLimitStats returns current rate limiting statistics
func (c *RateLimitController) GetRateLimitStats() map[string]interface{} {
	authStats := middleware.AuthRateLimiter.GetStats()
	signupStats := middleware.SignupRateLimiter.GetStats()
	
	return map[string]interface{}{
		"auth": authStats,
		"signup": signupStats,
	}
}

// getClientIP extracts the client IP from the request
func (c *RateLimitController) getClientIP(r *http.Request) string {
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

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}