package controllers

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"workspace/models"
)

// FileInfo represents information about a file in the repository
type FileInfo struct {
	Name        string
	Path        string
	IsDirectory bool
	Size        int64
	Modified    time.Time
	Content     string // Only populated for files being viewed
	Language    string // Programming language for syntax highlighting
}

// RepoFiles returns the files in the current repository directory
// Used by the file browser view to display repository contents
func (c *ReposController) RepoFiles() ([]*FileInfo, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	// Get the current path from query params (default to root)
	currentPath := c.Request.URL.Query().Get("path")
	if currentPath == "" {
		currentPath = "."
	}

	// Ensure the path is safe (no directory traversal)
	if strings.Contains(currentPath, "..") {
		return nil, errors.New("invalid path")
	}

	// Construct the full path
	fullPath := filepath.Join(repo.Path(), currentPath)

	// Check if the path exists and is within the repo
	if !isSubPath(repo.Path(), fullPath) {
		return nil, errors.New("path outside repository")
	}

	// Read directory contents
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return nil, err
	}

	var files []*FileInfo
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
			Name:        entry.Name(),
			Path:        filepath.Join(currentPath, entry.Name()),
			IsDirectory: entry.IsDir(),
			Size:        info.Size(),
			Modified:    info.ModTime(),
		}

		// Determine language for files
		if !entry.IsDir() {
			fileInfo.Language = getLanguageFromExtension(filepath.Ext(entry.Name()))
		}

		files = append(files, fileInfo)
	}

	return files, nil
}

// FileLines returns the lines of the current file being viewed
// Used for displaying file content with line numbers
func (c *ReposController) FileLines() ([]string, error) {
	file, err := c.CurrentFile()
	if err != nil {
		return nil, err
	}
	return strings.Split(file.Content, "\n"), nil
}

// FileLinesWithNumbers returns the file lines with their line numbers (1-based)
// Used by templates that need to display line numbers alongside content
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

// CurrentFile returns the file currently being viewed based on the path parameter
// Reads the file content and determines its programming language
func (c *ReposController) CurrentFile() (*FileInfo, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	// Get the file path from URL
	filePath := c.Request.URL.Query().Get("path")
	if filePath == "" {
		return nil, errors.New("no file specified")
	}

	// Ensure the path is safe
	if strings.Contains(filePath, "..") {
		return nil, errors.New("invalid file path")
	}

	// Construct the full path
	fullPath := filepath.Join(repo.Path(), filePath)

	// Check if the path is within the repo
	if !isSubPath(repo.Path(), fullPath) {
		return nil, errors.New("file outside repository")
	}

	// Get file info
	info, err := os.Stat(fullPath)
	if err != nil {
		return nil, err
	}

	if info.IsDir() {
		return nil, errors.New("path is a directory")
	}

	// Read file content (limit to 10MB to prevent memory issues)
	if info.Size() > 10*1024*1024 {
		return &FileInfo{
			Name:     info.Name(),
			Path:     filePath,
			Size:     info.Size(),
			Modified: info.ModTime(),
			Content:  "File too large to display",
			Language: getLanguageFromExtension(filepath.Ext(info.Name())),
		}, nil
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, err
	}

	// Check if binary
	if isBinary(content) {
		return &FileInfo{
			Name:     info.Name(),
			Path:     filePath,
			Size:     info.Size(),
			Modified: info.ModTime(),
			Content:  "Binary file cannot be displayed",
			Language: "binary",
		}, nil
	}

	return &FileInfo{
		Name:     info.Name(),
		Path:     filePath,
		Size:     info.Size(),
		Modified: info.ModTime(),
		Content:  string(content),
		Language: getLanguageFromExtension(filepath.Ext(info.Name())),
	}, nil
}

// RepoReadme finds and returns the README file in the repository root
// Checks for README.md, README.markdown, README.txt, and README
func (c *ReposController) RepoReadme() (*FileInfo, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	// Check for common README filenames
	readmeNames := []string{"README.md", "README.markdown", "README.txt", "README", "readme.md"}
	
	for _, name := range readmeNames {
		fullPath := filepath.Join(repo.Path(), name)
		info, err := os.Stat(fullPath)
		if err != nil {
			continue // File doesn't exist, try next
		}

		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}

		fileInfo := &FileInfo{
			Name:     name,
			Path:     name,
			Size:     info.Size(),
			Modified: info.ModTime(),
			Content:  string(content),
			Language: getLanguageFromExtension(filepath.Ext(name)),
		}

		return fileInfo, nil
	}

	return nil, nil // No README found
}

// HasFiles checks if the repository has any files
func (c *ReposController) HasFiles() bool {
	repo, err := c.CurrentRepo()
	if err != nil {
		return false
	}

	entries, err := os.ReadDir(repo.Path())
	if err != nil {
		return false
	}

	// Check if there are any non-hidden files
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".") {
			return true
		}
	}
	return false
}

// RepoIsEmpty checks if the repository is empty (no commits)
func (c *ReposController) RepoIsEmpty() bool {
	repo, err := c.CurrentRepo()
	if err != nil {
		return true
	}
	return repo.IsEmpty()
}

// RepoFileCount returns the total number of files in the repository
func (c *ReposController) RepoFileCount() (int, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return 0, err
	}

	count := 0
	err = filepath.Walk(repo.Path(), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files with errors
		}
		if !info.IsDir() && !strings.HasPrefix(info.Name(), ".") {
			count++
		}
		return nil
	})

	return count, err
}

// saveFile handles saving changes to an existing file
func (c *ReposController) saveFile(w http.ResponseWriter, r *http.Request) {
	repo, err := c.getCurrentRepoFromRequest(r)
	if err != nil {
		c.RenderError(w, r, err)
		return
	}

	// Get file path from form
	filePath := r.FormValue("path")
	if filePath == "" {
		c.RenderErrorMsg(w, r, "file path required")
		return
	}

	// Validate path
	if strings.Contains(filePath, "..") {
		c.RenderErrorMsg(w, r, "invalid file path")
		return
	}

	fullPath := filepath.Join(repo.Path(), filePath)
	if !isSubPath(repo.Path(), fullPath) {
		c.RenderErrorMsg(w, r, "file outside repository")
		return
	}

	// Get new content
	content := r.FormValue("content")

	// Write file
	err = os.WriteFile(fullPath, []byte(content), 0644)
	if err != nil {
		c.RenderError(w, r, err)
		return
	}

	// Log activity
	models.LogActivity("file_edited", fmt.Sprintf("Edited file %s", filePath),
		fmt.Sprintf("File %s was edited in repository %s", filePath, repo.Name),
		"", repo.ID, "file", filePath)

	// Redirect back to file view
	c.Redirect(w, r, fmt.Sprintf("/repos/%s/files?path=%s", repo.ID, filePath))
}

// createFile handles creating a new file in the repository
func (c *ReposController) createFile(w http.ResponseWriter, r *http.Request) {
	repo, err := c.getCurrentRepoFromRequest(r)
	if err != nil {
		c.RenderError(w, r, err)
		return
	}

	// Get file details from form
	fileName := r.FormValue("name")
	filePath := r.FormValue("path")
	content := r.FormValue("content")

	if fileName == "" {
		c.RenderErrorMsg(w, r, "file name required")
		return
	}

	// Construct full path
	var fullPath string
	if filePath != "" && filePath != "." {
		fullPath = filepath.Join(repo.Path(), filePath, fileName)
	} else {
		fullPath = filepath.Join(repo.Path(), fileName)
	}

	// Validate path
	if !isSubPath(repo.Path(), fullPath) {
		c.RenderErrorMsg(w, r, "invalid file path")
		return
	}

	// Check if file already exists
	if _, err := os.Stat(fullPath); err == nil {
		c.RenderErrorMsg(w, r, "file already exists")
		return
	}

	// Create directory if needed
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		c.RenderError(w, r, err)
		return
	}

	// Write file
	err = os.WriteFile(fullPath, []byte(content), 0644)
	if err != nil {
		c.RenderError(w, r, err)
		return
	}

	// Log activity
	relativePath := strings.TrimPrefix(fullPath, repo.Path()+"/")
	models.LogActivity("file_created", fmt.Sprintf("Created file %s", relativePath),
		fmt.Sprintf("File %s was created in repository %s", relativePath, repo.Name),
		"", repo.ID, "file", relativePath)

	// Redirect to the new file
	c.Redirect(w, r, fmt.Sprintf("/repos/%s/files?path=%s", repo.ID, relativePath))
}

// deleteFile handles deleting a file from the repository
func (c *ReposController) deleteFile(w http.ResponseWriter, r *http.Request) {
	repo, err := c.getCurrentRepoFromRequest(r)
	if err != nil {
		c.RenderError(w, r, err)
		return
	}

	// Get file path
	filePath := r.FormValue("path")
	if filePath == "" {
		c.RenderErrorMsg(w, r, "file path required")
		return
	}

	// Validate path
	if strings.Contains(filePath, "..") {
		c.RenderErrorMsg(w, r, "invalid file path")
		return
	}

	fullPath := filepath.Join(repo.Path(), filePath)
	if !isSubPath(repo.Path(), fullPath) {
		c.RenderErrorMsg(w, r, "file outside repository")
		return
	}

	// Check if file exists
	info, err := os.Stat(fullPath)
	if err != nil {
		c.RenderError(w, r, err)
		return
	}

	// Don't allow deleting directories through this endpoint
	if info.IsDir() {
		c.RenderErrorMsg(w, r, "cannot delete directories")
		return
	}

	// Delete the file
	err = os.Remove(fullPath)
	if err != nil {
		c.RenderError(w, r, err)
		return
	}

	// Log activity
	models.LogActivity("file_deleted", fmt.Sprintf("Deleted file %s", filePath),
		fmt.Sprintf("File %s was deleted from repository %s", filePath, repo.Name),
		"", repo.ID, "file", filePath)

	// Redirect to parent directory
	parentDir := filepath.Dir(filePath)
	if parentDir == "." {
		c.Redirect(w, r, fmt.Sprintf("/repos/%s/files", repo.ID))
	} else {
		c.Redirect(w, r, fmt.Sprintf("/repos/%s/files?path=%s", repo.ID, parentDir))
	}
}

// openInIDE handles opening a file in the VS Code workspace
func (c *ReposController) openInIDE(w http.ResponseWriter, r *http.Request) {
	repo, err := c.getCurrentRepoFromRequest(r)
	if err != nil {
		c.RenderError(w, r, err)
		return
	}

	// Get file path
	filePath := r.URL.Query().Get("path")
	
	// Construct the workspace URL
	// The workspace ID is typically the repository ID
	workspaceURL := fmt.Sprintf("/coder/%s/", repo.ID)
	
	// If a specific file is requested, we can append it to the URL
	// Note: This depends on how code-server handles file opening via URL
	if filePath != "" {
		// Ensure the path is safe
		if !strings.Contains(filePath, "..") {
			// code-server can open files via query params or path
			workspaceURL = fmt.Sprintf("/coder/%s/?folder=/workspace&open=%s", repo.ID, filePath)
		}
	}

	// Redirect to the workspace
	c.Redirect(w, r, workspaceURL)
}

// Helper functions

// isSubPath checks if a path is within a parent directory
func isSubPath(parent, path string) bool {
	rel, err := filepath.Rel(parent, path)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, "..") && !strings.HasPrefix(rel, "/")
}

// isBinary checks if content appears to be binary
func isBinary(content []byte) bool {
	if len(content) == 0 {
		return false
	}
	
	// Check for null bytes in first 8KB
	checkLen := len(content)
	if checkLen > 8192 {
		checkLen = 8192
	}
	
	for i := 0; i < checkLen; i++ {
		if content[i] == 0 {
			return true
		}
	}
	return false
}

// getLanguageFromExtension returns the programming language based on file extension
func getLanguageFromExtension(ext string) string {
	languages := map[string]string{
		".go":     "go",
		".js":     "javascript",
		".ts":     "typescript",
		".py":     "python",
		".rb":     "ruby",
		".java":   "java",
		".c":      "c",
		".cpp":    "cpp",
		".h":      "c",
		".hpp":    "cpp",
		".cs":     "csharp",
		".php":    "php",
		".html":   "html",
		".css":    "css",
		".scss":   "scss",
		".json":   "json",
		".xml":    "xml",
		".yaml":   "yaml",
		".yml":    "yaml",
		".md":     "markdown",
		".sh":     "bash",
		".sql":    "sql",
		".rs":     "rust",
		".swift":  "swift",
		".kt":     "kotlin",
		".r":      "r",
		".m":      "objc",
		".vue":    "vue",
		".jsx":    "javascript",
		".tsx":    "typescript",
	}

	if lang, ok := languages[strings.ToLower(ext)]; ok {
		return lang
	}
	return "text"
}