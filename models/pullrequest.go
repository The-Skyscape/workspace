package models

import "github.com/The-Skyscape/devtools/pkg/application"

type PullRequest struct {
	application.Model
	Title         string
	Body          string
	RepoID        string
	AuthorID      string
	BaseBranch    string
	CompareBranch string
	Status        string // "draft", "open", "merged", "closed"
}

func (*PullRequest) Table() string { return "pull_requests" }