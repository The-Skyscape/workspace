package tools

import (
	"fmt"
	"strings"
	"workspace/models"
)

// ListReposTool lists repositories
type ListReposTool struct{}

func (t *ListReposTool) Name() string {
	return "list_repos"
}

func (t *ListReposTool) Description() string {
	return "List all repositories. Optional params: visibility (public/private/all). Default is 'all' to see everything. When searching for a specific repo, always use 'all' visibility."
}

func (t *ListReposTool) ValidateParams(params map[string]interface{}) error {
	// Check visibility parameter if provided
	if vis, exists := params["visibility"]; exists {
		visStr, ok := vis.(string)
		if !ok {
			return fmt.Errorf("visibility must be a string")
		}
		if visStr != "public" && visStr != "private" && visStr != "all" && visStr != "" {
			return fmt.Errorf("visibility must be 'public', 'private', or 'all'")
		}
	}
	return nil
}

func (t *ListReposTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"visibility": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"public", "private", "all"},
				"description": "Filter by repository visibility",
				"default":     "all",
			},
		},
	}
}

func (t *ListReposTool) Execute(params map[string]interface{}, userID string) (string, error) {
	// Get user to check admin status
	user, err := models.Auth.GetUser(userID)
	if err != nil {
		return "", fmt.Errorf("failed to get user: %w", err)
	}
	
	visibility := "all"
	if vis, exists := params["visibility"]; exists {
		if visStr, ok := vis.(string); ok && visStr != "" {
			visibility = visStr
		}
	}
	
	var repos []*models.Repository
	
	// Query based on admin status and visibility
	if user.IsAdmin {
		// Admins can see all repos
		switch visibility {
		case "public":
			repos, err = models.Repositories.Search("WHERE Visibility = ? ORDER BY UpdatedAt DESC", "public")
		case "private":
			repos, err = models.Repositories.Search("WHERE Visibility = ? ORDER BY UpdatedAt DESC", "private")
		default:
			repos, err = models.Repositories.Search("ORDER BY UpdatedAt DESC")
		}
	} else {
		// Non-admins can only see public repos
		repos, err = models.Repositories.Search("WHERE Visibility = ? ORDER BY UpdatedAt DESC", "public")
	}
	
	if err != nil {
		return "", fmt.Errorf("failed to list repositories: %w", err)
	}
	
	if len(repos) == 0 {
		return "No repositories found.", nil
	}
	
	// Format the output - concise but complete list
	var result strings.Builder
	result.WriteString(fmt.Sprintf("Found %d repositories:\n\n", len(repos)))
	
	for i, repo := range repos {
		// Concise format: just name, ID, and visibility
		result.WriteString(fmt.Sprintf("%d. %s (%s, %s)\n", i+1, repo.Name, repo.ID, repo.Visibility))
	}
	
	return result.String(), nil
}

// GetRepoTool gets details about a specific repository
type GetRepoTool struct{}

func (t *GetRepoTool) Name() string {
	return "get_repo"
}

func (t *GetRepoTool) Description() string {
	return "Get detailed information about a specific repository. Required params: repo_id"
}

func (t *GetRepoTool) ValidateParams(params map[string]interface{}) error {
	repoID, exists := params["repo_id"]
	if !exists {
		return fmt.Errorf("repo_id is required")
	}
	
	if _, ok := repoID.(string); !ok {
		return fmt.Errorf("repo_id must be a string")
	}
	
	return nil
}

func (t *GetRepoTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"repo_id": map[string]interface{}{
				"type":        "string",
				"description": "The repository ID to get details for",
				"required":    true,
			},
		},
		"required": []string{"repo_id"},
	}
}

func (t *GetRepoTool) Execute(params map[string]interface{}, userID string) (string, error) {
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
	
	// Format detailed information - keep concise
	var result strings.Builder
	result.WriteString(fmt.Sprintf("Repository: %s (ID: %s)\n", repo.Name, repo.ID))
	if repo.Description != "" {
		result.WriteString(fmt.Sprintf("Description: %s\n", repo.Description))
	}
	result.WriteString(fmt.Sprintf("Visibility: %s\n", repo.Visibility))
	if repo.PrimaryLanguage != "" {
		result.WriteString(fmt.Sprintf("Language: %s\n", repo.PrimaryLanguage))
	}
	result.WriteString(fmt.Sprintf("Updated: %s\n", repo.UpdatedAt.Format("Jan 2, 2006")))
	
	return result.String(), nil
}

// CreateRepoTool creates a new repository
type CreateRepoTool struct{}

func (t *CreateRepoTool) Name() string {
	return "create_repo"
}

func (t *CreateRepoTool) Description() string {
	return "Create a new repository. Required params: name. Optional params: description, visibility (public/private)"
}

func (t *CreateRepoTool) ValidateParams(params map[string]interface{}) error {
	name, exists := params["name"]
	if !exists {
		return fmt.Errorf("name is required")
	}
	
	if _, ok := name.(string); !ok {
		return fmt.Errorf("name must be a string")
	}
	
	// Validate visibility if provided
	if vis, exists := params["visibility"]; exists {
		visStr, ok := vis.(string)
		if !ok {
			return fmt.Errorf("visibility must be a string")
		}
		if visStr != "public" && visStr != "private" && visStr != "" {
			return fmt.Errorf("visibility must be 'public' or 'private'")
		}
	}
	
	return nil
}

func (t *CreateRepoTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "The name of the repository to create",
				"required":    true,
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "A description of the repository",
			},
			"visibility": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"public", "private"},
				"description": "Repository visibility",
				"default":     "private",
			},
		},
		"required": []string{"name"},
	}
}

func (t *CreateRepoTool) Execute(params map[string]interface{}, userID string) (string, error) {
	// Get user to check admin status
	user, err := models.Auth.GetUser(userID)
	if err != nil {
		return "", fmt.Errorf("failed to get user: %w", err)
	}
	
	// Only admins can create repositories
	if !user.IsAdmin {
		return "", fmt.Errorf("only administrators can create repositories")
	}
	
	name := params["name"].(string)
	description := ""
	if desc, exists := params["description"]; exists {
		if descStr, ok := desc.(string); ok {
			description = descStr
		}
	}
	
	visibility := "private" // Default to private
	if vis, exists := params["visibility"]; exists {
		if visStr, ok := vis.(string); ok && visStr != "" {
			visibility = visStr
		}
	}
	
	// Create the repository
	repo, err := models.CreateRepository(name, description, visibility, userID)
	if err != nil {
		return "", fmt.Errorf("failed to create repository: %w", err)
	}
	
	// Format success message
	var result strings.Builder
	result.WriteString(fmt.Sprintf("✅ Successfully created repository: **%s**\n\n", repo.Name))
	result.WriteString(fmt.Sprintf("**ID:** %s\n", repo.ID))
	result.WriteString(fmt.Sprintf("**Description:** %s\n", description))
	result.WriteString(fmt.Sprintf("**Visibility:** %s\n", visibility))
	result.WriteString(fmt.Sprintf("\nYou can access it at: `/repos/%s`\n", repo.ID))
	
	return result.String(), nil
}

// GetRepoLinkTool generates links to repository pages
type GetRepoLinkTool struct{}

func (t *GetRepoLinkTool) Name() string {
	return "get_repo_link"
}

func (t *GetRepoLinkTool) Description() string {
	return "Generate a link to a repository page. Required params: repo_id. Optional params: view (files/issues/commits/settings)"
}

func (t *GetRepoLinkTool) ValidateParams(params map[string]interface{}) error {
	repoID, exists := params["repo_id"]
	if !exists {
		return fmt.Errorf("repo_id is required")
	}
	
	if _, ok := repoID.(string); !ok {
		return fmt.Errorf("repo_id must be a string")
	}
	
	// Validate view if provided
	if view, exists := params["view"]; exists {
		viewStr, ok := view.(string)
		if !ok {
			return fmt.Errorf("view must be a string")
		}
		validViews := []string{"", "files", "issues", "commits", "settings", "activity"}
		valid := false
		for _, v := range validViews {
			if viewStr == v {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("view must be one of: files, issues, commits, settings, activity")
		}
	}
	
	return nil
}

func (t *GetRepoLinkTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"repo_id": map[string]interface{}{
				"type":        "string",
				"description": "The repository ID",
				"required":    true,
			},
			"view": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"files", "issues", "commits", "settings"},
				"description": "The specific view to link to",
				"default":     "files",
			},
		},
		"required": []string{"repo_id"},
	}
}

func (t *GetRepoLinkTool) Execute(params map[string]interface{}, userID string) (string, error) {
	repoID := params["repo_id"].(string)
	
	// Get user to check permissions
	user, err := models.Auth.GetUser(userID)
	if err != nil {
		return "", fmt.Errorf("failed to get user: %w", err)
	}
	
	// Get repository to check it exists and permissions
	repo, err := models.Repositories.Get(repoID)
	if err != nil {
		return "", fmt.Errorf("repository not found: %s", repoID)
	}
	
	// Check permissions
	if repo.Visibility == "private" && !user.IsAdmin {
		return "", fmt.Errorf("access denied: repository is private")
	}
	
	// Build the link
	basePath := fmt.Sprintf("/repos/%s", repo.ID)
	view := ""
	if v, exists := params["view"]; exists {
		if viewStr, ok := v.(string); ok && viewStr != "" {
			view = viewStr
		}
	}
	
	link := basePath
	linkDescription := "main page"
	
	switch view {
	case "files":
		link = fmt.Sprintf("%s/files", basePath)
		linkDescription = "files browser"
	case "issues":
		link = fmt.Sprintf("%s/issues", basePath)
		linkDescription = "issues list"
	case "commits":
		link = fmt.Sprintf("%s/commits", basePath)
		linkDescription = "commit history"
	case "settings":
		if !user.IsAdmin {
			return "", fmt.Errorf("only administrators can access repository settings")
		}
		link = fmt.Sprintf("%s/settings", basePath)
		linkDescription = "settings page"
	case "activity":
		link = fmt.Sprintf("%s/activity", basePath)
		linkDescription = "activity log"
	}
	
	// Format the response
	result := fmt.Sprintf("Here's the link to the %s for repository **%s**:\n\n[%s](%s)\n", 
		linkDescription, repo.Name, link, link)
	
	return result, nil
}