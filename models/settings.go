package models

import (
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
)

// Settings represents global application settings
type Settings struct {
	application.Model
	
	// System Settings
	AppName             string
	AppDescription      string
	DefaultTheme        string // DaisyUI theme
	MaxRepoSize         int64
	MaxWorkspaces       int
	
	// Security Settings
	AllowPublicRepos    bool
	AllowSignup         bool
	RequireEmailVerify  bool
	SessionTimeout      int
	
	// Performance Settings
	CacheTTLMinutes     int
	MaxCacheSize        int64
	EnableGitCache      bool
	
	// Integration Settings
	GitHubEnabled       bool
	GitHubClientID      string
	GitHubClientSecret  string
	
	// Metadata
	LastUpdatedBy       string
	LastUpdatedAt       time.Time
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

// HasGitHubIntegration checks if GitHub integration is configured
func (s *Settings) HasGitHubIntegration() bool {
	return s.GitHubEnabled && s.GitHubClientID != "" && s.GitHubClientSecret != ""
}