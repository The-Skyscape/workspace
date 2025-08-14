package models

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"
	
	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/pkg/errors"
	"workspace/services"
)

type Action struct {
	application.Model
	Type          string // "on_push", "on_pr", "on_issue", "scheduled", "manual"
	Title         string // Action title/summary
	Description   string // Action description
	Trigger       string // JSON config for trigger conditions
	Script        string // Bash script or commands to execute
	Branch        string // Branch to run the action on
	Command       string // Simplified command to execute (alternative to Script)
	ArtifactPaths string // Comma-separated list of artifact paths to collect
	Status        string // "active", "running", "completed", "failed", "disabled"
	LastRun       *time.Time `json:"last_run,omitempty"`
	Output        string // Last execution output
	RepoID        string
	UserID        string
	SandboxName   string // Name of the sandbox used for execution
	ExitCode      int    // Exit code from sandbox execution
}

func (*Action) Table() string { return "actions" }

// Execute runs the action script in a sandbox container
func (a *Action) Execute() error {
	// Create a new action run record
	run := &ActionRun{
		ActionID:    a.ID,
		Status:      "running",
		TriggerType: "manual",
		TriggeredBy: a.UserID,
		Branch:      a.Branch,
	}
	
	err := run.Save()
	if err != nil {
		return errors.Wrap(err, "failed to create action run")
	}
	
	// Update action status to running
	a.Status = "running"
	now := time.Now()
	a.LastRun = &now
	
	err = Actions.Update(a)
	if err != nil {
		run.Status = "failed"
		run.Output = "Failed to update action status: " + err.Error()
		ActionRuns.Update(run)
		return errors.Wrap(err, "failed to update action status")
	}
	
	// Get repository
	repo, err := Repositories.Get(a.RepoID)
	if err != nil {
		a.markFailed("repository not found: " + err.Error())
		run.Status = "failed"
		run.Output = "Repository not found: " + err.Error()
		ActionRuns.Update(run)
		return err
	}
	
	// Generate unique sandbox name
	sandboxName := fmt.Sprintf("action-%s-%d", a.ID, time.Now().Unix())
	a.SandboxName = sandboxName
	run.SandboxName = sandboxName
	
	// Determine script to execute
	scriptToRun := a.Script
	if scriptToRun == "" && a.Command != "" {
		// Use simplified command if no script provided
		scriptToRun = "#!/bin/bash\nset -e\n" + a.Command
	}
	
	// Start sandbox execution
	err = services.Sandboxes.StartSandbox(
		sandboxName,
		repo.Path(),
		repo.Name,
		scriptToRun,
		600, // 10 minute timeout by default
	)
	
	if err != nil {
		a.markFailed("failed to start sandbox: " + err.Error())
		run.Status = "failed"
		run.Output = "Failed to start sandbox: " + err.Error()
		ActionRuns.Update(run)
		return err
	}
	
	// Monitor sandbox completion in a goroutine
	go a.monitorSandboxExecution(sandboxName, run.ID)
	
	// Update action with sandbox info
	Actions.Update(a)
	
	return nil
}

// monitorSandboxExecution monitors the sandbox and updates action status
func (a *Action) monitorSandboxExecution(sandboxName string, runID string) {
	startTime := time.Now()
	
	// Get the action run record
	run, err := ActionRuns.Get(runID)
	if err != nil {
		log.Printf("Failed to get action run %s: %v", runID, err)
		return
	}
	
	// Poll sandbox status
	for {
		time.Sleep(2 * time.Second)
		
		// Check if sandbox is still running
		if !services.Sandboxes.IsRunning(sandboxName) {
			// Calculate duration
			duration := int(time.Since(startTime).Seconds())
			
			// Get final output and exit code
			output, err := services.Sandboxes.GetOutput(sandboxName)
			if err != nil {
				log.Printf("Failed to get sandbox output for action %s: %v", a.ID, err)
				output = "Failed to retrieve output: " + err.Error()
			}
			
			a.Output = output
			a.ExitCode = services.Sandboxes.GetExitCode(sandboxName)
			
			// Update run record
			run.Output = output
			run.ExitCode = a.ExitCode
			run.Duration = duration
			
			// Update status based on exit code
			if a.ExitCode == 0 {
				a.markCompleted("execution successful")
				run.Status = "completed"
			} else {
				a.markFailed(fmt.Sprintf("execution failed with exit code %d", a.ExitCode))
				run.Status = "failed"
			}
			
			// Save run record
			ActionRuns.Update(run)
			
			// Collect artifacts before cleanup
			var artifactPaths []string
			if a.ArtifactPaths != "" {
				// Parse comma-separated artifact paths
				for _, path := range strings.Split(a.ArtifactPaths, ",") {
					if trimmed := strings.TrimSpace(path); trimmed != "" {
						artifactPaths = append(artifactPaths, trimmed)
					}
				}
			} else {
				// Default artifact paths if none specified
				artifactPaths = []string{
					"build.log",
					"test-results.xml",
					"coverage.html", 
					"dist.tar.gz",
					"output.log",
				}
			}
			if err := a.CollectArtifacts(sandboxName, artifactPaths, run.ID); err != nil {
				log.Printf("Failed to collect artifacts for action %s: %v", a.ID, err)
			}
			
			// Optionally cleanup sandbox after a delay
			go func() {
				time.Sleep(5 * time.Minute)
				if err := services.Sandboxes.CleanupSandbox(sandboxName); err != nil {
					log.Printf("Failed to cleanup sandbox %s: %v", sandboxName, err)
				}
			}()
			
			return
		}
	}
}

// markCompleted updates the action status to completed
func (a *Action) markCompleted(message string) {
	a.Status = "completed"
	if message != "" {
		if a.Output != "" {
			a.Output += "\n"
		}
		a.Output += "--- SUCCESS ---\n" + message
	}
	Actions.Update(a)
}

// markFailed updates the action status to failed
func (a *Action) markFailed(message string) {
	a.Status = "failed"
	if message != "" {
		if a.Output != "" {
			a.Output += "\n"
		}
		a.Output += "--- ERROR ---\n" + message
	}
	Actions.Update(a)
}

// CanExecute checks if the action can be executed
func (a *Action) CanExecute() bool {
	return a.Status != "running" && a.Status != "disabled" && (a.Script != "" || a.Command != "")
}

// ExecuteManually executes a manual action immediately
func (a *Action) ExecuteManually() error {
	if a.Type != "manual" {
		return errors.New("only manual actions can be executed directly")
	}
	
	if !a.CanExecute() {
		return errors.New("action cannot be executed at this time")
	}
	
	// Execute asynchronously to avoid blocking the request
	go func() {
		err := a.Execute()
		if err != nil {
			log.Printf("Action %s execution failed: %v", a.ID, err)
		} else {
			log.Printf("Action %s executed successfully", a.ID)
		}
	}()
	
	return nil
}

// ActionExecution represents a single execution instance
type ActionExecution struct {
	ActionID  string
	StartTime time.Time
	EndTime   *time.Time
	Status    string // "running", "completed", "failed"
	Output    string
	Error     string
}

// TriggerActionsByEvent executes all actions triggered by a specific event
func TriggerActionsByEvent(eventType, repoID string, eventData map[string]string) error {
	// Get all active actions for this repository that match the event type
	actions, err := Actions.Search("WHERE RepoID = ? AND Type = ? AND Status = 'active'", repoID, eventType)
	if err != nil {
		log.Printf("Failed to get actions for event %s in repo %s: %v", eventType, repoID, err)
		return err
	}

	// Execute each matching action
	for _, action := range actions {
		// Check if trigger conditions match (if any)
		if action.shouldTriggerForEvent(eventData) {
			go func(a *Action) {
				// Execute with sandbox (event data will be in environment)
				err := a.Execute()
				if err != nil {
					log.Printf("Action %s execution failed for event %s: %v", a.ID, eventType, err)
				} else {
					log.Printf("Action %s executed successfully for event %s", a.ID, eventType)
				}
			}(action)
		}
	}

	return nil
}

// ExecuteWithEventContext executes an action with additional event context
func (a *Action) ExecuteWithEventContext(eventData map[string]string) error {
	// For now, just execute normally - sandbox will have access to event data through environment
	return a.Execute()
}

// shouldTriggerForEvent checks if the action should trigger for the given event
func (a *Action) shouldTriggerForEvent(eventData map[string]string) bool {
	// If no trigger conditions are specified, always trigger
	if a.Trigger == "" {
		return true
	}
	
	// For now, implement basic trigger logic
	// In a production system, you might want to use JSON parsing for complex conditions
	
	// Check for branch-specific triggers
	if branch, exists := eventData["BRANCH"]; exists {
		if a.Trigger == fmt.Sprintf(`{"branch": "%s"}`, branch) {
			return true
		}
		if a.Trigger == `{"branch": "main"}` && branch == "main" {
			return true
		}
		if a.Trigger == `{"branch": "master"}` && branch == "master" {
			return true
		}
	}
	
	// Default to triggering if we can't parse the condition
	return true
}

// GetSandboxInfo returns information about the sandbox used for this action
func (a *Action) GetSandboxInfo() (*services.SandboxInfo, error) {
	if a.SandboxName == "" {
		return nil, errors.New("no sandbox associated with this action")
	}
	
	return services.Sandboxes.GetSandboxInfo(a.SandboxName)
}

// CollectArtifacts saves specified files from the sandbox as action artifacts
func (a *Action) CollectArtifacts(sandboxName string, paths []string, runID string) error {
	if len(paths) == 0 {
		// Default paths to collect if none specified
		paths = []string{
			"build.log",
			"test-results.xml", 
			"coverage.html",
			"dist.tar.gz",
		}
	}
	
	for _, path := range paths {
		// Try to extract the file from sandbox
		content, err := services.Sandboxes.ExtractFile(sandboxName, path)
		if err != nil {
			// File doesn't exist, skip it
			log.Printf("Artifact %s not found in sandbox %s: %v", path, sandboxName, err)
			continue
		}
		
		// Determine content type
		contentType := "application/octet-stream"
		if strings.HasSuffix(path, ".log") || strings.HasSuffix(path, ".txt") {
			contentType = "text/plain"
		} else if strings.HasSuffix(path, ".json") {
			contentType = "application/json"
		} else if strings.HasSuffix(path, ".xml") {
			contentType = "application/xml"
		} else if strings.HasSuffix(path, ".html") {
			contentType = "text/html"
		} else if strings.HasSuffix(path, ".tar.gz") || strings.HasSuffix(path, ".tgz") {
			contentType = "application/gzip"
		}
		
		// Create artifact record
		artifact := &ActionArtifact{
			ActionID:    a.ID,
			RunID:       runID,
			SandboxName: sandboxName,
			FileName:    filepath.Base(path),
			FilePath:    path,
			ContentType: contentType,
			Size:        int64(len(content)),
			Content:     content,
		}
		
		// Save to database
		if err := artifact.Save(); err != nil {
			log.Printf("Failed to save artifact %s: %v", path, err)
			continue
		}
		
		log.Printf("Saved artifact %s (%d bytes) from action %s", path, len(content), a.ID)
	}
	
	return nil
}