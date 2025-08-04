// Package models contains all database models and business logic for Skyscape Workspace.
// Each model represents a database table and includes methods for CRUD operations.
package models

import (
	"bytes"
	_ "embed"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/containers"
	"github.com/The-Skyscape/devtools/pkg/database"
	"github.com/pkg/errors"
)

// Workspace represents a containerized development environment.
// Each workspace runs VS Code (code-server) in a Docker container with persistent storage.
//
// Lifecycle:
//   1. Created via NewWorkspace() - assigns port and generates unique name
//   2. Started via Start() - launches Docker container with mounted volumes
//   3. Accessed via /coder/{ID}/ - proxies to code-server on workspace.Port
//   4. Stopped via Stop() - stops container but preserves volumes
//   5. Deleted via Delete() - removes container and cleans up resources
//
// Persistence: Three volumes are mounted to preserve state between restarts:
//   - /home/coder/.config - VS Code settings and extensions
//   - /home/coder/project - User's project files
//   - /workspace/repos/{repo-id} - Git repository (if RepoID is set)
type Workspace struct {
	application.Model
	Name         string    // Human-readable name (e.g., "workspace-1234567890")
	UserID       string    // Owner of the workspace
	Port         int       // Local port for VS Code server (8000-9000 range)
	Ready        bool      // Whether the container is fully started
	RepoID       string    // Optional: Associated repository ID
	LastUsed     time.Time // Track when workspace was last accessed
	CreatedAt    time.Time // When the workspace was created
	ErrorMessage string    // Store any error that occurred during startup
	Status       string    // Explicit status: "starting", "running", "stopped", "error"
}

// GetStatus returns the current status of the workspace
// Priority: stored Status field > container state > default "stopped"
func (w *Workspace) GetStatus() string {
	// If we have an explicit status, use it
	if w.Status != "" {
		return w.Status
	}
	
	// Fall back to checking container state
	s := w.Service()
	if s.IsRunning() {
		if w.Ready {
			return "running"
		}
		return "starting"
	}
	return "stopped"
}

func (*Workspace) Table() string { return "code_workspaces" }

func (w *Workspace) Service() *containers.Service {
	containerName := fmt.Sprintf("workspace-%s", w.ID)
	return &containers.Service{
		Host:    containers.Local(),
		Name:    containerName,
		Image:   "codercom/code-server:latest",
		Command: "--auth none --bind-addr 0.0.0.0:" + strconv.Itoa(w.Port),

		Mounts: map[string]string{
			fmt.Sprintf("%s/workspaces/%s/.config", database.DataDir(), w.ID): "/home/coder/.config",
			fmt.Sprintf("%s/workspaces/%s/project", database.DataDir(), w.ID): "/home/coder/project",
			fmt.Sprintf("%s/repos/%s", database.DataDir(), w.RepoID):          "/home/coder/repo",
		},
		Ports: map[int]int{
			w.Port: w.Port,
		},
		Env: map[string]string{
			"PORT":           strconv.Itoa(w.Port),
			"WORKSPACE_ID":   w.ID,
			"WORKSPACE_NAME": w.Name,
			"USER_ID":        w.UserID,
		},
	}
}

//go:embed resources/prepare-workspace.sh
var prepareWorkspace string

//go:embed resources/setup-workspace.sh
var setupWorkspace string

//go:embed resources/clone-git-repo.sh
var cloneRepository string

func (w *Workspace) Start(u *authentication.User, repo *GitRepo) error {
	// Set status to starting
	w.Status = "starting"
	w.ErrorMessage = ""
	if err := Workspaces.Update(w); err != nil {
		log.Printf("Failed to update workspace status: %v", err)
	}

	s := w.Service()
	if s.IsRunning() {
		log.Printf("Workspace %s already running", w.Name)
		w.Status = "running"
		w.Ready = true
		return Workspaces.Update(w)
	}

	host := containers.Local()
	host.SetStdout(os.Stdout)
	host.SetStderr(os.Stderr)
	
	// Prepare workspace directory
	if err := host.Exec("bash", "-c", fmt.Sprintf(prepareWorkspace, database.DataDir(), w.ID)); err != nil {
		w.Status = "error"
		w.ErrorMessage = "Failed to prepare workspace directory"
		Workspaces.Update(w)
		return errors.Wrap(err, "failed to prepare workspace ")
	}

	// Launch container
	if err := host.Launch(s); err != nil {
		w.Status = "error"
		w.ErrorMessage = "Failed to start container: " + err.Error()
		Workspaces.Update(w)
		return errors.Wrap(err, "failed to start workspace ")
	}

	// Setup workspace environment
	if err := host.Exec("bash", "-c", setupWorkspace); err != nil {
		w.Status = "error"
		w.ErrorMessage = "Failed to setup workspace environment"
		Workspaces.Update(w)
		return errors.Wrap(err, "failed to setup workspace: ")
	}

	// Clone repository if provided
	if repo != nil {
		if token, err := NewAccessToken(time.Now().Add(100_000 * time.Hour)); err == nil {
			w.Run(fmt.Sprintf(cloneRepository, token.ID, token.Secret, w.RepoID, u.Name, u.Email))
		} else {
			log.Println("Failed to create access token:", err)
		}
	}

	// Mark as ready and running
	w.Ready = true
	w.Status = "running"
	w.LastUsed = time.Now()
	return Workspaces.Update(w)
}

// Stop stops the workspace container
func (w *Workspace) Stop() error {
	s := w.Service()
	if !s.IsRunning() {
		log.Printf("Workspace %s is not running", w.Name)
		w.Status = "stopped"
		w.Ready = false
		return Workspaces.Update(w)
	}

	if err := s.Stop(); err != nil {
		return errors.Wrap(err, "failed to stop workspace")
	}

	w.Ready = false
	w.Status = "stopped"
	return Workspaces.Update(w)
}

// Delete stops and removes the workspace container and data
func (w *Workspace) Delete() error {
	s := w.Service()
	
	// Stop container if running
	if s.IsRunning() {
		if err := s.Stop(); err != nil {
			log.Printf("Failed to stop workspace %s: %v", w.Name, err)
		}
	}

	// Remove container
	if err := s.Remove(); err != nil {
		log.Printf("Failed to remove workspace container %s: %v", w.Name, err)
	}

	// Delete from database
	return Workspaces.Delete(w)
}

func (w *Workspace) Run(cmd string) (stdout bytes.Buffer, err error) {
	s := w.Service()
	containerName := fmt.Sprintf("workspace-%s", w.ID)

	s.SetStdout(&stdout)
	cmd = strings.ReplaceAll(cmd, "\n", "; ")
	cmd = strings.ReplaceAll(cmd, "; ;", ";")
	return stdout, s.Exec("docker", "exec", containerName, "bash", "-c", cmd)
}