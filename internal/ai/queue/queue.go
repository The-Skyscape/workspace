// Package queue implements an intelligent task queue for AI automation
package queue

import (
	"container/heap"
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"workspace/models"
)

// TaskType represents the type of AI task
type TaskType string

const (
	TaskIssueTriage      TaskType = "issue_triage"
	TaskPRReview         TaskType = "pr_review"
	TaskDailyReport      TaskType = "daily_report"
	TaskStaleManagement  TaskType = "stale_management"
	TaskDependencyUpdate TaskType = "dependency_update"
	TaskCodeReview       TaskType = "code_review"
	TaskAutoApprove      TaskType = "auto_approve"
	TaskSecurityScan     TaskType = "security_scan"
	TaskPerformanceCheck TaskType = "performance_check"
)

// Priority levels for tasks
const (
	PriorityCritical = 1
	PriorityHigh     = 2
	PriorityMedium   = 5
	PriorityLow      = 8
	PriorityIdle     = 10
)

// Task represents a queued AI task
type Task struct {
	ID         string         `json:"id"`
	Type       TaskType       `json:"type"`
	Priority   int            `json:"priority"`
	RepoID     string         `json:"repo_id"`
	UserID     string         `json:"user_id"`
	EntityType string         `json:"entity_type"`
	EntityID   string         `json:"entity_id"`
	Data       map[string]any `json:"data"`
	CreatedAt  time.Time      `json:"created_at"`
	StartedAt  *time.Time     `json:"started_at,omitempty"`
	Retries    int            `json:"retries"`
	MaxRetries int            `json:"max_retries"`
	Status     TaskStatus     `json:"status"`
	Error      string         `json:"error,omitempty"`
	Result     any            `json:"result,omitempty"`
}

// TaskStatus represents the status of a task
type TaskStatus string

const (
	StatusQueued     TaskStatus = "queued"
	StatusProcessing TaskStatus = "processing"
	StatusCompleted  TaskStatus = "completed"
	StatusFailed     TaskStatus = "failed"
	StatusRetrying   TaskStatus = "retrying"
	StatusCancelled  TaskStatus = "cancelled"
)

// Queue manages AI tasks with priority-based processing
type Queue struct {
	mu         sync.RWMutex
	tasks      taskHeap
	processing map[string]*Task
	workers    int
	maxWorkers int
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup

	// Metrics
	totalProcessed uint64
	totalFailed    uint64
	totalRetried   uint64
	averageTimeMs  uint64

	// Processors
	processors map[TaskType]Processor

	// Events
	onTaskComplete func(*Task)
	onTaskFail     func(*Task, error)

	// Configuration
	retryDelay      time.Duration
	maxRetries      int
	processInterval time.Duration
}

// Processor handles task execution
type Processor interface {
	Process(ctx context.Context, task *Task) error
	CanHandle(taskType TaskType) bool
}

// Config holds queue configuration
type Config struct {
	Workers         int
	MaxWorkers      int
	RetryDelay      time.Duration
	MaxRetries      int
	ProcessInterval time.Duration
}

// DefaultConfig returns default queue configuration
func DefaultConfig() *Config {
	return &Config{
		Workers:         3,
		MaxWorkers:      10,
		RetryDelay:      5 * time.Second,
		MaxRetries:      3,
		ProcessInterval: 2 * time.Second,
	}
}

// New creates a new AI task queue
func New(cfg *Config) *Queue {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	q := &Queue{
		tasks:           make(taskHeap, 0),
		processing:      make(map[string]*Task),
		workers:         cfg.Workers,
		maxWorkers:      cfg.MaxWorkers,
		ctx:             ctx,
		cancel:          cancel,
		processors:      make(map[TaskType]Processor),
		retryDelay:      cfg.RetryDelay,
		maxRetries:      cfg.MaxRetries,
		processInterval: cfg.ProcessInterval,
	}

	heap.Init(&q.tasks)
	return q
}

// RegisterProcessor registers a task processor
func (q *Queue) RegisterProcessor(taskType TaskType, processor Processor) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.processors[taskType] = processor
}

// Start begins processing tasks
func (q *Queue) Start() {
	q.mu.Lock()
	workers := q.workers
	q.mu.Unlock()

	for i := 0; i < workers; i++ {
		q.wg.Add(1)
		go q.worker(i)
	}

	// Start scheduler for periodic tasks
	q.wg.Add(1)
	go q.scheduler()

	log.Printf("AI Queue: Started with %d workers", workers)
}

// Stop gracefully stops the queue
func (q *Queue) Stop() {
	q.cancel()
	q.wg.Wait()
	log.Println("AI Queue: Stopped")
}

// Enqueue adds a task to the queue
func (q *Queue) Enqueue(task *Task) error {
	if task.ID == "" {
		task.ID = generateTaskID(task.Type, task.EntityID)
	}

	if task.MaxRetries == 0 {
		task.MaxRetries = q.maxRetries
	}

	task.Status = StatusQueued
	task.CreatedAt = time.Now()

	q.mu.Lock()
	heap.Push(&q.tasks, task)
	q.mu.Unlock()

	log.Printf("AI Queue: Enqueued task %s (type: %s, priority: %d)",
		task.ID, task.Type, task.Priority)

	// Log activity
	q.logActivity(task, "enqueued", true)

	return nil
}

// EnqueueWithPriority creates and enqueues a task with specific priority
func (q *Queue) EnqueueWithPriority(taskType TaskType, priority int, data map[string]any) (*Task, error) {
	task := &Task{
		Type:     taskType,
		Priority: priority,
		Data:     data,
		RepoID:   getStringFromData(data, "repo_id"),
		UserID:   getStringFromData(data, "user_id"),
	}

	err := q.Enqueue(task)
	return task, err
}

// worker processes tasks from the queue
func (q *Queue) worker(id int) {
	defer q.wg.Done()

	ticker := time.NewTicker(q.processInterval)
	defer ticker.Stop()

	for {
		select {
		case <-q.ctx.Done():
			return
		case <-ticker.C:
			q.processNext(id)
		}
	}
}

// processNext processes the next task in queue
func (q *Queue) processNext(workerID int) {
	task := q.dequeue()
	if task == nil {
		return
	}

	log.Printf("AI Queue Worker %d: Processing task %s (type: %s, priority: %d)",
		workerID, task.ID, task.Type, task.Priority)

	// Mark as processing
	now := time.Now()
	task.StartedAt = &now
	task.Status = StatusProcessing

	q.mu.Lock()
	q.processing[task.ID] = task
	q.mu.Unlock()

	// Process the task
	startTime := time.Now()
	err := q.processTask(task)
	duration := time.Since(startTime)

	// Update metrics
	atomic.AddUint64(&q.totalProcessed, 1)
	q.updateAverageTime(duration)

	// Handle result
	q.mu.Lock()
	delete(q.processing, task.ID)
	q.mu.Unlock()

	if err != nil {
		q.handleTaskError(task, err)
	} else {
		q.handleTaskSuccess(task, duration)
	}
}

// processTask executes a task using the appropriate processor
func (q *Queue) processTask(task *Task) error {
	processor, exists := q.processors[task.Type]
	if !exists {
		return fmt.Errorf("no processor registered for task type: %s", task.Type)
	}

	// Create timeout context
	ctx, cancel := context.WithTimeout(q.ctx, 5*time.Minute)
	defer cancel()

	return processor.Process(ctx, task)
}

// dequeue gets the next task from the priority queue
func (q *Queue) dequeue() *Task {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.tasks.Len() == 0 {
		return nil
	}

	task := heap.Pop(&q.tasks).(*Task)
	return task
}

// handleTaskSuccess handles successful task completion
func (q *Queue) handleTaskSuccess(task *Task, duration time.Duration) {
	task.Status = StatusCompleted

	log.Printf("AI Queue: Task %s completed in %v", task.ID, duration)

	// Log activity
	q.logActivity(task, "completed", true)

	// Call completion handler
	if q.onTaskComplete != nil {
		q.onTaskComplete(task)
	}
}

// handleTaskError handles task failure
func (q *Queue) handleTaskError(task *Task, err error) {
	task.Error = err.Error()
	task.Retries++

	atomic.AddUint64(&q.totalFailed, 1)

	if task.Retries < task.MaxRetries {
		// Retry the task
		task.Status = StatusRetrying
		task.Priority++ // Lower priority for retry

		atomic.AddUint64(&q.totalRetried, 1)

		// Re-enqueue after delay
		go func() {
			time.Sleep(q.retryDelay * time.Duration(task.Retries))
			q.Enqueue(task)
		}()

		log.Printf("AI Queue: Task %s failed, retrying (attempt %d/%d): %v",
			task.ID, task.Retries, task.MaxRetries, err)
	} else {
		// Final failure
		task.Status = StatusFailed

		log.Printf("AI Queue: Task %s failed after %d retries: %v",
			task.ID, task.MaxRetries, err)

		// Log activity
		q.logActivity(task, "failed", false)

		// Call failure handler
		if q.onTaskFail != nil {
			q.onTaskFail(task, err)
		}
	}
}

// updateAverageTime updates the average processing time
func (q *Queue) updateAverageTime(duration time.Duration) {
	ms := uint64(duration.Milliseconds())
	current := atomic.LoadUint64(&q.averageTimeMs)

	if current == 0 {
		atomic.StoreUint64(&q.averageTimeMs, ms)
	} else {
		// Exponential moving average
		newAvg := (current*9 + ms) / 10
		atomic.StoreUint64(&q.averageTimeMs, newAvg)
	}
}

// GetStats returns queue statistics
func (q *Queue) GetStats() map[string]any {
	q.mu.RLock()
	queueLength := q.tasks.Len()
	processingCount := len(q.processing)
	q.mu.RUnlock()

	return map[string]any{
		"queue_length":    queueLength,
		"processing":      processingCount,
		"workers":         q.workers,
		"total_processed": atomic.LoadUint64(&q.totalProcessed),
		"total_failed":    atomic.LoadUint64(&q.totalFailed),
		"total_retried":   atomic.LoadUint64(&q.totalRetried),
		"average_time_ms": atomic.LoadUint64(&q.averageTimeMs),
		"status":          q.getStatus(),
	}
}

// getStatus returns the queue status
func (q *Queue) getStatus() string {
	select {
	case <-q.ctx.Done():
		return "stopped"
	default:
		q.mu.RLock()
		processing := len(q.processing) > 0
		q.mu.RUnlock()

		if processing {
			return "processing"
		}
		return "idle"
	}
}

// scheduler handles periodic task scheduling
func (q *Queue) scheduler() {
	defer q.wg.Done()

	dailyTicker := time.NewTicker(24 * time.Hour)
	hourlyTicker := time.NewTicker(1 * time.Hour)

	defer dailyTicker.Stop()
	defer hourlyTicker.Stop()

	for {
		select {
		case <-q.ctx.Done():
			return
		case <-dailyTicker.C:
			q.scheduleDailyTasks()
		case <-hourlyTicker.C:
			q.scheduleHourlyTasks()
		}
	}
}

// scheduleDailyTasks schedules daily maintenance tasks
func (q *Queue) scheduleDailyTasks() {
	// Schedule daily reports for all repositories
	repos, err := models.Repositories.Search("")
	if err != nil {
		log.Printf("AI Queue: Failed to get repositories for daily tasks: %v", err)
		return
	}

	for _, repo := range repos {
		task := &Task{
			Type:     TaskDailyReport,
			Priority: PriorityLow,
			RepoID:   repo.ID,
			Data: map[string]any{
				"repo_id":   repo.ID,
				"repo_name": repo.Name,
			},
		}
		q.Enqueue(task)
	}

	log.Printf("AI Queue: Scheduled %d daily report tasks", len(repos))
}

// scheduleHourlyTasks schedules hourly maintenance tasks
func (q *Queue) scheduleHourlyTasks() {
	// Schedule stale issue management
	task := &Task{
		Type:     TaskStaleManagement,
		Priority: PriorityIdle,
		Data: map[string]any{
			"check_type": "hourly",
		},
	}
	q.Enqueue(task)

	log.Println("AI Queue: Scheduled hourly stale management task")
}

// logActivity logs task activity to the database
func (q *Queue) logActivity(task *Task, action string, success bool) {
	repo, _ := models.Repos.Get(task.RepoID)
	repoName := "System"
	if repo != nil {
		repoName = repo.Name
	}

	description := fmt.Sprintf("%s task %s", strings.Title(string(task.Type)), action)

	_, err := models.LogAIActivity(
		string(task.Type),
		task.RepoID,
		repoName,
		task.EntityType,
		task.EntityID,
		description,
		success,
		0, // Duration will be calculated from StartedAt if needed
	)

	if err != nil {
		log.Printf("AI Queue: Failed to log activity: %v", err)
	}
}

// Helper functions

func generateTaskID(taskType TaskType, entityID string) string {
	return fmt.Sprintf("%s_%s_%d", taskType, entityID, time.Now().UnixNano())
}

func getStringFromData(data map[string]any, key string) string {
	if val, ok := data[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// Priority queue implementation

type taskHeap []*Task

func (h taskHeap) Len() int           { return len(h) }
func (h taskHeap) Less(i, j int) bool { return h[i].Priority < h[j].Priority }
func (h taskHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *taskHeap) Push(x any) {
	*h = append(*h, x.(*Task))
}

func (h *taskHeap) Pop() any {
	old := *h
	n := len(old)
	task := old[n-1]
	*h = old[0 : n-1]
	return task
}
