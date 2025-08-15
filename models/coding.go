package models

import (
	"errors"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/database"
	"github.com/sosedoff/gitkit"
)

// Repository operations - kept for backward compatibility
// New code should use CreateRepository from repository.go instead

func NewRepo(repoID, name string) (repo *Repository, err error) {
	// This function is deprecated - use CreateRepository instead
	// Creating repository with empty description, private visibility, and no user
	return CreateRepository(name, "", "private", "")
}


// Git server HTTP handler

func GitServer(auth *authentication.Controller) http.Handler {
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
		token, err := AccessTokens.Get(creds.Username)
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
		// req.RepoPath contains the repository path being accessed
		// The operation is determined from the URL: "git-upload-pack" (pull/clone) or "git-receive-pack" (push)
		
		// Extract repository ID from path (format: /repo/{id}.git or /repo/{id})
		repoPath := strings.TrimPrefix(req.RepoPath, "/")
		repoPath = strings.TrimSuffix(repoPath, ".git")
		repoID := strings.TrimPrefix(repoPath, "repo/")
		
		// Get the repository to check visibility
		repo, err := Repositories.Get(repoID)
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

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		git.ServeHTTP(w, r)
	})
}


// Access token helpers