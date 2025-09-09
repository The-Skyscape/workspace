package models

import (
	"testing"
	"time"
	
	"github.com/The-Skyscape/devtools/pkg/testutils"
)

func TestEventModel(t *testing.T) {
	// Setup test database
	db := SetupTestDB(t)
	defer CleanupTestDB(t, db)
	
	t.Run("CreateEvent", func(t *testing.T) {
		// Create test data
		user := CreateTestUser(t, db, "event@example.com")
		repo := createTestRepository(t, "event-repo", user.ID)
		
		// Create event with metadata
		metadata := map[string]string{
			"title":       "Test Issue",
			"description": "Test description",
			"severity":    "high",
		}
		
		event, err := CreateEvent(
			EventIssueCreated,
			repo.ID,
			user.ID,
			"issue",
			"issue-123",
			metadata,
			int(PriorityHigh),
		)
		
		testutils.AssertNoError(t, err)
		testutils.AssertNotNil(t, event)
		testutils.AssertEqual(t, EventIssueCreated, event.Type)
		testutils.AssertEqual(t, EventStatusPending, event.Status)
		testutils.AssertEqual(t, int(PriorityHigh), event.Priority)
		testutils.AssertEqual(t, repo.ID, event.RepoID)
		testutils.AssertEqual(t, user.ID, event.UserID)
		testutils.AssertEqual(t, "issue", event.EntityType)
		testutils.AssertEqual(t, "issue-123", event.EntityID)
		testutils.AssertEqual(t, 3, event.MaxRetries)
		
		// Verify metadata
		title, err := event.GetMetadata("title")
		testutils.AssertNoError(t, err)
		testutils.AssertEqual(t, "Test Issue", title)
		
		severity, err := event.GetMetadata("severity")
		testutils.AssertNoError(t, err)
		testutils.AssertEqual(t, "high", severity)
		
		// Test non-existent metadata
		missing, err := event.GetMetadata("nonexistent")
		testutils.AssertNoError(t, err)
		testutils.AssertEqual(t, "", missing)
	})
	
	t.Run("EventMetadata", func(t *testing.T) {
		// Create event
		event, err := CreateEvent(
			EventPRCreated,
			"repo-1",
			"user-1",
			"pr",
			"pr-456",
			nil,
			int(PriorityMedium),
		)
		testutils.AssertNoError(t, err)
		
		// Set metadata
		err = event.SetMetadata("branch", "feature/test")
		testutils.AssertNoError(t, err)
		
		err = event.SetMetadata("commits", "5")
		testutils.AssertNoError(t, err)
		
		// Update existing metadata
		err = event.SetMetadata("commits", "6")
		testutils.AssertNoError(t, err)
		
		// Verify metadata
		branch, err := event.GetMetadata("branch")
		testutils.AssertNoError(t, err)
		testutils.AssertEqual(t, "feature/test", branch)
		
		commits, err := event.GetMetadata("commits")
		testutils.AssertNoError(t, err)
		testutils.AssertEqual(t, "6", commits)
		
		// Get all metadata
		allMetadata, err := event.Metadata()
		testutils.AssertNoError(t, err)
		testutils.AssertEqual(t, 2, len(allMetadata))
	})
	
	t.Run("EventStatusTransitions", func(t *testing.T) {
		// Create event
		event, err := CreateEvent(
			EventTestFailed,
			"repo-2",
			"user-2",
			"test",
			"test-789",
			nil,
			int(PriorityCritical),
		)
		testutils.AssertNoError(t, err)
		testutils.AssertEqual(t, EventStatusPending, event.Status)
		
		// Mark as processing
		err = event.MarkProcessing()
		testutils.AssertNoError(t, err)
		testutils.AssertEqual(t, EventStatusProcessing, event.Status)
		testutils.AssertNotNil(t, event.StartedAt)
		
		// Simulate some processing time
		time.Sleep(10 * time.Millisecond)
		
		// Mark as completed
		err = event.MarkCompleted()
		testutils.AssertNoError(t, err)
		testutils.AssertEqual(t, EventStatusCompleted, event.Status)
		testutils.AssertNotNil(t, event.CompletedAt)
		testutils.AssertNotNil(t, event.ProcessedAt)
		testutils.AssertTrue(t, event.Duration > 0)
	})
	
	t.Run("EventFailureAndRetry", func(t *testing.T) {
		// Create event
		event, err := CreateEvent(
			EventDeploymentDone,
			"repo-3",
			"user-3",
			"deployment",
			"deploy-321",
			nil,
			int(PriorityHigh),
		)
		testutils.AssertNoError(t, err)
		
		// Mark as processing
		err = event.MarkProcessing()
		testutils.AssertNoError(t, err)
		
		// Mark as failed
		err = event.MarkFailed("Connection timeout")
		testutils.AssertNoError(t, err)
		testutils.AssertEqual(t, EventStatusFailed, event.Status)
		testutils.AssertEqual(t, "Connection timeout", event.Error)
		testutils.AssertNotNil(t, event.CompletedAt)
		
		// Check if should retry
		testutils.AssertTrue(t, event.ShouldRetry())
		
		// Increment retry
		err = event.IncrementRetry()
		testutils.AssertNoError(t, err)
		testutils.AssertEqual(t, 1, event.RetryCount)
		testutils.AssertEqual(t, EventStatusRetrying, event.Status)
		testutils.AssertEqual(t, "", event.Error) // Error cleared on retry
		
		// Retry until max
		for i := 1; i < 3; i++ {
			event.Status = EventStatusFailed
			err = Events.Update(event)
			testutils.AssertNoError(t, err)
			
			err = event.IncrementRetry()
			testutils.AssertNoError(t, err)
		}
		
		// Should not retry after max retries
		event.Status = EventStatusFailed
		err = Events.Update(event)
		testutils.AssertNoError(t, err)
		testutils.AssertFalse(t, event.ShouldRetry())
	})
	
	t.Run("GetPendingEvents", func(t *testing.T) {
		// Create multiple events with different statuses
		pending1, err := CreateEvent(EventIssueCreated, "repo-4", "user-4", "issue", "i1", nil, 5)
		testutils.AssertNoError(t, err)
		
		pending2, err := CreateEvent(EventPRCreated, "repo-4", "user-4", "pr", "p1", nil, 3)
		testutils.AssertNoError(t, err)
		
		completed, err := CreateEvent(EventCommitPushed, "repo-4", "user-4", "commit", "c1", nil, 5)
		testutils.AssertNoError(t, err)
		err = completed.MarkCompleted()
		testutils.AssertNoError(t, err)
		
		retrying, err := CreateEvent(EventTestFailed, "repo-4", "user-4", "test", "t1", nil, 1)
		testutils.AssertNoError(t, err)
		retrying.Status = EventStatusRetrying
		err = Events.Update(retrying)
		testutils.AssertNoError(t, err)
		
		// Get pending events
		pendingEvents, err := GetPendingEvents(10)
		testutils.AssertNoError(t, err)
		
		// Should include pending and retrying, but not completed
		pendingCount := 0
		for _, e := range pendingEvents {
			if e.ID == pending1.ID || e.ID == pending2.ID || e.ID == retrying.ID {
				pendingCount++
			}
			testutils.AssertTrue(t, e.ID != completed.ID)
		}
		testutils.AssertTrue(t, pendingCount >= 3)
	})
	
	t.Run("EventCorrelation", func(t *testing.T) {
		correlationID := "correlation-123"
		
		// Create related events with same correlation ID
		event1, err := CreateEvent(EventIssueCreated, "repo-5", "user-5", "issue", "i1", nil, 5)
		testutils.AssertNoError(t, err)
		event1.CorrelationID = correlationID
		err = Events.Update(event1)
		testutils.AssertNoError(t, err)
		
		event2, err := CreateEvent(EventCommentAdded, "repo-5", "user-5", "issue", "i1", nil, 5)
		testutils.AssertNoError(t, err)
		event2.CorrelationID = correlationID
		err = Events.Update(event2)
		testutils.AssertNoError(t, err)
		
		event3, err := CreateEvent(EventIssueClosed, "repo-5", "user-5", "issue", "i1", nil, 5)
		testutils.AssertNoError(t, err)
		event3.CorrelationID = correlationID
		err = Events.Update(event3)
		testutils.AssertNoError(t, err)
		
		// Query by correlation ID
		correlated, err := Events.Search("WHERE CorrelationID = ? ORDER BY CreatedAt", correlationID)
		testutils.AssertNoError(t, err)
		testutils.AssertEqual(t, 3, len(correlated))
		
		// Verify they're all related to the same issue
		for _, e := range correlated {
			testutils.AssertEqual(t, "issue", e.EntityType)
			testutils.AssertEqual(t, "i1", e.EntityID)
			testutils.AssertEqual(t, correlationID, e.CorrelationID)
		}
	})
	
	t.Run("GetRepoEvents", func(t *testing.T) {
		repoID := "test-repo-events"
		
		// Create events for specific repo
		for i := 0; i < 5; i++ {
			_, err := CreateEvent(
				EventCommitPushed,
				repoID,
				"user-6",
				"commit",
				string(rune('a'+i)),
				nil,
				5,
			)
			testutils.AssertNoError(t, err)
		}
		
		// Get repo events
		repoEvents, err := GetRepoEvents(repoID, 3)
		testutils.AssertNoError(t, err)
		testutils.AssertEqual(t, 3, len(repoEvents))
		
		// Verify all are from the same repo
		for _, e := range repoEvents {
			testutils.AssertEqual(t, repoID, e.RepoID)
		}
	})
	
	t.Run("GetEntityEvents", func(t *testing.T) {
		entityType := "pr"
		entityID := "pr-test-123"
		
		// Create events for specific entity
		events := []EventType{
			EventPRCreated,
			EventPRUpdated,
			EventCommentAdded,
			EventPRMerged,
		}
		
		for _, eventType := range events {
			_, err := CreateEvent(
				eventType,
				"repo-7",
				"user-7",
				entityType,
				entityID,
				nil,
				5,
			)
			testutils.AssertNoError(t, err)
		}
		
		// Get entity events
		entityEvents, err := GetEntityEvents(entityType, entityID, 10)
		testutils.AssertNoError(t, err)
		testutils.AssertTrue(t, len(entityEvents) >= 4)
		
		// Verify all are for the same entity
		for _, e := range entityEvents {
			if e.EntityType == entityType && e.EntityID == entityID {
				// This is one of our events
				testutils.AssertEqual(t, entityType, e.EntityType)
				testutils.AssertEqual(t, entityID, e.EntityID)
			}
		}
	})
}