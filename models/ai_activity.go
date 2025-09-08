package models

import (
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
)

// AIActivity tracks AI task execution history
type AIActivity struct {
	application.Model
	Type        string    // issue_triage, pr_review, daily_report, etc.
	RepoID      string    // Repository ID
	RepoName    string    // Repository name for display
	EntityType  string    // issue, pull_request, etc.
	EntityID    string    // Issue/PR ID
	Description string    // Human-readable description
	Success     bool      // Whether the task succeeded
	Duration    int64     // Duration in milliseconds
	CreatedAt   time.Time // When the activity occurred
}

// Table returns the database table name
func (*AIActivity) Table() string {
	return "ai_activities"
}


// GetRecentForRepo returns recent AI activities for a repository
func GetRecentAIActivitiesForRepo(repoID string, limit int) ([]*AIActivity, error) {
	return AIActivities.Search("WHERE RepoID = ? ORDER BY CreatedAt DESC LIMIT ?", repoID, limit)
}

// GetRecentActivities returns recent AI activities across all repositories
func GetRecentAIActivities(limit int) ([]*AIActivity, error) {
	return AIActivities.Search("ORDER BY CreatedAt DESC LIMIT ?", limit)
}

// LogActivity creates a new AI activity record
func LogAIActivity(activityType, repoID, repoName, entityType, entityID, description string, success bool, duration int64) (*AIActivity, error) {
	activity := &AIActivity{
		Type:        activityType,
		RepoID:      repoID,
		RepoName:    repoName,
		EntityType:  entityType,
		EntityID:    entityID,
		Description: description,
		Success:     success,
		Duration:    duration,
		CreatedAt:   time.Now(),
	}
	return AIActivities.Insert(activity)
}