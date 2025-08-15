package models

import (
	"time"
	
	"github.com/The-Skyscape/devtools/pkg/application"
)

type Action struct {
	application.Model
	Type          string // "on_push", "on_pr", "on_issue", "scheduled", "manual"
	Title         string // Action title/summary
	Description   string // Action description
	Trigger       string // JSON config for trigger conditions
	Script        string // Shell script to execute
	Command       string // Simple command to execute (alternative to script)
	Branch        string // Branch to run against
	RepoID        string // Repository ID
	UserID        string // User who created the action
	Status        string // "active", "inactive", "running", "completed", "failed"
	
	// Execution tracking
	LastRun         *time.Time // When the action was last run
	LastSuccess     *time.Time // When the action last succeeded
	NextRun         *time.Time // For scheduled actions, when to run next
	ExecutionCount  int        // How many times this action has been executed
	SuccessCount    int        // How many times it succeeded
	FailureCount    int        // How many times it failed
	AverageRuntime  int        // Average runtime in seconds
	LastTriggeredBy string     // User who last triggered it
	LastTriggeredAt time.Time  // When it was last triggered
	LastSuccessAt   time.Time  // When it last succeeded
	SandboxName     string     // Current sandbox container name
	
	// Configuration
	TimeoutSeconds  int        // Maximum execution time
	MaxRetries      int        // Maximum retries on failure
	RetryDelay      int        // Delay between retries in seconds
	Environment     string     // JSON-encoded environment variables
	WorkingDir      string     // Working directory for execution
	Artifacts       string     // Files/directories to preserve after execution
	ArtifactPaths   string     // Comma-separated paths to collect as artifacts
	CachePaths      string     // Paths to cache between runs
	DockerImage     string     // Docker image to use (default: ubuntu:latest)
	
	// Output tracking
	Output      string     // Combined stdout/stderr from last execution
	ExitCode    int        // Exit code from last execution (-1 if not finished)
}

func (*Action) Table() string { return "actions" }

// NOTE: Execution logic has been moved to services/actions.go to avoid circular dependencies
// The following methods are now handled by services.Actions:
// - ExecuteAction(action *Action) error
// - ExecuteManually(action *Action) error
// - TriggerActionsByEvent(eventType, repoID string, eventData map[string]string) error

// CanExecute checks if the action can be executed
func (a *Action) CanExecute() bool {
	return a.Status != "running" && a.Status != "disabled" && (a.Script != "" || a.Command != "")
}