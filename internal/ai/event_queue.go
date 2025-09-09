package ai

import (
	"errors"
	"fmt"
	"log"
	"sync"
	"time"
	"workspace/models"
)

// EventType represents different types of AI events
type EventType string

const (
	EventIssueCreated     EventType = "issue_created"
	EventIssueUpdated     EventType = "issue_updated"
	EventIssueClosed      EventType = "issue_closed"
	EventPRCreated        EventType = "pr_created"
	EventPRUpdated        EventType = "pr_updated"
	EventPRMerged         EventType = "pr_merged"
	EventRepoCreated      EventType = "repo_created"
	EventCommitPushed     EventType = "commit_pushed"
	EventDailySchedule    EventType = "daily_schedule"
	EventWeeklySchedule   EventType = "weekly_schedule"
	EventStaleCheck       EventType = "stale_check"
	EventSecurityScan     EventType = "security_scan"
	EventDependencyCheck  EventType = "dependency_check"
)

// EventPriority determines processing order
type EventPriority int

const (
	PriorityLow    EventPriority = 1
	PriorityNormal EventPriority = 5
	PriorityHigh   EventPriority = 8
	PriorityCritical EventPriority = 10
)

// AIEvent represents an event that triggers AI processing
type AIEvent struct {
	ID         string                 `json:"id"`
	Type       EventType              `json:"type"`
	Priority   EventPriority          `json:"priority"`
	EntityType string                 `json:"entity_type"` // "issue", "pr", "repo", etc.
	EntityID   string                 `json:"entity_id"`
	UserID     string                 `json:"user_id"`
	RepoID     string                 `json:"repo_id"`
	Data       map[string]interface{} `json:"data"`
	CreatedAt  time.Time              `json:"created_at"`
	ProcessedAt *time.Time            `json:"processed_at"`
	Status     string                 `json:"status"` // "pending", "processing", "completed", "failed"
	Error      string                 `json:"error,omitempty"`
	RetryCount int                    `json:"retry_count"`
}

// EventProcessor handles specific event types
type EventProcessor interface {
	ProcessEvent(event *AIEvent) error
	CanHandle(eventType EventType) bool
}

// AIEventQueue manages the event-driven AI system
type AIEventQueue struct {
	events     chan *AIEvent
	processors map[EventType]EventProcessor
	workers    int
	wg         sync.WaitGroup
	mu         sync.RWMutex
	running    bool
	
	// Metrics
	processed  int64
	failed     int64
	startTime  time.Time
}

var EventQueue *AIEventQueue

// InitializeEventQueue creates and starts the event queue
func InitializeEventQueue(workers int) error {
	if EventQueue != nil && EventQueue.running {
		return errors.New("event queue already initialized")
	}
	
	EventQueue = &AIEventQueue{
		events:     make(chan *AIEvent, 1000), // Buffer up to 1000 events
		processors: make(map[EventType]EventProcessor),
		workers:    workers,
		startTime:  time.Now(),
	}
	
	// Register processors
	EventQueue.RegisterProcessor(EventIssueCreated, &IssueTriageProcessor{})
	EventQueue.RegisterProcessor(EventIssueUpdated, &IssueUpdateProcessor{})
	EventQueue.RegisterProcessor(EventPRCreated, &PRReviewProcessor{})
	EventQueue.RegisterProcessor(EventPRUpdated, &PRUpdateProcessor{})
	EventQueue.RegisterProcessor(EventDailySchedule, &DailyReportProcessor{})
	EventQueue.RegisterProcessor(EventStaleCheck, &StaleCheckProcessor{})
	EventQueue.RegisterProcessor(EventSecurityScan, &SecurityScanProcessor{})
	
	// Start workers
	EventQueue.Start()
	
	log.Printf("AI Event Queue: Initialized with %d workers", workers)
	return nil
}

// RegisterProcessor adds a processor for a specific event type
func (q *AIEventQueue) RegisterProcessor(eventType EventType, processor EventProcessor) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.processors[eventType] = processor
	log.Printf("AI Event Queue: Registered processor for %s events", eventType)
}

// PublishEvent adds an event to the queue
func (q *AIEventQueue) PublishEvent(event *AIEvent) error {
	if !q.running {
		return errors.New("event queue not running")
	}
	
	// Set defaults
	if event.ID == "" {
		event.ID = fmt.Sprintf("evt_%d", time.Now().UnixNano())
	}
	event.CreatedAt = time.Now()
	event.Status = "pending"
	
	// Log the event in AI activity
	activity := &models.AIActivity{
		Type:        string(event.Type),
		Description: fmt.Sprintf("Event queued: %s for %s %s", event.Type, event.EntityType, event.EntityID),
		RepoID:      event.RepoID,
		EntityType:  event.EntityType,
		EntityID:    event.EntityID,
		Success:     false, // Not yet processed
	}
	models.AIActivities.Insert(activity)
	
	// Non-blocking send with timeout
	select {
	case q.events <- event:
		log.Printf("AI Event Queue: Published %s event (priority %d)", event.Type, event.Priority)
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("timeout publishing event")
	}
}

// Start begins processing events
func (q *AIEventQueue) Start() {
	q.mu.Lock()
	defer q.mu.Unlock()
	
	if q.running {
		return
	}
	
	q.running = true
	
	// Start worker goroutines
	for i := 0; i < q.workers; i++ {
		q.wg.Add(1)
		go q.worker(i)
	}
	
	log.Printf("AI Event Queue: Started %d workers", q.workers)
}

// Stop gracefully shuts down the queue
func (q *AIEventQueue) Stop() {
	q.mu.Lock()
	q.running = false
	q.mu.Unlock()
	
	close(q.events)
	q.wg.Wait()
	
	log.Printf("AI Event Queue: Stopped (processed: %d, failed: %d)", q.processed, q.failed)
}

// worker processes events from the queue
func (q *AIEventQueue) worker(id int) {
	defer q.wg.Done()
	
	for event := range q.events {
		q.processEvent(event)
	}
}

// processEvent handles a single event
func (q *AIEventQueue) processEvent(event *AIEvent) {
	startTime := time.Now()
	event.Status = "processing"
	
	// Find processor for this event type
	q.mu.RLock()
	processor, exists := q.processors[event.Type]
	q.mu.RUnlock()
	
	if !exists {
		log.Printf("AI Event Queue: No processor for event type %s", event.Type)
		event.Status = "failed"
		event.Error = "no processor available"
		q.failed++
		return
	}
	
	// Process the event
	err := processor.ProcessEvent(event)
	
	now := time.Now()
	event.ProcessedAt = &now
	
	if err != nil {
		event.Status = "failed"
		event.Error = err.Error()
		event.RetryCount++
		
		// Retry logic for transient failures
		if event.RetryCount < 3 {
			log.Printf("AI Event Queue: Retrying %s event (attempt %d)", event.Type, event.RetryCount+1)
			time.Sleep(time.Duration(event.RetryCount) * 5 * time.Second)
			q.PublishEvent(event) // Re-queue for retry
		} else {
			log.Printf("AI Event Queue: Failed to process %s event after %d attempts: %v", 
				event.Type, event.RetryCount, err)
			q.failed++
			
			// Log failure in AI activity
			activity := &models.AIActivity{
				Type:        string(event.Type),
				Description: fmt.Sprintf("Event failed: %s - %v", event.Type, err),
				RepoID:      event.RepoID,
				EntityType:  event.EntityType,
				EntityID:    event.EntityID,
				Success:     false,
				Duration:    time.Since(startTime).Milliseconds(),
			}
			models.AIActivities.Insert(activity)
		}
	} else {
		event.Status = "completed"
		q.processed++
		
		// Log success in AI activity
		activity := &models.AIActivity{
			Type:        string(event.Type),
			Description: fmt.Sprintf("Event processed: %s", event.Type),
			RepoID:      event.RepoID,
			EntityType:  event.EntityType,
			EntityID:    event.EntityID,
			Success:     true,
			Duration:    time.Since(startTime).Milliseconds(),
		}
		models.AIActivities.Insert(activity)
		
		log.Printf("AI Event Queue: Processed %s event in %v", event.Type, time.Since(startTime))
	}
}

// GetStats returns queue statistics
func (q *AIEventQueue) GetStats() map[string]interface{} {
	q.mu.RLock()
	defer q.mu.RUnlock()
	
	return map[string]interface{}{
		"running":    q.running,
		"workers":    q.workers,
		"processed":  q.processed,
		"failed":     q.failed,
		"pending":    len(q.events),
		"uptime":     time.Since(q.startTime).String(),
		"processors": len(q.processors),
	}
}

// Helper functions for publishing common events

// PublishIssueEvent publishes an issue-related event
func PublishIssueEvent(eventType EventType, issue *models.Issue, userID string) error {
	if EventQueue == nil {
		return errors.New("event queue not initialized")
	}
	
	priority := PriorityNormal
	if issue.Priority == models.PriorityCritical {
		priority = PriorityCritical
	} else if issue.Priority == models.PriorityHigh {
		priority = PriorityHigh
	}
	
	event := &AIEvent{
		Type:       eventType,
		Priority:   priority,
		EntityType: "issue",
		EntityID:   issue.ID,
		UserID:     userID,
		RepoID:     issue.RepoID,
		Data: map[string]interface{}{
			"title":    issue.Title,
			"body":     issue.Body,
			"status":   issue.Status,
			"priority": issue.Priority,
		},
	}
	
	return EventQueue.PublishEvent(event)
}

// PublishPREvent publishes a PR-related event
func PublishPREvent(eventType EventType, pr *models.PullRequest, userID string) error {
	if EventQueue == nil {
		return errors.New("event queue not initialized")
	}
	
	event := &AIEvent{
		Type:       eventType,
		Priority:   PriorityHigh, // PRs are usually high priority
		EntityType: "pr",
		EntityID:   pr.ID,
		UserID:     userID,
		RepoID:     pr.RepoID,
		Data: map[string]interface{}{
			"title":       pr.Title,
			"description": pr.Body,
			"status":      pr.Status,
			"source":      pr.CompareBranch,
			"target":      pr.BaseBranch,
		},
	}
	
	return EventQueue.PublishEvent(event)
}

// PublishScheduledEvent publishes a scheduled event
func PublishScheduledEvent(eventType EventType) error {
	if EventQueue == nil {
		return errors.New("event queue not initialized")
	}
	
	event := &AIEvent{
		Type:       eventType,
		Priority:   PriorityLow, // Scheduled tasks are lower priority
		EntityType: "system",
		EntityID:   "scheduler",
		Data:       map[string]interface{}{"triggered_at": time.Now()},
	}
	
	return EventQueue.PublishEvent(event)
}