package controllers

import (
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"workspace/models"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
)

// Repos is a factory function with the prefix and instance
func Repos() (string, *ReposController) {
	return "repos", &ReposController{}
}

// ReposController handles repository management
type ReposController struct {
	application.BaseController
}

// Setup is called when the application is started
func (c *ReposController) Setup(app *application.App) {
	c.BaseController.Setup(app)

	auth := app.Use("auth").(*authentication.Controller)
	http.Handle("GET /repos", app.Serve("repos-list.html", auth.Required))
	http.Handle("GET /repos/{id}", app.Serve("repo-view.html", auth.Required))
	http.Handle("GET /repos/{id}/issues", app.Serve("repo-issues.html", auth.Required))
	http.Handle("GET /repos/{id}/prs", app.Serve("repo-prs.html", auth.Required))
	http.Handle("GET /repos/{id}/actions", app.Serve("repo-actions.html", auth.Required))
	http.Handle("GET /repos/{id}/settings", app.Serve("repo-settings.html", auth.Required))
	http.Handle("GET /repos/{id}/files", app.Serve("repo-files.html", auth.Required))
	http.Handle("GET /repos/{id}/files/{path...}", app.Serve("repo-file-view.html", auth.Required))
	http.Handle("GET /repos/{id}/edit/{path...}", app.Serve("repo-file-edit.html", auth.Required))
	http.Handle("GET /repos/{id}/commits", app.Serve("repo-commits.html", auth.Required))
	http.Handle("GET /repos/{id}/commits/{hash}/diff", app.Serve("repo-commit-diff.html", auth.Required))
	http.Handle("GET /repos/{id}/prs/{prID}/diff", app.Serve("repo-pr-diff.html", auth.Required))
	http.Handle("GET /repos/{id}/search", app.Serve("repo-search.html", auth.Required))

	http.Handle("POST /repos/create", app.ProtectFunc(c.createRepo, auth.Required))
	http.Handle("POST /repos/{id}/actions/create", app.ProtectFunc(c.createAction, auth.Required))
	http.Handle("POST /repos/{id}/actions/{actionID}/run", app.ProtectFunc(c.runAction, auth.Required))
	http.Handle("POST /repos/{id}/issues/create", app.ProtectFunc(c.createIssue, auth.Required))
	http.Handle("POST /repos/{id}/issues/{issueID}/edit", app.ProtectFunc(c.editIssue, auth.Required))
	http.Handle("POST /repos/{id}/issues/{issueID}/close", app.ProtectFunc(c.closeIssue, auth.Required))
	http.Handle("POST /repos/{id}/issues/{issueID}/reopen", app.ProtectFunc(c.reopenIssue, auth.Required))
	http.Handle("POST /repos/{id}/issues/{issueID}/comment", app.ProtectFunc(c.createIssueComment, auth.Required))
	http.Handle("DELETE /repos/{id}/issues/{issueID}", app.ProtectFunc(c.deleteIssue, auth.Required))
	http.Handle("POST /repos/{id}/prs/create", app.ProtectFunc(c.createPR, auth.Required))
	http.Handle("POST /repos/{id}/prs/{prID}/merge", app.ProtectFunc(c.mergePR, auth.Required))
	http.Handle("POST /repos/{id}/prs/{prID}/close", app.ProtectFunc(c.closePR, auth.Required))
	http.Handle("POST /repos/{id}/prs/{prID}/comment", app.ProtectFunc(c.createPRComment, auth.Required))
	http.Handle("POST /repos/{id}/permissions", app.ProtectFunc(c.grantPermission, auth.Required))
	http.Handle("DELETE /repos/{id}/permissions/{userID}", app.ProtectFunc(c.revokePermission, auth.Required))
	http.Handle("POST /repos/{id}/files/save", app.ProtectFunc(c.saveFile, auth.Required))
	http.Handle("POST /repos/{id}/files/create", app.ProtectFunc(c.createFile, auth.Required))
	http.Handle("DELETE /repos/{id}/files/{path...}", app.ProtectFunc(c.deleteFile, auth.Required))
	http.Handle("POST /repos/{id}/open-ide", app.ProtectFunc(c.openInIDE, auth.Required))

	http.Handle("/repo/", http.StripPrefix("/repo/", models.GitServer(auth)))
}

// Handle is called when each request is handled
func (c *ReposController) Handle(req *http.Request) application.Controller {
	c.Request = req
	return c
}

// CurrentRepo returns the repository from the URL path
func (c *ReposController) CurrentRepo() (*models.GitRepo, error) {
	return c.getCurrentRepoFromRequest(c.Request)
}

// getCurrentRepoFromRequest returns the repository from a specific request with permission checking
func (c *ReposController) getCurrentRepoFromRequest(r *http.Request) (*models.GitRepo, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errors.New("repository ID not found")
	}

	repo, err := models.GitRepos.Get(id)
	if err != nil {
		return nil, err
	}

	// Check permissions
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		return nil, errors.New("authentication required")
	}

	// Handle repositories without UserID (legacy repositories)
	if repo.UserID == "" {
		// Assign ownership to the current user for legacy repositories
		repo.UserID = user.ID
		models.GitRepos.Update(repo)
	}

	// Always grant the repository owner admin permissions if they don't have any
	if repo.UserID == user.ID {
		models.GrantPermission(user.ID, id, models.RoleAdmin)
	}

	// Check repository access permissions
	err = models.CheckRepoAccess(user, id, models.RoleRead)
	if err != nil {
		return nil, errors.New("access denied: " + err.Error())
	}

	return repo, nil
}

// RepoIssues returns issues for the current repository
func (c *ReposController) RepoIssues() ([]*models.Issue, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	return models.Issues.Search("WHERE RepoID = ? ORDER BY CreatedAt DESC", repo.ID)
}

// GetIssueComments returns comments for a specific issue
func (c *ReposController) GetIssueComments(issueID string) ([]*models.Comment, error) {
	// Check if user has access to the repository
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	// Verify the issue belongs to this repository
	issue, err := models.Issues.Get(issueID)
	if err != nil || issue.RepoID != repo.ID {
		return nil, errors.New("issue not found")
	}

	return models.GetIssueComments(issueID)
}

// RepoPullRequests returns pull requests for the current repository
func (c *ReposController) RepoPullRequests() ([]*models.PullRequest, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	return models.PullRequests.Search("WHERE RepoID = ? ORDER BY CreatedAt DESC", repo.ID)
}

// GetPRComments returns comments for a specific pull request
func (c *ReposController) GetPRComments(prID string) ([]*models.Comment, error) {
	// Check if user has access to the repository
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	// Verify the PR belongs to this repository
	pr, err := models.PullRequests.Get(prID)
	if err != nil || pr.RepoID != repo.ID {
		return nil, errors.New("pull request not found")
	}

	return models.GetPRComments(prID)
}

// RepoActions returns AI actions for the current repository
func (c *ReposController) RepoActions() ([]*models.Action, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	return models.Actions.Search("WHERE RepoID = ? ORDER BY CreatedAt DESC", repo.ID)
}

// createRepo handles repository creation
func (c *ReposController) createRepo(w http.ResponseWriter, r *http.Request) {
	// Authenticate user
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("authentication required"))
		return
	}

	// Validate required fields
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		c.Render(w, r, "error-message.html", errors.New("repository name is required"))
		return
	}

	// Extract optional fields with defaults
	description := strings.TrimSpace(r.FormValue("description"))
	visibility := r.FormValue("visibility")
	if visibility == "" {
		visibility = "private"
	}

	// Generate URL-friendly repository ID
	repoID := generateRepoID(name, user.ID)

	// Create the repository
	repo, err := models.NewRepo(repoID, name)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to create repository: "+err.Error()))
		return
	}

	// Set repository metadata and ownership
	repo.UserID = user.ID
	repo.Description = description
	repo.Visibility = visibility

	// Save the updated repository
	if err = models.GitRepos.Update(repo); err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to update repository: "+err.Error()))
		return
	}

	// Grant admin permissions to creator for explicit permission tracking
	// This is complementary to the UserID ownership check
	if err = models.GrantPermission(user.ID, repo.ID, models.RoleAdmin); err != nil {
		// Log warning but don't fail - UserID ownership is primary mechanism
		log.Printf("Warning: failed to grant admin permission for repo %s to user %s: %v", repo.ID, user.ID, err)
	}

	// Log the repository creation activity
	models.LogActivity("repo_created", "Created repository "+repo.Name,
		"New "+repo.Visibility+" repository created", user.ID, repo.ID, "repository", repo.ID)

	// Redirect to the new repository
	c.Redirect(w, r, "/repos/"+repo.ID)
}


// createAction handles automated action creation
func (c *ReposController) createAction(w http.ResponseWriter, r *http.Request) {
	// Authenticate user
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("authentication required"))
		return
	}

	repoID := r.PathValue("id")
	if repoID == "" {
		c.Render(w, r, "error-message.html", errors.New("repository ID required"))
		return
	}

	// Check if repository exists and user has access
	_, err = c.getCurrentRepoFromRequest(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Validate required fields
	title := strings.TrimSpace(r.FormValue("title"))
	actionType := strings.TrimSpace(r.FormValue("type"))
	script := strings.TrimSpace(r.FormValue("script"))

	if title == "" || actionType == "" || script == "" {
		c.Render(w, r, "error-message.html", errors.New("title, type, and script are required"))
		return
	}

	// Validate action type
	validTypes := map[string]bool{
		"on_push":   true,
		"on_pr":     true,
		"on_issue":  true,
		"scheduled": true,
		"manual":    true,
	}
	if !validTypes[actionType] {
		c.Render(w, r, "error-message.html", errors.New("invalid action type"))
		return
	}

	// Extract optional fields
	description := strings.TrimSpace(r.FormValue("description"))
	trigger := strings.TrimSpace(r.FormValue("trigger"))

	// Create the action
	action := &models.Action{
		Type:        actionType,
		Title:       title,
		Description: description,
		Trigger:     trigger,
		Script:      script,
		Status:      "active",
		RepoID:      repoID,
		UserID:      user.ID,
	}

	// Save the action
	_, err = models.Actions.Insert(action)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to create action: "+err.Error()))
		return
	}

	// Log the action creation activity
	models.LogActivity("action_created", "Created action: "+action.Title,
		"New "+action.Type+" action created", user.ID, repoID, "action", action.ID)

	// Refresh the page to show the new action
	c.Refresh(w, r)
}

// runAction handles manual execution of an action
func (c *ReposController) runAction(w http.ResponseWriter, r *http.Request) {
	// Authenticate user
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("authentication required"))
		return
	}

	repoID := r.PathValue("id")
	actionID := r.PathValue("actionID")

	if repoID == "" || actionID == "" {
		c.Render(w, r, "error-message.html", errors.New("repository ID and action ID required"))
		return
	}

	// Check if repository exists and user has access
	_, err = c.getCurrentRepoFromRequest(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Get the action
	action, err := models.Actions.Get(actionID)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("action not found"))
		return
	}

	// Verify action belongs to this repository
	if action.RepoID != repoID {
		c.Render(w, r, "error-message.html", errors.New("action not found in this repository"))
		return
	}

	// Execute the action
	err = action.ExecuteManually()
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to execute action: "+err.Error()))
		return
	}

	// Log the action execution activity
	models.LogActivity("action_executed", "Executed action: "+action.Title,
		"Manual execution triggered", user.ID, repoID, "action", action.ID)

	// Refresh the page to show updated status
	c.Refresh(w, r)
}

// AllRepos returns all repositories the current user has access to
func (c *ReposController) AllRepos() ([]*models.GitRepo, error) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(c.Request)
	if err != nil {
		return nil, err
	}

	// Get all repositories with access
	return models.GetUserAccessibleRepos(user.ID)
}

// RepoPermissions returns permissions for the current repository
func (c *ReposController) RepoPermissions() ([]*models.Permission, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	// Check admin permissions to view repository permissions
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(c.Request)
	if err != nil {
		return nil, err
	}
	err = models.CheckRepoAccess(user, repo.ID, models.RoleAdmin)
	if err != nil {
		return nil, err
	}

	return models.Permissions.Search("WHERE RepoID = ?", repo.ID)
}

// grantPermission handles granting permissions to users
func (c *ReposController) grantPermission(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("unauthorized"))
		return
	}

	repoID := r.PathValue("id")
	if repoID == "" {
		c.Render(w, r, "error-message.html", errors.New("repository ID required"))
		return
	}

	// Check admin permissions for granting permissions
	err = models.CheckRepoAccess(user, repoID, models.RoleAdmin)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("insufficient permissions"))
		return
	}

	targetUserEmail := r.FormValue("user_email")
	role := r.FormValue("role")

	if targetUserEmail == "" || role == "" {
		c.Render(w, r, "error-message.html", errors.New("user email and role are required"))
		return
	}

	// Find user by email
	targetUser, err := models.Auth.Users.Search("WHERE Email = ?", targetUserEmail)
	if err != nil || len(targetUser) == 0 {
		c.Render(w, r, "error-message.html", errors.New("user not found"))
		return
	}

	// Grant permission
	err = models.GrantPermission(targetUser[0].ID, repoID, role)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Refresh to show updated settings
	c.Refresh(w, r)
}

// revokePermission handles revoking permissions from users
func (c *ReposController) revokePermission(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("authentication required"))
		return
	}

	repoID := r.PathValue("id")
	targetUserID := r.PathValue("userID")

	if repoID == "" || targetUserID == "" {
		c.Render(w, r, "error-message.html", errors.New("repository ID and user ID required"))
		return
	}

	// Check admin permissions for revoking permissions
	err = models.CheckRepoAccess(user, repoID, models.RoleAdmin)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("insufficient permissions"))
		return
	}

	// Revoke permission
	err = models.RevokePermission(targetUserID, repoID)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Log activity
	models.LogActivity("permission_revoked", "Revoked repository access",
		"User access revoked", user.ID, repoID, "permission", targetUserID)

	c.Refresh(w, r)
}

// FileInfo represents a file or directory in the repository
type FileInfo struct {
	Name     string
	Path     string
	IsDir    bool
	Size     int64
	ModTime  time.Time
	Content  string
	Language string
	IsBinary bool
}

// SearchResult represents a search result within repository files
type SearchResult struct {
	File     string
	Path     string
	LineNum  int
	Line     string
	Context  []string
	Language string
}

// RepoFiles returns files in the repository directory
func (c *ReposController) RepoFiles() ([]*FileInfo, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	// Get current path and branch
	path := c.Request.PathValue("path")
	if path == "" {
		path = "."
	}
	branch := c.CurrentBranch()
	
	// If no branch exists, return empty list
	if branch == "" {
		return []*FileInfo{}, nil
	}

	// Use git to browse files on the selected branch
	gitBlob, err := repo.Open(branch, path)
	if err != nil {
		// If we can't open the path, the repository might be empty or path doesn't exist
		return []*FileInfo{}, nil
	}

	if !gitBlob.IsDirectory {
		// If it's a file, return single file info
		fileInfo := &FileInfo{
			Name:     filepath.Base(path),
			Path:     path,
			IsDir:    false,
			Language: getLanguageFromExtension(filepath.Ext(path)),
		}

		// Get file size and check if binary
		fileInfo.Size = gitBlob.Size()

		// Read content if not binary
		content, err := gitBlob.Content()
		if err == nil {
			fileInfo.IsBinary = isBinary([]byte(content))
			if !fileInfo.IsBinary {
				fileInfo.Content = content
			}
		}

		return []*FileInfo{fileInfo}, nil
	}

	// List directory contents using git
	blobs, err := gitBlob.Files()
	if err != nil {
		return nil, err
	}

	var files []*FileInfo
	for _, blob := range blobs {
		// Get file size for files
		var size int64
		if !blob.IsDirectory {
			size = blob.Size()
		}

		fileInfo := &FileInfo{
			Name:     blob.Path,
			Path:     filepath.Join(path, blob.Path),
			IsDir:    blob.IsDirectory,
			Size:     size,
			Language: getLanguageFromExtension(filepath.Ext(blob.Path)),
		}
		files = append(files, fileInfo)
	}

	return files, nil
}

// FileLines returns the lines of the current file for display
func (c *ReposController) FileLines() ([]string, error) {
	file, err := c.CurrentFile()
	if err != nil || file.IsBinary || file.IsDir {
		return nil, err
	}
	return strings.Split(file.Content, "\n"), nil
}

// CurrentFile returns the current file being viewed
func (c *ReposController) CurrentFile() (*FileInfo, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	path := c.Request.PathValue("path")
	if path == "" {
		return nil, errors.New("file path required")
	}

	branch := c.CurrentBranch()

	// Use git to access the file on the selected branch
	gitBlob, err := repo.Open(branch, path)
	if err != nil {
		return nil, err
	}

	fileInfo := &FileInfo{
		Name:     filepath.Base(path),
		Path:     path,
		IsDir:    gitBlob.IsDirectory,
		Language: getLanguageFromExtension(filepath.Ext(path)),
	}

	// Get file size
	if !gitBlob.IsDirectory {
		fileInfo.Size = gitBlob.Size()

		// Read content
		content, err := gitBlob.Content()
		if err == nil {
			fileInfo.IsBinary = isBinary([]byte(content))
			if !fileInfo.IsBinary {
				fileInfo.Content = content
			}
		}
	}

	return fileInfo, nil
}

// RepoCommits returns recent commits for the repository
func (c *ReposController) RepoCommits() ([]*models.GitCommit, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	// Get actual commits from Git
	commits, err := repo.GetCommits("HEAD", 50) // Get last 50 commits
	if err != nil {
		return nil, err
	}

	// Log activity for commit viewing
	auth := c.Use("auth").(*authentication.Controller)
	if user, _, err := auth.Authenticate(c.Request); err == nil {
		models.LogActivity("commits_viewed", "Viewed commits for "+repo.Name,
			"Browsed repository commit history", user.ID, repo.ID, "commits", "")
	}

	return commits, nil
}

// RepoBranches returns branches for the current repository
func (c *ReposController) RepoBranches() ([]*models.GitBranch, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	return repo.GetBranches()
}

// RepoPRDiff returns the diff for a pull request
func (c *ReposController) RepoPRDiff() ([]*models.GitDiff, error) {
	prID := c.Request.PathValue("prID")
	if prID == "" {
		return nil, errors.New("PR ID not found")
	}

	// Get PR details
	pr, err := models.PullRequests.Get(prID)
	if err != nil {
		return nil, err
	}

	// Get repository
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	return repo.GetPRDiff(pr.BaseBranch, pr.CompareBranch)
}

// RepoCommitDiff returns the diff for a specific commit
func (c *ReposController) RepoCommitDiff() ([]*models.GitDiff, error) {
	commitHash := c.Request.PathValue("hash")
	if commitHash == "" {
		return nil, errors.New("commit hash not found")
	}

	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	return repo.GetCommitDiff(commitHash)
}

// CurrentBranch returns the currently selected branch for browsing
func (c *ReposController) CurrentBranch() string {
	branch := c.Request.URL.Query().Get("branch")
	if branch == "" {
		// Get the default branch from the current repository
		if repo, err := c.CurrentRepo(); err == nil {
			defaultBranch := repo.GetDefaultBranch()
			if defaultBranch != "" {
				return defaultBranch
			}
		}
		return "" // Empty for repositories with no branches
	}
	return branch
}

// SearchCode searches for code within repository files
func (c *ReposController) SearchCode() ([]*SearchResult, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	// Get search query from request
	query := c.Request.URL.Query().Get("q")
	if query == "" {
		return []*SearchResult{}, nil
	}

	// Search within repository files
	results, err := c.searchInRepository(repo.ID, query)
	if err != nil {
		return nil, err
	}

	// Log search activity
	auth := c.Use("auth").(*authentication.Controller)
	if user, _, err := auth.Authenticate(c.Request); err == nil {
		models.LogActivity("code_search", "Searched for: "+query,
			"Code search performed", user.ID, repo.ID, "search", "")
	}

	return results, nil
}

// searchInRepository performs the actual file search within a repository
func (c *ReposController) searchInRepository(repoID, query string) ([]*SearchResult, error) {
	repoPath := filepath.Join("repos", repoID)
	var results []*SearchResult

	// Compile regex for case-insensitive search
	regex, err := regexp.Compile("(?i)" + regexp.QuoteMeta(query))
	if err != nil {
		return nil, errors.New("invalid search query")
	}

	// Walk through repository files
	err = filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files with errors
		}

		// Skip directories and hidden files
		if info.IsDir() || strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		// Get relative path from repo root
		relPath, err := filepath.Rel(repoPath, path)
		if err != nil {
			return nil
		}

		// Read file content
		content, err := os.ReadFile(path)
		if err != nil || isBinary(content) {
			return nil // Skip binary files and files we can't read
		}

		// Search within file content
		lines := strings.Split(string(content), "\n")
		for lineNum, line := range lines {
			if regex.MatchString(line) {
				// Get context lines (2 before and 2 after)
				context := getContextLines(lines, lineNum, 2)

				result := &SearchResult{
					File:     info.Name(),
					Path:     relPath,
					LineNum:  lineNum + 1, // 1-indexed
					Line:     line,
					Context:  context,
					Language: getLanguageFromExtension(filepath.Ext(path)),
				}
				results = append(results, result)

				// Limit results to prevent overwhelming response
				if len(results) >= 100 {
					return filepath.SkipDir
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return results, nil
}

// getContextLines returns context lines around a specific line number
func getContextLines(lines []string, lineNum, contextSize int) []string {
	start := lineNum - contextSize
	end := lineNum + contextSize + 1

	if start < 0 {
		start = 0
	}
	if end > len(lines) {
		end = len(lines)
	}

	return lines[start:end]
}

// HasFiles checks if the repository has any files
func (c *ReposController) HasFiles() bool {
	repo, err := c.CurrentRepo()
	if err != nil {
		return false
	}

	branch := c.CurrentBranch()
	// If no branch exists, no files
	if branch == "" {
		return false
	}
	// Try to list files in the root
	_, err = repo.Open(branch, ".")
	return err == nil
}

// RepoReadme detects and returns README content for the repository
func (c *ReposController) RepoReadme() (*FileInfo, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	// Look for README files in repository root using git
	branch := c.CurrentBranch()
	// If no branch exists, no README
	if branch == "" {
		return nil, nil
	}
	
	readmeFiles := []string{
		"README.md",
		"readme.md",
		"README.MD",
		"README.txt",
		"README.rst",
		"README",
		"readme",
	}

	for _, filename := range readmeFiles {
		// Try to open the file via git
		gitBlob, err := repo.Open(branch, filename)
		if err == nil && !gitBlob.IsDirectory {
			content, err := gitBlob.Content()
			if err != nil {
				continue
			}

			// Check if binary
			if isBinary([]byte(content)) {
				continue
			}

			fileInfo := &FileInfo{
				Name:     filename,
				Path:     filename,
				IsDir:    false,
				Size:     gitBlob.Size(),
				Content:  content,
				Language: getLanguageFromExtension(filepath.Ext(filename)),
				IsBinary: false,
			}

			return fileInfo, nil
		}
	}

	return nil, nil // No README found
}

// RenderMarkdown converts markdown content to HTML (accessible from templates)
func (c *ReposController) RenderMarkdown(content string) template.HTML {
	// Basic markdown parsing (simplified implementation)
	// In a production app, you'd use a proper markdown library like blackfriday or goldmark

	// Convert newlines to HTML
	html := strings.ReplaceAll(content, "\n", "<br>")

	// Headers
	html = regexp.MustCompile(`(?m)^# (.+)$`).ReplaceAllString(html, "<h1>$1</h1>")
	html = regexp.MustCompile(`(?m)^## (.+)$`).ReplaceAllString(html, "<h2>$1</h2>")
	html = regexp.MustCompile(`(?m)^### (.+)$`).ReplaceAllString(html, "<h3>$1</h3>")

	// Bold and italic
	html = regexp.MustCompile(`\*\*([^*]+)\*\*`).ReplaceAllString(html, "<strong>$1</strong>")
	html = regexp.MustCompile(`\*([^*]+)\*`).ReplaceAllString(html, "<em>$1</em>")

	// Code blocks (backticks)
	html = regexp.MustCompile("```([\\s\\S]*?)```").ReplaceAllString(html, "<pre class=\"bg-base-200 p-4 rounded\"><code>$1</code></pre>")
	html = regexp.MustCompile("`([^`]+)`").ReplaceAllString(html, "<code class=\"bg-base-200 px-1 rounded\">$1</code>")

	// Links
	html = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`).ReplaceAllString(html, "<a href=\"$2\" class=\"link link-primary\">$1</a>")

	// Lists (basic)
	html = regexp.MustCompile(`(?m)^- (.+)$`).ReplaceAllString(html, "<li>$1</li>")
	html = regexp.MustCompile("(<li>.*</li>)").ReplaceAllString(html, "<ul class=\"list-disc list-inside\">$1</ul>")

	return template.HTML(html)
}

// IsMarkdown checks if a file is a markdown file
func (c *ReposController) IsMarkdown(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".md" || ext == ".markdown" || ext == ".mdown"
}

// isBinary checks if content is binary
func isBinary(content []byte) bool {
	// Simple binary detection - check for null bytes
	for _, b := range content {
		if b == 0 {
			return true
		}
	}
	return false
}

// getLanguageFromExtension returns the programming language for syntax highlighting
func getLanguageFromExtension(ext string) string {
	languages := map[string]string{
		".go":         "go",
		".js":         "javascript",
		".ts":         "typescript",
		".py":         "python",
		".java":       "java",
		".cpp":        "cpp",
		".c":          "c",
		".css":        "css",
		".html":       "html",
		".md":         "markdown",
		".json":       "json",
		".yaml":       "yaml",
		".yml":        "yaml",
		".xml":        "xml",
		".sql":        "sql",
		".sh":         "bash",
		".dockerfile": "dockerfile",
		".rs":         "rust",
		".php":        "php",
		".rb":         "ruby",
	}

	if lang, exists := languages[strings.ToLower(ext)]; exists {
		return lang
	}
	return "text"
}

// createIssue handles issue creation
func (c *ReposController) createIssue(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("authentication required"))
		return
	}

	repoID := r.PathValue("id")
	if repoID == "" {
		c.Render(w, r, "error-message.html", errors.New("repository ID required"))
		return
	}

	// Check repository access
	_, err = c.getCurrentRepoFromRequest(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Validate required fields
	title := strings.TrimSpace(r.FormValue("title"))
	body := strings.TrimSpace(r.FormValue("body"))
	tags := strings.TrimSpace(r.FormValue("tags"))

	if title == "" {
		c.Render(w, r, "error-message.html", errors.New("issue title is required"))
		return
	}

	// Create the issue
	issue := &models.Issue{
		Title:      title,
		Body:       body,
		Tags:       tags,
		Status:     "open",
		RepoID:     repoID,
		AssigneeID: user.ID,
	}

	_, err = models.Issues.Insert(issue)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to create issue: "+err.Error()))
		return
	}

	// Log activity
	models.LogActivity("issue_created", "Created issue: "+issue.Title,
		"New issue opened", user.ID, repoID, "issue", issue.ID)

	// Trigger actions for issue creation event
	eventData := map[string]string{
		"ISSUE_ID":     issue.ID,
		"ISSUE_TITLE":  issue.Title,
		"ISSUE_STATUS": issue.Status,
		"AUTHOR_ID":    user.ID,
	}
	go models.TriggerActionsByEvent("on_issue", repoID, eventData)

	// Refresh to show new issue
	c.Refresh(w, r)
}

// closeIssue handles closing an issue
func (c *ReposController) closeIssue(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	repoID := r.PathValue("id")
	issueID := r.PathValue("issueID")

	if repoID == "" || issueID == "" {
		http.Error(w, "repository ID and issue ID required", http.StatusBadRequest)
		return
	}

	// Get and update issue
	issue, err := models.Issues.Get(issueID)
	if err != nil {
		http.Error(w, "issue not found", http.StatusNotFound)
		return
	}

	issue.Status = "closed"
	err = models.Issues.Update(issue)
	if err != nil {
		http.Error(w, "failed to close issue", http.StatusInternalServerError)
		return
	}

	// Log activity
	models.LogActivity("issue_closed", "Closed issue: "+issue.Title,
		"Issue marked as closed", user.ID, repoID, "issue", issue.ID)

	c.Refresh(w, r)
}

// reopenIssue handles reopening an issue
func (c *ReposController) reopenIssue(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	repoID := r.PathValue("id")
	issueID := r.PathValue("issueID")

	if repoID == "" || issueID == "" {
		http.Error(w, "repository ID and issue ID required", http.StatusBadRequest)
		return
	}

	// Get and update issue
	issue, err := models.Issues.Get(issueID)
	if err != nil {
		http.Error(w, "issue not found", http.StatusNotFound)
		return
	}

	issue.Status = "open"
	err = models.Issues.Update(issue)
	if err != nil {
		http.Error(w, "failed to reopen issue", http.StatusInternalServerError)
		return
	}

	// Log activity
	models.LogActivity("issue_reopened", "Reopened issue: "+issue.Title,
		"Issue marked as open", user.ID, repoID, "issue", issue.ID)

	c.Refresh(w, r)
}

// editIssue handles editing an issue
func (c *ReposController) editIssue(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("authentication required"))
		return
	}

	repoID := r.PathValue("id")
	issueID := r.PathValue("issueID")

	if repoID == "" || issueID == "" {
		c.Render(w, r, "error-message.html", errors.New("repository ID and issue ID required"))
		return
	}

	// Check repository write access
	err = models.CheckRepoAccess(user, repoID, models.RoleWrite)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("insufficient permissions"))
		return
	}

	// Get the issue
	issue, err := models.Issues.Get(issueID)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("issue not found"))
		return
	}

	// Update fields
	title := strings.TrimSpace(r.FormValue("title"))
	body := strings.TrimSpace(r.FormValue("body"))
	tags := strings.TrimSpace(r.FormValue("tags"))
	assigneeID := strings.TrimSpace(r.FormValue("assignee_id"))

	if title != "" {
		issue.Title = title
	}
	issue.Body = body
	issue.Tags = tags
	issue.AssigneeID = assigneeID

	// Save changes
	err = models.Issues.Update(issue)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to update issue"))
		return
	}

	// Log activity
	models.LogActivity("issue_updated", "Updated issue: "+issue.Title,
		"Issue details modified", user.ID, repoID, "issue", issue.ID)

	c.Refresh(w, r)
}

// deleteIssue handles deleting an issue
func (c *ReposController) deleteIssue(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	repoID := r.PathValue("id")
	issueID := r.PathValue("issueID")

	if repoID == "" || issueID == "" {
		http.Error(w, "repository ID and issue ID required", http.StatusBadRequest)
		return
	}

	// Check repository admin access
	err = models.CheckRepoAccess(user, repoID, models.RoleAdmin)
	if err != nil {
		http.Error(w, "insufficient permissions", http.StatusForbidden)
		return
	}

	// Get the issue for logging
	issue, err := models.Issues.Get(issueID)
	if err != nil {
		http.Error(w, "issue not found", http.StatusNotFound)
		return
	}

	// Delete the issue
	err = models.Issues.Delete(issue)
	if err != nil {
		http.Error(w, "failed to delete issue", http.StatusInternalServerError)
		return
	}

	// Log activity
	models.LogActivity("issue_deleted", "Deleted issue: "+issue.Title,
		"Issue permanently removed", user.ID, repoID, "issue", issue.ID)

	c.Refresh(w, r)
}

// createIssueComment handles adding a comment to an issue
func (c *ReposController) createIssueComment(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("authentication required"))
		return
	}

	repoID := r.PathValue("id")
	issueID := r.PathValue("issueID")
	body := strings.TrimSpace(r.FormValue("body"))

	if repoID == "" || issueID == "" || body == "" {
		c.Render(w, r, "error-message.html", errors.New("repository ID, issue ID, and comment body required"))
		return
	}

	// Check repository read access (anyone who can read can comment)
	err = models.CheckRepoAccess(user, repoID, models.RoleRead)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("insufficient permissions"))
		return
	}

	// Verify issue exists
	issue, err := models.Issues.Get(issueID)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("issue not found"))
		return
	}

	// Create comment
	_, err = models.CreateIssueComment(issueID, repoID, user.ID, body)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to create comment"))
		return
	}

	// Log activity
	models.LogActivity("comment_created", "Commented on issue: "+issue.Title,
		"New comment added", user.ID, repoID, "issue_comment", issueID)

	c.Refresh(w, r)
}

// createPR handles pull request creation
func (c *ReposController) createPR(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("authentication required"))
		return
	}

	repoID := r.PathValue("id")
	if repoID == "" {
		c.Render(w, r, "error-message.html", errors.New("repository ID required"))
		return
	}

	// Check repository access
	_, err = c.getCurrentRepoFromRequest(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Validate required fields
	title := strings.TrimSpace(r.FormValue("title"))
	body := strings.TrimSpace(r.FormValue("body"))
	baseBranch := strings.TrimSpace(r.FormValue("base_branch"))
	compareBranch := strings.TrimSpace(r.FormValue("compare_branch"))

	if title == "" || baseBranch == "" || compareBranch == "" {
		c.Render(w, r, "error-message.html", errors.New("title, base branch, and compare branch are required"))
		return
	}

	// Create the pull request
	pr := &models.PullRequest{
		Title:         title,
		Body:          body,
		RepoID:        repoID,
		AuthorID:      user.ID,
		BaseBranch:    baseBranch,
		CompareBranch: compareBranch,
		Status:        "open",
	}

	_, err = models.PullRequests.Insert(pr)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to create pull request: "+err.Error()))
		return
	}

	// Log activity
	models.LogActivity("pr_created", "Created pull request: "+pr.Title,
		"New pull request opened", user.ID, repoID, "pull_request", pr.ID)

	// Trigger actions for PR creation event
	eventData := map[string]string{
		"PR_ID":          pr.ID,
		"PR_TITLE":       pr.Title,
		"PR_STATUS":      pr.Status,
		"BASE_BRANCH":    pr.BaseBranch,
		"COMPARE_BRANCH": pr.CompareBranch,
		"AUTHOR_ID":      user.ID,
	}
	go models.TriggerActionsByEvent("on_pr", repoID, eventData)

	// Redirect to PRs page
	c.Redirect(w, r, "/repos/"+repoID+"/prs")
}

// mergePR handles merging a pull request
func (c *ReposController) mergePR(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	repoID := r.PathValue("id")
	prID := r.PathValue("prID")

	if repoID == "" || prID == "" {
		http.Error(w, "repository ID and PR ID required", http.StatusBadRequest)
		return
	}

	// Get PR and repository
	pr, err := models.PullRequests.Get(prID)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("pull request not found"))
		return
	}

	// Check if PR is still open
	if pr.Status != "open" {
		c.Render(w, r, "error-message.html", errors.New("pull request is not open"))
		return
	}

	// Get repository
	repo, err := c.getCurrentRepoFromRequest(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Check if merge is possible
	canMerge, err := repo.CanMergeBranch(pr.CompareBranch, pr.BaseBranch)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to check merge compatibility: "+err.Error()))
		return
	}

	if !canMerge {
		c.Render(w, r, "error-message.html", errors.New("merge conflicts detected - cannot auto-merge"))
		return
	}

	// Perform the actual git merge
	err = repo.MergeBranch(pr.CompareBranch, pr.BaseBranch)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to merge branches: "+err.Error()))
		return
	}

	// Update PR status
	pr.Status = "merged"
	err = models.PullRequests.Update(pr)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to update pull request status"))
		return
	}

	// Log activity
	models.LogActivity("pr_merged", "Merged pull request: "+pr.Title,
		"Pull request merged", user.ID, repoID, "pull_request", pr.ID)

	// Trigger actions for push event (merge is essentially a push to base branch)
	eventData := map[string]string{
		"BRANCH":         pr.BaseBranch,
		"PR_ID":          pr.ID,
		"PR_TITLE":       pr.Title,
		"COMPARE_BRANCH": pr.CompareBranch,
		"AUTHOR_ID":      user.ID,
		"EVENT_TYPE":     "merge",
	}
	go models.TriggerActionsByEvent("on_push", repoID, eventData)

	c.Refresh(w, r)
}

// closePR handles closing a pull request
func (c *ReposController) closePR(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	repoID := r.PathValue("id")
	prID := r.PathValue("prID")

	if repoID == "" || prID == "" {
		http.Error(w, "repository ID and PR ID required", http.StatusBadRequest)
		return
	}

	// Get and update PR
	pr, err := models.PullRequests.Get(prID)
	if err != nil {
		http.Error(w, "pull request not found", http.StatusNotFound)
		return
	}

	pr.Status = "closed"
	err = models.PullRequests.Update(pr)
	if err != nil {
		http.Error(w, "failed to close pull request", http.StatusInternalServerError)
		return
	}

	// Log activity
	models.LogActivity("pr_closed", "Closed pull request: "+pr.Title,
		"Pull request closed", user.ID, repoID, "pull_request", pr.ID)

	c.Refresh(w, r)
}

// createPRComment handles adding a comment to a pull request
func (c *ReposController) createPRComment(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("authentication required"))
		return
	}

	repoID := r.PathValue("id")
	prID := r.PathValue("prID")
	body := strings.TrimSpace(r.FormValue("body"))

	if repoID == "" || prID == "" || body == "" {
		c.Render(w, r, "error-message.html", errors.New("repository ID, PR ID, and comment body required"))
		return
	}

	// Check repository read access (anyone who can read can comment)
	err = models.CheckRepoAccess(user, repoID, models.RoleRead)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("insufficient permissions"))
		return
	}

	// Verify PR exists
	pr, err := models.PullRequests.Get(prID)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("pull request not found"))
		return
	}

	// Create comment
	_, err = models.CreatePRComment(prID, repoID, user.ID, body)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to create comment"))
		return
	}

	// Log activity
	models.LogActivity("comment_created", "Commented on PR: "+pr.Title,
		"New comment added", user.ID, repoID, "pr_comment", prID)

	c.Refresh(w, r)
}

// generateRepoID creates a URL-friendly, unique repository ID
// Format: {sanitized-name}-{user-suffix}
func generateRepoID(name, userID string) string {
	// Sanitize the repository name for URL use
	repoID := sanitizeForURL(name)

	// Ensure we have a valid base name
	if repoID == "" {
		repoID = "repository"
	}

	// Create a unique suffix from userID (first 8 chars)
	userSuffix := userID
	if len(userSuffix) > 8 {
		userSuffix = userSuffix[:8]
	}

	return repoID + "-" + userSuffix
}

// sanitizeForURL converts a string to a URL-friendly format
func sanitizeForURL(input string) string {
	// Convert to lowercase and trim whitespace
	result := strings.ToLower(strings.TrimSpace(input))

	// Replace any non-alphanumeric characters with hyphens
	reg := regexp.MustCompile(`[^a-z0-9]+`)
	result = reg.ReplaceAllString(result, "-")

	// Remove leading/trailing hyphens and limit length
	result = strings.Trim(result, "-")
	if len(result) > 50 {
		result = result[:50]
		result = strings.TrimRight(result, "-")
	}

	return result
}

// saveFile handles saving file content via web editor
func (c *ReposController) saveFile(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("authentication required"))
		return
	}

	repoID := r.PathValue("id")
	if repoID == "" {
		c.Render(w, r, "error-message.html", errors.New("repository ID required"))
		return
	}

	// Check write permissions
	err = models.CheckRepoAccess(user, repoID, models.RoleWrite)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("insufficient permissions"))
		return
	}

	// Get repository
	repo, err := c.getCurrentRepoFromRequest(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Get form data
	filePath := strings.TrimSpace(r.FormValue("path"))
	content := r.FormValue("content")
	commitMessage := strings.TrimSpace(r.FormValue("commit_message"))
	branch := strings.TrimSpace(r.FormValue("branch"))

	if filePath == "" || commitMessage == "" {
		c.Render(w, r, "error-message.html", errors.New("file path and commit message are required"))
		return
	}

	if branch == "" {
		branch = "main"
	}

	// Write file and commit changes
	err = repo.WriteFile(branch, filePath, content, commitMessage, user.ID)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to save file: "+err.Error()))
		return
	}

	// Log activity
	models.LogActivity("file_edited", "Edited file: "+filePath,
		commitMessage, user.ID, repoID, "file", filePath)

	// Redirect back to file view
	c.Redirect(w, r, "/repos/"+repoID+"/files/"+filePath+"?branch="+branch)
}

// createFile handles creating new files via web editor
func (c *ReposController) createFile(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("authentication required"))
		return
	}

	repoID := r.PathValue("id")
	if repoID == "" {
		c.Render(w, r, "error-message.html", errors.New("repository ID required"))
		return
	}

	// Check write permissions
	err = models.CheckRepoAccess(user, repoID, models.RoleWrite)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("insufficient permissions"))
		return
	}

	// Get repository
	repo, err := c.getCurrentRepoFromRequest(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Get form data
	filePath := strings.TrimSpace(r.FormValue("path"))
	content := r.FormValue("content")
	commitMessage := strings.TrimSpace(r.FormValue("commit_message"))
	branch := strings.TrimSpace(r.FormValue("branch"))

	if filePath == "" || commitMessage == "" {
		c.Render(w, r, "error-message.html", errors.New("file path and commit message are required"))
		return
	}

	if branch == "" {
		branch = "main"
	}

	// Create file and commit changes
	err = repo.WriteFile(branch, filePath, content, commitMessage, user.ID)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to create file: "+err.Error()))
		return
	}

	// Log activity
	models.LogActivity("file_created", "Created file: "+filePath,
		commitMessage, user.ID, repoID, "file", filePath)

	// Redirect to file view
	c.Redirect(w, r, "/repos/"+repoID+"/files/"+filePath+"?branch="+branch)
}

// deleteFile handles deleting files via web interface
func (c *ReposController) deleteFile(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("authentication required"))
		return
	}

	repoID := r.PathValue("id")
	filePath := r.PathValue("path")
	if repoID == "" || filePath == "" {
		c.Render(w, r, "error-message.html", errors.New("repository ID and file path required"))
		return
	}

	// Check write permissions
	err = models.CheckRepoAccess(user, repoID, models.RoleWrite)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("insufficient permissions"))
		return
	}

	// Get repository
	repo, err := c.getCurrentRepoFromRequest(r)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	commitMessage := r.FormValue("commit_message")
	if commitMessage == "" {
		commitMessage = "Delete " + filepath.Base(filePath)
	}

	branch := r.FormValue("branch")
	if branch == "" {
		branch = "main"
	}

	// Delete file and commit changes
	err = repo.DeleteFile(branch, filePath, commitMessage, user.ID)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to delete file: "+err.Error()))
		return
	}

	// Log activity
	models.LogActivity("file_deleted", "Deleted file: "+filePath,
		commitMessage, user.ID, repoID, "file", filePath)

	// Redirect to repository files
	c.Redirect(w, r, "/repos/"+repoID+"/files")
}

// openInIDE handles opening a repository in the IDE by cloning it
func (c *ReposController) openInIDE(w http.ResponseWriter, r *http.Request) {
	auth := c.App.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("authentication required"))
		return
	}

	repoID := r.PathValue("id")
	repo, err := models.GitRepos.Get(repoID)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("repository not found"))
		return
	}

	// Check permissions
	err = models.CheckRepoAccess(user, repoID, models.RoleRead)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("access denied"))
		return
	}

	// Create a temporary access token (valid for 5 minutes)
	token, err := models.CreateAccessToken(repoID, user.ID, 5*time.Minute)
	if err != nil {
		log.Printf("Failed to create access token: %v", err)
		c.Render(w, r, "error-message.html", errors.New("failed to create access token"))
		return
	}
	
	log.Printf("Created access token - ID: %s, Token: %s, RepoID: %s, ExpiresAt: %v", 
		token.ID, token.Token, token.RepoID, token.ExpiresAt.Format(time.RFC3339))

	// Build the clone URL with token as basic auth
	// Both containers use host network, so localhost works
	// Use token ID as username and token as password for basic auth
	// Use repo.ID for the URL (not repo.Name which can have spaces)
	cloneURL := fmt.Sprintf("http://%s:%s@localhost/repo/%s.git", token.ID, token.Token, repo.ID)

	// Execute git clone in the coder container
	// Use proper shell escaping for repository names that might contain spaces
	escapedName := strings.ReplaceAll(repo.Name, "'", "'\\''")
	cloneCmd := fmt.Sprintf(`
		cd /home/coder/project && 
		rm -rf '%s' && 
		git clone '%s' '%s' && 
		echo "Repository cloned successfully"
	`, escapedName, cloneURL, escapedName)

	// Execute in the coder container
	output, err := exec.Command("docker", "exec", "skyscape-coder", "bash", "-c", cloneCmd).CombinedOutput()
	if err != nil {
		c.Render(w, r, "error-message.html", fmt.Errorf("failed to clone repository: %s", string(output)))
		return
	}

	// Return success message with link to open IDE
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `
		<div class="alert alert-success">
			<svg xmlns="http://www.w3.org/2000/svg" class="stroke-current shrink-0 h-6 w-6" fill="none" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
			</svg>
			<div>
				<div class="font-bold">Repository ready in IDE!</div>
				<div class="text-sm">The repository has been cloned to /home/coder/project/%s</div>
			</div>
			<a href="/coder/" target="_blank" class="btn btn-sm btn-primary">Open IDE</a>
		</div>
	`, repo.Name)
}
