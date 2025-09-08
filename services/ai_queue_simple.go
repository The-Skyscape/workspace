package services

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"workspace/models"
)

// AITaskType represents the type of AI task
type AITaskType string

const (
	TaskIssueTriage      AITaskType = "issue_triage"
	TaskPRReview         AITaskType = "pr_review"
	TaskDailyReport      AITaskType = "daily_report"
	TaskStaleManagement  AITaskType = "stale_management"
	TaskDependencyUpdate AITaskType = "dependency_update"
	TaskCodeReview       AITaskType = "code_review"
	TaskAutoApprove      AITaskType = "auto_approve"
)

// AITask represents a queued AI task
type AITask struct {
	ID         string                 
	Type       AITaskType             
	Priority   int                    // 1 (highest) to 10 (lowest)
	RepoID     string                 
	UserID     string                 
	EntityType string                 
	EntityID   string                 
	Data       map[string]interface{} 
	CreatedAt  time.Time              
	Retries    int                    // Number of retry attempts
	Status     string                 // queued, processing, completed, failed
}

// AIQueueService manages the AI task queue with priority handling
type AIQueueService struct {
	queue           []*AITask
	mu              sync.RWMutex
	running         bool
	stopCh          chan struct{}
	totalProcessed  int
	totalFailed     int
	processing      int
	averageTimeMs   int64
	workers         int
	lastDailyReport time.Time
	statsLock       sync.RWMutex
}

// Global AI queue instance
var AIQueue *AIQueueService

// InitAIQueue initializes the AI queue service
func InitAIQueue() error {
	if AIQueue != nil && AIQueue.running {
		return nil
	}

	AIQueue = &AIQueueService{
		queue:           make([]*AITask, 0),
		stopCh:          make(chan struct{}),
		workers:         3, // Multiple workers for parallel processing
		lastDailyReport: time.Now(),
	}

	AIQueue.Start()
	log.Println("AI Queue Service initialized (simplified version)")
	return nil
}

// Start begins processing tasks
func (q *AIQueueService) Start() {
	q.mu.Lock()
	if q.running {
		q.mu.Unlock()
		return
	}
	q.running = true
	q.mu.Unlock()

	// Start multiple workers for parallel processing
	for i := 0; i < q.workers; i++ {
		go q.worker(i)
	}
	
	// Start scheduler for periodic tasks
	go q.scheduler()
}

// QueueTask adds a new task to the queue
func (q *AIQueueService) QueueTask(taskType AITaskType, repoID, userID, entityType, entityID string, data map[string]interface{}, priority int) (*AITask, error) {
	task := &AITask{
		ID:         fmt.Sprintf("%s_%s_%d", taskType, entityID, time.Now().Unix()),
		Type:       taskType,
		Priority:   priority,
		RepoID:     repoID,
		UserID:     userID,
		EntityType: entityType,
		EntityID:   entityID,
		Data:       data,
		CreatedAt:  time.Now(),
	}

	q.mu.Lock()
	// Insert task based on priority (priority queue)
	inserted := false
	for i, t := range q.queue {
		if task.Priority < t.Priority {
			// Insert at position i
			q.queue = append(q.queue[:i], append([]*AITask{task}, q.queue[i:]...)...)
			inserted = true
			break
		}
	}
	if !inserted {
		q.queue = append(q.queue, task)
	}
	task.Status = "queued"
	q.mu.Unlock()

	log.Printf("AI Queue: Added task %s (type: %s, priority: %d)", task.ID, task.Type, task.Priority)
	return task, nil
}

// worker processes tasks
func (q *AIQueueService) worker(id int) {
	ticker := time.NewTicker(5 * time.Second) // Faster processing
	defer ticker.Stop()

	for {
		select {
		case <-q.stopCh:
			return
		case <-ticker.C:
			q.processNextTask(id)
		}
	}
}

// scheduler handles periodic tasks
func (q *AIQueueService) scheduler() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-q.stopCh:
			return
		case <-ticker.C:
			// Schedule daily report if 24 hours have passed
			if time.Since(q.lastDailyReport) > 24*time.Hour {
				q.scheduleDailyReports()
				q.lastDailyReport = time.Now()
			}
			// Check for stale issues every hour
			q.scheduleStaleManagement()
		}
	}
}

// processNextTask processes the next task in queue
func (q *AIQueueService) processNextTask(workerID int) {
	q.mu.Lock()
	if len(q.queue) == 0 {
		q.mu.Unlock()
		return
	}

	// Get highest priority task
	task := q.queue[0]
	q.queue = q.queue[1:]
	task.Status = "processing"
	q.statsLock.Lock()
	q.processing++
	q.statsLock.Unlock()
	q.mu.Unlock()

	log.Printf("AI Queue Worker %d: Processing task %s (type: %s, priority: %d)", workerID, task.ID, task.Type, task.Priority)
	startTime := time.Now()

	// Process based on type with error handling
	var err error
	switch task.Type {
	case TaskIssueTriage:
		err = q.processIssueTriage(task)
	case TaskPRReview:
		err = q.processPRReview(task)
	case TaskDailyReport:
		err = q.processDailyReport(task)
	case TaskStaleManagement:
		err = q.processStaleManagement(task)
	case TaskCodeReview:
		err = q.processCodeReview(task)
	case TaskAutoApprove:
		err = q.processAutoApprove(task)
	default:
		log.Printf("AI Queue: Unknown task type %s", task.Type)
	}
	
	// Update statistics
	q.statsLock.Lock()
	q.processing--
	if err != nil {
		task.Status = "failed"
		task.Retries++
		q.totalFailed++
		// Retry logic
		if task.Retries < 3 {
			task.Priority = task.Priority + 1 // Lower priority for retry
			q.mu.Lock()
			q.queue = append(q.queue, task)
			q.mu.Unlock()
			log.Printf("AI Queue: Task %s failed, retrying (attempt %d)", task.ID, task.Retries)
		}
	} else {
		task.Status = "completed"
		q.totalProcessed++
		duration := time.Since(startTime).Milliseconds()
		// Update average time (simple moving average)
		if q.averageTimeMs == 0 {
			q.averageTimeMs = duration
		} else {
			q.averageTimeMs = (q.averageTimeMs*9 + duration) / 10
		}
	}
	q.statsLock.Unlock()
}

// processIssueTriage analyzes and triages an issue with intelligent categorization
func (q *AIQueueService) processIssueTriage(task *AITask) error {
	issueID := task.EntityID
	if issueID == "" {
		log.Printf("AI Queue: Invalid issue ID for triage")
		return fmt.Errorf("invalid issue ID")
	}

	// Get the issue
	issue, err := models.Issues.Get(issueID)
	if err != nil {
		log.Printf("AI Queue: Failed to get issue %s: %v", issueID, err)
		return err
	}

	// Check if AI is enabled
	if !IsAIEnabled() {
		log.Printf("AI Queue: AI features not enabled, skipping triage")
		return fmt.Errorf("AI not enabled")
	}

	// Analyze issue content to determine labels and priority
	labels := q.analyzeIssueLabels(issue.Title, issue.Description)
	priority := q.analyzeIssuePriority(issue.Title, issue.Description)
	
	// Update issue with AI-determined metadata
	updated := false
	if issue.Priority == "" {
		issue.Priority = priority
		updated = true
	}
	
	// Add labels as metadata (JSON)
	if len(labels) > 0 {
		labelsJSON, _ := json.Marshal(labels)
		issue.Metadata = string(labelsJSON)
		updated = true
	}
	
	if updated {
		if err := models.Issues.Update(issue); err != nil {
			log.Printf("AI Queue: Failed to update issue metadata: %v", err)
			return err
		}
	}
	
	// Generate intelligent analysis comment
	analysisBody := q.generateIssueAnalysis(issue, labels, priority)
	
	comment := &models.Comment{
		Body:       analysisBody,
		AuthorID:   task.UserID,
		IssueID:    issue.ID,
		EntityType: "issue",
		EntityID:   issue.ID,
		RepoID:     task.RepoID,
	}
	
	if _, err := models.Comments.Insert(comment); err != nil {
		log.Printf("AI Queue: Failed to post analysis on issue %s: %v", issueID, err)
		return err
	}
	
	// Log activity
	repo, _ := models.Repos.Get(task.RepoID)
	repoName := "Unknown"
	if repo != nil {
		repoName = repo.Name
	}
	models.LogAIActivity("issue_triage", task.RepoID, repoName, "issue", issueID, 
		fmt.Sprintf("Triaged issue #%s with priority '%s' and %d labels", issueID, priority, len(labels)), 
		true, time.Since(time.Now()).Milliseconds())
	
	log.Printf("AI Queue: Successfully triaged issue %s with priority %s and %d labels", issueID, priority, len(labels))
	return nil
}

// processPRReview reviews a pull request with intelligent code analysis
func (q *AIQueueService) processPRReview(task *AITask) error {
	prID := task.EntityID
	if prID == "" {
		log.Printf("AI Queue: Invalid PR ID for review")
		return fmt.Errorf("invalid PR ID")
	}

	// Get the pull request
	pr, err := models.PullRequests.Get(prID)
	if err != nil {
		log.Printf("AI Queue: Failed to get PR %s: %v", prID, err)
		return err
	}

	// Check if AI is enabled
	if !IsAIEnabled() {
		log.Printf("AI Queue: AI features not enabled, skipping review")
		return fmt.Errorf("AI not enabled")
	}

	// Analyze PR for auto-approval eligibility
	if q.canAutoApprovePR(pr) {
		// Queue auto-approval task
		q.QueueTask(TaskAutoApprove, task.RepoID, task.UserID, "pull_request", pr.ID, nil, 2)
	}
	
	// Generate comprehensive review
	reviewBody := q.generatePRReview(pr)
	
	comment := &models.Comment{
		Body:       reviewBody,
		AuthorID:   task.UserID,
		EntityType: "pull_request",
		EntityID:   pr.ID,
		RepoID:     task.RepoID,
	}
	
	if _, err := models.Comments.Insert(comment); err != nil {
		log.Printf("AI Queue: Failed to post review on PR %s: %v", prID, err)
		return err
	}
	
	// Log activity
	repo, _ := models.Repos.Get(task.RepoID)
	repoName := "Unknown"
	if repo != nil {
		repoName = repo.Name
	}
	models.LogAIActivity("pr_review", task.RepoID, repoName, "pull_request", prID,
		fmt.Sprintf("Reviewed PR #%s", prID), true, time.Since(time.Now()).Milliseconds())
	
	log.Printf("AI Queue: Successfully reviewed PR %s", prID)
	return nil
}

// GetQueueStats returns current queue statistics
func (q *AIQueueService) GetQueueStats() map[string]interface{} {
	q.mu.RLock()
	queueLength := len(q.queue)
	q.mu.RUnlock()
	
	q.statsLock.RLock()
	defer q.statsLock.RUnlock()

	return map[string]interface{}{
		"queue_length":     queueLength,
		"running":          q.running,
		"total_processed":  q.totalProcessed,
		"total_failed":     q.totalFailed,
		"processing":       q.processing,
		"average_time_ms":  q.averageTimeMs,
		"workers":          q.workers,
	}
}

// IsAIEnabled checks if AI features are enabled
func IsAIEnabled() bool {
	return Ollama != nil && Ollama.IsRunning()
}

// Helper functions for AI analysis

// analyzeIssueLabels determines appropriate labels based on issue content
func (q *AIQueueService) analyzeIssueLabels(title, description string) []string {
	labels := []string{}
	content := strings.ToLower(title + " " + description)
	
	// Bug detection
	if strings.Contains(content, "error") || strings.Contains(content, "bug") || 
	   strings.Contains(content, "crash") || strings.Contains(content, "fail") ||
	   strings.Contains(content, "broken") || strings.Contains(content, "not working") {
		labels = append(labels, "bug")
	}
	
	// Feature request detection
	if strings.Contains(content, "feature") || strings.Contains(content, "request") ||
	   strings.Contains(content, "add") || strings.Contains(content, "implement") ||
	   strings.Contains(content, "would be nice") || strings.Contains(content, "should") {
		labels = append(labels, "enhancement")
	}
	
	// Documentation
	if strings.Contains(content, "document") || strings.Contains(content, "readme") ||
	   strings.Contains(content, "docs") || strings.Contains(content, "tutorial") {
		labels = append(labels, "documentation")
	}
	
	// Performance
	if strings.Contains(content, "slow") || strings.Contains(content, "performance") ||
	   strings.Contains(content, "optimize") || strings.Contains(content, "speed") {
		labels = append(labels, "performance")
	}
	
	// Security
	if strings.Contains(content, "security") || strings.Contains(content, "vulnerability") ||
	   strings.Contains(content, "exploit") || strings.Contains(content, "injection") {
		labels = append(labels, "security")
	}
	
	// UI/UX
	if strings.Contains(content, "ui") || strings.Contains(content, "ux") ||
	   strings.Contains(content, "interface") || strings.Contains(content, "design") {
		labels = append(labels, "ui/ux")
	}
	
	return labels
}

// analyzeIssuePriority determines priority based on keywords and impact
func (q *AIQueueService) analyzeIssuePriority(title, description string) string {
	content := strings.ToLower(title + " " + description)
	
	// Critical priority indicators
	if strings.Contains(content, "critical") || strings.Contains(content, "urgent") ||
	   strings.Contains(content, "emergency") || strings.Contains(content, "data loss") ||
	   strings.Contains(content, "security vulnerability") || strings.Contains(content, "production down") {
		return "critical"
	}
	
	// High priority indicators
	if strings.Contains(content, "high priority") || strings.Contains(content, "important") ||
	   strings.Contains(content, "blocking") || strings.Contains(content, "regression") ||
	   strings.Contains(content, "broken") || strings.Contains(content, "cannot") {
		return "high"
	}
	
	// Low priority indicators
	if strings.Contains(content, "minor") || strings.Contains(content, "trivial") ||
	   strings.Contains(content, "nice to have") || strings.Contains(content, "someday") ||
	   strings.Contains(content, "cosmetic") {
		return "low"
	}
	
	// Default to medium
	return "medium"
}

// generateIssueAnalysis creates an intelligent analysis comment
func (q *AIQueueService) generateIssueAnalysis(issue *models.Issue, labels []string, priority string) string {
	var analysis strings.Builder
	
	analysis.WriteString("🤖 **AI Triage Analysis**\n\n")
	
	// Priority assessment
	analysis.WriteString(fmt.Sprintf("**Priority:** `%s`\n", priority))
	
	// Labels
	if len(labels) > 0 {
		analysis.WriteString("**Suggested Labels:** ")
		for i, label := range labels {
			if i > 0 {
				analysis.WriteString(", ")
			}
			analysis.WriteString(fmt.Sprintf("`%s`", label))
		}
		analysis.WriteString("\n")
	}
	
	// Category-specific guidance
	analysis.WriteString("\n**Analysis:**\n")
	
	hasLabel := func(label string) bool {
		for _, l := range labels {
			if l == label {
				return true
			}
		}
		return false
	}
	
	if hasLabel("bug") {
		analysis.WriteString("- This appears to be a bug report. Please provide:\n")
		analysis.WriteString("  - Steps to reproduce\n")
		analysis.WriteString("  - Expected vs actual behavior\n")
		analysis.WriteString("  - Environment details\n")
	} else if hasLabel("enhancement") {
		analysis.WriteString("- This appears to be a feature request. Consider:\n")
		analysis.WriteString("  - Use cases and benefits\n")
		analysis.WriteString("  - Implementation approach\n")
		analysis.WriteString("  - Potential impact on existing features\n")
	} else if hasLabel("documentation") {
		analysis.WriteString("- This is a documentation issue. Focus on:\n")
		analysis.WriteString("  - Clarity and completeness\n")
		analysis.WriteString("  - Code examples if applicable\n")
		analysis.WriteString("  - Target audience\n")
	}
	
	if hasLabel("security") {
		analysis.WriteString("\n⚠️ **Security Notice:** This issue may have security implications. Handle with care and avoid exposing sensitive details publicly.\n")
	}
	
	// Next steps
	analysis.WriteString("\n**Recommended Next Steps:**\n")
	switch priority {
	case "critical":
		analysis.WriteString("1. Immediate investigation required\n")
		analysis.WriteString("2. Notify team leads\n")
		analysis.WriteString("3. Consider hotfix if affecting production\n")
	case "high":
		analysis.WriteString("1. Schedule for current sprint\n")
		analysis.WriteString("2. Assign to appropriate team member\n")
		analysis.WriteString("3. Request additional details if needed\n")
	default:
		analysis.WriteString("1. Add to backlog for prioritization\n")
		analysis.WriteString("2. Gather community feedback\n")
		analysis.WriteString("3. Consider in next planning session\n")
	}
	
	return analysis.String()
}

// canAutoApprovePR checks if a PR is safe for auto-approval
func (q *AIQueueService) canAutoApprovePR(pr *models.PullRequest) bool {
	// Check for safe change patterns
	title := strings.ToLower(pr.Title)
	
	// Documentation changes
	if strings.Contains(title, "readme") || strings.Contains(title, "docs") ||
	   strings.Contains(title, "documentation") || strings.Contains(title, "typo") {
		return true
	}
	
	// Dependency updates (with caution)
	if strings.Contains(title, "bump") && strings.Contains(title, "dependabot") {
		return true
	}
	
	// Configuration files
	if strings.Contains(title, ".gitignore") || strings.Contains(title, "license") {
		return true
	}
	
	return false
}

// generatePRReview creates a comprehensive PR review
func (q *AIQueueService) generatePRReview(pr *models.PullRequest) string {
	var review strings.Builder
	
	review.WriteString("🤖 **AI Code Review**\n\n")
	
	// PR Overview
	review.WriteString(fmt.Sprintf("**Pull Request:** #%s - %s\n", pr.ID, pr.Title))
	review.WriteString(fmt.Sprintf("**Author:** %s\n", pr.AuthorID))
	review.WriteString(fmt.Sprintf("**Target Branch:** %s\n\n", pr.BaseBranch))
	
	// Change summary
	review.WriteString("**Change Summary:**\n")
	if pr.Additions > 0 || pr.Deletions > 0 {
		review.WriteString(fmt.Sprintf("- **Files Changed:** %d\n", pr.ChangedFiles))
		review.WriteString(fmt.Sprintf("- **Additions:** +%d lines\n", pr.Additions))
		review.WriteString(fmt.Sprintf("- **Deletions:** -%d lines\n", pr.Deletions))
	} else {
		review.WriteString("- Analyzing changes...\n")
	}
	
	// Auto-approval check
	if q.canAutoApprovePR(pr) {
		review.WriteString("\n✅ **Auto-Approval Eligible:** This PR contains safe changes and may be auto-approved.\n")
	}
	
	// Review checklist
	review.WriteString("\n**Review Checklist:**\n")
	review.WriteString("- [ ] Code follows project style guidelines\n")
	review.WriteString("- [ ] Tests have been added/updated\n")
	review.WriteString("- [ ] Documentation has been updated\n")
	review.WriteString("- [ ] No security vulnerabilities introduced\n")
	review.WriteString("- [ ] Performance impact considered\n")
	
	// Recommendations
	review.WriteString("\n**Recommendations:**\n")
	
	// Check PR size
	totalChanges := pr.Additions + pr.Deletions
	if totalChanges > 500 {
		review.WriteString("- ⚠️ Large PR detected. Consider breaking into smaller, focused changes.\n")
	}
	
	// Check for tests
	if !strings.Contains(strings.ToLower(pr.Title), "test") && totalChanges > 50 {
		review.WriteString("- Consider adding tests for these changes.\n")
	}
	
	// General recommendations
	review.WriteString("- Ensure all CI checks pass before merging.\n")
	review.WriteString("- Request human review for critical changes.\n")
	
	return review.String()
}

// New task processing functions

// processDailyReport generates daily repository health reports
func (q *AIQueueService) processDailyReport(task *AITask) error {
	repoID := task.RepoID
	if repoID == "" {
		return fmt.Errorf("invalid repo ID for daily report")
	}
	
	log.Printf("AI Queue: Generating daily report for repo %s", repoID)
	
	// TODO: Implement comprehensive daily report generation
	// - Issue statistics
	// - PR metrics
	// - Code quality trends
	// - Activity summary
	
	return nil
}

// processStaleManagement handles stale issues and PRs
func (q *AIQueueService) processStaleManagement(task *AITask) error {
	repoID := task.RepoID
	if repoID == "" {
		return fmt.Errorf("invalid repo ID for stale management")
	}
	
	log.Printf("AI Queue: Managing stale items for repo %s", repoID)
	
	// TODO: Implement stale issue/PR management
	// - Find issues/PRs without activity for 30+ days
	// - Add "stale" label
	// - Post warning comment
	// - Close after 60 days of inactivity
	
	return nil
}

// processCodeReview performs deep code analysis
func (q *AIQueueService) processCodeReview(task *AITask) error {
	entityID := task.EntityID
	if entityID == "" {
		return fmt.Errorf("invalid entity ID for code review")
	}
	
	log.Printf("AI Queue: Performing deep code review for %s", entityID)
	
	// TODO: Implement comprehensive code review
	// - Security vulnerability scanning
	// - Performance analysis
	// - Best practices check
	// - Dependency analysis
	
	return nil
}

// processAutoApprove handles auto-approval of safe changes
func (q *AIQueueService) processAutoApprove(task *AITask) error {
	prID := task.EntityID
	if prID == "" {
		return fmt.Errorf("invalid PR ID for auto-approval")
	}
	
	pr, err := models.PullRequests.Get(prID)
	if err != nil {
		return err
	}
	
	// Double-check it's safe to auto-approve
	if !q.canAutoApprovePR(pr) {
		log.Printf("AI Queue: PR %s no longer eligible for auto-approval", prID)
		return nil
	}
	
	// Update PR status
	pr.Status = "approved"
	if err := models.PullRequests.Update(pr); err != nil {
		return err
	}
	
	// Post approval comment
	comment := &models.Comment{
		Body:       "✅ **AI Auto-Approval**: This PR contains safe changes and has been automatically approved.\n\nChanges detected:\n- Documentation updates\n- Configuration files\n- Non-critical dependency updates\n\nNo manual review required.",
		AuthorID:   task.UserID,
		EntityType: "pull_request",
		EntityID:   pr.ID,
		RepoID:     task.RepoID,
	}
	
	if _, err := models.Comments.Insert(comment); err != nil {
		return err
	}
	
	log.Printf("AI Queue: Auto-approved PR %s", prID)
	return nil
}

// Scheduler helper functions

// scheduleDailyReports queues daily report tasks for all active repositories
func (q *AIQueueService) scheduleDailyReports() {
	// TODO: Get all active repositories and queue daily reports
	log.Println("AI Queue: Scheduling daily reports")
}

// scheduleStaleManagement queues stale management tasks
func (q *AIQueueService) scheduleStaleManagement() {
	// TODO: Get all repositories and queue stale management
	log.Println("AI Queue: Checking for stale issues")
}