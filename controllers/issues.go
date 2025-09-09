package controllers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"workspace/internal/ai"
	"workspace/models"
	"workspace/services"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
)

// Issues controller prefix
func Issues() (string, *IssuesController) {
	return "issues", &IssuesController{}
}

// IssuesController handles issue-related operations
type IssuesController struct {
	application.BaseController
}

// Handle returns a new controller instance for the request
func (c IssuesController) Handle(req *http.Request) application.Controller {
	c.Request = req
	return &c
}

// Setup registers routes
func (c *IssuesController) Setup(app *application.App) {
	c.BaseController.Setup(app)
	auth := app.Use("auth").(*AuthController)

	// Issues - view on public repos or as admin
	http.Handle("GET /repos/{id}/issues", app.Serve("repo-issues.html", PublicOrAdmin()))
	http.Handle("GET /repos/{id}/issues/kanban", app.Serve("repo-issues-kanban.html", PublicOrAdmin()))
	http.Handle("GET /repos/{id}/issues/search", app.ProtectFunc(c.searchIssues, PublicOrAdmin()))
	http.Handle("GET /repos/{id}/issues/more", app.Serve("issues-more.html", PublicOrAdmin()))
	http.Handle("GET /repos/{id}/issues/{issueID}", app.Serve("repo-issue-view.html", PublicOrAdmin()))

	// Issue operations - authenticated users on public repos, admins on any
	http.Handle("POST /repos/{id}/issues/create", app.ProtectFunc(c.createIssue, PublicRepoOnly()))
	http.Handle("POST /repos/{id}/issues/{issueID}/comment", app.ProtectFunc(c.createIssueComment, PublicRepoOnly()))

	// Issue modifications - author or admin only
	http.Handle("POST /repos/{id}/issues/{issueID}/close", app.ProtectFunc(c.closeIssue, auth.Required))
	http.Handle("POST /repos/{id}/issues/{issueID}/reopen", app.ProtectFunc(c.reopenIssue, auth.Required))
	http.Handle("POST /repos/{id}/issues/{issueID}/edit", app.ProtectFunc(c.editIssue, auth.Required))
	http.Handle("POST /repos/{id}/issues/{issueID}/move", app.ProtectFunc(c.moveIssue, auth.Required))

	// Issue deletion - admin only
	http.Handle("POST /repos/{id}/issues/{issueID}/delete", app.ProtectFunc(c.deleteIssue, AdminOnly()))
}

// CurrentRepo returns the current repository from the request
func (c *IssuesController) CurrentRepo() (*models.Repository, error) {
	reposController := c.Use("repos").(*ReposController)
	return reposController.CurrentRepo()
}

// RepoIssues returns issues for the current repository
func (c *IssuesController) RepoIssues() ([]*models.Issue, error) {
	reposController := c.Use("repos").(*ReposController)
	repo, err := reposController.CurrentRepo()
	if err != nil {
		return nil, err
	}

	// Get search query from request
	searchQuery := c.Request.URL.Query().Get("search")
	includeClosed := c.Request.URL.Query().Get("includeClosed") == "true"

	// Build search condition
	condition := "WHERE RepoID = ?"
	args := []interface{}{repo.ID}

	// Add status filter
	if !includeClosed {
		condition += " AND Status = ?"
		args = append(args, "open")
	}

	// Add search filter if provided
	if searchQuery != "" {
		condition += " AND (Title LIKE ? OR Body LIKE ?)"
		searchPattern := "%" + searchQuery + "%"
		args = append(args, searchPattern, searchPattern)
	}

	// Add ordering and limit for initial load
	condition += " ORDER BY CreatedAt DESC LIMIT 20"

	// Search issues
	issues, err := models.Issues.Search(condition, args...)
	if err != nil {
		return nil, err
	}

	return issues, nil
}

// MoreIssues returns the next page of issues for infinite scroll
func (c *IssuesController) MoreIssues() ([]*models.Issue, error) {
	reposController := c.Use("repos").(*ReposController)
	repo, err := reposController.CurrentRepo()
	if err != nil {
		return nil, err
	}

	// Parse offset from query params
	offsetStr := c.Request.URL.Query().Get("offset")
	offset := 0
	if offsetStr != "" {
		fmt.Sscanf(offsetStr, "%d", &offset)
	}

	// Get filter options
	includeClosed := c.Request.URL.Query().Get("includeClosed") == "true"

	// Get next batch of issues
	issues, _, err := models.GetRepoIssuesPaginated(repo.ID, includeClosed, 20, offset)
	return issues, err
}

// HasMoreIssues checks if there are more issues to load
func (c *IssuesController) HasMoreIssues() bool {
	reposController := c.Use("repos").(*ReposController)
	repo, err := reposController.CurrentRepo()
	if err != nil {
		return false
	}

	offsetStr := c.Request.URL.Query().Get("offset")
	offset := 0
	if offsetStr != "" {
		fmt.Sscanf(offsetStr, "%d", &offset)
	}

	includeClosed := c.Request.URL.Query().Get("includeClosed") == "true"
	issues, total, err := models.GetRepoIssuesPaginated(repo.ID, includeClosed, 20, offset)
	if err != nil {
		return false
	}

	return (offset + len(issues)) < total
}

// NextIssuesOffset returns the offset for the next page of issues
func (c *IssuesController) NextIssuesOffset() int {
	offsetStr := c.Request.URL.Query().Get("offset")
	offset := 0
	if offsetStr != "" {
		fmt.Sscanf(offsetStr, "%d", &offset)
	}
	return offset + 20
}

// SearchQuery returns the current search query for issues
func (c *IssuesController) SearchQuery() string {
	return c.Request.URL.Query().Get("search")
}

// IncludeClosed returns whether to include closed issues
func (c *IssuesController) IncludeClosed() bool {
	return c.Request.URL.Query().Get("includeClosed") == "true"
}

// CurrentIssue returns the issue from the request
func (c *IssuesController) CurrentIssue() (*models.Issue, error) {
	issueID := c.Request.PathValue("issueID")
	if issueID == "" {
		return nil, errors.New("issue ID required")
	}
	return models.Issues.Get(issueID)
}

// CurrentUser returns the currently authenticated user
func (c *IssuesController) CurrentUser() *authentication.User {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(c.Request)
	if err != nil {
		return nil
	}
	return user
}

// IsAdmin returns true if the current user is an admin
func (c *IssuesController) IsAdmin() bool {
	user := c.CurrentUser()
	return user != nil && user.IsAdmin
}

// CanCreateIssue returns true if the current user can create issues in the current repo
func (c *IssuesController) CanCreateIssue() bool {
	// Get current repo to check visibility
	repo, err := c.CurrentRepo()
	if err != nil {
		return false
	}

	// Admins can always create issues
	if c.IsAdmin() {
		return true
	}

	// Non-admins can create issues on public repos if authenticated
	user := c.CurrentUser()
	return user != nil && repo.Visibility == "public"
}

// CanEditIssue returns true if the current user can edit the given issue
func (c *IssuesController) CanEditIssue(issue *models.Issue) bool {
	if issue == nil {
		return false
	}

	// Admins can edit any issue
	if c.IsAdmin() {
		return true
	}

	// Authors can edit their own issues
	user := c.CurrentUser()
	return user != nil && issue.AuthorID == user.ID
}

// IssueComments returns comments for the current issue
func (c *IssuesController) IssueComments() ([]*models.Comment, error) {
	issue, err := c.CurrentIssue()
	if err != nil {
		return nil, err
	}
	return models.GetIssueComments(issue.ID)
}

// searchIssues handles issue search requests with HTMX
func (c *IssuesController) searchIssues(w http.ResponseWriter, r *http.Request) {
	// Access already checked by route middleware
	repoID := r.PathValue("id")
	if repoID == "" {
		c.RenderErrorMsg(w, r, "repository ID required")
		return
	}

	// Render just the issues list partial - query params are read from request in template methods
	c.App.Render(w, r, "issues-list-partial.html", nil)
}

// createIssue handles issue creation
func (c *IssuesController) createIssue(w http.ResponseWriter, r *http.Request) {
	// Access already checked by route middleware (PublicRepoOnly)
	auth := c.Use("auth").(*AuthController)
	user, _, _ := auth.Authenticate(r)

	repoID := r.PathValue("id")
	if repoID == "" {
		c.RenderErrorMsg(w, r, "repository ID required")
		return
	}

	// Validate required fields
	title := strings.TrimSpace(r.FormValue("title"))
	body := strings.TrimSpace(r.FormValue("body"))
	column := strings.TrimSpace(r.FormValue("column"))

	if title == "" {
		c.RenderErrorMsg(w, r, "issue title is required")
		return
	}

	// Create the issue
	issue := &models.Issue{
		Title:      title,
		Body:       body,
		Column:     column, // Will be "" for todo, "in_progress", or "done"
		Status:     "open",
		RepoID:     repoID,
		AuthorID:   user.ID, // Set the author
		AssigneeID: user.ID, // Initially assign to creator
	}

	// If column is "done", set status to closed
	if column == "done" {
		issue.Status = "closed"
	}

	_, err := models.Issues.Insert(issue)
	if err != nil {
		c.RenderError(w, r, fmt.Errorf("failed to create issue: %w", err))
		return
	}

	// Log activity
	models.LogActivity("issue_created", "Created issue: "+issue.Title,
		"New issue opened", user.ID, repoID, "issue", issue.ID)

	// Trigger actions for issue creation event
	eventData := map[string]string{
		"ISSUE_ID":     issue.ID,
		"ISSUE_TITLE":  issue.Title,
		"ISSUE_STATUS": string(issue.Status),
		"AUTHOR_ID":    user.ID,
	}
	go services.TriggerActionsByEvent("on_issue", repoID, eventData)

	// Trigger AI event for issue triage if AI is enabled
	if services.Ollama.IsRunning() {
		go func() {
			if err := ai.PublishIssueEvent(ai.EventIssueCreated, issue, user.ID); err != nil {
				log.Printf("Failed to publish issue created event: %v", err)
			}
		}()
	}

	// Refresh to show new issue
	c.Refresh(w, r)
}

// closeIssue handles closing an issue
func (c *IssuesController) closeIssue(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}

	repoID := r.PathValue("id")
	issueID := r.PathValue("issueID")

	if repoID == "" || issueID == "" {
		c.RenderErrorMsg(w, r, "repository ID and issue ID required")
		return
	}

	// Get and update issue
	issue, err := models.Issues.Get(issueID)
	if err != nil {
		c.RenderErrorMsg(w, r, "issue not found")
		return
	}

	// Check if user is admin or issue author
	if !user.IsAdmin && issue.AuthorID != user.ID {
		c.RenderErrorMsg(w, r, "only the author or admin can close this issue")
		return
	}

	issue.Status = "closed"
	err = models.Issues.Update(issue)
	if err != nil {
		c.RenderErrorMsg(w, r, "failed to close issue")
		return
	}

	// Log activity
	models.LogActivity("issue_closed", "Closed issue: "+issue.Title,
		"Issue marked as closed", user.ID, repoID, "issue", issue.ID)

	c.Refresh(w, r)
}

// reopenIssue handles reopening an issue
func (c *IssuesController) reopenIssue(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}

	repoID := r.PathValue("id")
	issueID := r.PathValue("issueID")

	if repoID == "" || issueID == "" {
		c.RenderErrorMsg(w, r, "repository ID and issue ID required")
		return
	}

	// Get and update issue
	issue, err := models.Issues.Get(issueID)
	if err != nil {
		c.RenderErrorMsg(w, r, "issue not found")
		return
	}

	// Check if user is admin or issue author
	if !user.IsAdmin && issue.AuthorID != user.ID {
		c.RenderErrorMsg(w, r, "only the author or admin can reopen this issue")
		return
	}

	issue.Status = "open"
	err = models.Issues.Update(issue)
	if err != nil {
		c.RenderErrorMsg(w, r, "failed to reopen issue")
		return
	}

	// Log activity
	models.LogActivity("issue_reopened", "Reopened issue: "+issue.Title,
		"Issue marked as open", user.ID, repoID, "issue", issue.ID)

	c.Refresh(w, r)
}

// editIssue handles editing an issue
func (c *IssuesController) editIssue(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}

	repoID := r.PathValue("id")
	issueID := r.PathValue("issueID")

	if repoID == "" || issueID == "" {
		c.RenderErrorMsg(w, r, "repository ID and issue ID required")
		return
	}

	// Get the issue
	issue, err := models.Issues.Get(issueID)
	if err != nil {
		c.RenderErrorMsg(w, r, "issue not found")
		return
	}

	// Check if user is admin or issue author
	if !user.IsAdmin && issue.AuthorID != user.ID {
		c.RenderErrorMsg(w, r, "only the author or admin can edit this issue")
		return
	}

	// Update fields
	title := strings.TrimSpace(r.FormValue("title"))
	body := strings.TrimSpace(r.FormValue("body"))
	assigneeID := strings.TrimSpace(r.FormValue("assignee_id"))

	if title != "" {
		issue.Title = title
	}
	issue.Body = body
	issue.AssigneeID = assigneeID

	// Save changes
	err = models.Issues.Update(issue)
	if err != nil {
		c.RenderErrorMsg(w, r, "failed to update issue")
		return
	}

	// Log activity
	models.LogActivity("issue_updated", "Updated issue: "+issue.Title,
		"Issue details modified", user.ID, repoID, "issue", issue.ID)

	c.Refresh(w, r)
}

// deleteIssue handles deleting an issue
func (c *IssuesController) deleteIssue(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}

	repoID := r.PathValue("id")
	issueID := r.PathValue("issueID")

	if repoID == "" || issueID == "" {
		c.RenderErrorMsg(w, r, "repository ID and issue ID required")
		return
	}

	// Get the issue for logging
	issue, err := models.Issues.Get(issueID)
	if err != nil {
		c.RenderErrorMsg(w, r, "issue not found")
		return
	}

	// Admin access already verified by route middleware

	// Delete the issue
	err = models.Issues.Delete(issue)
	if err != nil {
		c.RenderErrorMsg(w, r, "failed to delete issue")
		return
	}

	// Log activity
	models.LogActivity("issue_deleted", "Deleted issue: "+issue.Title,
		"Issue permanently removed", user.ID, repoID, "issue", issue.ID)

	c.Refresh(w, r)
}

// createIssueComment handles adding a comment to an issue
func (c *IssuesController) createIssueComment(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}

	repoID := r.PathValue("id")
	issueID := r.PathValue("issueID")
	body := strings.TrimSpace(r.FormValue("body"))

	if repoID == "" || issueID == "" || body == "" {
		c.RenderErrorMsg(w, r, "repository ID, issue ID, and comment body required")
		return
	}

	// Access already verified by route middleware

	// Verify issue exists
	issue, err := models.Issues.Get(issueID)
	if err != nil {
		c.RenderErrorMsg(w, r, "issue not found")
		return
	}

	// Create comment
	_, err = models.CreateIssueComment(issueID, repoID, user.ID, body)
	if err != nil {
		c.RenderErrorMsg(w, r, "failed to create comment")
		return
	}

	// Log activity
	models.LogActivity("comment_created", "Commented on issue: "+issue.Title,
		"New comment added", user.ID, repoID, "issue_comment", issueID)

	c.Refresh(w, r)
}

// moveIssue handles moving an issue between Kanban columns
func (c *IssuesController) moveIssue(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}

	repoID := r.PathValue("id")
	issueID := r.PathValue("issueID")
	newStatus := r.FormValue("status")

	if repoID == "" || issueID == "" || newStatus == "" {
		c.RenderErrorMsg(w, r, "repository ID, issue ID, and status required")
		return
	}

	// Validate status
	validStatuses := []string{"todo", "in_progress", "done", "open", "closed"}
	isValid := false
	for _, s := range validStatuses {
		if newStatus == s {
			isValid = true
			break
		}
	}
	if !isValid {
		c.RenderErrorMsg(w, r, "invalid status")
		return
	}

	// Get issue
	issue, err := models.Issues.Get(issueID)
	if err != nil {
		c.RenderErrorMsg(w, r, "issue not found")
		return
	}

	// Check if user can modify (author or admin)
	if issue.AuthorID != user.ID && !user.IsAdmin {
		c.RenderErrorMsg(w, r, "unauthorized")
		return
	}

	// Update column and status based on kanban movement
	oldColumn := issue.Column

	// Update based on target column
	switch newStatus {
	case "todo":
		issue.Column = "" // Empty string means todo column (default)
		issue.Status = "open"

	case "in_progress":
		issue.Column = "in_progress"
		issue.Status = "open"

	case "done":
		issue.Column = "done"
		issue.Status = "closed"
	}

	err = models.Issues.Update(issue)
	if err != nil {
		c.RenderErrorMsg(w, r, "failed to update issue")
		return
	}

	// Log activity
	oldColumnDisplay := oldColumn
	if oldColumnDisplay == "" {
		oldColumnDisplay = "todo"
	}
	models.LogActivity("issue_moved", fmt.Sprintf("Moved issue from %s to %s", oldColumnDisplay, newStatus),
		fmt.Sprintf("Issue %s moved", issue.Title), user.ID, repoID, "issue", issue.ID)

	// Test with w.WriteHeader(200) as requested
	w.WriteHeader(200)
}

// GetIssuesByStatus returns issues grouped by status for Kanban view
func (c *IssuesController) GetIssuesByStatus() map[string][]*models.Issue {
	repo, err := c.CurrentRepo()
	if err != nil {
		return map[string][]*models.Issue{
			"todo":        {},
			"in_progress": {},
			"done":        {},
		}
	}

	// Get all issues for the repository
	allIssues, err := models.Issues.Search("WHERE RepoID = ? ORDER BY CreatedAt DESC", repo.ID)
	if err != nil {
		return map[string][]*models.Issue{
			"todo":        {},
			"in_progress": {},
			"done":        {},
		}
	}

	// Group by column
	grouped := map[string][]*models.Issue{
		"todo":        {},
		"in_progress": {},
		"done":        {},
	}

	for _, issue := range allIssues {
		// Use Column field to determine placement
		switch issue.Column {
		case "in_progress":
			grouped["in_progress"] = append(grouped["in_progress"], issue)
		case "done":
			grouped["done"] = append(grouped["done"], issue)
		default:
			// Empty string or any other value goes to todo
			grouped["todo"] = append(grouped["todo"], issue)
		}
	}

	return grouped
}
