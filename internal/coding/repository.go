package coding

import (
	"errors"
	"log"
	"net/http"
	"path/filepath"

	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/database"

	"github.com/sosedoff/gitkit"
)

type Repository struct {
	db     *database.DynamicDB
	repos  *database.Collection[*GitRepo]
	spaces *database.Collection[*Workspace]
	tokens *database.Collection[*AccessToken]
}

func Manage(db *database.DynamicDB) *Repository {
	return &Repository{
		db:     db,
		repos:  database.Manage(db, new(GitRepo)),
		spaces: database.Manage(db, new(Workspace)),
		tokens: database.Manage(db, new(AccessToken)),
	}
}

func (repo *Repository) GitServer(auth *authentication.Controller) http.Handler {
	git := gitkit.New(gitkit.Config{
		Dir:        filepath.Join(database.DataDir(), "repos"),
		AutoCreate: true,
		Auth:       true,
	})

	git.AuthFunc = func(creds gitkit.Credential, req *gitkit.Request) (bool, error) {
		if creds.Username == "" || creds.Password == "" {
			return false, nil
		}

		if _, err := repo.GetAccessToken(creds.Username, creds.Password); err == nil {
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

func (repo *Repository) Workspace(auth *authentication.Controller) http.Handler {
	return auth.ProtectFunc(func(w http.ResponseWriter, r *http.Request) {
		u, _, err := auth.Authenticate(r)
		if err != nil {
			auth.Render(w, r, "error-message", err)
			return
		}

		ws, err := repo.GetWorkspace(u.ID)
		if ws == nil || err != nil {
			others, _ := repo.Workspaces()
			port := 8000 + len(others)
			ws, err = repo.NewWorkspace(u.ID, port, nil)
			if err != nil {
				auth.Render(w, r, "error-message", err)
				return
			}

			go func() {
				if err := ws.Start(u); err != nil {
					log.Println("Failed to start workspace:", err)
					return
				}
			}()
		}

		if !ws.Ready {
			auth.Render(w, r, "loading.html", nil)
			return
		}

		s := ws.Service()
		s.Proxy(8080).ServeHTTP(w, r)
	}, false)
}

// GetRepos returns the repos collection for direct access
func (r *Repository) GetRepos() *database.Collection[*GitRepo] {
	return r.repos
}

// UpdateRepo updates an existing repository
func (r *Repository) UpdateRepo(repo *GitRepo) error {
	return r.repos.Update(repo)
}
