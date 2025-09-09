package models

import (
	"log"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
)

// EventMetadata stores key-value metadata for events
type EventMetadata struct {
	application.Model
	EventID string
	Key     string
	Value   string
}

func (*EventMetadata) Table() string { return "event_metadata" }

// EventType represents different types of system events
type EventType string

const (
	EventIssueCreated    EventType = "issue_created"
	EventIssueUpdated    EventType = "issue_updated"
	EventIssueClosed     EventType = "issue_closed"
	EventIssueReopened   EventType = "issue_reopened"
	EventPRCreated       EventType = "pr_created"
	EventPRUpdated       EventType = "pr_updated"
	EventPRMerged        EventType = "pr_merged"
	EventPRClosed        EventType = "pr_closed"
	EventCommentAdded    EventType = "comment_added"
	EventRepoCreated     EventType = "repo_created"
	EventCommitPushed    EventType = "commit_pushed"
	EventTestFailed      EventType = "test_failed"
	EventBuildCompleted  EventType = "build_completed"
	EventDeploymentDone  EventType = "deployment_done"
	EventDailySchedule   EventType = "daily_schedule"
	EventWeeklySchedule  EventType = "weekly_schedule"
	EventStaleCheck      EventType = "stale_check"
	EventSecurityScan    EventType = "security_scan"
	EventDependencyCheck EventType = "dependency_check"
)

// EventStatus represents the processing status of an event
type EventStatus string

const (
	EventStatusPending    EventStatus = "pending"
	EventStatusProcessing EventStatus = "processing"
	EventStatusCompleted  EventStatus = "completed"
	EventStatusFailed     EventStatus = "failed"
	EventStatusRetrying   EventStatus = "retrying"
	EventStatusCancelled  EventStatus = "cancelled"
	EventStatusSkipped    EventStatus = "skipped"
)

// Event represents a system event that can trigger actions
type Event struct {
	application.Model
	Type         EventType
	Status       EventStatus
	Priority     int // 1-10, 1 being highest
	EntityType   string // "issue", "pr", "repo", etc.
	EntityID     string
	UserID       string
	RepoID       string
	ProcessedAt  *time.Time
	StartedAt    *time.Time
	CompletedAt  *time.Time
	RetryCount   int
	MaxRetries   int
	Error        string
	CorrelationID string // For linking related events
	Duration     int64 // Processing duration in milliseconds
}

func (*Event) Table() string { return "events" }

// Metadata returns all metadata entries for this event
func (e *Event) Metadata() ([]*EventMetadata, error) {
	return EventMetadataEntries.Search("WHERE EventID = ?", e.ID)
}

// GetMetadata returns a specific metadata value
func (e *Event) GetMetadata(key string) (string, error) {
	entries, err := EventMetadataEntries.Search("WHERE EventID = ? AND Key = ?", e.ID, key)
	if err != nil {
		return "", err
	}
	if len(entries) > 0 {
		return entries[0].Value, nil
	}
	return "", nil
}

// SetMetadata sets a metadata key-value pair
func (e *Event) SetMetadata(key, value string) error {
	// Check if key exists
	entries, err := EventMetadataEntries.Search("WHERE EventID = ? AND Key = ?", e.ID, key)
	if err != nil {
		return err
	}
	
	if len(entries) > 0 {
		// Update existing
		entries[0].Value = value
		return EventMetadataEntries.Update(entries[0])
	}
	
	// Create new
	_, err = EventMetadataEntries.Insert(&EventMetadata{
		EventID: e.ID,
		Key:     key,
		Value:   value,
	})
	return err
}

// Repository returns the repository for this event
func (e *Event) Repository() (*Repository, error) {
	if e.RepoID == "" {
		return nil, nil
	}
	return Repos.Get(e.RepoID)
}

// MarkProcessing updates the event status to processing
func (e *Event) MarkProcessing() error {
	now := time.Now()
	e.Status = EventStatusProcessing
	e.StartedAt = &now
	return Events.Update(e)
}

// MarkCompleted updates the event status to completed
func (e *Event) MarkCompleted() error {
	now := time.Now()
	e.Status = EventStatusCompleted
	e.CompletedAt = &now
	e.ProcessedAt = &now
	if e.StartedAt != nil {
		e.Duration = now.Sub(*e.StartedAt).Milliseconds()
	}
	return Events.Update(e)
}

// MarkFailed updates the event status to failed
func (e *Event) MarkFailed(errorMsg string) error {
	now := time.Now()
	e.Status = EventStatusFailed
	e.CompletedAt = &now
	e.Error = errorMsg
	if e.StartedAt != nil {
		e.Duration = now.Sub(*e.StartedAt).Milliseconds()
	}
	return Events.Update(e)
}

// ShouldRetry determines if the event should be retried
func (e *Event) ShouldRetry() bool {
	return e.RetryCount < e.MaxRetries && e.Status == EventStatusFailed
}

// IncrementRetry increments the retry counter
func (e *Event) IncrementRetry() error {
	e.RetryCount++
	e.Status = EventStatusRetrying
	e.Error = "" // Clear previous error
	return Events.Update(e)
}

func init() {
	// Create indexes for events table
	go func() {
		Events.Index("Type")
		Events.Index("Status")
		Events.Index("RepoID")
		Events.Index("EntityType")
		Events.Index("EntityID")
		Events.Index("UserID")
		Events.Index("Priority")
		Events.Index("CreatedAt DESC")
		Events.Index("ProcessedAt")
		Events.Index("CorrelationID")
		// Composite indexes for common queries
		Events.Index("Status, Priority DESC, CreatedAt")
		Events.Index("RepoID, Type, CreatedAt DESC")
		Events.Index("EntityType, EntityID, CreatedAt DESC")
	}()
	
	// Create indexes for event_metadata table
	go func() {
		EventMetadataEntries.Index("EventID")
		EventMetadataEntries.Index("Key")
		// Composite index for unique constraint and fast lookups
		EventMetadataEntries.Index("EventID, Key")
	}()
}

// GetPendingEvents returns events that need processing
func GetPendingEvents(limit int) ([]*Event, error) {
	return Events.Search("WHERE Status IN ('pending', 'retrying') ORDER BY Priority, CreatedAt LIMIT ?", limit)
}

// GetRepoEvents returns events for a specific repository
func GetRepoEvents(repoID string, limit int) ([]*Event, error) {
	return Events.Search("WHERE RepoID = ? ORDER BY CreatedAt DESC LIMIT ?", repoID, limit)
}

// GetEntityEvents returns events for a specific entity
func GetEntityEvents(entityType, entityID string, limit int) ([]*Event, error) {
	return Events.Search("WHERE EntityType = ? AND EntityID = ? ORDER BY CreatedAt DESC LIMIT ?", entityType, entityID, limit)
}

// CreateEvent creates a new event with metadata
func CreateEvent(eventType EventType, repoID, userID, entityType, entityID string, metadata map[string]string, priority int) (*Event, error) {
	event := &Event{
		Type:       eventType,
		Status:     EventStatusPending,
		Priority:   priority,
		EntityType: entityType,
		EntityID:   entityID,
		UserID:     userID,
		RepoID:     repoID,
		MaxRetries: 3,
	}
	
	inserted, err := Events.Insert(event)
	if err != nil {
		return nil, err
	}
	event = inserted
	
	// Add metadata entries
	for key, value := range metadata {
		if err := event.SetMetadata(key, value); err != nil {
			// Log error but don't fail event creation
			log.Printf("Failed to set metadata %s=%s for event %s: %v", key, value, event.ID, err)
		}
	}
	
	return event, nil
}