package models

import (
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/database"
	"github.com/sosedoff/gitkit"
)

// Repository operations

func NewRepo(repoID, name string) (repo *GitRepo, err error) {
	// Create model with custom ID if provided, otherwise generate one
	var model database.Model
	if repoID == "" {
		model = DB.NewModel("")
	} else {
		model = DB.NewModel(repoID)
	}
	
	repo, err = GitRepos.Insert(&GitRepo{
		Model:      model,
		Name:       name,
		Visibility: "private",
		UserID:     "", // Will be set by the caller
	})
	if err != nil {
		return nil, err
	}

	if err = os.Mkdir(repo.Path(), 0755); err != nil {
		return nil, err
	}

	_, _, err = repo.Run("init", "--bare")
	return repo, err
}

// Repository operations are available directly via GitRepos collection:
// - GitRepos.Get(id)
// - GitRepos.Search(query, args...)
// - GitRepos.Update(repo)


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

		if _, err := GetAccessToken(creds.Username, creds.Password); err == nil {
			return true, nil
		}

		if user, err := auth.GetUser(creds.Username); err != nil {
			return false, errors.New("invalid username or password")
		} else if ok := user.VerifyPassword(creds.Password); !ok {
			return false, errors.New("invalid username or password")
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
