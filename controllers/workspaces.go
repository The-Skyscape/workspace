package controllers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"workspace/services"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/containers"
)

// Workspaces is a factory function that returns the URL prefix and controller instance.
func Workspaces() (string, *WorkspacesController) {
	return "workspaces", &WorkspacesController{}
}

// WorkspacesController handles development workspace management (VS Code and Jupyter)
type WorkspacesController struct {
	application.BaseController
}

// Setup registers all routes for workspace management.
func (w *WorkspacesController) Setup(app *application.App) {
	w.BaseController.Setup(app)

	auth := app.Use("auth").(*authentication.Controller)
	
	// Register coder proxy handler for Code IDE (admin only)
	http.Handle("/coder/", http.StripPrefix("/coder/", 
		auth.ProtectFunc(w.proxyToCodeServer, true)))

	// Initialize the coder service on startup
	if err := services.Coder.Init(); err != nil {
		// Log the error but don't fail startup
		// The service can be started manually if needed
		log.Printf("Warning: Failed to initialize coder service: %v", err)
	}

	// Register IPython/Jupyter proxy handler (admin only)
	// Don't strip prefix - Jupyter is configured with base_url=/ipython/
	// The trailing slash makes this a catch-all for /ipython/* paths
	http.Handle("/ipython/", auth.ProtectFunc(w.proxyToJupyter, true))

	// Initialize IPython service on startup  
	if err := services.IPython.Init(); err != nil {
		log.Printf("Warning: Failed to initialize IPython service: %v", err)
	}
}

// Handle is called when each request is handled
func (w WorkspacesController) Handle(req *http.Request) application.Controller {
	w.Request = req
	return &w
}

// proxyToCodeServer handles requests to the code-server instance
func (w *WorkspacesController) proxyToCodeServer(wr http.ResponseWriter, r *http.Request) {
	if !services.Coder.IsRunning() {
		w.Render(wr, r, "error-message.html", errors.New("coder service is not running"))
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

// proxyToJupyter handles requests to the Jupyter notebook instance
func (w *WorkspacesController) proxyToJupyter(wr http.ResponseWriter, r *http.Request) {
	if !services.IPython.IsRunning() {
		w.Render(wr, r, "error-message.html", errors.New("IPython/Jupyter service is not running"))
		return
	}

	// Create reverse proxy
	target, err := url.Parse(fmt.Sprintf("http://localhost:%d", services.IPython.GetPort()))
	if err != nil {
		w.Render(wr, r, "error-message.html", errors.New("failed to create proxy"))
		return
	}
	
	proxy := httputil.NewSingleHostReverseProxy(target)
	
	// Jupyter is configured with base_url=/ipython/ and expects the full path
	// The request path already includes /ipython/ prefix since we don't strip it
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

// IPythonService returns the IPython service instance for template access
func (w *WorkspacesController) IPythonService() *services.IPythonService {
	return services.IPython
}

// IPythonStatus returns the status of the global IPython service
func (w *WorkspacesController) IPythonStatus() *services.IPythonStatus {
	return services.IPython.GetStatus()
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

// GetIPythonWorkspace checks if a Jupyter workspace exists for the given repository
func (w *WorkspacesController) GetIPythonWorkspace(repoID string) error {
	// This would check if an IPython workspace exists for the repository
	// For now, we just check if the service is running
	if !services.IPython.IsRunning() {
		return errors.New("IPython service is not running")
	}
	return nil
}