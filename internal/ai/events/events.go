package events

import (
	"context"
	"fmt"
	"log"
	"time"
	"workspace/internal/ai/queue"
)

// EventType represents the type of event
type EventType string

const (
	EventIssueCreated   EventType = "issue_created"
	EventIssueUpdated   EventType = "issue_updated"
	EventIssueClosed    EventType = "issue_closed"
	EventPRCreated      EventType = "pr_created"
	EventPRUpdated      EventType = "pr_updated"
	EventPRMerged       EventType = "pr_merged"
	EventCommitPushed   EventType = "commit_pushed"
	EventTestFailed     EventType = "test_failed"
	EventDeploymentDone EventType = "deployment_done"
	EventBuildCompleted EventType = "build_completed"
)

// Event represents an event that can trigger AI actions
type Event struct {
	Type         EventType
	RepositoryID string
	UserID       string
	EntityID     string
	Metadata     map[string]any
	Timestamp    time.Time
}

// EventHandler handles events and triggers appropriate AI actions
type EventHandler struct {
	queue *queue.Queue
}

// NewEventHandler creates a new event handler
func NewEventHandler(q *queue.Queue) *EventHandler {
	return &EventHandler{
		queue: q,
	}
}

// Handle processes an event and enqueues appropriate AI tasks
func (h *EventHandler) Handle(ctx context.Context, event Event) error {
	log.Printf("EventHandler: Processing %s event for entity %s", event.Type, event.EntityID)

	switch event.Type {
	case EventIssueCreated:
		return h.handleIssueCreated(ctx, event)
	case EventPRCreated:
		return h.handlePRCreated(ctx, event)
	case EventCommitPushed:
		return h.handleCommitPushed(ctx, event)
	case EventTestFailed:
		return h.handleTestFailed(ctx, event)
	case EventDeploymentDone:
		return h.handleDeploymentDone(ctx, event)
	case EventPRMerged:
		return h.handlePRMerged(ctx, event)
	case EventBuildCompleted:
		return h.handleBuildCompleted(ctx, event)
	default:
		log.Printf("EventHandler: Unknown event type %s", event.Type)
		return nil
	}
}

// handleIssueCreated handles new issue creation events
func (h *EventHandler) handleIssueCreated(ctx context.Context, event Event) error {
	// Enqueue issue triage task with high priority
	task := &queue.Task{
		ID:         fmt.Sprintf("issue-triage-%s-%d", event.EntityID, time.Now().Unix()),
		Type:       queue.TaskIssueTriage,
		Priority:   queue.PriorityHigh,
		RepoID:     event.RepositoryID,
		UserID:     event.UserID,
		EntityType: "issue",
		EntityID:   event.EntityID,
		Data: map[string]any{
			"issue_id": event.EntityID,
			"action":   "triage",
		},
		CreatedAt:  time.Now(),
		MaxRetries: 3,
	}

	if err := h.queue.Enqueue(task); err != nil {
		return fmt.Errorf("failed to enqueue issue triage task: %w", err)
	}

	log.Printf("EventHandler: Enqueued issue triage task for issue %s", event.EntityID)

	// Also check if this is a bug report and needs immediate attention
	if metadata, ok := event.Metadata["labels"].([]string); ok {
		for _, label := range metadata {
			if label == "bug" || label == "critical" {
				// Enqueue with critical priority
				task.Priority = queue.PriorityCritical
				h.queue.Enqueue(task)
				break
			}
		}
	}

	return nil
}

// handlePRCreated handles new PR creation events
func (h *EventHandler) handlePRCreated(ctx context.Context, event Event) error {
	// Enqueue PR review task
	task := &queue.Task{
		ID:         fmt.Sprintf("pr-review-%s-%d", event.EntityID, time.Now().Unix()),
		Type:       queue.TaskPRReview,
		Priority:   queue.PriorityMedium,
		RepoID:     event.RepositoryID,
		UserID:     event.UserID,
		EntityType: "pr",
		EntityID:   event.EntityID,
		Data: map[string]any{
			"pr_id":  event.EntityID,
			"action": "review",
		},
		CreatedAt:  time.Now(),
		MaxRetries: 3,
	}

	if err := h.queue.Enqueue(task); err != nil {
		return fmt.Errorf("failed to enqueue PR review task: %w", err)
	}

	log.Printf("EventHandler: Enqueued PR review task for PR %s", event.EntityID)

	// Check if this is a documentation-only PR (might be auto-approved)
	if files, ok := event.Metadata["files"].([]string); ok {
		allDocs := true
		for _, file := range files {
			if !isDocumentationFile(file) {
				allDocs = false
				break
			}
		}
		if allDocs {
			// Enqueue auto-approval check
			approveTask := &queue.Task{
				ID:         fmt.Sprintf("pr-auto-approve-%s-%d", event.EntityID, time.Now().Unix()),
				Type:       queue.TaskAutoApprove,
				Priority:   queue.PriorityLow,
				RepoID:     event.RepositoryID,
				UserID:     event.UserID,
				EntityType: "pr",
				EntityID:   event.EntityID,
				Data: map[string]any{
					"pr_id":  event.EntityID,
					"reason": "documentation_only",
				},
				CreatedAt: time.Now(),
			}
			h.queue.Enqueue(approveTask)
		}
	}

	return nil
}

// handleCommitPushed handles commit push events
func (h *EventHandler) handleCommitPushed(ctx context.Context, event Event) error {
	// Trigger CI checks
	task := &queue.Task{
		ID:         fmt.Sprintf("ci-check-%s-%d", event.EntityID, time.Now().Unix()),
		Type:       queue.TaskCodeReview,
		Priority:   queue.PriorityMedium,
		RepoID:     event.RepositoryID,
		UserID:     event.UserID,
		EntityType: "commit",
		EntityID:   event.EntityID,
		Data: map[string]any{
			"commit_sha": event.EntityID,
			"branch":     event.Metadata["branch"],
			"action":     "ci_check",
		},
		CreatedAt: time.Now(),
	}

	if err := h.queue.Enqueue(task); err != nil {
		return fmt.Errorf("failed to enqueue CI check task: %w", err)
	}

	// If this is on main/master branch, might need deployment
	if branch, ok := event.Metadata["branch"].(string); ok {
		if branch == "main" || branch == "master" {
			deployTask := &queue.Task{
				ID:         fmt.Sprintf("deploy-check-%s-%d", event.EntityID, time.Now().Unix()),
				Type:       "deployment_check",
				Priority:   queue.PriorityLow,
				RepoID:     event.RepositoryID,
				UserID:     event.UserID,
				EntityType: "commit",
				EntityID:   event.EntityID,
				Data: map[string]any{
					"commit_sha":  event.EntityID,
					"branch":      branch,
					"environment": "staging",
				},
				CreatedAt: time.Now(),
			}
			h.queue.Enqueue(deployTask)
		}
	}

	return nil
}

// handleTestFailed handles test failure events
func (h *EventHandler) handleTestFailed(ctx context.Context, event Event) error {
	// Create issue for test failure
	failedTests := event.Metadata["failed_tests"].([]string)

	task := &queue.Task{
		ID:         fmt.Sprintf("test-failure-%s-%d", event.EntityID, time.Now().Unix()),
		Type:       "create_issue_for_failure",
		Priority:   queue.PriorityHigh,
		RepoID:     event.RepositoryID,
		UserID:     event.UserID,
		EntityType: "test_run",
		EntityID:   event.EntityID,
		Data: map[string]any{
			"run_id":       event.EntityID,
			"failed_tests": failedTests,
			"branch":       event.Metadata["branch"],
			"commit":       event.Metadata["commit"],
		},
		CreatedAt: time.Now(),
	}

	if err := h.queue.Enqueue(task); err != nil {
		return fmt.Errorf("failed to enqueue test failure task: %w", err)
	}

	log.Printf("EventHandler: Enqueued issue creation for %d test failures", len(failedTests))

	return nil
}

// handleDeploymentDone handles deployment completion events
func (h *EventHandler) handleDeploymentDone(ctx context.Context, event Event) error {
	success := event.Metadata["success"].(bool)
	environment := event.Metadata["environment"].(string)

	if !success {
		// Deployment failed - high priority investigation
		task := &queue.Task{
			ID:         fmt.Sprintf("deploy-failure-%s-%d", event.EntityID, time.Now().Unix()),
			Type:       "deployment_failure_analysis",
			Priority:   queue.PriorityCritical,
			RepoID:     event.RepositoryID,
			UserID:     event.UserID,
			EntityType: "deployment",
			EntityID:   event.EntityID,
			Data: map[string]any{
				"deployment_id": event.EntityID,
				"environment":   environment,
				"error":         event.Metadata["error"],
			},
			CreatedAt: time.Now(),
		}

		h.queue.Enqueue(task)

		// If production failed, might need to rollback
		if environment == "production" {
			rollbackTask := &queue.Task{
				ID:         fmt.Sprintf("rollback-check-%s-%d", event.EntityID, time.Now().Unix()),
				Type:       "rollback_evaluation",
				Priority:   queue.PriorityCritical,
				RepoID:     event.RepositoryID,
				UserID:     event.UserID,
				EntityType: "deployment",
				EntityID:   event.EntityID,
				Data: map[string]any{
					"deployment_id": event.EntityID,
					"environment":   environment,
				},
				CreatedAt: time.Now(),
			}
			h.queue.Enqueue(rollbackTask)
		}
	} else {
		// Deployment succeeded - monitor for issues
		task := &queue.Task{
			ID:         fmt.Sprintf("deploy-monitor-%s-%d", event.EntityID, time.Now().Unix()),
			Type:       "deployment_monitoring",
			Priority:   queue.PriorityLow,
			RepoID:     event.RepositoryID,
			UserID:     event.UserID,
			EntityType: "deployment",
			EntityID:   event.EntityID,
			Data: map[string]any{
				"deployment_id": event.EntityID,
				"environment":   environment,
				"version":       event.Metadata["version"],
			},
			CreatedAt: time.Now(),
		}

		h.queue.Enqueue(task)
	}

	return nil
}

// handlePRMerged handles PR merge events
func (h *EventHandler) handlePRMerged(ctx context.Context, event Event) error {
	// Update related issues
	task := &queue.Task{
		ID:         fmt.Sprintf("pr-merged-%s-%d", event.EntityID, time.Now().Unix()),
		Type:       "update_related_issues",
		Priority:   queue.PriorityLow,
		RepoID:     event.RepositoryID,
		UserID:     event.UserID,
		EntityType: "pr",
		EntityID:   event.EntityID,
		Data: map[string]any{
			"pr_id":     event.EntityID,
			"merged_by": event.UserID,
			"merged_at": time.Now(),
		},
		CreatedAt: time.Now(),
	}

	if err := h.queue.Enqueue(task); err != nil {
		return fmt.Errorf("failed to enqueue PR merged task: %w", err)
	}

	// Check if this closes any issues
	if issues, ok := event.Metadata["closes_issues"].([]string); ok {
		for _, issueID := range issues {
			closeTask := &queue.Task{
				ID:         fmt.Sprintf("close-issue-%s-%d", issueID, time.Now().Unix()),
				Type:       "close_issue",
				Priority:   queue.PriorityMedium,
				RepoID:     event.RepositoryID,
				UserID:     event.UserID,
				EntityType: "issue",
				EntityID:   issueID,
				Data: map[string]any{
					"issue_id":  issueID,
					"closed_by": event.UserID,
					"pr_id":     event.EntityID,
				},
				CreatedAt: time.Now(),
			}
			h.queue.Enqueue(closeTask)
		}
	}

	return nil
}

// handleBuildCompleted handles build completion events
func (h *EventHandler) handleBuildCompleted(ctx context.Context, event Event) error {
	success := event.Metadata["success"].(bool)

	if !success {
		// Build failed - analyze and possibly create issue
		task := &queue.Task{
			ID:         fmt.Sprintf("build-failure-%s-%d", event.EntityID, time.Now().Unix()),
			Type:       "build_failure_analysis",
			Priority:   queue.PriorityHigh,
			RepoID:     event.RepositoryID,
			UserID:     event.UserID,
			EntityType: "build",
			EntityID:   event.EntityID,
			Data: map[string]any{
				"build_id": event.EntityID,
				"error":    event.Metadata["error"],
				"branch":   event.Metadata["branch"],
			},
			CreatedAt: time.Now(),
		}

		h.queue.Enqueue(task)
	} else {
		// Build succeeded - might trigger deployment
		if branch, ok := event.Metadata["branch"].(string); ok {
			if branch == "main" || branch == "master" {
				deployTask := &queue.Task{
					ID:         fmt.Sprintf("auto-deploy-%s-%d", event.EntityID, time.Now().Unix()),
					Type:       "auto_deployment",
					Priority:   queue.PriorityMedium,
					RepoID:     event.RepositoryID,
					UserID:     event.UserID,
					EntityType: "build",
					EntityID:   event.EntityID,
					Data: map[string]any{
						"build_id":    event.EntityID,
						"branch":      branch,
						"environment": "staging",
					},
					CreatedAt: time.Now(),
				}
				h.queue.Enqueue(deployTask)
			}
		}
	}

	return nil
}

// isDocumentationFile checks if a file is documentation
func isDocumentationFile(filename string) bool {
	docExtensions := []string{".md", ".txt", ".rst", ".adoc"}
	docDirs := []string{"docs/", "documentation/", "README"}

	for _, ext := range docExtensions {
		if len(filename) > len(ext) && filename[len(filename)-len(ext):] == ext {
			return true
		}
	}

	for _, dir := range docDirs {
		if len(filename) > len(dir) && filename[:len(dir)] == dir {
			return true
		}
	}

	return false
}

// TriggerEvent is a helper function to trigger an event
func TriggerEvent(ctx context.Context, handler *EventHandler, eventType EventType, repoID, userID, entityID string, metadata map[string]any) error {
	event := Event{
		Type:         eventType,
		RepositoryID: repoID,
		UserID:       userID,
		EntityID:     entityID,
		Metadata:     metadata,
		Timestamp:    time.Now(),
	}

	return handler.Handle(ctx, event)
}
