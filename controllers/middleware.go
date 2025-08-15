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
			// Not authenticated - redirect to signin
			http.Redirect(w, r, "/signin", http.StatusSeeOther)
			return false
		}
		
		if !user.IsAdmin {
			// Not admin - show error
			RenderErrorMessage(w, r, "Admin access required")
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
			RenderErrorMessage(w, r, "Repository ID required")
			return false
		}
		
		repo, err := models.Repositories.Get(repoID)
		if err != nil {
			RenderErrorMessage(w, r, "Repository not found")
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
			RenderErrorMessage(w, r, "Access denied - private repository")
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
			http.Redirect(w, r, "/signin", http.StatusSeeOther)
			return false
		}
		
		// Admins can do anything
		if user.IsAdmin {
			return true
		}
		
		// Check if user is the author
		authorID, err := getAuthorID(r)
		if err != nil {
			RenderErrorMessage(w, r, "Resource not found")
			return false
		}
		
		if user.ID == authorID {
			return true
		}
		
		RenderErrorMessage(w, r, "Permission denied - you can only modify your own content")
		return false
	}
}

// PublicRepoOnly - AccessCheck that allows authenticated users on public repos, admins on any
func PublicRepoOnly() application.AccessCheck {
	return func(app *application.App, w http.ResponseWriter, r *http.Request) bool {
		auth := app.Use("auth").(*authentication.Controller)
		user, _, err := auth.Authenticate(r)
		if err != nil {
			http.Redirect(w, r, "/signin", http.StatusSeeOther)
			return false
		}
		
		repoID := r.PathValue("id")
		if repoID == "" {
			RenderErrorMessage(w, r, "Repository ID required")
			return false
		}
		
		repo, err := models.Repositories.Get(repoID)
		if err != nil {
			RenderErrorMessage(w, r, "Repository not found")
			return false
		}
		
		// Admins can access any repo
		if user.IsAdmin {
			return true
		}
		
		// Non-admins can only access public repos
		if repo.Visibility != "public" {
			RenderErrorMessage(w, r, "Access denied - private repository")
			return false
		}
		
		return true
	}
}

// RenderErrorMessage is a helper to render error messages consistently
func RenderErrorMessage(w http.ResponseWriter, r *http.Request, message string) {
	// Create a simple BaseController to use its RenderErrorMsg method
	bc := &application.BaseController{Request: r}
	bc.RenderErrorMsg(w, r, message)
}