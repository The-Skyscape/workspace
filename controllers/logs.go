package controllers

import (
	"encoding/json"
	"net/http"
	"workspace/middleware"

	"github.com/The-Skyscape/devtools/pkg/application"
)

// Logs is a factory function with the prefix and instance
func Logs() (string, *LogsController) {
	return "logs", &LogsController{}
}

// LogsController handles application logging and monitoring
type LogsController struct {
	application.BaseController
}

// Setup registers routes for logs viewing
func (c *LogsController) Setup(app *application.App) {
	c.BaseController.Setup(app)

	// Logs viewing page (admin only)
	http.Handle("GET /logs", app.Serve("logs.html", AdminOnly()))

	// API endpoints for log data (admin only)
	http.Handle("GET /api/logs/recent", app.ProtectFunc(c.getRecentLogs, AdminOnly()))
	http.Handle("GET /api/logs/stats", app.ProtectFunc(c.getLogStats, AdminOnly()))
}

// Handle prepares controller for request
func (c LogsController) Handle(req *http.Request) application.Controller {
	c.Request = req
	return &c
}

// GetRecentLogs returns recent log entries for templates
func (c *LogsController) GetRecentLogs(limit int) []middleware.LogEntry {
	return middleware.AppLogger.GetRecentLogs(limit)
}

// GetLogStats returns log statistics for templates
func (c *LogsController) GetLogStats() map[string]any {
	return middleware.AppLogger.GetLogStats()
}

// API handlers

func (c *LogsController) getRecentLogs(w http.ResponseWriter, r *http.Request) {
	c.SetRequest(r)
	// Use pagination helper for limit
	pagination := c.Pagination(100) // default 100 items
	limit := pagination.Limit

	// Cap at maximum 1000 for safety
	if limit > 1000 {
		limit = 1000
	}

	logs := middleware.AppLogger.GetRecentLogs(limit)

	// Return as JSON
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

func (c *LogsController) getLogStats(w http.ResponseWriter, r *http.Request) {
	c.SetRequest(r)
	stats := middleware.AppLogger.GetLogStats()

	// Add rate limiting stats
	rateLimitController := c.Use("ratelimit").(*RateLimitController)
	stats["rate_limiting"] = rateLimitController.GetRateLimitStats()

	// Return as JSON
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}
