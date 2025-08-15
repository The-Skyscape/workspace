package models

import (
	"github.com/The-Skyscape/devtools/pkg/application"
)

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

func init() {
	// Create indexes for pull requests table
	go func() {
		PullRequests.Index("RepoID")
		PullRequests.Index("AuthorID")
		PullRequests.Index("Status")
		PullRequests.Index("CreatedAt DESC")
	}()
}

// GetRepoPRsPaginated returns paginated pull requests for a repository
func GetRepoPRsPaginated(repoID string, includeClosed bool, limit, offset int) ([]*PullRequest, int, error) {
	condition := "WHERE RepoID = ?"
	args := []interface{}{repoID}
	
	if !includeClosed {
		condition += " AND Status = 'open'"
	}
	
	condition += " ORDER BY CreatedAt DESC"
	
	return PullRequests.SearchPaginated(condition, limit, offset, args...)
}

