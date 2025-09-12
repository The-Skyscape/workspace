// Package ai provides the main AI service coordination
package ai

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"workspace/internal/ai/processors"
	"workspace/internal/ai/queue"
	"workspace/internal/ai/websocket"
	"workspace/models"
)

// Service coordinates all AI components
type Service struct {
	Queue   *queue.Queue
	Hub     *websocket.Hub
	mu      sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc
	running bool
	config  *Config
}

// Config holds AI service configuration
type Config struct {
	Enabled          bool
	Workers          int
	MaxWorkers       int
	AutomationLevel  string // conservative, balanced, aggressive
	ResponseDelay    time.Duration
	IssueTriage      bool
	PRReview         bool
	AutoApprove      bool
	DailyReports     bool
	StaleManagement  bool
	SecurityScan     bool
	DependencyUpdate bool
}

// DefaultConfig returns default AI configuration
func DefaultConfig() *Config {
	return &Config{
		Enabled:          true,
		Workers:          3,
		MaxWorkers:       10,
		AutomationLevel:  "balanced",
		ResponseDelay:    30 * time.Second,
		IssueTriage:      true,
		PRReview:         true,
		AutoApprove:      false,
		DailyReports:     false,
		StaleManagement:  false,
		SecurityScan:     false,
		DependencyUpdate: false,
	}
}

var (
	// Global AI service instance
	Instance *Service
	once     sync.Once
)

// Initialize creates and starts the global AI service
func Initialize(config *Config) error {
	var initErr error

	once.Do(func() {
		if config == nil {
			config = DefaultConfig()
		}

		ctx, cancel := context.WithCancel(context.Background())

		// Create the service
		Instance = &Service{
			config: config,
			ctx:    ctx,
			cancel: cancel,
		}

		// Initialize queue
		queueConfig := &queue.Config{
			Workers:         config.Workers,
			MaxWorkers:      config.MaxWorkers,
			RetryDelay:      5 * time.Second,
			MaxRetries:      3,
			ProcessInterval: 2 * time.Second,
		}

		Instance.Queue = queue.New(queueConfig)

		// Register processors
		Instance.registerProcessors()

		// Initialize WebSocket hub
		Instance.Hub = websocket.NewHub()

		// Start services
		if config.Enabled {
			Instance.Start()
		}
	})

	return initErr
}

// Start begins AI service operation
func (s *Service) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		log.Println("AI Service: Already running")
		return
	}

	// Start the queue
	s.Queue.Start()

	// Start WebSocket hub
	go s.Hub.Run()

	// Start monitoring goroutine
	go s.monitor()

	s.running = true
	log.Println("AI Service: Started successfully")
}

// Stop gracefully stops the AI service
func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	s.cancel()
	s.Queue.Stop()
	s.running = false

	log.Println("AI Service: Stopped")
}

// IsRunning returns whether the service is running
func (s *Service) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// registerProcessors registers all task processors with the queue
func (s *Service) registerProcessors() {
	// Register issue processor
	s.Queue.RegisterProcessor(queue.TaskIssueTriage, processors.NewIssueProcessor())

	// Register PR processor
	s.Queue.RegisterProcessor(queue.TaskPRReview, processors.NewPRProcessor())
	s.Queue.RegisterProcessor(queue.TaskAutoApprove, processors.NewPRProcessor())

	// Register report processor
	s.Queue.RegisterProcessor(queue.TaskDailyReport, processors.NewReportProcessor())

	// Register stale management processor
	s.Queue.RegisterProcessor(queue.TaskStaleManagement, processors.NewStaleProcessor())

	// Register security processor
	s.Queue.RegisterProcessor(queue.TaskSecurityScan, processors.NewSecurityProcessor())

	// Register dependency processor
	s.Queue.RegisterProcessor(queue.TaskDependencyUpdate, processors.NewDependencyProcessor())

	log.Println("AI Service: Registered all processors")
}

// monitor runs periodic monitoring and broadcasts stats
func (s *Service) monitor() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			// Get queue stats
			stats := s.Queue.GetStats()

			// Broadcast to WebSocket clients
			if s.Hub != nil {
				s.Hub.BroadcastQueueStats(stats)
			}
		}
	}
}

// EnqueueIssue creates a task for issue triage
func (s *Service) EnqueueIssue(issue *models.Issue, userID string) error {
	if !s.config.IssueTriage {
		return nil // Feature disabled
	}

	// Add response delay for less aggressive automation
	if s.config.AutomationLevel == "conservative" {
		time.Sleep(s.config.ResponseDelay * 2)
	} else if s.config.AutomationLevel == "balanced" {
		time.Sleep(s.config.ResponseDelay)
	}
	// No delay for aggressive mode

	task := &queue.Task{
		Type:       queue.TaskIssueTriage,
		Priority:   queue.PriorityMedium,
		RepoID:     issue.RepoID,
		UserID:     userID,
		EntityType: "issue",
		EntityID:   issue.ID,
		Data: map[string]any{
			"issue_id": issue.ID,
		},
	}

	return s.Queue.Enqueue(task)
}

// EnqueuePR creates a task for PR review
func (s *Service) EnqueuePR(pr *models.PullRequest, userID string) error {
	if !s.config.PRReview {
		return nil // Feature disabled
	}

	// Determine priority based on PR characteristics
	priority := queue.PriorityMedium

	// Add response delay for less aggressive automation
	if s.config.AutomationLevel == "conservative" {
		time.Sleep(s.config.ResponseDelay * 2)
	} else if s.config.AutomationLevel == "balanced" {
		time.Sleep(s.config.ResponseDelay)
	}

	task := &queue.Task{
		Type:       queue.TaskPRReview,
		Priority:   priority,
		RepoID:     pr.RepoID,
		UserID:     userID,
		EntityType: "pull_request",
		EntityID:   pr.ID,
		Data: map[string]any{
			"pr_id": pr.ID,
		},
	}

	// If auto-approve is enabled and PR might be eligible, add auto-approve task
	if s.config.AutoApprove {
		autoTask := &queue.Task{
			Type:       queue.TaskAutoApprove,
			Priority:   queue.PriorityLow,
			RepoID:     pr.RepoID,
			UserID:     userID,
			EntityType: "pull_request",
			EntityID:   pr.ID,
			Data: map[string]any{
				"pr_id": pr.ID,
			},
		}
		s.Queue.Enqueue(autoTask)
	}

	return s.Queue.Enqueue(task)
}

// GetQueueStats returns current queue statistics
func (s *Service) GetQueueStats() map[string]any {
	if s.Queue == nil {
		return map[string]any{
			"status": "offline",
		}
	}
	return s.Queue.GetStats()
}

// GetConfig returns current configuration
func (s *Service) GetConfig() *Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// EnqueueTask creates a generic task for the AI queue
func (s *Service) EnqueueTask(taskType string, data map[string]any, priority int) error {
	if s.Queue == nil {
		return fmt.Errorf("queue not initialized")
	}

	// Get user ID from data
	userID, _ := data["user_id"].(string)

	// Map task type string to TaskType
	var tType queue.TaskType
	switch taskType {
	case "daily_report":
		tType = queue.TaskDailyReport
	case "security_scan":
		tType = queue.TaskSecurityScan
	case "stale_management":
		tType = queue.TaskStaleManagement
	case "dependency_check":
		tType = queue.TaskDependencyUpdate
	default:
		return fmt.Errorf("unknown task type: %s", taskType)
	}

	task := &queue.Task{
		Type:     tType,
		Priority: priority,
		UserID:   userID,
		Data:     data,
	}

	return s.Queue.Enqueue(task)
}

// UpdateConfig updates the AI configuration
func (s *Service) UpdateConfig(config *Config) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.config = config

	// Restart if needed
	if config.Enabled && !s.running {
		go s.Start()
	} else if !config.Enabled && s.running {
		go s.Stop()
	}

	log.Printf("AI Service: Configuration updated")
}

// BroadcastActivity sends activity update to WebSocket clients
func (s *Service) BroadcastActivity(activity *models.AIActivity) {
	if s.Hub != nil {
		s.Hub.BroadcastActivity(activity)
	}
}

// BroadcastTaskUpdate sends task update to WebSocket clients
func (s *Service) BroadcastTaskUpdate(task *queue.Task) {
	if s.Hub != nil {
		s.Hub.BroadcastTaskUpdate(task)
	}
}
