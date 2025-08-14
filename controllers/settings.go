package controllers

import (
	"cmp"
	"net/http"
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

	// Load and apply theme from settings
	settings, err := models.GetSettings()
	if err == nil && settings.DefaultTheme != "" {
		app.SetTheme(settings.DefaultTheme)
	}

	// Create admin-only access check that redirects to profile
	adminRequired := func(app *application.App, r *http.Request) string {
		user, _, err := auth.Authenticate(r)
		if err != nil {
			return "/signin"
		}
		if !user.IsAdmin {
			// Non-admins get redirected to profile page
			return "/settings/profile"
		}
		return ""
	}

	// Settings pages (admin only)
	http.Handle("GET /settings", app.Serve("settings.html", adminRequired))
	http.Handle("POST /settings", app.ProtectFunc(s.updateSettings, adminRequired))
	http.Handle("POST /settings/theme", app.ProtectFunc(s.updateTheme, adminRequired))
	
	// Profile settings - GET is for all authenticated users, POST is admin only
	http.Handle("GET /settings/profile", app.Serve("settings-profile.html", auth.Required))
	http.Handle("POST /settings/profile", app.ProtectFunc(s.updateProfile, adminRequired))
}

func (s *SettingsController) Handle(req *http.Request) application.Controller {
	s.Request = req
	return s
}


// GetSettings returns the current global settings
func (s *SettingsController) GetSettings() (*models.Settings, error) {
	return models.GetSettings()
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

	// Update only provided fields using cmp.Or to preserve existing values
	settings.AppName = cmp.Or(r.FormValue("app_name"), settings.AppName)
	settings.AppDescription = cmp.Or(r.FormValue("app_description"), settings.AppDescription)
	settings.DefaultTheme = cmp.Or(r.FormValue("default_theme"), settings.DefaultTheme)
	
	// Security Settings (checkboxes) - only update if field is present
	if _, exists := r.Form["allow_public_repos"]; exists {
		settings.AllowPublicRepos = r.FormValue("allow_public_repos") == "true"
	}
	if _, exists := r.Form["allow_signup"]; exists {
		settings.AllowSignup = r.FormValue("allow_signup") == "true"
	}
	if _, exists := r.Form["require_email_verify"]; exists {
		settings.RequireEmailVerify = r.FormValue("require_email_verify") == "true"
	}

	// GitHub Integration
	if _, exists := r.Form["github_enabled"]; exists {
		settings.GitHubEnabled = r.FormValue("github_enabled") == "true"
	}
	settings.GitHubClientID = cmp.Or(r.FormValue("github_client_id"), settings.GitHubClientID)
	settings.GitHubClientSecret = cmp.Or(r.FormValue("github_client_secret"), settings.GitHubClientSecret)

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

	// Update App.Theme if theme was changed
	if r.Form.Has("default_theme") {
		s.App.SetTheme(settings.DefaultTheme)
	}

	// Log activity
	models.LogActivity("settings_updated", "Updated global settings", 
		"Administrator updated global settings", user.ID, "", "settings", "")

	// Redirect back to settings page
	s.Redirect(w, r, "/settings")
}


// updateTheme handles theme change requests
func (s *SettingsController) updateTheme(w http.ResponseWriter, r *http.Request) {
	auth := s.App.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil || !user.IsAdmin {
		s.Render(w, r, "error-message.html", map[string]interface{}{
			"Message": "Unauthorized",
		})
		return
	}

	theme := r.URL.Query().Get("theme")
	if theme == "" {
		s.Render(w, r, "error-message.html", map[string]interface{}{
			"Message": "Theme not specified",
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

	// Update theme
	settings.DefaultTheme = theme
	settings.LastUpdatedBy = user.Email
	settings.LastUpdatedAt = time.Now()

	// Save to database
	err = models.GlobalSettings.Update(settings)
	if err != nil {
		s.Render(w, r, "error-message.html", map[string]interface{}{
			"Message": "Failed to update theme: " + err.Error(),
		})
		return
	}

	// Update App theme
	s.App.SetTheme(theme)

	// Log activity
	models.LogActivity("theme_updated", "Updated UI theme to " + theme, 
		"Administrator changed theme to " + theme, user.ID, "", "settings", "")

	// Return success (HTMX will reload the page)
	w.WriteHeader(http.StatusOK)
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

	// Get existing profile
	profile, err := models.GetAdminProfile()
	if err != nil {
		s.Render(w, r, "error-message.html", map[string]interface{}{
			"Message": "Failed to load profile: " + err.Error(),
		})
		return
	}

	// Update only the fields that are present in the form
	if r.Form.Has("bio") {
		profile.Bio = r.FormValue("bio")
	}
	if r.Form.Has("title") {
		profile.Title = r.FormValue("title")
	}
	if r.Form.Has("website") {
		profile.Website = r.FormValue("website")
	}
	if r.Form.Has("github") {
		profile.GitHub = r.FormValue("github")
	}
	if r.Form.Has("twitter") {
		profile.Twitter = r.FormValue("twitter")
	}
	if r.Form.Has("linkedin") {
		profile.LinkedIn = r.FormValue("linkedin")
	}
	
	// Checkboxes - only update if field is present
	if r.Form.Has("show_email") {
		profile.ShowEmail = r.FormValue("show_email") == "true"
	}
	if r.Form.Has("show_stats") {
		profile.ShowStats = r.FormValue("show_stats") == "true"
	}

	// Save profile
	err = models.Profiles.Update(profile)
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