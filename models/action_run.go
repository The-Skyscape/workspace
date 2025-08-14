package models

import (
	"fmt"
	"time"
	
	"github.com/The-Skyscape/devtools/pkg/application"
)

// ActionRun represents a single execution of an action
type ActionRun struct {
	application.Model
	ActionID     string    // ID of the action that was run
	Status       string    // "running", "completed", "failed"
	ExitCode     int       // Exit code from execution
	Output       string    // Execution output/logs
	SandboxName  string    // Name of the sandbox used
	Duration     int       // Duration in seconds
	TriggerType  string    // "manual", "scheduled", "webhook", "push", "pr"
	TriggeredBy  string    // UserID who triggered it (for manual runs)
	Branch       string    // Branch the action ran on
	CommitSHA    string    // Git commit SHA if applicable
	// Note: StartedAt uses CreatedAt from Model
	// CompletedAt can be calculated from CreatedAt + Duration
}

// Table returns the database table name
func (*ActionRun) Table() string { return "action_runs" }

// GetByAction returns all runs for a specific action
func GetRunsByAction(actionID string) ([]*ActionRun, error) {
	return ActionRuns.Search("WHERE ActionID = ? ORDER BY CreatedAt DESC", actionID)
}

// GetLatestByAction returns the most recent run for an action
func GetLatestRunByAction(actionID string) (*ActionRun, error) {
	runs, err := ActionRuns.Search("WHERE ActionID = ? ORDER BY CreatedAt DESC LIMIT 1", actionID)
	if err != nil {
		return nil, err
	}
	if len(runs) == 0 {
		return nil, nil
	}
	return runs[0], nil
}

// GetRunningByAction returns any currently running instance of an action
func GetRunningByAction(actionID string) (*ActionRun, error) {
	runs, err := ActionRuns.Search("WHERE ActionID = ? AND Status = 'running' ORDER BY CreatedAt DESC LIMIT 1", actionID)
	if err != nil {
		return nil, err
	}
	if len(runs) == 0 {
		return nil, nil
	}
	return runs[0], nil
}

// GetStartedAt returns the start time (uses CreatedAt)
func (r *ActionRun) GetStartedAt() time.Time {
	return r.CreatedAt
}

// GetCompletedAt returns the completion time (calculated from CreatedAt + Duration)
func (r *ActionRun) GetCompletedAt() *time.Time {
	if r.Status == "running" || r.Duration == 0 {
		return nil
	}
	completedAt := r.CreatedAt.Add(time.Duration(r.Duration) * time.Second)
	return &completedAt
}

// FormatDuration returns a human-readable duration string
func (r *ActionRun) FormatDuration() string {
	if r.Duration == 0 {
		return "< 1s"
	}
	if r.Duration < 60 {
		return fmt.Sprintf("%ds", r.Duration)
	}
	if r.Duration < 3600 {
		minutes := r.Duration / 60
		seconds := r.Duration % 60
		if seconds == 0 {
			return fmt.Sprintf("%dm", minutes)
		}
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	hours := r.Duration / 3600
	minutes := (r.Duration % 3600) / 60
	if minutes == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh %dm", hours, minutes)
}

// IsRunning returns true if this run is currently executing
func (r *ActionRun) IsRunning() bool {
	return r.Status == "running"
}

// Save creates a new run record
func (r *ActionRun) Save() error {
	run, err := ActionRuns.Insert(r)
	if err != nil {
		return err
	}
	r.ID = run.ID
	r.CreatedAt = run.CreatedAt
	r.UpdatedAt = run.UpdatedAt
	return nil
}