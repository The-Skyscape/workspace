package models

import (
	"cmp"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

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

// Workspace operations

func NewWorkspace(userID string, port int, repo *GitRepo) (*Workspace, error) {
	now := time.Now()
	workspace := &Workspace{
		Name:      fmt.Sprintf("workspace-%d", now.Unix()),
		UserID:    userID,
		Port:      port,
		Ready:     false,
		CreatedAt: now,
		LastUsed:  now,
	}
	if repo != nil {
		workspace.RepoID = repo.ID
		workspace.Name = fmt.Sprintf("%s-%d", repo.Name, now.Unix())
	}

	return Workspaces.Insert(workspace)
}

// GetWorkspace returns the most recently used workspace for a user
func GetWorkspace(userID string) (*Workspace, error) {
	w, err := Workspaces.Search(`WHERE UserID = ? ORDER BY LastUsed DESC LIMIT 1`, userID)
	if err != nil || len(w) == 0 {
		return nil, cmp.Or(err, errors.New("workspace not found"))
	}

	return w[0], nil
}

// GetWorkspaceByID returns a specific workspace by ID
func GetWorkspaceByID(id string) (*Workspace, error) {
	return Workspaces.Get(id)
}

// GetUserWorkspaces returns all workspaces for a user
func GetUserWorkspaces(userID string) ([]*Workspace, error) {
	return Workspaces.Search("WHERE UserID = ? ORDER BY CreatedAt DESC", userID)
}

func GetWorkspaces() ([]*Workspace, error) {
	return Workspaces.Search(``)
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

// Workspace HTTP handler

func WorkspaceHandler(auth *authentication.Controller) http.Handler {
	return auth.ProtectFunc(func(w http.ResponseWriter, r *http.Request) {
		u, _, err := auth.Authenticate(r)
		if err != nil {
			auth.Render(w, r, "error-message", err)
			return
		}

		// Extract workspace ID from path or query params
		path := r.URL.Path
		wsID := ""
		
		// Check if workspace ID is in the path (e.g., /coder/workspace-id/...)
		parts := strings.Split(strings.TrimPrefix(path, "/coder/"), "/")
		if len(parts) > 0 && strings.HasPrefix(parts[0], "workspace-") {
			wsID = parts[0]
			// Update path for proxy
			r.URL.Path = "/" + strings.Join(parts[1:], "/")
		} else if id := r.URL.Query().Get("workspace"); id != "" {
			wsID = id
		}

		var ws *Workspace
		if wsID != "" {
			// Get specific workspace by ID
			ws, err = GetWorkspaceByID(wsID)
			if err != nil || ws == nil || ws.UserID != u.ID {
				auth.Render(w, r, "error-message", errors.New("workspace not found or access denied"))
				return
			}
		} else {
			// No workspace ID provided - redirect to workspace list
			http.Redirect(w, r, "/workspaces", http.StatusFound)
			return
		}

		// Start workspace if not ready
		if !ws.Ready || ws.GetStatus() != "running" {
			go func() {
				if err := ws.Start(u, nil); err != nil {
					log.Println("Failed to start workspace:", err)
					return
				}
			}()
			
			// Show loading page
			auth.Render(w, r, "loading.html", map[string]any{
				"WorkspaceID": ws.ID,
				"Message": "Starting workspace...",
			})
			return
		}

		// Update last used timestamp
		ws.LastUsed = time.Now()
		Workspaces.Update(ws)

		// Proxy to code-server
		s := ws.Service()
		s.Proxy(ws.Port).ServeHTTP(w, r)
	}, false)
}

// Helper to get repo for a workspace
func (w *Workspace) Repo() (*GitRepo, error) {
	if w.RepoID == "" {
		return nil, nil
	}
	return GitRepos.Get(w.RepoID)
}

// Access token helpers
