package models

import (
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
)

// Worker represents an AI worker container instance
type Worker struct {
	application.Model
	Name          string    // Worker display name
	Description   string    // Worker description/purpose
	AIModel       string    // AI model to use (e.g., "llama3.2", "mistral")
	ContainerID   string    // Docker container name
	Port          int       // Port the worker is running on
	Status        string    // starting, running, stopped, error
	ErrorMessage  string    // Error details if status is error
	UserID        string    // Owner of the worker
	CreatedAt     time.Time
	LastActiveAt  time.Time
}

// Table returns the database table name
func (*Worker) Table() string { return "workers" }

// Status constants
const (
	WorkerStatusStarting = "starting"
	WorkerStatusRunning  = "running"
	WorkerStatusStopped  = "stopped"
	WorkerStatusError    = "error"
)

// IsActive checks if the worker is in an active state
func (w *Worker) IsActive() bool {
	return w.Status == WorkerStatusRunning || w.Status == WorkerStatusStarting
}

// MarkActive updates the last active timestamp
func (w *Worker) MarkActive() {
	w.LastActiveAt = time.Now()
	Workers.Update(w)
}

// GetSessions returns all sessions for this worker
func (w *Worker) GetSessions() ([]*WorkerSession, error) {
	return WorkerSessions.Search("WHERE WorkerID = ? ORDER BY LastActiveAt DESC", w.ID)
}

// GetActiveSessions returns active sessions for this worker
func (w *Worker) GetActiveSessions() ([]*WorkerSession, error) {
	// Consider sessions active if used in last 30 minutes
	cutoff := time.Now().Add(-30 * time.Minute)
	return WorkerSessions.Search("WHERE WorkerID = ? AND LastActiveAt > ? ORDER BY LastActiveAt DESC", 
		w.ID, cutoff)
}