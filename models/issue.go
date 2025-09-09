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
	Status     IssueStatus // "open", "closed", "in_progress", "resolved"
	Column     string      // Kanban column: "todo", "in_progress", "done"
	Priority   IssuePriority // 1-10, 1 being highest
	AuthorID   string      // User who created the issue
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

// IssueStatus represents the status of an issue
type IssueStatus string

const (
	IssueStatusOpen       IssueStatus = "open"
	IssueStatusClosed     IssueStatus = "closed"
	IssueStatusInProgress IssueStatus = "in_progress"
	IssueStatusResolved   IssueStatus = "resolved"
)

// IssuePriority represents the priority level (1-10, 1 being highest)
type IssuePriority int

const (
	PriorityCritical IssuePriority = 1
	PriorityHigh     IssuePriority = 3
	PriorityMedium   IssuePriority = 5
	PriorityLow      IssuePriority = 8
	PriorityNone     IssuePriority = 10
)

func (*Issue) Table() string { return "issues" }

// Labels returns all labels (tags) for this issue
func (i *Issue) Labels() ([]*TagDefinition, error) {
	return GetIssueLabels(i.ID)
}

// LabelNames returns a comma-separated string of label names for form inputs
func (i *Issue) LabelNames() string {
	labels, err := i.Labels()
	if err != nil || len(labels) == 0 {
		return ""
	}
	
	labelNames := make([]string, len(labels))
	for idx, label := range labels {
		labelNames[idx] = label.Name
	}
	return strings.Join(labelNames, ", ")
}

// HasLabel checks if the issue has a specific label
func (i *Issue) HasLabel(labelName string) bool {
	labels, err := i.Labels()
	if err != nil {
		return false
	}
	
	labelName = strings.ToLower(strings.TrimSpace(labelName))
	for _, label := range labels {
		if strings.ToLower(label.Name) == labelName {
			return true
		}
	}
	return false
}

// Tags returns the legacy IssueTag objects for backward compatibility with templates
func (i *Issue) Tags() ([]*IssueTag, error) {
	// Get the modern labels
	labels, err := i.Labels()
	if err != nil {
		return nil, err
	}
	
	// Convert to legacy IssueTags
	tags := make([]*IssueTag, len(labels))
	for idx, label := range labels {
		tags[idx] = &IssueTag{
			IssueID: i.ID,
			Tag:     label.Name,
		}
	}
	
	return tags, nil
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
func GetRepoIssuesPaginated(repoID string, includeClosed bool, limit, offset int) ([]*Issue, int, error) {
	condition := "WHERE RepoID = ?"
	args := []interface{}{repoID}
	
	if !includeClosed {
		condition += " AND Status = ?"
		args = append(args, IssueStatusOpen)
	}
	
	condition += " ORDER BY Priority, CreatedAt DESC"
	
	return Issues.SearchPaginated(condition, limit, offset, args...)
}

// CreateIssue creates a new issue with proper defaults
func CreateIssue(title, body string, authorID, repoID string) (*Issue, error) {
	issue := &Issue{
		Title:    title,
		Body:     body,
		Status:   IssueStatusOpen,
		Column:   "todo",
		Priority: PriorityMedium,
		AuthorID: authorID,
		RepoID:   repoID,
	}
	
	inserted, err := Issues.Insert(issue)
	if err != nil {
		return nil, err
	}
	issue = inserted
	
	// Create event for issue creation
	_, err = CreateEvent(
		EventIssueCreated,
		repoID,
		authorID,
		"issue",
		issue.ID,
		map[string]string{
			"title": title,
			"body":  body,
		},
		int(PriorityHigh),
	)
	
	return issue, err
}

