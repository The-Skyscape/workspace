package services

import (
	"fmt"
	"log"
	"os"
	"sync"

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

	// Check if already running
	if i.IsRunning() {
		log.Println("IPython service already running")
		return nil
	}

	// Ensure notebooks directory exists
	if err := os.MkdirAll(i.config.NotebooksDir, 0755); err != nil {
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

	i.service = &containers.Service{
		Host:          host,
		Name:          i.config.ContainerName,
		Image:         "jupyter/datascience-notebook:latest",
		Network:       "bridge",
		RestartPolicy: "always",
		Command:       command,
		Ports: map[int]int{
			8888: i.config.Port,
		},
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

	// Start the service
	return i.Start()
}

// Start starts the IPython service
func (i *IPythonService) Start() error {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.service == nil {
		return errors.New("IPython service not initialized")
	}

	if i.IsRunning() {
		return nil
	}

	log.Printf("Starting IPython/Jupyter service on port %d", i.config.Port)
	
	// Launch the container
	if err := containers.Launch(i.service.Host, i.service); err != nil {
		return errors.Wrap(err, "failed to launch IPython container")
	}

	// Wait for Jupyter to be ready
	if err := i.service.WaitForReady(60, func() error {
		// Check if Jupyter is responding
		return i.service.Host.Exec("curl", "-f", fmt.Sprintf("http://localhost:%d/api", i.config.Port))
	}); err != nil {
		log.Printf("Warning: Jupyter may not be fully ready: %v", err)
	}

	log.Printf("IPython/Jupyter service started successfully")
	log.Printf("Access Jupyter Lab at: http://localhost:%d", i.config.Port)
	if !i.config.EnableAuth {
		log.Printf("Warning: Authentication is disabled for development")
	}

	return nil
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