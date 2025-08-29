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
	
	// Get user details for git config
	user, err := models.Auth.Users.Get(config.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user details: %w", err)
	}
	
	gitUserName := user.Name
	if gitUserName == "" {
		gitUserName = user.Email
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

# Configure git for push operations
git config --global user.name "%s"
git config --global user.email "%s"
git config --global push.default current

# Install Claude CLI if not present
if ! command -v claude &> /dev/null; then
    echo "Installing Claude CLI..."
    curl -fsSL https://claude.ai/cli/install.sh | sh || true
fi

# Verify installation
claude --version || echo "Claude CLI not available yet"

# Create a named pipe for Claude communication
mkfifo /tmp/claude_input 2>/dev/null || true
mkfifo /tmp/claude_output 2>/dev/null || true

# Start Claude in streaming JSON mode (background process)
echo "Starting Claude AI assistant in streaming mode..."
claude --input-format=stream-json \
       --output-format=stream-json \
       --replay-user-messages \
       --allowed-tools "Bash(git:*),Bash(cd:*),Bash(ls:*),Bash(cat:*),Edit,Read,Write" \
       --dangerously-skip-permissions \
       < /tmp/claude_input > /tmp/claude_output 2>/tmp/claude.log &

echo $! > /tmp/claude.pid

# Give Claude a moment to start
sleep 2

# Check if Claude started successfully
if kill -0 $(cat /tmp/claude.pid) 2>/dev/null; then
    echo "Claude AI assistant is ready!"
else
    echo "Failed to start Claude. Check /tmp/claude.log for details."
    cat /tmp/claude.log
fi

echo "Worker initialization complete"
`, apiKey, cloneCommands, gitUserName, user.Email)

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

// ExecuteStreamingMessage sends a message to Claude and returns a stream handler
func (wm *WorkerManager) ExecuteStreamingMessage(sandbox SandboxInterface, message string) (*StreamHandler, error) {
	if !sandbox.IsRunning() {
		return nil, fmt.Errorf("sandbox is not running")
	}

	// Create a new stream handler
	handler, err := NewStreamHandler(sandbox)
	if err != nil {
		return nil, fmt.Errorf("failed to create stream handler: %w", err)
	}

	// Start the handler
	if err := handler.Start(); err != nil {
		return nil, fmt.Errorf("failed to start stream handler: %w", err)
	}

	// Send the initial message
	if err := handler.SendMessage(message); err != nil {
		handler.Stop()
		return nil, fmt.Errorf("failed to send message: %w", err)
	}

	return handler, nil
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