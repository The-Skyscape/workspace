package models

import (
	"time"
	"github.com/The-Skyscape/devtools/pkg/application"
)

type Action struct {
	application.Model
	Type        string // "on_push", "on_pr", "on_issue", "scheduled", "manual"
	Title       string // Action title/summary
	Description string // Action description
	Trigger     string // JSON config for trigger conditions
	Script      string // Bash script or commands to execute
	Status      string // "active", "running", "completed", "failed", "disabled"
	LastRun     *time.Time `json:"last_run,omitempty"`
	Output      string // Last execution output
	RepoID      string
	UserID      string
}

func (*Action) Table() string { return "actions" }