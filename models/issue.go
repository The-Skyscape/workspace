package models

import "github.com/The-Skyscape/devtools/pkg/application"

type Issue struct {
	application.Model
	Title      string
	Body       string
	Tags       string // JSON array of tags
	Status     string // "open", "closed"
	AssigneeID string
	RepoID     string
}

func (*Issue) Table() string { return "issues" }