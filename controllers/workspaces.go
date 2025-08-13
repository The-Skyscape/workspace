package controllers

import (
	"log"
	"net/http"
	"workspace/models"
	"workspace/services"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
)

// Workspaces is a factory function that returns the URL prefix and controller instance.
func Workspaces() (string, *WorkspacesController) {
	return "workspaces", &WorkspacesController{}
}

// WorkspacesController handles workspace management including coder proxy
type WorkspacesController struct {
	application.BaseController
}

// Setup registers all routes for workspace management.
func (c *WorkspacesController) Setup(app *application.App) {
	c.BaseController.Setup(app)

	auth := app.Use("auth").(*authentication.Controller)
	
	// Register coder proxy handler for VS Code workspaces
	http.Handle("/coder/", http.StripPrefix("/coder/", models.CoderHandler(auth)))

	// Initialize the coder service on startup
	if err := services.Coder.Init(); err != nil {
		// Log the error but don't fail startup
		// The service can be started manually if needed
		log.Printf("Warning: Failed to initialize coder service: %v", err)
	}
}

// Handle is called when each request is handled
func (c *WorkspacesController) Handle(req *http.Request) application.Controller {
	c.Request = req
	return c
}

// CoderStatus returns the status of the global coder service
func (c *WorkspacesController) CoderStatus() map[string]interface{} {
	return map[string]interface{}{
		"running":   services.Coder.IsRunning(),
		"port":      services.Coder.GetPort(),
		"adminOnly": services.Coder.IsAdminOnly(),
		"url":       "/coder/",
	}
}