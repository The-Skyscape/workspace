package models

import (
	"github.com/The-Skyscape/devtools/pkg/application"
)

type Comment struct {
	application.Model
	Body       string
	AuthorID   string
	RepoID     string
	IssueID    string // Can be empty if it's a PR comment
	PRID       string // Can be empty if it's an issue comment
	LineNumber int    // For inline code comments on PRs
	FilePath   string // For inline code comments on PRs
	EntityID   string // Generic entity ID for future use
	EntityType string // "issue", "pr", etc.
}

func (*Comment) Table() string { return "comments" }

func init() {
	// Create indexes for comments table
	go func() {
		Comments.Index("IssueID")
		Comments.Index("PRID")
		Comments.Index("AuthorID")
		Comments.Index("CreatedAt DESC")
	}()
}


// GetIssueComments returns all comments for an issue
func GetIssueComments(issueID string) ([]*Comment, error) {
	return Comments.Search("WHERE IssueID = ? ORDER BY CreatedAt ASC", issueID)
}

// GetPRComments returns all comments for a pull request
func GetPRComments(prID string) ([]*Comment, error) {
	return Comments.Search("WHERE PRID = ? ORDER BY CreatedAt ASC", prID)
}

// CreateIssueComment creates a new comment on an issue
func CreateIssueComment(issueID, repoID, authorID, body string) (*Comment, error) {
	comment := &Comment{
		Body:     body,
		AuthorID: authorID,
		RepoID:   repoID,
		IssueID:  issueID,
	}
	return Comments.Insert(comment)
}

// CreatePRComment creates a new comment on a pull request
func CreatePRComment(prID, repoID, authorID, body string) (*Comment, error) {
	comment := &Comment{
		Body:     body,
		AuthorID: authorID,
		RepoID:   repoID,
		PRID:     prID,
	}
	return Comments.Insert(comment)
}

// CreatePRLineComment creates an inline comment on a specific line in a PR
func CreatePRLineComment(prID, repoID, authorID, body, filePath string, lineNumber int) (*Comment, error) {
	comment := &Comment{
		Body:       body,
		AuthorID:   authorID,
		RepoID:     repoID,
		PRID:       prID,
		FilePath:   filePath,
		LineNumber: lineNumber,
	}
	return Comments.Insert(comment)
}