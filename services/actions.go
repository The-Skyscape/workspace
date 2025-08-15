package services

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"workspace/models"

	"github.com/pkg/errors"
)

// ActionService handles action execution and sandbox management
type ActionService struct{}

var Actions = &ActionService{}

// TriggerActionsByEvent executes all actions triggered by a specific event
func TriggerActionsByEvent(eventType, repoID string, eventData map[string]string) error {
	// Get all active actions for this repository that match the event type
	actions, err := models.Actions.Search("WHERE RepoID = ? AND Type = ? AND Status = 'active'", repoID, eventType)
	if err != nil {
		log.Printf("Failed to get actions for event %s in repo %s: %v", eventType, repoID, err)
		return err
	}

	// Execute each matching action
	for _, action := range actions {
		// For now, always trigger - complex trigger conditions can be added later
		go func(a *models.Action) {
			// Set the trigger info
			a.LastTriggeredBy = eventData["user_id"]
			// Execute with sandbox (event data will be in environment)
			err := Actions.ExecuteAction(a)
			if err != nil {
				log.Printf("Action %s execution failed for event %s: %v", a.ID, eventType, err)
			} else {
				log.Printf("Action %s executed successfully for event %s", a.ID, eventType)
			}
		}(action)
	}

	return nil
}

// ExecuteAction executes an action by starting a sandbox
func (s *ActionService) ExecuteAction(action *models.Action) error {
	// Create action run record
	run := &models.ActionRun{
		Model:       models.DB.NewModel(""),
		ActionID:    action.ID,
		Branch:      action.Branch,
		Status:      "running",
		TriggeredBy: action.LastTriggeredBy,
		TriggerType: "manual",
		SandboxName: fmt.Sprintf("action-%s-%d", action.ID, time.Now().Unix()),
	}
	
	run, err := models.ActionRuns.Insert(run)
	if err != nil {
		return errors.Wrap(err, "failed to create action run")
	}
	
	// Get repository
	repo, err := models.Repositories.Get(action.RepoID)
	if err != nil {
		run.Status = "failed"
		run.Output = "Repository not found: " + err.Error()
		models.ActionRuns.Update(run)
		return err
	}
	
	// Determine script to execute
	scriptToRun := action.Script
	if scriptToRun == "" && action.Command != "" {
		// Use simplified command if no script provided
		scriptToRun = "#!/bin/bash\nset -e\n" + action.Command
	}
	
	// Start sandbox execution
	err = Sandboxes.StartSandbox(
		run.SandboxName,
		repo.Path(),
		repo.Name,
		scriptToRun,
		600, // 10 minute timeout by default
	)
	
	if err != nil {
		run.Status = "failed"
		run.Output = "Failed to start sandbox: " + err.Error()
		models.ActionRuns.Update(run)
		return err
	}
	
	// Monitor sandbox completion in a goroutine
	go s.monitorSandboxExecution(action, run)
	
	// Update action with last triggered info
	action.LastTriggeredAt = time.Now()
	action.SandboxName = run.SandboxName
	models.Actions.Update(action)
	
	return nil
}

// monitorSandboxExecution monitors the sandbox and updates action status
func (s *ActionService) monitorSandboxExecution(action *models.Action, run *models.ActionRun) {
	sandboxName := run.SandboxName
	startTime := time.Now()
	
	// Poll for completion
	for {
		// Check timeout (default 10 minutes)
		if time.Since(startTime) > 10*time.Minute {
			run.Status = "failed"
			run.Output = "Action timed out after 10 minutes"
			run.ExitCode = -1
			run.Duration = int(time.Since(startTime).Seconds())
			models.ActionRuns.Update(run)
			
			// Clean up sandbox
			go func() {
				time.Sleep(5 * time.Minute)
				if err := Sandboxes.CleanupSandbox(sandboxName); err != nil {
					log.Printf("Failed to cleanup sandbox %s: %v", sandboxName, err)
				}
			}()
			return
		}
		
		// Check if sandbox is still running
		if !Sandboxes.IsRunning(sandboxName) {
			// Get final output
			output, err := Sandboxes.GetOutput(sandboxName)
			if err != nil {
				log.Printf("Failed to get output for sandbox %s: %v", sandboxName, err)
			}
			
			// Get exit code
			run.ExitCode = Sandboxes.GetExitCode(sandboxName)
			run.Output = output
			run.Duration = int(time.Since(startTime).Seconds())
			
			// Update status based on exit code
			if run.ExitCode == 0 {
				run.Status = "success"
				action.Status = "success"
				action.LastSuccessAt = time.Now()
			} else {
				run.Status = "failed"
				action.Status = "failed"
			}
			
			// Update records
			models.ActionRuns.Update(run)
			models.Actions.Update(action)
			
			// Collect artifacts if configured
			if action.ArtifactPaths != "" && run.Status == "success" {
				s.collectArtifacts(action, run)
			}
			
			// Schedule cleanup after delay
			go func() {
				time.Sleep(5 * time.Minute)
				if err := Sandboxes.CleanupSandbox(sandboxName); err != nil {
					log.Printf("Failed to cleanup sandbox %s: %v", sandboxName, err)
				}
			}()
			
			// Log activity
			status := "completed"
			if run.Status == "failed" {
				status = "failed"
			}
			models.LogActivity("action_run", 
				fmt.Sprintf("Action '%s' %s", action.Title, status),
				fmt.Sprintf("Action execution %s with exit code %d", status, run.ExitCode),
				action.LastTriggeredBy, action.RepoID, "action", run.ID)
			
			return
		}
		
		// Wait before next check
		time.Sleep(2 * time.Second)
	}
}

// collectArtifacts collects artifacts from the sandbox
func (s *ActionService) collectArtifacts(action *models.Action, run *models.ActionRun) {
	if action.ArtifactPaths == "" {
		return
	}
	
	// Parse artifact paths (comma-separated)
	paths := strings.Split(action.ArtifactPaths, ",")
	for i, path := range paths {
		paths[i] = strings.TrimSpace(path)
	}
	
	// Collect each artifact
	for _, path := range paths {
		if path == "" {
			continue
		}
		
		// Extract file from sandbox
		content, err := Sandboxes.ExtractFile(run.SandboxName, path)
		if err != nil {
			log.Printf("Failed to extract artifact %s: %v", path, err)
			continue
		}
		
		// Create artifact record
		artifact := &models.ActionArtifact{
			Model:       models.DB.NewModel(""),
			ActionID:    action.ID,
			RunID:       run.ID,
			SandboxName: run.SandboxName,
			FileName:    filepath.Base(path),
			FilePath:    path,
			GroupName:   filepath.Base(path), // Use filename as group for now
			Size:        int64(len(content)),
			Content:     content,
		}
		
		// Save artifact
		if _, err := models.ActionArtifacts.Insert(artifact); err != nil {
			log.Printf("Failed to save artifact %s: %v", path, err)
		}
	}
}