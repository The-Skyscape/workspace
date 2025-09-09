package models

import (
	"github.com/The-Skyscape/devtools/pkg/application"
)

type Comment struct {
	application.Model
	Body       string
	AuthorID   string
	RepoID     string
	EntityType string // "issue", "pr", "commit", etc.
	EntityID   string // ID of the entity being commented on
	
	// Optional fields for code comments
	FilePath   string // For inline code comments
	LineNumber int    // For inline code comments (0 means not a line comment)
	CommitSHA  string // For commit-specific comments
}

func (*Comment) Table() string { return "comments" }

func init() {
	// Create indexes for comments table
	go func() {
		Comments.Index("EntityType")
		Comments.Index("EntityID")
		Comments.Index("AuthorID")
		Comments.Index("RepoID")
		Comments.Index("CreatedAt DESC")
		// Composite index for entity lookups
		Comments.Index("EntityType, EntityID, CreatedAt")
	}()
}


// GetEntityComments returns all comments for an entity
func GetEntityComments(entityType, entityID string) ([]*Comment, error) {
	return Comments.Search("WHERE EntityType = ? AND EntityID = ? ORDER BY CreatedAt ASC", entityType, entityID)
}

// GetIssueComments returns all comments for an issue
func GetIssueComments(issueID string) ([]*Comment, error) {
	return GetEntityComments("issue", issueID)
}

// GetPRComments returns all comments for a pull request
func GetPRComments(prID string) ([]*Comment, error) {
	return GetEntityComments("pr", prID)
}

// CreateComment creates a new comment on any entity
func CreateComment(entityType, entityID, repoID, authorID, body string) (*Comment, error) {
	comment := &Comment{
		Body:       body,
		AuthorID:   authorID,
		RepoID:     repoID,
		EntityType: entityType,
		EntityID:   entityID,
	}
	return Comments.Insert(comment)
}

// CreateIssueComment creates a new comment on an issue
func CreateIssueComment(issueID, repoID, authorID, body string) (*Comment, error) {
	return CreateComment("issue", issueID, repoID, authorID, body)
}

// CreatePRComment creates a new comment on a pull request
func CreatePRComment(prID, repoID, authorID, body string) (*Comment, error) {
	return CreateComment("pr", prID, repoID, authorID, body)
}

// CreateLineComment creates an inline comment on a specific line
func CreateLineComment(entityType, entityID, repoID, authorID, body, filePath string, lineNumber int) (*Comment, error) {
	comment := &Comment{
		Body:       body,
		AuthorID:   authorID,
		RepoID:     repoID,
		EntityType: entityType,
		EntityID:   entityID,
		FilePath:   filePath,
		LineNumber: lineNumber,
	}
	return Comments.Insert(comment)
}

// Author returns the user who authored this comment
func (c *Comment) Author() (*User, error) {
	if c.AuthorID == "" {
		return nil, nil
	}
	return Users.Get(c.AuthorID)
}

// Repository returns the repository this comment belongs to
func (c *Comment) Repository() (*Repository, error) {
	if c.RepoID == "" {
		return nil, nil
	}
	return Repos.Get(c.RepoID)
}