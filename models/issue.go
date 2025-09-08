package models

import (
	"strings"
	"time"
	
	"github.com/The-Skyscape/devtools/pkg/application"
)

type Issue struct {
	application.Model
	Title      string
	Body       string
	Status     string // "open", "closed"
	Column     string // Kanban column: "" (default/todo), "in_progress", "done"
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

// Tags returns all IssueTag objects for this issue
func (i *Issue) Tags() ([]*IssueTag, error) {
	return IssueTags.Search("WHERE IssueID = ? ORDER BY Tag", i.ID)
}

// TagsString returns a comma-separated string of tags for form inputs
func (i *Issue) TagsString() string {
	tags, err := i.Tags()
	if err != nil || len(tags) == 0 {
		return ""
	}
	
	tagStrings := make([]string, len(tags))
	for idx, tag := range tags {
		tagStrings[idx] = tag.Tag
	}
	return strings.Join(tagStrings, ", ")
}

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

