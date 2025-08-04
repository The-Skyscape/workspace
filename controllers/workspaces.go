package controllers

import (
	"errors"
	"net/http"
	"workspace/models"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
)

// Workspaces is a factory function with the prefix and instance
func Workspaces() (string, *WorkspacesController) {
	return "workspaces", &WorkspacesController{}
}

// WorkspacesController handles workspace lifecycle management
type WorkspacesController struct {
	application.BaseController
}

// Setup is called when the application is started
func (c *WorkspacesController) Setup(app *application.App) {
	c.BaseController.Setup(app)

	auth := app.Use("auth").(*authentication.Controller)
	http.Handle("GET /workspace", app.ProtectFunc(c.redirectToUserWorkspace, auth.Required))
	http.Handle("GET /workspace/{id}", app.Serve("workspace-launcher.html", auth.Required))
	http.Handle("POST /workspace/{id}/start", app.ProtectFunc(c.startWorkspace, auth.Required))
	http.Handle("POST /workspace/{id}/stop", app.ProtectFunc(c.stopWorkspace, auth.Required))
	http.Handle("DELETE /workspace/{id}", app.ProtectFunc(c.deleteWorkspace, auth.Required))

	http.Handle("/coder/", http.StripPrefix("/coder/", models.WorkspaceHandler(auth)))
}

// Handle is called when each request is handled
func (c *WorkspacesController) Handle(req *http.Request) application.Controller {
	c.Request = req
	return c
}

// CurrentWorkspace returns the workspace from the URL path
func (c *WorkspacesController) CurrentWorkspace() (*models.Workspace, error) {
	id := c.Request.PathValue("id")
	if id == "" {
		return nil, errors.New("workspace ID not found")
	}

	// For now, we'll get the workspace by user ID since that's what the coding package supports
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(c.Request)
	if err != nil {
		return nil, errors.New("unauthorized")
	}

	return models.GetWorkspace(user.ID)
}

// getCurrentWorkspaceFromRequest returns the workspace from a specific request
func (c *WorkspacesController) getCurrentWorkspaceFromRequest(r *http.Request) (*models.Workspace, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errors.New("workspace ID not found")
	}

	// For now, we'll get the workspace by user ID since that's what the coding package supports
	auth := c.App.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		return nil, errors.New("unauthorized")
	}

	return models.GetWorkspace(user.ID)
}

// WorkspaceRepo returns the repository associated with the current workspace
func (c *WorkspacesController) WorkspaceRepo() (*models.GitRepo, error) {
	workspace, err := c.CurrentWorkspace()
	if err != nil {
		return nil, err
	}

	return workspace.Repo()
}

// redirectToUserWorkspace redirects to the user's workspace proxy or creates one
func (c *WorkspacesController) redirectToUserWorkspace(w http.ResponseWriter, r *http.Request) {
	auth := c.App.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("unauthorized"))
		return
	}

	// Check if user has a workspace
	workspace, err := models.GetWorkspace(user.ID)
	if err != nil || workspace == nil {
		// Create workspace if it doesn't exist
		workspaces, _ := models.GetWorkspaces()
		port := 8000 + len(workspaces)
		workspace, err = models.NewWorkspace(user.ID, port, nil)
		if err != nil {
			c.Render(w, r, "error-message.html", err)
			return
		}
	}

	// Redirect to workspace proxy which handles starting/loading
	http.Redirect(w, r, "/coder/", http.StatusSeeOther)
}

// startWorkspace handles starting a workspace container
func (c *WorkspacesController) startWorkspace(w http.ResponseWriter, r *http.Request) {
	workspace, err := c.getCurrentWorkspaceFromRequest(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	auth := c.App.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("unauthorized"))
		return
	}

	// Start the workspace using the coding package
	go func() {
		if err := workspace.Start(user, nil); err != nil {
			// TODO: Handle error better
			return
		}
	}()

	c.Refresh(w, r)
}

// stopWorkspace handles stopping a workspace container
func (c *WorkspacesController) stopWorkspace(w http.ResponseWriter, r *http.Request) {
	workspace, err := c.getCurrentWorkspaceFromRequest(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	if err := workspace.Stop(); err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	c.Refresh(w, r)
}

// deleteWorkspace handles deleting a workspace
func (c *WorkspacesController) deleteWorkspace(w http.ResponseWriter, r *http.Request) {
	workspace, err := c.getCurrentWorkspaceFromRequest(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	if err := workspace.Delete(); err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Redirect to dashboard
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}
