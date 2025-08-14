package controllers

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"workspace/models"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

// Repos controller prefix
func Repos() (string, *ReposController) {
	return "repos", &ReposController{}
}

// ReposController handles repository-related operations
type ReposController struct {
	application.BaseController
}

// Setup registers routes
func (c *ReposController) Setup(app *application.App) {
	c.BaseController.Setup(app)
	auth := app.Use("auth").(*authentication.Controller)

	// Repository browsing/reading
	http.Handle("GET /repos", app.Serve("repos-list.html", auth.Required))
	http.Handle("GET /repos/search", app.ProtectFunc(c.searchRepositories, auth.Required))
	http.Handle("GET /repos/{id}", app.Serve("repo-view.html", auth.Required))
	http.Handle("GET /repos/{id}/activity", app.Serve("repo-activity.html", auth.Required))
	http.Handle("GET /repos/{id}/files", app.Serve("repo-files.html", auth.Required))
	http.Handle("GET /repos/{id}/files/{path...}", app.Serve("repo-file-view.html", auth.Required))
	http.Handle("GET /repos/{id}/edit/{path...}", app.Serve("repo-file-edit.html", auth.Required))
	http.Handle("GET /repos/{id}/commits", app.Serve("repo-commits.html", auth.Required))
	http.Handle("GET /repos/{id}/commits/{hash}/diff", app.Serve("repo-commit-diff.html", auth.Required))
	http.Handle("GET /repos/{id}/settings", app.Serve("repo-settings.html", auth.Required))

	// Repository management
	http.Handle("POST /repos/create", app.ProtectFunc(c.createRepository, auth.Required))
	http.Handle("POST /repos/import", app.ProtectFunc(c.importRepository, auth.Required))
	http.Handle("POST /repos/{id}/settings/update", app.ProtectFunc(c.updateRepository, auth.Required))
	http.Handle("POST /repos/{id}/delete", app.ProtectFunc(c.deleteRepository, auth.Required))
	http.Handle("POST /repos/{id}/clone", app.ProtectFunc(c.openInIDE, auth.Required))

	// Permissions
	http.Handle("POST /repos/{id}/settings/permissions/grant", app.ProtectFunc(c.grantPermission, auth.Required))
	http.Handle("POST /repos/{id}/settings/permissions/{userID}/revoke", app.ProtectFunc(c.revokePermission, auth.Required))

	// File operations
	http.Handle("POST /repos/{id}/files/save", app.ProtectFunc(c.saveFile, auth.Required))
	http.Handle("POST /repos/{id}/files/create", app.ProtectFunc(c.createFile, auth.Required))
	http.Handle("POST /repos/{id}/files/delete/{path...}", app.ProtectFunc(c.deleteFile, auth.Required))

	// Git server
	http.Handle("/repo/", http.StripPrefix("/repo/", models.GitServer(auth)))
}

// Handle returns a new controller instance for the request
func (c ReposController) Handle(req *http.Request) application.Controller {
	c.Request = req
	return &c
}

// Helper methods for repo-tabs partial to access other controllers

// RepoIssues returns issues for the current repository (delegates to IssuesController)
func (c *ReposController) RepoIssues() ([]*models.Issue, error) {
	issuesController := c.Use("issues").(*IssuesController)
	return issuesController.RepoIssues()
}

// RepoPullRequests returns pull requests for the current repository (delegates to PullRequestsController)
func (c *ReposController) RepoPullRequests() ([]*models.PullRequest, error) {
	prsController := c.Use("prs").(*PullRequestsController)
	return prsController.RepoPullRequests()
}

// RepoActions returns actions for the current repository (delegates to ActionsController)
func (c *ReposController) RepoActions() ([]*models.Action, error) {
	actionsController := c.Use("actions").(*ActionsController)
	return actionsController.RepoActions()
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

// UserRepos returns the repositories for the current user
func (c *ReposController) UserRepos() ([]*models.Repository, error) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(c.Request)
	if err != nil {
		return nil, err
	}

	// Get repositories for user
	repos, err := models.Repositories.Search("WHERE UserID = ? OR Visibility = ?", user.ID, "public")
	if err != nil {
		return nil, err
	}

	// Also get repos where user has permissions
	permissions, err := models.Permissions.Search("WHERE UserID = ?", user.ID)
	if err != nil {
		return nil, err
	}

	// Add repos from permissions
	for _, perm := range permissions {
		repo, err := models.Repositories.Get(perm.RepoID)
		if err == nil {
			// Check if repo is already in list
			found := false
			for _, r := range repos {
				if r.ID == repo.ID {
					found = true
					break
				}
			}
			if !found {
				repos = append(repos, repo)
			}
		}
	}

	return repos, nil
}

// CurrentRepo returns the repository for the current request
func (c *ReposController) CurrentRepo() (*models.Repository, error) {
	return c.getCurrentRepoFromRequest(c.Request)
}

// getCurrentRepoFromRequest returns the repository for the given request
func (c *ReposController) getCurrentRepoFromRequest(r *http.Request) (*models.Repository, error) {
	repoID := r.PathValue("id")
	if repoID == "" {
		return nil, errors.New("repository ID required")
	}

	repo, err := models.Repositories.Get(repoID)
	if err != nil {
		return nil, errors.New("repository not found")
	}

	// Check permissions
	auth := c.Use("auth").(*authentication.Controller)
	if user, _, err := auth.Authenticate(r); err == nil {
		// Allow owner or users with any permission
		if repo.UserID != user.ID && repo.Visibility != "public" {
			err = models.CheckRepoAccess(user, repoID, models.RoleRead)
			if err != nil {
				return nil, errors.New("access denied")
			}
		}
	} else if repo.Visibility != "public" {
		// Not authenticated and repo is private
		return nil, errors.New("authentication required")
	}

	return repo, nil
}

// createRepository handles repository creation
func (c *ReposController) createRepository(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("authentication required"))
		return
	}

	// Check if user can create repositories (admin only)
	if !models.CanUserCreateRepo(user) {
		c.Render(w, r, "error-message.html", errors.New("only administrators can create repositories"))
		return
	}

	// Validate required fields
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		c.Render(w, r, "error-message.html", errors.New("repository name required"))
		return
	}

	// Create repository with standard fields
	repo := &models.Repository{
		Name:        name,
		Description: strings.TrimSpace(r.FormValue("description")),
		Visibility:  r.FormValue("visibility"),
		UserID:      user.ID,
	}

	// Insert into database
	repo, err = models.Repositories.Insert(repo)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to create repository: "+err.Error()))
		return
	}

	// Git repository will be auto-created by gitkit when first accessed

	// Grant owner admin permission
	err = models.GrantPermission(user.ID, repo.ID, models.RoleAdmin)
	if err != nil {
		log.Printf("Warning: Failed to grant admin permission to owner: %v", err)
	}

	// Log activity
	models.LogActivity("repo_created", "Created repository: "+repo.Name, "New repository initialized", user.ID, repo.ID, "repository", "")

	// Redirect to new repository
	c.Redirect(w, r, "/repos/"+repo.ID)
}

// updateRepository handles repository settings updates
func (c *ReposController) updateRepository(w http.ResponseWriter, r *http.Request) {
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

	// Get repository first to check ownership
	repo, err := models.Repositories.Get(repoID)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("repository not found"))
		return
	}

	// Check if user can update repository (admin or owner)
	if !models.IsUserAdmin(user) && repo.UserID != user.ID {
		c.Render(w, r, "error-message.html", errors.New("insufficient permissions"))
		return
	}


	// Update fields
	if name := strings.TrimSpace(r.FormValue("name")); name != "" {
		repo.Name = name
	}
	if desc := strings.TrimSpace(r.FormValue("description")); desc != "" {
		repo.Description = desc
	}
	repo.Visibility = r.FormValue("visibility")

	// Save changes
	err = models.Repositories.Update(repo)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to update repository"))
		return
	}

	// Log activity
	models.LogActivity("repo_updated", "Updated repository settings",
		"Repository configuration changed", user.ID, repo.ID, "repository", "")

	// Refresh the page
	c.Refresh(w, r)
}

// deleteRepository handles repository deletion
func (c *ReposController) deleteRepository(w http.ResponseWriter, r *http.Request) {
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

	// Get repository for deletion
	repo, err := models.Repositories.Get(repoID)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("repository not found"))
		return
	}

	// Check if user can delete repository (admin or owner)
	if !models.CanUserDeleteRepo(user, repo) {
		c.Render(w, r, "error-message.html", errors.New("insufficient permissions"))
		return
	}

	// Delete repository directory
	if err := os.RemoveAll(repo.Path()); err != nil {
		log.Printf("Warning: Failed to delete repository directory: %v", err)
	}

	// Delete from database
	err = models.Repositories.Delete(repo)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to delete repository"))
		return
	}

	// Log activity
	models.LogActivity("repo_deleted", "Deleted repository: "+repo.Name,
		"Repository permanently removed", user.ID, repo.ID, "repository", "")

	// Redirect to repositories list
	c.Redirect(w, r, "/repos")
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

	// Check if it's a file or directory
	if path != "." && path != "" && repo.FileExists(branch, path) {
		// It's a file, get its content
		file, err := repo.GetFile(branch, path)
		if err != nil {
			return []*FileInfo{}, nil
		}

		// Get modification time for this single file
		modTime, err := repo.GetFileModTime(branch, file.Path)
		if err != nil || modTime.IsZero() {
			modTime = time.Now() // Fallback to current time
		}
		
		fileInfo := &FileInfo{
			Name:     file.Name,
			Path:     file.Path,
			IsDir:    false,
			Size:     file.Size,
			ModTime:  modTime,
			IsBinary: file.IsBinary,
			Language: file.Language,
		}

		if !file.IsBinary {
			fileInfo.Content = file.Content
		}

		return []*FileInfo{fileInfo}, nil
	}

	// List directory contents
	nodes, err := repo.GetFileTree(branch, path)
	if err != nil {
		return nil, err
	}

	var files []*FileInfo
	for _, node := range nodes {
		fileInfo := &FileInfo{
			Name:     node.Name,
			Path:     node.Path,
			IsDir:    node.Type == "dir",
			Size:     node.Size,
			ModTime:  node.ModTime, // Now populated from git history
			Language: getLanguageFromExtension(filepath.Ext(node.Name)),
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

// FileLinesWithNumbers returns the file lines with their line numbers (1-based)
func (c *ReposController) FileLinesWithNumbers() ([]struct{Number int; Content string}, error) {
	lines, err := c.FileLines()
	if err != nil {
		return nil, err
	}
	
	result := make([]struct{Number int; Content string}, len(lines))
	for i, line := range lines {
		result[i].Number = i + 1
		result[i].Content = line
	}
	return result, nil
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
	
	// If no branch specified, use default
	if branch == "" {
		branch = repo.GetDefaultBranch()
	}
	
	// If still no branch, the repository is empty
	if branch == "" {
		return nil, nil // Return nil to show the "File Not Found" UI
	}

	// Check if it's a directory first
	if repo.IsDirectory(branch, path) {
		// It's a directory - for now use current time for directories
		return &FileInfo{
			Name:    filepath.Base(path),
			Path:    path,
			IsDir:   true,
			ModTime: time.Now(),
		}, nil
	}

	// Check if file exists
	if !repo.FileExists(branch, path) {
		return nil, nil // Return nil to show the "File Not Found" UI
	}

	// Get file content
	file, err := repo.GetFile(branch, path)
	if err != nil {
		return nil, err
	}

	// Get modification time for this file
	modTime, err := repo.GetFileModTime(branch, file.Path)
	if err != nil || modTime.IsZero() {
		modTime = time.Now() // Fallback to current time
	}
	
	fileInfo := &FileInfo{
		Name:     file.Name,
		Path:     file.Path,
		IsDir:    false,
		Size:     file.Size,
		ModTime:  modTime,
		IsBinary: file.IsBinary,
		Language: file.Language,
	}

	if !file.IsBinary {
		fileInfo.Content = file.Content
	}

	return fileInfo, nil
}

// RepoCommits returns recent commits for the repository
func (c *ReposController) RepoCommits() ([]*models.Commit, error) {
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
func (c *ReposController) RepoBranches() ([]*models.Branch, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	return repo.GetBranches()
}

// RepoCommitDiff returns the diff for a specific commit
func (c *ReposController) RepoCommitDiff() (*models.Diff, error) {
	commit := c.Request.PathValue("commit")
	if commit == "" {
		return nil, errors.New("commit hash required")
	}

	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	return repo.GetCommitDiff(commit)
}

// RepoCommitDiffContent returns the full diff content for a specific commit
func (c *ReposController) RepoCommitDiffContent() (string, error) {
	commit := c.Request.PathValue("commit")
	if commit == "" {
		return "", errors.New("commit hash required")
	}

	repo, err := c.CurrentRepo()
	if err != nil {
		return "", err
	}

	return repo.GetCommitDiffContent(commit)
}

// RepoLanguageStats returns language statistics for the repository
func (c *ReposController) RepoLanguageStats() (map[string]int, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	return repo.GetLanguageStats()
}

// RepoContributors returns the list of contributors for the repository
func (c *ReposController) RepoContributors() ([]*models.Contributor, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	return repo.GetContributors()
}

// RepoFileCount returns the total number of files in the repository
func (c *ReposController) RepoFileCount() (int, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return 0, err
	}

	return repo.GetFileCount()
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
	// Get the repository to use its Path() method
	repo, err := models.Repositories.Get(repoID)
	if err != nil {
		return nil, errors.New("repository not found")
	}
	
	repoPath := repo.Path()
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
	nodes, err := repo.GetFileTree(branch, ".")
	return err == nil && len(nodes) > 0
}

// RepoIsEmpty checks if the current repository has no commits
func (c *ReposController) RepoIsEmpty() bool {
	repo, err := c.CurrentRepo()
	if err != nil {
		return true // If we can't get the repo, treat as empty
	}
	return repo.IsEmpty()
}

// RepoActivities returns activities for the current repository with an optional limit
func (c *ReposController) RepoActivities(limit ...int) ([]*models.Activity, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	// Default limit is 10, but can be overridden
	activityLimit := 10
	if len(limit) > 0 && limit[0] > 0 {
		activityLimit = limit[0]
	}

	return repo.GetRecentActivities(activityLimit)
}

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

	// Use the new GetREADME method
	file, err := repo.GetREADME(branch)
	if err != nil || file == nil {
		return nil, nil
	}

	fileInfo := &FileInfo{
		Name:     file.Name,
		Path:     file.Path,
		IsDir:    false,
		Size:     file.Size,
		Content:  file.Content,
		Language: file.Language,
		IsBinary: file.IsBinary,
	}

	return fileInfo, nil // No README found
}

// RenderMarkdown converts markdown content to HTML using goldmark
func (c *ReposController) RenderMarkdown(content string) template.HTML {
	// Create a new goldmark markdown processor with GitHub Flavored Markdown extensions
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,           // GitHub Flavored Markdown (tables, strikethrough, etc.)
			extension.Linkify,       // Auto-linkify URLs
			extension.TaskList,      // Task list support
			extension.Typographer,   // Smart punctuation
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(), // Auto-generate heading IDs for anchors
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(), // Preserve line breaks
			html.WithXHTML(),     // XHTML output
			// Note: WithUnsafe() would allow raw HTML, but we'll keep it safe for security
		),
	)

	// Convert markdown to HTML
	var buf bytes.Buffer
	if err := md.Convert([]byte(content), &buf); err != nil {
		// If conversion fails, return the original content escaped
		return template.HTML(template.HTMLEscapeString(content))
	}

	// Post-process the HTML to add Tailwind classes
	htmlStr := buf.String()
	
	// Add classes to elements for DaisyUI/Tailwind styling
	htmlStr = strings.ReplaceAll(htmlStr, "<pre>", `<pre class="bg-base-200 p-4 rounded overflow-x-auto">`)
	htmlStr = strings.ReplaceAll(htmlStr, "<code>", `<code class="bg-base-200 px-1 rounded text-sm">`)
	htmlStr = strings.ReplaceAll(htmlStr, "<blockquote>", `<blockquote class="border-l-4 border-primary pl-4 italic">`)
	htmlStr = strings.ReplaceAll(htmlStr, "<table>", `<table class="table table-zebra">`)
	htmlStr = strings.ReplaceAll(htmlStr, "<a ", `<a class="link link-primary" `)
	htmlStr = strings.ReplaceAll(htmlStr, "<ul>", `<ul class="list-disc list-inside">`)
	htmlStr = strings.ReplaceAll(htmlStr, "<ol>", `<ol class="list-decimal list-inside">`)
	
	// Fix code blocks that are inside pre tags (remove the extra styling)
	htmlStr = regexp.MustCompile(`<pre[^>]*><code class="[^"]*">([^<]*)</code></pre>`).ReplaceAllString(
		htmlStr, 
		`<pre class="bg-base-200 p-4 rounded overflow-x-auto"><code>$1</code></pre>`,
	)
	
	return template.HTML(htmlStr)
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

	// Update the file
	err = repo.WriteFile(branch, filePath, content, commitMessage, user.Name, user.Email)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to save file: "+err.Error()))
		return
	}

	// Log activity
	models.LogActivity(
		"file_updated",
		fmt.Sprintf("Updated file %s", filePath),
		fmt.Sprintf("Updated file %s in branch %s with message: %s", filePath, branch, commitMessage),
		user.ID,
		repo.ID,
		"file",
		filePath,
	)

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

	// Create the file
	err = repo.WriteFile(branch, filePath, content, commitMessage, user.Name, user.Email)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to create file: "+err.Error()))
		return
	}
	
	// Log activity
	models.LogActivity(
		"file_created",
		fmt.Sprintf("Created file %s", filePath),
		fmt.Sprintf("Created file %s in branch %s with message: %s", filePath, branch, commitMessage),
		user.ID,
		repo.ID,
		"file",
		filePath,
	)

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

	// Delete the file
	err = repo.DeleteFile(branch, filePath, commitMessage, user.Name, user.Email)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to delete file: "+err.Error()))
		return
	}

	// Log activity
	models.LogActivity(
		"file_deleted",
		fmt.Sprintf("Deleted file %s", filePath),
		fmt.Sprintf("Deleted file %s from branch %s with message: %s", filePath, branch, commitMessage),
		user.ID,
		repo.ID,
		"file",
		filePath,
	)

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
	repo, err := models.Repositories.Get(repoID)
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
	cloneURL := fmt.Sprintf("http://%s:%s@localhost/repo/%s", token.ID, token.Token, repo.ID)

	// Execute git clone or pull in the coder container
	// Use proper shell escaping for repository names that might contain spaces
	escapedName := strings.ReplaceAll(repo.Name, "'", "'\\''")
	
	// Check if repository already exists and update it, otherwise clone it
	cloneCmd := fmt.Sprintf(`
		cd /home/coder/project && 
		if [ -d '%s/.git' ]; then
			echo "Repository already exists, updating..." &&
			cd '%s' &&
			git fetch origin &&
			git pull origin $(git symbolic-ref --short HEAD 2>/dev/null || echo 'master') &&
			echo "Repository updated successfully"
		else
			echo "Cloning repository..." &&
			rm -rf '%s' &&
			git clone '%s' '%s' &&
			echo "Repository cloned successfully"
		fi
	`, escapedName, escapedName, escapedName, cloneURL, escapedName)

	// Execute in the coder container
	output, err := exec.Command("docker", "exec", "skyscape-coder", "bash", "-c", cloneCmd).CombinedOutput()
	if err != nil {
		c.Render(w, r, "error-message.html", fmt.Errorf("failed to clone/update repository: %s", string(output)))
		return
	}

	// Determine if it was updated or cloned based on output
	action := "prepared"
	if strings.Contains(string(output), "updated successfully") {
		action = "updated"
	} else if strings.Contains(string(output), "cloned successfully") {
		action = "cloned"
	}
	
	// Return success message with link to open IDE
	// Use repo.Name for the folder path since that's what we clone it as
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `
		<div class="alert alert-success">
			<svg xmlns="http://www.w3.org/2000/svg" class="stroke-current shrink-0 h-6 w-6" fill="none" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
			</svg>
			<div>
				<div class="font-bold">Repository ready in IDE!</div>
				<div class="text-sm">The repository has been %s at /home/coder/project/%s</div>
			</div>
			<a href="/coder/?folder=/home/coder/project/%s" target="_blank" class="btn btn-sm btn-primary">Open IDE</a>
		</div>
	`, action, repo.Name, url.QueryEscape(repo.Name))
}

// importRepository imports a repository from GitHub
func (c *ReposController) importRepository(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("authentication required"))
		return
	}

	// Get form data
	githubURL := strings.TrimSpace(r.FormValue("github_url"))
	name := strings.TrimSpace(r.FormValue("name"))
	visibility := r.FormValue("visibility")
	setupIntegration := r.FormValue("setup_integration") == "on"

	// Parse GitHub URL
	if !strings.HasPrefix(githubURL, "https://github.com/") {
		c.Render(w, r, "error-message.html", errors.New("invalid GitHub URL"))
		return
	}

	// Extract repo name from URL if not provided
	urlParts := strings.Split(strings.TrimPrefix(githubURL, "https://github.com/"), "/")
	if len(urlParts) < 2 {
		c.Render(w, r, "error-message.html", errors.New("invalid GitHub repository URL"))
		return
	}
	
	if name == "" {
		name = strings.TrimSuffix(urlParts[1], ".git")
	}

	// Validate repository name (basic validation)
	if len(name) < 2 || len(name) > 40 {
		c.Render(w, r, "error-message.html", errors.New("repository name must be 2-40 characters"))
		return
	}

	// Create repository in database
	repo := &models.Repository{
		Name:        name,
		Description: fmt.Sprintf("Imported from %s", githubURL),
		UserID:      user.ID,
		Visibility:  visibility,
	}

	repo, err = models.Repositories.Insert(repo)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to create repository: "+err.Error()))
		return
	}

	// Clone the GitHub repository
	repoPath := repo.Path()
	cmd := exec.Command("git", "clone", githubURL, repoPath)
	if err := cmd.Run(); err != nil {
		// Clean up on failure
		models.Repositories.Delete(repo)
		c.Render(w, r, "error-message.html", errors.New("failed to clone repository: "+err.Error()))
		return
	}

	// Set up GitHub integration if requested
	if setupIntegration {
		// Add GitHub as remote for future integration
		cmd = exec.Command("git", "remote", "add", "github", githubURL)
		cmd.Dir = repoPath
		cmd.Run() // Ignore error if remote already exists
		
		// TODO: Store integration settings when Integration model is available
	}

	// Log activity
	models.LogActivity("repo_imported", fmt.Sprintf("Imported repository %s from GitHub", name),
		"Repository imported from GitHub", user.ID, repo.ID, "import", "")

	// Redirect to the new repository
	c.Redirect(w, r, fmt.Sprintf("/repos/%s", repo.ID))
}

// searchRepositories handles HTMX search requests for repositories
func (c *ReposController) searchRepositories(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.Render(w, r, "repos-list-partial.html", nil)
		return
	}

	// Get search query
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	filter := r.URL.Query().Get("filter") // all, public, private

	// Get user's repositories
	repos, err := c.SearchUserRepositories(user, query, filter)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Render partial for HTMX
	c.Render(w, r, "repos-list-partial.html", repos)
}

// SearchUserRepositories searches for repositories by name or description
func (c *ReposController) SearchUserRepositories(user *authentication.User, query, filter string) ([]*models.Repository, error) {
	// Get all user repositories
	userRepos, err := models.ListUserRepositories(user.ID)
	if err != nil {
		return nil, err
	}

	var filtered []*models.Repository
	queryLower := strings.ToLower(query)

	for _, repo := range userRepos {
		// Apply visibility filter
		if filter != "" && filter != "all" {
			if filter != repo.Visibility {
				continue
			}
		}

		// If no query, include all (after filter)
		if query == "" {
			filtered = append(filtered, repo)
			continue
		}

		// Search in name and description
		if strings.Contains(strings.ToLower(repo.Name), queryLower) ||
			strings.Contains(strings.ToLower(repo.Description), queryLower) {
			filtered = append(filtered, repo)
		}
	}

	return filtered, nil
}