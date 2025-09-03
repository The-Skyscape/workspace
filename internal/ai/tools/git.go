package tools

import (
	"fmt"
	"strings"
	"workspace/models"
)

// GitStatusTool shows the status of a repository
type GitStatusTool struct{}

func (t *GitStatusTool) Name() string {
	return "git_status"
}

func (t *GitStatusTool) Description() string {
	return "Show the status of a repository including modified, staged, and untracked files. Required params: repo_id. Optional params: branch"
}

func (t *GitStatusTool) ValidateParams(params map[string]interface{}) error {
	repoID, exists := params["repo_id"]
	if !exists {
		return fmt.Errorf("repo_id is required")
	}
	if _, ok := repoID.(string); !ok {
		return fmt.Errorf("repo_id must be a string")
	}
	return nil
}

func (t *GitStatusTool) Schema() map[string]interface{} {
	return SimpleSchema(map[string]interface{}{
		"repo_id": map[string]interface{}{
			"type":        "string",
			"description": "The repository ID",
			"required":    true,
		},
		"branch": map[string]interface{}{
			"type":        "string",
			"description": "Branch to check status for",
		},
	})
}

func (t *GitStatusTool) Execute(params map[string]interface{}, userID string) (string, error) {
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
	
	// Get current branch
	currentBranch := repo.GetDefaultBranch()
	if branch, exists := params["branch"]; exists {
		if branchStr, ok := branch.(string); ok && branchStr != "" {
			currentBranch = branchStr
		}
	}
	
	// Get status
	stdout, stderr, err := repo.Git("status", "--porcelain", "-b")
	if err != nil {
		return "", fmt.Errorf("failed to get status: %s", stderr.String())
	}
	
	lines := strings.Split(stdout.String(), "\n")
	
	// Parse branch info from first line
	var branchInfo string
	if len(lines) > 0 && strings.HasPrefix(lines[0], "##") {
		branchInfo = strings.TrimPrefix(lines[0], "## ")
		lines = lines[1:] // Remove branch line from status
	}
	
	// Categorize files
	var staged, modified, untracked []string
	for _, line := range lines {
		if line == "" {
			continue
		}
		
		if len(line) < 3 {
			continue
		}
		
		status := line[:2]
		file := strings.TrimSpace(line[3:])
		
		switch {
		case status[0] != ' ' && status[0] != '?':
			staged = append(staged, file)
		case status[1] != ' ' && status[1] != '?':
			modified = append(modified, file)
		case status == "??":
			untracked = append(untracked, file)
		}
	}
	
	// Get latest commit info
	commitOut, _, _ := repo.Git("log", "-1", "--oneline")
	lastCommit := strings.TrimSpace(commitOut.String())
	
	// Build response
	var result strings.Builder
	result.WriteString(fmt.Sprintf("## Git Status for %s\n\n", repo.Name))
	
	if branchInfo != "" {
		result.WriteString(fmt.Sprintf("**Branch:** %s\n", branchInfo))
	} else {
		result.WriteString(fmt.Sprintf("**Branch:** %s\n", currentBranch))
	}
	
	if lastCommit != "" {
		result.WriteString(fmt.Sprintf("**Latest commit:** %s\n\n", lastCommit))
	}
	
	if len(staged) == 0 && len(modified) == 0 && len(untracked) == 0 {
		result.WriteString("✅ Working tree is clean - no changes to commit\n")
	} else {
		if len(staged) > 0 {
			result.WriteString(fmt.Sprintf("**Staged changes (%d):**\n", len(staged)))
			for _, file := range staged {
				result.WriteString(fmt.Sprintf("  ✅ %s\n", file))
			}
			result.WriteString("\n")
		}
		
		if len(modified) > 0 {
			result.WriteString(fmt.Sprintf("**Modified files (%d):**\n", len(modified)))
			for _, file := range modified {
				result.WriteString(fmt.Sprintf("  📝 %s\n", file))
			}
			result.WriteString("\n")
		}
		
		if len(untracked) > 0 {
			result.WriteString(fmt.Sprintf("**Untracked files (%d):**\n", len(untracked)))
			for _, file := range untracked {
				result.WriteString(fmt.Sprintf("  ❓ %s\n", file))
			}
			result.WriteString("\n")
		}
	}
	
	return result.String(), nil
}

// GitDiffTool shows differences in a repository
type GitDiffTool struct{}

func (t *GitDiffTool) Name() string {
	return "git_diff"
}

func (t *GitDiffTool) Description() string {
	return "Show differences in a repository. Required params: repo_id. Optional params: path (specific file), from (commit/branch), to (commit/branch), staged (bool)"
}

func (t *GitDiffTool) ValidateParams(params map[string]interface{}) error {
	repoID, exists := params["repo_id"]
	if !exists {
		return fmt.Errorf("repo_id is required")
	}
	if _, ok := repoID.(string); !ok {
		return fmt.Errorf("repo_id must be a string")
	}
	return nil
}

func (t *GitDiffTool) Schema() map[string]interface{} {
	return SimpleSchema(map[string]interface{}{
		"repo_id": map[string]interface{}{
			"type":        "string",
			"description": "The repository ID",
			"required":    true,
		},
		"staged": map[string]interface{}{
			"type":        "boolean",
			"description": "Show staged changes instead of unstaged",
			"default":     false,
		},
		"file": map[string]interface{}{
			"type":        "string",
			"description": "Specific file to show diff for",
		},
	})
}

func (t *GitDiffTool) Execute(params map[string]interface{}, userID string) (string, error) {
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
	
	// Build diff command
	args := []string{"diff"}
	
	// Check if we want staged changes
	if staged, exists := params["staged"]; exists {
		if stagedBool, ok := staged.(bool); ok && stagedBool {
			args = append(args, "--cached")
		}
	}
	
	// Add from/to refs if specified
	if from, exists := params["from"]; exists {
		if fromStr, ok := from.(string); ok && fromStr != "" {
			if to, exists := params["to"]; exists {
				if toStr, ok := to.(string); ok && toStr != "" {
					args = append(args, fromStr, toStr)
				} else {
					args = append(args, fromStr)
				}
			} else {
				args = append(args, fromStr)
			}
		}
	}
	
	// Add specific path if provided
	if path, exists := params["path"]; exists {
		if pathStr, ok := path.(string); ok && pathStr != "" {
			args = append(args, "--", pathStr)
		}
	}
	
	// Get diff
	stdout, stderr, err := repo.Git(args...)
	if err != nil && stderr.String() != "" {
		return "", fmt.Errorf("failed to get diff: %s", stderr.String())
	}
	
	diff := stdout.String()
	if diff == "" {
		return "No differences found", nil
	}
	
	// Get summary stats
	statsArgs := append([]string{"diff", "--stat"}, args[1:]...)
	statsOut, _, _ := repo.Git(statsArgs...)
	
	// Build response
	var result strings.Builder
	result.WriteString(fmt.Sprintf("## Git Diff for %s\n\n", repo.Name))
	
	if stats := statsOut.String(); stats != "" && stats != diff {
		result.WriteString("**Summary:**\n```\n")
		result.WriteString(stats)
		result.WriteString("```\n\n")
	}
	
	result.WriteString("**Changes:**\n```diff\n")
	
	// Limit diff output for very large diffs
	if len(diff) > 50000 {
		result.WriteString(diff[:50000])
		result.WriteString("\n\n... (truncated, diff too large)")
	} else {
		result.WriteString(diff)
	}
	result.WriteString("\n```\n")
	
	return result.String(), nil
}

// GitCommitTool creates a commit in a repository
type GitCommitTool struct{}

func (t *GitCommitTool) Name() string {
	return "git_commit"
}

func (t *GitCommitTool) Description() string {
	return "Create a commit with staged changes. Required params: repo_id, message. Optional params: add_all (bool to stage all changes first)"
}

func (t *GitCommitTool) ValidateParams(params map[string]interface{}) error {
	repoID, exists := params["repo_id"]
	if !exists {
		return fmt.Errorf("repo_id is required")
	}
	if _, ok := repoID.(string); !ok {
		return fmt.Errorf("repo_id must be a string")
	}
	
	message, exists := params["message"]
	if !exists {
		return fmt.Errorf("message is required")
	}
	if _, ok := message.(string); !ok {
		return fmt.Errorf("message must be a string")
	}
	
	return nil
}

func (t *GitCommitTool) Schema() map[string]interface{} {
	return SimpleSchema(map[string]interface{}{
		"repo_id": map[string]interface{}{
			"type":        "string",
			"description": "The repository ID",
			"required":    true,
		},
		"message": map[string]interface{}{
			"type":        "string",
			"description": "Commit message",
			"required":    true,
		},
		"files": map[string]interface{}{
			"type":        "array",
			"description": "Files to stage and commit (empty = all)",
			"items": map[string]interface{}{
				"type": "string",
			},
		},
	})
}

func (t *GitCommitTool) Execute(params map[string]interface{}, userID string) (string, error) {
	repoID := params["repo_id"].(string)
	message := params["message"].(string)
	
	// Get user for permissions and commit author
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
		return "", fmt.Errorf("access denied: only admins can commit")
	}
	
	// Check if we should stage all changes first
	if addAll, exists := params["add_all"]; exists {
		if addAllBool, ok := addAll.(bool); ok && addAllBool {
			_, stderr, err := repo.Git("add", "-A")
			if err != nil {
				return "", fmt.Errorf("failed to stage changes: %s", stderr.String())
			}
		}
	}
	
	// Check if there are changes to commit
	statusOut, _, _ := repo.Git("status", "--porcelain")
	if statusOut.String() == "" {
		return "No changes to commit", nil
	}
	
	// Configure user for this commit
	repo.Git("config", "user.name", user.Name)
	repo.Git("config", "user.email", user.Email)
	
	// Create commit
	stdout, stderr, err := repo.Git("commit", "-m", message)
	if err != nil {
		return "", fmt.Errorf("failed to commit: %s", stderr.String())
	}
	
	// Get the new commit info
	commitOut, _, _ := repo.Git("log", "-1", "--oneline")
	newCommit := strings.TrimSpace(commitOut.String())
	
	// Build response
	var result strings.Builder
	result.WriteString(fmt.Sprintf("✅ **Commit Created Successfully**\n\n"))
	result.WriteString(fmt.Sprintf("**Repository:** %s\n", repo.Name))
	result.WriteString(fmt.Sprintf("**Commit:** %s\n", newCommit))
	result.WriteString(fmt.Sprintf("**Author:** %s <%s>\n", user.Name, user.Email))
	result.WriteString(fmt.Sprintf("\n**Output:**\n```\n%s\n```", stdout.String()))
	
	return result.String(), nil
}

// GitBranchTool manages branches in a repository
type GitBranchTool struct{}

func (t *GitBranchTool) Name() string {
	return "git_branch"
}

func (t *GitBranchTool) Description() string {
	return "Manage branches in a repository. Required params: repo_id. Optional params: action (list/create/switch/delete), name (for create/switch/delete), from (source branch for create)"
}

func (t *GitBranchTool) ValidateParams(params map[string]interface{}) error {
	repoID, exists := params["repo_id"]
	if !exists {
		return fmt.Errorf("repo_id is required")
	}
	if _, ok := repoID.(string); !ok {
		return fmt.Errorf("repo_id must be a string")
	}
	
	// Validate action-specific params
	if action, exists := params["action"]; exists {
		actionStr, ok := action.(string)
		if !ok {
			return fmt.Errorf("action must be a string")
		}
		
		switch actionStr {
		case "create", "switch", "delete":
			if _, exists := params["name"]; !exists {
				return fmt.Errorf("name is required for %s action", actionStr)
			}
		case "list", "":
			// No additional params needed
		default:
			return fmt.Errorf("invalid action: %s (use list/create/switch/delete)", actionStr)
		}
	}
	
	return nil
}

func (t *GitBranchTool) Schema() map[string]interface{} {
	return SimpleSchema(map[string]interface{}{
		"repo_id": map[string]interface{}{
			"type":        "string",
			"description": "The repository ID",
			"required":    true,
		},
		"action": map[string]interface{}{
			"type":        "string",
			"enum":        []string{"list", "create", "delete", "switch"},
			"description": "Branch operation to perform",
			"default":     "list",
		},
		"name": map[string]interface{}{
			"type":        "string",
			"description": "Branch name (for create/delete/switch)",
		},
	})
}

func (t *GitBranchTool) Execute(params map[string]interface{}, userID string) (string, error) {
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
	
	// Get action (default to list)
	action := "list"
	if a, exists := params["action"]; exists {
		if actionStr, ok := a.(string); ok && actionStr != "" {
			action = actionStr
		}
	}
	
	switch action {
	case "list":
		branches, err := repo.GetBranches()
		if err != nil {
			return "", fmt.Errorf("failed to list branches: %w", err)
		}
		
		var result strings.Builder
		result.WriteString(fmt.Sprintf("## Branches in %s\n\n", repo.Name))
		
		if len(branches) == 0 {
			result.WriteString("No branches found\n")
		} else {
			for _, branch := range branches {
				icon := "  "
				if branch.IsCurrent {
					icon = "→ "
				}
				if branch.IsDefault {
					result.WriteString(fmt.Sprintf("%s**%s** (default) - %s\n", icon, branch.Name, branch.LastCommit))
				} else {
					result.WriteString(fmt.Sprintf("%s%s - %s\n", icon, branch.Name, branch.LastCommit))
				}
			}
		}
		
		return result.String(), nil
		
	case "create":
		if !user.IsAdmin {
			return "", fmt.Errorf("access denied: only admins can create branches")
		}
		
		name := params["name"].(string)
		from := ""
		if f, exists := params["from"]; exists {
			if fromStr, ok := f.(string); ok {
				from = fromStr
			}
		}
		
		err := repo.CreateBranch(name, from)
		if err != nil {
			return "", fmt.Errorf("failed to create branch: %w", err)
		}
		
		return fmt.Sprintf("✅ Branch '%s' created successfully in %s", name, repo.Name), nil
		
	case "switch":
		if !user.IsAdmin {
			return "", fmt.Errorf("access denied: only admins can switch branches")
		}
		
		name := params["name"].(string)
		
		_, stderr, err := repo.Git("checkout", name)
		if err != nil {
			return "", fmt.Errorf("failed to switch branch: %s", stderr.String())
		}
		
		return fmt.Sprintf("✅ Switched to branch '%s' in %s", name, repo.Name), nil
		
	case "delete":
		if !user.IsAdmin {
			return "", fmt.Errorf("access denied: only admins can delete branches")
		}
		
		name := params["name"].(string)
		
		_, stderr, err := repo.Git("branch", "-d", name)
		if err != nil {
			// Try force delete if normal delete fails
			_, stderr, err = repo.Git("branch", "-D", name)
			if err != nil {
				return "", fmt.Errorf("failed to delete branch: %s", stderr.String())
			}
		}
		
		return fmt.Sprintf("✅ Branch '%s' deleted from %s", name, repo.Name), nil
		
	default:
		return "", fmt.Errorf("invalid action: %s", action)
	}
}

// GitLogTool shows commit history
type GitLogTool struct{}

func (t *GitLogTool) Name() string {
	return "git_log"
}

func (t *GitLogTool) Description() string {
	return "Show commit history of a repository. Required params: repo_id. Optional params: branch, limit (number of commits), oneline (bool)"
}

func (t *GitLogTool) ValidateParams(params map[string]interface{}) error {
	repoID, exists := params["repo_id"]
	if !exists {
		return fmt.Errorf("repo_id is required")
	}
	if _, ok := repoID.(string); !ok {
		return fmt.Errorf("repo_id must be a string")
	}
	return nil
}

func (t *GitLogTool) Schema() map[string]interface{} {
	return SimpleSchema(map[string]interface{}{
		"repo_id": map[string]interface{}{
			"type":        "string",
			"description": "The repository ID",
			"required":    true,
		},
		"limit": map[string]interface{}{
			"type":        "integer",
			"description": "Number of commits to show",
			"default":     10,
		},
		"branch": map[string]interface{}{
			"type":        "string",
			"description": "Branch to show log for",
		},
	})
}

func (t *GitLogTool) Execute(params map[string]interface{}, userID string) (string, error) {
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
	
	// Get parameters
	branch := ""
	if b, exists := params["branch"]; exists {
		if branchStr, ok := b.(string); ok && branchStr != "" {
			branch = branchStr
		}
	}
	if branch == "" {
		branch = repo.GetDefaultBranch()
	}
	
	limit := 10
	if l, exists := params["limit"]; exists {
		switch v := l.(type) {
		case float64:
			limit = int(v)
		case int:
			limit = v
		}
	}
	
	// Use repository's GetCommits method for detailed info
	commits, err := repo.GetCommits(branch, limit)
	if err != nil {
		return "", fmt.Errorf("failed to get commits: %w", err)
	}
	
	// Build response
	var result strings.Builder
	result.WriteString(fmt.Sprintf("## Commit History for %s\n", repo.Name))
	result.WriteString(fmt.Sprintf("**Branch:** %s\n", branch))
	result.WriteString(fmt.Sprintf("**Showing:** %d commits\n\n", len(commits)))
	
	if len(commits) == 0 {
		result.WriteString("No commits found\n")
	} else {
		// Check if oneline format is requested
		oneline := false
		if o, exists := params["oneline"]; exists {
			if onelineBool, ok := o.(bool); ok {
				oneline = onelineBool
			}
		}
		
		if oneline {
			for _, commit := range commits {
				result.WriteString(fmt.Sprintf("%s %s\n", commit.ShortHash, commit.Message))
			}
		} else {
			for _, commit := range commits {
				result.WriteString(fmt.Sprintf("**Commit:** %s\n", commit.ShortHash))
				result.WriteString(fmt.Sprintf("**Author:** %s <%s>\n", commit.Author, commit.Email))
				result.WriteString(fmt.Sprintf("**Date:** %s (%s)\n", 
					commit.Date.Format("2006-01-02 15:04:05"),
					commit.RelativeTime()))
				result.WriteString(fmt.Sprintf("**Message:** %s\n\n", commit.Message))
			}
		}
	}
	
	return result.String(), nil
}