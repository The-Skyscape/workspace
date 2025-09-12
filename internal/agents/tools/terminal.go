package tools

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"workspace/models"
	"workspace/services"
)

// RunCommandTool executes shell commands in a sandboxed environment
type RunCommandTool struct{}

func (t *RunCommandTool) Name() string {
	return "run_command"
}

func (t *RunCommandTool) Description() string {
	return "Execute a shell command in a sandboxed environment. Required params: command. Optional params: repo_id, working_dir, timeout_seconds"
}

func (t *RunCommandTool) ValidateParams(params map[string]any) error {
	command, exists := params["command"]
	if !exists || command == nil || command == "" {
		return fmt.Errorf("command is required")
	}

	if _, ok := command.(string); !ok {
		return fmt.Errorf("command must be a string")
	}

	// Validate timeout if provided
	if timeout, exists := params["timeout_seconds"]; exists {
		if timeoutFloat, ok := timeout.(float64); ok {
			if timeoutFloat < 1 || timeoutFloat > 300 {
				return fmt.Errorf("timeout_seconds must be between 1 and 300")
			}
		}
	}

	return nil
}

func (t *RunCommandTool) Schema() map[string]any {
	return SimpleSchema(map[string]any{
		"command": map[string]any{
			"type":        "string",
			"description": "The shell command to execute",
			"required":    true,
		},
		"repo_id": map[string]any{
			"type":        "string",
			"description": "Repository ID to execute command in (optional)",
		},
		"working_dir": map[string]any{
			"type":        "string",
			"description": "Working directory for command execution (default: /workspace/repo)",
		},
		"timeout_seconds": map[string]any{
			"type":        "number",
			"description": "Command timeout in seconds (default: 30, max: 300)",
			"default":     30,
		},
	})
}

func (t *RunCommandTool) Execute(params map[string]any, userID string) (string, error) {
	// Get user to check permissions
	user, err := models.Auth.GetUser(userID)
	if err != nil {
		return "", fmt.Errorf("failed to get user: %w", err)
	}

	if !user.IsAdmin {
		return "", fmt.Errorf("only administrators can execute commands")
	}

	command := params["command"].(string)

	// Get optional parameters
	var repoID, workingDir string
	var repoPath string

	if rid, exists := params["repo_id"]; exists && rid != nil {
		repoID = rid.(string)
		// Get repository to mount
		repo, err := models.Repositories.Get(repoID)
		if err != nil {
			return "", fmt.Errorf("repository not found: %s", repoID)
		}
		repoPath = repo.Path()
	}

	if wd, exists := params["working_dir"]; exists && wd != nil {
		workingDir = wd.(string)
	} else {
		workingDir = "/workspace/repo"
	}

	timeout := 30
	if t, exists := params["timeout_seconds"]; exists && t != nil {
		if tFloat, ok := t.(float64); ok {
			timeout = int(tFloat)
		}
	}

	// Create sandbox for command execution
	sandboxName := fmt.Sprintf("ai-cmd-%s-%d", user.ID, time.Now().Unix())

	// Wrap command to include working directory
	wrappedCommand := fmt.Sprintf("cd %s && %s", workingDir, command)

	sandbox, err := services.NewSandbox(
		sandboxName,
		repoPath,
		repoID,
		wrappedCommand,
		timeout,
	)
	if err != nil {
		return "", fmt.Errorf("failed to create sandbox: %w", err)
	}

	// Start sandbox
	if err := sandbox.Start(); err != nil {
		return "", fmt.Errorf("failed to start sandbox: %w", err)
	}

	// Wait for completion (up to timeout)
	startTime := time.Now()
	for time.Since(startTime) < time.Duration(timeout+5)*time.Second {
		if !sandbox.IsRunning() {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Get output
	output, err := sandbox.GetOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get output: %w", err)
	}

	// Get exit code
	exitCode := sandbox.GetExitCode()

	// Clean up sandbox
	sandbox.Cleanup()

	// Format result
	var result strings.Builder
	result.WriteString(fmt.Sprintf("## Command Execution\n\n"))
	result.WriteString(fmt.Sprintf("**Command:** `%s`\n", command))
	if repoID != "" {
		result.WriteString(fmt.Sprintf("**Repository:** %s\n", repoID))
	}
	result.WriteString(fmt.Sprintf("**Working Directory:** %s\n", workingDir))
	result.WriteString(fmt.Sprintf("**Exit Code:** %d\n", exitCode))
	result.WriteString(fmt.Sprintf("**Execution Time:** %.2fs\n\n", time.Since(startTime).Seconds()))

	result.WriteString("### Output:\n")
	result.WriteString("```\n")
	result.WriteString(output)
	result.WriteString("\n```\n")

	if exitCode != 0 {
		result.WriteString(fmt.Sprintf("\n⚠️ Command failed with exit code %d", exitCode))
	}

	return result.String(), nil
}

// GetWorkingDirectoryTool shows the current working directory
type GetWorkingDirectoryTool struct{}

func (t *GetWorkingDirectoryTool) Name() string {
	return "get_working_directory"
}

func (t *GetWorkingDirectoryTool) Description() string {
	return "Get the current working directory in a repository. Required params: repo_id"
}

func (t *GetWorkingDirectoryTool) ValidateParams(params map[string]any) error {
	repoID, exists := params["repo_id"]
	if !exists || repoID == nil || repoID == "" {
		return fmt.Errorf("repo_id is required")
	}

	if _, ok := repoID.(string); !ok {
		return fmt.Errorf("repo_id must be a string")
	}

	return nil
}

func (t *GetWorkingDirectoryTool) Schema() map[string]any {
	return SimpleSchema(map[string]any{
		"repo_id": map[string]any{
			"type":        "string",
			"description": "The repository ID",
			"required":    true,
		},
	})
}

func (t *GetWorkingDirectoryTool) Execute(params map[string]any, userID string) (string, error) {
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

	// Get absolute path
	absPath, _ := filepath.Abs(repo.Path())

	return fmt.Sprintf("Current working directory for repository '%s':\n%s", repo.Name, absPath), nil
}

// InstallPackageTool installs dependencies using package managers
type InstallPackageTool struct{}

func (t *InstallPackageTool) Name() string {
	return "install_package"
}

func (t *InstallPackageTool) Description() string {
	return "Install packages using npm, pip, go get, or other package managers. Required params: package_manager, packages. Optional params: repo_id"
}

func (t *InstallPackageTool) ValidateParams(params map[string]any) error {
	pm, exists := params["package_manager"]
	if !exists || pm == nil || pm == "" {
		return fmt.Errorf("package_manager is required")
	}

	pmStr, ok := pm.(string)
	if !ok {
		return fmt.Errorf("package_manager must be a string")
	}

	validManagers := []string{"npm", "pip", "go", "apt", "cargo", "gem", "composer"}
	valid := false
	for _, v := range validManagers {
		if pmStr == v {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("package_manager must be one of: npm, pip, go, apt, cargo, gem, composer")
	}

	packages, exists := params["packages"]
	if !exists || packages == nil || packages == "" {
		return fmt.Errorf("packages is required")
	}

	if _, ok := packages.(string); !ok {
		return fmt.Errorf("packages must be a string")
	}

	return nil
}

func (t *InstallPackageTool) Schema() map[string]any {
	return SimpleSchema(map[string]any{
		"package_manager": map[string]any{
			"type":        "string",
			"enum":        []string{"npm", "pip", "go", "apt", "cargo", "gem", "composer"},
			"description": "The package manager to use",
			"required":    true,
		},
		"packages": map[string]any{
			"type":        "string",
			"description": "Space-separated list of packages to install",
			"required":    true,
		},
		"repo_id": map[string]any{
			"type":        "string",
			"description": "Repository ID to install packages in (optional)",
		},
		"save": map[string]any{
			"type":        "boolean",
			"description": "Save to dependencies (npm --save, pip freeze > requirements.txt)",
			"default":     true,
		},
	})
}

func (t *InstallPackageTool) Execute(params map[string]any, userID string) (string, error) {
	// Get user to check permissions
	user, err := models.Auth.GetUser(userID)
	if err != nil {
		return "", fmt.Errorf("failed to get user: %w", err)
	}

	if !user.IsAdmin {
		return "", fmt.Errorf("only administrators can install packages")
	}

	packageManager := params["package_manager"].(string)
	packages := params["packages"].(string)

	save := true
	if s, exists := params["save"]; exists && s != nil {
		if sBool, ok := s.(bool); ok {
			save = sBool
		}
	}

	// Build install command based on package manager
	var command string
	switch packageManager {
	case "npm":
		if save {
			command = fmt.Sprintf("npm install --save %s", packages)
		} else {
			command = fmt.Sprintf("npm install %s", packages)
		}
	case "pip":
		command = fmt.Sprintf("pip install %s", packages)
		if save {
			command += " && pip freeze > requirements.txt"
		}
	case "go":
		command = fmt.Sprintf("go get %s", packages)
		if save {
			command += " && go mod tidy"
		}
	case "apt":
		command = fmt.Sprintf("apt-get update && apt-get install -y %s", packages)
	case "cargo":
		command = fmt.Sprintf("cargo add %s", packages)
	case "gem":
		command = fmt.Sprintf("gem install %s", packages)
		if save {
			command += " && bundle add " + packages
		}
	case "composer":
		command = fmt.Sprintf("composer require %s", packages)
	default:
		return "", fmt.Errorf("unsupported package manager: %s", packageManager)
	}

	// Execute using RunCommandTool
	runCmd := &RunCommandTool{}
	result, err := runCmd.Execute(map[string]any{
		"command":         command,
		"repo_id":         params["repo_id"],
		"timeout_seconds": 120, // 2 minutes for package installation
	}, userID)

	if err != nil {
		return "", fmt.Errorf("failed to install packages: %w", err)
	}

	return fmt.Sprintf("## Package Installation\n\n**Package Manager:** %s\n**Packages:** %s\n\n%s",
		packageManager, packages, result), nil
}

// ListProcessesTool shows running processes
type ListProcessesTool struct{}

func (t *ListProcessesTool) Name() string {
	return "list_processes"
}

func (t *ListProcessesTool) Description() string {
	return "List running processes in the system. Optional params: filter"
}

func (t *ListProcessesTool) ValidateParams(params map[string]any) error {
	return nil
}

func (t *ListProcessesTool) Schema() map[string]any {
	return SimpleSchema(map[string]any{
		"filter": map[string]any{
			"type":        "string",
			"description": "Filter processes by name (optional)",
		},
	})
}

func (t *ListProcessesTool) Execute(params map[string]any, userID string) (string, error) {
	// Get user to check permissions
	user, err := models.Auth.GetUser(userID)
	if err != nil {
		return "", fmt.Errorf("failed to get user: %w", err)
	}

	if !user.IsAdmin {
		return "", fmt.Errorf("only administrators can list processes")
	}

	command := "ps aux"
	if filter, exists := params["filter"]; exists && filter != nil {
		command = fmt.Sprintf("ps aux | grep %s", filter.(string))
	}

	// Execute using RunCommandTool
	runCmd := &RunCommandTool{}
	result, err := runCmd.Execute(map[string]any{
		"command":         command,
		"timeout_seconds": 10,
	}, userID)

	if err != nil {
		return "", fmt.Errorf("failed to list processes: %w", err)
	}

	return result, nil
}
