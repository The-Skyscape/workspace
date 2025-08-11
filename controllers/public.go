package controllers

import (
	"errors"
	"net/http"
	"strings"
	"workspace/models"

	"github.com/The-Skyscape/devtools/pkg/application"
)

// Public is a factory function with the prefix and instance
func Public() (string, *PublicController) {
	return "public", &PublicController{}
}

// PublicController handles public repository access for unauthenticated users
type PublicController struct {
	application.BaseController
}

// Setup is called when the application is started
func (c *PublicController) Setup(app *application.App) {
	c.BaseController.Setup(app)

	// Public repository routes (no authentication required)
	http.Handle("GET /public/repos/{id}", app.Serve("public-repo-view.html", nil))
	http.Handle("GET /public/repos/{id}/issues", app.Serve("public-repo-issues.html", nil))
	http.Handle("POST /public/repos/{id}/issues", app.ProtectFunc(c.submitIssue, nil))
}

// Handle is called when each request is handled
func (c *PublicController) Handle(req *http.Request) application.Controller {
	c.Request = req
	return c
}

// CurrentRepo returns the public repository from the URL path
func (c *PublicController) CurrentRepo() (*models.Repository, error) {
	return c.getPublicRepoFromRequest(c.Request)
}

// getPublicRepoFromRequest returns a public repository (no authentication required)
func (c *PublicController) getPublicRepoFromRequest(r *http.Request) (*models.Repository, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errors.New("repository ID not found")
	}

	repo, err := models.Repositories.Get(id)
	if err != nil {
		return nil, errors.New("repository not found")
	}

	// Only allow access to public repositories
	if repo.Visibility != "public" {
		return nil, errors.New("repository not found")
	}

	// Repository state is determined dynamically by IsEmpty() method

	return repo, nil
}

// PublicRepoIssues returns issues for the current public repository
func (c *PublicController) PublicRepoIssues() ([]*models.Issue, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	return models.Issues.Search("WHERE RepoID = ? ORDER BY CreatedAt DESC", repo.ID)
}

// submitIssue handles public issue submission
func (c *PublicController) submitIssue(w http.ResponseWriter, r *http.Request) {
	// Get the public repository
	repo, err := c.getPublicRepoFromRequest(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Validate required fields
	title := strings.TrimSpace(r.FormValue("title"))
	body := strings.TrimSpace(r.FormValue("body"))
	email := strings.TrimSpace(r.FormValue("email"))

	if title == "" {
		c.Render(w, r, "error-message.html", errors.New("issue title is required"))
		return
	}

	if email == "" {
		c.Render(w, r, "error-message.html", errors.New("email is required for notifications"))
		return
	}

	// Basic email validation
	if !strings.Contains(email, "@") || !strings.Contains(email, ".") {
		c.Render(w, r, "error-message.html", errors.New("please provide a valid email address"))
		return
	}

	// Create the issue
	issue := &models.Issue{
		Title:      title,
		Body:       body,
		Status:     "open",
		RepoID:     repo.ID,
		AssigneeID: email, // Store submitter email in AssigneeID for public issues
		Tags:       "public-submission", // Tag to identify public submissions
	}

	_, err = models.Issues.Insert(issue)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to create issue: "+err.Error()))
		return
	}

	// Redirect back to the issues page with success
	c.Redirect(w, r, "/public/repos/"+repo.ID+"/issues?submitted=true")
}

// RepoOwnerInfo returns basic information about the repository owner
func (c *PublicController) RepoOwnerInfo() (map[string]interface{}, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	// Get owner information (simplified for public view)
	if repo.UserID != "" {
		// Look up owner information from Auth system if needed
		// For now, return basic info
		return map[string]interface{}{
			"id":     repo.UserID,
			"name":   "Repository Owner",
			"avatar": "https://ui-avatars.com/api/?name=Owner&size=40&background=3b82f6&color=white",
		}, nil
	}

	return map[string]interface{}{
		"name":   "Unknown",
		"avatar": "https://ui-avatars.com/api/?name=?&size=40&background=6b7280&color=white",
	}, nil
}