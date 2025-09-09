package controllers

import (
	"encoding/json"
	"net/http"
	"runtime"
	"time"

	"workspace/models"
	"workspace/services"

	"github.com/The-Skyscape/devtools/pkg/application"
)

// Health controller for system health checks
func Health() (string, *HealthController) {
	return "health", &HealthController{}
}

type HealthController struct {
	application.BaseController
}

func (c *HealthController) Setup(app *application.App) {
	c.BaseController.Setup(app)

	// Public health endpoint (no auth required)
	http.HandleFunc("GET /health", c.healthCheck)
	http.HandleFunc("GET /health/detailed", c.detailedHealthCheck)
}

func (c *HealthController) Handle(req *http.Request) application.Controller {
	c.Request = req
	return c
}

// HealthStatus represents the health check response
type HealthStatus struct {
	Status    string            `json:"status"`
	Timestamp time.Time         `json:"timestamp"`
	Checks    map[string]string `json:"checks,omitempty"`
	Version   string            `json:"version"`
	Uptime    string            `json:"uptime"`
}

var startTime = time.Now()

// healthCheck provides a simple health check endpoint
func (c *HealthController) healthCheck(w http.ResponseWriter, r *http.Request) {
	c.SetRequest(r)
		status := HealthStatus{
		Status:    "healthy",
		Timestamp: time.Now(),
		Version:   "1.0.0",
		Uptime:    time.Since(startTime).Round(time.Second).String(),
	}

	// Check database connection by trying to get settings
	if _, err := models.GetSettings(); err != nil {
		status.Status = "unhealthy"
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// detailedHealthCheck provides detailed health information
func (c *HealthController) detailedHealthCheck(w http.ResponseWriter, r *http.Request) {
	c.SetRequest(r)
		checks := make(map[string]string)
	status := "healthy"

	// Database check
	if _, err := models.GetSettings(); err != nil {
		checks["database"] = "unhealthy: " + err.Error()
		status = "unhealthy"
	} else {
		checks["database"] = "healthy"
	}

	// Docker check
	if services.IsDockerAvailable() {
		checks["docker"] = "healthy"
	} else {
		checks["docker"] = "unavailable"
	}

	// Coder service check
	if services.Coder.IsRunning() {
		checks["coder_service"] = "running"
	} else {
		checks["coder_service"] = "stopped"
	}

	// Memory check
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	allocMB := m.Alloc / 1024 / 1024
	if allocMB > 1024 { // Warning if over 1GB
		checks["memory"] = "warning: high usage"
	} else {
		checks["memory"] = "healthy"
	}

	// Goroutines check
	numGoroutines := runtime.NumGoroutine()
	if numGoroutines > 1000 {
		checks["goroutines"] = "warning: high count"
	} else {
		checks["goroutines"] = "healthy"
	}

	healthStatus := HealthStatus{
		Status:    status,
		Timestamp: time.Now(),
		Checks:    checks,
		Version:   "1.0.0",
		Uptime:    time.Since(startTime).Round(time.Second).String(),
	}

	w.Header().Set("Content-Type", "application/json")
	if status == "unhealthy" {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(healthStatus)
}
