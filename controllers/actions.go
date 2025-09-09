package controllers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"workspace/models"
	"workspace/services"

	"github.com/The-Skyscape/devtools/pkg/application"
)

// Actions controller prefix
func Actions() (string, *ActionsController) {
	return "actions", &ActionsController{}
}

// ActionsController handles action-related operations
type ActionsController struct {
	application.BaseController
}

// Handle returns a new controller instance for the request
func (c ActionsController) Handle(req *http.Request) application.Controller {
	c.Request = req
	return &c
}

// Setup registers routes
func (c *ActionsController) Setup(app *application.App) {
	c.BaseController.Setup(app)

	// Actions - view on public repos or as admin
	http.Handle("GET /repos/{id}/actions", app.Serve("repo-actions.html", PublicOrAdmin()))
	// Main action view redirects to info tab
	http.Handle("GET /repos/{id}/actions/{actionID}", app.ProtectFunc(c.redirectToInfo, PublicOrAdmin()))
	// Tab-specific routes
	http.Handle("GET /repos/{id}/actions/{actionID}/info", app.Serve("repo-action-info.html", PublicOrAdmin()))
	http.Handle("GET /repos/{id}/actions/{actionID}/command", app.Serve("repo-action-command.html", PublicOrAdmin()))
	http.Handle("GET /repos/{id}/actions/{actionID}/logs", app.Serve("repo-action-logs.html", PublicOrAdmin()))
	http.Handle("GET /repos/{id}/actions/{actionID}/history", app.Serve("repo-action-history.html", PublicOrAdmin()))
	http.Handle("GET /repos/{id}/actions/{actionID}/artifacts", app.Serve("repo-action-artifacts.html", PublicOrAdmin()))
	// HTMX partials for dynamic updates
	http.Handle("GET /repos/{id}/actions/{actionID}/logs-partial", app.ProtectFunc(c.getActionLogs, PublicOrAdmin()))
	http.Handle("GET /repos/{id}/actions/{actionID}/artifacts-partial", app.ProtectFunc(c.getActionArtifacts, PublicOrAdmin()))
	// Action operations - admin only
	http.Handle("POST /repos/{id}/actions/create", app.ProtectFunc(c.createAction, AdminOnly()))
	http.Handle("POST /repos/{id}/actions/{actionID}/run", app.ProtectFunc(c.runAction, AdminOnly()))
	http.Handle("POST /repos/{id}/actions/{actionID}/disable", app.ProtectFunc(c.disableAction, AdminOnly()))
	http.Handle("POST /repos/{id}/actions/{actionID}/enable", app.ProtectFunc(c.enableAction, AdminOnly()))
	// Artifact download - public repos or admin
	http.Handle("GET /repos/{id}/actions/{actionID}/artifacts/{artifactID}/download", app.ProtectFunc(c.downloadArtifact, PublicOrAdmin()))
}

// RepoActions returns actions for the current repository
func (c *ActionsController) RepoActions() ([]*models.Action, error) {
	reposController := c.Use("repos").(*ReposController)
	repo, err := reposController.CurrentRepo()
	if err != nil {
		return nil, err
	}
	return models.Actions.Search("WHERE RepoID = ? ORDER BY CreatedAt DESC", repo.ID)
}

// CurrentAction returns the action from the request
func (c *ActionsController) CurrentAction() (*models.Action, error) {
	actionID := c.Request.PathValue("actionID")
	if actionID == "" {
		return nil, errors.New("action ID required")
	}
	return models.Actions.Get(actionID)
}

// ActionArtifacts returns artifacts for the current action
func (c *ActionsController) ActionArtifacts() ([]*models.ActionArtifact, error) {
	action, err := c.CurrentAction()
	if err != nil {
		return nil, err
	}
	return models.GetArtifactsByAction(action.ID)
}

// RepoBranches returns branches for the current repository via repos controller
func (c *ActionsController) RepoBranches() ([]*models.Branch, error) {
	reposController := c.Use("repos").(*ReposController)
	return reposController.RepoBranches()
}

// redirectToInfo redirects from base action URL to info tab
func (c *ActionsController) redirectToInfo(w http.ResponseWriter, r *http.Request) {
	c.SetRequest(r)
	
	repoID := r.PathValue("id")
	actionID := r.PathValue("actionID")
	c.Redirect(w, r, fmt.Sprintf("/repos/%s/actions/%s/info", repoID, actionID))
}

// createAction handles action creation
func (c *ActionsController) createAction(w http.ResponseWriter, r *http.Request) {
	c.SetRequest(r)
	
	// Admin access already verified by route middleware

	auth := c.Use("auth").(*AuthController)
	user := auth.GetAuthenticatedUser(r)
	if user == nil {
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}

	repoID := r.PathValue("id")
	if repoID == "" {
		c.RenderErrorMsg(w, r, "repository ID required")
		return
	}

	// Parse form parameters
	p := c.Params()
	title := strings.TrimSpace(p.String("title", ""))
	description := strings.TrimSpace(p.String("description", ""))
	actionType := strings.TrimSpace(p.String("type", ""))
	branch := strings.TrimSpace(p.String("branch", ""))
	command := strings.TrimSpace(p.String("command", ""))
	artifactPaths := strings.TrimSpace(p.String("artifact_paths", ""))

	// Validate required fields
	v := c.Validator()
	v.CheckRequired("title", title)
	v.CheckRequired("type", actionType)
	v.CheckRequired("command", command)
	
	if err := v.Result(); err != nil {
		c.RenderValidationError(w, r, err)
		return
	}

	// Create action
	action := &models.Action{
		Title:         title,
		Description:   description,
		Type:          actionType,
		Branch:        branch,
		Command:       command,
		ArtifactPaths: artifactPaths,
		Status:        "active",
		RepoID:        repoID,
		UserID:        user.ID,
	}

	_, err := models.Actions.Insert(action)
	if err != nil {
		c.RenderError(w, r, fmt.Errorf("failed to create action: %w", err))
		return
	}

	// Log activity
	models.LogActivity("action_created", "Created action: "+action.Title,
		"New action configured", user.ID, repoID, "action", action.ID)

	// Redirect to actions page
	c.Redirect(w, r, "/repos/"+repoID+"/actions")
}

// runAction handles manual action execution
func (c *ActionsController) runAction(w http.ResponseWriter, r *http.Request) {
	c.SetRequest(r)
	
	// Admin access already verified by route middleware

	auth := c.Use("auth").(*AuthController)
	user := auth.GetAuthenticatedUser(r)

	repoID := r.PathValue("id")
	actionID := r.PathValue("actionID")

	if repoID == "" || actionID == "" {
		c.RenderErrorMsg(w, r, "repository ID and action ID required")
		return
	}

	// Get action
	action, err := models.Actions.Get(actionID)
	if err != nil {
		c.RenderErrorMsg(w, r, "action not found")
		return
	}

	// Run action manually
	if action.Type != "manual" {
		c.RenderErrorMsg(w, r, "only manual actions can be executed directly")
		return
	}

	if !action.CanExecute() {
		c.RenderErrorMsg(w, r, "action cannot be executed at this time")
		return
	}

	// Execute using the Actions service for parallel execution
	if err := services.Actions.ExecuteAction(action, "manual"); err != nil {
		c.RenderErrorMsg(w, r, fmt.Sprintf("Failed to queue action: %v", err))
		return
	}

	// Log activity
	models.LogActivity("action_run", "Manually ran action: "+action.Title,
		"Action executed manually", user.ID, repoID, "action", action.ID)

	// Refresh to show status
	c.Refresh(w, r)
}

// disableAction handles disabling an action
func (c *ActionsController) disableAction(w http.ResponseWriter, r *http.Request) {
	c.SetRequest(r)
	
	// Admin access already verified by route middleware

	auth := c.Use("auth").(*AuthController)
	user := auth.GetAuthenticatedUser(r)

	repoID := r.PathValue("id")
	actionID := r.PathValue("actionID")

	if repoID == "" || actionID == "" {
		c.RenderErrorMsg(w, r, "repository ID and action ID required")
		return
	}

	// Get and update action
	action, err := models.Actions.Get(actionID)
	if err != nil {
		c.RenderErrorMsg(w, r, "action not found")
		return
	}

	action.Status = "disabled"
	err = models.Actions.Update(action)
	if err != nil {
		c.RenderErrorMsg(w, r, "failed to disable action")
		return
	}

	// Log activity
	models.LogActivity("action_disabled", "Disabled action: "+action.Title,
		"Action disabled", user.ID, repoID, "action", action.ID)

	c.Refresh(w, r)
}

// enableAction handles enabling an action
func (c *ActionsController) enableAction(w http.ResponseWriter, r *http.Request) {
	c.SetRequest(r)
	
	// Admin access already verified by route middleware

	auth := c.Use("auth").(*AuthController)
	user := auth.GetAuthenticatedUser(r)

	repoID := r.PathValue("id")
	actionID := r.PathValue("actionID")

	if repoID == "" || actionID == "" {
		c.RenderErrorMsg(w, r, "repository ID and action ID required")
		return
	}

	// Get and update action
	action, err := models.Actions.Get(actionID)
	if err != nil {
		c.RenderErrorMsg(w, r, "action not found")
		return
	}

	action.Status = "active"
	err = models.Actions.Update(action)
	if err != nil {
		c.RenderErrorMsg(w, r, "failed to enable action")
		return
	}

	// Log activity
	models.LogActivity("action_enabled", "Enabled action: "+action.Title,
		"Action enabled", user.ID, repoID, "action", action.ID)

	c.Refresh(w, r)
}

// downloadArtifact handles artifact download
func (c *ActionsController) downloadArtifact(w http.ResponseWriter, r *http.Request) {
	c.SetRequest(r)
	
	// Access already verified by route middleware (PublicOrAdmin)

	auth := c.Use("auth").(*AuthController)
	user := auth.GetAuthenticatedUser(r)

	repoID := r.PathValue("id")
	artifactID := r.PathValue("artifactID")

	if repoID == "" || artifactID == "" {
		http.Error(w, "repository ID and artifact ID required", http.StatusBadRequest)
		return
	}

	// Get artifact
	artifact, err := models.ActionArtifacts.Get(artifactID)
	if err != nil {
		http.Error(w, "artifact not found", http.StatusNotFound)
		return
	}

	// Set headers for download
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", artifact.FileName))
	w.Header().Set("Content-Length", strconv.Itoa(len(artifact.Content)))

	// Write content
	w.Write(artifact.Content)

	// Log activity
	models.LogActivity("artifact_downloaded", "Downloaded artifact: "+artifact.FileName,
		"Artifact file downloaded", user.ID, repoID, "artifact", artifact.ID)
}

// getActionLogs handles fetching action logs via HTMX
func (c *ActionsController) getActionLogs(w http.ResponseWriter, r *http.Request) {
	c.SetRequest(r)
	
	// Access already verified by route middleware

	repoID := r.PathValue("id")
	actionID := r.PathValue("actionID")

	if repoID == "" || actionID == "" {
		c.RenderErrorMsg(w, r, "repository ID and action ID required")
		return
	}

	// Render logs partial
	c.App.Render(w, r, "action-logs-partial.html", nil)
}

// getActionArtifacts handles fetching action artifacts via HTMX
func (c *ActionsController) getActionArtifacts(w http.ResponseWriter, r *http.Request) {
	c.SetRequest(r)
	
	// Access already verified by route middleware

	repoID := r.PathValue("id")
	actionID := r.PathValue("actionID")

	if repoID == "" || actionID == "" {
		c.RenderErrorMsg(w, r, "repository ID and action ID required")
		return
	}

	// Render artifacts partial
	c.App.Render(w, r, "action-artifacts-partial.html", nil)
}

// IsActionRunning checks if the current action is running
func (c *ActionsController) IsActionRunning() bool {
	action, err := c.CurrentAction()
	if err != nil {
		return false
	}
	return action.Status == "running"
}

// ActionLogs returns formatted logs for display
func (c *ActionsController) ActionLogs() string {
	action, err := c.CurrentAction()
	if err != nil || action.Output == "" {
		return "No logs available yet."
	}
	return action.Output
}

// FormatFileSize formats bytes into human readable size
func (c *ActionsController) FormatFileSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

// LastRun returns the most recent run for the current action
func (c *ActionsController) LastRun() (*models.ActionRun, error) {
	action, err := c.CurrentAction()
	if err != nil {
		return nil, err
	}
	return models.GetLatestRunByAction(action.ID)
}

// ActionRuns returns all runs for the current action
func (c *ActionsController) ActionRuns() ([]*models.ActionRun, error) {
	action, err := c.CurrentAction()
	if err != nil {
		return nil, err
	}
	return models.GetRunsByAction(action.ID)
}

// TotalRuns returns the total number of runs for the current action
func (c *ActionsController) TotalRuns() int {
	runs, err := c.ActionRuns()
	if err != nil {
		return 0
	}
	return len(runs)
}

// SuccessfulRuns returns the number of successful runs
func (c *ActionsController) SuccessfulRuns() int {
	runs, err := c.ActionRuns()
	if err != nil {
		return 0
	}
	count := 0
	for _, run := range runs {
		if run.Status == "completed" && run.ExitCode == 0 {
			count++
		}
	}
	return count
}

// FailedRuns returns the number of failed runs
func (c *ActionsController) FailedRuns() int {
	runs, err := c.ActionRuns()
	if err != nil {
		return 0
	}
	count := 0
	for _, run := range runs {
		if run.Status == "failed" || run.ExitCode != 0 {
			count++
		}
	}
	return count
}

// SuccessRate returns the success rate as a percentage
func (c *ActionsController) SuccessRate() int {
	total := c.TotalRuns()
	if total == 0 {
		return 0
	}
	successful := c.SuccessfulRuns()
	return (successful * 100) / total
}

// GroupedArtifacts returns artifacts grouped by name for the current action
func (c *ActionsController) GroupedArtifacts() (map[string][]*models.ActionArtifact, error) {
	action, err := c.CurrentAction()
	if err != nil {
		return nil, err
	}
	return models.GetGroupedArtifactsByAction(action.ID)
}

// executeAction executes an action by creating a sandbox
func (c *ActionsController) executeAction(action *models.Action, userID string) {
	// Create action run record
	run := &models.ActionRun{
		Model:       models.DB.NewModel(""),
		ActionID:    action.ID,
		Branch:      action.Branch,
		Status:      "running",
		TriggeredBy: userID,
		TriggerType: "manual",
		SandboxName: fmt.Sprintf("action-%s-%d", action.ID, time.Now().Unix()),
	}

	run, err := models.ActionRuns.Insert(run)
	if err != nil {
		log.Printf("Failed to create action run: %v", err)
		return
	}

	// Get repository
	repo, err := models.Repositories.Get(action.RepoID)
	if err != nil {
		run.Status = "failed"
		run.Output = "Repository not found: " + err.Error()
		models.ActionRuns.Update(run)
		return
	}

	// Determine script to execute
	scriptToRun := action.Script
	if scriptToRun == "" && action.Command != "" {
		// Use simplified command if no script provided
		scriptToRun = "#!/bin/bash\nset -e\n" + action.Command
	}

	// Create and start sandbox
	sandbox, err := services.NewSandbox(
		run.SandboxName,
		repo.Path(),
		repo.Name,
		scriptToRun,
		600, // 10 minute timeout by default
	)
	if err != nil {
		run.Status = "failed"
		run.Output = "Failed to create sandbox: " + err.Error()
		models.ActionRuns.Update(run)
		return
	}

	// Start sandbox execution
	if err := sandbox.Start(); err != nil {
		run.Status = "failed"
		run.Output = "Failed to start sandbox: " + err.Error()
		models.ActionRuns.Update(run)
		return
	}

	// Update action with last triggered info
	action.LastTriggeredAt = time.Now()
	action.LastTriggeredBy = userID
	action.SandboxName = run.SandboxName
	models.Actions.Update(action)

	// Monitor sandbox completion in a goroutine
	go c.monitorExecution(action, run, sandbox)
}

// monitorExecution monitors the sandbox and updates action status
func (c *ActionsController) monitorExecution(action *models.Action, run *models.ActionRun, sandbox *services.Sandbox) {
	startTime := time.Now()

	// Wait for completion
	sandbox.WaitForCompletion()

	// Get final output
	output, err := sandbox.GetOutput()
	if err != nil {
		log.Printf("Failed to get output for sandbox %s: %v", sandbox.Name, err)
	}

	// Get exit code
	run.ExitCode = sandbox.GetExitCode()
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
		c.collectArtifacts(action, run, sandbox)
	}

	// Schedule cleanup after delay
	go func() {
		time.Sleep(5 * time.Minute)
		if err := sandbox.Cleanup(); err != nil {
			log.Printf("Failed to cleanup sandbox %s: %v", sandbox.Name, err)
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
}

// collectArtifacts collects artifacts from the sandbox
func (c *ActionsController) collectArtifacts(action *models.Action, run *models.ActionRun, sandbox *services.Sandbox) {
	if action.ArtifactPaths == "" {
		return
	}

	// Parse artifact paths (comma-separated)
	paths := strings.Split(action.ArtifactPaths, ",")
	for i, path := range paths {
		paths[i] = strings.TrimSpace(path)
	}

	// Extract artifacts from sandbox
	artifacts, err := sandbox.ExtractArtifacts(paths)
	if err != nil {
		log.Printf("Failed to extract artifacts: %v", err)
		return
	}

	// Save each artifact
	for path, content := range artifacts {
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
