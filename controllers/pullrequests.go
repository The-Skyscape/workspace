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

	// Pull Requests - view on public repos or as admin
	http.Handle("GET /repos/{id}/prs", app.Serve("repo-prs.html", PublicOrAdmin()))
	http.Handle("GET /repos/{id}/prs/search", app.ProtectFunc(c.searchPRs, PublicOrAdmin()))
	http.Handle("GET /repos/{id}/prs/more", app.Serve("prs-more.html", PublicOrAdmin()))
	http.Handle("GET /repos/{id}/prs/{prID}/diff", app.Serve("repo-pr-diff.html", PublicOrAdmin()))
	
	// PR operations - authenticated users on public repos, admins on any
	http.Handle("POST /repos/{id}/prs/create", app.ProtectFunc(c.createPR, PublicRepoOnly()))
	http.Handle("POST /repos/{id}/prs/{prID}/comment", app.ProtectFunc(c.createPRComment, PublicRepoOnly()))
	
	// PR merge - admin only
	http.Handle("POST /repos/{id}/prs/{prID}/merge", app.ProtectFunc(c.mergePR, AdminOnly()))
	
	// PR close - author or admin
	http.Handle("POST /repos/{id}/prs/{prID}/close", app.ProtectFunc(c.closePR, auth.Required))
}

// CurrentRepo returns the current repository from the request
func (c *PullRequestsController) CurrentRepo() (*models.Repository, error) {
	reposController := c.Use("repos").(*ReposController)
	return reposController.CurrentRepo()
}

// RepoPullRequests returns pull requests for the current repository
func (c *PullRequestsController) RepoPullRequests() ([]*models.PullRequest, error) {
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

	// Search pull requests
	prs, err := models.PullRequests.Search(condition, args...)
	if err != nil {
		return nil, err
	}

	return prs, nil
}

// MorePRs returns the next page of PRs for infinite scroll
func (c *PullRequestsController) MorePRs() ([]*models.PullRequest, error) {
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
	
	// Get next batch of PRs
	prs, _, err := models.GetRepoPRsPaginated(repo.ID, includeClosed, 20, offset)
	return prs, err
}

// HasMorePRs checks if there are more PRs to load
func (c *PullRequestsController) HasMorePRs() bool {
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
	prs, total, err := models.GetRepoPRsPaginated(repo.ID, includeClosed, 20, offset)
	if err != nil {
		return false
	}
	
	return (offset + len(prs)) < total
}

// NextPRsOffset returns the offset for the next page of PRs
func (c *PullRequestsController) NextPRsOffset() int {
	offsetStr := c.Request.URL.Query().Get("offset")
	offset := 0
	if offsetStr != "" {
		fmt.Sscanf(offsetStr, "%d", &offset)
	}
	return offset + 20
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

	reposController := c.Use("repos").(*ReposController)
	repo, err := reposController.CurrentRepo()
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

	reposController := c.Use("repos").(*ReposController)
	repo, err := reposController.CurrentRepo()
	if err != nil {
		return "", err
	}

	return repo.GetPRDiffContent(pr.BaseBranch, pr.CompareBranch)
}

// SearchQuery returns the current search query
func (c *PullRequestsController) SearchQuery() string {
	return c.Request.URL.Query().Get("search")
}

// IncludeClosed returns whether closed PRs should be included
func (c *PullRequestsController) IncludeClosed() bool {
	return c.Request.URL.Query().Get("includeClosed") == "true"
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
	// Access already verified by route middleware

	repoID := r.PathValue("id")
	if repoID == "" {
		c.RenderErrorMsg(w, r, "repository ID required")
		return
	}

	// Render just the PRs list partial - query params are read from request in template methods
	c.App.Render(w, r, "prs-list-partial.html", nil)
}

// createPR handles pull request creation
func (c *PullRequestsController) createPR(w http.ResponseWriter, r *http.Request) {
	// Access already verified by route middleware (PublicRepoOnly)

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
	baseBranch := strings.TrimSpace(r.FormValue("base_branch"))
	compareBranch := strings.TrimSpace(r.FormValue("compare_branch"))

	if title == "" || baseBranch == "" || compareBranch == "" {
		c.RenderErrorMsg(w, r, "title, base branch, and compare branch are required")
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

	_, err := models.PullRequests.Insert(pr)
	if err != nil {
		c.RenderError(w, r, fmt.Errorf("failed to create pull request: %w", err))
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
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}

	repoID := r.PathValue("id")
	prID := r.PathValue("prID")

	if repoID == "" || prID == "" {
		c.RenderErrorMsg(w, r, "repository ID and PR ID required")
		return
	}

	// Get PR first
	pr, err := models.PullRequests.Get(prID)
	if err != nil {
		c.RenderErrorMsg(w, r, "pull request not found")
		return
	}

	// Only admins can merge PRs
	if !user.IsAdmin {
		c.RenderErrorMsg(w, r, "only admins can merge pull requests")
		return
	}

	// Check if PR is still open
	if pr.Status != "open" {
		c.RenderErrorMsg(w, r, "pull request is not open")
		return
	}

	// Get repository
	repo, err := models.Repositories.Get(repoID)
	if err != nil {
		c.RenderErrorMsg(w, r, "repository not found")
		return
	}

	// Check if branches can be merged
	canMerge, err := repo.CanMergeBranch(pr.CompareBranch, pr.BaseBranch)
	if err != nil {
		c.RenderError(w, r, fmt.Errorf("failed to check merge compatibility: %w", err))
		return
	}

	if !canMerge {
		c.RenderErrorMsg(w, r, "merge conflicts detected - cannot auto-merge")
		return
	}

	// Perform the actual git merge
	mergeMessage := fmt.Sprintf("Merge pull request #%s: %s", prID, pr.Title)
	err = repo.MergeBranch(pr.CompareBranch, pr.BaseBranch, mergeMessage, user.Name, user.Email)
	if err != nil {
		c.RenderError(w, r, fmt.Errorf("failed to merge branches: %w", err))
		return
	}

	// Update PR status
	pr.Status = "merged"
	err = models.PullRequests.Update(pr)
	if err != nil {
		c.RenderErrorMsg(w, r, "failed to update pull request status")
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
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}

	repoID := r.PathValue("id")
	prID := r.PathValue("prID")

	if repoID == "" || prID == "" {
		c.RenderErrorMsg(w, r, "repository ID and PR ID required")
		return
	}

	// Get and update PR
	pr, err := models.PullRequests.Get(prID)
	if err != nil {
		c.RenderErrorMsg(w, r, "pull request not found")
		return
	}
	
	// Check if user is admin or PR author
	if !user.IsAdmin && pr.AuthorID != user.ID {
		c.RenderErrorMsg(w, r, "only the author or admin can close this pull request")
		return
	}

	pr.Status = "closed"
	err = models.PullRequests.Update(pr)
	if err != nil {
		c.RenderErrorMsg(w, r, "failed to close pull request")
		return
	}

	// Log activity
	models.LogActivity("pr_closed", "Closed pull request: "+pr.Title,
		"Pull request closed", user.ID, repoID, "pull_request", pr.ID)

	c.Refresh(w, r)
}

// createPRComment handles adding a comment to a pull request
func (c *PullRequestsController) createPRComment(w http.ResponseWriter, r *http.Request) {
	// Access already verified by route middleware (PublicRepoOnly)

	auth := c.Use("auth").(*authentication.Controller)
	user, _, _ := auth.Authenticate(r)

	repoID := r.PathValue("id")
	prID := r.PathValue("prID")
	body := strings.TrimSpace(r.FormValue("body"))

	if repoID == "" || prID == "" || body == "" {
		c.RenderErrorMsg(w, r, "repository ID, PR ID, and comment body required")
		return
	}

	// Verify PR exists
	pr, err := models.PullRequests.Get(prID)
	if err != nil {
		c.RenderErrorMsg(w, r, "pull request not found")
		return
	}

	// Create comment
	_, err = models.CreatePRComment(prID, repoID, user.ID, body)
	if err != nil {
		c.RenderErrorMsg(w, r, "failed to create comment")
		return
	}

	// Log activity
	models.LogActivity("comment_created", "Commented on PR: "+pr.Title,
		"New comment added", user.ID, repoID, "pr_comment", prID)

	c.Refresh(w, r)
}

