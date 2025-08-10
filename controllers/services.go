package controllers

import (
	"errors"
	"log"
	"net/http"
	"workspace/models"
	"workspace/services"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
)

// Services is a factory function that returns the URL prefix and controller instance.
func Services() (string, *ServicesController) {
	return "services", &ServicesController{}
}

// ServicesController handles global service management (admin only)
type ServicesController struct {
	application.BaseController
}

// Setup registers all routes for service management.
// Routes:
//   GET  /services          - List all services and their status (admin only)
//   POST /services/coder/start - Start the global coder service (admin only)
//   POST /services/coder/stop  - Stop the global coder service (admin only)
//   /coder/*                - Proxy to coder service (admin only)
func (c *ServicesController) Setup(app *application.App) {
	c.BaseController.Setup(app)

	auth := app.Use("auth").(*authentication.Controller)
	http.Handle("GET /services", app.Serve("services.html", auth.Required))
	http.Handle("POST /services/coder/start", app.ProtectFunc(c.startCoder, auth.Required))
	http.Handle("POST /services/coder/stop", app.ProtectFunc(c.stopCoder, auth.Required))
	
	// Register coder proxy handler
	http.Handle("/coder/", http.StripPrefix("/coder/", models.CoderHandler(auth)))

	// Initialize the coder service on startup
	if err := services.Coder.Init(); err != nil {
		// Log the error but don't fail startup
		// The service can be started manually from the UI
		log.Printf("Warning: Failed to initialize coder service: %v", err)
	}
}

// Handle is called when each request is handled
func (c *ServicesController) Handle(req *http.Request) application.Controller {
	c.Request = req
	return c
}

// CoderStatus returns the status of the global coder service
func (c *ServicesController) CoderStatus() map[string]interface{} {
	return map[string]interface{}{
		"running":   services.Coder.IsRunning(),
		"port":      services.Coder.GetPort(),
		"adminOnly": services.Coder.IsAdminOnly(),
		"url":       "/coder/",
	}
}

// startCoder starts the global coder service
func (c *ServicesController) startCoder(w http.ResponseWriter, r *http.Request) {
	// Additional admin check
	auth := c.App.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil || !user.IsAdmin {
		c.Render(w, r, "error-message.html", errors.New("admin access required"))
		return
	}

	if err := services.Coder.Start(); err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	c.Refresh(w, r)
}

// stopCoder stops the global coder service
func (c *ServicesController) stopCoder(w http.ResponseWriter, r *http.Request) {
	// Additional admin check
	auth := c.App.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil || !user.IsAdmin {
		c.Render(w, r, "error-message.html", errors.New("admin access required"))
		return
	}

	if err := services.Coder.Stop(); err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	c.Refresh(w, r)
}