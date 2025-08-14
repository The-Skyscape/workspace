package models

import (
	"encoding/json"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
)

// Settings represents global application settings
type Settings struct {
	application.Model
	
	// System Settings
	AppName             string `json:"app_name"`
	AppDescription      string `json:"app_description"`
	DefaultTheme        string `json:"default_theme"` // DaisyUI theme
	MaxRepoSize         int64  `json:"max_repo_size_mb"`
	MaxWorkspaces       int    `json:"max_workspaces_per_user"`
	
	// Security Settings
	AllowPublicRepos    bool   `json:"allow_public_repos"`
	AllowSignup         bool   `json:"allow_signup"`
	RequireEmailVerify  bool   `json:"require_email_verify"`
	SessionTimeout      int    `json:"session_timeout_hours"`
	
	// Performance Settings
	CacheTTLMinutes     int    `json:"cache_ttl_minutes"`
	MaxCacheSize        int64  `json:"max_cache_size_mb"`
	EnableGitCache      bool   `json:"enable_git_cache"`
	
	// Integration Settings
	GitHubEnabled       bool   `json:"github_enabled"`
	GitHubClientID      string `json:"github_client_id"`
	GitHubClientSecret  string `json:"github_client_secret"`
	
	// Metadata
	LastUpdatedBy       string    `json:"last_updated_by"`
	LastUpdatedAt       time.Time `json:"last_updated_at"`
}

// Table returns the database table name
func (*Settings) Table() string { return "settings" }

// GetSettings retrieves the global settings (creates default if not exists)
func GetSettings() (*Settings, error) {
	// Try to get existing settings
	settings, err := GlobalSettings.Get("global")
	if err != nil {
		// Create default settings
		settings = &Settings{
			Model:               DB.NewModel("global"),
			AppName:             "Skyscape",
			AppDescription:      "Self-hosted developer platform with Git repositories and cloud workspaces",
			DefaultTheme:        "corporate",
			MaxRepoSize:         1024, // 1GB default
			MaxWorkspaces:       5,
			AllowPublicRepos:    true,
			AllowSignup:         true,
			RequireEmailVerify:  false,
			SessionTimeout:      24, // 24 hours
			CacheTTLMinutes:     60,
			MaxCacheSize:        100,
			EnableGitCache:      true,
			GitHubEnabled:       false,
			LastUpdatedAt:       time.Now(),
		}
		
		// Insert default settings
		settings, err = GlobalSettings.Insert(settings)
		if err != nil {
			return nil, err
		}
	}
	
	return settings, nil
}

// UpdateSettings updates the global settings
func UpdateSettings(updates map[string]interface{}, updatedBy string) (*Settings, error) {
	settings, err := GetSettings()
	if err != nil {
		return nil, err
	}
	
	// Marshal updates to JSON and unmarshal into settings
	jsonData, err := json.Marshal(updates)
	if err != nil {
		return nil, err
	}
	
	if err := json.Unmarshal(jsonData, settings); err != nil {
		return nil, err
	}
	
	// Update metadata
	settings.LastUpdatedBy = updatedBy
	settings.LastUpdatedAt = time.Now()
	
	// Save to database
	err = GlobalSettings.Update(settings)
	if err != nil {
		return nil, err
	}
	
	return settings, nil
}

// HasGitHubIntegration checks if GitHub integration is configured
func (s *Settings) HasGitHubIntegration() bool {
	return s.GitHubEnabled && s.GitHubClientID != "" && s.GitHubClientSecret != ""
}

// SanitizedSettings returns settings with sensitive fields removed
func (s *Settings) SanitizedSettings() map[string]interface{} {
	return map[string]interface{}{
		"app_name":              s.AppName,
		"app_description":       s.AppDescription,
		"default_theme":         s.DefaultTheme,
		"max_repo_size_mb":      s.MaxRepoSize,
		"max_workspaces":        s.MaxWorkspaces,
		"allow_public_repos":    s.AllowPublicRepos,
		"allow_signup":          s.AllowSignup,
		"require_email_verify":  s.RequireEmailVerify,
		"session_timeout_hours": s.SessionTimeout,
		"cache_ttl_minutes":     s.CacheTTLMinutes,
		"max_cache_size_mb":     s.MaxCacheSize,
		"enable_git_cache":      s.EnableGitCache,
		"github_enabled":        s.GitHubEnabled,
		"has_github":            s.HasGitHubIntegration(),
		"last_updated_at":       s.LastUpdatedAt,
		"last_updated_by":       s.LastUpdatedBy,
	}
}