package controllers

import (
	"log"
	"net/http"
	"workspace/models"
	"workspace/services"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
)

// Coder is a factory function that returns the URL prefix and controller instance.
func Coder() (string, *CoderController) {
	return "coder", &CoderController{}
}

// CoderController handles Code IDE management and coder service proxy
type CoderController struct {
	application.BaseController
}

// CoderStatus represents the status of the coder service
type CoderStatus struct {
	Running   bool
	Port      int
	AdminOnly bool
	URL       string
}

// Setup registers all routes for Code IDE management.
func (c *CoderController) Setup(app *application.App) {
	c.BaseController.Setup(app)

	auth := app.Use("auth").(*authentication.Controller)
	
	// Register coder proxy handler for Code IDE
	http.Handle("/coder/", http.StripPrefix("/coder/", models.CoderHandler(auth)))

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

// CoderStatus returns the status of the global coder service
func (c *CoderController) CoderStatus() *CoderStatus {
	return &CoderStatus{
		Running:   services.Coder.IsRunning(),
		Port:      services.Coder.GetPort(),
		AdminOnly: services.Coder.IsAdminOnly(),
		URL:       "/coder/",
	}
}