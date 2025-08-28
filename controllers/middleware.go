package controllers

import (
	"net/http"

	"workspace/models"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
)

// AdminOnly - AccessCheck that requires admin user
func AdminOnly() application.AccessCheck {
	return func(app *application.App, w http.ResponseWriter, r *http.Request) bool {
		auth := app.Use("auth").(*authentication.Controller)
		user, _, err := auth.Authenticate(r)
		if err != nil {
			// Not authenticated - render signin page in place
			app.Render(w, r, "signin.html", nil)
			return false
		}
		
		if !user.IsAdmin {
			// Authenticated but not admin - show insufficient permissions
			app.Render(w, r, "insufficient-permissions.html", nil)
			return false
		}
		
		return true
	}
}

// PublicOrAdmin - AccessCheck for public repos or admin access
func PublicOrAdmin() application.AccessCheck {
	return func(app *application.App, w http.ResponseWriter, r *http.Request) bool {
		repoID := r.PathValue("id")
		if repoID == "" {
			app.Render(w, r, "signin.html", nil)
			return false
		}
		
		repo, err := models.Repositories.Get(repoID)
		if err != nil {
			app.Render(w, r, "error-404.html", nil)
			return false
		}
		
		// Public repos are accessible to all
		if repo.Visibility == "public" {
			return true
		}
		
		// Private repos require admin
		auth := app.Use("auth").(*authentication.Controller)
		user, _, err := auth.Authenticate(r)
		if err != nil || !user.IsAdmin {
			app.Render(w, r, "signin.html", nil)
			return false
		}
		
		return true
	}
}

// AuthorOrAdmin - AccessCheck for resource author or admin
func AuthorOrAdmin(getAuthorID func(r *http.Request) (string, error)) application.AccessCheck {
	return func(app *application.App, w http.ResponseWriter, r *http.Request) bool {
		auth := app.Use("auth").(*authentication.Controller)
		user, _, err := auth.Authenticate(r)
		if err != nil {
			app.Render(w, r, "signin.html", nil)
			return false
		}
		
		// Admins can do anything
		if user.IsAdmin {
			return true
		}
		
		// Check if user is the author
		authorID, err := getAuthorID(r)
		if err != nil {
			app.Render(w, r, "error-404.html", nil)
			return false
		}
		
		if user.ID == authorID {
			return true
		}
		
		// User is authenticated but not author or admin
		app.Render(w, r, "insufficient-permissions.html", nil)
		return false
	}
}

// PublicRepoOnly - AccessCheck that allows authenticated users on public repos, admins on any
func PublicRepoOnly() application.AccessCheck {
	return func(app *application.App, w http.ResponseWriter, r *http.Request) bool {
		auth := app.Use("auth").(*authentication.Controller)
		user, _, err := auth.Authenticate(r)
		if err != nil {
			app.Render(w, r, "signin.html", nil)
			return false
		}
		
		repoID := r.PathValue("id")
		if repoID == "" {
			app.Render(w, r, "error-404.html", nil)
			return false
		}
		
		repo, err := models.Repositories.Get(repoID)
		if err != nil {
			app.Render(w, r, "error-404.html", nil)
			return false
		}
		
		// Admins can access any repo
		if user.IsAdmin {
			return true
		}
		
		// Non-admins can only access public repos
		if repo.Visibility != "public" {
			app.Render(w, r, "insufficient-permissions.html", nil)
			return false
		}
		
		return true
	}
}