package tools

import (
	"fmt"
	"strings"
	"workspace/models"
)

// ListFilesTool lists files and directories in a repository
type ListFilesTool struct{}

func (t *ListFilesTool) Name() string {
	return "list_files"
}

func (t *ListFilesTool) Description() string {
	return "List files and directories in a repository. Required params: repo_id. Optional params: path (directory path), branch"
}

func (t *ListFilesTool) ValidateParams(params map[string]any) error {
	repoID, exists := params["repo_id"]
	if !exists {
		return fmt.Errorf("repo_id is required")
	}

	if _, ok := repoID.(string); !ok {
		return fmt.Errorf("repo_id must be a string")
	}

	// Validate path if provided
	if path, exists := params["path"]; exists {
		if pathStr, ok := path.(string); ok {
			if strings.Contains(pathStr, "..") {
				return fmt.Errorf("invalid path: directory traversal not allowed")
			}
		}
	}

	return nil
}

func (t *ListFilesTool) Schema() map[string]any {
	return SimpleSchema(map[string]any{
		"repo_id": map[string]any{
			"type":        "string",
			"description": "The repository ID",
			"required":    true,
		},
		"path": map[string]any{
			"type":        "string",
			"description": "Directory path to list files from",
			"default":     ".",
		},
		"branch": map[string]any{
			"type":        "string",
			"description": "Branch name to list files from",
		},
	})
}

func (t *ListFilesTool) Execute(params map[string]any, userID string) (string, error) {
	repoID := params["repo_id"].(string)

	// Get user to check permissions
	user, err := models.Auth.GetUser(userID)
	if err != nil {
		return "", fmt.Errorf("failed to get user: %w", err)
	}

	// Get repository
	repo, err := models.Repositories.Get(repoID)
	if err != nil {
		return "", fmt.Errorf("repository not found: %s", repoID)
	}

	// Check permissions
	if repo.Visibility == "private" && !user.IsAdmin {
		return "", fmt.Errorf("access denied: repository is private")
	}

	// Get path parameter
	path := ""
	if p, exists := params["path"]; exists {
		if pathStr, ok := p.(string); ok {
			path = pathStr
		}
	}
	if path == "" || path == "/" {
		path = "."
	}

	// Get branch parameter
	branch := ""
	if b, exists := params["branch"]; exists {
		if branchStr, ok := b.(string); ok {
			branch = branchStr
		}
	}
	if branch == "" {
		branch = repo.GetDefaultBranch()
	}

	// Get file tree
	files, err := repo.GetFileTree(branch, path)
	if err != nil {
		return "", fmt.Errorf("failed to list files: %w", err)
	}

	// Filter out hidden files
	var visibleFiles []*models.FileNode
	for _, file := range files {
		if !strings.HasPrefix(file.Name, ".") {
			visibleFiles = append(visibleFiles, file)
		}
	}

	if len(visibleFiles) == 0 {
		return fmt.Sprintf("No files found in %s (branch: %s, path: %s)", repo.Name, branch, path), nil
	}

	// Format output
	var result strings.Builder
	result.WriteString(fmt.Sprintf("## Files in %s\n", repo.Name))
	result.WriteString(fmt.Sprintf("**Branch:** %s\n", branch))
	if path != "." {
		result.WriteString(fmt.Sprintf("**Path:** %s\n", path))
	}
	result.WriteString(fmt.Sprintf("\nFound %d items:\n\n", len(visibleFiles)))

	// Separate directories and files
	var dirs, regularFiles []*models.FileNode
	for _, file := range visibleFiles {
		if file.IsDir() {
			dirs = append(dirs, file)
		} else {
			regularFiles = append(regularFiles, file)
		}
	}

	// List directories first
	if len(dirs) > 0 {
		result.WriteString("**Directories:**\n")
		for _, dir := range dirs {
			result.WriteString(fmt.Sprintf("📁 %s/\n", dir.Name))
		}
		result.WriteString("\n")
	}

	// List files
	if len(regularFiles) > 0 {
		result.WriteString("**Files:**\n")
		for _, file := range regularFiles {
			sizeStr := formatFileSize(file.Size)
			result.WriteString(fmt.Sprintf("📄 %s (%s)\n", file.Name, sizeStr))
		}
	}

	// Add exploration hint
	result.WriteString("\n💡 *Use read_file to examine specific files for more details.*")

	return result.String(), nil
}

// ReadFileTool reads the content of a file in a repository
type ReadFileTool struct{}

func (t *ReadFileTool) Name() string {
	return "read_file"
}

func (t *ReadFileTool) Description() string {
	return "Read the content of a file in a repository. Required params: repo_id, path. Optional params: branch"
}

func (t *ReadFileTool) ValidateParams(params map[string]any) error {
	repoID, exists := params["repo_id"]
	if !exists || repoID == nil || repoID == "" {
		return fmt.Errorf("repo_id is required")
	}

	if _, ok := repoID.(string); !ok {
		return fmt.Errorf("repo_id must be a string")
	}

	path, exists := params["path"]
	if !exists || path == nil || path == "" {
		return fmt.Errorf("path is required")
	}

	pathStr, ok := path.(string)
	if !ok {
		return fmt.Errorf("path must be a string")
	}

	if strings.Contains(pathStr, "..") {
		return fmt.Errorf("invalid path: directory traversal not allowed")
	}

	return nil
}

func (t *ReadFileTool) Schema() map[string]any {
	return SimpleSchema(map[string]any{
		"repo_id": map[string]any{
			"type":        "string",
			"description": "The repository ID",
			"required":    true,
		},
		"path": map[string]any{
			"type":        "string",
			"description": "Path to the file to read",
			"required":    true,
		},
		"branch": map[string]any{
			"type":        "string",
			"description": "Branch name to read file from",
		},
	})
}

func (t *ReadFileTool) Execute(params map[string]any, userID string) (string, error) {
	repoIDVal, exists := params["repo_id"]
	if !exists || repoIDVal == nil || repoIDVal == "" {
		return "", fmt.Errorf("repo_id is required")
	}
	repoID, ok := repoIDVal.(string)
	if !ok {
		return "", fmt.Errorf("repo_id must be a string")
	}

	pathVal, exists := params["path"]
	if !exists || pathVal == nil || pathVal == "" {
		return "", fmt.Errorf("path is required")
	}
	path, ok := pathVal.(string)
	if !ok {
		return "", fmt.Errorf("path must be a string")
	}

	// Get user to check permissions
	user, err := models.Auth.GetUser(userID)
	if err != nil {
		return "", fmt.Errorf("failed to get user: %w", err)
	}

	// Get repository
	repo, err := models.Repositories.Get(repoID)
	if err != nil {
		return "", fmt.Errorf("repository not found: %s", repoID)
	}

	// Check permissions
	if repo.Visibility == "private" && !user.IsAdmin {
		return "", fmt.Errorf("access denied: repository is private")
	}

	// Get branch parameter
	branch := ""
	if b, exists := params["branch"]; exists {
		if branchStr, ok := b.(string); ok {
			branch = branchStr
		}
	}
	if branch == "" {
		branch = repo.GetDefaultBranch()
	}

	// Get file content
	file, err := repo.GetFile(branch, path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// Format output
	var result strings.Builder
	result.WriteString(fmt.Sprintf("## File: %s\n", file.Name))
	result.WriteString(fmt.Sprintf("**Repository:** %s\n", repo.Name))
	result.WriteString(fmt.Sprintf("**Path:** %s\n", file.Path))
	result.WriteString(fmt.Sprintf("**Branch:** %s\n", branch))
	result.WriteString(fmt.Sprintf("**Size:** %s\n", formatFileSize(file.Size)))

	if file.Language != "" && file.Language != "text" {
		result.WriteString(fmt.Sprintf("**Language:** %s\n", file.Language))
	}

	result.WriteString("\n")

	if file.IsBinary {
		result.WriteString("*Binary file - content cannot be displayed*\n")
	} else if file.Size > 1024*1024 { // 1MB
		result.WriteString("*File is very large - showing first 50KB*\n\n")
		content := file.Content
		if len(content) > 50*1024 {
			content = content[:50*1024] + "\n\n... (truncated)"
		}
		result.WriteString("```" + file.Language + "\n")
		result.WriteString(content)
		result.WriteString("\n```\n")
	} else {
		result.WriteString("```" + file.Language + "\n")
		result.WriteString(file.Content)
		result.WriteString("\n```\n")
	}

	return result.String(), nil
}

// SearchFilesTool searches for files by name pattern in a repository
type SearchFilesTool struct{}

func (t *SearchFilesTool) Name() string {
	return "search_files"
}

func (t *SearchFilesTool) Description() string {
	return "Search for files by name pattern in a repository. Required params: repo_id, pattern. Optional params: branch"
}

func (t *SearchFilesTool) ValidateParams(params map[string]any) error {
	repoID, exists := params["repo_id"]
	if !exists {
		return fmt.Errorf("repo_id is required")
	}

	if _, ok := repoID.(string); !ok {
		return fmt.Errorf("repo_id must be a string")
	}

	pattern, exists := params["pattern"]
	if !exists {
		return fmt.Errorf("pattern is required")
	}

	if _, ok := pattern.(string); !ok {
		return fmt.Errorf("pattern must be a string")
	}

	return nil
}

func (t *SearchFilesTool) Schema() map[string]any {
	return SimpleSchema(map[string]any{
		"repo_id": map[string]any{
			"type":        "string",
			"description": "The repository ID",
			"required":    true,
		},
		"pattern": map[string]any{
			"type":        "string",
			"description": "Search pattern (supports wildcards like *.go)",
			"required":    true,
		},
		"branch": map[string]any{
			"type":        "string",
			"description": "Branch name to search in",
		},
	})
}

func (t *SearchFilesTool) Execute(params map[string]any, userID string) (string, error) {
	repoID := params["repo_id"].(string)
	pattern := params["pattern"].(string)

	// Get user to check permissions
	user, err := models.Auth.GetUser(userID)
	if err != nil {
		return "", fmt.Errorf("failed to get user: %w", err)
	}

	// Get repository
	repo, err := models.Repositories.Get(repoID)
	if err != nil {
		return "", fmt.Errorf("repository not found: %s", repoID)
	}

	// Check permissions
	if repo.Visibility == "private" && !user.IsAdmin {
		return "", fmt.Errorf("access denied: repository is private")
	}

	// Get branch parameter
	branch := ""
	if b, exists := params["branch"]; exists {
		if branchStr, ok := b.(string); ok {
			branch = branchStr
		}
	}
	if branch == "" {
		branch = repo.GetDefaultBranch()
	}

	// Search for files recursively
	var matches []string
	pattern = strings.ToLower(pattern)

	var searchDir func(path string) error
	searchDir = func(path string) error {
		files, err := repo.GetFileTree(branch, path)
		if err != nil {
			return err
		}

		for _, file := range files {
			// Skip hidden files
			if strings.HasPrefix(file.Name, ".") {
				continue
			}

			// Check if name matches pattern
			nameLower := strings.ToLower(file.Name)
			if matchesPattern(nameLower, pattern) {
				matches = append(matches, file.Path)
			}

			// Recursively search directories
			if file.IsDir() {
				if err := searchDir(file.Path); err != nil {
					// Continue searching even if one directory fails
					continue
				}
			}
		}
		return nil
	}

	// Start search from root
	if err := searchDir("."); err != nil {
		return "", fmt.Errorf("search failed: %w", err)
	}

	// Format results
	var result strings.Builder
	result.WriteString(fmt.Sprintf("## Search Results for '%s' in %s\n", pattern, repo.Name))
	result.WriteString(fmt.Sprintf("**Branch:** %s\n\n", branch))

	if len(matches) == 0 {
		result.WriteString("No files found matching the pattern.\n")
	} else {
		result.WriteString(fmt.Sprintf("Found %d matching files:\n\n", len(matches)))
		for _, match := range matches {
			result.WriteString(fmt.Sprintf("📄 %s\n", match))
		}

		// Add exploration hint
		result.WriteString("\n💡 *Use read_file to examine these files for their content.*")
	}

	return result.String(), nil
}

// Helper functions

func formatFileSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d bytes", size)
	} else if size < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	} else {
		return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
	}
}

func matchesPattern(name, pattern string) bool {
	// Simple pattern matching - supports * wildcard and partial matches
	if strings.Contains(pattern, "*") {
		// Convert pattern to simple regex-like matching
		parts := strings.Split(pattern, "*")
		if len(parts) == 2 {
			// Pattern like "*.go" or "test*"
			if parts[0] == "" {
				// Suffix match
				return strings.HasSuffix(name, parts[1])
			} else if parts[1] == "" {
				// Prefix match
				return strings.HasPrefix(name, parts[0])
			} else {
				// Contains both prefix and suffix
				return strings.HasPrefix(name, parts[0]) && strings.HasSuffix(name, parts[1])
			}
		}
	}

	// Partial match - check if pattern is contained in name
	return strings.Contains(name, pattern)
}
