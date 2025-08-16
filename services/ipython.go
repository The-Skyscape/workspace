package services

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"workspace/models"

	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/containers"
	"github.com/The-Skyscape/devtools/pkg/database"
	"github.com/pkg/errors"
)

// IPythonConfig holds configuration for the IPython/Jupyter service
type IPythonConfig struct {
	Port          int
	ContainerName string
	DataDir       string
	NotebooksDir  string
	Token         string
	EnableAuth    bool
}

// IPythonService manages a single global Jupyter notebook instance
type IPythonService struct {
	config  *IPythonConfig
	service *containers.Service
	mu      sync.RWMutex
}

// IPythonStatus represents the current status of the IPython service
type IPythonStatus struct {
	Running      bool
	Port         int
	URL          string
	Token        string
	Health       string
	NotebooksDir string
}

var (
	// Global IPython/Jupyter instance
	IPython = NewIPythonService()
)

// NewIPythonService creates a new IPython service with default configuration
func NewIPythonService() *IPythonService {
	return &IPythonService{
		config: &IPythonConfig{
			Port:          8888,
			ContainerName: "skyscape-ipython",
			DataDir:       fmt.Sprintf("%s/ipython", database.DataDir()),
			NotebooksDir:  fmt.Sprintf("%s/ipython/notebooks", database.DataDir()),
			Token:         "", // No token for dev mode
			EnableAuth:    false,
		},
	}
}

// Init initializes the IPython service and starts it if not already running
func (i *IPythonService) Init() error {
	i.mu.Lock()
	defer i.mu.Unlock()

	// Check if service already exists and is running
	existing := containers.Local().Service(i.config.ContainerName)
	if existing != nil && existing.IsRunning() {
		log.Println("IPython service already running")
		i.service = existing
		return nil
	}

	// Start the service
	log.Println("Initializing IPython service...")
	
	// Ensure notebooks directory exists with proper permissions
	if err := os.MkdirAll(i.config.NotebooksDir, 0777); err != nil {
		return errors.Wrap(err, "failed to create notebooks directory")
	}

	// Create the service configuration
	host := containers.Local()
	
	// Build command based on auth settings
	command := "start-notebook.sh"
	if !i.config.EnableAuth {
		command += " --NotebookApp.token='' --NotebookApp.password=''"
	} else if i.config.Token != "" {
		command += fmt.Sprintf(" --NotebookApp.token='%s'", i.config.Token)
	}
	command += " --NotebookApp.allow_origin='*'"
	command += " --NotebookApp.ip='0.0.0.0'"
	command += " --NotebookApp.port=8888"
	command += " --NotebookApp.base_url='/ipython/'"

	i.service = &containers.Service{
		Host:          host,
		Name:          i.config.ContainerName,
		Image:         "jupyter/datascience-notebook:latest",
		Network:       "host",
		RestartPolicy: "always",
		Command:       command,
		Mounts: map[string]string{
			i.config.NotebooksDir: "/home/jovyan/work",
		},
		Env: map[string]string{
			"JUPYTER_ENABLE_LAB": "yes",
			"GRANT_SUDO":         "yes",
			"CHOWN_HOME":         "yes",
			"CHOWN_HOME_OPTS":    "-R",
		},
	}

	// Start the service WITHOUT calling i.Start() to avoid deadlock
	if err := i.startInternal(); err != nil {
		return err
	}
	return nil
}

// startInternal starts the IPython service without acquiring the lock
func (i *IPythonService) startInternal() error {
	if i.service == nil {
		return errors.New("IPython service not initialized")
	}

	// Check if already running
	var stdout bytes.Buffer
	i.service.Host.SetStdout(&stdout)
	err := i.service.Host.Exec("docker", "inspect", "-f", "{{.State.Status}}", i.config.ContainerName)
	if err == nil && strings.TrimSpace(stdout.String()) == "running" {
		return nil
	}

	log.Printf("Starting IPython/Jupyter service on port %d", i.config.Port)
	
	// Launch the container
	if err := containers.Launch(i.service.Host, i.service); err != nil {
		return errors.Wrap(err, "failed to launch IPython container")
	}

	// Wait for Jupyter to be ready
	if err := i.service.WaitForReady(60*time.Second, func() error {
		// Check if Jupyter is responding
		return i.service.Host.Exec("curl", "-f", fmt.Sprintf("http://localhost:%d/api", i.config.Port))
	}); err != nil {
		log.Printf("Warning: Jupyter may not be fully ready: %v", err)
	}

	log.Printf("IPython/Jupyter service started successfully")
	log.Printf("Access Jupyter Lab at: http://localhost:%d/ipython/", i.config.Port)
	if !i.config.EnableAuth {
		log.Printf("Warning: Authentication is disabled for development")
	}

	return nil
}

// Start starts the IPython service
func (i *IPythonService) Start() error {
	i.mu.Lock()
	defer i.mu.Unlock()
	
	return i.startInternal()
}

// Stop stops the IPython service
func (i *IPythonService) Stop() error {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.service == nil {
		return nil
	}

	log.Println("Stopping IPython/Jupyter service")
	return i.service.Stop()
}

// IsRunning checks if the IPython service is running
func (i *IPythonService) IsRunning() bool {
	if i.service == nil {
		// Check if container exists from a previous run
		existing := containers.Local().Service(i.config.ContainerName)
		if existing != nil {
			i.service = existing
		}
	}
	return i.service != nil && i.service.IsRunning()
}

// Restart restarts the IPython service
func (i *IPythonService) Restart() error {
	if err := i.Stop(); err != nil {
		return err
	}
	return i.Start()
}

// GetStatus returns the current status of the IPython service
func (i *IPythonService) GetStatus() *IPythonStatus {
	i.mu.RLock()
	defer i.mu.RUnlock()

	status := &IPythonStatus{
		Running:      i.IsRunning(),
		Port:         i.config.Port,
		URL:          fmt.Sprintf("http://localhost:%d", i.config.Port),
		Token:        i.config.Token,
		NotebooksDir: i.config.NotebooksDir,
	}

	if status.Running {
		status.Health = "healthy"
	} else {
		status.Health = "stopped"
	}

	return status
}

// GetPort returns the port the service is running on
func (i *IPythonService) GetPort() int {
	i.mu.RLock()
	defer i.mu.RUnlock()
	
	return i.config.Port
}

// GetConfig returns the current configuration
func (i *IPythonService) GetConfig() *IPythonConfig {
	i.mu.RLock()
	defer i.mu.RUnlock()
	
	// Return a copy to prevent external modification
	config := *i.config
	return &config
}

// ExecuteCode executes Python code in the notebook environment
func (i *IPythonService) ExecuteCode(code string) (string, error) {
	if !i.IsRunning() {
		return "", errors.New("IPython service is not running")
	}

	// Execute Python code via docker exec
	output, err := i.service.ExecInContainerWithOutput("python", "-c", code)
	if err != nil {
		return "", errors.Wrap(err, "failed to execute code")
	}

	return output, nil
}

// CreateNotebook creates a new notebook file
func (i *IPythonService) CreateNotebook(name string) error {
	if !i.IsRunning() {
		return errors.New("IPython service is not running")
	}

	// Create an empty notebook structure
	notebookContent := `{
 "cells": [],
 "metadata": {
  "kernelspec": {
   "display_name": "Python 3",
   "language": "python",
   "name": "python3"
  },
  "language_info": {
   "name": "python",
   "version": "3.9"
  }
 },
 "nbformat": 4,
 "nbformat_minor": 5
}`

	notebookPath := fmt.Sprintf("%s/%s.ipynb", i.config.NotebooksDir, name)
	return os.WriteFile(notebookPath, []byte(notebookContent), 0644)
}

// ====== Repository Operations ======

// CloneRepository creates a working copy of the repository in the Jupyter workspace
func (i *IPythonService) CloneRepository(repo *models.Repository, user *authentication.User) error {
	i.mu.RLock()
	defer i.mu.RUnlock()

	if i.service == nil || !i.IsRunning() {
		return errors.New("IPython service is not running")
	}

	// Create access token for cloning
	token, err := models.CreateAccessToken(repo.ID, user.ID, 100*365*24*time.Hour)
	if err != nil {
		return errors.Wrap(err, "failed to create access token")
	}

	// Build clone URL for localhost (port 80 in production)
	cloneURL := fmt.Sprintf("http://%s:%s@localhost/repo/%s", token.ID, token.Token, repo.ID)

	// Get user details for git config
	gitUserName := user.Email // Default to email
	gitUserEmail := user.Email
	if user.Name != "" {
		gitUserName = user.Name
	}

	// Build clone command (use repo.ID for directory name to avoid conflicts)
	cloneCmd := i.buildCloneCommand(repo.ID, cloneURL, gitUserName, gitUserEmail)

	// Execute using container abstraction
	if err := i.service.ExecInContainer("bash", "-c", cloneCmd); err != nil {
		return errors.Wrapf(err, "failed to clone repository %s", repo.ID)
	}

	log.Printf("Repository %s cloned in Jupyter workspace", repo.ID)
	return nil
}

// UpdateRepository updates the working copy after a git push
func (i *IPythonService) UpdateRepository(repoID string) error {
	i.mu.RLock()
	defer i.mu.RUnlock()

	if i.service == nil || !i.IsRunning() {
		return errors.New("IPython service is not running")
	}

	repo, err := models.Repositories.Get(repoID)
	if err != nil {
		return errors.Wrap(err, "repository not found")
	}

	user, err := models.Auth.Users.Get(repo.UserID)
	if err != nil {
		return errors.Wrap(err, "failed to get repository owner")
	}

	// Check if repository exists in Jupyter workspace (use repo.ID for directory)
	checkCmd := i.buildCheckCommand(repo.ID)
	output, err := i.service.ExecInContainerWithOutput("bash", "-c", checkCmd)
	if err != nil {
		log.Printf("Failed to check repository existence: %v", err)
		// Try to clone if check fails
		return i.CloneRepository(repo, user)
	}

	if strings.TrimSpace(output) == "not-exists" {
		// Repository doesn't exist, clone it
		return i.CloneRepository(repo, user)
	}

	// Update existing repository (use repo.ID for directory)
	updateCmd := i.buildUpdateCommand(repo.ID)
	if err := i.service.ExecInContainer("bash", "-c", updateCmd); err != nil {
		log.Printf("Update failed for %s: %v", repoID, err)
		// If update fails, try to re-clone
		return i.CloneRepository(repo, user)
	}

	log.Printf("Repository %s updated in Jupyter workspace", repoID)
	return nil
}

// RemoveRepository removes the working copy when a repository is deleted
func (i *IPythonService) RemoveRepository(repoID string) error {
	i.mu.RLock()
	defer i.mu.RUnlock()

	if i.service == nil || !i.IsRunning() {
		// Service not running, nothing to remove
		return nil
	}

	// If repository doesn't exist in DB, still try to remove directory using the ID
	var removeID string
	repo, err := models.Repositories.Get(repoID)
	if err != nil {
		// Repository might already be deleted, use the ID directly
		removeID = repoID
	} else {
		removeID = repo.ID
	}

	removeCmd := i.buildRemoveCommand(removeID)
	if err := i.service.ExecInContainer("bash", "-c", removeCmd); err != nil {
		log.Printf("Failed to remove %s from Jupyter workspace: %v", repoID, err)
		// Don't return error as this is cleanup
	}

	log.Printf("Repository %s removed from Jupyter workspace", repoID)
	return nil
}

// buildCloneCommand builds the git clone command for Jupyter environment
func (i *IPythonService) buildCloneCommand(repoID, cloneURL, gitUserName, gitUserEmail string) string {
	// Escape repository ID for shell
	escapedID := strings.ReplaceAll(repoID, "'", "'\\''")

	// Note: In Jupyter container, the work directory is /home/jovyan/work
	return fmt.Sprintf(`
		cd /home/jovyan/work && 
		if [ -d '%s/.git' ]; then
			echo "Repository exists, updating..." &&
			cd '%s' &&
			git fetch origin || true
		else
			echo "Cloning repository..." &&
			git clone '%s' '%s' &&
			cd '%s' &&
			git config user.name "%s" &&
			git config user.email "%s"
		fi
	`, escapedID, escapedID, cloneURL, escapedID, escapedID,
		gitUserName, gitUserEmail)
}

// buildCheckCommand builds the command to check if repository exists
func (i *IPythonService) buildCheckCommand(repoID string) string {
	escapedID := strings.ReplaceAll(repoID, "'", "'\\''")

	return fmt.Sprintf(`
		cd /home/jovyan/work && 
		[ -d '%s/.git' ] && echo "exists" || echo "not-exists"
	`, escapedID)
}

// buildUpdateCommand builds the git pull command
func (i *IPythonService) buildUpdateCommand(repoID string) string {
	escapedID := strings.ReplaceAll(repoID, "'", "'\\''")

	return fmt.Sprintf(`
		cd /home/jovyan/work/%s &&
		git stash save "Auto-stash before update" &&
		git fetch origin &&
		git pull origin $(git symbolic-ref --short HEAD 2>/dev/null || echo 'master') &&
		git stash pop || true
	`, escapedID)
}

// buildRemoveCommand builds the rm command
func (i *IPythonService) buildRemoveCommand(repoID string) string {
	escapedID := strings.ReplaceAll(repoID, "'", "'\\''")
	return fmt.Sprintf(`rm -rf '/home/jovyan/work/%s'`, escapedID)
}