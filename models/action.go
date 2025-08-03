package models

import (
	"github.com/The-Skyscape/devtools/pkg/application"
)

type Action struct {
	application.Model
	Type      string    // "conversation", "pr_generation", "code_review", "summarization"
	Title     string    // Action title/summary
	Question  string    // User's original question
	Response  string    // AI response
	Status    string    // "active", "completed", "failed"
	RepoID    string
	UserID    string
}

func (*Action) Table() string { return "actions" }