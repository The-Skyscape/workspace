package controllers

import (
	"errors"
	"log"
	"net/http"
	"workspace/models"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
)

// Workspaces is a factory function that returns the URL prefix and controller instance.
// The prefix "workspaces" means this controller handles all routes starting with /workspaces
func Workspaces() (string, *WorkspacesController) {
	return "workspaces", &WorkspacesController{}
}

// WorkspacesController handles workspace lifecycle management
type WorkspacesController struct {
	application.BaseController
}

// Setup registers all routes for workspace management.
// Routes:
//   GET  /workspaces         - List all user workspaces (renders workspaces-list.html)
//   GET  /workspace          - Redirect to user's active workspace or workspace list
//   GET  /workspace/{id}     - Show workspace launcher page
//   POST /workspace/{id}/start - Start a stopped workspace container
//   POST /workspace/{id}/stop  - Stop a running workspace container
//   DELETE /workspace/{id}   - Delete a workspace and its container
//   POST /workspaces/create  - Create a new workspace (blank or from repo)
//   /coder/{id}/*           - Proxy to VS Code server in workspace container
func (c *WorkspacesController) Setup(app *application.App) {
	c.BaseController.Setup(app)

	auth := app.Use("auth").(*authentication.Controller)
	http.Handle("GET /workspaces", app.Serve("workspaces-list.html", auth.Required))
	http.Handle("GET /workspace", app.ProtectFunc(c.redirectToUserWorkspace, auth.Required))
	http.Handle("GET /workspace/{id}", app.Serve("workspace-launcher.html", auth.Required))
	http.Handle("POST /workspace/{id}/start", app.ProtectFunc(c.startWorkspace, auth.Required))
	http.Handle("POST /workspace/{id}/stop", app.ProtectFunc(c.stopWorkspace, auth.Required))
	http.Handle("DELETE /workspace/{id}", app.ProtectFunc(c.deleteWorkspace, auth.Required))
	http.Handle("POST /workspaces/create", app.ProtectFunc(c.createWorkspace, auth.Required))

	// Coder proxy - handles both /coder/ and /coder/workspace-id/...
	http.Handle("/coder/", http.StripPrefix("/coder/", models.WorkspaceHandler(auth)))
}

// Handle is called when each request is handled
func (c *WorkspacesController) Handle(req *http.Request) application.Controller {
	c.Request = req
	return c
}

// CurrentWorkspace returns the workspace from the URL path parameter {id}.
// This method is accessible from templates as {{workspaces.CurrentWorkspace}}
// It also verifies the current user owns the workspace.
func (c *WorkspacesController) CurrentWorkspace() (*models.Workspace, error) {
	id := c.Request.PathValue("id")
	if id == "" {
		return nil, errors.New("workspace ID not found")
	}

	// Get workspace by ID
	workspace, err := models.GetWorkspaceByID(id)
	if err != nil {
		return nil, err
	}

	// Verify ownership
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(c.Request)
	if err != nil {
		return nil, errors.New("unauthorized")
	}

	if workspace.UserID != user.ID {
		return nil, errors.New("access denied")
	}

	return workspace, nil
}

// UserWorkspaces returns all workspaces owned by the authenticated user.
// This method is accessible from templates as {{workspaces.UserWorkspaces}}
// Used in workspaces-list.html to show all user's workspaces.
func (c *WorkspacesController) UserWorkspaces() ([]*models.Workspace, error) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(c.Request)
	if err != nil {
		return nil, errors.New("unauthorized")
	}

	return models.GetUserWorkspaces(user.ID)
}

// getCurrentWorkspaceFromRequest returns the workspace from a specific request
func (c *WorkspacesController) getCurrentWorkspaceFromRequest(r *http.Request) (*models.Workspace, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errors.New("workspace ID not found")
	}

	// Get workspace by ID
	workspace, err := models.GetWorkspaceByID(id)
	if err != nil {
		return nil, err
	}

	// Verify ownership
	auth := c.App.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		return nil, errors.New("unauthorized")
	}

	if workspace.UserID != user.ID {
		return nil, errors.New("access denied")
	}

	return workspace, nil
}

// WorkspaceRepo returns the repository associated with the current workspace
func (c *WorkspacesController) WorkspaceRepo() (*models.GitRepo, error) {
	workspace, err := c.CurrentWorkspace()
	if err != nil {
		return nil, err
	}

	return workspace.Repo()
}

// redirectToUserWorkspace redirects to the user's workspace proxy or shows workspace list
func (c *WorkspacesController) redirectToUserWorkspace(w http.ResponseWriter, r *http.Request) {
	auth := c.App.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("unauthorized"))
		return
	}

	// Get user's most recent workspace
	workspace, err := models.GetWorkspace(user.ID)
	if err != nil || workspace == nil {
		// No workspace, redirect to workspace list
		c.Redirect(w, r, "/workspaces")
		return
	}

	// Redirect to workspace proxy with ID
	c.Redirect(w, r, "/coder/"+workspace.ID+"/")
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
			log.Printf("Failed to start workspace %s: %v", workspace.ID, err)
			workspace.Status = "error"
			workspace.ErrorMessage = err.Error()
			models.Workspaces.Update(workspace)
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

	// Redirect to workspaces list
	c.Redirect(w, r, "/workspaces")
}

// createWorkspace handles creating a new workspace
func (c *WorkspacesController) createWorkspace(w http.ResponseWriter, r *http.Request) {
	auth := c.App.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("unauthorized"))
		return
	}

	// Get optional repository ID
	repoID := r.FormValue("repo_id")
	var repo *models.GitRepo
	if repoID != "" {
		repo, err = models.GitRepos.Get(repoID)
		if err != nil {
			c.Render(w, r, "error-message.html", errors.New("repository not found"))
			return
		}

		// Check access
		err = models.CheckRepoAccess(user, repoID, models.RoleRead)
		if err != nil {
			c.Render(w, r, "error-message.html", errors.New("insufficient permissions"))
			return
		}
	}

	// Get available port
	workspaces, _ := models.GetWorkspaces()
	port := 8000
	usedPorts := make(map[int]bool)
	for _, ws := range workspaces {
		usedPorts[ws.Port] = true
	}
	for usedPorts[port] {
		port++
	}

	// Create new workspace
	workspace, err := models.NewWorkspace(user.ID, port, repo)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Start the workspace asynchronously
	go func() {
		if err := workspace.Start(user, repo); err != nil {
			log.Printf("Failed to start workspace %s: %v", workspace.ID, err)
		}
	}()

	// Log activity
	if repo != nil {
		models.LogActivity("workspace_created", "Created workspace for "+repo.Name,
			"New development workspace", user.ID, repo.ID, "workspace", workspace.ID)
	} else {
		models.LogActivity("workspace_created", "Created blank workspace",
			"New development workspace", user.ID, "", "workspace", workspace.ID)
	}

	// Redirect to workspace
	c.Redirect(w, r, "/coder/"+workspace.ID+"/")
}
