package claude

import (
	"fmt"
	"log"
	"strings"
	"time"
	"workspace/models"
)

// WorkerManager handles AI worker lifecycle operations
type WorkerManager struct {
	authManager    *AuthManager
	sandboxService SandboxServiceInterface
}

// WorkerConfig represents configuration for a new worker
type WorkerConfig struct {
	WorkerID   string
	RepoIDs    []string  // Repository IDs for access token creation
	RepoNames  []string  // Repository names for directory naming
	UserID     string    // User ID for access token creation
}

// NewWorkerManager creates a new worker manager
func NewWorkerManager(authManager *AuthManager, sandboxService SandboxServiceInterface) *WorkerManager {
	return &WorkerManager{
		authManager:    authManager,
		sandboxService: sandboxService,
	}
}

// InitializeWorker sets up a worker sandbox with repositories
func (wm *WorkerManager) InitializeWorker(config WorkerConfig) (SandboxInterface, error) {
	// Validate API key
	if !wm.authManager.IsConfigured() {
		return nil, fmt.Errorf("Claude API key not configured")
	}
	
	apiKey := wm.authManager.GetAPIKey()
	if apiKey == "" {
		return nil, fmt.Errorf("Claude API key is empty")
	}

	// Generate clone commands with access tokens
	cloneCommands, err := wm.generateCloneCommandsWithTokens(config.RepoIDs, config.RepoNames, config.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate clone commands: %w", err)
	}
	
	// Create initialization script
	initScript := fmt.Sprintf(`#!/bin/bash
set -e

# Export Claude API key
export ANTHROPIC_API_KEY="%s"

# Create workspace directory
mkdir -p /workspace/repos

# Clone repositories with access tokens
echo "Cloning repositories..."
%s

# Install Claude CLI if not present
if ! command -v claude &> /dev/null; then
    echo "Installing Claude CLI..."
    curl -fsSL https://claude.ai/cli/install.sh | sh || true
fi

# Verify installation
claude --version || echo "Claude CLI not available yet"

echo "Worker initialization complete"
`, apiKey, cloneCommands)

	// Create sandbox
	sandboxName := fmt.Sprintf("claude-worker-%s", config.WorkerID)
	sandbox, err := wm.sandboxService.NewSandbox(
		sandboxName,
		"", // No single repo path for multi-repo workers
		"multi-repo",
		initScript,
		0, // No timeout for persistent workers
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create sandbox: %w", err)
	}

	// Start sandbox
	if err := sandbox.Start(); err != nil {
		sandbox.Cleanup()
		return nil, fmt.Errorf("failed to start sandbox: %w", err)
	}

	log.Printf("Worker %s initialized with sandbox %s", config.WorkerID, sandboxName)
	return sandbox, nil
}

// generateCloneCommandsWithTokens creates git clone commands with access tokens
func (wm *WorkerManager) generateCloneCommandsWithTokens(repoIDs, repoNames []string, userID string) (string, error) {
	var commands []string
	
	for i, repoID := range repoIDs {
		if i < len(repoNames) {
			// Create access token for this repository
			token, err := models.CreateAccessToken(repoID, userID, 100*365*24*time.Hour)
			if err != nil {
				return "", fmt.Errorf("failed to create access token for repo %s: %w", repoID, err)
			}
			
			// Build clone URL with token (using host.docker.internal since this runs inside container)
			cloneURL := fmt.Sprintf("http://%s:%s@host.docker.internal/repo/%s", token.ID, token.Token, repoID)
			
			// Create clone command with proper directory naming
			cmd := fmt.Sprintf(`git clone '%s' /workspace/repos/%s`, cloneURL, repoNames[i])
			commands = append(commands, cmd)
		}
	}
	return strings.Join(commands, "\n"), nil
}

// ExecuteMessage sends a message to Claude in the worker sandbox
func (wm *WorkerManager) ExecuteMessage(sandbox SandboxInterface, message string) (string, error) {
	if !sandbox.IsRunning() {
		return "", fmt.Errorf("sandbox is not running")
	}

	// Use the existing ExecuteClaudeCommand from SandboxManager
	sm := NewSandboxManager(wm.sandboxService, wm.authManager)
	return sm.ExecuteClaudeCommand(sandbox, message)
}

// CleanupWorker stops and removes a worker sandbox
func (wm *WorkerManager) CleanupWorker(sandboxID string) error {
	sandbox, err := wm.sandboxService.GetSandbox(sandboxID)
	if err != nil {
		// Sandbox might already be gone
		log.Printf("Worker cleanup: sandbox %s not found: %v", sandboxID, err)
		return nil
	}
	
	if sandbox.IsRunning() {
		if err := sandbox.Stop(); err != nil {
			log.Printf("Worker cleanup: failed to stop sandbox %s: %v", sandboxID, err)
		}
	}
	
	if err := sandbox.Cleanup(); err != nil {
		log.Printf("Worker cleanup: failed to cleanup sandbox %s: %v", sandboxID, err)
	}
	
	return nil
}