package models

import (
	"errors"
	"log"
	"net/http"
	"path/filepath"

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

		// Check if it's token-based auth (using token ID as username, token value as password)
		token, err := AccessTokens.Get(creds.Username)
		if err == nil && token != nil && token.Token == creds.Password {
			// Token matches, allow access
			log.Printf("Token auth successful - ID: %s", creds.Username)
			return true, nil
		}

		// Fall back to username/password authentication
		if user, err := auth.GetUser(creds.Username); err != nil {
			return false, errors.New("invalid username or password")
		} else if ok := user.VerifyPassword(creds.Password); !ok {
			return false, errors.New("invalid username or password")
		}

		log.Printf("User auth successful for %s", creds.Username)
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
