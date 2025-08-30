package ai

import (
	"fmt"
	"log"
	"sync"
)

// AuthManager handles AI Worker API authentication
type AuthManager struct {
	vault      VaultInterface
	apiKey     string
	configured bool
	mu         sync.RWMutex // Only for API key, not for stats
}

// VaultInterface defines the interface for vault operations
type VaultInterface interface {
	GetSecret(path string) (map[string]interface{}, error)
	StoreSecret(path string, data map[string]interface{}) error
	DeleteSecret(path string) error
}

// NewAuthManager creates a new authentication manager
func NewAuthManager(vault VaultInterface) *AuthManager {
	am := &AuthManager{
		vault: vault,
	}
	am.Initialize()
	return am
}

// Initialize loads the API key from vault
func (am *AuthManager) Initialize() error {
	am.mu.Lock()
	defer am.mu.Unlock()

	if am.vault == nil {
		log.Println("Worker: Vault not configured, skipping initialization")
		return nil
	}

	secret, err := am.vault.GetSecret("integrations/worker")
	if err != nil {
		// Not an error if not configured yet
		log.Println("Worker: No API key configured")
		return nil
	}

	if apiKey, ok := secret["api_key"].(string); ok && apiKey != "" {
		am.apiKey = apiKey
		am.configured = true
		// Don't log on every initialization
	}

	return nil
}

// SetAPIKey stores the API key in vault
func (am *AuthManager) SetAPIKey(apiKey string) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	if am.vault == nil {
		return fmt.Errorf("vault not configured")
	}

	// Validate the API key first
	client := NewClient(apiKey)
	if err := client.ValidateAPIKey(); err != nil {
		return fmt.Errorf("invalid API key: %w", err)
	}

	// Store in vault
	data := map[string]interface{}{
		"api_key": apiKey,
	}

	if err := am.vault.StoreSecret("integrations/worker", data); err != nil {
		return fmt.Errorf("failed to store API key: %w", err)
	}

	am.apiKey = apiKey
	am.configured = true
	log.Println("Worker: API key stored in vault")

	return nil
}

// GetAPIKey retrieves the current API key
func (am *AuthManager) GetAPIKey() string {
	am.mu.RLock()
	defer am.mu.RUnlock()
	return am.apiKey
}

// IsConfigured checks if Worker is configured
func (am *AuthManager) IsConfigured() bool {
	am.mu.RLock()
	defer am.mu.RUnlock()
	return am.configured
}

// RemoveConfiguration removes the Worker configuration
func (am *AuthManager) RemoveConfiguration() error {
	am.mu.Lock()
	defer am.mu.Unlock()

	if am.vault == nil {
		return fmt.Errorf("vault not configured")
	}

	if err := am.vault.DeleteSecret("integrations/worker"); err != nil {
		return fmt.Errorf("failed to remove configuration: %w", err)
	}

	am.apiKey = ""
	am.configured = false
	log.Println("Worker: Configuration removed")

	return nil
}