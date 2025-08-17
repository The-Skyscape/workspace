package models

import (
	"errors"
	"time"

	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/database"
)

// UserGitHub extends the authentication.User with GitHub-specific fields
// We'll store these in a separate table linked by user ID
type UserGitHub struct {
	database.Model
	UserID            string    `json:"user_id"`
	GitHubUsername    string    `json:"github_username"`
	GitHubID          int64     `json:"github_id"`
	GitHubAvatarURL   string    `json:"github_avatar_url"`
	GitHubConnectedAt time.Time `json:"github_connected_at"`
	LastSyncAt        time.Time `json:"last_sync_at"`
}

// Table returns the database table name
func (*UserGitHub) Table() string { return "user_github" }

// GetGitHubUser retrieves GitHub data for a user
func GetGitHubUser(userID string) (*UserGitHub, error) {
	results, err := GitHubUsers.Search("WHERE user_id = ?", userID)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, errors.New("github user not found")
	}
	return results[0], nil
}

// SetGitHubUser creates or updates GitHub data for a user
func SetGitHubUser(userID string, username string, githubID int64, avatarURL string) (*UserGitHub, error) {
	// Check if exists
	existing, err := GetGitHubUser(userID)
	if err == nil {
		// Update existing
		existing.GitHubUsername = username
		existing.GitHubID = githubID
		existing.GitHubAvatarURL = avatarURL
		existing.LastSyncAt = time.Now()
		err = GitHubUsers.Update(existing)
		return existing, err
	}
	
	// Create new
	githubUser := &UserGitHub{
		UserID:            userID,
		GitHubUsername:    username,
		GitHubID:          githubID,
		GitHubAvatarURL:   avatarURL,
		GitHubConnectedAt: time.Now(),
		LastSyncAt:        time.Now(),
	}
	
	return GitHubUsers.Insert(githubUser)
}

// DisconnectGitHub removes GitHub connection for a user
func DisconnectGitHub(userID string) error {
	githubUser, err := GetGitHubUser(userID)
	if err != nil {
		return err
	}
	return GitHubUsers.Delete(githubUser)
}

// IsGitHubConnected checks if a user has connected GitHub
func IsGitHubConnected(userID string) bool {
	_, err := GetGitHubUser(userID)
	return err == nil
}

// GetUserWithGitHub combines authentication.User with GitHub data
type UserWithGitHub struct {
	*authentication.User
	GitHubUsername    string    `json:"github_username,omitempty"`
	GitHubConnectedAt time.Time `json:"github_connected_at,omitempty"`
}

// GetUserWithGitHub retrieves a user with their GitHub data
func GetUserWithGitHub(userID string) (*UserWithGitHub, error) {
	user, err := Auth.Users.Get(userID)
	if err != nil {
		return nil, err
	}
	
	result := &UserWithGitHub{
		User: user,
	}
	
	// Try to get GitHub data
	githubUser, err := GetGitHubUser(userID)
	if err == nil {
		result.GitHubUsername = githubUser.GitHubUsername
		result.GitHubConnectedAt = githubUser.GitHubConnectedAt
	}
	
	return result, nil
}