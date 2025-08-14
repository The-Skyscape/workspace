package models

import (
	"github.com/The-Skyscape/devtools/pkg/application"
)

type Activity struct {
	application.Model
	Type        string // "repo_created", "repo_updated", "issue_created", "pr_created", "action_run", "workspace_launched"
	Title       string // Human-readable activity title
	Description string // Activity description
	UserID      string // User who performed the activity
	RepoID      string `json:"repo_id,omitempty"` // Repository related to activity (optional)
	EntityType  string `json:"entity_type,omitempty"` // "repository", "issue", "pr", "action", "workspace"
	EntityID    string `json:"entity_id,omitempty"` // ID of the related entity
	Metadata    string `json:"metadata,omitempty"` // JSON metadata for activity
}

func (*Activity) Table() string { return "activities" }

func init() {
	// Create indexes for activities table
	go func() {
		Activities.Index("RepoID")
		Activities.Index("UserID")
		Activities.Index("Type")
		Activities.Index("CreatedAt DESC")
	}()
}


// LogActivity creates a new activity record
func LogActivity(activityType, title, description, userID, repoID, entityType, entityID string) error {
	activity := &Activity{
		Type:        activityType,
		Title:       title,
		Description: description,
		UserID:      userID,
		RepoID:      repoID,
		EntityType:  entityType,
		EntityID:    entityID,
	}

	_, err := Activities.Insert(activity)
	return err
}