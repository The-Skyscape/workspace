package claude

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// SandboxManager handles Claude sandbox operations
type SandboxManager struct {
	sandboxService SandboxServiceInterface
	authManager    *AuthManager
}

// SandboxServiceInterface defines the interface for sandbox operations
type SandboxServiceInterface interface {
	NewSandbox(name, repoPath, repoName, command string, timeoutSecs int) (SandboxInterface, error)
	GetSandbox(name string) (SandboxInterface, error)
}

// SandboxInterface defines the interface for a sandbox instance
type SandboxInterface interface {
	Start() error
	Stop() error
	IsRunning() bool
	Execute(command string) (string, int, error)
	GetOutput() (string, error)
	GetLogs(tail int) (string, error)
	WaitForCompletion() error
	Cleanup() error
	GetStatus() map[string]interface{}
}

// NewSandboxManager creates a new sandbox manager
func NewSandboxManager(sandboxService SandboxServiceInterface, authManager *AuthManager) *SandboxManager {
	return &SandboxManager{
		sandboxService: sandboxService,
		authManager:    authManager,
	}
}

// CreateClaudeSandbox creates a new sandbox with Claude CLI installed
func (sm *SandboxManager) CreateClaudeSandbox(sessionID string, repoPath string, repoName string) (SandboxInterface, error) {
	if !sm.authManager.IsConfigured() {
		return nil, errors.New("Claude API key not configured")
	}

	apiKey := sm.authManager.GetAPIKey()
	if apiKey == "" {
		return nil, errors.New("Claude API key is empty")
	}

	// Create initialization script that installs Claude CLI and sets up environment
	initScript := fmt.Sprintf(`#!/bin/bash
set -e

# Export Claude API key
export ANTHROPIC_API_KEY="%s"

# Install Claude CLI if not present
if ! command -v claude &> /dev/null; then
    echo "Installing Claude CLI..."
    # Download and install Claude CLI
    curl -fsSL https://claude.ai/cli/install.sh | sh
fi

# Verify installation
claude --version

# Change to repository directory
cd /workspace/repo

echo "Claude sandbox ready for repository: %s"
echo "You can now use 'claude' command to interact with the AI"
`, apiKey, repoName)

	// Create sandbox with initialization
	sandboxName := fmt.Sprintf("claude-%s-%d", sessionID, time.Now().Unix())
	sandbox, err := sm.sandboxService.NewSandbox(
		sandboxName,
		repoPath,
		repoName,
		initScript,
		300, // 5 minute timeout
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Claude sandbox")
	}

	// Start the sandbox
	if err := sandbox.Start(); err != nil {
		sandbox.Cleanup()
		return nil, errors.Wrap(err, "failed to start Claude sandbox")
	}

	log.Printf("Claude sandbox created: %s", sandboxName)
	return sandbox, nil
}

// ExecuteClaudeCommand executes a Claude command in the sandbox
func (sm *SandboxManager) ExecuteClaudeCommand(sandbox SandboxInterface, prompt string) (string, error) {
	if !sandbox.IsRunning() {
		return "", errors.New("sandbox is not running")
	}

	// Escape the prompt for shell execution
	escapedPrompt := strings.ReplaceAll(prompt, `"`, `\"`)
	escapedPrompt = strings.ReplaceAll(escapedPrompt, `$`, `\$`)
	escapedPrompt = strings.ReplaceAll(escapedPrompt, "`", "\\`")

	// Build Claude command
	command := fmt.Sprintf(`claude "%s"`, escapedPrompt)

	output, exitCode, err := sandbox.Execute(command)
	if err != nil {
		return output, errors.Wrapf(err, "Claude command failed with exit code %d", exitCode)
	}

	return output, nil
}

// AnalyzeRepository uses Claude to analyze a repository
func (sm *SandboxManager) AnalyzeRepository(sandbox SandboxInterface, analysisType string) (string, error) {
	prompts := map[string]string{
		"security": "Analyze this repository for security vulnerabilities and create a list of issues in JSON format with title and description fields.",
		"quality":  "Review this codebase for code quality issues and suggest improvements in JSON format with title and description fields.",
		"documentation": "Identify missing or outdated documentation in this repository and suggest improvements in JSON format with title and description fields.",
		"performance": "Analyze this code for performance bottlenecks and optimization opportunities in JSON format with title and description fields.",
		"dependencies": "Review the dependencies in this project for outdated packages or security issues in JSON format with title and description fields.",
	}

	prompt, exists := prompts[analysisType]
	if !exists {
		prompt = analysisType // Use custom prompt if not a predefined type
	}

	// Add context about the repository structure
	fullPrompt := fmt.Sprintf(`Please analyze the repository in the current directory. %s

First, examine the repository structure by listing files:
ls -la

Then provide your analysis.`, prompt)

	return sm.ExecuteClaudeCommand(sandbox, fullPrompt)
}

// StreamClaudeCommand executes a Claude command and streams the output
func (sm *SandboxManager) StreamClaudeCommand(sandbox SandboxInterface, prompt string, outputChan chan<- string) error {
	if !sandbox.IsRunning() {
		return errors.New("sandbox is not running")
	}

	// Start command execution in background
	go func() {
		defer close(outputChan)

		// Send initial message
		outputChan <- "Executing Claude command...\n"

		output, err := sm.ExecuteClaudeCommand(sandbox, prompt)
		if err != nil {
			outputChan <- fmt.Sprintf("Error: %v\n", err)
			return
		}

		// Stream output in chunks
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			outputChan <- line + "\n"
			time.Sleep(10 * time.Millisecond) // Small delay for streaming effect
		}
	}()

	return nil
}

// CreateIssuesFromAnalysis parses Claude's analysis and creates issues
func (sm *SandboxManager) CreateIssuesFromAnalysis(analysis string) ([]map[string]string, error) {
	// Look for JSON array in the analysis
	startIdx := strings.Index(analysis, "[")
	endIdx := strings.LastIndex(analysis, "]")
	
	if startIdx == -1 || endIdx == -1 || startIdx >= endIdx {
		// Try to extract issues from plain text
		return sm.extractIssuesFromText(analysis), nil
	}

	// Extract JSON portion
	// jsonStr := analysis[startIdx : endIdx+1]
	
	// Parse JSON (simplified - in production use proper JSON parsing)
	// var issues []map[string]string
	// TODO: Implement proper JSON parsing
	// For now, return a simple extraction
	return sm.extractIssuesFromText(analysis), nil
}

// extractIssuesFromText extracts issues from plain text analysis
func (sm *SandboxManager) extractIssuesFromText(text string) []map[string]string {
	var issues []map[string]string
	
	lines := strings.Split(text, "\n")
	var currentIssue map[string]string
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		
		// Look for issue indicators
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "• ") {
			if currentIssue != nil && currentIssue["title"] != "" {
				issues = append(issues, currentIssue)
			}
			currentIssue = map[string]string{
				"title":       strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(line, "- "), "* "), "• "),
				"description": "",
			}
		} else if currentIssue != nil && line != "" {
			if currentIssue["description"] != "" {
				currentIssue["description"] += "\n"
			}
			currentIssue["description"] += line
		}
	}
	
	// Add last issue if exists
	if currentIssue != nil && currentIssue["title"] != "" {
		issues = append(issues, currentIssue)
	}
	
	return issues
}