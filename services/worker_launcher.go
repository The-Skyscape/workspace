package services

import (
	"bytes"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"workspace/internal/ai"
	workerPkg "workspace/internal/worker"
	"workspace/models"
	
	"github.com/The-Skyscape/devtools/pkg/containers"

	"github.com/pkg/errors"
)

// WorkerLauncher manages local worker containers
type WorkerLauncher struct {
	workers map[string]*WorkerContainer
	mu      sync.RWMutex
}

// WorkerContainer represents a running worker container
type WorkerContainer struct {
	ID          string
	ContainerID string
	Port        int
	APIKey      string
	CreatedAt   time.Time
	container   *containers.Service
	binaryPath  string // Path to extracted worker binary for cleanup
}

var (
	// Global worker launcher instance
	workerLauncher *WorkerLauncher
	launcherOnce   sync.Once
)

// GetWorkerLauncher returns the singleton worker launcher
func GetWorkerLauncher() *WorkerLauncher {
	launcherOnce.Do(func() {
		workerLauncher = &WorkerLauncher{
			workers: make(map[string]*WorkerContainer),
		}
		// Clean up any orphaned worker containers on startup
		go workerLauncher.cleanupOrphanedWorkers()
	})
	return workerLauncher
}

// LaunchWorker creates and starts a new worker container
func (l *WorkerLauncher) LaunchWorker(workerID string) (*WorkerContainer, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Check if worker already exists
	if worker, exists := l.workers[workerID]; exists {
		return worker, nil
	}

	// Find an available port
	port, err := l.findAvailablePort()
	if err != nil {
		return nil, errors.Wrap(err, "failed to find available port")
	}

	// Generate API key for this worker
	apiKey := l.generateAPIKey()

	// Extract the embedded worker binary to a temporary location
	workerPath, err := workerPkg.ExtractWorkerBinary()
	if err != nil {
		return nil, errors.Wrap(err, "failed to extract worker binary")
	}
	
	// Ensure cleanup on any error
	cleanupBinary := true
	defer func() {
		if cleanupBinary && workerPath != "" {
			if err := workerPkg.CleanupWorkerBinary(workerPath); err != nil {
				log.Printf("WorkerLauncher: Failed to cleanup worker binary on error: %v", err)
			}
		}
	}()

	// Get the Anthropic API key from Vault
	authManager := ai.NewAuthManager(models.Secrets)
	anthropicKey := authManager.GetAPIKey()
	if anthropicKey == "" {
		return nil, errors.New("Anthropic API key not configured - please configure it in settings")
	}

	// Create container configuration
	container := &containers.Service{
		Name:    fmt.Sprintf("worker-%s", workerID),
		Image:   "skyscape:latest",
		Command: "/worker",
		Ports: map[int]int{
			port: 8080, // Map random host port to container's 8080
		},
		Env: map[string]string{
			"WORKER_API_KEY":    apiKey,
			"ANTHROPIC_API_KEY": anthropicKey,
			"AUTH_SECRET":       os.Getenv("AUTH_SECRET"),
			"PORT":              "8080",
		},
		Mounts: map[string]string{
			"/var/run/docker.sock": "/var/run/docker.sock", // Allow worker to manage containers
			fmt.Sprintf("/root/.skyscape/workers/%s", workerID): "/root/.skyscape",
		},
		Copied: map[string]string{
			workerPath: "/worker", // Copy the worker binary into the container
		},
		RestartPolicy: "unless-stopped",
	}

	// Launch the container
	log.Printf("WorkerLauncher: Launching worker %s on port %d", workerID, port)
	if err := containers.Launch(containers.Local(), container); err != nil {
		return nil, errors.Wrap(err, "failed to launch worker container")
	}

	// Wait for worker to be ready
	if err := l.waitForWorker(port); err != nil {
		// Clean up on failure
		container.Stop()
		return nil, errors.Wrap(err, "worker failed to start")
	}

	// Create worker record
	worker := &WorkerContainer{
		ID:          workerID,
		ContainerID: container.Name,
		Port:        port,
		APIKey:      apiKey,
		CreatedAt:   time.Now(),
		container:   container,
		binaryPath:  workerPath,
	}

	l.workers[workerID] = worker
	log.Printf("WorkerLauncher: Worker %s started successfully on port %d", workerID, port)
	
	// Update the global Worker client to use this port and API key
	if Worker != nil {
		Worker.UpdateEndpoint(fmt.Sprintf("http://localhost:%d", port), apiKey)
	} else {
		// Initialize if not already done
		Worker = NewWorkerClient(fmt.Sprintf("http://localhost:%d", port), apiKey)
	}

	// Success - don't cleanup the binary (container will handle it)
	cleanupBinary = false
	return worker, nil
}

// StopWorker stops and removes a worker container
func (l *WorkerLauncher) StopWorker(workerID string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	worker, exists := l.workers[workerID]
	if !exists {
		return errors.New("worker not found")
	}

	// Stop the container
	if err := worker.container.Stop(); err != nil {
		log.Printf("WorkerLauncher: Failed to stop worker %s: %v", workerID, err)
	}

	// Clean up the extracted worker binary
	if worker.binaryPath != "" {
		if err := workerPkg.CleanupWorkerBinary(worker.binaryPath); err != nil {
			log.Printf("WorkerLauncher: Failed to cleanup worker binary: %v", err)
		}
	}

	// Remove from tracking
	delete(l.workers, workerID)
	
	log.Printf("WorkerLauncher: Worker %s stopped", workerID)
	return nil
}

// GetWorker returns information about a running worker
func (l *WorkerLauncher) GetWorker(workerID string) (*WorkerContainer, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	worker, exists := l.workers[workerID]
	if !exists {
		return nil, errors.New("worker not found")
	}

	return worker, nil
}

// ListWorkers returns all running workers
func (l *WorkerLauncher) ListWorkers() []*WorkerContainer {
	l.mu.RLock()
	defer l.mu.RUnlock()

	workers := make([]*WorkerContainer, 0, len(l.workers))
	for _, worker := range l.workers {
		workers = append(workers, worker)
	}

	return workers
}

// findAvailablePort finds a random available port
func (l *WorkerLauncher) findAvailablePort() (int, error) {
	// Start with a random port in the range 9000-9999
	rand.Seed(time.Now().UnixNano())
	basePort := 9000 + rand.Intn(1000)

	for i := 0; i < 100; i++ {
		port := basePort + i
		
		// Check if port is already used by another worker
		used := false
		for _, worker := range l.workers {
			if worker.Port == port {
				used = true
				break
			}
		}
		if used {
			continue
		}

		// Check if port is available on the system
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			listener.Close()
			return port, nil
		}
	}

	return 0, errors.New("no available ports found")
}

// generateAPIKey generates a random API key
func (l *WorkerLauncher) generateAPIKey() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 32)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

// waitForWorker waits for the worker to be ready
func (l *WorkerLauncher) waitForWorker(port int) error {
	client := NewWorkerClient(fmt.Sprintf("http://localhost:%d", port), "")
	
	// Try for up to 30 seconds
	for i := 0; i < 30; i++ {
		if client.IsHealthy() {
			return nil
		}
		time.Sleep(time.Second)
	}
	
	return errors.New("worker failed to become healthy")
}

// CleanupWorkers stops all running workers
func (l *WorkerLauncher) CleanupWorkers() {
	l.mu.Lock()
	defer l.mu.Unlock()

	for id, worker := range l.workers {
		if err := worker.container.Stop(); err != nil {
			log.Printf("WorkerLauncher: Failed to stop worker %s: %v", id, err)
		}
		// Clean up the extracted worker binary
		if worker.binaryPath != "" {
			if err := workerPkg.CleanupWorkerBinary(worker.binaryPath); err != nil {
				log.Printf("WorkerLauncher: Failed to cleanup worker binary: %v", err)
			}
		}
	}

	l.workers = make(map[string]*WorkerContainer)
}

// cleanupOrphanedWorkers removes any worker containers left from previous runs
func (l *WorkerLauncher) cleanupOrphanedWorkers() {
	log.Println("WorkerLauncher: Checking for orphaned worker containers...")
	
	host := containers.Local()
	
	// List all containers and look for worker containers
	var stdout bytes.Buffer
	host.SetStdout(&stdout)
	
	// List all containers with worker- prefix
	if err := host.Exec("docker", "ps", "-a", "--filter", "name=worker-", "--format", "{{.Names}}"); err != nil {
		log.Printf("WorkerLauncher: Failed to list containers: %v", err)
		return
	}
	
	containerNames := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	cleaned := 0
	
	for _, name := range containerNames {
		if name != "" && strings.HasPrefix(name, "worker-") {
			log.Printf("WorkerLauncher: Removing orphaned container: %s", name)
			if err := host.Exec("docker", "rm", "-f", name); err != nil {
				log.Printf("WorkerLauncher: Failed to remove container %s: %v", name, err)
			} else {
				cleaned++
			}
		}
	}
	
	if cleaned > 0 {
		log.Printf("WorkerLauncher: Cleaned up %d orphaned worker containers", cleaned)
	}
}