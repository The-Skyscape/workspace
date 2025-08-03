package controllers

import (
	"errors"
	"net/http"
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
	
	http.Handle("POST /repos/create", app.ProtectFunc(c.createRepo, auth.Required))
	http.Handle("POST /repos/{id}/launch-workspace", app.ProtectFunc(c.launchWorkspace, auth.Required))
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

// getCurrentRepoFromRequest returns the repository from a specific request
func (c *ReposController) getCurrentRepoFromRequest(r *http.Request) (*coding.GitRepo, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errors.New("repository ID not found")
	}
	
	repo, err := models.Coding.GetRepo(id)
	if err != nil {
		return nil, err
	}

	// TODO: Add permission checking here
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

	return models.Actions.Search("WHERE RepoID = ? ORDER BY Timestamp DESC", repo.ID)
}

// createRepo handles repository creation
func (c *ReposController) createRepo(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("unauthorized"))
		return
	}

	name := r.FormValue("name")
	if name == "" {
		c.Render(w, r, "error-message.html", errors.New("repository name is required"))
		return
	}

	// Use the coding package to create a new repository
	repo, err := models.Coding.NewRepo(user.ID, name)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

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

	repo, err := c.getCurrentRepoFromRequest(r)
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

	// Redirect to workspace launcher
	http.Redirect(w, r, "/workspace/"+workspace.ID, http.StatusSeeOther)
}