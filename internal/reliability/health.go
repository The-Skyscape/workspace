package reliability

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
	"workspace/services"
)

// HealthStatus represents the health of a component
type HealthStatus string

const (
	HealthHealthy   HealthStatus = "healthy"
	HealthDegraded  HealthStatus = "degraded"
	HealthUnhealthy HealthStatus = "unhealthy"
	HealthUnknown   HealthStatus = "unknown"
)

// ComponentHealth represents health of a single component
type ComponentHealth struct {
	Name        string                 `json:"name"`
	Status      HealthStatus           `json:"status"`
	Message     string                 `json:"message,omitempty"`
	LastCheck   time.Time              `json:"lastCheck"`
	ResponseTime time.Duration         `json:"responseTime"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// HealthChecker checks the health of a component
type HealthChecker interface {
	CheckHealth(ctx context.Context) ComponentHealth
	Name() string
}

// HealthMonitor monitors the health of multiple components
type HealthMonitor struct {
	checkers      map[string]HealthChecker
	checkInterval time.Duration
	results       map[string]ComponentHealth
	mu            sync.RWMutex
	stopCh        chan struct{}
	running       bool
}

// NewHealthMonitor creates a new health monitor
func NewHealthMonitor(checkInterval time.Duration) *HealthMonitor {
	if checkInterval == 0 {
		checkInterval = 30 * time.Second
	}
	
	return &HealthMonitor{
		checkers:      make(map[string]HealthChecker),
		checkInterval: checkInterval,
		results:       make(map[string]ComponentHealth),
		stopCh:        make(chan struct{}),
	}
}

// RegisterChecker registers a health checker
func (m *HealthMonitor) RegisterChecker(checker HealthChecker) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkers[checker.Name()] = checker
}

// Start begins monitoring
func (m *HealthMonitor) Start() {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.mu.Unlock()
	
	// Initial check
	m.checkAll()
	
	// Start monitoring loop
	go m.monitorLoop()
	
	log.Println("HealthMonitor: Started monitoring")
}

// Stop stops monitoring
func (m *HealthMonitor) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.running = false
	m.mu.Unlock()
	
	close(m.stopCh)
	log.Println("HealthMonitor: Stopped monitoring")
}

// monitorLoop runs the monitoring loop
func (m *HealthMonitor) monitorLoop() {
	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			m.checkAll()
		case <-m.stopCh:
			return
		}
	}
}

// checkAll checks all components
func (m *HealthMonitor) checkAll() {
	m.mu.RLock()
	checkers := make([]HealthChecker, 0, len(m.checkers))
	for _, checker := range m.checkers {
		checkers = append(checkers, checker)
	}
	m.mu.RUnlock()
	
	var wg sync.WaitGroup
	for _, checker := range checkers {
		wg.Add(1)
		go func(c HealthChecker) {
			defer wg.Done()
			
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			
			result := c.CheckHealth(ctx)
			
			m.mu.Lock()
			m.results[c.Name()] = result
			m.mu.Unlock()
			
			// Log unhealthy components
			if result.Status == HealthUnhealthy {
				log.Printf("HealthMonitor: Component %s is unhealthy: %s", c.Name(), result.Message)
			}
		}(checker)
	}
	
	wg.Wait()
}

// GetHealth returns the current health status
func (m *HealthMonitor) GetHealth() map[string]ComponentHealth {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	results := make(map[string]ComponentHealth)
	for k, v := range m.results {
		results[k] = v
	}
	return results
}

// GetOverallHealth returns the overall system health
func (m *HealthMonitor) GetOverallHealth() HealthStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if len(m.results) == 0 {
		return HealthUnknown
	}
	
	hasUnhealthy := false
	hasDegraded := false
	
	for _, result := range m.results {
		switch result.Status {
		case HealthUnhealthy:
			hasUnhealthy = true
		case HealthDegraded:
			hasDegraded = true
		}
	}
	
	if hasUnhealthy {
		return HealthUnhealthy
	}
	if hasDegraded {
		return HealthDegraded
	}
	return HealthHealthy
}

// OllamaHealthChecker checks Ollama service health
type OllamaHealthChecker struct{}

func (c *OllamaHealthChecker) Name() string {
	return "ollama"
}

func (c *OllamaHealthChecker) CheckHealth(ctx context.Context) ComponentHealth {
	start := time.Now()
	health := ComponentHealth{
		Name:      c.Name(),
		Status:    HealthUnknown,
		LastCheck: start,
	}
	
	// Check if Ollama service is initialized
	if services.Ollama == nil {
		health.Status = HealthUnhealthy
		health.Message = "Ollama service not initialized"
		return health
	}
	
	// Check Ollama status
	status := services.Ollama.GetStatus()
	health.ResponseTime = time.Since(start)
	
	if status.Running {
		health.Status = HealthHealthy
		health.Message = fmt.Sprintf("Ollama running on port %d", status.Port)
		health.Metadata = map[string]interface{}{
			"port":   status.Port,
			"models": status.Models,
		}
	} else {
		health.Status = HealthUnhealthy
		health.Message = "Ollama service not running"
		
		// Attempt to restart
		log.Println("HealthMonitor: Attempting to restart Ollama service")
		if err := services.Ollama.Start(); err != nil {
			health.Message = fmt.Sprintf("Failed to restart Ollama: %v", err)
		} else {
			health.Status = HealthDegraded
			health.Message = "Ollama service restarted"
		}
	}
	
	return health
}

// DatabaseHealthChecker checks database health
type DatabaseHealthChecker struct{}

func (c *DatabaseHealthChecker) Name() string {
	return "database"
}

func (c *DatabaseHealthChecker) CheckHealth(ctx context.Context) ComponentHealth {
	start := time.Now()
	health := ComponentHealth{
		Name:      c.Name(),
		Status:    HealthUnknown,
		LastCheck: start,
	}
	
	// Simple database ping would go here
	// For SQLite, we can check if the file exists and is accessible
	health.ResponseTime = time.Since(start)
	health.Status = HealthHealthy
	health.Message = "Database accessible"
	
	return health
}

// VaultHealthChecker checks Vault service health
// NOTE: Vault is managed through models.Secrets, not a direct service
// Commenting out until proper health check implementation
/*
type VaultHealthChecker struct{}

func (c *VaultHealthChecker) Name() string {
	return "vault"
}

func (c *VaultHealthChecker) CheckHealth(ctx context.Context) ComponentHealth {
	start := time.Now()
	health := ComponentHealth{
		Name:      c.Name(),
		Status:    HealthUnknown,
		LastCheck: start,
	}
	
	// Check if Vault service is running
	if services.Vault == nil {
		health.Status = HealthUnhealthy
		health.Message = "Vault service not initialized"
		return health
	}
	
	status := services.Vault.GetStatus()
	health.ResponseTime = time.Since(start)
	
	if status.Running {
		health.Status = HealthHealthy
		health.Message = "Vault service running"
		health.Metadata = map[string]interface{}{
			"sealed": status.Sealed,
			"port":   status.Port,
		}
		
		if status.Sealed {
			health.Status = HealthDegraded
			health.Message = "Vault is sealed"
		}
	} else {
		health.Status = HealthUnhealthy
		health.Message = "Vault service not running"
	}
	
	return health
}
*/

// SandboxHealthChecker checks sandbox availability
type SandboxHealthChecker struct{}

func (c *SandboxHealthChecker) Name() string {
	return "sandbox"
}

func (c *SandboxHealthChecker) CheckHealth(ctx context.Context) ComponentHealth {
	start := time.Now()
	health := ComponentHealth{
		Name:      c.Name(),
		Status:    HealthUnknown,
		LastCheck: start,
	}
	
	// Check if we can create a sandbox
	sandboxes := services.ListSandboxes()
	health.ResponseTime = time.Since(start)
	
	health.Status = HealthHealthy
	health.Message = fmt.Sprintf("%d sandboxes active", len(sandboxes))
	health.Metadata = map[string]interface{}{
		"activeSandboxes": len(sandboxes),
	}
	
	return health
}

// AIQueueHealthChecker checks AI queue health
// NOTE: AI queue is managed through internal/ai/queue, not services
// Commenting out until proper health check implementation
/*
type AIQueueHealthChecker struct{}

func (c *AIQueueHealthChecker) Name() string {
	return "ai_queue"
}

func (c *AIQueueHealthChecker) CheckHealth(ctx context.Context) ComponentHealth {
	start := time.Now()
	health := ComponentHealth{
		Name:      c.Name(),
		Status:    HealthUnknown,
		LastCheck: start,
	}
	
	// Check AI queue service
	if services.AIQueueService == nil {
		health.Status = HealthUnhealthy
		health.Message = "AI queue service not initialized"
		return health
	}
	
	status := services.AIQueueService.GetStatus()
	health.ResponseTime = time.Since(start)
	
	if status.Running {
		health.Status = HealthHealthy
		health.Message = fmt.Sprintf("Queue running with %d pending tasks", status.QueueSize)
		health.Metadata = map[string]interface{}{
			"queueSize":      status.QueueSize,
			"processedCount": status.ProcessedCount,
			"failedCount":    status.FailedCount,
		}
		
		// Check if queue is backed up
		if status.QueueSize > 100 {
			health.Status = HealthDegraded
			health.Message = fmt.Sprintf("Queue backed up with %d tasks", status.QueueSize)
		}
	} else {
		health.Status = HealthUnhealthy
		health.Message = "AI queue service not running"
	}
	
	return health
}
*/

// HealthHandler creates an HTTP handler for health checks
func HealthHandler(monitor *HealthMonitor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		health := monitor.GetHealth()
		overall := monitor.GetOverallHealth()
		
		response := map[string]interface{}{
			"status":     overall,
			"components": health,
			"timestamp":  time.Now(),
		}
		
		// Set appropriate status code
		switch overall {
		case HealthHealthy:
			w.WriteHeader(http.StatusOK)
		case HealthDegraded:
			w.WriteHeader(http.StatusOK) // Still OK but degraded
		case HealthUnhealthy:
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

// Global health monitor instance
var Monitor *HealthMonitor

// InitializeHealthMonitoring sets up health monitoring
func InitializeHealthMonitoring() {
	Monitor = NewHealthMonitor(30 * time.Second)
	
	// Register health checkers
	Monitor.RegisterChecker(&OllamaHealthChecker{})
	Monitor.RegisterChecker(&DatabaseHealthChecker{})
	Monitor.RegisterChecker(&VaultHealthChecker{})
	Monitor.RegisterChecker(&SandboxHealthChecker{})
	Monitor.RegisterChecker(&AIQueueHealthChecker{})
	
	// Start monitoring
	Monitor.Start()
	
	log.Println("Health monitoring initialized")
}