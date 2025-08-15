package controllers

import (
	"errors"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"workspace/models"
	"workspace/services"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/database"
	"github.com/sosedoff/gitkit"
)

// Git is a factory function that returns the controller name and instance
func Git() (string, *GitController) {
	return "git", &GitController{}
}

// GitController handles Git server operations (clone, push, pull)
type GitController struct {
	application.BaseController
	gitServer *gitkit.Server
}

// Setup initializes the Git server and registers routes
func (c *GitController) Setup(app *application.App) {
	c.BaseController.Setup(app)

	auth := app.Use("auth").(*authentication.Controller)

	// Initialize Git server
	c.initGitServer(auth)

	// Register Git HTTP endpoints
	// These handle git clone, push, pull operations
	http.Handle("/repo/", http.StripPrefix("/repo/", c.gitServer))
}

// Handle prepares the controller for the current request
func (c *GitController) Handle(req *http.Request) application.Controller {
	c.Request = req
	return c
}

// initGitServer initializes the gitkit server with authentication
func (c *GitController) initGitServer(auth *authentication.Controller) {
	git := gitkit.New(gitkit.Config{
		Dir:        filepath.Join(database.DataDir(), "repos"),
		AutoCreate: true,
		Auth:       true,
	})

	git.AuthFunc = func(creds gitkit.Credential, req *gitkit.Request) (bool, error) {
		if creds.Username == "" || creds.Password == "" {
			return false, nil
		}

		// First authenticate the user
		var user *authentication.User

		// Check if it's token-based auth (using token ID as username, token value as password)
		token, err := models.AccessTokens.Get(creds.Username)
		if err == nil && token != nil && token.Token == creds.Password {
			// Token matches, get the user associated with the token
			user, err = auth.Users.Get(token.UserID)
			if err != nil {
				return false, errors.New("invalid token user")
			}
			log.Printf("Token auth successful - ID: %s", creds.Username)
		} else {
			// Fall back to username/password authentication
			user, err = auth.GetUser(creds.Username)
			if err != nil {
				return false, errors.New("invalid username or password")
			}
			if !user.VerifyPassword(creds.Password) {
				return false, errors.New("invalid username or password")
			}
			log.Printf("User auth successful for %s", creds.Username)
		}

		// Now check repository access based on operation
		// IMPORTANT: req.RepoName contains just the repository ID (e.g., "test", "bang")
		// because we use http.StripPrefix("/repo/", c.gitServer) in Setup()
		// The operation is determined from the URL: "git-upload-pack" (pull/clone) or "git-receive-pack" (push)
		repoID := req.RepoName

		// Get the repository to check visibility
		repo, err := models.Repositories.Get(repoID)
		if err != nil {
			log.Printf("Repository not found: %s", repoID)
			return false, errors.New("repository not found")
		}

		// Check if this is a push or pull operation based on the URL
		isPush := strings.Contains(req.Request.URL.Path, "git-receive-pack") ||
			strings.Contains(req.Request.URL.Query().Get("service"), "git-receive-pack")
		isPull := strings.Contains(req.Request.URL.Path, "git-upload-pack") ||
			strings.Contains(req.Request.URL.Query().Get("service"), "git-upload-pack")

		// Check access based on operation
		if isPush {
			// Push operation - admin only
			if !user.IsAdmin {
				log.Printf("Push denied for non-admin user %s to repo %s", user.Email, repoID)
				return false, errors.New("only admins can push to repositories")
			}

			// Schedule a workspace update after the push completes
			// We do this in a goroutine to not block the Git operation
			go func() {
				// Wait a moment for the push to complete
				time.Sleep(2 * time.Second)

				// Update the working copy in Code Server
				if err := services.Coder.UpdateRepository(repoID); err != nil {
					log.Printf("Failed to update repository in Code Server after push: %v", err)
				}
			}()
		} else if isPull {
			// Pull/clone operation - check repository visibility
			if repo.Visibility != "public" && !user.IsAdmin {
				log.Printf("Pull denied for non-admin user %s to private repo %s", user.Email, repoID)
				return false, errors.New("access denied - private repository")
			}
		}

		return true, nil
	}

	if err := git.Setup(); err != nil {
		log.Fatal("Failed to setup git server: ", err)
	}

	c.gitServer = git
}

// IsGitRequest checks if the current request is a Git operation
func (c *GitController) IsGitRequest() bool {
	path := c.Request.URL.Path
	return strings.HasPrefix(path, "/repo/") &&
		(strings.Contains(path, ".git") ||
			strings.Contains(path, "git-upload-pack") ||
			strings.Contains(path, "git-receive-pack"))
}
