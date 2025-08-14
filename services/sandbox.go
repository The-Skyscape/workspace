package services

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
	
	"github.com/The-Skyscape/devtools/pkg/containers"
	"github.com/The-Skyscape/devtools/pkg/database"
	"github.com/pkg/errors"
)

// SandboxService provides stateless sandbox container execution
type SandboxService struct {
	host containers.Host
}

var (
	// Global sandbox service instance
	Sandboxes = &SandboxService{
		host: containers.Local(),
	}
)

// SandboxInfo represents runtime information about a sandbox
type SandboxInfo struct {
	Name        string
	RepoID      string
	Command     string
	Status      string // running, stopped, completed
	StartTime   time.Time
	Output      string
	ExitCode    int
	IsRunning   bool
}

// containerName returns the Docker container name for a sandbox
func (s *SandboxService) containerName(sandboxName string) string {
	return fmt.Sprintf("skyscape-sandbox-%s", sandboxName)
}

// StartSandbox starts a sandbox container with the given parameters
func (s *SandboxService) StartSandbox(name, repoPath, repoName, command string, timeoutSecs int) error {
	// Prepare directories
	sandboxDir := fmt.Sprintf("%s/sandboxes/%s", database.DataDir(), name)
	workspaceDir := fmt.Sprintf("%s/workspace", sandboxDir)
	repoDir := fmt.Sprintf("%s/repo", workspaceDir)
	
	// Clean up any existing sandbox directory
	os.RemoveAll(sandboxDir)
	
	// Create directories
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		return errors.Wrap(err, "failed to create sandbox directories")
	}
	
	// Clone repository to sandbox directory
	log.Printf("Cloning repository to sandbox %s", name)
	cloneCmd := fmt.Sprintf("git clone --bare %s %s.git && git clone %s.git %s && rm -rf %s.git",
		repoPath, repoDir, repoDir, repoDir, repoDir)
	
	if err := s.host.Exec("bash", "-c", cloneCmd); err != nil {
		return errors.Wrap(err, "failed to clone repository")
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
		return errors.Wrap(err, "failed to create command script")
	}
	
	// Create service configuration
	containerName := s.containerName(name)
	service := &containers.Service{
		Host:  s.host,
		Name:  containerName,
		Image: "skyscape:latest",
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
	
	// Launch the container
	log.Printf("Starting sandbox container %s", containerName)
	if err := containers.Launch(s.host, service); err != nil {
		return errors.Wrap(err, "failed to launch sandbox container")
	}
	
	// Start monitoring in a goroutine
	go s.monitorContainer(name, timeoutSecs)
	
	return nil
}

// monitorContainer monitors a running container with timeout
func (s *SandboxService) monitorContainer(name string, timeoutSecs int) {
	containerName := s.containerName(name)
	startTime := time.Now()
	timeout := time.Duration(timeoutSecs) * time.Second
	
	for {
		service := &containers.Service{
			Host: s.host,
			Name: containerName,
		}
		
		if !service.IsRunning() {
			log.Printf("Sandbox %s container stopped", name)
			return
		}
		
		if time.Since(startTime) > timeout {
			log.Printf("Sandbox %s timed out after %v", name, timeout)
			s.StopSandbox(name)
			return
		}
		
		time.Sleep(2 * time.Second)
	}
}

// StopSandbox stops a running sandbox container
func (s *SandboxService) StopSandbox(name string) error {
	containerName := s.containerName(name)
	service := &containers.Service{
		Host: s.host,
		Name: containerName,
	}
	
	if err := service.Stop(); err != nil {
		log.Printf("Failed to stop sandbox container %s: %v", containerName, err)
		return err
	}
	
	return nil
}

// GetOutput retrieves the output from a sandbox
func (s *SandboxService) GetOutput(name string) (string, error) {
	outputFile := fmt.Sprintf("%s/sandboxes/%s/output.log", database.DataDir(), name)
	output, err := os.ReadFile(outputFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "No output available yet", nil
		}
		return "", errors.Wrap(err, "failed to read output file")
	}
	
	return string(output), nil
}

// GetExitCode gets the exit code of a container
func (s *SandboxService) GetExitCode(name string) int {
	var stdout bytes.Buffer
	s.host.SetStdout(&stdout)
	containerName := s.containerName(name)
	err := s.host.Exec("docker", "inspect", "-f", "{{.State.ExitCode}}", containerName)
	
	exitCode := 0
	if err == nil {
		fmt.Sscanf(strings.TrimSpace(stdout.String()), "%d", &exitCode)
	}
	return exitCode
}

// IsRunning checks if a sandbox container is running
func (s *SandboxService) IsRunning(name string) bool {
	containerName := s.containerName(name)
	service := &containers.Service{
		Host: s.host,
		Name: containerName,
	}
	return service.IsRunning()
}

// CleanupSandbox removes a sandbox container and its files
func (s *SandboxService) CleanupSandbox(name string) error {
	// Remove container if it exists
	containerName := s.containerName(name)
	service := &containers.Service{
		Host: s.host,
		Name: containerName,
	}
	
	// Try to remove the container (ignore errors if it doesn't exist)
	service.Remove()
	
	// Remove sandbox directory
	sandboxDir := fmt.Sprintf("%s/sandboxes/%s", database.DataDir(), name)
	if err := os.RemoveAll(sandboxDir); err != nil {
		log.Printf("Failed to remove sandbox directory %s: %v", sandboxDir, err)
	}
	
	return nil
}

// DownloadFile downloads a specific file from a sandbox
func (s *SandboxService) DownloadFile(name string, filePath string) ([]byte, error) {
	// Security: ensure file path is within workspace
	if strings.Contains(filePath, "..") {
		return nil, errors.New("invalid file path")
	}
	
	// Read file directly from mounted directory
	fullPath := fmt.Sprintf("%s/sandboxes/%s/workspace/%s", database.DataDir(), name, filePath)
	
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read file")
	}
	
	return data, nil
}

// GetArtifactsList returns a list of files in the sandbox workspace
func (s *SandboxService) GetArtifactsList(name string) ([]string, error) {
	workspaceDir := fmt.Sprintf("%s/sandboxes/%s/workspace", database.DataDir(), name)
	
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
		
		files = append(files, relPath)
		return nil
	})
	
	if err != nil {
		return nil, errors.Wrap(err, "failed to list artifacts")
	}
	
	return files, nil
}

// CreateArtifactArchive creates a tar archive of specified files
func (s *SandboxService) CreateArtifactArchive(name string, files []string) (io.Reader, error) {
	workspaceDir := fmt.Sprintf("%s/sandboxes/%s/workspace", database.DataDir(), name)
	
	// Create tar archive in memory
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	defer tw.Close()
	
	for _, file := range files {
		// Security check
		if strings.Contains(file, "..") {
			continue
		}
		
		fullPath := filepath.Join(workspaceDir, file)
		
		// Get file info
		info, err := os.Stat(fullPath)
		if err != nil {
			continue
		}
		
		// Create tar header
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			continue
		}
		header.Name = file
		
		// Write header
		if err := tw.WriteHeader(header); err != nil {
			continue
		}
		
		// Write file content
		if !info.IsDir() {
			data, err := os.ReadFile(fullPath)
			if err != nil {
				continue
			}
			if _, err := tw.Write(data); err != nil {
				continue
			}
		}
	}
	
	return &buf, nil
}

// Exec executes a command in a running sandbox container
func (s *SandboxService) Exec(name string, command string) (string, int, error) {
	containerName := s.containerName(name)
	
	// Check if container is running
	if !s.IsRunning(name) {
		return "", -1, errors.New("sandbox is not running")
	}
	
	// Execute command in container
	var stdout, stderr bytes.Buffer
	s.host.SetStdout(&stdout)
	s.host.SetStderr(&stderr)
	
	err := s.host.Exec("docker", "exec", containerName, "bash", "-c", command)
	
	// Get exit code
	exitCode := 0
	if err != nil {
		// Try to extract exit code from error
		exitCode = 1
	}
	
	// Combine stdout and stderr
	output := stdout.String()
	if stderr.String() != "" {
		output += "\n" + stderr.String()
	}
	
	return output, exitCode, nil
}

// ExtractFile extracts a single file from a sandbox container
func (s *SandboxService) ExtractFile(name string, filePath string) ([]byte, error) {
	// Security check: prevent directory traversal
	if strings.Contains(filePath, "..") {
		return nil, errors.New("invalid file path")
	}
	
	// Read file from sandbox workspace directory
	fullPath := fmt.Sprintf("%s/sandboxes/%s/workspace/%s", database.DataDir(), name, filePath)
	
	// Check if file exists
	if _, err := os.Stat(fullPath); err != nil {
		if os.IsNotExist(err) {
			// Try to read from the repo subdirectory
			fullPath = fmt.Sprintf("%s/sandboxes/%s/workspace/repo/%s", database.DataDir(), name, filePath)
			if _, err := os.Stat(fullPath); err != nil {
				return nil, errors.Wrap(err, "file not found")
			}
		} else {
			return nil, errors.Wrap(err, "failed to check file")
		}
	}
	
	// Read file content
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read file")
	}
	
	return data, nil
}

// ListSandboxes returns info about all sandbox containers
func (s *SandboxService) ListSandboxes() ([]*SandboxInfo, error) {
	// List all containers with sandbox prefix
	var stdout bytes.Buffer
	s.host.SetStdout(&stdout)
	err := s.host.Exec("docker", "ps", "-a", "--filter", "name=skyscape-sandbox-", "--format", "{{.Names}}|{{.Status}}|{{.CreatedAt}}")
	if err != nil {
		return nil, err
	}
	
	var sandboxes []*SandboxInfo
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if line == "" {
			continue
		}
		
		parts := strings.Split(line, "|")
		if len(parts) < 2 {
			continue
		}
		
		name := strings.TrimPrefix(parts[0], "skyscape-sandbox-")
		status := "stopped"
		if strings.Contains(parts[1], "Up") {
			status = "running"
		} else if strings.Contains(parts[1], "Exited (0)") {
			status = "completed"
		} else if strings.Contains(parts[1], "Exited") {
			status = "failed"
		}
		
		info := &SandboxInfo{
			Name:      name,
			Status:    status,
			IsRunning: status == "running",
		}
		
		// Try to get output
		if output, err := s.GetOutput(name); err == nil {
			info.Output = output
		}
		
		// Get exit code if not running
		if !info.IsRunning {
			info.ExitCode = s.GetExitCode(name)
		}
		
		sandboxes = append(sandboxes, info)
	}
	
	return sandboxes, nil
}

// GetSandboxInfo returns info about a specific sandbox
func (s *SandboxService) GetSandboxInfo(name string) (*SandboxInfo, error) {
	info := &SandboxInfo{
		Name:      name,
		IsRunning: s.IsRunning(name),
	}
	
	if info.IsRunning {
		info.Status = "running"
	} else {
		// Check exit code to determine status
		info.ExitCode = s.GetExitCode(name)
		if info.ExitCode == 0 {
			info.Status = "completed"
		} else {
			info.Status = "failed"
		}
	}
	
	// Get output
	if output, err := s.GetOutput(name); err == nil {
		info.Output = output
	}
	
	// Try to read command from script
	scriptPath := fmt.Sprintf("%s/sandboxes/%s/run.sh", database.DataDir(), name)
	if scriptContent, err := os.ReadFile(scriptPath); err == nil {
		// Extract command from script (it's after "Command: " line)
		lines := strings.Split(string(scriptContent), "\n")
		for i, line := range lines {
			if strings.Contains(line, "echo \"Command: ") {
				// Next few lines after the echo contain the actual command
				if i+4 < len(lines) {
					// Skip the echo lines and get to the actual command
					for j := i + 4; j < len(lines); j++ {
						if strings.Contains(lines[j], "EXIT_CODE=") {
							break
						}
						if lines[j] != "" && !strings.Contains(lines[j], "echo") {
							info.Command = strings.TrimSpace(lines[j])
							break
						}
					}
				}
				break
			}
		}
	}
	
	return info, nil
}