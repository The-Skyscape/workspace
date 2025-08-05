package models

import (
	"errors"
	"net/http"
	"workspace/services"

	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/containers"
)

// CoderHandler returns an HTTP handler for the global coder service (admin only)
func CoderHandler(auth *authentication.Controller) http.Handler {
	return auth.ProtectFunc(func(w http.ResponseWriter, r *http.Request) {
		// Authenticate user
		u, _, err := auth.Authenticate(r)
		if err != nil {
			auth.Render(w, r, "error-message", err)
			return
		}

		// Check if user is admin
		if !u.IsAdmin {
			auth.Render(w, r, "error-message", errors.New("admin access required"))
			return
		}

		// Check if coder service is running
		if !services.Coder.IsRunning() {
			auth.Render(w, r, "error-message", errors.New("coder service is not running"))
			return
		}

		// Strip the /coder/ prefix from the path
		r.URL.Path = services.Coder.StripProxyPath(r.URL.Path)

		// Get the service and proxy the request
		service := &containers.Service{
			Host: containers.Local(),
			Name: "skyscape-coder",
		}
		
		proxy := service.Proxy(services.Coder.GetPort())
		proxy.ServeHTTP(w, r)
	}, false)
}