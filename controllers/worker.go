package controllers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"workspace/internal/ai"
	"workspace/models"
)

// WorkerController handles Claude AI integration
type WorkerController struct {
	application.BaseController
}

// Worker returns the controller factory
func Worker() (string, *WorkerController) {
	return "worker", &WorkerController{}
}

// Setup initializes the Worker controller
func (c *WorkerController) Setup(app *application.App) {
	c.App = app
	auth := app.Use("auth").(*authentication.Controller)

	// Configuration endpoints
	http.Handle("POST /worker/configure", app.ProtectFunc(c.configure, auth.Required))
	http.Handle("DELETE /worker/configure", app.ProtectFunc(c.removeConfiguration, auth.Required))
	http.Handle("GET /worker/status", app.ProtectFunc(c.getStatus, auth.Required))
	http.Handle("GET /worker/stats", app.ProtectFunc(c.getStats, auth.Required))

	// Worker endpoints
	http.Handle("GET /worker/panel/content", app.ProtectFunc(c.getPanelContent, auth.Required))
	http.Handle("POST /worker/workers", app.ProtectFunc(c.createWorker, auth.Required))
	http.Handle("DELETE /worker/workers/{id}", app.ProtectFunc(c.deleteWorker, auth.Required))
	http.Handle("GET /worker/workers/{id}/chat", app.ProtectFunc(c.getWorkerChat, auth.Required))
	http.Handle("GET /worker/workers/{id}/history", app.ProtectFunc(c.getChatHistory, auth.Required))
	http.Handle("POST /worker/workers/{id}/message", app.ProtectFunc(c.sendMessage, auth.Required))
	http.Handle("GET /worker/workers/{id}/stream", app.ProtectFunc(c.streamResponse, auth.Required))

	// Panel UI endpoint
	http.Handle("GET /worker/panel", app.Serve("worker-panel.html", auth.Required))
}

// Handle prepares the controller for request handling
func (c WorkerController) Handle(req *http.Request) application.Controller {
	c.Request = req
	return &c
}

// IsConfigured checks if Claude is configured for templates
func (c *WorkerController) IsConfigured() bool {
	authManager := ai.NewAuthManager(models.Secrets)
	return authManager.IsConfigured()
}

// GetStats returns usage statistics for templates
func (c *WorkerController) GetStats() *models.WorkerUsage {
	stats, _ := models.GetUsageStats()
	return stats
}

// AllWorkers returns all workers for the current user
func (c *WorkerController) AllWorkers() []*models.AIWorker {
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
func (c *WorkerController) GetWorkerRepos(workerID string) []*models.Repository {
	repos, _ := models.GetRepositoriesForWorker(workerID)
	return repos
}

// configure handles Claude API configuration
func (c *WorkerController) configure(w http.ResponseWriter, r *http.Request) {
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

	apiKey := r.FormValue("worker_api_key")
	if apiKey == "" {
		c.RenderErrorMsg(w, r, "API key is required")
		return
	}

	// Store API key
	authManager := ai.NewAuthManager(models.Secrets)
	if err := authManager.SetAPIKey(apiKey); err != nil {
		log.Printf("Worker: Failed to store API key: %v", err)
		c.RenderErrorMsg(w, r, fmt.Sprintf("Failed to configure Worker service: %v", err))
		return
	}

	// Update settings to indicate Claude is enabled
	settings, _ := models.GetSettings()
	if settings != nil {
		settings.HasWorkerIntegration = true
		models.GlobalSettings.Update(settings)
	}

	// Return success message
	type AlertData struct {
		Message string
	}
	c.Render(w, r, "success-alert.html", &AlertData{
		Message: "AI Worker service configured successfully",
	})
}

// removeConfiguration removes Claude configuration
func (c *WorkerController) removeConfiguration(w http.ResponseWriter, r *http.Request) {
	// Check admin permission
	user, _, err := c.App.Use("auth").(*authentication.Controller).Authenticate(r)
	if err != nil || !user.IsAdmin {
		c.RenderErrorMsg(w, r, "admin permission required")
		return
	}

	authManager := ai.NewAuthManager(models.Secrets)
	if err := authManager.RemoveConfiguration(); err != nil {
		c.RenderErrorMsg(w, r, fmt.Sprintf("Failed to remove configuration: %v", err))
		return
	}

	// Update settings
	settings, _ := models.GetSettings()
	if settings != nil {
		settings.HasWorkerIntegration = false
		models.GlobalSettings.Update(settings)
	}

	type AlertData struct {
		Message string
	}
	c.Render(w, r, "success-alert.html", &AlertData{
		Message: "AI Worker configuration removed",
	})
}

// getStatus returns Claude configuration status
func (c *WorkerController) getStatus(w http.ResponseWriter, r *http.Request) {
	authManager := ai.NewAuthManager(models.Secrets)
	status := map[string]interface{}{
		"configured": authManager.IsConfigured(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// getStats returns usage statistics
func (c *WorkerController) getStats(w http.ResponseWriter, r *http.Request) {
	stats, err := models.GetUsageStats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// getPanelContent returns the appropriate panel content based on worker state
func (c *WorkerController) getPanelContent(w http.ResponseWriter, r *http.Request) {
	// Always show the workers list view which handles all states
	c.Render(w, r, "worker-list.html", nil)
}

