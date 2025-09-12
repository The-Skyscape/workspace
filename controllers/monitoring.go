package controllers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/monitoring"
)

// MonitoringController handles system resource monitoring
type MonitoringController struct {
	application.BaseController
	collector *monitoring.Collector

	// Container monitoring state
	containers              []monitoring.ContainerStats
	containersMu            *sync.RWMutex
	containerUpdateInterval time.Duration
	stopContainerMonitor    chan struct{}
}

// Monitoring is the factory function for the monitoring controller
func Monitoring() (string, *MonitoringController) {
	return "monitoring", &MonitoringController{
		collector:               monitoring.NewCollector(false, 100), // Don't include containers in main collector
		containersMu:            &sync.RWMutex{},
		containerUpdateInterval: 15 * time.Second,
		stopContainerMonitor:    make(chan struct{}),
	}
}

// Setup registers monitoring routes
func (m *MonitoringController) Setup(app *application.App) {
	m.BaseController.Setup(app)
	auth := app.Use("auth").(*AuthController)

	// Start collecting statistics on startup
	m.collector.Start()

	// Start background container monitoring
	m.startContainerMonitor()

	// Create admin-only access check that redirects to profile
	adminRequired := func(app *application.App, w http.ResponseWriter, r *http.Request) bool {
		user, _, err := auth.Authenticate(r)
		if err != nil {
			// Redirect to signin if not authenticated
			m.Redirect(w, r, "/signin")
			return false
		}
		if !user.IsAdmin {
			// Non-admins get redirected to profile page
			m.Redirect(w, r, "/settings/profile")
			return false
		}
		return true
	}

	// Live monitoring dashboard (now part of settings, admin only)
	http.Handle("GET /settings/monitoring", app.Serve("settings-monitoring.html", adminRequired))

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
func (m MonitoringController) Handle(req *http.Request) application.Controller {
	m.Request = req
	return &m
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
		m.Render(w, r, "monitoring-error.html", map[string]any{
			"Error": err.Error(),
		})
		return
	}

	// Return HTML partial for HTMX
	m.Render(w, r, "monitoring-stats.html", map[string]any{
		"Stats":  stats,
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

	m.Render(w, r, "monitoring-cpu.html", map[string]any{
		"CPU":  stats.CPU,
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

	m.Render(w, r, "monitoring-memory.html", map[string]any{
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

	m.Render(w, r, "monitoring-disk.html", map[string]any{
		"Disk": stats.Disk,
	})
}

// getContainersPartial returns container stats as HTML partial
func (m *MonitoringController) getContainersPartial(w http.ResponseWriter, r *http.Request) {
	// Use cached container data for instant response
	containers := m.GetContainers()

	m.Render(w, r, "monitoring-containers.html", containers)
}

// getAlertsPartial returns alerts as HTML partial
func (m *MonitoringController) getAlertsPartial(w http.ResponseWriter, r *http.Request) {
	alerts := m.collector.CheckAlerts()

	m.Render(w, r, "monitoring-alerts.html", map[string]any{
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

// GetContainers returns cached container stats for templates
func (m *MonitoringController) GetContainers() []monitoring.ContainerStats {
	m.containersMu.RLock()
	defer m.containersMu.RUnlock()

	// Return a copy to avoid race conditions
	result := make([]monitoring.ContainerStats, len(m.containers))
	copy(result, m.containers)
	return result
}

// startContainerMonitor starts the background container monitoring goroutine
func (m *MonitoringController) startContainerMonitor() {
	go func() {
		// Initial update
		m.updateContainers()

		ticker := time.NewTicker(m.containerUpdateInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				m.updateContainers()
			case <-m.stopContainerMonitor:
				return
			}
		}
	}()
}

// updateContainers fetches Docker container stats and updates cached state
func (m *MonitoringController) updateContainers() {
	// Check if Docker is available
	if _, err := exec.LookPath("docker"); err != nil {
		log.Println("Docker not found, skipping container monitoring")
		return
	}

	// Use longer timeout for Docker operations
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get container stats using docker stats
	cmd := exec.CommandContext(ctx, "docker", "stats", "--no-stream", "--format",
		"{{.Container}}|{{.Name}}|{{.CPUPerc}}|{{.MemUsage}}|{{.MemPerc}}|{{.NetIO}}|{{.BlockIO}}")

	output, err := cmd.Output()
	if err != nil {
		log.Printf("Failed to get Docker stats: %v", err)
		return
	}

	var containers []monitoring.ContainerStats
	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) < 7 {
			continue
		}

		// Parse CPU percentage
		cpuStr := strings.TrimSuffix(parts[2], "%")
		cpuPercent, _ := strconv.ParseFloat(cpuStr, 64)

		// Parse memory percentage
		memStr := strings.TrimSuffix(parts[4], "%")
		memPercent, _ := strconv.ParseFloat(memStr, 64)

		// Parse memory usage (extract bytes from string like "1.5GiB / 2GiB")
		memUsageParts := strings.Split(parts[3], " / ")
		memUsage := uint64(0)
		if len(memUsageParts) > 0 {
			memUsage = parseMemoryString(memUsageParts[0])
		}

		// Determine status (simplified - in real implementation would use docker ps)
		status := "running"

		container := monitoring.ContainerStats{
			ID:         parts[0][:12], // First 12 chars of container ID
			Name:       parts[1],
			Status:     status,
			CPUPercent: cpuPercent,
			MemUsage:   memUsage,
			MemPercent: memPercent,
			NetIO:      parts[5],
			BlockIO:    parts[6],
		}

		containers = append(containers, container)
	}

	// Update cached state
	m.containersMu.Lock()
	m.containers = containers
	m.containersMu.Unlock()
}

// parseMemoryString converts memory strings like "1.5GiB" to bytes
func parseMemoryString(s string) uint64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	// Remove unit and parse number
	var value float64
	var unit string

	for i, r := range s {
		if (r < '0' || r > '9') && r != '.' {
			value, _ = strconv.ParseFloat(s[:i], 64)
			unit = s[i:]
			break
		}
	}

	// Convert to bytes based on unit
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "b":
		return uint64(value)
	case "kib", "kb":
		return uint64(value * 1024)
	case "mib", "mb":
		return uint64(value * 1024 * 1024)
	case "gib", "gb":
		return uint64(value * 1024 * 1024 * 1024)
	default:
		return uint64(value)
	}
}
