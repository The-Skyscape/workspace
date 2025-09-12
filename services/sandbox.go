package services

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/The-Skyscape/devtools/pkg/containers"
	"github.com/The-Skyscape/devtools/pkg/database"
	"github.com/pkg/errors"
)

// Sandbox represents a containerized execution environment
type Sandbox struct {
	Name        string
	RepoPath    string
	RepoName    string
	Command     string
	TimeoutSecs int
	Container   *containers.Service
	startTime   time.Time
	mu          sync.RWMutex
}

// sandboxRegistry keeps track of active sandboxes
var (
	sandboxRegistry = make(map[string]*Sandbox)
	registryMu      sync.RWMutex
)

// NewSandbox creates a new sandbox instance and prepares it for execution
func NewSandbox(name, repoPath, repoName, command string, timeoutSecs int) (*Sandbox, error) {
	// Check if sandbox already exists
	registryMu.RLock()
	if existing, exists := sandboxRegistry[name]; exists {
		registryMu.RUnlock()
		return existing, nil
	}
	registryMu.RUnlock()

	// Prepare directories
	sandboxDir := fmt.Sprintf("%s/sandboxes/%s", database.DataDir(), name)
	workspaceDir := fmt.Sprintf("%s/workspace", sandboxDir)
	repoDir := fmt.Sprintf("%s/repo", workspaceDir)

	// Clean up any existing sandbox directory
	os.RemoveAll(sandboxDir)

	// Create directories
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		return nil, errors.Wrap(err, "failed to create sandbox directories")
	}

	// Clone repository to sandbox directory (only if repoPath is provided)
	host := containers.Local()
	if repoPath != "" {
		log.Printf("Cloning repository to sandbox %s", name)
		cloneCmd := fmt.Sprintf("git clone --bare %s %s.git && git clone %s.git %s && rm -rf %s.git",
			repoPath, repoDir, repoDir, repoDir, repoDir)

		if err := host.Exec("bash", "-c", cloneCmd); err != nil {
			return nil, errors.Wrap(err, "failed to clone repository")
		}
	} else {
		log.Printf("Creating sandbox %s without repository (multi-repo mode)", name)
	}

	// Create command script
	scriptPath := fmt.Sprintf("%s/run.sh", sandboxDir)
	scriptContent := fmt.Sprintf(`#!/bin/bash
set -e
cd /workspace/repo

echo "=== Sandbox Started: %s ==="
echo "Repository: %s"
echo "Command: %s"
echo "==================================="
echo ""

# Execute the user command
%s

EXIT_CODE=$?
echo ""
echo "==================================="
echo "=== Sandbox Completed with exit code: $EXIT_CODE ==="
exit $EXIT_CODE
`, name, repoName, command, command)

	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		return nil, errors.Wrap(err, "failed to create command script")
	}

	// Create container configuration
	containerName := fmt.Sprintf("skyscape-sandbox-%s", name)
	container := &containers.Service{
		Host:    host,
		Name:    containerName,
		Image:   "skyscape:latest",
		Command: "/bin/bash -c '/bin/bash /sandbox/run.sh > /sandbox/output.log 2>&1'",
		Network: "bridge",
		Mounts: map[string]string{
			workspaceDir: "/workspace",
			sandboxDir:   "/sandbox",
		},
		Env: map[string]string{
			"SANDBOX_NAME": name,
		},
	}

	sandbox := &Sandbox{
		Name:        name,
		RepoPath:    repoPath,
		RepoName:    repoName,
		Command:     command,
		TimeoutSecs: timeoutSecs,
		Container:   container,
	}

	// Register sandbox
	registryMu.Lock()
	sandboxRegistry[name] = sandbox
	registryMu.Unlock()

	return sandbox, nil
}

// GetSandbox retrieves an existing sandbox by name
func GetSandbox(name string) (*Sandbox, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()

	sandbox, exists := sandboxRegistry[name]
	if !exists {
		return nil, errors.Errorf("sandbox %s not found", name)
	}
	return sandbox, nil
}

// ListSandboxes returns all registered sandboxes
func ListSandboxes() []*Sandbox {
	registryMu.RLock()
	defer registryMu.RUnlock()

	sandboxes := make([]*Sandbox, 0, len(sandboxRegistry))
	for _, sandbox := range sandboxRegistry {
		sandboxes = append(sandboxes, sandbox)
	}
	return sandboxes
}

// Start launches the sandbox container and begins execution
func (s *Sandbox) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.IsRunning() {
		return errors.New("sandbox is already running")
	}

	log.Printf("Starting sandbox container %s", s.Container.Name)
	if err := containers.Launch(s.Container.Host, s.Container); err != nil {
		return errors.Wrap(err, "failed to launch sandbox container")
	}

	s.startTime = time.Now()

	// Start monitoring in a goroutine if timeout is set
	if s.TimeoutSecs > 0 {
		go s.monitorTimeout()
	}

	return nil
}

// Stop stops the sandbox container
func (s *Sandbox) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.Container.Stop(); err != nil {
		log.Printf("Failed to stop sandbox container %s: %v", s.Container.Name, err)
		return err
	}

	return nil
}

// IsRunning checks if the sandbox container is running
func (s *Sandbox) IsRunning() bool {
	return s.Container.IsRunning()
}

// Execute runs a command inside the sandbox container
func (s *Sandbox) Execute(command string) (string, int, error) {
	if !s.IsRunning() {
		return "", -1, errors.New("sandbox is not running")
	}

	output, err := s.Container.ExecInContainerWithOutput("bash", "-c", command)
	if err != nil {
		// Try to extract exit code from error
		return output, 1, err
	}

	return output, 0, nil
}

// GetOutput retrieves the output from the sandbox
func (s *Sandbox) GetOutput() (string, error) {
	outputFile := fmt.Sprintf("%s/sandboxes/%s/output.log", database.DataDir(), s.Name)
	output, err := os.ReadFile(outputFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "No output available yet", nil
		}
		return "", errors.Wrap(err, "failed to read output file")
	}

	return string(output), nil
}

// GetLogs retrieves container logs
func (s *Sandbox) GetLogs(tail int) (string, error) {
	return s.Container.GetLogs(tail)
}

// GetExitCode gets the exit code of the container
func (s *Sandbox) GetExitCode() int {
	if !s.IsRunning() {
		// Use host to inspect container exit code
		var stdout strings.Builder
		s.Container.Host.SetStdout(&stdout)
		err := s.Container.Host.Exec("docker", "inspect", "-f", "{{.State.ExitCode}}", s.Container.Name)
		if err == nil {
			var exitCode int
			fmt.Sscanf(strings.TrimSpace(stdout.String()), "%d", &exitCode)
			return exitCode
		}
	}
	return -1
}

// WaitForCompletion waits for the sandbox to complete execution
func (s *Sandbox) WaitForCompletion() error {
	for s.IsRunning() {
		time.Sleep(1 * time.Second)
	}
	return nil
}

// DownloadFile downloads a specific file from the sandbox
func (s *Sandbox) DownloadFile(filePath string) ([]byte, error) {
	// Security: ensure file path is within workspace
	if strings.Contains(filePath, "..") {
		return nil, errors.New("invalid file path")
	}

	// Read file directly from mounted directory
	fullPath := fmt.Sprintf("%s/sandboxes/%s/workspace/%s", database.DataDir(), s.Name, filePath)

	// Try workspace/repo subdirectory if file not found
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		fullPath = fmt.Sprintf("%s/sandboxes/%s/workspace/repo/%s", database.DataDir(), s.Name, filePath)
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read file")
	}

	return data, nil
}

// ExtractArtifacts extracts multiple files/paths from the sandbox
func (s *Sandbox) ExtractArtifacts(paths []string) (map[string][]byte, error) {
	artifacts := make(map[string][]byte)

	for _, path := range paths {
		// Clean and validate path
		path = strings.TrimSpace(path)
		if path == "" || strings.Contains(path, "..") {
			continue
		}

		// Check if it's a pattern or specific file
		if strings.Contains(path, "*") {
			// Handle glob pattern
			baseDir := fmt.Sprintf("%s/sandboxes/%s/workspace/repo", database.DataDir(), s.Name)
			matches, err := filepath.Glob(filepath.Join(baseDir, path))
			if err != nil {
				log.Printf("Failed to glob pattern %s: %v", path, err)
				continue
			}

			for _, match := range matches {
				relPath, _ := filepath.Rel(baseDir, match)
				if data, err := os.ReadFile(match); err == nil {
					artifacts[relPath] = data
				}
			}
		} else if strings.HasSuffix(path, "/") {
			// Handle directory
			dirPath := fmt.Sprintf("%s/sandboxes/%s/workspace/repo/%s", database.DataDir(), s.Name, path)
			err := filepath.Walk(dirPath, func(filePath string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return nil
				}

				relPath, _ := filepath.Rel(fmt.Sprintf("%s/sandboxes/%s/workspace/repo", database.DataDir(), s.Name), filePath)
				if data, err := os.ReadFile(filePath); err == nil {
					artifacts[relPath] = data
				}
				return nil
			})
			if err != nil {
				log.Printf("Failed to walk directory %s: %v", path, err)
			}
		} else {
			// Handle single file
			if data, err := s.DownloadFile(path); err == nil {
				artifacts[path] = data
			}
		}
	}

	return artifacts, nil
}

// ListFiles lists files in the sandbox workspace
func (s *Sandbox) ListFiles(pattern string) ([]string, error) {
	workspaceDir := fmt.Sprintf("%s/sandboxes/%s/workspace", database.DataDir(), s.Name)

	var files []string
	err := filepath.Walk(workspaceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and hidden files
		if info.IsDir() || strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		// Get relative path from workspace
		relPath, err := filepath.Rel(workspaceDir, path)
		if err != nil {
			return err
		}

		// Apply pattern matching if specified
		if pattern != "" {
			matched, _ := filepath.Match(pattern, relPath)
			if !matched {
				return nil
			}
		}

		files = append(files, relPath)
		return nil
	})

	if err != nil {
		return nil, errors.Wrap(err, "failed to list files")
	}

	return files, nil
}

// Cleanup removes the sandbox container and its files
func (s *Sandbox) Cleanup() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove container if it exists
	if s.Container != nil {
		s.Container.Remove()
	}

	// Remove sandbox directory
	sandboxDir := fmt.Sprintf("%s/sandboxes/%s", database.DataDir(), s.Name)
	if err := os.RemoveAll(sandboxDir); err != nil {
		log.Printf("Failed to remove sandbox directory %s: %v", sandboxDir, err)
	}

	// Remove from registry
	registryMu.Lock()
	delete(sandboxRegistry, s.Name)
	registryMu.Unlock()

	return nil
}

// monitorTimeout monitors the sandbox and stops it if it exceeds the timeout
func (s *Sandbox) monitorTimeout() {
	timeout := time.Duration(s.TimeoutSecs) * time.Second
	time.Sleep(timeout)

	if s.IsRunning() {
		log.Printf("Sandbox %s timed out after %v", s.Name, timeout)
		s.Stop()
	}
}

// GetStatus returns current status information about the sandbox
func (s *Sandbox) GetStatus() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := map[string]any{
		"name":       s.Name,
		"running":    s.IsRunning(),
		"start_time": s.startTime,
	}

	if !s.IsRunning() && !s.startTime.IsZero() {
		status["duration"] = time.Since(s.startTime).Seconds()
		status["exit_code"] = s.GetExitCode()
	}

	return status
}

// TriggerActionsByEvent triggers actions based on an event
// This is a module-level function that controllers can call to trigger actions
func TriggerActionsByEvent(eventType, repoID string, eventData map[string]string) error {
	// This function would typically:
	// 1. Query the database for actions matching the event type and repository
	// 2. Check trigger conditions based on eventData
	// 3. Execute matching actions

	// For now, we'll provide a stub implementation
	// The actual implementation would need to import models package,
	// but that would create a circular dependency
	// This should be handled by the controller that calls this function

	log.Printf("TriggerActionsByEvent called: type=%s, repo=%s, data=%+v",
		eventType, repoID, eventData)

	// Return nil to indicate no error
	// The actual execution logic should be in the controllers
	return nil
}
