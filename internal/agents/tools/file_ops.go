package tools

import (
	"fmt"
	"strings"
	"workspace/models"
)

// EditFileTool edits an existing file in a repository
type EditFileTool struct{}

func (t *EditFileTool) Name() string {
	return "edit_file"
}

func (t *EditFileTool) Description() string {
	return "Edit an existing file in a repository. Required params: repo_id, path, content, message. Optional params: branch"
}

func (t *EditFileTool) ValidateParams(params map[string]interface{}) error {
	repoID, exists := params["repo_id"]
	if !exists {
		return fmt.Errorf("repo_id is required")
	}
	if _, ok := repoID.(string); !ok {
		return fmt.Errorf("repo_id must be a string")
	}
	
	path, exists := params["path"]
	if !exists {
		return fmt.Errorf("path is required")
	}
	pathStr, ok := path.(string)
	if !ok {
		return fmt.Errorf("path must be a string")
	}
	if strings.Contains(pathStr, "..") {
		return fmt.Errorf("invalid path: directory traversal not allowed")
	}
	
	content, exists := params["content"]
	if !exists {
		return fmt.Errorf("content is required")
	}
	if _, ok := content.(string); !ok {
		return fmt.Errorf("content must be a string")
	}
	
	message, exists := params["message"]
	if !exists {
		return fmt.Errorf("message is required (commit message)")
	}
	if _, ok := message.(string); !ok {
		return fmt.Errorf("message must be a string")
	}
	
	return nil
}

func (t *EditFileTool) Schema() map[string]interface{} {
	return SimpleSchema(map[string]interface{}{
		"repo_id": map[string]interface{}{
			"type":        "string",
			"description": "The repository ID",
			"required":    true,
		},
		"path": map[string]interface{}{
			"type":        "string",
			"description": "Path to the file to edit",
			"required":    true,
		},
		"content": map[string]interface{}{
			"type":        "string",
			"description": "New content for the file",
			"required":    true,
		},
		"message": map[string]interface{}{
			"type":        "string",
			"description": "Commit message describing the change",
			"required":    true,
		},
		"branch": map[string]interface{}{
			"type":        "string",
			"description": "Branch to edit file on",
		},
	})
}

func (t *EditFileTool) Execute(params map[string]interface{}, userID string) (string, error) {
	repoID := params["repo_id"].(string)
	path := params["path"].(string)
	content := params["content"].(string)
	message := params["message"].(string)
	
	// Get user for permissions and author info
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
		return "", fmt.Errorf("access denied: only admins can edit files")
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
	
	// Update the file
	err = repo.UpdateFile(branch, path, content, message, user.Name, user.Email)
	if err != nil {
		return "", fmt.Errorf("failed to update file: %w", err)
	}
	
	// Format success response
	var result strings.Builder
	result.WriteString(fmt.Sprintf("✅ **File Updated Successfully**\n\n"))
	result.WriteString(fmt.Sprintf("**Repository:** %s\n", repo.Name))
	result.WriteString(fmt.Sprintf("**File:** %s\n", path))
	result.WriteString(fmt.Sprintf("**Branch:** %s\n", branch))
	result.WriteString(fmt.Sprintf("**Commit Message:** %s\n", message))
	result.WriteString(fmt.Sprintf("**Author:** %s <%s>\n", user.Name, user.Email))
	
	return result.String(), nil
}

// WriteFileTool creates a new file or overwrites an existing file
type WriteFileTool struct{}

func (t *WriteFileTool) Name() string {
	return "write_file"
}

func (t *WriteFileTool) Description() string {
	return "Create a new file or overwrite an existing file in a repository. Required params: repo_id, path, content, message. Optional params: branch"
}

func (t *WriteFileTool) ValidateParams(params map[string]interface{}) error {
	repoID, exists := params["repo_id"]
	if !exists {
		return fmt.Errorf("repo_id is required")
	}
	if _, ok := repoID.(string); !ok {
		return fmt.Errorf("repo_id must be a string")
	}
	
	path, exists := params["path"]
	if !exists {
		return fmt.Errorf("path is required")
	}
	pathStr, ok := path.(string)
	if !ok {
		return fmt.Errorf("path must be a string")
	}
	if strings.Contains(pathStr, "..") {
		return fmt.Errorf("invalid path: directory traversal not allowed")
	}
	
	content, exists := params["content"]
	if !exists {
		return fmt.Errorf("content is required")
	}
	if _, ok := content.(string); !ok {
		return fmt.Errorf("content must be a string")
	}
	
	message, exists := params["message"]
	if !exists {
		return fmt.Errorf("message is required (commit message)")
	}
	if _, ok := message.(string); !ok {
		return fmt.Errorf("message must be a string")
	}
	
	return nil
}

func (t *WriteFileTool) Schema() map[string]interface{} {
	return SimpleSchema(map[string]interface{}{
		"repo_id": map[string]interface{}{
			"type":        "string",
			"description": "The repository ID",
			"required":    true,
		},
		"path": map[string]interface{}{
			"type":        "string",
			"description": "Path for the new file",
			"required":    true,
		},
		"content": map[string]interface{}{
			"type":        "string",
			"description": "Content for the new file",
			"required":    true,
		},
		"message": map[string]interface{}{
			"type":        "string",
			"description": "Commit message describing the file creation",
			"required":    true,
		},
		"branch": map[string]interface{}{
			"type":        "string",
			"description": "Branch to create file on",
		},
	})
}

func (t *WriteFileTool) Execute(params map[string]interface{}, userID string) (string, error) {
	repoID := params["repo_id"].(string)
	path := params["path"].(string)
	content := params["content"].(string)
	message := params["message"].(string)
	
	// Get user for permissions and author info
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
		return "", fmt.Errorf("access denied: only admins can write files")
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
	
	// Check if file exists to determine if it's create or overwrite
	existingFile, _ := repo.GetFile(branch, path)
	isNew := existingFile == nil
	
	// Write the file (creates or overwrites)
	err = repo.WriteFile(branch, path, content, message, user.Name, user.Email)
	if err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}
	
	// Format success response
	var result strings.Builder
	if isNew {
		result.WriteString(fmt.Sprintf("✅ **File Created Successfully**\n\n"))
	} else {
		result.WriteString(fmt.Sprintf("✅ **File Overwritten Successfully**\n\n"))
	}
	result.WriteString(fmt.Sprintf("**Repository:** %s\n", repo.Name))
	result.WriteString(fmt.Sprintf("**File:** %s\n", path))
	result.WriteString(fmt.Sprintf("**Branch:** %s\n", branch))
	result.WriteString(fmt.Sprintf("**Commit Message:** %s\n", message))
	result.WriteString(fmt.Sprintf("**Author:** %s <%s>\n", user.Name, user.Email))
	
	return result.String(), nil
}

// DeleteFileTool removes a file from a repository
type DeleteFileTool struct{}

func (t *DeleteFileTool) Name() string {
	return "delete_file"
}

func (t *DeleteFileTool) Description() string {
	return "Delete a file from a repository. Required params: repo_id, path, message. Optional params: branch"
}

func (t *DeleteFileTool) ValidateParams(params map[string]interface{}) error {
	repoID, exists := params["repo_id"]
	if !exists {
		return fmt.Errorf("repo_id is required")
	}
	if _, ok := repoID.(string); !ok {
		return fmt.Errorf("repo_id must be a string")
	}
	
	path, exists := params["path"]
	if !exists {
		return fmt.Errorf("path is required")
	}
	pathStr, ok := path.(string)
	if !ok {
		return fmt.Errorf("path must be a string")
	}
	if strings.Contains(pathStr, "..") {
		return fmt.Errorf("invalid path: directory traversal not allowed")
	}
	
	message, exists := params["message"]
	if !exists {
		return fmt.Errorf("message is required (commit message)")
	}
	if _, ok := message.(string); !ok {
		return fmt.Errorf("message must be a string")
	}
	
	return nil
}

func (t *DeleteFileTool) Schema() map[string]interface{} {
	return SimpleSchema(map[string]interface{}{
		"repo_id": map[string]interface{}{
			"type":        "string",
			"description": "The repository ID",
			"required":    true,
		},
		"path": map[string]interface{}{
			"type":        "string",
			"description": "Path to the file to delete",
			"required":    true,
		},
		"message": map[string]interface{}{
			"type":        "string",
			"description": "Commit message describing the deletion",
			"required":    true,
		},
		"branch": map[string]interface{}{
			"type":        "string",
			"description": "Branch to delete file from",
		},
	})
}

func (t *DeleteFileTool) Execute(params map[string]interface{}, userID string) (string, error) {
	repoID := params["repo_id"].(string)
	path := params["path"].(string)
	message := params["message"].(string)
	
	// Get user for permissions and author info
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
		return "", fmt.Errorf("access denied: only admins can delete files")
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
	
	// Delete the file
	err = repo.DeleteFile(branch, path, message, user.Name, user.Email)
	if err != nil {
		return "", fmt.Errorf("failed to delete file: %w", err)
	}
	
	// Format success response
	var result strings.Builder
	result.WriteString(fmt.Sprintf("✅ **File Deleted Successfully**\n\n"))
	result.WriteString(fmt.Sprintf("**Repository:** %s\n", repo.Name))
	result.WriteString(fmt.Sprintf("**File:** %s\n", path))
	result.WriteString(fmt.Sprintf("**Branch:** %s\n", branch))
	result.WriteString(fmt.Sprintf("**Commit Message:** %s\n", message))
	result.WriteString(fmt.Sprintf("**Author:** %s <%s>\n", user.Name, user.Email))
	
	return result.String(), nil
}

// MoveFileTool renames or moves a file within a repository
type MoveFileTool struct{}

func (t *MoveFileTool) Name() string {
	return "move_file"
}

func (t *MoveFileTool) Description() string {
	return "Move or rename a file within a repository. Required params: repo_id, old_path, new_path, message. Optional params: branch"
}

func (t *MoveFileTool) ValidateParams(params map[string]interface{}) error {
	repoID, exists := params["repo_id"]
	if !exists {
		return fmt.Errorf("repo_id is required")
	}
	if _, ok := repoID.(string); !ok {
		return fmt.Errorf("repo_id must be a string")
	}
	
	oldPath, exists := params["old_path"]
	if !exists {
		return fmt.Errorf("old_path is required")
	}
	oldPathStr, ok := oldPath.(string)
	if !ok {
		return fmt.Errorf("old_path must be a string")
	}
	if strings.Contains(oldPathStr, "..") {
		return fmt.Errorf("invalid old_path: directory traversal not allowed")
	}
	
	newPath, exists := params["new_path"]
	if !exists {
		return fmt.Errorf("new_path is required")
	}
	newPathStr, ok := newPath.(string)
	if !ok {
		return fmt.Errorf("new_path must be a string")
	}
	if strings.Contains(newPathStr, "..") {
		return fmt.Errorf("invalid new_path: directory traversal not allowed")
	}
	
	message, exists := params["message"]
	if !exists {
		return fmt.Errorf("message is required (commit message)")
	}
	if _, ok := message.(string); !ok {
		return fmt.Errorf("message must be a string")
	}
	
	return nil
}

func (t *MoveFileTool) Schema() map[string]interface{} {
	return SimpleSchema(map[string]interface{}{
		"repo_id": map[string]interface{}{
			"type":        "string",
			"description": "The repository ID",
			"required":    true,
		},
		"old_path": map[string]interface{}{
			"type":        "string",
			"description": "Current path of the file",
			"required":    true,
		},
		"new_path": map[string]interface{}{
			"type":        "string",
			"description": "New path for the file",
			"required":    true,
		},
		"message": map[string]interface{}{
			"type":        "string",
			"description": "Commit message describing the move/rename",
			"required":    true,
		},
		"branch": map[string]interface{}{
			"type":        "string",
			"description": "Branch to move file on",
		},
	})
}

func (t *MoveFileTool) Execute(params map[string]interface{}, userID string) (string, error) {
	repoID := params["repo_id"].(string)
	oldPath := params["old_path"].(string)
	newPath := params["new_path"].(string)
	message := params["message"].(string)
	
	// Get user for permissions and author info
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
		return "", fmt.Errorf("access denied: only admins can move files")
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
	
	// Get the original file content
	file, err := repo.GetFile(branch, oldPath)
	if err != nil {
		return "", fmt.Errorf("source file not found: %w", err)
	}
	
	// Create the file at new location
	err = repo.WriteFile(branch, newPath, file.Content, message, user.Name, user.Email)
	if err != nil {
		return "", fmt.Errorf("failed to create file at new location: %w", err)
	}
	
	// Delete the old file
	deleteMessage := fmt.Sprintf("Move %s to %s", oldPath, newPath)
	err = repo.DeleteFile(branch, oldPath, deleteMessage, user.Name, user.Email)
	if err != nil {
		// Try to rollback by deleting the new file
		_ = repo.DeleteFile(branch, newPath, "Rollback failed move", user.Name, user.Email)
		return "", fmt.Errorf("failed to delete original file: %w", err)
	}
	
	// Format success response
	var result strings.Builder
	result.WriteString(fmt.Sprintf("✅ **File Moved Successfully**\n\n"))
	result.WriteString(fmt.Sprintf("**Repository:** %s\n", repo.Name))
	result.WriteString(fmt.Sprintf("**From:** %s\n", oldPath))
	result.WriteString(fmt.Sprintf("**To:** %s\n", newPath))
	result.WriteString(fmt.Sprintf("**Branch:** %s\n", branch))
	result.WriteString(fmt.Sprintf("**Commit Message:** %s\n", message))
	result.WriteString(fmt.Sprintf("**Author:** %s <%s>\n", user.Name, user.Email))
	
	return result.String(), nil
}