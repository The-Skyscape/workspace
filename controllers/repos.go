package controllers

import (
	"errors"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"workspace/models"
	"workspace/internal/coding"

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
	http.Handle("GET /repos/{id}/commits", app.Serve("repo-commits.html", auth.Required))
	http.Handle("GET /repos/{id}/search", app.Serve("repo-search.html", auth.Required))

	http.Handle("POST /repos/create", app.ProtectFunc(c.createRepo, auth.Required))
	http.Handle("POST /repos/{id}/launch-workspace", app.ProtectFunc(c.launchWorkspace, auth.Required))
	http.Handle("POST /repos/{id}/actions/create", app.ProtectFunc(c.createAction, auth.Required))
	http.Handle("POST /repos/{id}/issues/create", app.ProtectFunc(c.createIssue, auth.Required))
	http.Handle("POST /repos/{id}/issues/{issueID}/close", app.ProtectFunc(c.closeIssue, auth.Required))
	http.Handle("POST /repos/{id}/issues/{issueID}/reopen", app.ProtectFunc(c.reopenIssue, auth.Required))
	http.Handle("POST /repos/{id}/prs/create", app.ProtectFunc(c.createPR, auth.Required))
	http.Handle("POST /repos/{id}/prs/{prID}/merge", app.ProtectFunc(c.mergePR, auth.Required))
	http.Handle("POST /repos/{id}/prs/{prID}/close", app.ProtectFunc(c.closePR, auth.Required))
	http.Handle("POST /repos/{id}/permissions", app.ProtectFunc(c.grantPermission, auth.Required))
	http.Handle("DELETE /repos/{id}/permissions/{userID}", app.ProtectFunc(c.revokePermission, auth.Required))

	http.Handle("/repo/", http.StripPrefix("/repo/", models.Coding.GitServer(auth)))
}

// Handle is called when each request is handled
func (c *ReposController) Handle(req *http.Request) application.Controller {
	c.Request = req
	return c
}

// CurrentRepo returns the repository from the URL path
func (c *ReposController) CurrentRepo() (*coding.GitRepo, error) {
	return c.getCurrentRepoFromRequest(c.Request)
}

// getCurrentRepoFromRequest returns the repository from a specific request with permission checking
func (c *ReposController) getCurrentRepoFromRequest(r *http.Request) (*coding.GitRepo, error) {
	id := r.PathValue("id")
	if id == "" {
		return nil, errors.New("repository ID not found")
	}

	repo, err := models.Coding.GetRepo(id)
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
		models.Coding.UpdateRepo(repo)
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

// RepoPullRequests returns pull requests for the current repository
func (c *ReposController) RepoPullRequests() ([]*models.PullRequest, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	return models.PullRequests.Search("WHERE RepoID = ? ORDER BY CreatedAt DESC", repo.ID)
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
	repo, err := models.Coding.NewRepo(repoID, name)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to create repository: "+err.Error()))
		return
	}

	// Set repository metadata and ownership
	repo.UserID = user.ID
	repo.Description = description
	repo.Visibility = visibility

	// Save the updated repository
	if err = models.Coding.UpdateRepo(repo); err != nil {
		c.Render(w, r, "error-message.html", errors.New("failed to update repository: "+err.Error()))
		return
	}

	// Grant admin permissions to creator for explicit permission tracking
	// This is complementary to the UserID ownership check
	if err = models.GrantPermission(user.ID, repo.ID, models.RoleAdmin); err != nil {
		// Log warning but don't fail - UserID ownership is primary mechanism
		// TODO: Add proper logging
	}

	// Log the repository creation activity
	models.LogActivity("repo_created", "Created repository "+repo.Name,
		"New "+repo.Visibility+" repository created", user.ID, repo.ID, "repository", repo.ID)

	// Redirect to the new repository
	http.Redirect(w, r, "/repos/"+repo.ID, http.StatusSeeOther)
}

// launchWorkspace handles workspace creation for a repository
func (c *ReposController) launchWorkspace(w http.ResponseWriter, r *http.Request) {
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

	// Check repository access permissions (write required for workspace launch)
	err = models.CheckRepoAccess(user, repoID, models.RoleWrite)
	if err != nil {
		c.Render(w, r, "error-message.html", errors.New("insufficient permissions to launch workspace"))
		return
	}

	repo, err := models.Coding.GetRepo(repoID)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Check if workspace already exists
	existingWorkspace, err := models.Coding.GetWorkspace(user.ID)
	if err == nil && existingWorkspace != nil {
		http.Redirect(w, r, "/workspace/"+existingWorkspace.ID, http.StatusSeeOther)
		return
	}

	// Get available port
	workspaces, _ := models.Coding.Workspaces()
	port := 8000 + len(workspaces)

	// Create new workspace using coding package
	workspace, err := models.Coding.NewWorkspace(user.ID, port, repo)
	if err != nil {
		c.Render(w, r, "error-message.html", err)
		return
	}

	// Log the workspace launch activity
	models.LogActivity("workspace_launched", "Launched workspace for "+repo.Name,
		"Development workspace created", user.ID, repo.ID, "workspace", workspace.ID)

	// Redirect to workspace launcher
	http.Redirect(w, r, "/workspace/"+workspace.ID, http.StatusSeeOther)
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

	// Redirect back to settings
	http.Redirect(w, r, "/repos/"+repoID+"/settings", http.StatusSeeOther)
}

// revokePermission handles revoking permissions from users
func (c *ReposController) revokePermission(w http.ResponseWriter, r *http.Request) {
	auth := c.Use("auth").(*authentication.Controller)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	repoID := r.PathValue("id")
	targetUserID := r.PathValue("userID")

	if repoID == "" || targetUserID == "" {
		http.Error(w, "Repository ID and User ID required", http.StatusBadRequest)
		return
	}

	// Check admin permissions for revoking permissions
	err = models.CheckRepoAccess(user, repoID, models.RoleAdmin)
	if err != nil {
		http.Error(w, "Insufficient permissions", http.StatusForbidden)
		return
	}

	// Revoke permission
	err = models.RevokePermission(targetUserID, repoID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
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

	path := c.Request.PathValue("path")
	if path == "" {
		path = "."
	}

	// Get repository file system path
	repoPath := filepath.Join("repos", repo.ID)
	fullPath := filepath.Join(repoPath, path)

	// Check if path exists and is within repo
	if !strings.HasPrefix(fullPath, repoPath) {
		return nil, errors.New("invalid path")
	}

	var files []*FileInfo

	// Check if it's a file or directory
	info, err := os.Stat(fullPath)
	if err != nil {
		// If repo directory doesn't exist, return empty list
		if os.IsNotExist(err) {
			return files, nil
		}
		return nil, err
	}

	if info.IsDir() {
		// List directory contents
		entries, err := os.ReadDir(fullPath)
		if err != nil {
			return nil, err
		}

		for _, entry := range entries {
			// Skip hidden files and .git directory
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}

			info, err := entry.Info()
			if err != nil {
				continue
			}

			fileInfo := &FileInfo{
				Name:    entry.Name(),
				Path:    filepath.Join(path, entry.Name()),
				IsDir:   entry.IsDir(),
				Size:    info.Size(),
				ModTime: info.ModTime(),
			}

			files = append(files, fileInfo)
		}
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

	// Get repository file system path
	repoPath := filepath.Join("repos", repo.ID)
	fullPath := filepath.Join(repoPath, path)

	// Security check
	if !strings.HasPrefix(fullPath, repoPath) {
		return nil, errors.New("invalid path")
	}

	// Get file info
	info, err := os.Stat(fullPath)
	if err != nil {
		return nil, err
	}

	fileInfo := &FileInfo{
		Name:    filepath.Base(path),
		Path:    path,
		IsDir:   info.IsDir(),
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}

	// If it's a file, read content
	if !info.IsDir() {
		content, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, err
		}

		// Check if binary
		fileInfo.IsBinary = isBinary(content)
		if !fileInfo.IsBinary {
			fileInfo.Content = string(content)
		}
		fileInfo.Language = getLanguageFromExtension(filepath.Ext(path))
	}

	return fileInfo, nil
}

// RepoCommits returns recent commits for the repository
func (c *ReposController) RepoCommits() ([]map[string]interface{}, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	// For now, return mock commit data
	// TODO: Integrate with actual Git command or library
	commits := []map[string]interface{}{
		{
			"hash":    "abc123",
			"message": "Initial commit",
			"author":  "Developer",
			"date":    time.Now().AddDate(0, 0, -1),
		},
		{
			"hash":    "def456",
			"message": "Add README",
			"author":  "Developer",
			"date":    time.Now().AddDate(0, 0, -2),
		},
	}

	// Log activity for commit viewing
	auth := c.Use("auth").(*authentication.Controller)
	if user, _, err := auth.Authenticate(c.Request); err == nil {
		models.LogActivity("commits_viewed", "Viewed commits for "+repo.Name,
			"Browsed repository commit history", user.ID, repo.ID, "commits", "")
	}

	return commits, nil
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

// RepoReadme detects and returns README content for the repository
func (c *ReposController) RepoReadme() (*FileInfo, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	// Look for README files in repository root
	repoPath := filepath.Join("repos", repo.ID)
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
		fullPath := filepath.Join(repoPath, filename)
		if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
			content, err := os.ReadFile(fullPath)
			if err != nil {
				continue
			}

			// Check if binary
			if isBinary(content) {
				continue
			}

			fileInfo := &FileInfo{
				Name:     filename,
				Path:     filename,
				IsDir:    false,
				Size:     info.Size(),
				ModTime:  info.ModTime(),
				Content:  string(content),
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

	// Redirect to issues page
	http.Redirect(w, r, "/repos/"+repoID+"/issues", http.StatusSeeOther)
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

	w.WriteHeader(http.StatusOK)
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

	w.WriteHeader(http.StatusOK)
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

	// Redirect to PRs page
	http.Redirect(w, r, "/repos/"+repoID+"/prs", http.StatusSeeOther)
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

	// Get and update PR
	pr, err := models.PullRequests.Get(prID)
	if err != nil {
		http.Error(w, "pull request not found", http.StatusNotFound)
		return
	}

	pr.Status = "merged"
	err = models.PullRequests.Update(pr)
	if err != nil {
		http.Error(w, "failed to merge pull request", http.StatusInternalServerError)
		return
	}

	// Log activity
	models.LogActivity("pr_merged", "Merged pull request: "+pr.Title,
		"Pull request merged", user.ID, repoID, "pull_request", pr.ID)

	w.WriteHeader(http.StatusOK)
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

	w.WriteHeader(http.StatusOK)
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
