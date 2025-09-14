package controllers

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"workspace/models"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/database"
)

type SettingsController struct {
	application.Controller
}

func Settings() (string, *SettingsController) {
	return "settings", &SettingsController{}
}

func (s *SettingsController) Setup(app *application.App) {
	s.Controller.Setup(app)
	auth := app.Use("auth").(*AuthController)

	// Load and apply theme from settings
	settings, err := models.GetSettings()
	if err == nil && settings.DefaultTheme != "" {
		app.SetTheme(settings.DefaultTheme)
	}

	// Create admin-only access check that redirects to profile
	adminRequired := func(app *application.App, w http.ResponseWriter, r *http.Request) bool {
		user, _, err := auth.Authenticate(r)
		if err != nil {
			// Redirect to signin if not authenticated
			s.Redirect(w, r, "/signin")
			return false
		}
		if !user.IsAdmin {
			// Non-admins get redirected to account page
			s.Redirect(w, r, "/settings/account")
			return false
		}
		return true
	}

	// Settings pages (admin only)
	http.Handle("GET /settings", app.Serve("settings.html", adminRequired))
	http.Handle("POST /settings", app.ProtectFunc(s.updateSettings, adminRequired))
	http.Handle("POST /settings/theme", app.ProtectFunc(s.updateTheme, adminRequired))
	// GitHub settings moved to IntegrationsController

	// User Account settings - for individual users
	http.Handle("GET /settings/account", app.Serve("settings-account.html", auth.Required))
	http.Handle("POST /settings/account", app.ProtectFunc(s.updateAccount, auth.Required))
	http.Handle("POST /settings/account/password", app.ProtectFunc(s.updatePassword, auth.Required))
	http.Handle("POST /settings/account/avatar", app.ProtectFunc(s.uploadAvatar, auth.Required))

	// SSH Key management (admin only for now)
	http.Handle("GET /settings/ssh-keys", app.Serve("settings-ssh-keys.html", adminRequired))
	http.Handle("POST /settings/ssh-keys", app.ProtectFunc(s.addSSHKey, adminRequired))
	http.Handle("DELETE /settings/ssh-keys/{id}", app.ProtectFunc(s.deleteSSHKey, adminRequired))

	// Serve avatar images
	http.HandleFunc("GET /avatar/{filename}", s.serveAvatar)

	// Workspace Profile settings - GET is for all authenticated users, POST is admin only
	http.Handle("GET /settings/workspace", app.Serve("settings-workspace.html", auth.Required))
	http.Handle("POST /settings/workspace", app.ProtectFunc(s.updateWorkspace, adminRequired))
}

func (s SettingsController) Handle(req *http.Request) application.Handler {
	s.Request = req
	return &s
}

// GetSettings returns the current global settings
func (s *SettingsController) GetSettings() (*models.Settings, error) {
	return models.GetSettings()
}

// GitHub OAuth methods moved to IntegrationsController

// updateSettings handles the main settings form submission
func (s *SettingsController) updateSettings(w http.ResponseWriter, r *http.Request) {
	s.SetRequest(r)
	// Access already checked by route middleware (adminRequired)
	auth := s.App.Use("auth").(*AuthController)
	user := auth.CurrentUser()

	// Get current settings
	settings, err := models.GetSettings()
	if err != nil {
		s.RenderError(w, r, err)
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

	// Update metadata
	settings.LastUpdatedBy = user.Email
	settings.LastUpdatedAt = time.Now()

	// Save to database
	err = models.GlobalSettings.Update(settings)
	if err != nil {
		s.RenderError(w, r, err)
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
	s.SetRequest(r)
	// Access already checked by route middleware (adminRequired)
	auth := s.App.Use("auth").(*AuthController)
	user := auth.CurrentUser()

	theme := r.URL.Query().Get("theme")
	if theme == "" {
		s.RenderError(w, r, errors.New("theme not specified"))
		return
	}

	// Get current settings
	settings, err := models.GetSettings()
	if err != nil {
		s.RenderError(w, r, err)
		return
	}

	// Update theme
	settings.DefaultTheme = theme
	settings.LastUpdatedBy = user.Email
	settings.LastUpdatedAt = time.Now()

	// Save to database
	err = models.GlobalSettings.Update(settings)
	if err != nil {
		s.RenderError(w, r, err)
		return
	}

	// Update App theme
	s.App.SetTheme(theme)

	// Log activity
	models.LogActivity("theme_updated", "Updated UI theme to "+theme,
		"Administrator changed theme to "+theme, user.ID, "", "settings", "")

	// Refresh the page to apply the new theme
	s.Refresh(w, r)
}

// GitHub settings methods moved to IntegrationsController

// GetProfile returns the admin user's profile for settings page
func (s *SettingsController) GetProfile() (*models.Profile, error) {
	return models.GetAdminProfile()
}

// GetWorkspace returns the workspace profile for settings page
func (s *SettingsController) GetWorkspace() (*models.Profile, error) {
	return models.GetAdminProfile()
}

// updateAccount handles user account settings form submission
func (s *SettingsController) updateAccount(w http.ResponseWriter, r *http.Request) {
	s.SetRequest(r)
	// Access already checked by route middleware (auth.Required)
	auth := s.App.Use("auth").(*AuthController)
	user := auth.CurrentUser()

	// Parse form to get values
	if err := r.ParseForm(); err != nil {
		s.RenderError(w, r, err)
		return
	}

	// Update user fields that are present
	if r.Form.Has("name") {
		user.Name = r.FormValue("name")
	}
	if r.Form.Has("email") {
		user.Email = r.FormValue("email")
	}
	if r.Form.Has("handle") {
		user.Handle = r.FormValue("handle")
	}
	if r.Form.Has("avatar") {
		user.Avatar = r.FormValue("avatar")
	}

	// Save user updates
	err := auth.Users.Update(user)
	if err != nil {
		s.RenderError(w, r, err)
		return
	}

	// Log activity
	models.LogActivity("account_updated", "Updated account settings",
		"User updated their account settings", user.ID, "", "account", "")

	// Refresh to show updated values
	s.Refresh(w, r)
}

// updatePassword handles password change requests
func (s *SettingsController) updatePassword(w http.ResponseWriter, r *http.Request) {
	s.SetRequest(r)
	// Access already checked by route middleware (auth.Required)
	auth := s.App.Use("auth").(*AuthController)
	user := auth.CurrentUser()

	currentPassword := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")
	confirmPassword := r.FormValue("confirm_password")

	// Validate passwords match
	if newPassword != confirmPassword {
		s.RenderError(w, r, errors.New("passwords do not match"))
		return
	}

	// Verify current password
	if !user.VerifyPassword(currentPassword) {
		s.RenderError(w, r, errors.New("current password is incorrect"))
		return
	}

	// Update password
	err := user.SetupPassword(newPassword)
	if err != nil {
		s.RenderError(w, r, err)
		return
	}

	// Save user
	if err := auth.Users.Update(user); err != nil {
		s.RenderError(w, r, err)
		return
	}

	// Log activity
	models.LogActivity("password_changed", "Changed password",
		"User changed their password", user.ID, "", "account", "")

	// Redirect to account page with success message
	s.Redirect(w, r, "/settings/account")
}

// uploadAvatar handles avatar image upload
func (s *SettingsController) uploadAvatar(w http.ResponseWriter, r *http.Request) {
	s.SetRequest(r)
	// Access already checked by route middleware (auth.Required)
	auth := s.App.Use("auth").(*AuthController)
	user := auth.CurrentUser()

	// Parse multipart form (max 10MB)
	err := r.ParseMultipartForm(10 << 20)
	if err != nil {
		s.RenderError(w, r, errors.New("failed to parse upload"))
		return
	}

	// Get the uploaded file
	file, handler, err := r.FormFile("avatar")
	if err != nil {
		s.RenderError(w, r, errors.New("no file uploaded"))
		return
	}
	defer file.Close()

	// Validate file type
	contentType := handler.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		s.RenderError(w, r, errors.New("file must be an image"))
		return
	}

	// Create avatars directory
	avatarDir := filepath.Join(database.DataDir(), "avatars")
	os.MkdirAll(avatarDir, 0755)

	// Generate filename based on user ID and file extension
	ext := filepath.Ext(handler.Filename)
	if ext == "" {
		// Default to .jpg if no extension
		ext = ".jpg"
	}
	filename := fmt.Sprintf("%s%s", user.ID, ext)
	filePath := filepath.Join(avatarDir, filename)

	// Create the destination file
	dst, err := os.Create(filePath)
	if err != nil {
		s.RenderError(w, r, errors.New("failed to save avatar"))
		return
	}
	defer dst.Close()

	// Copy the uploaded file to the destination
	_, err = io.Copy(dst, file)
	if err != nil {
		s.RenderError(w, r, errors.New("failed to save avatar"))
		return
	}

	// Update user's avatar URL to internal path
	user.Avatar = fmt.Sprintf("/avatar/%s", filename)
	err = auth.Users.Update(user)
	if err != nil {
		s.RenderError(w, r, errors.New("failed to update profile"))
		return
	}

	// Log activity
	models.LogActivity("avatar_uploaded", "Uploaded new avatar",
		"User uploaded a new avatar image", user.ID, "", "account", "")

	// Redirect back to account page
	s.Redirect(w, r, "/settings/account")
}

// serveAvatar serves uploaded avatar images
func (s *SettingsController) serveAvatar(w http.ResponseWriter, r *http.Request) {
	s.SetRequest(r)
	filename := r.PathValue("filename")
	if filename == "" {
		http.NotFound(w, r)
		return
	}

	// Build the full path to the avatar file
	avatarPath := filepath.Join(database.DataDir(), "avatars", filename)

	// Check if file exists
	if _, err := os.Stat(avatarPath); os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	}

	// Serve the file
	http.ServeFile(w, r, avatarPath)
}

// updateWorkspace handles workspace profile settings form submission
func (s *SettingsController) updateWorkspace(w http.ResponseWriter, r *http.Request) {
	s.SetRequest(r)
	// Access already checked by route middleware (adminRequired)
	auth := s.App.Use("auth").(*AuthController)
	user := auth.CurrentUser()

	// Parse the form to get all values
	if err := r.ParseForm(); err != nil {
		s.RenderError(w, r, err)
		return
	}

	// Get existing profile
	profile, err := models.GetAdminProfile()
	if err != nil {
		s.RenderError(w, r, err)
		return
	}

	// Update only the fields that are present in the form
	// Allow clearing fields by sending empty values
	if r.Form.Has("name") {
		profile.Name = r.FormValue("name")
	}
	if r.Form.Has("description") {
		profile.Description = r.FormValue("description")
	}
	if r.Form.Has("bio") {
		profile.Bio = r.FormValue("bio")
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

	// Checkboxes need special handling - empty checkbox means false
	if r.Form.Has("show_email") {
		profile.ShowEmail = r.FormValue("show_email") == "true"
	}
	if r.Form.Has("show_stats") {
		profile.ShowStats = r.FormValue("show_stats") == "true"
	}

	// Save profile
	err = models.Profiles.Update(profile)
	if err != nil {
		s.RenderError(w, r, err)
		return
	}

	// Log activity
	models.LogActivity("workspace_updated", "Updated workspace profile",
		"Administrator updated workspace profile settings", user.ID, "", "workspace", "")

	// Refresh to show updated values
	s.Refresh(w, r)
}

// GetSSHKeys returns all SSH keys for the current user
func (s *SettingsController) GetSSHKeys() ([]*models.SSHKey, error) {
	auth := s.App.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(s.Request)
	if err != nil {
		return nil, err
	}

	return models.GetUserSSHKeys(user.ID)
}

// addSSHKey handles adding a new SSH key
func (s *SettingsController) addSSHKey(w http.ResponseWriter, r *http.Request) {
	s.SetRequest(r)
	// Access already checked by route middleware (adminRequired)
	auth := s.App.Use("auth").(*AuthController)
	user := auth.CurrentUser()

	// Parse form
	name := strings.TrimSpace(r.FormValue("name"))
	publicKey := strings.TrimSpace(r.FormValue("public_key"))

	if publicKey == "" {
		s.RenderError(w, r, errors.New("SSH key is required"))
		return
	}

	// Check if user has reached the limit (10 keys)
	count, _ := models.CountUserSSHKeys(user.ID)
	if count >= 10 {
		s.RenderError(w, r, errors.New("Maximum of 10 SSH keys allowed per user"))
		return
	}

	// Create the SSH key
	sshKey, err := models.CreateSSHKey(user.ID, name, publicKey)
	if err != nil {
		s.RenderError(w, r, fmt.Errorf("Failed to add SSH key: %v", err))
		return
	}

	// Log activity
	models.LogActivity("ssh_key_added", fmt.Sprintf("Added SSH key: %s", sshKey.Name),
		fmt.Sprintf("User added SSH key with fingerprint %s", sshKey.Fingerprint),
		user.ID, "", "ssh_key", sshKey.ID)

	// Redirect back to SSH keys page
	s.Redirect(w, r, "/settings/ssh-keys")
}

// deleteSSHKey handles removing an SSH key
func (s *SettingsController) deleteSSHKey(w http.ResponseWriter, r *http.Request) {
	s.SetRequest(r)
	// Access already checked by route middleware (adminRequired)
	auth := s.App.Use("auth").(*AuthController)
	user := auth.CurrentUser()

	keyID := r.PathValue("id")
	if keyID == "" {
		s.RenderError(w, r, errors.New("key ID required"))
		return
	}

	// Delete the key (function verifies ownership)
	err := models.DeleteSSHKey(keyID, user.ID)
	if err != nil {
		s.RenderError(w, r, fmt.Errorf("Failed to delete SSH key: %v", err))
		return
	}

	// Log activity
	models.LogActivity("ssh_key_deleted", fmt.Sprintf("Deleted SSH key ID: %s", keyID),
		"User deleted an SSH key", user.ID, "", "ssh_key", keyID)

	// Refresh the page to update the SSH key list
	s.Refresh(w, r)
}
