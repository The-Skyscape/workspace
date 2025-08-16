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
)

// IPython is a factory function that returns the URL prefix and controller instance.
func IPython() (string, *IPythonController) {
	return "ipython", &IPythonController{}
}

// IPythonController handles Jupyter notebook management and service proxy
type IPythonController struct {
	application.BaseController
}

// Setup registers all routes for Jupyter notebook management.
func (c *IPythonController) Setup(app *application.App) {
	c.BaseController.Setup(app)

	auth := app.Use("auth").(*authentication.Controller)
	
	// Register IPython/Jupyter proxy handler (admin only)
	// Don't strip prefix - Jupyter is configured with base_url=/ipython/
	// The trailing slash makes this a catch-all for /ipython/* paths
	http.Handle("/ipython/", auth.ProtectFunc(c.proxyToJupyter, true))

	// Initialize the IPython service on startup in background
	go func() {
		if err := services.IPython.Init(); err != nil {
			log.Printf("Warning: Failed to initialize IPython service: %v", err)
		}
	}()
}

// Handle is called when each request is handled
func (c IPythonController) Handle(req *http.Request) application.Controller {
	c.Request = req
	return &c
}

// proxyToJupyter handles requests to the Jupyter notebook instance
func (c *IPythonController) proxyToJupyter(w http.ResponseWriter, r *http.Request) {
	if !services.IPython.IsRunning() {
		c.Render(w, r, "error-message.html", errors.New("IPython/Jupyter service is not running"))
		return
	}

	// Create reverse proxy
	target, err := url.Parse(fmt.Sprintf("http://localhost:%d", services.IPython.GetPort()))
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to create proxy"))
		return
	}
	
	proxy := httputil.NewSingleHostReverseProxy(target)
	
	// Jupyter is configured with base_url=/ipython/ and expects the full path
	// The request path already includes /ipython/ prefix since we don't strip it
	proxy.ServeHTTP(w, r)
}

// IPythonStatus returns the status of the global IPython service
func (c *IPythonController) IPythonStatus() *services.IPythonStatus {
	return services.IPython.GetStatus()
}