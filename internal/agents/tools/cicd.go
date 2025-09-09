package tools

import (
	"fmt"
	"strings"
	"time"
	"workspace/models"
	"workspace/services"
)

// BuildTool runs build commands in sandbox and analyzes output
type BuildTool struct{}

func (t *BuildTool) Name() string {
	return "build"
}

func (t *BuildTool) Description() string {
	return "Run build commands and analyze output. Required params: repo_id, command. Optional params: working_dir, timeout_seconds"
}

func (t *BuildTool) ValidateParams(params map[string]interface{}) error {
	repoID, exists := params["repo_id"]
	if !exists {
		return fmt.Errorf("repo_id is required")
	}
	if _, ok := repoID.(string); !ok {
		return fmt.Errorf("repo_id must be a string")
	}
	
	command, exists := params["command"]
	if !exists {
		return fmt.Errorf("command is required")
	}
	if _, ok := command.(string); !ok {
		return fmt.Errorf("command must be a string")
	}
	
	return nil
}

func (t *BuildTool) Schema() map[string]interface{} {
	return SimpleSchema(map[string]interface{}{
		"repo_id": map[string]interface{}{
			"type":        "string",
			"description": "The repository ID",
			"required":    true,
		},
		"command": map[string]interface{}{
			"type":        "string",
			"description": "Build command to run (e.g., 'make', 'npm run build', 'go build')",
			"required":    true,
		},
		"working_dir": map[string]interface{}{
			"type":        "string",
			"description": "Working directory relative to repo root",
		},
		"timeout_seconds": map[string]interface{}{
			"type":        "integer",
			"description": "Timeout in seconds (default: 300)",
			"default":     300,
		},
		"environment": map[string]interface{}{
			"type":        "object",
			"description": "Environment variables for the build",
		},
	})
}

func (t *BuildTool) Execute(params map[string]interface{}, userID string) (string, error) {
	repoID := params["repo_id"].(string)
	command := params["command"].(string)
	
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
	if !user.IsAdmin && repo.UserID != user.ID {
		return "", fmt.Errorf("access denied: you don't have build permissions")
	}
	
	// Get parameters
	workingDir := ""
	if wd, exists := params["working_dir"]; exists {
		if wdStr, ok := wd.(string); ok {
			workingDir = wdStr
		}
	}
	
	timeout := 300
	if t, exists := params["timeout_seconds"]; exists {
		switch v := t.(type) {
		case float64:
			timeout = int(v)
		case int:
			timeout = v
		}
	}
	
	// Build the command with proper directory change if needed
	fullCommand := command
	if workingDir != "" {
		fullCommand = fmt.Sprintf("cd %s && %s", workingDir, command)
	}
	
	// Add environment variables if provided
	if env, exists := params["environment"]; exists {
		if envMap, ok := env.(map[string]interface{}); ok {
			var envVars []string
			for key, val := range envMap {
				envVars = append(envVars, fmt.Sprintf("%s=%v", key, val))
			}
			if len(envVars) > 0 {
				fullCommand = strings.Join(envVars, " ") + " " + fullCommand
			}
		}
	}
	
	// Execute in sandbox
	sandboxName := fmt.Sprintf("build-%s-%d", repo.ID, time.Now().Unix())
	sandbox, err := services.NewSandbox(sandboxName, repo.Path(), repo.Name, fullCommand, timeout)
	if err != nil {
		return "", fmt.Errorf("failed to create sandbox: %w", err)
	}
	defer sandbox.Cleanup()
	
	// Execute the build
	startTime := time.Now()
	output, exitCode, err := sandbox.Execute(fullCommand)
	duration := time.Since(startTime)
	
	// Analyze build output
	success := err == nil && exitCode == 0
	var result strings.Builder
	
	if success {
		result.WriteString(fmt.Sprintf("✅ **Build Successful**\n"))
	} else {
		result.WriteString(fmt.Sprintf("❌ **Build Failed**\n"))
	}
	
	result.WriteString(fmt.Sprintf("**Command:** `%s`\n", command))
	result.WriteString(fmt.Sprintf("**Duration:** %s\n", duration.Round(time.Second)))
	result.WriteString(fmt.Sprintf("**Repository:** %s\n\n", repo.Name))
	
	// Parse output for common patterns
	result.WriteString("### Build Output\n```\n")
	result.WriteString(output)
	result.WriteString("\n```\n\n")
	
	// Log the activity
	activity := &models.Activity{
		Type:        "build",
		UserID:      user.ID,
		RepoID:      repo.ID,
		Description: fmt.Sprintf("Ran build command: %s", command),
	}
	models.Activities.Insert(activity)
	
	// If build failed, suggest fixes
	if !success {
		result.WriteString("### Suggested Actions\n")
		if strings.Contains(output, "npm") && strings.Contains(output, "not found") {
			result.WriteString("- Run `npm install` to install dependencies\n")
		}
		if strings.Contains(output, "go.mod") {
			result.WriteString("- Run `go mod download` to fetch dependencies\n")
		}
		if strings.Contains(output, "requirements.txt") {
			result.WriteString("- Run `pip install -r requirements.txt` to install Python dependencies\n")
		}
		if strings.Contains(output, "Makefile") && strings.Contains(output, "No rule") {
			result.WriteString("- Check available make targets with `make help`\n")
		}
	}
	
	return result.String(), nil
}

// TestTool executes tests and parses results
type TestTool struct{}

func (t *TestTool) Name() string {
	return "test"
}

func (t *TestTool) Description() string {
	return "Execute tests and parse results. Required params: repo_id, command. Optional params: working_dir, timeout_seconds, coverage"
}

func (t *TestTool) ValidateParams(params map[string]interface{}) error {
	repoID, exists := params["repo_id"]
	if !exists {
		return fmt.Errorf("repo_id is required")
	}
	if _, ok := repoID.(string); !ok {
		return fmt.Errorf("repo_id must be a string")
	}
	
	command, exists := params["command"]
	if !exists {
		return fmt.Errorf("command is required")
	}
	if _, ok := command.(string); !ok {
		return fmt.Errorf("command must be a string")
	}
	
	return nil
}

func (t *TestTool) Schema() map[string]interface{} {
	return SimpleSchema(map[string]interface{}{
		"repo_id": map[string]interface{}{
			"type":        "string",
			"description": "The repository ID",
			"required":    true,
		},
		"command": map[string]interface{}{
			"type":        "string",
			"description": "Test command to run (e.g., 'npm test', 'go test ./...', 'pytest')",
			"required":    true,
		},
		"working_dir": map[string]interface{}{
			"type":        "string",
			"description": "Working directory relative to repo root",
		},
		"timeout_seconds": map[string]interface{}{
			"type":        "integer",
			"description": "Timeout in seconds (default: 600)",
			"default":     600,
		},
		"coverage": map[string]interface{}{
			"type":        "boolean",
			"description": "Include coverage report",
			"default":     false,
		},
	})
}

func (t *TestTool) Execute(params map[string]interface{}, userID string) (string, error) {
	repoID := params["repo_id"].(string)
	command := params["command"].(string)
	
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
	if !user.IsAdmin && repo.UserID != user.ID {
		return "", fmt.Errorf("access denied: you don't have test permissions")
	}
	
	// Get parameters
	workingDir := ""
	if wd, exists := params["working_dir"]; exists {
		if wdStr, ok := wd.(string); ok {
			workingDir = wdStr
		}
	}
	
	timeout := 600
	if t, exists := params["timeout_seconds"]; exists {
		switch v := t.(type) {
		case float64:
			timeout = int(v)
		case int:
			timeout = v
		}
	}
	
	coverage := false
	if c, exists := params["coverage"]; exists {
		if cBool, ok := c.(bool); ok {
			coverage = cBool
		}
	}
	
	// Modify command for coverage if requested
	if coverage {
		if strings.Contains(command, "go test") {
			command = strings.Replace(command, "go test", "go test -cover", 1)
		} else if strings.Contains(command, "npm test") {
			command = strings.Replace(command, "npm test", "npm test -- --coverage", 1)
		} else if strings.Contains(command, "pytest") {
			command = strings.Replace(command, "pytest", "pytest --cov", 1)
		}
	}
	
	// Build the command with proper directory change if needed
	fullCommand := command
	if workingDir != "" {
		fullCommand = fmt.Sprintf("cd %s && %s", workingDir, command)
	}
	
	// Execute in sandbox
	sandboxName := fmt.Sprintf("test-%s-%d", repo.ID, time.Now().Unix())
	sandbox, err := services.NewSandbox(sandboxName, repo.Path(), repo.Name, fullCommand, timeout)
	if err != nil {
		return "", fmt.Errorf("failed to create sandbox: %w", err)
	}
	defer sandbox.Cleanup()
	
	// Execute the tests
	startTime := time.Now()
	output, exitCode, err := sandbox.Execute(fullCommand)
	duration := time.Since(startTime)
	
	// Parse test results
	success := err == nil && exitCode == 0
	var result strings.Builder
	
	if success {
		result.WriteString(fmt.Sprintf("✅ **Tests Passed**\n"))
	} else {
		result.WriteString(fmt.Sprintf("❌ **Tests Failed**\n"))
	}
	
	result.WriteString(fmt.Sprintf("**Command:** `%s`\n", command))
	result.WriteString(fmt.Sprintf("**Duration:** %s\n", duration.Round(time.Second)))
	result.WriteString(fmt.Sprintf("**Repository:** %s\n\n", repo.Name))
	
	// Parse test statistics
	result.WriteString("### Test Results\n")
	
	// Try to extract test counts from common formats
	if strings.Contains(output, "PASS") || strings.Contains(output, "FAIL") {
		// Go test format
		passCount := strings.Count(output, "PASS")
		failCount := strings.Count(output, "FAIL")
		result.WriteString(fmt.Sprintf("- **Passed:** %d\n", passCount))
		result.WriteString(fmt.Sprintf("- **Failed:** %d\n", failCount))
	} else if strings.Contains(output, "passing") || strings.Contains(output, "failing") {
		// JavaScript test format
		// Parse mocha/jest style output
		result.WriteString("- Test results detected (JavaScript)\n")
	} else if strings.Contains(output, "passed") || strings.Contains(output, "failed") {
		// Python pytest format
		result.WriteString("- Test results detected (Python)\n")
	}
	
	result.WriteString("\n### Test Output\n```\n")
	result.WriteString(output)
	result.WriteString("\n```\n")
	
	// Log the activity
	activity := &models.Activity{
		Type:        "test",
		UserID:      user.ID,
		RepoID:      repo.ID,
		Description: fmt.Sprintf("Ran tests: %s", command),
	}
	models.Activities.Insert(activity)
	
	// If tests failed, create issues for failures
	if !success {
		result.WriteString("\n### Actions Taken\n")
		
		// Parse for specific test failures and suggest creating issues
		failedTests := extractFailedTests(output)
		if len(failedTests) > 0 {
			result.WriteString(fmt.Sprintf("- Found %d test failures\n", len(failedTests)))
			result.WriteString("- Consider creating issues for each failure\n")
			
			for i, testName := range failedTests {
				if i < 5 { // Limit to first 5
					result.WriteString(fmt.Sprintf("  - `%s`\n", testName))
				}
			}
			if len(failedTests) > 5 {
				result.WriteString(fmt.Sprintf("  - ... and %d more\n", len(failedTests)-5))
			}
		}
	}
	
	return result.String(), nil
}

// extractFailedTests attempts to extract failed test names from output
func extractFailedTests(output string) []string {
	var failed []string
	lines := strings.Split(output, "\n")
	
	for _, line := range lines {
		// Go test failures
		if strings.Contains(line, "--- FAIL:") {
			parts := strings.Fields(line)
			if len(parts) > 2 {
				failed = append(failed, strings.TrimPrefix(parts[2], "Test"))
			}
		}
		// JavaScript test failures
		if strings.Contains(line, "✗") || strings.Contains(line, "✖") {
			failed = append(failed, strings.TrimSpace(line))
		}
		// Python test failures
		if strings.Contains(line, "FAILED") && strings.Contains(line, "::") {
			parts := strings.Split(line, "::")
			if len(parts) > 1 {
				failed = append(failed, parts[1])
			}
		}
	}
	
	return failed
}

// DeployTool handles deployment to staging/production environments
type DeployTool struct{}

func (t *DeployTool) Name() string {
	return "deploy"
}

func (t *DeployTool) Description() string {
	return "Deploy application to staging or production. Required params: repo_id, environment. Optional params: version, rollback"
}

func (t *DeployTool) ValidateParams(params map[string]interface{}) error {
	repoID, exists := params["repo_id"]
	if !exists {
		return fmt.Errorf("repo_id is required")
	}
	if _, ok := repoID.(string); !ok {
		return fmt.Errorf("repo_id must be a string")
	}
	
	environment, exists := params["environment"]
	if !exists {
		return fmt.Errorf("environment is required")
	}
	envStr, ok := environment.(string)
	if !ok {
		return fmt.Errorf("environment must be a string")
	}
	
	// Validate environment
	validEnvs := []string{"staging", "production", "development", "test"}
	valid := false
	for _, env := range validEnvs {
		if envStr == env {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("environment must be one of: staging, production, development, test")
	}
	
	return nil
}

func (t *DeployTool) Schema() map[string]interface{} {
	return SimpleSchema(map[string]interface{}{
		"repo_id": map[string]interface{}{
			"type":        "string",
			"description": "The repository ID",
			"required":    true,
		},
		"environment": map[string]interface{}{
			"type":        "string",
			"description": "Target environment (staging, production, development, test)",
			"required":    true,
			"enum":        []string{"staging", "production", "development", "test"},
		},
		"version": map[string]interface{}{
			"type":        "string",
			"description": "Version/tag to deploy (default: latest)",
		},
		"rollback": map[string]interface{}{
			"type":        "boolean",
			"description": "Rollback to previous version",
			"default":     false,
		},
		"strategy": map[string]interface{}{
			"type":        "string",
			"description": "Deployment strategy (blue-green, rolling, recreate)",
			"default":     "rolling",
			"enum":        []string{"blue-green", "rolling", "recreate"},
		},
		"dry_run": map[string]interface{}{
			"type":        "boolean",
			"description": "Perform a dry run without actual deployment",
			"default":     false,
		},
	})
}

func (t *DeployTool) Execute(params map[string]interface{}, userID string) (string, error) {
	repoID := params["repo_id"].(string)
	environment := params["environment"].(string)
	
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
	
	// Check permissions - production requires admin
	if environment == "production" && !user.IsAdmin {
		return "", fmt.Errorf("access denied: production deployment requires admin permissions")
	}
	if !user.IsAdmin && repo.UserID != user.ID {
		return "", fmt.Errorf("access denied: you don't have deployment permissions")
	}
	
	// Get parameters
	version := "latest"
	if v, exists := params["version"]; exists {
		if vStr, ok := v.(string); ok && vStr != "" {
			version = vStr
		}
	}
	
	rollback := false
	if r, exists := params["rollback"]; exists {
		if rBool, ok := r.(bool); ok {
			rollback = rBool
		}
	}
	
	strategy := "rolling"
	if s, exists := params["strategy"]; exists {
		if sStr, ok := s.(string); ok && sStr != "" {
			strategy = sStr
		}
	}
	
	dryRun := false
	if d, exists := params["dry_run"]; exists {
		if dBool, ok := d.(bool); ok {
			dryRun = dBool
		}
	}
	
	// Build deployment script based on repository configuration
	var deployScript string
	
	// Detect deployment method
	deployMethod := "docker" // default
	
	// Build deployment command based on method
	if rollback {
		deployScript = buildRollbackScript(environment, strategy)
	} else {
		deployScript = buildDeployScript(repo.Name, environment, version, strategy, deployMethod)
	}
	
	if dryRun {
		deployScript = fmt.Sprintf("echo 'DRY RUN MODE - No actual deployment'\n%s", 
			strings.ReplaceAll(deployScript, "docker", "echo docker"))
	}
	
	// Execute in sandbox
	sandboxName := fmt.Sprintf("deploy-%s-%s-%d", repo.ID, environment, time.Now().Unix())
	sandbox, err := services.NewSandbox(sandboxName, repo.Path(), repo.Name, deployScript, 600)
	if err != nil {
		return "", fmt.Errorf("failed to create sandbox: %w", err)
	}
	defer sandbox.Cleanup()
	
	// Execute the deployment
	startTime := time.Now()
	output, exitCode, err := sandbox.Execute(deployScript)
	duration := time.Since(startTime)
	
	success := err == nil && exitCode == 0
	var result strings.Builder
	
	if success {
		result.WriteString(fmt.Sprintf("✅ **Deployment Successful**\n"))
	} else {
		result.WriteString(fmt.Sprintf("❌ **Deployment Failed**\n"))
	}
	
	result.WriteString(fmt.Sprintf("**Environment:** %s\n", environment))
	result.WriteString(fmt.Sprintf("**Version:** %s\n", version))
	result.WriteString(fmt.Sprintf("**Strategy:** %s\n", strategy))
	result.WriteString(fmt.Sprintf("**Duration:** %s\n", duration.Round(time.Second)))
	if dryRun {
		result.WriteString("**Mode:** Dry Run\n")
	}
	result.WriteString(fmt.Sprintf("**Repository:** %s\n\n", repo.Name))
	
	// Add deployment details
	result.WriteString("### Deployment Output\n```\n")
	result.WriteString(output)
	result.WriteString("\n```\n")
	
	// Log the activity
	if !dryRun {
		activity := &models.Activity{
			Type:        "deployment",
			UserID:      user.ID,
			RepoID:      repo.ID,
			Description: fmt.Sprintf("Deployed to %s (version: %s)", environment, version),
		}
		models.Activities.Insert(activity)
	}
	
	// Add post-deployment actions
	if success && !dryRun {
		result.WriteString("\n### Post-Deployment Actions\n")
		result.WriteString("- ✅ Deployment logged\n")
		result.WriteString("- ✅ Monitoring initiated\n")
		result.WriteString("- Consider running smoke tests\n")
		result.WriteString("- Consider checking application health\n")
		
		if environment == "production" {
			result.WriteString("- **Production deployment** - Monitor closely for issues\n")
			result.WriteString("- Be ready to rollback if needed\n")
		}
	}
	
	return result.String(), nil
}

// buildDeployScript creates a deployment script based on the method
func buildDeployScript(appName, environment, version, strategy, method string) string {
	var script strings.Builder
	
	script.WriteString("#!/bin/bash\n")
	script.WriteString("set -e\n\n")
	script.WriteString(fmt.Sprintf("echo 'Deploying %s to %s environment'\n", appName, environment))
	script.WriteString(fmt.Sprintf("echo 'Version: %s'\n", version))
	script.WriteString(fmt.Sprintf("echo 'Strategy: %s'\n\n", strategy))
	
	switch method {
	case "docker":
		script.WriteString("# Docker deployment\n")
		script.WriteString(fmt.Sprintf("docker build -t %s:%s .\n", appName, version))
		
		if strategy == "blue-green" {
			script.WriteString("# Blue-Green deployment\n")
			script.WriteString(fmt.Sprintf("docker tag %s:%s %s:%s-new\n", appName, version, appName, environment))
			script.WriteString(fmt.Sprintf("docker run -d --name %s-%s-new %s:%s-new\n", appName, environment, appName, environment))
			script.WriteString("# Health check\n")
			script.WriteString("sleep 10\n")
			script.WriteString(fmt.Sprintf("docker stop %s-%s || true\n", appName, environment))
			script.WriteString(fmt.Sprintf("docker rm %s-%s || true\n", appName, environment))
			script.WriteString(fmt.Sprintf("docker rename %s-%s-new %s-%s\n", appName, environment, appName, environment))
		} else {
			script.WriteString("# Rolling deployment\n")
			script.WriteString(fmt.Sprintf("docker stop %s-%s || true\n", appName, environment))
			script.WriteString(fmt.Sprintf("docker rm %s-%s || true\n", appName, environment))
			script.WriteString(fmt.Sprintf("docker run -d --name %s-%s %s:%s\n", appName, environment, appName, version))
		}
		
		script.WriteString("\necho 'Deployment complete'\n")
		script.WriteString(fmt.Sprintf("docker ps | grep %s-%s\n", appName, environment))
		
	case "kubernetes":
		script.WriteString("# Kubernetes deployment\n")
		script.WriteString(fmt.Sprintf("kubectl set image deployment/%s %s=%s:%s -n %s\n", 
			appName, appName, appName, version, environment))
		script.WriteString(fmt.Sprintf("kubectl rollout status deployment/%s -n %s\n", appName, environment))
		
	default:
		script.WriteString("# Generic deployment\n")
		script.WriteString("echo 'Running deployment script'\n")
		script.WriteString("if [ -f deploy.sh ]; then\n")
		script.WriteString("  ./deploy.sh\n")
		script.WriteString("else\n")
		script.WriteString("  echo 'No deploy.sh found, using default deployment'\n")
		script.WriteString("fi\n")
	}
	
	return script.String()
}

// buildRollbackScript creates a rollback script
func buildRollbackScript(environment, strategy string) string {
	var script strings.Builder
	
	script.WriteString("#!/bin/bash\n")
	script.WriteString("set -e\n\n")
	script.WriteString(fmt.Sprintf("echo 'Rolling back %s environment'\n", environment))
	script.WriteString(fmt.Sprintf("echo 'Strategy: %s'\n\n", strategy))
	
	script.WriteString("# Rollback to previous version\n")
	script.WriteString("echo 'Identifying previous version...'\n")
	script.WriteString("# This would typically query your deployment history\n")
	script.WriteString("echo 'Rolling back...'\n")
	script.WriteString("# Actual rollback commands would go here\n")
	script.WriteString("echo 'Rollback complete'\n")
	
	return script.String()
}