package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/pkg/errors"
)

// VaultClient provides a simple interface to Vault for secret storage
type VaultClient struct {
	baseURL string
	token   string
	client  *http.Client
}

// NewVaultClient creates a new Vault client
func NewVaultClient() *VaultClient {
	config := Vault.GetConfig()
	return &VaultClient{
		baseURL: fmt.Sprintf("http://localhost:%d", config.Port),
		token:   config.RootToken,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// StoreSecret stores a secret at the given path
func (v *VaultClient) StoreSecret(path string, data map[string]interface{}) error {
	url := fmt.Sprintf("%s/v1/secret/data/%s", v.baseURL, path)
	
	payload := map[string]interface{}{
		"data": data,
	}
	
	body, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "failed to marshal secret data")
	}
	
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return errors.Wrap(err, "failed to create request")
	}
	
	req.Header.Set("X-Vault-Token", v.token)
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := v.client.Do(req)
	if err != nil {
		return errors.Wrap(err, "failed to store secret")
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("vault returned status %d: %s", resp.StatusCode, string(body))
	}
	
	return nil
}

// GetSecret retrieves a secret from the given path
func (v *VaultClient) GetSecret(path string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/v1/secret/data/%s", v.baseURL, path)
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create request")
	}
	
	req.Header.Set("X-Vault-Token", v.token)
	
	resp, err := v.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get secret")
	}
	defer resp.Body.Close()
	
	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("secret not found")
	}
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vault returned status %d: %s", resp.StatusCode, string(body))
	}
	
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, errors.Wrap(err, "failed to decode response")
	}
	
	// Extract the actual secret data from the Vault response structure
	if data, ok := result["data"].(map[string]interface{}); ok {
		if secretData, ok := data["data"].(map[string]interface{}); ok {
			return secretData, nil
		}
	}
	
	return nil, errors.New("unexpected vault response format")
}

// DeleteSecret removes a secret at the given path
func (v *VaultClient) DeleteSecret(path string) error {
	url := fmt.Sprintf("%s/v1/secret/metadata/%s", v.baseURL, path)
	
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return errors.Wrap(err, "failed to create request")
	}
	
	req.Header.Set("X-Vault-Token", v.token)
	
	resp, err := v.client.Do(req)
	if err != nil {
		return errors.Wrap(err, "failed to delete secret")
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("vault returned status %d: %s", resp.StatusCode, string(body))
	}
	
	return nil
}

// StoreTemporary stores a temporary value with TTL
func (v *VaultClient) StoreTemporary(key string, value string, ttl time.Duration) error {
	data := map[string]interface{}{
		"value":      value,
		"expires_at": time.Now().Add(ttl).Unix(),
	}
	return v.StoreSecret(fmt.Sprintf("temp/%s", key), data)
}

// GetTemporary retrieves a temporary value
func (v *VaultClient) GetTemporary(key string) (string, error) {
	secret, err := v.GetSecret(fmt.Sprintf("temp/%s", key))
	if err != nil {
		return "", err
	}
	
	// Check if expired
	if expiresAt, ok := secret["expires_at"].(float64); ok {
		if time.Now().Unix() > int64(expiresAt) {
			// Clean up expired secret
			v.DeleteSecret(fmt.Sprintf("temp/%s", key))
			return "", errors.New("temporary value expired")
		}
	}
	
	if value, ok := secret["value"].(string); ok {
		return value, nil
	}
	
	return "", errors.New("invalid temporary value format")
}

// StoreGitHubToken stores a GitHub access token for a user
func (v *VaultClient) StoreGitHubToken(userID string, token string, username string) error {
	data := map[string]interface{}{
		"access_token":    token,
		"github_username": username,
		"stored_at":       time.Now().Unix(),
	}
	return v.StoreSecret(fmt.Sprintf("github/users/%s", userID), data)
}

// GetGitHubToken retrieves a GitHub access token for a user
func (v *VaultClient) GetGitHubToken(userID string) (string, error) {
	secret, err := v.GetSecret(fmt.Sprintf("github/users/%s", userID))
	if err != nil {
		return "", err
	}
	
	if token, ok := secret["access_token"].(string); ok {
		return token, nil
	}
	
	return "", errors.New("no GitHub token found")
}

// StoreRepoSecret stores a secret for a repository
func (v *VaultClient) StoreRepoSecret(repoID string, key string, value interface{}) error {
	// Get existing secrets for the repo
	existing, _ := v.GetSecret(fmt.Sprintf("repos/%s", repoID))
	if existing == nil {
		existing = make(map[string]interface{})
	}
	
	// Update with new value
	existing[key] = value
	existing["updated_at"] = time.Now().Unix()
	
	return v.StoreSecret(fmt.Sprintf("repos/%s", repoID), existing)
}

// GetRepoSecret retrieves a specific secret for a repository
func (v *VaultClient) GetRepoSecret(repoID string, key string) (interface{}, error) {
	secrets, err := v.GetSecret(fmt.Sprintf("repos/%s", repoID))
	if err != nil {
		return nil, err
	}
	
	if value, ok := secrets[key]; ok {
		return value, nil
	}
	
	return nil, errors.New("secret not found")
}

// ListSecrets lists all secrets at a given path (metadata only)
func (v *VaultClient) ListSecrets(path string) ([]string, error) {
	url := fmt.Sprintf("%s/v1/secret/metadata/%s", v.baseURL, path)
	
	req, err := http.NewRequest("LIST", url, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create request")
	}
	
	req.Header.Set("X-Vault-Token", v.token)
	
	resp, err := v.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list secrets")
	}
	defer resp.Body.Close()
	
	if resp.StatusCode == http.StatusNotFound {
		return []string{}, nil
	}
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vault returned status %d: %s", resp.StatusCode, string(body))
	}
	
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, errors.Wrap(err, "failed to decode response")
	}
	
	// Extract the keys from the response
	if data, ok := result["data"].(map[string]interface{}); ok {
		if keys, ok := data["keys"].([]interface{}); ok {
			paths := make([]string, len(keys))
			for i, key := range keys {
				paths[i] = key.(string)
			}
			return paths, nil
		}
	}
	
	return []string{}, nil
}