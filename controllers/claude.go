package controllers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"workspace/internal/claude"
	"workspace/models"
)

// ClaudeController handles Claude AI integration
type ClaudeController struct {
	application.BaseController
}

// Claude returns the controller factory
func Claude() (string, *ClaudeController) {
	return "claude", &ClaudeController{}
}

// Setup initializes the Claude controller
func (c *ClaudeController) Setup(app *application.App) {
	c.App = app
	auth := app.Use("auth").(*authentication.Controller)

	// Configuration endpoints
	http.Handle("POST /claude/configure", app.ProtectFunc(c.configure, auth.Required))
	http.Handle("DELETE /claude/configure", app.ProtectFunc(c.removeConfiguration, auth.Required))
	http.Handle("GET /claude/status", app.ProtectFunc(c.getStatus, auth.Required))
	http.Handle("GET /claude/stats", app.ProtectFunc(c.getStats, auth.Required))

	// Worker endpoints
	http.Handle("GET /claude/panel/content", app.ProtectFunc(c.getPanelContent, auth.Required))
	http.Handle("POST /claude/workers", app.ProtectFunc(c.createWorker, auth.Required))
	http.Handle("DELETE /claude/workers/{id}", app.ProtectFunc(c.deleteWorker, auth.Required))
	http.Handle("GET /claude/workers/{id}/chat", app.ProtectFunc(c.getWorkerChat, auth.Required))
	http.Handle("GET /claude/workers/{id}/history", app.ProtectFunc(c.getChatHistory, auth.Required))
	http.Handle("POST /claude/workers/{id}/message", app.ProtectFunc(c.sendMessage, auth.Required))
	http.Handle("GET /claude/workers/{id}/stream", app.ProtectFunc(c.streamResponse, auth.Required))

	// Panel UI endpoint
	http.Handle("GET /claude/panel", app.Serve("claude-panel.html", auth.Required))
}

// Handle prepares the controller for request handling
func (c ClaudeController) Handle(req *http.Request) application.Controller {
	c.Request = req
	return &c
}

// IsConfigured checks if Claude is configured for templates
func (c *ClaudeController) IsConfigured() bool {
	authManager := claude.NewAuthManager(models.Secrets)
	return authManager.IsConfigured()
}

// GetStats returns usage statistics for templates
func (c *ClaudeController) GetStats() *models.ClaudeUsage {
	stats, _ := models.GetUsageStats()
	return stats
}

// CurrentWorker returns the active worker for the current user (deprecated - use AllWorkers)
func (c *ClaudeController) CurrentWorker() *models.AIWorker {
	if c.Request == nil {
		return nil
	}
	user, _, err := c.App.Use("auth").(*authentication.Controller).Authenticate(c.Request)
	if err != nil {
		return nil
	}
	workers, _ := models.AIWorkers.Search("WHERE UserID = ? AND Status != ? ORDER BY LastActiveAt DESC", user.ID, models.WorkerStatusStopped)
	if len(workers) > 0 {
		return workers[0]
	}
	return nil
}

// AllWorkers returns all workers for the current user
func (c *ClaudeController) AllWorkers() []*models.AIWorker {
	if c.Request == nil {
		return nil
	}
	user, _, err := c.App.Use("auth").(*authentication.Controller).Authenticate(c.Request)
	if err != nil {
		return nil
	}
	workers, _ := models.AIWorkers.Search("WHERE UserID = ? AND Status != ? ORDER BY LastActiveAt DESC", user.ID, models.WorkerStatusStopped)
	return workers
}

// GetWorkerRepos returns repositories for a specific worker
func (c *ClaudeController) GetWorkerRepos(workerID string) []*models.Repository {
	repos, _ := models.GetRepositoriesForWorker(workerID)
	return repos
}

// HasWorker checks if the user has an active worker
func (c *ClaudeController) HasWorker() bool {
	return c.CurrentWorker() != nil
}

// WorkerStatus returns the status of the current worker
func (c *ClaudeController) WorkerStatus() string {
	worker := c.CurrentWorker()
	if worker != nil {
		return worker.Status
	}
	return ""
}

// WorkerRepos returns repositories associated with the current worker
func (c *ClaudeController) WorkerRepos() []*models.Repository {
	worker := c.CurrentWorker()
	if worker == nil {
		return nil
	}
	repos, _ := models.GetRepositoriesForWorker(worker.ID)
	return repos
}

// ChatHistory returns the chat history for the current worker
func (c *ClaudeController) ChatHistory() []*models.ChatMessage {
	worker := c.CurrentWorker()
	if worker == nil {
		return nil
	}
	messages, _ := models.GetConversationHistory(worker.ID)
	return messages
}

// WorkerReposCloned checks if repos are cloned (for progress indicator)
func (c *ClaudeController) WorkerReposCloned() bool {
	// This would check actual sandbox state in production
	// For now, return false during creation
	worker := c.CurrentWorker()
	return worker != nil && worker.Status != models.WorkerStatusCreating
}

// configure handles Claude API configuration
func (c *ClaudeController) configure(w http.ResponseWriter, r *http.Request) {
	// Check admin permission
	user, _, err := c.App.Use("auth").(*authentication.Controller).Authenticate(r)
	if err != nil || !user.IsAdmin {
		c.RenderErrorMsg(w, r, "admin permission required")
		return
	}

	// Parse form data
	if err := r.ParseForm(); err != nil {
		c.RenderErrorMsg(w, r, "invalid form data")
		return
	}

	apiKey := r.FormValue("claude_api_key")
	if apiKey == "" {
		c.RenderErrorMsg(w, r, "API key is required")
		return
	}

	// Store API key
	authManager := claude.NewAuthManager(models.Secrets)
	if err := authManager.SetAPIKey(apiKey); err != nil {
		log.Printf("Claude: Failed to store API key: %v", err)
		c.RenderErrorMsg(w, r, fmt.Sprintf("Failed to configure Claude: %v", err))
		return
	}

	// Update settings to indicate Claude is enabled
	settings, _ := models.GetSettings()
	if settings != nil {
		settings.HasClaudeIntegration = true
		models.GlobalSettings.Update(settings)
	}

	// Return success message
	c.Render(w, r, "success-alert.html", map[string]interface{}{
		"Message": "Claude integration configured successfully",
	})
}

// removeConfiguration removes Claude configuration
func (c *ClaudeController) removeConfiguration(w http.ResponseWriter, r *http.Request) {
	// Check admin permission
	user, _, err := c.App.Use("auth").(*authentication.Controller).Authenticate(r)
	if err != nil || !user.IsAdmin {
		c.RenderErrorMsg(w, r, "admin permission required")
		return
	}

	authManager := claude.NewAuthManager(models.Secrets)
	if err := authManager.RemoveConfiguration(); err != nil {
		c.RenderErrorMsg(w, r, fmt.Sprintf("Failed to remove configuration: %v", err))
		return
	}

	// Update settings
	settings, _ := models.GetSettings()
	if settings != nil {
		settings.HasClaudeIntegration = false
		models.GlobalSettings.Update(settings)
	}

	c.Render(w, r, "success-alert.html", map[string]interface{}{
		"Message": "Claude configuration removed",
	})
}

// getStatus returns Claude configuration status
func (c *ClaudeController) getStatus(w http.ResponseWriter, r *http.Request) {
	authManager := claude.NewAuthManager(models.Secrets)
	status := map[string]interface{}{
		"configured": authManager.IsConfigured(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// getStats returns usage statistics
func (c *ClaudeController) getStats(w http.ResponseWriter, r *http.Request) {
	stats, err := models.GetUsageStats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// getPanelContent returns the appropriate panel content based on worker state
func (c *ClaudeController) getPanelContent(w http.ResponseWriter, r *http.Request) {
	// Always show the workers list view which handles all states
	c.Render(w, r, "claude-workers-list.html", nil)
}

