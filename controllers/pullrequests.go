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

// PullRequests controller prefix
func PullRequests() (string, *PullRequestsController) {
	return "prs", &PullRequestsController{}
}

// PullRequestsController handles pull request operations
type PullRequestsController struct {
	application.BaseController
}

// Handle returns a new controller instance for the request
func (c PullRequestsController) Handle(req *http.Request) application.Controller {
	c.Request = req
	return &c
}

// Setup registers routes
func (c *PullRequestsController) Setup(app *application.App) {
	c.BaseController.Setup(app)
	auth := app.Use("auth").(*authentication.Controller)

	// Pull Requests
	http.Handle("GET /repos/{id}/prs", app.Serve("repo-prs.html", auth.Required))
	http.Handle("GET /repos/{id}/prs/search", app.ProtectFunc(c.searchPRs, auth.Required))
	http.Handle("GET /repos/{id}/prs/{prID}/diff", app.Serve("repo-pr-diff.html", auth.Required))
	http.Handle("POST /repos/{id}/prs/create", app.ProtectFunc(c.createPR, auth.Required))
	http.Handle("POST /repos/{id}/prs/{prID}/merge", app.ProtectFunc(c.mergePR, auth.Required))
	http.Handle("POST /repos/{id}/prs/{prID}/close", app.ProtectFunc(c.closePR, auth.Required))
	http.Handle("POST /repos/{id}/prs/{prID}/comment", app.ProtectFunc(c.createPRComment, auth.Required))
}

// CurrentRepo returns the current repository from the request
func (c *PullRequestsController) CurrentRepo() (*models.Repository, error) {
	return c.getCurrentRepo()
}

// RepoPullRequests returns pull requests for the current repository
func (c *PullRequestsController) RepoPullRequests() ([]*models.PullRequest, error) {
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

	// Search pull requests
	prs, err := models.PullRequests.Search(condition, args...)
	if err != nil {
		return nil, err
	}

	return prs, nil
}

// CurrentPullRequest returns the pull request from the request
func (c *PullRequestsController) CurrentPullRequest() (*models.PullRequest, error) {
	prID := c.Request.PathValue("prID")
	if prID == "" {
		return nil, errors.New("pull request ID required")
	}
	return models.PullRequests.Get(prID)
}

// PRComments returns comments for the current pull request
func (c *PullRequestsController) PRComments() ([]*models.Comment, error) {
	pr, err := c.CurrentPullRequest()
	if err != nil {
		return nil, err
	}
	return models.GetPRComments(pr.ID)
}

// RepoPRDiff returns the diff for a pull request
func (c *PullRequestsController) RepoPRDiff() (*models.PRDiff, error) {
	prID := c.Request.PathValue("prID")
	if prID == "" {
		return nil, errors.New("pull request ID required")
	}

	pr, err := models.PullRequests.Get(prID)
	if err != nil {
		return nil, err
	}

	repo, err := c.getCurrentRepo()
	if err != nil {
		return nil, err
	}

	return repo.GetPRDiff(pr.BaseBranch, pr.CompareBranch)
}

// RepoPRDiffContent returns the full diff content for a pull request
func (c *PullRequestsController) RepoPRDiffContent() (string, error) {
	prID := c.Request.PathValue("prID")
	if prID == "" {
		return "", errors.New("pull request ID required")
	}

	pr, err := models.PullRequests.Get(prID)
	if err != nil {
		return "", err
	}

	repo, err := c.getCurrentRepo()
	if err != nil {
		return "", err
	}

	return repo.GetPRDiffContent(pr.BaseBranch, pr.CompareBranch)
}

// SearchQuery returns the current search query
func (c *PullRequestsController) SearchQuery() string {
	return c.Request.URL.Query().Get("search")
}

// IncludeClosed returns whether to include closed PRs
func (c *PullRequestsController) IncludeClosed() bool {
	return c.Request.URL.Query().Get("includeClosed") == "true"
}

// getCurrentRepo helper to get current repository via repos controller
func (c *PullRequestsController) getCurrentRepo() (*models.Repository, error) {
	reposController := c.Use("repos").(*ReposController)
	return reposController.CurrentRepo()
}

// RepoBranches returns branches for the current repository via repos controller
func (c *PullRequestsController) RepoBranches() ([]*models.Branch, error) {
	reposController := c.Use("repos").(*ReposController)
	return reposController.RepoBranches()
}

// IncludeMerged returns whether merged/closed PRs are included from request
func (c *PullRequestsController) IncludeMerged() bool {
	if c.Request == nil {
		return false
	}
	return c.Request.URL.Query().Get("includeMerged") == "true"
}

// searchPRs handles PR search requests with HTMX
func (c *PullRequestsController) searchPRs(w http.ResponseWriter, r *http.Request) {
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

	// Check repository access
	err = models.CheckRepoAccess(user, repoID, models.RoleRead)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Render just the PRs list partial - query params are read from request in template methods
	c.App.Render(w, r, "prs-list-partial.html", nil)
}

// createPR handles pull request creation
func (c *PullRequestsController) createPR(w http.ResponseWriter, r *http.Request) {
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

	// Check repository access
	err = models.CheckRepoAccess(user, repoID, models.RoleRead)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Validate required fields
	title := strings.TrimSpace(r.FormValue("title"))
	body := strings.TrimSpace(r.FormValue("body"))
	baseBranch := strings.TrimSpace(r.FormValue("base_branch"))
	compareBranch := strings.TrimSpace(r.FormValue("compare_branch"))

	if title == "" || baseBranch == "" || compareBranch == "" {
		c.Render(w, r, "error-message.html", errors.New("title, base branch, and compare branch are required"))
		return
	}

	// Create the pull request
	pr := &models.PullRequest{
		Title:         title,
		Body:          body,
		RepoID:        repoID,
		AuthorID:      user.ID,
		BaseBranch:    baseBranch,
		CompareBranch: compareBranch,
		Status:        "open",
	}

	_, err = models.PullRequests.Insert(pr)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to create pull request: "+err.Error()))
		return
	}

	// Log activity
	models.LogActivity("pr_created", "Created pull request: "+pr.Title,
		"New pull request opened", user.ID, repoID, "pull_request", pr.ID)

	// Trigger actions for PR creation event
	eventData := map[string]string{
		"PR_ID":          pr.ID,
		"PR_TITLE":       pr.Title,
		"PR_STATUS":      pr.Status,
		"BASE_BRANCH":    pr.BaseBranch,
		"COMPARE_BRANCH": pr.CompareBranch,
		"AUTHOR_ID":      user.ID,
	}
	go models.TriggerActionsByEvent("on_pr", repoID, eventData)

	// Redirect to PRs page
	c.Redirect(w, r, "/repos/"+repoID+"/prs")
}

// mergePR handles merging a pull request
func (c *PullRequestsController) mergePR(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	repoID := r.PathValue("id")
	prID := r.PathValue("prID")

	if repoID == "" || prID == "" {
		http.Error(w, "repository ID and PR ID required", http.StatusBadRequest)
		return
	}

	// Get PR first
	pr, err := models.PullRequests.Get(prID)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("pull request not found"))
		return
	}

	// Check if user can merge PR (admin or user with write permission)
	if !models.CanUserMergePR(user, pr) {
		c.Render(w, r, "error-message.html", errors.New("insufficient permissions to merge"))
		return
	}

	// Check if PR is still open
	if pr.Status != "open" {
		c.Render(w, r, "error-message.html", errors.New("pull request is not open"))
		return
	}

	// Get repository
	repo, err := models.Repositories.Get(repoID)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("repository not found"))
		return
	}

	// Check if branches can be merged
	canMerge, err := repo.CanMergeBranch(pr.CompareBranch, pr.BaseBranch)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to check merge compatibility: "+err.Error()))
		return
	}

	if !canMerge {
		c.Render(w, r, "error-message.html", errors.New("merge conflicts detected - cannot auto-merge"))
		return
	}

	// Perform the actual git merge
	mergeMessage := fmt.Sprintf("Merge pull request #%s: %s", prID, pr.Title)
	err = repo.MergeBranch(pr.CompareBranch, pr.BaseBranch, mergeMessage, user.Name, user.Email)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to merge branches: "+err.Error()))
		return
	}

	// Update PR status
	pr.Status = "merged"
	err = models.PullRequests.Update(pr)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to update pull request status"))
		return
	}

	// Log activity
	models.LogActivity("pr_merged", "Merged pull request: "+pr.Title,
		"Pull request merged", user.ID, repoID, "pull_request", pr.ID)

	// Trigger actions for push event (merge is essentially a push to base branch)
	eventData := map[string]string{
		"BRANCH":         pr.BaseBranch,
		"PR_ID":          pr.ID,
		"PR_TITLE":       pr.Title,
		"COMPARE_BRANCH": pr.CompareBranch,
		"AUTHOR_ID":      user.ID,
		"EVENT_TYPE":     "merge",
	}
	go models.TriggerActionsByEvent("on_push", repoID, eventData)

	c.Refresh(w, r)
}

// closePR handles closing a pull request
func (c *PullRequestsController) closePR(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	repoID := r.PathValue("id")
	prID := r.PathValue("prID")

	if repoID == "" || prID == "" {
		http.Error(w, "repository ID and PR ID required", http.StatusBadRequest)
		return
	}

	// Get and update PR
	pr, err := models.PullRequests.Get(prID)
	if err != nil {
		http.Error(w, "pull request not found", http.StatusNotFound)
		return
	}

	pr.Status = "closed"
	err = models.PullRequests.Update(pr)
	if err != nil {
		http.Error(w, "failed to close pull request", http.StatusInternalServerError)
		return
	}

	// Log activity
	models.LogActivity("pr_closed", "Closed pull request: "+pr.Title,
		"Pull request closed", user.ID, repoID, "pull_request", pr.ID)

	c.Refresh(w, r)
}

// createPRComment handles adding a comment to a pull request
func (c *PullRequestsController) createPRComment(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("authentication required"))
		return
	}

	repoID := r.PathValue("id")
	prID := r.PathValue("prID")
	body := strings.TrimSpace(r.FormValue("body"))

	if repoID == "" || prID == "" || body == "" {
		c.Render(w, r, "error-message.html", errors.New("repository ID, PR ID, and comment body required"))
		return
	}

	// Check repository read access (anyone who can read can comment)
	err = models.CheckRepoAccess(user, repoID, models.RoleRead)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("insufficient permissions"))
		return
	}

	// Verify PR exists
	pr, err := models.PullRequests.Get(prID)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("pull request not found"))
		return
	}

	// Create comment
	_, err = models.CreatePRComment(prID, repoID, user.ID, body)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to create comment"))
		return
	}

	// Log activity
	models.LogActivity("comment_created", "Commented on PR: "+pr.Title,
		"New comment added", user.ID, repoID, "pr_comment", prID)

	c.Refresh(w, r)
}