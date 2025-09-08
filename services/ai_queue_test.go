package services

import (
	"testing"
	"time"
	"workspace/models"
)

// TestAIQueueInitialization tests queue service initialization
func TestAIQueueInitialization(t *testing.T) {
	// Initialize queue
	err := InitAIQueue()
	if err != nil {
		t.Fatalf("Failed to initialize AI queue: %v", err)
	}

	// Check if queue is running
	if AIQueue == nil {
		t.Fatal("AI queue is nil after initialization")
	}

	if !AIQueue.running {
		t.Fatal("AI queue is not running after initialization")
	}

	// Check worker count
	if AIQueue.workers != 3 {
		t.Errorf("Expected 3 workers, got %d", AIQueue.workers)
	}
}

// TestQueueTaskPriority tests priority-based task queuing
func TestQueueTaskPriority(t *testing.T) {
	// Initialize queue
	InitAIQueue()
	
	// Clear existing queue
	AIQueue.mu.Lock()
	AIQueue.queue = make([]*AITask, 0)
	AIQueue.mu.Unlock()

	// Add tasks with different priorities
	task1, _ := AIQueue.QueueTask(TaskIssueTriage, "repo1", "user1", "issue", "issue1", nil, 5)
	task2, _ := AIQueue.QueueTask(TaskPRReview, "repo1", "user1", "pr", "pr1", nil, 3)
	task3, _ := AIQueue.QueueTask(TaskAutoApprove, "repo1", "user1", "pr", "pr2", nil, 1)

	// Check queue order
	AIQueue.mu.RLock()
	defer AIQueue.mu.RUnlock()

	if len(AIQueue.queue) != 3 {
		t.Fatalf("Expected 3 tasks in queue, got %d", len(AIQueue.queue))
	}

	// Should be ordered by priority (1, 3, 5)
	if AIQueue.queue[0].ID != task3.ID {
		t.Errorf("Expected task3 (priority 1) first, got %s", AIQueue.queue[0].ID)
	}
	if AIQueue.queue[1].ID != task2.ID {
		t.Errorf("Expected task2 (priority 3) second, got %s", AIQueue.queue[1].ID)
	}
	if AIQueue.queue[2].ID != task1.ID {
		t.Errorf("Expected task1 (priority 5) third, got %s", AIQueue.queue[2].ID)
	}
}

// TestLabelAnalysis tests issue label detection
func TestLabelAnalysis(t *testing.T) {
	InitAIQueue()
	
	tests := []struct {
		title       string
		description string
		expected    []string
	}{
		{
			title:       "Application crashes on startup",
			description: "The app throws an error when I try to run it",
			expected:    []string{"bug"},
		},
		{
			title:       "Add dark mode feature",
			description: "It would be nice to have a dark theme option",
			expected:    []string{"enhancement"},
		},
		{
			title:       "Update README documentation",
			description: "The docs need to be updated with new API endpoints",
			expected:    []string{"documentation"},
		},
		{
			title:       "App is very slow",
			description: "Performance issues when loading large datasets",
			expected:    []string{"performance"},
		},
		{
			title:       "Security vulnerability in authentication",
			description: "SQL injection possible in login form",
			expected:    []string{"security"},
		},
		{
			title:       "Improve UI design",
			description: "The interface needs better user experience",
			expected:    []string{"ui/ux"},
		},
	}

	for _, test := range tests {
		labels := AIQueue.analyzeIssueLabels(test.title, test.description)
		
		if len(labels) == 0 {
			t.Errorf("No labels detected for '%s'", test.title)
			continue
		}

		found := false
		for _, expected := range test.expected {
			for _, label := range labels {
				if label == expected {
					found = true
					break
				}
			}
		}

		if !found {
			t.Errorf("Expected label %v not found in %v for '%s'", test.expected, labels, test.title)
		}
	}
}

// TestPriorityAnalysis tests issue priority detection
func TestPriorityAnalysis(t *testing.T) {
	InitAIQueue()
	
	tests := []struct {
		title       string
		description string
		expected    string
	}{
		{
			title:       "CRITICAL: Production down",
			description: "Complete data loss occurring",
			expected:    "critical",
		},
		{
			title:       "High priority bug",
			description: "This is blocking our release",
			expected:    "high",
		},
		{
			title:       "Minor typo in UI",
			description: "Small cosmetic issue",
			expected:    "low",
		},
		{
			title:       "Regular feature request",
			description: "Add new functionality",
			expected:    "medium",
		},
	}

	for _, test := range tests {
		priority := AIQueue.analyzeIssuePriority(test.title, test.description)
		
		if priority != test.expected {
			t.Errorf("Expected priority '%s' for '%s', got '%s'", test.expected, test.title, priority)
		}
	}
}

// TestAutoApprovalDetection tests PR auto-approval eligibility
func TestAutoApprovalDetection(t *testing.T) {
	InitAIQueue()
	
	tests := []struct {
		title    string
		expected bool
	}{
		{
			title:    "Update README.md",
			expected: true,
		},
		{
			title:    "Fix typo in documentation",
			expected: true,
		},
		{
			title:    "Update .gitignore",
			expected: true,
		},
		{
			title:    "Bump dependencies by dependabot",
			expected: true,
		},
		{
			title:    "Add new feature",
			expected: false,
		},
		{
			title:    "Refactor authentication logic",
			expected: false,
		},
	}

	for _, test := range tests {
		pr := &models.PullRequest{
			Title: test.title,
		}
		
		result := AIQueue.canAutoApprovePR(pr)
		
		if result != test.expected {
			t.Errorf("Expected auto-approval %v for '%s', got %v", test.expected, test.title, result)
		}
	}
}

// TestQueueStatistics tests queue statistics tracking
func TestQueueStatistics(t *testing.T) {
	InitAIQueue()
	
	// Add some tasks
	AIQueue.QueueTask(TaskIssueTriage, "repo1", "user1", "issue", "issue1", nil, 3)
	AIQueue.QueueTask(TaskPRReview, "repo1", "user1", "pr", "pr1", nil, 4)
	
	// Get stats
	stats := AIQueue.GetQueueStats()
	
	// Check required fields
	if stats["running"] != true {
		t.Error("Queue should be running")
	}
	
	if stats["workers"] != 3 {
		t.Errorf("Expected 3 workers, got %v", stats["workers"])
	}
	
	// Queue length should be at least 2
	queueLength := stats["queue_length"].(int)
	if queueLength < 2 {
		t.Errorf("Expected at least 2 tasks in queue, got %d", queueLength)
	}
}

// TestRetryLogic tests task retry mechanism
func TestRetryLogic(t *testing.T) {
	InitAIQueue()
	
	// Create a task that will fail
	task := &AITask{
		ID:         "test-retry",
		Type:       TaskIssueTriage,
		Priority:   3,
		RepoID:     "repo1",
		UserID:     "user1",
		EntityType: "issue",
		EntityID:   "nonexistent", // This will cause a failure
		CreatedAt:  time.Now(),
		Retries:    0,
		Status:     "queued",
	}
	
	// Add to queue
	AIQueue.mu.Lock()
	AIQueue.queue = append(AIQueue.queue, task)
	AIQueue.mu.Unlock()
	
	// Process the task (it should fail and be retried)
	AIQueue.processNextTask(0)
	
	// Check if task was retried
	if task.Retries != 1 {
		t.Errorf("Expected 1 retry, got %d", task.Retries)
	}
	
	// Check if priority was lowered
	if task.Priority != 4 {
		t.Errorf("Expected priority to be lowered to 4, got %d", task.Priority)
	}
}

// TestConcurrentAccess tests thread safety
func TestConcurrentAccess(t *testing.T) {
	InitAIQueue()
	
	// Clear queue
	AIQueue.mu.Lock()
	AIQueue.queue = make([]*AITask, 0)
	AIQueue.mu.Unlock()
	
	// Concurrently add tasks
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			AIQueue.QueueTask(
				TaskIssueTriage,
				"repo1",
				"user1",
				"issue",
				string(rune(id)),
				nil,
				id%5+1,
			)
			done <- true
		}(i)
	}
	
	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
	
	// Check queue length
	AIQueue.mu.RLock()
	queueLength := len(AIQueue.queue)
	AIQueue.mu.RUnlock()
	
	if queueLength != 10 {
		t.Errorf("Expected 10 tasks in queue, got %d", queueLength)
	}
}

// TestAnalysisContent tests the generated analysis content
func TestAnalysisContent(t *testing.T) {
	InitAIQueue()
	
	issue := &models.Issue{
		Title:       "Critical bug causing data loss",
		Description: "Users are experiencing data loss when saving",
	}
	
	labels := []string{"bug", "security"}
	priority := "critical"
	
	analysis := AIQueue.generateIssueAnalysis(issue, labels, priority)
	
	// Check if analysis contains expected elements
	if analysis == "" {
		t.Fatal("Analysis should not be empty")
	}
	
	// Should contain priority
	if !contains(analysis, "Priority:") || !contains(analysis, "critical") {
		t.Error("Analysis should contain priority information")
	}
	
	// Should contain labels
	if !contains(analysis, "bug") {
		t.Error("Analysis should contain bug label")
	}
	
	// Should contain security warning
	if !contains(analysis, "Security Notice") {
		t.Error("Analysis should contain security warning for security-labeled issues")
	}
	
	// Should contain next steps
	if !contains(analysis, "Next Steps") {
		t.Error("Analysis should contain recommended next steps")
	}
}

// TestPRReviewContent tests the generated PR review content
func TestPRReviewContent(t *testing.T) {
	InitAIQueue()
	
	pr := &models.PullRequest{
		Title:        "Add new feature",
		AuthorID:     "user123",
		BaseBranch:   "main",
		Additions:    600,
		Deletions:    100,
		ChangedFiles: 10,
	}
	
	review := AIQueue.generatePRReview(pr)
	
	// Check if review contains expected elements
	if review == "" {
		t.Fatal("Review should not be empty")
	}
	
	// Should contain PR info
	if !contains(review, "Pull Request:") {
		t.Error("Review should contain PR information")
	}
	
	// Should contain change summary
	if !contains(review, "Change Summary:") {
		t.Error("Review should contain change summary")
	}
	
	// Should warn about large PR
	if !contains(review, "Large PR detected") {
		t.Error("Review should warn about large PR (>500 lines)")
	}
	
	// Should contain checklist
	if !contains(review, "Review Checklist:") {
		t.Error("Review should contain review checklist")
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || contains(s[1:], substr)))
}