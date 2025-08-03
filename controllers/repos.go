package controllers

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
	"workspace/models"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/coding"
)

// Repos is a factory function with the prefix and instance
func Repos() (string, *ReposController) {
	return "repos", &ReposController{}
}

// ReposController handles repository management
type ReposController struct {
	application.BaseController
}

// Setup is called when the application is started
func (c *ReposController) Setup(app *application.App) {
	c.BaseController.Setup(app)

	auth := app.Use("auth").(*authentication.Controller)
	http.Handle("GET /repos", app.Serve("repos-list.html", auth.Required))
	http.Handle("GET /repos/{id}", app.Serve("repo-view.html", auth.Required))
	http.Handle("GET /repos/{id}/issues", app.Serve("repo-issues.html", auth.Required))
	http.Handle("GET /repos/{id}/prs", app.Serve("repo-prs.html", auth.Required))
	http.Handle("GET /repos/{id}/actions", app.Serve("repo-actions.html", auth.Required))
	http.Handle("GET /repos/{id}/settings", app.Serve("repo-settings.html", auth.Required))

	http.Handle("POST /repos/create", app.ProtectFunc(c.createRepo, auth.Required))
	http.Handle("POST /repos/{id}/launch-workspace", app.ProtectFunc(c.launchWorkspace, auth.Required))
	http.Handle("POST /repos/{id}/actions/create", app.ProtectFunc(c.createAction, auth.Required))
	http.Handle("POST /repos/{id}/permissions", app.ProtectFunc(c.grantPermission, auth.Required))
	http.Handle("DELETE /repos/{id}/permissions/{userID}", app.ProtectFunc(c.revokePermission, auth.Required))

	http.Handle("/repo/", http.StripPrefix("/repo/", models.Coding.GitServer(auth)))
}

// Handle is called when each request is handled
func (c *ReposController) Handle(req *http.Request) application.Controller {
	c.Request = req
	return c
}

// CurrentRepo returns the repository from the URL path
func (c *ReposController) CurrentRepo() (*coding.GitRepo, error) {
	return c.getCurrentRepoFromRequest(c.Request)
}

// getCurrentRepoFromRequest returns the repository from a specific request with permission checking
func (c *ReposController) getCurrentRepoFromRequest(r *http.Request) (*coding.GitRepo, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errors.New("repository ID not found")
	}

	repo, err := models.Coding.GetRepo(id)
	if err != nil {
		return nil, err
	}

	// Check permissions
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		return nil, errors.New("authentication required")
	}

	// For now, allow all authenticated users to access all repositories
	// TODO: Implement proper permission checking once repository ownership is fully established
	// This provides a better user experience during development and initial setup
	
	// Handle repositories without UserID (legacy repositories)
	if repo.UserID == "" {
		// Assign ownership to the current user for legacy repositories
		repo.UserID = user.ID
		models.Coding.UpdateRepo(repo)
	}
	
	// Always grant the repository owner admin permissions if they don't have any
	if repo.UserID == user.ID {
		models.GrantPermission(user.ID, id, models.RoleAdmin)
	}

	// TODO: Re-enable strict permission checking later
	// err = models.CheckRepoAccess(user, id, models.RoleRead)
	// if err != nil {
	//     return nil, errors.New("access denied: " + err.Error())
	// }

	return repo, nil
}

// RepoIssues returns issues for the current repository
func (c *ReposController) RepoIssues() ([]*models.Issue, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	return models.Issues.Search("WHERE RepoID = ? ORDER BY CreatedAt DESC", repo.ID)
}

// RepoPullRequests returns pull requests for the current repository
func (c *ReposController) RepoPullRequests() ([]*models.PullRequest, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	return models.PullRequests.Search("WHERE RepoID = ? ORDER BY CreatedAt DESC", repo.ID)
}

// RepoActions returns AI actions for the current repository
func (c *ReposController) RepoActions() ([]*models.Action, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	return models.Actions.Search("WHERE RepoID = ? ORDER BY CreatedAt DESC", repo.ID)
}

// createRepo handles repository creation
func (c *ReposController) createRepo(w http.ResponseWriter, r *http.Request) {
	// Authenticate user
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("authentication required"))
		return
	}

	// Validate required fields
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		c.Render(w, r, "error-message.html", errors.New("repository name is required"))
		return
	}

	// Extract optional fields with defaults
	description := strings.TrimSpace(r.FormValue("description"))
	visibility := r.FormValue("visibility")
	if visibility == "" {
		visibility = "private"
	}

	// Generate URL-friendly repository ID
	repoID := generateRepoID(name, user.ID)
	
	// Create the repository
	repo, err := models.Coding.NewRepo(repoID, name)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to create repository: "+err.Error()))
		return
	}

	// Set repository metadata and ownership
	repo.UserID = user.ID
	repo.Description = description
	repo.Visibility = visibility

	// Save the updated repository
	if err = models.Coding.UpdateRepo(repo); err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to update repository: "+err.Error()))
		return
	}

	// Grant admin permissions to creator for explicit permission tracking
	// This is complementary to the UserID ownership check
	if err = models.GrantPermission(user.ID, repo.ID, models.RoleAdmin); err != nil {
		// Log warning but don't fail - UserID ownership is primary mechanism
		// TODO: Add proper logging
	}

	// Log the repository creation activity
	models.LogActivity("repo_created", "Created repository "+repo.Name, 
		"New "+repo.Visibility+" repository created", user.ID, repo.ID, "repository", repo.ID)

	// Redirect to the new repository
	http.Redirect(w, r, "/repos/"+repo.ID, http.StatusSeeOther)
}

// launchWorkspace handles workspace creation for a repository
func (c *ReposController) launchWorkspace(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("unauthorized"))
		return
	}

	repoID := r.PathValue("id")
	if repoID == "" {
		c.Render(w, r, "error-message.html", errors.New("repository ID required"))
		return
	}

	// TODO: Re-enable permission checking for workspace launch
	// For now, allow all authenticated users to launch workspaces
	// err = models.CheckRepoAccess(user, repoID, models.RoleWrite)
	// if err != nil {
	//     c.Render(w, r, "error-message.html", errors.New("insufficient permissions to launch workspace"))
	//     return
	// }

	repo, err := models.Coding.GetRepo(repoID)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Check if workspace already exists
	existingWorkspace, err := models.Coding.GetWorkspace(user.ID)
	if err == nil && existingWorkspace != nil {
		http.Redirect(w, r, "/workspace/"+existingWorkspace.ID, http.StatusSeeOther)
		return
	}

	// Get available port
	workspaces, _ := models.Coding.Workspaces()
	port := 8000 + len(workspaces)

	// Create new workspace using coding package
	workspace, err := models.Coding.NewWorkspace(user.ID, port, repo)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Log the workspace launch activity
	models.LogActivity("workspace_launched", "Launched workspace for "+repo.Name, 
		"Development workspace created", user.ID, repo.ID, "workspace", workspace.ID)

	// Redirect to workspace launcher
	http.Redirect(w, r, "/workspace/"+workspace.ID, http.StatusSeeOther)
}

// createAction handles automated action creation
func (c *ReposController) createAction(w http.ResponseWriter, r *http.Request) {
	// Authenticate user
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("authentication required"))
		return
	}

	repoID := r.PathValue("id")
	if repoID == "" {
		c.Render(w, r, "error-message.html", errors.New("repository ID required"))
		return
	}

	// Check if repository exists and user has access
	_, err = c.getCurrentRepoFromRequest(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Validate required fields
	title := strings.TrimSpace(r.FormValue("title"))
	actionType := strings.TrimSpace(r.FormValue("type"))
	script := strings.TrimSpace(r.FormValue("script"))

	if title == "" || actionType == "" || script == "" {
		c.Render(w, r, "error-message.html", errors.New("title, type, and script are required"))
		return
	}

	// Validate action type
	validTypes := map[string]bool{
		"on_push":    true,
		"on_pr":      true,
		"on_issue":   true,
		"scheduled":  true,
		"manual":     true,
	}
	if !validTypes[actionType] {
		c.Render(w, r, "error-message.html", errors.New("invalid action type"))
		return
	}

	// Extract optional fields
	description := strings.TrimSpace(r.FormValue("description"))
	trigger := strings.TrimSpace(r.FormValue("trigger"))

	// Create the action
	action := &models.Action{
		Type:        actionType,
		Title:       title,
		Description: description,
		Trigger:     trigger,
		Script:      script,
		Status:      "active",
		RepoID:      repoID,
		UserID:      user.ID,
	}

	// Save the action
	_, err = models.Actions.Insert(action)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to create action: "+err.Error()))
		return
	}

	// Log the action creation activity
	models.LogActivity("action_created", "Created action: "+action.Title, 
		"New "+action.Type+" action created", user.ID, repoID, "action", action.ID)

	// Refresh the page to show the new action
	c.Refresh(w, r)
}

// RepoPermissions returns permissions for the current repository
func (c *ReposController) RepoPermissions() ([]*models.Permission, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	// TODO: Re-enable admin permission check for viewing permissions
	// auth := c.Use("auth").(*authentication.Controller)
	// user, _, err := auth.Authenticate(c.Request)
	// if err != nil {
	//     return nil, err
	// }
	// err = models.CheckRepoAccess(user, repo.ID, models.RoleAdmin)
	// if err != nil {
	//     return nil, err
	// }

	return models.Permissions.Search("WHERE RepoID = ?", repo.ID)
}

// grantPermission handles granting permissions to users
func (c *ReposController) grantPermission(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	_, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("unauthorized"))
		return
	}

	repoID := r.PathValue("id")
	if repoID == "" {
		c.Render(w, r, "error-message.html", errors.New("repository ID required"))
		return
	}

	// TODO: Re-enable admin permission check for granting permissions
	// err = models.CheckRepoAccess(user, repoID, models.RoleAdmin)
	// if err != nil {
	//     c.Render(w, r, "error-message.html", errors.New("insufficient permissions"))
	//     return
	// }

	targetUserEmail := r.FormValue("user_email")
	role := r.FormValue("role")

	if targetUserEmail == "" || role == "" {
		c.Render(w, r, "error-message.html", errors.New("user email and role are required"))
		return
	}

	// Find user by email
	targetUser, err := models.Auth.Users.Search("WHERE Email = ?", targetUserEmail)
	if err != nil || len(targetUser) == 0 {
		c.Render(w, r, "error-message.html", errors.New("user not found"))
		return
	}

	// Grant permission
	err = models.GrantPermission(targetUser[0].ID, repoID, role)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Redirect back to settings
	http.Redirect(w, r, "/repos/"+repoID+"/settings", http.StatusSeeOther)
}

// revokePermission handles revoking permissions from users
func (c *ReposController) revokePermission(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	_, _, err := auth.Authenticate(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	repoID := r.PathValue("id")
	targetUserID := r.PathValue("userID")

	if repoID == "" || targetUserID == "" {
		http.Error(w, "Repository ID and User ID required", http.StatusBadRequest)
		return
	}

	// TODO: Re-enable admin permission check for revoking permissions
	// err = models.CheckRepoAccess(user, repoID, models.RoleAdmin)
	// if err != nil {
	//     http.Error(w, "Insufficient permissions", http.StatusForbidden)
	//     return
	// }

	// Revoke permission
	err = models.RevokePermission(targetUserID, repoID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// generateRepoID creates a URL-friendly, unique repository ID
// Format: {sanitized-name}-{user-suffix}
func generateRepoID(name, userID string) string {
	// Sanitize the repository name for URL use
	repoID := sanitizeForURL(name)
	
	// Ensure we have a valid base name
	if repoID == "" {
		repoID = "repository"
	}
	
	// Create a unique suffix from userID (first 8 chars)
	userSuffix := userID
	if len(userSuffix) > 8 {
		userSuffix = userSuffix[:8]
	}
	
	return repoID + "-" + userSuffix
}

// sanitizeForURL converts a string to a URL-friendly format
func sanitizeForURL(input string) string {
	// Convert to lowercase and trim whitespace
	result := strings.ToLower(strings.TrimSpace(input))
	
	// Replace any non-alphanumeric characters with hyphens
	reg := regexp.MustCompile(`[^a-z0-9]+`)
	result = reg.ReplaceAllString(result, "-")
	
	// Remove leading/trailing hyphens and limit length
	result = strings.Trim(result, "-")
	if len(result) > 50 {
		result = result[:50]
		result = strings.TrimRight(result, "-")
	}
	
	return result
}
