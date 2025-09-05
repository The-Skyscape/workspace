package tools

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"workspace/models"
)

// CreateIssueTool creates a new issue in a repository
type CreateIssueTool struct{}

func (t *CreateIssueTool) Name() string {
	return "create_issue"
}

func (t *CreateIssueTool) Description() string {
	return "Create a new issue in a repository. Required params: repo_id, title, body. Optional params: tags (array of strings)"
}

func (t *CreateIssueTool) ValidateParams(params map[string]interface{}) error {
	repoID, exists := params["repo_id"]
	if !exists {
		return fmt.Errorf("repo_id is required")
	}
	if _, ok := repoID.(string); !ok {
		return fmt.Errorf("repo_id must be a string")
	}
	
	title, exists := params["title"]
	if !exists {
		return fmt.Errorf("title is required")
	}
	if _, ok := title.(string); !ok {
		return fmt.Errorf("title must be a string")
	}
	
	body, exists := params["body"]
	if !exists {
		return fmt.Errorf("body is required")
	}
	if _, ok := body.(string); !ok {
		return fmt.Errorf("body must be a string")
	}
	
	return nil
}

func (t *CreateIssueTool) Schema() map[string]interface{} {
	return SimpleSchema(map[string]interface{}{
		"repo_id": map[string]interface{}{
			"type":        "string",
			"description": "The repository ID",
			"required":    true,
		},
		"title": map[string]interface{}{
			"type":        "string",
			"description": "Issue title",
			"required":    true,
		},
		"body": map[string]interface{}{
			"type":        "string",
			"description": "Issue description",
			"required":    true,
		},
		"labels": map[string]interface{}{
			"type":        "array",
			"description": "Labels to apply to the issue",
			"items": map[string]interface{}{
				"type": "string",
			},
		},
	})
}

func (t *CreateIssueTool) Execute(params map[string]interface{}, userID string) (string, error) {
	repoID := params["repo_id"].(string)
	title := params["title"].(string)
	body := params["body"].(string)
	
	// Get user for permissions
	user, err := models.Auth.GetUser(userID)
	if err != nil {
		return "", fmt.Errorf("failed to get user: %w", err)
	}
	
	// Get repository
	repo, err := models.Repositories.Get(repoID)
	if err != nil {
		return "", fmt.Errorf("repository not found: %s", repoID)
	}
	
	// Check write permissions
	if !user.IsAdmin {
		return "", fmt.Errorf("access denied: only admins can create issues")
	}
	
	// Process tags if provided
	tagsJSON := "[]"
	if tags, exists := params["tags"]; exists {
		switch v := tags.(type) {
		case []interface{}:
			// Convert to string array
			strTags := make([]string, len(v))
			for i, tag := range v {
				if tagStr, ok := tag.(string); ok {
					strTags[i] = tagStr
				}
			}
			if tagsBytes, err := json.Marshal(strTags); err == nil {
				tagsJSON = string(tagsBytes)
			}
		case []string:
			if tagsBytes, err := json.Marshal(v); err == nil {
				tagsJSON = string(tagsBytes)
			}
		}
	}
	
	// Create issue
	issue := &models.Issue{
		Title:      title,
		Body:       body,
		Tags:       tagsJSON,
		Status:     "open",
		AuthorID:   user.ID,
		RepoID:     repoID,
		SyncStatus: "local_only",
	}
	
	issue, err = models.Issues.Insert(issue)
	if err != nil {
		return "", fmt.Errorf("failed to create issue: %w", err)
	}
	
	// Build response
	var result strings.Builder
	result.WriteString(fmt.Sprintf("✅ **Issue Created Successfully**\n\n"))
	result.WriteString(fmt.Sprintf("**Issue #%s:** %s\n", issue.ID, title))
	result.WriteString(fmt.Sprintf("**Repository:** %s\n", repo.Name))
	result.WriteString(fmt.Sprintf("**Status:** Open\n"))
	result.WriteString(fmt.Sprintf("**Author:** %s\n", user.Name))
	
	if tagsJSON != "[]" {
		result.WriteString(fmt.Sprintf("**Tags:** %s\n", tagsJSON))
	}
	
	result.WriteString(fmt.Sprintf("\n**Description:**\n%s\n", body))
	
	return result.String(), nil
}

// ListIssuesTool lists issues in a repository
type ListIssuesTool struct{}

func (t *ListIssuesTool) Name() string {
	return "list_issues"
}

func (t *ListIssuesTool) Description() string {
	return "List issues in a repository. Required params: repo_id. Optional params: status (open/closed/all), limit (number)"
}

func (t *ListIssuesTool) ValidateParams(params map[string]interface{}) error {
	repoID, exists := params["repo_id"]
	if !exists {
		return fmt.Errorf("repo_id is required")
	}
	if _, ok := repoID.(string); !ok {
		return fmt.Errorf("repo_id must be a string")
	}
	return nil
}

func (t *ListIssuesTool) Schema() map[string]interface{} {
	return SimpleSchema(map[string]interface{}{
		"repo_id": map[string]interface{}{
			"type":        "string",
			"description": "The repository ID",
			"required":    true,
		},
		"state": map[string]interface{}{
			"type":        "string",
			"enum":        []string{"open", "closed", "all"},
			"description": "Filter by issue state",
			"default":     "open",
		},
	})
}

func (t *ListIssuesTool) Execute(params map[string]interface{}, userID string) (string, error) {
	repoID := params["repo_id"].(string)
	
	// Get user for permissions
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
	
	// Get status filter
	status := "open"
	includeClosed := false
	if s, exists := params["status"]; exists {
		if statusStr, ok := s.(string); ok {
			status = statusStr
			if status == "all" || status == "closed" {
				includeClosed = true
			}
		}
	}
	
	// Get limit
	limit := 20
	if l, exists := params["limit"]; exists {
		switch v := l.(type) {
		case float64:
			limit = int(v)
		case int:
			limit = v
		}
	}
	
	// Get issues
	issues, _, err := models.GetRepoIssuesPaginated(repoID, includeClosed, limit, 0)
	if err != nil {
		return "", fmt.Errorf("failed to list issues: %w", err)
	}
	
	// Filter by specific status if needed
	if status == "closed" {
		filtered := make([]*models.Issue, 0)
		for _, issue := range issues {
			if issue.Status == "closed" {
				filtered = append(filtered, issue)
			}
		}
		issues = filtered
	}
	
	// Build response
	var result strings.Builder
	result.WriteString(fmt.Sprintf("## Issues in %s\n", repo.Name))
	result.WriteString(fmt.Sprintf("**Filter:** %s\n", status))
	result.WriteString(fmt.Sprintf("**Found:** %d issues\n\n", len(issues)))
	
	if len(issues) == 0 {
		result.WriteString("No issues found.\n")
	} else {
		for _, issue := range issues {
			statusIcon := "🔵"
			if issue.Status == "closed" {
				statusIcon = "✅"
			}
			
			// Get author name
			author := "Unknown"
			if authorUser, err := models.Auth.GetUser(issue.AuthorID); err == nil {
				author = authorUser.Name
			}
			
			result.WriteString(fmt.Sprintf("%s **#%s: %s**\n", statusIcon, issue.ID, issue.Title))
			result.WriteString(fmt.Sprintf("   Author: %s | Created: %s\n", 
				author, 
				issue.CreatedAt.Format("Jan 2, 2006")))
			
			// Show tags if any
			if issue.Tags != "" && issue.Tags != "[]" {
				result.WriteString(fmt.Sprintf("   Tags: %s\n", issue.Tags))
			}
			
			// Show truncated body
			bodyPreview := issue.Body
			if len(bodyPreview) > 100 {
				bodyPreview = bodyPreview[:97] + "..."
			}
			if bodyPreview != "" {
				result.WriteString(fmt.Sprintf("   %s\n", bodyPreview))
			}
			result.WriteString("\n")
		}
	}
	
	return result.String(), nil
}

// UpdateIssueTool updates an existing issue
type UpdateIssueTool struct{}

func (t *UpdateIssueTool) Name() string {
	return "update_issue"
}

func (t *UpdateIssueTool) Description() string {
	return "Update an existing issue. Required params: issue_id. Optional params: title, body, status (open/closed), tags"
}

func (t *UpdateIssueTool) ValidateParams(params map[string]interface{}) error {
	issueID, exists := params["issue_id"]
	if !exists {
		return fmt.Errorf("issue_id is required")
	}
	if _, ok := issueID.(string); !ok {
		return fmt.Errorf("issue_id must be a string")
	}
	
	// At least one field to update must be provided
	hasUpdate := false
	for _, field := range []string{"title", "body", "status", "tags"} {
		if _, exists := params[field]; exists {
			hasUpdate = true
			break
		}
	}
	if !hasUpdate {
		return fmt.Errorf("at least one field to update must be provided (title, body, status, or tags)")
	}
	
	return nil
}

func (t *UpdateIssueTool) Schema() map[string]interface{} {
	return SimpleSchema(map[string]interface{}{
		"repo_id": map[string]interface{}{
			"type":        "string",
			"description": "The repository ID",
			"required":    true,
		},
		"issue_number": map[string]interface{}{
			"type":        "integer",
			"description": "Issue number to update",
			"required":    true,
		},
		"state": map[string]interface{}{
			"type":        "string",
			"enum":        []string{"open", "closed"},
			"description": "New state for the issue",
		},
		"title": map[string]interface{}{
			"type":        "string",
			"description": "New title for the issue",
		},
		"body": map[string]interface{}{
			"type":        "string",
			"description": "New body for the issue",
		},
	})
}

func (t *UpdateIssueTool) Execute(params map[string]interface{}, userID string) (string, error) {
	issueID := params["issue_id"].(string)
	
	// Get user for permissions
	user, err := models.Auth.GetUser(userID)
	if err != nil {
		return "", fmt.Errorf("failed to get user: %w", err)
	}
	
	// Check admin permissions
	if !user.IsAdmin {
		return "", fmt.Errorf("access denied: only admins can update issues")
	}
	
	// Get issue
	issue, err := models.Issues.Get(issueID)
	if err != nil {
		return "", fmt.Errorf("issue not found: %s", issueID)
	}
	
	// Track what was updated
	var updates []string
	
	// Update fields if provided
	if title, exists := params["title"]; exists {
		if titleStr, ok := title.(string); ok && titleStr != "" {
			issue.Title = titleStr
			updates = append(updates, "title")
		}
	}
	
	if body, exists := params["body"]; exists {
		if bodyStr, ok := body.(string); ok && bodyStr != "" {
			issue.Body = bodyStr
			updates = append(updates, "body")
		}
	}
	
	if status, exists := params["status"]; exists {
		if statusStr, ok := status.(string); ok {
			if statusStr == "open" || statusStr == "closed" {
				issue.Status = statusStr
				updates = append(updates, "status")
			}
		}
	}
	
	if tags, exists := params["tags"]; exists {
		switch v := tags.(type) {
		case []interface{}:
			strTags := make([]string, len(v))
			for i, tag := range v {
				if tagStr, ok := tag.(string); ok {
					strTags[i] = tagStr
				}
			}
			if tagsBytes, err := json.Marshal(strTags); err == nil {
				issue.Tags = string(tagsBytes)
				updates = append(updates, "tags")
			}
		case []string:
			if tagsBytes, err := json.Marshal(v); err == nil {
				issue.Tags = string(tagsBytes)
				updates = append(updates, "tags")
			}
		}
	}
	
	// Save updates
	issue.UpdatedAt = time.Now()
	err = models.Issues.Update(issue)
	if err != nil {
		return "", fmt.Errorf("failed to update issue: %w", err)
	}
	
	// Get repository for response
	repo, _ := models.Repositories.Get(issue.RepoID)
	repoName := "Unknown"
	if repo != nil {
		repoName = repo.Name
	}
	
	// Build response
	var result strings.Builder
	result.WriteString(fmt.Sprintf("✅ **Issue Updated Successfully**\n\n"))
	result.WriteString(fmt.Sprintf("**Issue #%s:** %s\n", issue.ID, issue.Title))
	result.WriteString(fmt.Sprintf("**Repository:** %s\n", repoName))
	result.WriteString(fmt.Sprintf("**Status:** %s\n", issue.Status))
	result.WriteString(fmt.Sprintf("**Updated Fields:** %s\n", strings.Join(updates, ", ")))
	
	return result.String(), nil
}

// CreatePRTool creates a new pull request
type CreatePRTool struct{}

func (t *CreatePRTool) Name() string {
	return "create_pr"
}

func (t *CreatePRTool) Description() string {
	return "Create a new pull request. Required params: repo_id, title, body, base_branch, compare_branch"
}

func (t *CreatePRTool) ValidateParams(params map[string]interface{}) error {
	requiredFields := []string{"repo_id", "title", "body", "base_branch", "compare_branch"}
	for _, field := range requiredFields {
		value, exists := params[field]
		if !exists {
			return fmt.Errorf("%s is required", field)
		}
		if _, ok := value.(string); !ok {
			return fmt.Errorf("%s must be a string", field)
		}
	}
	
	// Check that base and compare branches are different
	if params["base_branch"] == params["compare_branch"] {
		return fmt.Errorf("base_branch and compare_branch must be different")
	}
	
	return nil
}

func (t *CreatePRTool) Schema() map[string]interface{} {
	return SimpleSchema(map[string]interface{}{
		"repo_id": map[string]interface{}{
			"type":        "string",
			"description": "The repository ID",
			"required":    true,
		},
		"title": map[string]interface{}{
			"type":        "string",
			"description": "Pull request title",
			"required":    true,
		},
		"body": map[string]interface{}{
			"type":        "string",
			"description": "Pull request description",
			"required":    true,
		},
		"head": map[string]interface{}{
			"type":        "string",
			"description": "Head branch (source)",
			"required":    true,
		},
		"base": map[string]interface{}{
			"type":        "string",
			"description": "Base branch (target)",
			"required":    true,
		},
	})
}

func (t *CreatePRTool) Execute(params map[string]interface{}, userID string) (string, error) {
	repoID := params["repo_id"].(string)
	title := params["title"].(string)
	body := params["body"].(string)
	baseBranch := params["base_branch"].(string)
	compareBranch := params["compare_branch"].(string)
	
	// Get user for permissions
	user, err := models.Auth.GetUser(userID)
	if err != nil {
		return "", fmt.Errorf("failed to get user: %w", err)
	}
	
	// Get repository
	repo, err := models.Repositories.Get(repoID)
	if err != nil {
		return "", fmt.Errorf("repository not found: %s", repoID)
	}
	
	// Check write permissions
	if !user.IsAdmin {
		return "", fmt.Errorf("access denied: only admins can create pull requests")
	}
	
	// Verify branches exist
	branches, err := repo.GetBranches()
	if err != nil {
		return "", fmt.Errorf("failed to get branches: %w", err)
	}
	
	baseExists := false
	compareExists := false
	for _, branch := range branches {
		if branch.Name == baseBranch {
			baseExists = true
		}
		if branch.Name == compareBranch {
			compareExists = true
		}
	}
	
	if !baseExists {
		return "", fmt.Errorf("base branch '%s' does not exist", baseBranch)
	}
	if !compareExists {
		return "", fmt.Errorf("compare branch '%s' does not exist", compareBranch)
	}
	
	// Check for differences between branches
	stdout, _, err := repo.Git("log", "--oneline", baseBranch+".."+compareBranch)
	if err != nil || stdout.String() == "" {
		return "", fmt.Errorf("no differences between %s and %s", baseBranch, compareBranch)
	}
	
	// Create pull request
	pr := &models.PullRequest{
		Title:         title,
		Body:          body,
		RepoID:        repoID,
		AuthorID:      user.ID,
		BaseBranch:    baseBranch,
		CompareBranch: compareBranch,
		Status:        "open",
		SyncStatus:    "local_only",
	}
	
	pr, err = models.PullRequests.Insert(pr)
	if err != nil {
		return "", fmt.Errorf("failed to create pull request: %w", err)
	}
	
	// Get commit count
	commitCount := len(strings.Split(strings.TrimSpace(stdout.String()), "\n"))
	
	// Build response
	var result strings.Builder
	result.WriteString(fmt.Sprintf("✅ **Pull Request Created Successfully**\n\n"))
	result.WriteString(fmt.Sprintf("**PR #%s:** %s\n", pr.ID, title))
	result.WriteString(fmt.Sprintf("**Repository:** %s\n", repo.Name))
	result.WriteString(fmt.Sprintf("**Base:** %s ← **Compare:** %s\n", baseBranch, compareBranch))
	result.WriteString(fmt.Sprintf("**Commits:** %d\n", commitCount))
	result.WriteString(fmt.Sprintf("**Author:** %s\n", user.Name))
	result.WriteString(fmt.Sprintf("**Status:** Open\n"))
	result.WriteString(fmt.Sprintf("\n**Description:**\n%s\n", body))
	
	return result.String(), nil
}

// ListPRsTool lists pull requests in a repository
type ListPRsTool struct{}

func (t *ListPRsTool) Name() string {
	return "list_prs"
}

func (t *ListPRsTool) Description() string {
	return "List pull requests in a repository. Required params: repo_id. Optional params: status (open/closed/merged/all), limit (number)"
}

func (t *ListPRsTool) ValidateParams(params map[string]interface{}) error {
	repoID, exists := params["repo_id"]
	if !exists {
		return fmt.Errorf("repo_id is required")
	}
	if _, ok := repoID.(string); !ok {
		return fmt.Errorf("repo_id must be a string")
	}
	return nil
}

func (t *ListPRsTool) Schema() map[string]interface{} {
	return SimpleSchema(map[string]interface{}{
		"repo_id": map[string]interface{}{
			"type":        "string",
			"description": "The repository ID",
			"required":    true,
		},
		"state": map[string]interface{}{
			"type":        "string",
			"enum":        []string{"open", "closed", "merged", "all"},
			"description": "Filter by PR state",
			"default":     "open",
		},
	})
}

func (t *ListPRsTool) Execute(params map[string]interface{}, userID string) (string, error) {
	repoID := params["repo_id"].(string)
	
	// Get user for permissions
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
	
	// Get status filter
	status := "open"
	includeClosed := false
	if s, exists := params["status"]; exists {
		if statusStr, ok := s.(string); ok {
			status = statusStr
			if status == "all" || status == "closed" || status == "merged" {
				includeClosed = true
			}
		}
	}
	
	// Get limit
	limit := 20
	if l, exists := params["limit"]; exists {
		switch v := l.(type) {
		case float64:
			limit = int(v)
		case int:
			limit = v
		}
	}
	
	// Get pull requests
	prs, _, err := models.GetRepoPRsPaginated(repoID, includeClosed, limit, 0)
	if err != nil {
		return "", fmt.Errorf("failed to list pull requests: %w", err)
	}
	
	// Filter by specific status if needed
	if status != "all" && status != "open" {
		filtered := make([]*models.PullRequest, 0)
		for _, pr := range prs {
			if pr.Status == status {
				filtered = append(filtered, pr)
			}
		}
		prs = filtered
	}
	
	// Build response
	var result strings.Builder
	result.WriteString(fmt.Sprintf("## Pull Requests in %s\n", repo.Name))
	result.WriteString(fmt.Sprintf("**Filter:** %s\n", status))
	result.WriteString(fmt.Sprintf("**Found:** %d pull requests\n\n", len(prs)))
	
	if len(prs) == 0 {
		result.WriteString("No pull requests found.\n")
	} else {
		for _, pr := range prs {
			statusIcon := "🔵"
			switch pr.Status {
			case "merged":
				statusIcon = "🟣"
			case "closed":
				statusIcon = "🔴"
			case "draft":
				statusIcon = "⚪"
			}
			
			// Get author name
			author := "Unknown"
			if authorUser, err := models.Auth.GetUser(pr.AuthorID); err == nil {
				author = authorUser.Name
			}
			
			result.WriteString(fmt.Sprintf("%s **#%s: %s**\n", statusIcon, pr.ID, pr.Title))
			result.WriteString(fmt.Sprintf("   %s ← %s\n", pr.BaseBranch, pr.CompareBranch))
			result.WriteString(fmt.Sprintf("   Author: %s | Created: %s\n", 
				author, 
				pr.CreatedAt.Format("Jan 2, 2006")))
			
			// Show truncated body
			bodyPreview := pr.Body
			if len(bodyPreview) > 100 {
				bodyPreview = bodyPreview[:97] + "..."
			}
			if bodyPreview != "" {
				result.WriteString(fmt.Sprintf("   %s\n", bodyPreview))
			}
			result.WriteString("\n")
		}
	}
	
	return result.String(), nil
}