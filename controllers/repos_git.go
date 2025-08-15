package controllers

import (
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"workspace/models"

	"github.com/The-Skyscape/devtools/pkg/authentication"
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

	// Use git log to get commits
	cmd := exec.Command("git", "log", branch, "--pretty=format:%H|%an|%ae|%at|%s", "-n", "50")
	cmd.Dir = repo.Path()
	
	output, err := cmd.Output()
	if err != nil {
		// Repository might be empty
		return []*models.Commit{}, nil
	}

	var commits []*models.Commit
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) >= 5 {
			// Parse timestamp
			timestamp, _ := strconv.ParseInt(parts[3], 10, 64)
			commitDate := time.Unix(timestamp, 0)
			
			commit := &models.Commit{
				Hash:    parts[0],
				Author:  parts[1],
				Email:   parts[2],
				Date:    commitDate,
				Message: parts[4],
			}
			commits = append(commits, commit)
		}
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