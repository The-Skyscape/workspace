package models

import (
	"fmt"

	"github.com/The-Skyscape/devtools/pkg/database"
	"github.com/The-Skyscape/devtools/pkg/security"
)

// Secrets provides the secrets collection with automatic fallback from Vault -> File -> Memory
// This manages all API keys and sensitive configuration for the application
var Secrets = security.Manage(
	security.WithVault(
		security.WithContainerName("skyscape-vault"),
		security.WithPort(8200),
		security.WithDataDir(fmt.Sprintf("%s/vault", database.DataDir())),
		security.WithDevMode(true),
		security.WithRootToken("skyscape-dev-token"),
	),
)

// GitHub OAuth token storage keys
const (
	GitHubOAuthPrefix = "github/oauth/"
	GitHubUserPrefix  = "github/users/"
	GitHubRepoPrefix  = "github/repos/"
)

// StoreGitHubOAuthToken stores a GitHub OAuth token for a user
func StoreGitHubOAuthToken(userID string, token string) error {
	key := fmt.Sprintf("%s%s", GitHubUserPrefix, userID)
	return Secrets.StoreSecret(key, map[string]interface{}{
		"token": token,
	})
}

// GetGitHubOAuthToken retrieves a GitHub OAuth token for a user
func GetGitHubOAuthToken(userID string) (string, error) {
	key := fmt.Sprintf("%s%s", GitHubUserPrefix, userID)
	secret, err := Secrets.GetSecret(key)
	if err != nil {
		return "", err
	}
	
	token, ok := secret["token"].(string)
	if !ok {
		return "", fmt.Errorf("token not found or invalid format")
	}
	
	return token, nil
}

// StoreGitHubRepoIntegration stores GitHub integration data for a repository
func StoreGitHubRepoIntegration(repoID string, data map[string]interface{}) error {
	key := fmt.Sprintf("%s%s", GitHubRepoPrefix, repoID)
	return Secrets.StoreSecret(key, data)
}

// GetGitHubRepoIntegration retrieves GitHub integration data for a repository
func GetGitHubRepoIntegration(repoID string) (map[string]interface{}, error) {
	key := fmt.Sprintf("%s%s", GitHubRepoPrefix, repoID)
	return Secrets.GetSecret(key)
}

// DeleteGitHubRepoIntegration removes GitHub integration data for a repository
func DeleteGitHubRepoIntegration(repoID string) error {
	key := fmt.Sprintf("%s%s", GitHubRepoPrefix, repoID)
	return Secrets.DeleteSecret(key)
}

// DeleteGitHubOAuthToken removes a user's GitHub OAuth token from vault
func DeleteGitHubOAuthToken(userID string) error {
	key := fmt.Sprintf("%s%s", GitHubUserPrefix, userID)
	return Secrets.DeleteSecret(key)
}