package controllers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/monitoring"
)

// MonitoringController handles system resource monitoring
type MonitoringController struct {
	application.BaseController
	collector *monitoring.Collector
}

// Monitoring is the factory function for the monitoring controller
func Monitoring() (string, *MonitoringController) {
	return "monitoring", &MonitoringController{
		collector: monitoring.NewCollector(true, 100), // Include containers, keep 100 history items
	}
}

// Setup registers monitoring routes
func (m *MonitoringController) Setup(app *application.App) {
	m.BaseController.Setup(app)
	auth := app.Use("auth").(*authentication.Controller)

	// Start collecting statistics on startup
	m.collector.Start()

	// Live monitoring dashboard (now part of settings)
	http.Handle("GET /settings/monitoring", app.Serve("settings-monitoring.html", auth.Required))
	
	// API endpoints for live updates
	http.Handle("GET /monitoring/stats", app.ProtectFunc(m.getCurrentStats, auth.Required))
	http.Handle("GET /monitoring/stats/live", app.ProtectFunc(m.getLiveStats, auth.Required))
	http.Handle("GET /monitoring/history", app.ProtectFunc(m.getHistory, auth.Required))
	http.Handle("GET /monitoring/alerts", app.ProtectFunc(m.getAlerts, auth.Required))
	http.Handle("GET /monitoring/processes", app.ProtectFunc(m.getTopProcesses, auth.Required))
	
	// HTMX partial updates
	http.Handle("GET /monitoring/partial/cpu", app.ProtectFunc(m.getCPUPartial, auth.Required))
	http.Handle("GET /monitoring/partial/memory", app.ProtectFunc(m.getMemoryPartial, auth.Required))
	http.Handle("GET /monitoring/partial/disk", app.ProtectFunc(m.getDiskPartial, auth.Required))
	http.Handle("GET /monitoring/partial/containers", app.ProtectFunc(m.getContainersPartial, auth.Required))
	http.Handle("GET /monitoring/partial/alerts", app.ProtectFunc(m.getAlertsPartial, auth.Required))
}

// Handle prepares the controller for each request
func (m *MonitoringController) Handle(req *http.Request) application.Controller {
	return m
}

// GetCurrentStats returns current system statistics for templates
func (m *MonitoringController) GetCurrentStats() *monitoring.SystemStats {
	stats, _ := m.collector.GetCurrent()
	return stats
}

// GetAlertCount returns the number of current alerts
func (m *MonitoringController) GetAlertCount() int {
	return len(m.collector.CheckAlerts())
}

// getCurrentStats returns current statistics as JSON
func (m *MonitoringController) getCurrentStats(w http.ResponseWriter, r *http.Request) {
	stats, err := m.collector.GetCurrent()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// getLiveStats returns live statistics for HTMX polling
func (m *MonitoringController) getLiveStats(w http.ResponseWriter, r *http.Request) {
	stats, err := m.collector.GetCurrent()
	if err != nil {
		m.Render(w, r, "monitoring-error.html", map[string]interface{}{
			"Error": err.Error(),
		})
		return
	}

	// Return HTML partial for HTMX
	m.Render(w, r, "monitoring-stats.html", map[string]interface{}{
		"Stats": stats,
		"Alerts": m.collector.CheckAlerts(),
	})
}

// getHistory returns statistics history
func (m *MonitoringController) getHistory(w http.ResponseWriter, r *http.Request) {
	// Parse duration parameter
	durationStr := r.URL.Query().Get("duration")
	duration := 1 * time.Hour // Default to 1 hour
	
	if durationStr != "" {
		if d, err := time.ParseDuration(durationStr); err == nil {
			duration = d
		}
	}

	history := m.collector.GetHistorySince(time.Now().Add(-duration))
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(history)
}

// getAlerts returns current resource alerts
func (m *MonitoringController) getAlerts(w http.ResponseWriter, r *http.Request) {
	alerts := m.collector.CheckAlerts()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(alerts)
}

// getTopProcesses returns top processes by CPU or memory
func (m *MonitoringController) getTopProcesses(w http.ResponseWriter, r *http.Request) {
	sortBy := r.URL.Query().Get("sort")
	if sortBy == "" {
		sortBy = "cpu"
	}
	
	countStr := r.URL.Query().Get("count")
	count := 10
	if c, err := strconv.Atoi(countStr); err == nil && c > 0 {
		count = c
	}

	monitor := monitoring.NewMonitor(false)
	processes, err := monitor.GetTopProcesses(count, sortBy)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(processes)
}

// getCPUPartial returns CPU stats as HTML partial
func (m *MonitoringController) getCPUPartial(w http.ResponseWriter, r *http.Request) {
	stats, err := m.collector.GetCurrent()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	m.Render(w, r, "monitoring-cpu.html", map[string]interface{}{
		"CPU": stats.CPU,
		"Load": stats.LoadAverage,
	})
}

// getMemoryPartial returns memory stats as HTML partial
func (m *MonitoringController) getMemoryPartial(w http.ResponseWriter, r *http.Request) {
	stats, err := m.collector.GetCurrent()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	m.Render(w, r, "monitoring-memory.html", map[string]interface{}{
		"Memory": stats.Memory,
	})
}

// getDiskPartial returns disk stats as HTML partial
func (m *MonitoringController) getDiskPartial(w http.ResponseWriter, r *http.Request) {
	stats, err := m.collector.GetCurrent()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	m.Render(w, r, "monitoring-disk.html", map[string]interface{}{
		"Disk": stats.Disk,
	})
}

// getContainersPartial returns container stats as HTML partial
func (m *MonitoringController) getContainersPartial(w http.ResponseWriter, r *http.Request) {
	stats, err := m.collector.GetCurrent()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	m.Render(w, r, "monitoring-containers.html", map[string]interface{}{
		"Containers": stats.Containers,
	})
}

// getAlertsPartial returns alerts as HTML partial
func (m *MonitoringController) getAlertsPartial(w http.ResponseWriter, r *http.Request) {
	alerts := m.collector.CheckAlerts()
	
	m.Render(w, r, "monitoring-alerts.html", map[string]interface{}{
		"Alerts": alerts,
	})
}

// FormatBytes formats bytes for display
func (m *MonitoringController) FormatBytes(bytes uint64) string {
	return monitoring.FormatBytes(bytes)
}

// FormatPercent formats percentage for display
func (m *MonitoringController) FormatPercent(percent float64) string {
	return monitoring.FormatPercent(percent)
}