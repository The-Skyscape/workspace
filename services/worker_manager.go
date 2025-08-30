package services

import (
	"fmt"
	"log"
	"sync"
	
	"github.com/pkg/errors"
)

// WorkerManager manages both container lifecycle and API communication
type WorkerManager struct {
	launcher *WorkerLauncher
	clients  map[string]*WorkerClient // Map of worker ID to its client
	mu       sync.RWMutex
}

var (
	workerManager *WorkerManager
	managerOnce   sync.Once
)

// GetWorkerManager returns the singleton worker manager
func GetWorkerManager() *WorkerManager {
	managerOnce.Do(func() {
		workerManager = &WorkerManager{
			launcher: GetWorkerLauncher(),
			clients:  make(map[string]*WorkerClient),
		}
	})
	return workerManager
}

// LaunchWorker creates a new worker container and its associated client
func (m *WorkerManager) LaunchWorker(workerID string) (*WorkerContainer, error) {
	// Launch the container
	container, err := m.launcher.LaunchWorker(workerID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to launch container")
	}
	
	// Create a client for this specific worker
	baseURL := fmt.Sprintf("http://localhost:%d", container.Port)
	client := NewWorkerClient(baseURL, container.APIKey)
	
	// Store the client
	m.mu.Lock()
	m.clients[workerID] = client
	m.mu.Unlock()
	
	log.Printf("WorkerManager: Created worker %s with client on port %d", workerID, container.Port)
	
	return container, nil
}

// GetClient returns the API client for a specific worker
func (m *WorkerManager) GetClient(workerID string) (*WorkerClient, error) {
	m.mu.RLock()
	client, exists := m.clients[workerID]
	m.mu.RUnlock()
	
	if !exists {
		// Try to recreate client from existing container
		container, err := m.launcher.GetWorker(workerID)
		if err != nil {
			return nil, errors.Wrap(err, "worker not found")
		}
		
		// Create a new client
		baseURL := fmt.Sprintf("http://localhost:%d", container.Port)
		client = NewWorkerClient(baseURL, container.APIKey)
		
		// Store it
		m.mu.Lock()
		m.clients[workerID] = client
		m.mu.Unlock()
	}
	
	return client, nil
}

// StopWorker stops a worker container and removes its client
func (m *WorkerManager) StopWorker(workerID string) error {
	// Remove the client
	m.mu.Lock()
	delete(m.clients, workerID)
	m.mu.Unlock()
	
	// Stop the container
	return m.launcher.StopWorker(workerID)
}

// GetContainer returns information about a worker's container
func (m *WorkerManager) GetContainer(workerID string) (*WorkerContainer, error) {
	return m.launcher.GetWorker(workerID)
}