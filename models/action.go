package models

import (
	"time"
	"github.com/The-Skyscape/devtools/pkg/application"
)

type Action struct {
	application.Model
	Type      string    // "pr_generation", "qa", "summarization"
	Content   string    // The AI action content
	RepoID    string
	UserID    string
	Timestamp time.Time
}

func (*Action) Table() string { return "actions" }