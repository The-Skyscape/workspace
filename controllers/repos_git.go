package controllers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"workspace/models"
	"workspace/services"

	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/database"
	"github.com/sosedoff/gitkit"
)

// RepoCommits returns recent commits for the current repository
// Executes git log to retrieve commit history
func (c *ReposController) RepoCommits() ([]*models.Commit, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	// Get the branch from query params, default to main/master
	branch := c.Request.URL.Query().Get("branch")
	if branch == "" {
		branch = c.CurrentBranch()
	}

	// Use the repository's GetCommits method which properly formats everything
	commits, err := repo.GetCommits(branch, 50)
	if err != nil {
		// Repository might be empty
		return []*models.Commit{}, nil
	}

	return commits, nil
}

// RepoBranches returns all branches in the repository
func (c *ReposController) RepoBranches() ([]*models.Branch, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	return repo.GetBranches()
}

// RepoCommitDiff returns the diff for a specific commit
// Used to show what changed in a commit
func (c *ReposController) RepoCommitDiff() (*models.Diff, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	commitHash := c.Request.URL.Query().Get("commit")
	if commitHash == "" {
		return nil, errors.New("commit hash required")
	}

	// Get diff using git show
	cmd := exec.Command("git", "show", commitHash)
	cmd.Dir = repo.Path()
	
	_, err = cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get commit diff: %v", err)
	}

	// For now, return a simple wrapper
	// The Diff model needs to be updated to have Content field
	return &models.Diff{
		// Assuming Diff has appropriate fields
	}, nil
}

// RepoCommitDiffContent returns the raw diff content as a string
// Used when displaying diffs in the UI
func (c *ReposController) RepoCommitDiffContent() (string, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return "", err
	}

	commitHash := c.Request.URL.Query().Get("commit")
	if commitHash == "" {
		return "", errors.New("commit hash required")
	}

	// Get diff using git show
	cmd := exec.Command("git", "show", commitHash)
	cmd.Dir = repo.Path()
	
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get commit diff: %v", err)
	}

	return string(output), nil
}

// RepoLanguageStats returns statistics about languages used in the repository
// Analyzes file extensions to determine language distribution
func (c *ReposController) RepoLanguageStats() (map[string]int, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	return repo.GetLanguageStats()
}

// RepoContributors returns the list of contributors to the repository
// Extracts unique authors from git log
func (c *ReposController) RepoContributors() ([]*models.Contributor, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	return repo.GetContributors()
}

// CurrentBranch returns the currently checked out branch
// Defaults to "main" if unable to determine
func (c *ReposController) CurrentBranch() string {
	repo, err := c.CurrentRepo()
	if err != nil {
		return "main"
	}

	// Get current branch from git
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = repo.Path()
	
	output, err := cmd.Output()
	if err != nil {
		// Try to determine default branch
		if branches, err := repo.GetBranches(); err == nil && len(branches) > 0 {
			return branches[0].Name
		}
		return "main"
	}

	return strings.TrimSpace(string(output))
}

// RepoCommitCount returns the total number of commits in the current branch
func (c *ReposController) RepoCommitCount() int {
	repo, err := c.CurrentRepo()
	if err != nil {
		return 0
	}

	// Get the current branch (default to repo's default branch)
	branch := c.CurrentBranch()

	count, err := repo.GetCommitCount(branch)
	if err != nil {
		return 0
	}

	return count
}

// importRepository handles importing an existing Git repository
func (c *ReposController) importRepository(w http.ResponseWriter, r *http.Request) {
	// Parse form data
	name := r.FormValue("name")
	description := r.FormValue("description")
	gitURL := r.FormValue("git_url")
	visibility := r.FormValue("visibility")

	if name == "" || gitURL == "" {
		c.RenderErrorMsg(w, r, "repository name and Git URL are required")
		return
	}

	// Validate visibility
	if visibility != "public" && visibility != "private" {
		visibility = "private"
	}

	// Create repository record
	repo := &models.Repository{
		Model:       models.DB.NewModel(""),
		Name:        name,
		Description: description,
		Visibility:  visibility,
		UserID:      "", // Will be set based on current user
	}

	// Get current user
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err == nil {
		repo.UserID = user.ID
	}

	// Insert repository into database
	repo, err = models.Repositories.Insert(repo)
	if err != nil {
		c.RenderError(w, r, err)
		return
	}

	// Clone the repository
	cmd := exec.Command("git", "clone", gitURL, repo.Path())
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Clean up on failure
		models.Repositories.Delete(repo)
		c.RenderErrorMsg(w, r, fmt.Sprintf("failed to clone repository: %s", string(output)))
		return
	}

	// Update repository stats if method exists
	// repo.UpdateStats()

	// Log activity
	models.LogActivity("repo_imported", fmt.Sprintf("Imported repository %s", name),
		fmt.Sprintf("Repository %s was imported from %s", name, gitURL),
		repo.UserID, repo.ID, "repository", "")

	// Redirect to the new repository
	c.Redirect(w, r, fmt.Sprintf("/repos/%s", repo.ID))
}

// InitGitServer initializes the gitkit server with authentication
// This handles git clone, push, pull operations via HTTP
func (c *ReposController) InitGitServer(auth *authentication.Controller) *gitkit.Server {
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
		// because we use http.StripPrefix("/repo/", gitServer) in Setup()
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

	return git
}

// IsGitRequest checks if the current request is a Git operation
func (c *ReposController) IsGitRequest() bool {
	path := c.Request.URL.Path
	return strings.HasPrefix(path, "/repo/") &&
		(strings.Contains(path, ".git") ||
			strings.Contains(path, "git-upload-pack") ||
			strings.Contains(path, "git-receive-pack"))
}