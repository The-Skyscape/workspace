package ai

import (
	"fmt"
	"testing"
)

// MockSandbox implements SandboxInterface for testing
type MockSandbox struct {
	name      string
	running   bool
	output    string
	exitCode  int
	executeErr error
}

func (m *MockSandbox) Start() error {
	m.running = true
	return nil
}

func (m *MockSandbox) Stop() error {
	m.running = false
	return nil
}

func (m *MockSandbox) IsRunning() bool {
	return m.running
}

func (m *MockSandbox) Execute(command string) (string, int, error) {
	if m.executeErr != nil {
		return "", m.exitCode, m.executeErr
	}
	return m.output, m.exitCode, nil
}

func (m *MockSandbox) GetOutput() (string, error) {
	return m.output, nil
}

func (m *MockSandbox) GetLogs(tail int) (string, error) {
	return m.output, nil
}

func (m *MockSandbox) WaitForCompletion() error {
	return nil
}

func (m *MockSandbox) Cleanup() error {
	m.running = false
	return nil
}

func (m *MockSandbox) GetStatus() map[string]interface{} {
	return map[string]interface{}{
		"running": m.running,
		"name":    m.name,
	}
}

// MockSandboxService implements SandboxServiceInterface for testing
type MockSandboxService struct {
	sandboxes map[string]*MockSandbox
}

func NewMockSandboxService() *MockSandboxService {
	return &MockSandboxService{
		sandboxes: make(map[string]*MockSandbox),
	}
}

func (m *MockSandboxService) NewSandbox(name, repoPath, repoName, command string, timeoutSecs int) (SandboxInterface, error) {
	sandbox := &MockSandbox{
		name:    name,
		running: false,
		output:  "Sandbox created successfully",
	}
	m.sandboxes[name] = sandbox
	return sandbox, nil
}

func (m *MockSandboxService) GetSandbox(name string) (SandboxInterface, error) {
	if sandbox, exists := m.sandboxes[name]; exists {
		return sandbox, nil
	}
	return nil, fmt.Errorf("sandbox not found: %s", name)
}

func TestWorkerManager_InitializeWorker(t *testing.T) {
	// Setup mocks
	vault := NewMockVault()
	// Set API key directly in vault
	vault.StoreSecret("integrations/claude", map[string]interface{}{
		"api_key": "test-api-key",
	})
	authManager := NewAuthManager(vault)
	
	sandboxService := NewMockSandboxService()
	workerManager := NewWorkerManager(authManager, sandboxService)
	
	// Test worker initialization
	config := WorkerConfig{
		WorkerID:  "test-worker-123",
		RepoIDs:   []string{"repo-id-1", "repo-id-2"},
		RepoNames: []string{"repo1", "repo2"},
		UserID:    "user-123",
	}
	
	sandbox, err := workerManager.InitializeWorker(config)
	if err != nil {
		t.Fatalf("Failed to initialize worker: %v", err)
	}
	
	if sandbox == nil {
		t.Fatal("Expected sandbox to be created")
	}
	
	if !sandbox.IsRunning() {
		t.Error("Expected sandbox to be running after initialization")
	}
}

func TestWorkerManager_InitializeWorker_NoAPIKey(t *testing.T) {
	// Setup mocks without API key
	vault := NewMockVault()
	authManager := NewAuthManager(vault)
	// Don't set API key
	
	sandboxService := NewMockSandboxService()
	workerManager := NewWorkerManager(authManager, sandboxService)
	
	// Test worker initialization without API key
	config := WorkerConfig{
		WorkerID:  "test-worker-123",
		RepoIDs:   []string{"repo-id-1"},
		RepoNames: []string{"repo"},
		UserID:    "user-123",
	}
	
	_, err := workerManager.InitializeWorker(config)
	if err == nil {
		t.Fatal("Expected error when initializing without API key")
	}
	
	if err.Error() != "Claude API key not configured" {
		t.Errorf("Expected 'Claude API key not configured' error, got: %v", err)
	}
}

func TestWorkerManager_ExecuteMessage(t *testing.T) {
	// Setup mocks
	vault := NewMockVault()
	vault.StoreSecret("integrations/claude", map[string]interface{}{
		"api_key": "test-api-key",
	})
	authManager := NewAuthManager(vault)
	
	sandboxService := NewMockSandboxService()
	workerManager := NewWorkerManager(authManager, sandboxService)
	
	// Create a mock sandbox
	mockSandbox := &MockSandbox{
		name:    "test-sandbox",
		running: true,
		output:  "Claude response: Hello!",
	}
	
	// Test message execution
	response, err := workerManager.ExecuteMessage(mockSandbox, "Hello Claude")
	if err != nil {
		t.Fatalf("Failed to execute message: %v", err)
	}
	
	if response != "Claude response: Hello!" {
		t.Errorf("Expected 'Claude response: Hello!', got: %s", response)
	}
}

func TestWorkerManager_ExecuteMessage_NotRunning(t *testing.T) {
	// Setup mocks
	vault := NewMockVault()
	vault.StoreSecret("integrations/claude", map[string]interface{}{
		"api_key": "test-api-key",
	})
	authManager := NewAuthManager(vault)
	
	sandboxService := NewMockSandboxService()
	workerManager := NewWorkerManager(authManager, sandboxService)
	
	// Create a stopped sandbox
	mockSandbox := &MockSandbox{
		name:    "test-sandbox",
		running: false,
	}
	
	// Test message execution on stopped sandbox
	_, err := workerManager.ExecuteMessage(mockSandbox, "Hello Claude")
	if err == nil {
		t.Fatal("Expected error when executing on stopped sandbox")
	}
	
	if err.Error() != "sandbox is not running" {
		t.Errorf("Expected 'sandbox is not running' error, got: %v", err)
	}
}

func TestWorkerManager_CleanupWorker(t *testing.T) {
	// Setup mocks
	vault := NewMockVault()
	authManager := NewAuthManager(vault)
	
	sandboxService := NewMockSandboxService()
	workerManager := NewWorkerManager(authManager, sandboxService)
	
	// Create a sandbox
	sandboxName := "claude-worker-test"
	sandbox, _ := sandboxService.NewSandbox(sandboxName, "", "", "", 0)
	sandbox.Start()
	
	// Test cleanup
	err := workerManager.CleanupWorker(sandboxName)
	if err != nil {
		t.Fatalf("Failed to cleanup worker: %v", err)
	}
	
	if sandbox.IsRunning() {
		t.Error("Expected sandbox to be stopped after cleanup")
	}
}

func TestWorkerManager_GenerateCloneCommands(t *testing.T) {
	// Note: This test would need a mock for models.CreateAccessToken
	// For now, we'll skip testing the internal generateCloneCommandsWithTokens
	// as it requires database access
	t.Skip("Skipping test that requires database access")
}