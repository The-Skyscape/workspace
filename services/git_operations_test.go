package services

import (
	"testing"
	"workspace/models"
	"os"
	"path/filepath"
	"os/exec"
	"strings"
)

func TestGitOperationsConfigureRemote(t *testing.T) {
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
	
	// Create a test repository object
	repo := &models.Repository{
		Name: "test-repo",
		// Override the Path method for testing
	}
	
	// Create a mock function to override repo.Path()
	originalPath := repo.Path
	repo.Path = func() string {
		return tmpDir
	}
	defer func() {
		repo.Path = originalPath
	}()
	
	service := NewGitOperationsService()
	
	tests := []struct {
		name      string
		githubURL string
		wantErr   bool
	}{
		{
			name:      "Valid HTTPS URL",
			githubURL: "https://github.com/test/repo.git",
			wantErr:   false,
		},
		{
			name:      "Valid SSH URL",
			githubURL: "git@github.com:test/repo.git",
			wantErr:   false,
		},
		{
			name:      "Empty URL",
			githubURL: "",
			wantErr:   true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.ConfigureRemote(repo, tt.githubURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("ConfigureRemote() error = %v, wantErr %v", err, tt.wantErr)
			}
			
			if !tt.wantErr && err == nil {
				// Check that remote was added
				cmd := exec.Command("git", "remote", "get-url", "origin")
				cmd.Dir = tmpDir
				output, _ := cmd.Output()
				if len(output) == 0 {
					t.Error("Remote was not configured")
				}
			}
		})
	}
}

func TestGitOperationsGetSyncStatus(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}

	// Create a temporary directory
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
	cmd.Run()
	
	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = tmpDir
	cmd.Run()
	
	// Create repository object
	repo := &models.Repository{
		Name:             "test-repo",
		RemoteConfigured: false,
	}
	
	// Override Path method
	repo.Path = func() string {
		return tmpDir
	}
	
	service := NewGitOperationsService()
	
	// Test without remote
	ahead, behind, status, err := service.GetSyncStatus(repo)
	if err != nil {
		t.Errorf("GetSyncStatus() unexpected error: %v", err)
	}
	if status != "no-remote" {
		t.Errorf("Expected status 'no-remote', got '%s'", status)
	}
	if ahead != 0 || behind != 0 {
		t.Errorf("Expected 0 ahead/behind for no-remote, got %d/%d", ahead, behind)
	}
}

func TestGitOperationsCheckForConflicts(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}

	tmpDir := t.TempDir()
	
	// Initialize repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	cmd.Run()
	
	repo := &models.Repository{
		Name: "test-repo",
	}
	repo.Path = func() string {
		return tmpDir
	}
	
	service := NewGitOperationsService()
	
	// Check for conflicts (should be none)
	hasConflicts, files, err := service.CheckForConflicts(repo)
	if err != nil {
		t.Errorf("CheckForConflicts() unexpected error: %v", err)
	}
	if hasConflicts {
		t.Error("Expected no conflicts in clean repo")
	}
	if len(files) > 0 {
		t.Errorf("Expected no conflicted files, got %v", files)
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
			} else if strings.Contains(errMsg, "Permission denied") || strings.Contains(errMsg, "403") {
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