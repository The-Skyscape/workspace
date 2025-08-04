package models

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
	
	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/database"
	"github.com/pkg/errors"
)

type Action struct {
	application.Model
	Type        string // "on_push", "on_pr", "on_issue", "scheduled", "manual"
	Title       string // Action title/summary
	Description string // Action description
	Trigger     string // JSON config for trigger conditions
	Script      string // Bash script or commands to execute
	Status      string // "active", "running", "completed", "failed", "disabled"
	LastRun     *time.Time `json:"last_run,omitempty"`
	Output      string // Last execution output
	RepoID      string
	UserID      string
}

func (*Action) Table() string { return "actions" }

// Execute runs the action script in the repository context
func (a *Action) Execute() error {
	// Update status to running
	a.Status = "running"
	now := time.Now()
	a.LastRun = &now
	
	err := Actions.Update(a)
	if err != nil {
		return errors.Wrap(err, "failed to update action status")
	}
	
	// Get repository
	repo, err := GitRepos.Get(a.RepoID)
	if err != nil {
		a.markFailed("repository not found: " + err.Error())
		return err
	}
	
	// Execute in repository directory
	repoPath := filepath.Join(database.DataDir(), "repos", repo.ID)
	
	// Create execution context
	output, err := a.executeScript(repoPath)
	
	// Update action with results
	a.Output = output
	if err != nil {
		a.markFailed("execution failed: " + err.Error())
		return err
	} else {
		a.markCompleted("execution successful")
		return nil
	}
}

// executeScript executes the action script in the given directory
func (a *Action) executeScript(workDir string) (string, error) {
	if a.Script == "" {
		return "", errors.New("no script to execute")
	}
	
	// Create a temporary script file
	scriptPath := filepath.Join(workDir, ".action_script.sh")
	
	// Write script to file
	scriptContent := fmt.Sprintf("#!/bin/bash\nset -e\ncd %s\n\n%s", workDir, a.Script)
	err := os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	if err != nil {
		return "", errors.Wrap(err, "failed to write script file")
	}
	
	// Clean up script file after execution
	defer os.Remove(scriptPath)
	
	// Execute the script
	cmd := exec.Command("bash", scriptPath)
	cmd.Dir = workDir
	
	// Set up environment variables
	cmd.Env = append(os.Environ(),
		"REPO_ID="+a.RepoID,
		"USER_ID="+a.UserID,
		"ACTION_ID="+a.ID,
		"ACTION_TYPE="+a.Type,
		"WORKSPACE_ROOT="+workDir,
	)
	
	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	err = cmd.Run()
	
	// Combine stdout and stderr for output
	output := stdout.String()
	if stderr.String() != "" {
		output += "\n--- STDERR ---\n" + stderr.String()
	}
	
	if err != nil {
		return output, errors.Wrap(err, "script execution failed")
	}
	
	return output, nil
}

// markCompleted updates the action status to completed
func (a *Action) markCompleted(message string) {
	a.Status = "completed"
	if message != "" {
		a.Output += "\n--- SUCCESS ---\n" + message
	}
	Actions.Update(a)
}

// markFailed updates the action status to failed
func (a *Action) markFailed(message string) {
	a.Status = "failed"
	if message != "" {
		a.Output += "\n--- ERROR ---\n" + message
	}
	Actions.Update(a)
}

// CanExecute checks if the action can be executed
func (a *Action) CanExecute() bool {
	return a.Status != "running" && a.Status != "disabled" && a.Script != ""
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
				// Set additional environment variables for the event
				err := a.ExecuteWithEventContext(eventData)
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
	// Update status to running
	a.Status = "running"
	now := time.Now()
	a.LastRun = &now
	
	err := Actions.Update(a)
	if err != nil {
		return errors.Wrap(err, "failed to update action status")
	}
	
	// Get repository
	repo, err := GitRepos.Get(a.RepoID)
	if err != nil {
		a.markFailed("repository not found: " + err.Error())
		return err
	}
	
	// Execute in repository directory with event context
	repoPath := filepath.Join(database.DataDir(), "repos", repo.ID)
	
	// Create execution context with event data
	output, err := a.executeScriptWithEventData(repoPath, eventData)
	
	// Update action with results
	a.Output = output
	if err != nil {
		a.markFailed("execution failed: " + err.Error())
		return err
	} else {
		a.markCompleted("execution successful")
		return nil
	}
}

// executeScriptWithEventData executes the action script with event context
func (a *Action) executeScriptWithEventData(workDir string, eventData map[string]string) (string, error) {
	if a.Script == "" {
		return "", errors.New("no script to execute")
	}
	
	// Create a temporary script file
	scriptPath := filepath.Join(workDir, ".action_script.sh")
	
	// Write script to file
	scriptContent := fmt.Sprintf("#!/bin/bash\nset -e\ncd %s\n\n%s", workDir, a.Script)
	err := os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	if err != nil {
		return "", errors.Wrap(err, "failed to write script file")
	}
	
	// Clean up script file after execution
	defer os.Remove(scriptPath)
	
	// Execute the script
	cmd := exec.Command("bash", scriptPath)
	cmd.Dir = workDir
	
	// Set up environment variables including event data
	env := append(os.Environ(),
		"REPO_ID="+a.RepoID,
		"USER_ID="+a.UserID,
		"ACTION_ID="+a.ID,
		"ACTION_TYPE="+a.Type,
		"WORKSPACE_ROOT="+workDir,
	)
	
	// Add event-specific environment variables
	for key, value := range eventData {
		env = append(env, key+"="+value)
	}
	
	cmd.Env = env
	
	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	err = cmd.Run()
	
	// Combine stdout and stderr for output
	output := stdout.String()
	if stderr.String() != "" {
		output += "\n--- STDERR ---\n" + stderr.String()
	}
	
	if err != nil {
		return output, errors.Wrap(err, "script execution failed")
	}
	
	return output, nil
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