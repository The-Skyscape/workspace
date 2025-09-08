package controllers

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"workspace/models"

	"github.com/The-Skyscape/devtools/pkg/authentication"
)

// SearchResult represents a code search result
type SearchResult struct {
	File     string   // Filename
	Path     string   // Relative path from repo root
	LineNum  int      // Line number (1-indexed)
	Line     string   // The matching line
	Context  []string // Context lines around the match
	Language string   // Programming language
}

// SearchCode searches for code within the current repository
// Walks the file tree, skips binary files, and returns matches with context
// Returns up to 100 results to prevent overwhelming the UI
func (c *ReposController) SearchCode() ([]*SearchResult, error) {
	repo, err := c.CurrentRepo()
	if err != nil {
		return nil, err
	}

	query := c.Request.URL.Query().Get("q")
	if query == "" {
		return []*SearchResult{}, nil
	}

	// Delegate to the internal search method
	return c.searchInRepository(repo.ID, query)
}

// searchInRepository performs the actual file search within a repository
// This is a private helper that does the heavy lifting of searching
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
			// Skip .git directory entirely
			if info.IsDir() && info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip large files (>1MB) to prevent memory issues
		if info.Size() > 1024*1024 {
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
// Returns 'contextSize' lines before and after the target line
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

// searchRepositories handles repository search via HTMX
// Searches repository names and descriptions
func (c *ReposController) searchRepositories(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	filter := r.URL.Query().Get("filter") // "all", "public", "private"

	// Get current user
	auth := c.App.Use("auth").(*AuthController)
	user, _, err := auth.Authenticate(r)
	if err != nil {
		c.RenderError(w, r, err)
		return
	}

	repos, err := c.searchUserRepositories(user, query, filter)
	if err != nil {
		c.RenderError(w, r, err)
		return
	}

	// Render partial with results
	c.Render(w, r, "repos-list-partial.html", repos)
}

// searchUserRepositories searches repositories accessible to a user
// This should be made private as it takes a user parameter (HATEOAS violation)
func (c *ReposController) searchUserRepositories(user *authentication.User, query, filter string) ([]*models.Repository, error) {
	var conditions []string
	var args []interface{}

	// Base condition: user's repositories or public ones
	if user.IsAdmin {
		// Admin sees all
		conditions = append(conditions, "1=1")
	} else {
		conditions = append(conditions, "(UserID = ? OR Visibility = 'public')")
		args = append(args, user.ID)
	}

	// Add search query if provided
	if query != "" {
		conditions = append(conditions, "(LOWER(Name) LIKE LOWER(?) OR LOWER(Description) LIKE LOWER(?))")
		searchPattern := "%" + query + "%"
		args = append(args, searchPattern, searchPattern)
	}

	// Add visibility filter
	switch filter {
	case "public":
		conditions = append(conditions, "Visibility = 'public'")
	case "private":
		conditions = append(conditions, "Visibility = 'private'")
		// "all" or default: no additional filter
	}

	// Build query
	whereClause := strings.Join(conditions, " AND ")
	fullQuery := "WHERE " + whereClause + " ORDER BY UpdatedAt DESC LIMIT 50"

	return models.Repositories.Search(fullQuery, args...)
}
