package controllers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"workspace/models"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
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
	auth := app.Use("auth").(*authentication.Controller)

	// Actions
	http.Handle("GET /repos/{id}/actions", app.Serve("repo-actions.html", auth.Required))
	// Main action view redirects to info tab
	http.Handle("GET /repos/{id}/actions/{actionID}", app.ProtectFunc(c.redirectToInfo, auth.Required))
	// Tab-specific routes
	http.Handle("GET /repos/{id}/actions/{actionID}/info", app.Serve("repo-action-info.html", auth.Required))
	http.Handle("GET /repos/{id}/actions/{actionID}/command", app.Serve("repo-action-command.html", auth.Required))
	http.Handle("GET /repos/{id}/actions/{actionID}/logs", app.Serve("repo-action-logs.html", auth.Required))
	http.Handle("GET /repos/{id}/actions/{actionID}/history", app.Serve("repo-action-history.html", auth.Required))
	http.Handle("GET /repos/{id}/actions/{actionID}/artifacts", app.Serve("repo-action-artifacts.html", auth.Required))
	// HTMX partials for dynamic updates
	http.Handle("GET /repos/{id}/actions/{actionID}/logs-partial", app.ProtectFunc(c.getActionLogs, auth.Required))
	http.Handle("GET /repos/{id}/actions/{actionID}/artifacts-partial", app.ProtectFunc(c.getActionArtifacts, auth.Required))
	// Action operations
	http.Handle("POST /repos/{id}/actions/create", app.ProtectFunc(c.createAction, auth.Required))
	http.Handle("POST /repos/{id}/actions/{actionID}/run", app.ProtectFunc(c.runAction, auth.Required))
	http.Handle("POST /repos/{id}/actions/{actionID}/disable", app.ProtectFunc(c.disableAction, auth.Required))
	http.Handle("POST /repos/{id}/actions/{actionID}/enable", app.ProtectFunc(c.enableAction, auth.Required))
	http.Handle("GET /repos/{id}/actions/{actionID}/artifacts/{artifactID}/download", app.ProtectFunc(c.downloadArtifact, auth.Required))
}

// RepoActions returns actions for the current repository
func (c *ActionsController) RepoActions() ([]*models.Action, error) {
	repo, err := c.getCurrentRepo()
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

// getCurrentRepo helper to get current repository via repos controller
func (c *ActionsController) getCurrentRepo() (*models.Repository, error) {
	reposController := c.Use("repos").(*ReposController)
	return reposController.CurrentRepo()
}

// RepoBranches returns branches for the current repository via repos controller
func (c *ActionsController) RepoBranches() ([]*models.Branch, error) {
	reposController := c.Use("repos").(*ReposController)
	return reposController.RepoBranches()
}

// redirectToInfo redirects from base action URL to info tab
func (c *ActionsController) redirectToInfo(w http.ResponseWriter, r *http.Request) {
	repoID := r.PathValue("id")
	actionID := r.PathValue("actionID")
	c.Redirect(w, r, fmt.Sprintf("/repos/%s/actions/%s/info", repoID, actionID))
}

// createAction handles action creation
func (c *ActionsController) createAction(w http.ResponseWriter, r *http.Request) {
	// Use shared middleware for permission checking
	if !RepoWriteRequired()(c.App, w, r) {
		return
	}

	auth := c.Use("auth").(*authentication.Controller)
	user, _, _ := auth.Authenticate(r)

	repoID := r.PathValue("id")
	if repoID == "" {
		c.RenderErrorMsg(w, r, "repository ID required")
		return
	}

	// Validate required fields
	title := strings.TrimSpace(r.FormValue("title"))
	description := strings.TrimSpace(r.FormValue("description"))
	actionType := strings.TrimSpace(r.FormValue("type"))
	branch := strings.TrimSpace(r.FormValue("branch"))
	command := strings.TrimSpace(r.FormValue("command"))
	artifactPaths := strings.TrimSpace(r.FormValue("artifact_paths"))

	if title == "" || actionType == "" || command == "" {
		c.RenderErrorMsg(w, r, "title, type, and command are required")
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
	// Use shared middleware for permission checking
	if !RepoWriteRequired()(c.App, w, r) {
		return
	}

	auth := c.Use("auth").(*authentication.Controller)
	user, _, _ := auth.Authenticate(r)

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
	err = action.ExecuteManually()
	if err != nil {
		c.RenderError(w, r, fmt.Errorf("failed to run action: %w", err))
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
	// Use shared middleware for permission checking
	if !RepoAdminRequired()(c.App, w, r) {
		return
	}

	auth := c.Use("auth").(*authentication.Controller)
	user, _, _ := auth.Authenticate(r)

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
	// Use shared middleware for permission checking
	if !RepoAdminRequired()(c.App, w, r) {
		return
	}

	auth := c.Use("auth").(*authentication.Controller)
	user, _, _ := auth.Authenticate(r)

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
	// Use shared middleware for permission checking
	if !RepoReadRequired()(c.App, w, r) {
		return
	}

	auth := c.Use("auth").(*authentication.Controller)
	user, _, _ := auth.Authenticate(r)

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
	// Use shared middleware for permission checking
	if !RepoReadRequired()(c.App, w, r) {
		return
	}

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
	// Use shared middleware for permission checking
	if !RepoReadRequired()(c.App, w, r) {
		return
	}

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