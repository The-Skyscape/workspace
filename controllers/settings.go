package controllers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"workspace/models"
)

type SettingsController struct {
	application.BaseController
	Request *http.Request
	App     *application.App
}

func Settings() (string, *SettingsController) {
	return "settings", &SettingsController{}
}

func (s *SettingsController) Setup(app *application.App) {
	s.App = app
	s.BaseController.Setup(app)
	auth := app.Use("auth").(*authentication.Controller)

	// Settings pages (admin only)
	http.Handle("GET /settings", app.Serve("settings.html", auth.Required))
	http.Handle("POST /settings", app.ProtectFunc(s.updateSettings, auth.Required))
	
	// Profile settings
	http.Handle("GET /settings/profile", app.Serve("settings-profile.html", auth.Required))
	http.Handle("POST /settings/profile", app.ProtectFunc(s.updateProfile, auth.Required))
}

func (s *SettingsController) Handle(req *http.Request) application.Controller {
	s.Request = req
	return s
}


// GetSettings returns the current global settings
func (s *SettingsController) GetSettings() (*models.Settings, error) {
	return models.GetSettings()
}

// GetSanitizedSettings returns settings safe for display
func (s *SettingsController) GetSanitizedSettings() map[string]interface{} {
	settings, err := models.GetSettings()
	if err != nil {
		return map[string]interface{}{}
	}
	return settings.SanitizedSettings()
}

// updateSettings handles the main settings form submission
func (s *SettingsController) updateSettings(w http.ResponseWriter, r *http.Request) {
	auth := s.App.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil || !user.IsAdmin {
		s.Render(w, r, "error-message.html", map[string]interface{}{
			"Message": "Unauthorized",
		})
		return
	}

	// Get current settings
	settings, err := models.GetSettings()
	if err != nil {
		s.Render(w, r, "error-message.html", map[string]interface{}{
			"Message": "Failed to load settings: " + err.Error(),
		})
		return
	}

	// Update from form values directly (HATEOAS pattern)
	settings.AppName = r.FormValue("app_name")
	settings.AppDescription = r.FormValue("app_description")
	settings.DefaultTheme = r.FormValue("default_theme")
	
	// Parse numeric values
	if size := r.FormValue("max_repo_size_mb"); size != "" {
		if val, err := strconv.ParseInt(size, 10, 64); err == nil {
			settings.MaxRepoSize = val
		}
	}
	if workspaces := r.FormValue("max_workspaces"); workspaces != "" {
		if val, err := strconv.Atoi(workspaces); err == nil {
			settings.MaxWorkspaces = val
		}
	}
	
	// Security Settings (checkboxes)
	settings.AllowPublicRepos = r.FormValue("allow_public_repos") == "true"
	settings.AllowSignup = r.FormValue("allow_signup") == "true"
	settings.RequireEmailVerify = r.FormValue("require_email_verify") == "true"
	
	if timeout := r.FormValue("session_timeout_hours"); timeout != "" {
		if val, err := strconv.Atoi(timeout); err == nil {
			settings.SessionTimeout = val
		}
	}

	// Performance Settings
	if ttl := r.FormValue("cache_ttl_minutes"); ttl != "" {
		if val, err := strconv.Atoi(ttl); err == nil {
			settings.CacheTTLMinutes = val
		}
	}
	if size := r.FormValue("max_cache_size_mb"); size != "" {
		if val, err := strconv.ParseInt(size, 10, 64); err == nil {
			settings.MaxCacheSize = val
		}
	}
	settings.EnableGitCache = r.FormValue("enable_git_cache") == "true"

	// GitHub Integration
	settings.GitHubEnabled = r.FormValue("github_enabled") == "true"
	settings.GitHubClientID = r.FormValue("github_client_id")
	settings.GitHubClientSecret = r.FormValue("github_client_secret")

	// Update metadata
	settings.LastUpdatedBy = user.Email
	settings.LastUpdatedAt = time.Now()

	// Save to database
	err = models.GlobalSettings.Update(settings)
	if err != nil {
		s.Render(w, r, "error-message.html", map[string]interface{}{
			"Message": "Failed to update settings: " + err.Error(),
		})
		return
	}

	// Log activity
	models.LogActivity("settings_updated", "Updated global settings", 
		"Administrator updated global settings", user.ID, "", "settings", "")

	// Redirect back to settings page
	s.Redirect(w, r, "/settings")
}


// GetProfile returns the admin user's profile for settings page
func (s *SettingsController) GetProfile() (*models.Profile, error) {
	return models.GetAdminProfile()
}

// updateProfile handles profile settings form submission
func (s *SettingsController) updateProfile(w http.ResponseWriter, r *http.Request) {
	auth := s.App.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil || !user.IsAdmin {
		s.Render(w, r, "error-message.html", map[string]interface{}{
			"Message": "Unauthorized",
		})
		return
	}

	// Parse form values
	updates := map[string]interface{}{
		"bio":        r.FormValue("bio"),
		"title":      r.FormValue("title"),
		"website":    r.FormValue("website"),
		"github":     r.FormValue("github"),
		"twitter":    r.FormValue("twitter"),
		"linkedin":   r.FormValue("linkedin"),
		"show_email": r.FormValue("show_email") == "true",
		"show_stats": r.FormValue("show_stats") == "true",
	}

	// Update profile
	_, err = models.UpdateAdminProfile(updates)
	if err != nil {
		s.Render(w, r, "error-message.html", map[string]interface{}{
			"Message": "Failed to update profile: " + err.Error(),
		})
		return
	}

	// Log activity
	models.LogActivity("profile_updated", "Updated public profile", 
		"Administrator updated public profile settings", user.ID, "", "profile", "")

	// Redirect back to profile settings page
	s.Redirect(w, r, "/settings/profile")
}