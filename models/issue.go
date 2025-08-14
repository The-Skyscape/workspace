package models

import (
	"github.com/The-Skyscape/devtools/pkg/application"
)

type Issue struct {
	application.Model
	Title      string
	Body       string
	Tags       string // JSON array of tags
	Status     string // "open", "closed"
	AuthorID   string // User who created the issue
	AssigneeID string
	RepoID     string
}

func (*Issue) Table() string { return "issues" }

func init() {
	// Create indexes for issues table
	go func() {
		Issues.Index("RepoID")
		Issues.Index("AuthorID")
		Issues.Index("Status")
		Issues.Index("CreatedAt DESC")
	}()
}

