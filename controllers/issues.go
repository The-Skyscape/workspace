package controllers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"workspace/models"

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
	auth := app.Use("auth").(*authentication.Controller)

	// Issues
	http.Handle("GET /repos/{id}/issues", app.Serve("repo-issues.html", auth.Required))
	http.Handle("GET /repos/{id}/issues/search", app.ProtectFunc(c.searchIssues, auth.Required))
	http.Handle("POST /repos/{id}/issues/create", app.ProtectFunc(c.createIssue, auth.Required))
	http.Handle("POST /repos/{id}/issues/{issueID}/close", app.ProtectFunc(c.closeIssue, auth.Required))
	http.Handle("POST /repos/{id}/issues/{issueID}/reopen", app.ProtectFunc(c.reopenIssue, auth.Required))
	http.Handle("POST /repos/{id}/issues/{issueID}/edit", app.ProtectFunc(c.editIssue, auth.Required))
	http.Handle("POST /repos/{id}/issues/{issueID}/delete", app.ProtectFunc(c.deleteIssue, auth.Required))
	http.Handle("POST /repos/{id}/issues/{issueID}/comment", app.ProtectFunc(c.createIssueComment, auth.Required))
}

// CurrentRepo returns the current repository from the request
func (c *IssuesController) CurrentRepo() (*models.Repository, error) {
	return c.getCurrentRepo()
}

// RepoIssues returns issues for the current repository
func (c *IssuesController) RepoIssues() ([]*models.Issue, error) {
	repo, err := c.getCurrentRepo()
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

	// Search issues
	issues, err := models.Issues.Search(condition, args...)
	if err != nil {
		return nil, err
	}

	return issues, nil
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

// IssueComments returns comments for the current issue
func (c *IssuesController) IssueComments() ([]*models.Comment, error) {
	issue, err := c.CurrentIssue()
	if err != nil {
		return nil, err
	}
	return models.GetIssueComments(issue.ID)
}

// getCurrentRepo helper to get current repository via repos controller
func (c *IssuesController) getCurrentRepo() (*models.Repository, error) {
	reposController := c.Use("repos").(*ReposController)
	return reposController.CurrentRepo()
}

// searchIssues handles issue search requests with HTMX
func (c *IssuesController) searchIssues(w http.ResponseWriter, r *http.Request) {
	// Use shared middleware for permission checking
	if !RepoReadRequired()(c.App, w, r) {
		return
	}

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
	// Use shared middleware for permission checking
	if !RepoReadRequired()(c.App, w, r) {
		return
	}

	auth := c.Use("auth").(*authentication.Controller)
	user, _, _ := auth.Authenticate(r)

	repoID := r.PathValue("id")
	if repoID == "" {
		c.RenderErrorMsg(w, r, "repository ID required")
		return
	}

	// Validate required fields
	title := strings.TrimSpace(r.FormValue("title"))
	body := strings.TrimSpace(r.FormValue("body"))
	tags := strings.TrimSpace(r.FormValue("tags"))

	if title == "" {
		c.RenderErrorMsg(w, r, "issue title is required")
		return
	}

	// Create the issue
	issue := &models.Issue{
		Title:      title,
		Body:       body,
		Tags:       tags,
		Status:     "open",
		RepoID:     repoID,
		AuthorID:   user.ID,  // Set the author
		AssigneeID: user.ID,  // Initially assign to creator
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
		"ISSUE_STATUS": issue.Status,
		"AUTHOR_ID":    user.ID,
	}
	go models.TriggerActionsByEvent("on_issue", repoID, eventData)

	// Refresh to show new issue
	c.Refresh(w, r)
}

// closeIssue handles closing an issue
func (c *IssuesController) closeIssue(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
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

	// Check if user can update this issue
	if !models.CanUserUpdateIssue(user, issue) {
		c.RenderErrorMsg(w, r, "insufficient permissions")
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
	auth := c.Use("auth").(*authentication.Controller)
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

	// Check if user can update this issue
	if !models.CanUserUpdateIssue(user, issue) {
		c.RenderErrorMsg(w, r, "insufficient permissions")
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
	auth := c.Use("auth").(*authentication.Controller)
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

	// Check if user can update this issue
	if !models.CanUserUpdateIssue(user, issue) {
		c.RenderErrorMsg(w, r, "insufficient permissions")
		return
	}

	// Update fields
	title := strings.TrimSpace(r.FormValue("title"))
	body := strings.TrimSpace(r.FormValue("body"))
	tags := strings.TrimSpace(r.FormValue("tags"))
	assigneeID := strings.TrimSpace(r.FormValue("assignee_id"))

	if title != "" {
		issue.Title = title
	}
	issue.Body = body
	issue.Tags = tags
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
	auth := c.Use("auth").(*authentication.Controller)
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

	// Check if user can delete this issue (admin only)
	if !models.IsUserAdmin(user) {
		c.RenderErrorMsg(w, r, "insufficient permissions")
		return
	}

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
	auth := c.Use("auth").(*authentication.Controller)
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

	// Check repository read access (anyone who can read can comment)
	err = models.CheckRepoAccess(user, repoID, models.RoleRead)
	if err != nil {
		c.RenderErrorMsg(w, r, "insufficient permissions")
		return
	}

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