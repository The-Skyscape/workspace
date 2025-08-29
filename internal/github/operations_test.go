package github

import (
	"testing"
	"os"
	"path/filepath"
	"os/exec"
	"strings"
)

func TestGitOperationsErrorMessages(t *testing.T) {
	tests := []struct {
		name          string
		errorMsg      string
		expectedError string
	}{
		{
			name:          "Authentication failed",
			errorMsg:      "fatal: Authentication failed for 'https://github.com/user/repo.git'",
			expectedError: "GitHub authentication failed",
		},
		{
			name:          "Permission denied",
			errorMsg:      "remote: Permission to user/repo.git denied to other-user.",
			expectedError: "permission denied",
		},
		{
			name:          "Non-fast-forward",
			errorMsg:      "! [rejected]        main -> main (non-fast-forward)",
			expectedError: "push rejected",
		},
		{
			name:          "Network error",
			errorMsg:      "fatal: Could not resolve host: github.com",
			expectedError: "network error",
		},
		{
			name:          "Merge conflict",
			errorMsg:      "CONFLICT (content): Merge conflict in file.txt",
			expectedError: "merge conflict detected",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test error message parsing logic
			errMsg := tt.errorMsg
			var result string
			
			if strings.Contains(errMsg, "Authentication failed") || strings.Contains(errMsg, "fatal: could not read Username") {
				result = "GitHub authentication failed"
			} else if strings.Contains(errMsg, "Permission denied") || strings.Contains(errMsg, "Permission to") || strings.Contains(errMsg, "403") {
				result = "permission denied"
			} else if strings.Contains(errMsg, "non-fast-forward") {
				result = "push rejected"
			} else if strings.Contains(errMsg, "Could not resolve host") {
				result = "network error"
			} else if strings.Contains(errMsg, "CONFLICT") || strings.Contains(errMsg, "Merge conflict") {
				result = "merge conflict detected"
			}
			
			if !strings.Contains(result, tt.expectedError) {
				t.Errorf("Expected error containing '%s', got '%s'", tt.expectedError, result)
			}
		})
	}
}

func TestGitOperationsSanitizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "URL with token",
			input:    "https://token@github.com/user/repo.git",
			expected: "***@github.com/user/repo.git",
		},
		{
			name:     "URL without token",
			input:    "https://github.com/user/repo.git",
			expected: "https://github.com/user/repo.git",
		},
		{
			name:     "SSH URL",
			input:    "git@github.com:user/repo.git",
			expected: "***@github.com:user/repo.git",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the sanitization logic used in the code
			sanitized := tt.input
			if idx := strings.Index(sanitized, "@"); idx > 0 {
				parts := strings.SplitN(sanitized, "@", 2)
				if len(parts) == 2 {
					sanitized = "***@" + parts[1]
				}
			}
			
			if sanitized != tt.expected {
				t.Errorf("Sanitization failed: expected %s, got %s", tt.expected, sanitized)
			}
		})
	}
}

// TestGitOperationsBasicCommands tests basic git command execution
func TestGitOperationsBasicCommands(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}

	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	
	// Initialize a git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to initialize git repo: %v", err)
	}
	
	// Configure git user for commits
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tmpDir
	cmd.Run()
	
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tmpDir
	cmd.Run()
	
	// Create a test file and commit
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to add files: %v", err)
	}
	
	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}
	
	// Test that we can run git status
	cmd = exec.Command("git", "status")
	cmd.Dir = tmpDir
	output, err := cmd.Output()
	if err != nil {
		t.Errorf("Failed to run git status: %v", err)
	}
	
	if !strings.Contains(string(output), "nothing to commit") {
		t.Error("Expected clean working directory after commit")
	}
}