package middleware

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

// LogLevel represents the severity of a log message
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
	FATAL
)

func (l LogLevel) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	case FATAL:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// LogEntry represents a structured log entry
type LogEntry struct {
	Timestamp  time.Time      `json:"timestamp"`
	Level      string         `json:"level"`
	Message    string         `json:"message"`
	Method     string         `json:"method,omitempty"`
	Path       string         `json:"path,omitempty"`
	StatusCode int            `json:"status_code,omitempty"`
	Duration   float64        `json:"duration_ms,omitempty"`
	IP         string         `json:"ip,omitempty"`
	UserAgent  string         `json:"user_agent,omitempty"`
	UserID     int            `json:"user_id,omitempty"`
	RequestID  string         `json:"request_id,omitempty"`
	Error      string         `json:"error,omitempty"`
	StackTrace string         `json:"stack_trace,omitempty"`
	Extra      map[string]any `json:"extra,omitempty"`
}

// Logger provides structured logging capabilities
type Logger struct {
	level      LogLevel
	output     *log.Logger
	buffer     []LogEntry
	bufferLock sync.RWMutex
	maxBuffer  int
}

// NewLogger creates a new logger instance
func NewLogger(level LogLevel) *Logger {
	return &Logger{
		level:     level,
		output:    log.New(os.Stdout, "", 0),
		buffer:    make([]LogEntry, 0, 1000),
		maxBuffer: 1000,
	}
}

// Log writes a log entry at the specified level
func (l *Logger) Log(level LogLevel, message string, extra map[string]any) {
	if level < l.level {
		return
	}

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level.String(),
		Message:   message,
		Extra:     extra,
	}

	// Add stack trace for errors
	if level >= ERROR {
		entry.StackTrace = l.getStackTrace()
	}

	// Store in buffer
	l.bufferLock.Lock()
	l.buffer = append(l.buffer, entry)
	if len(l.buffer) > l.maxBuffer {
		l.buffer = l.buffer[len(l.buffer)-l.maxBuffer:]
	}
	l.bufferLock.Unlock()

	// Output based on environment
	if os.Getenv("LOG_FORMAT") == "json" {
		jsonBytes, _ := json.Marshal(entry)
		l.output.Println(string(jsonBytes))
	} else {
		// Human-readable format for development
		color := l.getColor(level)
		reset := "\033[0m"
		if os.Getenv("NO_COLOR") != "" {
			color = ""
			reset = ""
		}

		timestamp := entry.Timestamp.Format("15:04:05")
		l.output.Printf("%s%s [%s]%s %s", color, timestamp, entry.Level, reset, entry.Message)

		if entry.Error != "" {
			l.output.Printf("  Error: %s", entry.Error)
		}
		if entry.StackTrace != "" && os.Getenv("DEBUG") == "true" {
			l.output.Printf("  Stack:\n%s", entry.StackTrace)
		}
	}
}

// Helper methods for different log levels
func (l *Logger) Debug(message string, extra ...map[string]any) {
	e := l.mergeExtra(extra...)
	l.Log(DEBUG, message, e)
}

func (l *Logger) Info(message string, extra ...map[string]any) {
	e := l.mergeExtra(extra...)
	l.Log(INFO, message, e)
}

func (l *Logger) Warn(message string, extra ...map[string]any) {
	e := l.mergeExtra(extra...)
	l.Log(WARN, message, e)
}

func (l *Logger) Error(message string, err error, extra ...map[string]any) {
	e := l.mergeExtra(extra...)
	if err != nil {
		e["error"] = err.Error()
	}
	l.Log(ERROR, message, e)
}

func (l *Logger) Fatal(message string, err error, extra ...map[string]any) {
	e := l.mergeExtra(extra...)
	if err != nil {
		e["error"] = err.Error()
	}
	l.Log(FATAL, message, e)
	os.Exit(1)
}

// LogRequest logs HTTP request details
func (l *Logger) LogRequest(r *http.Request, statusCode int, duration time.Duration, userID int) {
	entry := LogEntry{
		Timestamp:  time.Now(),
		Level:      INFO.String(),
		Message:    fmt.Sprintf("%s %s", r.Method, r.URL.Path),
		Method:     r.Method,
		Path:       r.URL.Path,
		StatusCode: statusCode,
		Duration:   duration.Seconds() * 1000, // Convert to milliseconds
		IP:         l.getClientIP(r),
		UserAgent:  r.UserAgent(),
		RequestID:  r.Header.Get("X-Request-ID"),
	}

	if userID > 0 {
		entry.UserID = userID
	}

	// Determine log level based on status code
	level := INFO
	if statusCode >= 500 {
		level = ERROR
	} else if statusCode >= 400 {
		level = WARN
	}

	// Store in buffer
	l.bufferLock.Lock()
	l.buffer = append(l.buffer, entry)
	if len(l.buffer) > l.maxBuffer {
		l.buffer = l.buffer[len(l.buffer)-l.maxBuffer:]
	}
	l.bufferLock.Unlock()

	// Output
	if os.Getenv("LOG_FORMAT") == "json" {
		jsonBytes, _ := json.Marshal(entry)
		l.output.Println(string(jsonBytes))
	} else {
		color := l.getColor(level)
		reset := "\033[0m"
		if os.Getenv("NO_COLOR") != "" {
			color = ""
			reset = ""
		}

		timestamp := entry.Timestamp.Format("15:04:05")
		l.output.Printf("%s%s [HTTP]%s %d %s %s (%.2fms) %s",
			color, timestamp, reset, statusCode, r.Method, r.URL.Path, entry.Duration, entry.IP)
	}
}

// GetRecentLogs returns recent log entries from the buffer
func (l *Logger) GetRecentLogs(limit int) []LogEntry {
	l.bufferLock.RLock()
	defer l.bufferLock.RUnlock()

	if limit <= 0 || limit > len(l.buffer) {
		limit = len(l.buffer)
	}

	start := len(l.buffer) - limit
	if start < 0 {
		start = 0
	}

	result := make([]LogEntry, limit)
	copy(result, l.buffer[start:])
	return result
}

// GetLogStats returns statistics about recent logs
func (l *Logger) GetLogStats() map[string]any {
	l.bufferLock.RLock()
	defer l.bufferLock.RUnlock()

	stats := map[string]int{
		"total": len(l.buffer),
		"debug": 0,
		"info":  0,
		"warn":  0,
		"error": 0,
		"fatal": 0,
	}

	for _, entry := range l.buffer {
		switch entry.Level {
		case "DEBUG":
			stats["debug"]++
		case "INFO":
			stats["info"]++
		case "WARN":
			stats["warn"]++
		case "ERROR":
			stats["error"]++
		case "FATAL":
			stats["fatal"]++
		}
	}

	return map[string]any{
		"counts":      stats,
		"buffer_size": l.maxBuffer,
	}
}

// HTTPLoggingMiddleware creates middleware for logging HTTP requests
func (l *Logger) HTTPLoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip logging for static assets and health checks
		if strings.HasPrefix(r.URL.Path, "/public/") ||
			strings.HasPrefix(r.URL.Path, "/health") ||
			strings.HasPrefix(r.URL.Path, "/favicon.ico") {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()

		// Wrap response writer to capture status code
		lrw := &loggingResponseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		// Add request ID if not present
		if r.Header.Get("X-Request-ID") == "" {
			r.Header.Set("X-Request-ID", l.generateRequestID())
		}

		// Serve the request
		next.ServeHTTP(lrw, r)

		// Log the request
		duration := time.Since(start)
		l.LogRequest(r, lrw.statusCode, duration, 0) // UserID would be extracted from context
	})
}

// Helper functions

func (l *Logger) getStackTrace() string {
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	return string(buf[:n])
}

func (l *Logger) getColor(level LogLevel) string {
	switch level {
	case DEBUG:
		return "\033[36m" // Cyan
	case INFO:
		return "\033[32m" // Green
	case WARN:
		return "\033[33m" // Yellow
	case ERROR:
		return "\033[31m" // Red
	case FATAL:
		return "\033[35m" // Magenta
	default:
		return ""
	}
}

func (l *Logger) getClientIP(r *http.Request) string {
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		parts := strings.Split(forwarded, ",")
		return strings.TrimSpace(parts[0])
	}

	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return realIP
	}

	return strings.Split(r.RemoteAddr, ":")[0]
}

func (l *Logger) generateRequestID() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), runtime.NumGoroutine())
}

func (l *Logger) mergeExtra(extras ...map[string]any) map[string]any {
	result := make(map[string]any)
	for _, extra := range extras {
		for k, v := range extra {
			result[k] = v
		}
	}
	return result
}

// loggingResponseWriter wraps http.ResponseWriter to capture status code
type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	if !lrw.written {
		lrw.statusCode = code
		lrw.written = true
		lrw.ResponseWriter.WriteHeader(code)
	}
}

func (lrw *loggingResponseWriter) Write(b []byte) (int, error) {
	if !lrw.written {
		lrw.written = true
	}
	return lrw.ResponseWriter.Write(b)
}

// Global logger instance
var AppLogger = NewLogger(INFO)

// Initialize logger based on environment
func init() {
	// Set log level based on environment
	if os.Getenv("DEBUG") == "true" {
		AppLogger.level = DEBUG
	} else if os.Getenv("LOG_LEVEL") != "" {
		switch strings.ToUpper(os.Getenv("LOG_LEVEL")) {
		case "DEBUG":
			AppLogger.level = DEBUG
		case "INFO":
			AppLogger.level = INFO
		case "WARN":
			AppLogger.level = WARN
		case "ERROR":
			AppLogger.level = ERROR
		case "FATAL":
			AppLogger.level = FATAL
		}
	}

	AppLogger.Info("Logger initialized", map[string]any{
		"level":  AppLogger.level.String(),
		"format": os.Getenv("LOG_FORMAT"),
	})
}
