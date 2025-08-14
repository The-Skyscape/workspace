package controllers

import (
	"net/http"

	"workspace/models"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
)

// AdminRequired checks if the current user is an admin
func AdminRequired(app *application.App, w http.ResponseWriter, r *http.Request) bool {
	auth := app.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		// Not authenticated - redirect to signin
		http.Redirect(w, r, "/signin", http.StatusSeeOther)
		return false
	}

	if !models.IsUserAdmin(user) {
		// Not admin - show error
		RenderErrorMessage(w, r, "Admin access required")
		return false
	}

	return true
}

// AuthRequired checks if the user is authenticated
func AuthRequired(app *application.App, w http.ResponseWriter, r *http.Request) bool {
	auth := app.Use("auth").(*authentication.Controller)
	_, _, err := auth.Authenticate(r)
	if err != nil {
		// Not authenticated - redirect to signin
		http.Redirect(w, r, "/signin", http.StatusSeeOther)
		return false
	}
	return true
}

// RepoReadRequired returns a middleware that checks read access to a repository
func RepoReadRequired() func(*application.App, http.ResponseWriter, *http.Request) bool {
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

		// Check repository access
		err = models.CheckRepoAccess(user, repoID, models.RoleRead)
		if err != nil {
			RenderErrorMessage(w, r, "Insufficient permissions to access this repository")
			return false
		}

		return true
	}
}

// RepoWriteRequired returns a middleware that checks write access to a repository
func RepoWriteRequired() func(*application.App, http.ResponseWriter, *http.Request) bool {
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

		// Check repository write access
		err = models.CheckRepoAccess(user, repoID, models.RoleWrite)
		if err != nil {
			RenderErrorMessage(w, r, "Insufficient permissions to modify this repository")
			return false
		}

		return true
	}
}

// RepoAdminRequired returns a middleware that checks admin access to a repository
func RepoAdminRequired() func(*application.App, http.ResponseWriter, *http.Request) bool {
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

		// Check repository admin access
		err = models.CheckRepoAccess(user, repoID, models.RoleAdmin)
		if err != nil {
			RenderErrorMessage(w, r, "Admin permissions required for this repository")
			return false
		}

		return true
	}
}

// RepoOwnerRequired returns a middleware that checks if user owns the repository
func RepoOwnerRequired() func(*application.App, http.ResponseWriter, *http.Request) bool {
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

		// Get repository to check ownership
		repo, err := models.Repositories.Get(repoID)
		if err != nil {
			RenderErrorMessage(w, r, "Repository not found")
			return false
		}

		// Check if user is the owner or an admin
		if repo.UserID != user.ID && !models.IsUserAdmin(user) {
			RenderErrorMessage(w, r, "Only the repository owner can perform this action")
			return false
		}

		return true
	}
}

// CanCreateRepoRequired checks if user can create repositories
func CanCreateRepoRequired(app *application.App, w http.ResponseWriter, r *http.Request) bool {
	auth := app.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		http.Redirect(w, r, "/signin", http.StatusSeeOther)
		return false
	}

	if !models.CanUserCreateRepo(user) {
		RenderErrorMessage(w, r, "Only administrators can create repositories")
		return false
	}

	return true
}

// RenderErrorMessage is a helper to render error messages consistently
func RenderErrorMessage(w http.ResponseWriter, r *http.Request, message string) {
	// Create a simple BaseController to use its RenderErrorMsg method
	bc := &application.BaseController{Request: r}
	bc.RenderErrorMsg(w, r, message)
}