package models

import (
	"time"
	
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
	
	// GitHub Sync Fields
	GitHubNumber  int       // GitHub issue number
	GitHubID      int64     // GitHub issue ID
	GitHubURL     string    // GitHub issue URL
	GitHubState   string    // GitHub state (open, closed)
	LastSyncAt    time.Time // Last sync timestamp
	SyncStatus    string    // "synced", "pending", "conflict", "local_only"
	SyncDirection string    // "push", "pull", "both", "none"
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

// GetRepoIssuesPaginated returns paginated issues for a repository
func GetRepoIssuesPaginated(repoID string, includeClosted bool, limit, offset int) ([]*Issue, int, error) {
	condition := "WHERE RepoID = ?"
	args := []interface{}{repoID}
	
	if !includeClosted {
		condition += " AND Status = 'open'"
	}
	
	condition += " ORDER BY CreatedAt DESC"
	
	return Issues.SearchPaginated(condition, limit, offset, args...)
}

