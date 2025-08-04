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

type Workspace struct {
	application.Model
	Name   string
	Port   int
	Ready  bool
	RepoID string
}

// Status returns the current status of the workspace based on container state
func (w *Workspace) Status() string {
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
	return &containers.Service{
		Host:    containers.Local(),
		Name:    w.Name,
		Image:   "codercom/code-server:latest",
		Command: "--auth none",

		Mounts: map[string]string{
			fmt.Sprintf("%s/workspaces/%s/.config:/home/coder/.config", database.DataDir(), w.Name): "/home/coder/.config",
			fmt.Sprintf("%s/workspaces/%s/project:/home/coder/project", database.DataDir(), w.Name): "/home/coder/project",
		},
		Ports: map[int]int{
			w.Port: w.Port,
		},
		Env: map[string]string{
			"PORT": strconv.Itoa(w.Port),
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
	s := w.Service()
	if s.IsRunning() {
		log.Printf("Workspace %s already running", w.Name)
		return nil
	}

	host := containers.Local()
	host.SetStdout(os.Stdout)
	host.SetStderr(os.Stderr)
	if err := host.Exec("bash", "-c", fmt.Sprintf(prepareWorkspace, database.DataDir(), w.Name)); err != nil {
		return errors.Wrap(err, "failed to prepare workspace ")
	}

	if err := host.Launch(s); err != nil {
		return errors.Wrap(err, "failed to start workspace ")
	}

	if err := host.Exec("bash", "-c", setupWorkspace); err != nil {
		return errors.Wrap(err, "failed to setup workspace: ")
	}

	if repo != nil {
		if token, err := NewAccessToken(time.Now().Add(100_000 * time.Hour)); err == nil {
			w.Run(fmt.Sprintf(cloneRepository, token.ID, token.Secret, w.RepoID, u.Name, u.Email))
		} else {
			log.Println("Failed to create access token:", err)
		}
	} else {
		log.Println("Failed to load repo:", repo, nil)
	}

	w.Ready = true
	return Workspaces.Update(w)
}

// Stop stops the workspace container
func (w *Workspace) Stop() error {
	s := w.Service()
	if !s.IsRunning() {
		log.Printf("Workspace %s is not running", w.Name)
		return nil
	}

	if err := s.Stop(); err != nil {
		return errors.Wrap(err, "failed to stop workspace")
	}

	w.Ready = false
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

	s.SetStdout(&stdout)
	cmd = strings.ReplaceAll(cmd, "\n", "; ")
	cmd = strings.ReplaceAll(cmd, "; ;", ";")
	return stdout, s.Exec("docker", "exec", w.Name, "bash", "-c", cmd)
}