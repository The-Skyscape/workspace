package claude

import (
	"testing"
)

// MockVault implements VaultInterface for testing
type MockVault struct {
	secrets map[string]map[string]interface{}
}

func NewMockVault() *MockVault {
	return &MockVault{
		secrets: make(map[string]map[string]interface{}),
	}
}

func (m *MockVault) GetSecret(path string) (map[string]interface{}, error) {
	if secret, exists := m.secrets[path]; exists {
		return secret, nil
	}
	return nil, nil
}

func (m *MockVault) StoreSecret(path string, data map[string]interface{}) error {
	m.secrets[path] = data
	return nil
}

func (m *MockVault) DeleteSecret(path string) error {
	delete(m.secrets, path)
	return nil
}

func TestAuthManager_SetAndGetAPIKey(t *testing.T) {
	vault := NewMockVault()
	authManager := NewAuthManager(vault)
	
	// Directly store API key in vault to skip validation
	testKey := "test-api-key-123"
	vault.StoreSecret("integrations/claude", map[string]interface{}{
		"api_key": testKey,
	})
	
	// Re-initialize to load from vault
	authManager.Initialize()
	
	// Test getting API key
	retrievedKey := authManager.GetAPIKey()
	if retrievedKey != testKey {
		t.Errorf("Expected API key %s, got %s", testKey, retrievedKey)
	}
	
	// Test IsConfigured
	if !authManager.IsConfigured() {
		t.Error("Expected authManager to be configured after setting API key")
	}
}

func TestAuthManager_RemoveConfiguration(t *testing.T) {
	vault := NewMockVault()
	authManager := NewAuthManager(vault)
	
	// Set configuration directly in vault
	vault.StoreSecret("integrations/claude", map[string]interface{}{
		"api_key": "test-key",
	})
	authManager.Initialize()
	
	err := authManager.RemoveConfiguration()
	if err != nil {
		t.Fatalf("Failed to remove configuration: %v", err)
	}
	
	// Verify configuration is removed
	if authManager.IsConfigured() {
		t.Error("Expected authManager to not be configured after removal")
	}
	
	if authManager.GetAPIKey() != "" {
		t.Error("Expected empty API key after removal")
	}
}

