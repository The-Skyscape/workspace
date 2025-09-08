package controllers

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"workspace/models"
)

// RepoFiles returns the files in the current repository directory
// Used by the file browser view to display repository contents
func (c *ReposController) RepoFiles() ([]*models.FileNode, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	// Get the current branch (default to repo's default branch)
	branch := c.CurrentBranch()

	// Get the current path from query params (default to root)
	currentPath := c.Request.URL.Query().Get("path")
	if currentPath == "" {
		currentPath = "."
	}

	// Ensure the path is safe (no directory traversal)
	if strings.Contains(currentPath, "..") {
		return nil, errors.New("invalid path")
	}

	// Use the Git-aware model method to get file tree
	files, err := repo.GetFileTree(branch, currentPath)
	if err != nil {
		return nil, err
	}

	// Filter out hidden files (starting with .)
	var visibleFiles []*models.FileNode
	for _, file := range files {
		if !strings.HasPrefix(file.Name, ".") {
			visibleFiles = append(visibleFiles, file)
		}
	}

	return visibleFiles, nil
}

// FileLines returns the lines of the current file being viewed
// Used for displaying file content with line numbers
func (c *ReposController) FileLines() ([]string, error) {
	file, err := c.CurrentFile()
	if err != nil {
		return nil, err
	}
	// Handle empty content
	if file.Content == "" {
		return []string{}, nil
	}
	return strings.Split(file.Content, "\n"), nil
}

// FileLinesWithNumbers returns the file lines with their line numbers (1-based)
// Used by templates that need to display line numbers alongside content
func (c *ReposController) FileLinesWithNumbers() ([]struct {
	Number  int
	Content string
}, error) {
	lines, err := c.FileLines()
	if err != nil {
		return nil, err
	}

	result := make([]struct {
		Number  int
		Content string
	}, len(lines))
	for i, line := range lines {
		result[i].Number = i + 1
		result[i].Content = line
	}
	return result, nil
}

// CurrentFile returns the file currently being viewed based on the path parameter
// Reads the file content and determines its programming language
func (c *ReposController) CurrentFile() (*models.File, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	// Get the file path from URL
	filePath := c.Request.URL.Query().Get("path")
	if filePath == "" {
		// Try getting from path parameter (for routes like /repos/{id}/files/{path...})
		filePath = c.Request.PathValue("path")
		if filePath == "" {
			return nil, errors.New("no file specified")
		}
	}

	// Ensure the path is safe
	if strings.Contains(filePath, "..") {
		return nil, errors.New("invalid file path")
	}

	// Get the current branch
	branch := c.CurrentBranch()

	// Use the Git-aware model method to get the file
	file, err := repo.GetFile(branch, filePath)
	if err != nil {
		return nil, err
	}

	// Handle large files
	if file.Size > 10*1024*1024 {
		file.Content = "File too large to display"
	}

	// Handle binary files - the model already sets IsBinary
	if file.IsBinary {
		file.Content = "Binary file cannot be displayed"
	}

	return file, nil
}

// RepoReadme finds and returns the README file in the repository root
// Checks for README.md, README.markdown, README.txt, and README
func (c *ReposController) RepoReadme() (*models.File, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	// Get the current branch
	branch := c.CurrentBranch()

	// Use the model's GetREADME method which handles common README filenames
	readme, err := repo.GetREADME(branch)
	if err != nil {
		return nil, err
	}

	return readme, nil // Returns nil if no README found
}

// HasFiles checks if the repository has any files
func (c *ReposController) HasFiles() bool {
	repo, err := c.CurrentRepo()
	if err != nil {
		return false
	}

	// Get the current branch
	branch := c.CurrentBranch()

	// Use Git-aware method to check for files
	files, err := repo.GetFileTree(branch, ".")
	if err != nil {
		return false
	}

	// Check if there are any visible files
	for _, file := range files {
		if !strings.HasPrefix(file.Name, ".") {
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

	// Get the current branch
	branch := c.CurrentBranch()

	// Use Git-aware method to count files
	count := 0
	var countFiles func(path string) error
	countFiles = func(path string) error {
		files, err := repo.GetFileTree(branch, path)
		if err != nil {
			return err
		}

		for _, file := range files {
			if strings.HasPrefix(file.Name, ".") {
				continue
			}
			if file.IsDir() {
				// Recursively count files in subdirectories
				if err := countFiles(file.Path); err != nil {
					return err
				}
			} else {
				count++
			}
		}
		return nil
	}

	err = countFiles(".")
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
		".go":    "go",
		".js":    "javascript",
		".ts":    "typescript",
		".py":    "python",
		".rb":    "ruby",
		".java":  "java",
		".c":     "c",
		".cpp":   "cpp",
		".h":     "c",
		".hpp":   "cpp",
		".cs":    "csharp",
		".php":   "php",
		".html":  "html",
		".css":   "css",
		".scss":  "scss",
		".json":  "json",
		".xml":   "xml",
		".yaml":  "yaml",
		".yml":   "yaml",
		".md":    "markdown",
		".sh":    "bash",
		".sql":   "sql",
		".rs":    "rust",
		".swift": "swift",
		".kt":    "kotlin",
		".r":     "r",
		".m":     "objc",
		".vue":   "vue",
		".jsx":   "javascript",
		".tsx":   "typescript",
		".ipynb": "jupyter",
	}

	if lang, ok := languages[strings.ToLower(ext)]; ok {
		return lang
	}
	return "text"
}
