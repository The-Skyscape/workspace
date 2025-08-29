package models

import (
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
)

// AIWorker represents a persistent Claude AI assistant
type AIWorker struct {
	application.Model
	UserID       string    // Owner of the worker
	Name         string    // Display name for the worker
	Status       string    // "creating", "ready", "error", "stopped"
	SandboxID    string    // Persistent sandbox container ID
	LastActiveAt time.Time // Last time the worker was used
}

// Table returns the database table name
func (*AIWorker) Table() string { return "ai_workers" }

// Status constants
const (
	WorkerStatusCreating = "creating"
	WorkerStatusReady    = "ready"
	WorkerStatusError    = "error"
	WorkerStatusStopped  = "stopped"
)

// IsActive checks if the worker is in an active state
func (w *AIWorker) IsActive() bool {
	return w.Status == WorkerStatusReady || w.Status == WorkerStatusCreating
}

// MarkActive updates the last active timestamp
func (w *AIWorker) MarkActive() {
	w.LastActiveAt = time.Now()
	AIWorkers.Update(w)
}