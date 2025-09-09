package security

import (
	"fmt"
	"html"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// Sanitizer provides input sanitization and validation
type Sanitizer struct {
	// SQL injection patterns
	sqlPatterns []*regexp.Regexp
	// Command injection patterns
	cmdPatterns []*regexp.Regexp
	// Path traversal patterns
	pathPatterns []*regexp.Regexp
	// XSS patterns
	xssPatterns []*regexp.Regexp
}

// NewSanitizer creates a new input sanitizer
func NewSanitizer() *Sanitizer {
	return &Sanitizer{
		sqlPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)(union|select|insert|update|delete|drop|create|alter|exec|execute|script|javascript|eval)`),
			regexp.MustCompile(`(?i)(;|--|\*|\/\*|\*\/|xp_|sp_|0x)`),
			regexp.MustCompile(`(?i)(concat|char|ascii|substring|cast|convert|varchar)`),
			regexp.MustCompile(`['";\\].*?(or|and|union|select|insert|update|delete|drop)`),
		},
		cmdPatterns: []*regexp.Regexp{
			regexp.MustCompile(`[;&|<>$\x60]`),
			regexp.MustCompile(`\$\([^)]+\)`),
			regexp.MustCompile(`\$\{[^}]+\}`),
			regexp.MustCompile(`(?i)(bash|sh|cmd|powershell|python|perl|ruby|php)`),
		},
		pathPatterns: []*regexp.Regexp{
			regexp.MustCompile(`\.\.\/`),
			regexp.MustCompile(`\.\.\\`),
			regexp.MustCompile(`^\/`),
			regexp.MustCompile(`^~`),
			regexp.MustCompile(`(?i)(etc\/passwd|windows\/system32)`),
		},
		xssPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)<script[^>]*>.*?</script>`),
			regexp.MustCompile(`(?i)(javascript|vbscript):`),
			regexp.MustCompile(`(?i)on(click|load|error|mouseover|focus|blur|change|submit)=`),
			regexp.MustCompile(`(?i)<iframe[^>]*>`),
			regexp.MustCompile(`(?i)<object[^>]*>`),
			regexp.MustCompile(`(?i)<embed[^>]*>`),
		},
	}
}

// SanitizeString removes potentially dangerous characters from a string
func (s *Sanitizer) SanitizeString(input string, maxLength int) string {
	// Trim whitespace
	input = strings.TrimSpace(input)

	// Limit length
	if maxLength > 0 && len(input) > maxLength {
		input = input[:maxLength]
	}

	// Remove null bytes
	input = strings.ReplaceAll(input, "\x00", "")

	// Remove control characters except newlines and tabs
	var result strings.Builder
	for _, r := range input {
		if r == '\n' || r == '\t' || !unicode.IsControl(r) {
			result.WriteRune(r)
		}
	}

	return result.String()
}

// SanitizeHTML escapes HTML special characters
func (s *Sanitizer) SanitizeHTML(input string) string {
	return html.EscapeString(input)
}

// SanitizeSQL checks for SQL injection attempts
func (s *Sanitizer) SanitizeSQL(input string) (string, error) {
	// Check for SQL injection patterns
	for _, pattern := range s.sqlPatterns {
		if pattern.MatchString(input) {
			return "", fmt.Errorf("potential SQL injection detected")
		}
	}

	// Escape single quotes for SQL strings
	sanitized := strings.ReplaceAll(input, "'", "''")
	
	return sanitized, nil
}

// SanitizeCommand checks for command injection attempts
func (s *Sanitizer) SanitizeCommand(input string) (string, error) {
	// Check for command injection patterns
	for _, pattern := range s.cmdPatterns {
		if pattern.MatchString(input) {
			return "", fmt.Errorf("potential command injection detected")
		}
	}

	// Only allow alphanumeric, spaces, and basic punctuation
	var result strings.Builder
	for _, r := range input {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || 
		   r == ' ' || r == '.' || r == '-' || r == '_' || r == '/' {
			result.WriteRune(r)
		}
	}

	return result.String(), nil
}

// SanitizePath validates and cleans file paths
func (s *Sanitizer) SanitizePath(basePath, userPath string) (string, error) {
	// Check for path traversal patterns
	for _, pattern := range s.pathPatterns {
		if pattern.MatchString(userPath) {
			return "", fmt.Errorf("potential path traversal detected")
		}
	}

	// Clean the path
	cleaned := filepath.Clean(userPath)

	// Ensure it's within the base path
	fullPath := filepath.Join(basePath, cleaned)
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", err
	}

	absBase, err := filepath.Abs(basePath)
	if err != nil {
		return "", err
	}

	// Check if the path is within the base directory
	if !strings.HasPrefix(absPath, absBase) {
		return "", fmt.Errorf("path traversal attempt blocked")
	}

	return absPath, nil
}

// SanitizeURL validates and cleans URLs
func (s *Sanitizer) SanitizeURL(input string) (string, error) {
	// Parse the URL
	u, err := url.Parse(input)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	// Only allow http and https schemes
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("invalid URL scheme: %s", u.Scheme)
	}

	// Check for javascript: and data: URLs
	if strings.HasPrefix(strings.ToLower(input), "javascript:") ||
	   strings.HasPrefix(strings.ToLower(input), "data:") {
		return "", fmt.Errorf("potentially malicious URL scheme")
	}

	// Rebuild the URL to ensure it's clean
	clean := &url.URL{
		Scheme:   u.Scheme,
		Host:     u.Host,
		Path:     u.Path,
		RawQuery: u.RawQuery,
	}

	return clean.String(), nil
}

// SanitizeEmail validates email addresses
func (s *Sanitizer) SanitizeEmail(input string) (string, error) {
	// Basic email validation regex
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	
	input = strings.TrimSpace(strings.ToLower(input))
	
	if !emailRegex.MatchString(input) {
		return "", fmt.Errorf("invalid email address")
	}

	// Check for common injection attempts in email
	if strings.ContainsAny(input, "<>\"'`;") {
		return "", fmt.Errorf("potentially malicious email address")
	}

	return input, nil
}

// SanitizeFilename cleans filenames for safe storage
func (s *Sanitizer) SanitizeFilename(input string) string {
	// Remove path separators
	input = strings.ReplaceAll(input, "/", "")
	input = strings.ReplaceAll(input, "\\", "")
	
	// Remove special characters that could cause issues
	input = strings.ReplaceAll(input, "..", "")
	input = strings.ReplaceAll(input, "~", "")
	
	// Replace spaces with underscores
	input = strings.ReplaceAll(input, " ", "_")
	
	// Only allow alphanumeric, dash, underscore, and dot
	var result strings.Builder
	for _, r := range input {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || 
		   r == '-' || r == '_' || r == '.' {
			result.WriteRune(r)
		}
	}

	filename := result.String()
	
	// Prevent empty filename
	if filename == "" {
		filename = "unnamed"
	}

	// Limit length
	if len(filename) > 255 {
		// Preserve extension if possible
		ext := filepath.Ext(filename)
		base := filename[:255-len(ext)]
		filename = base + ext
	}

	return filename
}

// CheckXSS detects potential XSS attacks
func (s *Sanitizer) CheckXSS(input string) error {
	for _, pattern := range s.xssPatterns {
		if pattern.MatchString(input) {
			return fmt.Errorf("potential XSS attack detected")
		}
	}
	return nil
}

// ValidateInput performs comprehensive input validation
func (s *Sanitizer) ValidateInput(input string, inputType string) (string, error) {
	switch inputType {
	case "sql":
		return s.SanitizeSQL(input)
	case "command":
		return s.SanitizeCommand(input)
	case "html":
		return s.SanitizeHTML(input), nil
	case "url":
		return s.SanitizeURL(input)
	case "email":
		return s.SanitizeEmail(input)
	case "filename":
		return s.SanitizeFilename(input), nil
	default:
		// Generic sanitization
		sanitized := s.SanitizeString(input, 0)
		if err := s.CheckXSS(sanitized); err != nil {
			return "", err
		}
		return sanitized, nil
	}
}

// SanitizeMap sanitizes all values in a map
func (s *Sanitizer) SanitizeMap(input map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	
	for key, value := range input {
		// Sanitize the key
		sanitizedKey := s.SanitizeString(key, 100)
		
		// Sanitize the value based on type
		switch v := value.(type) {
		case string:
			result[sanitizedKey] = s.SanitizeString(v, 0)
		case []string:
			sanitized := make([]string, len(v))
			for i, str := range v {
				sanitized[i] = s.SanitizeString(str, 0)
			}
			result[sanitizedKey] = sanitized
		case map[string]interface{}:
			result[sanitizedKey] = s.SanitizeMap(v)
		default:
			// Keep other types as-is (numbers, booleans, etc.)
			result[sanitizedKey] = value
		}
	}
	
	return result
}

// ValidateLength checks if string length is within bounds
func (s *Sanitizer) ValidateLength(input string, min, max int) error {
	length := len(input)
	if length < min {
		return fmt.Errorf("input too short: minimum %d characters required", min)
	}
	if max > 0 && length > max {
		return fmt.Errorf("input too long: maximum %d characters allowed", max)
	}
	return nil
}

// ValidateAlphanumeric checks if string contains only alphanumeric characters
func (s *Sanitizer) ValidateAlphanumeric(input string) error {
	for _, r := range input {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return fmt.Errorf("input must contain only letters and numbers")
		}
	}
	return nil
}

// ValidateIdentifier validates identifiers (usernames, repo names, etc.)
func (s *Sanitizer) ValidateIdentifier(input string) error {
	// Check length
	if err := s.ValidateLength(input, 3, 50); err != nil {
		return err
	}

	// Must start with letter or number
	if len(input) > 0 {
		first := rune(input[0])
		if !unicode.IsLetter(first) && !unicode.IsDigit(first) {
			return fmt.Errorf("identifier must start with a letter or number")
		}
	}

	// Only allow alphanumeric, dash, and underscore
	validChars := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !validChars.MatchString(input) {
		return fmt.Errorf("identifier can only contain letters, numbers, dashes, and underscores")
	}

	// No consecutive special characters
	if strings.Contains(input, "--") || strings.Contains(input, "__") {
		return fmt.Errorf("identifier cannot contain consecutive special characters")
	}

	return nil
}

// Global sanitizer instance
var DefaultSanitizer = NewSanitizer()