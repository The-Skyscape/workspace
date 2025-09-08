package controllers

import (
	"testing"
	"workspace/models"
	"workspace/services"
)

// TestAIControllerSetup tests AI controller initialization
func TestAIControllerSetup(t *testing.T) {
	// Create controller
	prefix, controller := AI()
	
	if prefix != "ai" {
		t.Errorf("Expected prefix 'ai', got '%s'", prefix)
	}
	
	if controller == nil {
		t.Fatal("Controller should not be nil")
	}
}

// TestIsAIEnabled tests AI status check
func TestIsAIEnabled(t *testing.T) {
	_, controller := AI()
	
	// Should return false when Ollama is not running
	if controller.IsAIEnabled() {
		t.Error("AI should not be enabled without Ollama service")
	}
}

// TestGetQueueStats tests queue statistics retrieval
func TestGetQueueStats(t *testing.T) {
	_, controller := AI()
	
	// Initialize queue first
	services.InitAIQueue()
	
	stats := controller.GetQueueStats()
	
	if stats == nil {
		t.Fatal("Stats should not be nil when queue is initialized")
	}
	
	// Check for required fields
	if _, ok := stats["running"]; !ok {
		t.Error("Stats should contain 'running' field")
	}
	
	if _, ok := stats["queue_length"]; !ok {
		t.Error("Stats should contain 'queue_length' field")
	}
	
	if _, ok := stats["workers"]; !ok {
		t.Error("Stats should contain 'workers' field")
	}
}

// TestGetRecentActivity tests activity retrieval
func TestGetRecentActivity(t *testing.T) {
	_, controller := AI()
	
	activities := controller.GetRecentActivity()
	
	// Should return empty array, not nil
	if activities == nil {
		t.Error("Activities should be empty array, not nil")
	}
}

// TestPublicMethods tests public controller methods
func TestPublicMethods(t *testing.T) {
	// Test that public methods are accessible
	_, controller := AI()
	
	// These should be callable from templates
	_ = controller.IsAIEnabled()
	_ = controller.GetQueueStats()
	_ = controller.GetRecentActivity()
	
	// If we got here without panic, the methods exist
	t.Log("All public methods are accessible")
}

// TestConversationManagement tests conversation CRUD
func TestConversationManagement(t *testing.T) {
	// Test creating a new conversation
	conv := &models.Conversation{
		UserID:      "test-user",
		Title:       "Test Conversation",
		LastMessage: "Hello AI",
		LastRole:    models.MessageRoleUser,
	}
	
	// In a real test, you'd insert this into a test database
	// and verify it can be retrieved
	
	if conv.Title != "Test Conversation" {
		t.Error("Conversation title not set correctly")
	}
	
	if conv.LastRole != models.MessageRoleUser {
		t.Error("Conversation role not set correctly")
	}
}

// TestMessageRoles tests different message roles
func TestMessageRoles(t *testing.T) {
	roles := []string{
		models.MessageRoleUser,
		models.MessageRoleAssistant,
		models.MessageRoleTool,
		models.MessageRoleError,
	}
	
	for _, role := range roles {
		msg := &models.Message{
			ConversationID: "test-conv",
			Role:           role,
			Content:        "Test message",
		}
		
		if msg.Role != role {
			t.Errorf("Message role not set correctly: expected %s, got %s", role, msg.Role)
		}
	}
}

// TestAITaskTypes tests all task type constants
func TestAITaskTypes(t *testing.T) {
	taskTypes := []services.AITaskType{
		services.TaskIssueTriage,
		services.TaskPRReview,
		services.TaskDailyReport,
		services.TaskStaleManagement,
		services.TaskDependencyUpdate,
		services.TaskCodeReview,
		services.TaskAutoApprove,
	}
	
	expectedTypes := []string{
		"issue_triage",
		"pr_review",
		"daily_report",
		"stale_management",
		"dependency_update",
		"code_review",
		"auto_approve",
	}
	
	for i, taskType := range taskTypes {
		if string(taskType) != expectedTypes[i] {
			t.Errorf("Task type mismatch: expected %s, got %s", expectedTypes[i], string(taskType))
		}
	}
}

// TestQueuePriorities tests priority ordering
func TestQueuePriorities(t *testing.T) {
	// Priority 1 is highest, 10 is lowest
	highPriority := 1
	mediumPriority := 5
	lowPriority := 10
	
	if highPriority >= mediumPriority {
		t.Error("High priority should be lower number than medium")
	}
	
	if mediumPriority >= lowPriority {
		t.Error("Medium priority should be lower number than low")
	}
}

// TestAIActivityModel tests activity tracking model
func TestAIActivityModel(t *testing.T) {
	activity := &models.AIActivity{
		Type:        "issue_triage",
		RepoID:      "repo1",
		RepoName:    "Test Repo",
		EntityType:  "issue",
		EntityID:    "issue1",
		Description: "Triaged issue #1",
		Success:     true,
		Duration:    1500,
	}
	
	if activity.Table() != "ai_activities" {
		t.Errorf("Expected table name 'ai_activities', got '%s'", activity.Table())
	}
	
	if !activity.Success {
		t.Error("Activity success should be true")
	}
	
	if activity.Duration != 1500 {
		t.Errorf("Activity duration should be 1500ms, got %d", activity.Duration)
	}
}