package controllers

import (
	"errors"
	"log"
	"net/http"
	"workspace/services"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/containers"
)

// Coder is a factory function that returns the URL prefix and controller instance.
func Coder() (string, *CoderController) {
	return "coder", &CoderController{}
}

// CoderController handles Code IDE management and coder service proxy
type CoderController struct {
	application.BaseController
}

// Setup registers all routes for Code IDE management.
func (c *CoderController) Setup(app *application.App) {
	c.BaseController.Setup(app)

	auth := app.Use("auth").(*authentication.Controller)
	
	// Register coder proxy handler for Code IDE (admin only)
	http.Handle("/coder/", http.StripPrefix("/coder/", 
		auth.ProtectFunc(c.proxyToCodeServer, true)))

	// Initialize the coder service on startup
	if err := services.Coder.Init(); err != nil {
		// Log the error but don't fail startup
		// The service can be started manually if needed
		log.Printf("Warning: Failed to initialize coder service: %v", err)
	}
}

// Handle is called when each request is handled
func (c *CoderController) Handle(req *http.Request) application.Controller {
	c.Request = req
	return c
}

// proxyToCodeServer handles requests to the code-server instance
func (c *CoderController) proxyToCodeServer(w http.ResponseWriter, r *http.Request) {
	if !services.Coder.IsRunning() {
		c.Render(w, r, "error-message.html", errors.New("coder service is not running"))
		return
	}

	// Path is already stripped by http.StripPrefix
	service := &containers.Service{
		Host: containers.Local(),
		Name: "skyscape-coder",
	}
	
	proxy := service.Proxy(services.Coder.GetPort())
	proxy.ServeHTTP(w, r)
}

// CoderStatus returns the status of the global coder service
func (c *CoderController) CoderStatus() *services.CoderStatus {
	return services.Coder.GetStatus()
}