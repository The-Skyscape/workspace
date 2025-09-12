package controllers

import (
	"errors"
	"log"
	"net/http"
	"workspace/services"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/containers"
)

// Workspaces is a factory function that returns the URL prefix and controller instance.
func Workspaces() (string, *WorkspacesController) {
	return "workspaces", &WorkspacesController{}
}

// WorkspacesController handles development workspace management (VS Code)
type WorkspacesController struct {
	application.Controller
}

// Setup registers all routes for workspace management.
func (w *WorkspacesController) Setup(app *application.App) {
	w.Controller.Setup(app)

	auth := app.Use("auth").(*AuthController)

	// Register coder proxy handler for Code IDE (admin only)
	http.Handle("/coder/", http.StripPrefix("/coder/",
		auth.ProtectFunc(w.proxyToCodeServer, true)))

	// Initialize the coder service on startup in background
	go func() {
		if err := services.Coder.Init(); err != nil {
			// Log the error but don't fail startup
			// The service can be started manually if needed
			log.Printf("Warning: Failed to initialize coder service: %v", err)
		}
	}()

}

// Handle is called when each request is handled
func (w WorkspacesController) Handle(req *http.Request) application.IController {
	w.Request = req
	return &w
}

// proxyToCodeServer handles requests to the code-server instance
func (w *WorkspacesController) proxyToCodeServer(wr http.ResponseWriter, r *http.Request) {
	if !services.Coder.IsRunning() {
		w.RenderError(wr, r, errors.New("coder service is not running"))
		return
	}

	// Use the containers.Service proxy method like the original
	service := &containers.Service{
		Host: containers.Local(),
		Name: "skyscape-coder",
	}

	proxy := service.Proxy(services.Coder.GetPort())
	proxy.ServeHTTP(wr, r)
}

// CoderStatus returns the status of the global coder service
func (w *WorkspacesController) CoderStatus() *services.CoderStatus {
	return services.Coder.GetStatus()
}

// CoderService returns the Coder service instance for template access
func (w *WorkspacesController) CoderService() *services.CoderService {
	return services.Coder
}

// GetCoderWorkspace checks if a workspace exists for the given repository
func (w *WorkspacesController) GetCoderWorkspace(repoID string) error {
	// This would check if a Coder workspace exists for the repository
	// For now, we just check if the service is running
	if !services.Coder.IsRunning() {
		return errors.New("Coder service is not running")
	}
	return nil
}
