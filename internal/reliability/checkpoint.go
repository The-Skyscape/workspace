package reliability

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/The-Skyscape/devtools/pkg/database"
)

// Checkpoint represents a saved state in an operation
type Checkpoint struct {
	ID          string         `json:"id"`
	OperationID string         `json:"operationId"`
	Step        int            `json:"step"`
	TotalSteps  int            `json:"totalSteps"`
	State       map[string]any `json:"state"`
	Timestamp   time.Time      `json:"timestamp"`
	Metadata    map[string]any `json:"metadata"`
}

// CheckpointManager manages checkpoints for operations
type CheckpointManager struct {
	storageDir string
	mu         sync.RWMutex
	active     map[string]*OperationCheckpoint
}

// NewCheckpointManager creates a new checkpoint manager
func NewCheckpointManager() *CheckpointManager {
	storageDir := filepath.Join(database.DataDir(), "checkpoints")
	os.MkdirAll(storageDir, 0755)

	return &CheckpointManager{
		storageDir: storageDir,
		active:     make(map[string]*OperationCheckpoint),
	}
}

// StartOperation begins tracking a new operation
func (m *CheckpointManager) StartOperation(operationID string, totalSteps int) *OperationCheckpoint {
	m.mu.Lock()
	defer m.mu.Unlock()

	op := &OperationCheckpoint{
		ID:          operationID,
		TotalSteps:  totalSteps,
		CurrentStep: 0,
		StartTime:   time.Now(),
		State:       make(map[string]any),
		manager:     m,
	}

	m.active[operationID] = op
	return op
}

// GetOperation retrieves an active operation
func (m *CheckpointManager) GetOperation(operationID string) (*OperationCheckpoint, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	op, exists := m.active[operationID]
	return op, exists
}

// LoadCheckpoint loads a checkpoint from disk
func (m *CheckpointManager) LoadCheckpoint(operationID string) (*Checkpoint, error) {
	filename := filepath.Join(m.storageDir, fmt.Sprintf("%s.json", operationID))

	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("checkpoint not found for operation %s", operationID)
		}
		return nil, err
	}

	var checkpoint Checkpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil, err
	}

	return &checkpoint, nil
}

// SaveCheckpoint saves a checkpoint to disk
func (m *CheckpointManager) SaveCheckpoint(checkpoint *Checkpoint) error {
	filename := filepath.Join(m.storageDir, fmt.Sprintf("%s.json", checkpoint.OperationID))

	data, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return err
	}

	// Write to temp file first, then rename for atomicity
	tempFile := filename + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return err
	}

	return os.Rename(tempFile, filename)
}

// CleanupCheckpoint removes a checkpoint
func (m *CheckpointManager) CleanupCheckpoint(operationID string) error {
	m.mu.Lock()
	delete(m.active, operationID)
	m.mu.Unlock()

	filename := filepath.Join(m.storageDir, fmt.Sprintf("%s.json", operationID))
	return os.Remove(filename)
}

// ListCheckpoints returns all saved checkpoints
func (m *CheckpointManager) ListCheckpoints() ([]*Checkpoint, error) {
	files, err := os.ReadDir(m.storageDir)
	if err != nil {
		return nil, err
	}

	var checkpoints []*Checkpoint
	for _, file := range files {
		if filepath.Ext(file.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(m.storageDir, file.Name()))
		if err != nil {
			log.Printf("Failed to read checkpoint %s: %v", file.Name(), err)
			continue
		}

		var checkpoint Checkpoint
		if err := json.Unmarshal(data, &checkpoint); err != nil {
			log.Printf("Failed to parse checkpoint %s: %v", file.Name(), err)
			continue
		}

		checkpoints = append(checkpoints, &checkpoint)
	}

	return checkpoints, nil
}

// OperationCheckpoint tracks progress of a single operation
type OperationCheckpoint struct {
	ID          string
	TotalSteps  int
	CurrentStep int
	StartTime   time.Time
	State       map[string]any
	Error       error
	mu          sync.RWMutex
	manager     *CheckpointManager
}

// SaveState saves the current state
func (op *OperationCheckpoint) SaveState(state map[string]any) error {
	op.mu.Lock()
	defer op.mu.Unlock()

	// Merge state
	for k, v := range state {
		op.State[k] = v
	}

	checkpoint := &Checkpoint{
		ID:          fmt.Sprintf("%s-%d", op.ID, op.CurrentStep),
		OperationID: op.ID,
		Step:        op.CurrentStep,
		TotalSteps:  op.TotalSteps,
		State:       op.State,
		Timestamp:   time.Now(),
		Metadata: map[string]any{
			"startTime": op.StartTime,
			"duration":  time.Since(op.StartTime).String(),
		},
	}

	return op.manager.SaveCheckpoint(checkpoint)
}

// NextStep advances to the next step
func (op *OperationCheckpoint) NextStep() error {
	op.mu.Lock()
	defer op.mu.Unlock()

	op.CurrentStep++

	// Save checkpoint after each step
	checkpoint := &Checkpoint{
		ID:          fmt.Sprintf("%s-%d", op.ID, op.CurrentStep),
		OperationID: op.ID,
		Step:        op.CurrentStep,
		TotalSteps:  op.TotalSteps,
		State:       op.State,
		Timestamp:   time.Now(),
	}

	return op.manager.SaveCheckpoint(checkpoint)
}

// GetProgress returns the current progress
func (op *OperationCheckpoint) GetProgress() float64 {
	op.mu.RLock()
	defer op.mu.RUnlock()

	if op.TotalSteps == 0 {
		return 0
	}

	return float64(op.CurrentStep) / float64(op.TotalSteps) * 100
}

// Complete marks the operation as complete
func (op *OperationCheckpoint) Complete() error {
	op.mu.Lock()
	defer op.mu.Unlock()

	op.CurrentStep = op.TotalSteps

	// Save final checkpoint
	checkpoint := &Checkpoint{
		ID:          fmt.Sprintf("%s-complete", op.ID),
		OperationID: op.ID,
		Step:        op.CurrentStep,
		TotalSteps:  op.TotalSteps,
		State:       op.State,
		Timestamp:   time.Now(),
		Metadata: map[string]any{
			"completed": true,
			"duration":  time.Since(op.StartTime).String(),
		},
	}

	if err := op.manager.SaveCheckpoint(checkpoint); err != nil {
		return err
	}

	// Clean up after a delay
	go func() {
		time.Sleep(5 * time.Minute)
		op.manager.CleanupCheckpoint(op.ID)
	}()

	return nil
}

// Fail marks the operation as failed
func (op *OperationCheckpoint) Fail(err error) error {
	op.mu.Lock()
	defer op.mu.Unlock()

	op.Error = err

	// Save failure checkpoint
	checkpoint := &Checkpoint{
		ID:          fmt.Sprintf("%s-failed", op.ID),
		OperationID: op.ID,
		Step:        op.CurrentStep,
		TotalSteps:  op.TotalSteps,
		State:       op.State,
		Timestamp:   time.Now(),
		Metadata: map[string]any{
			"failed":   true,
			"error":    err.Error(),
			"duration": time.Since(op.StartTime).String(),
		},
	}

	return op.manager.SaveCheckpoint(checkpoint)
}

// ResumableOperation represents an operation that can be resumed
type ResumableOperation struct {
	ID         string
	Name       string
	TotalSteps int
	Execute    func(ctx context.Context, checkpoint *OperationCheckpoint) error
	Rollback   func(ctx context.Context, checkpoint *OperationCheckpoint) error
}

// Run executes the operation with checkpoint support
func (op *ResumableOperation) Run(ctx context.Context, manager *CheckpointManager) error {
	// Check if we're resuming
	existingCheckpoint, err := manager.LoadCheckpoint(op.ID)

	var checkpoint *OperationCheckpoint

	if err == nil && existingCheckpoint != nil {
		// Resume from checkpoint
		log.Printf("Resuming operation %s from step %d/%d",
			op.ID, existingCheckpoint.Step, existingCheckpoint.TotalSteps)

		checkpoint = &OperationCheckpoint{
			ID:          op.ID,
			TotalSteps:  op.TotalSteps,
			CurrentStep: existingCheckpoint.Step,
			StartTime:   existingCheckpoint.Timestamp,
			State:       existingCheckpoint.State,
			manager:     manager,
		}

		manager.mu.Lock()
		manager.active[op.ID] = checkpoint
		manager.mu.Unlock()
	} else {
		// Start fresh
		checkpoint = manager.StartOperation(op.ID, op.TotalSteps)
	}

	// Execute with recovery
	if err := op.Execute(ctx, checkpoint); err != nil {
		checkpoint.Fail(err)

		// Attempt rollback if provided
		if op.Rollback != nil {
			log.Printf("Rolling back operation %s", op.ID)
			if rbErr := op.Rollback(ctx, checkpoint); rbErr != nil {
				log.Printf("Rollback failed for operation %s: %v", op.ID, rbErr)
			}
		}

		return err
	}

	// Mark complete
	return checkpoint.Complete()
}

// TransactionalOperation wraps operations in a transaction-like pattern
type TransactionalOperation struct {
	Operations []ResumableOperation
}

// Execute runs all operations with rollback on failure
func (t *TransactionalOperation) Execute(ctx context.Context, manager *CheckpointManager) error {
	completed := make([]int, 0)

	for i, op := range t.Operations {
		if err := op.Run(ctx, manager); err != nil {
			// Rollback completed operations in reverse order
			for j := len(completed) - 1; j >= 0; j-- {
				idx := completed[j]
				if t.Operations[idx].Rollback != nil {
					checkpoint, _ := manager.GetOperation(t.Operations[idx].ID)
					if rbErr := t.Operations[idx].Rollback(ctx, checkpoint); rbErr != nil {
						log.Printf("Failed to rollback operation %s: %v",
							t.Operations[idx].ID, rbErr)
					}
				}
			}
			return fmt.Errorf("operation %d failed: %w", i, err)
		}
		completed = append(completed, i)
	}

	return nil
}

// Global checkpoint manager
var CheckpointMgr = NewCheckpointManager()
