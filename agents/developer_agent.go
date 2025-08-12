package agents

import (
	"fmt"
	"path/filepath"
	"strings"
	"workspace/models"
	"workspace/services"
)

// DeveloperAgent is an AI agent with direct access to workspace models
type DeveloperAgent struct {
	ollama *services.OllamaService
	repoID string
}

// NewDeveloperAgent creates a new developer agent
func NewDeveloperAgent(repoID string) *DeveloperAgent {
	return &DeveloperAgent{
		ollama: services.Ollama,
		repoID: repoID,
	}
}

// ProcessQuery processes a user query with tool execution capabilities
func (d *DeveloperAgent) ProcessQuery(query string, context []map[string]string) (string, error) {
	// Build system prompt with available tools
	systemPrompt := d.buildSystemPrompt()
	
	// Add system message if not present
	messages := []map[string]string{}
	if len(context) == 0 || context[0]["role"] != "system" {
		messages = append(messages, map[string]string{
			"role":    "system",
			"content": systemPrompt,
		})
	}
	messages = append(messages, context...)
	messages = append(messages, map[string]string{
		"role":    "user",
		"content": query,
	})
	
	// Select appropriate model based on query complexity
	model := d.selectModel(query)
	
	// Get initial response
	response, err := d.ollama.Chat(messages, model)
	if err != nil {
		return "", err
	}
	
	// Check if response contains tool calls
	toolCalls := d.parseToolCalls(response)
	if len(toolCalls) > 0 {
		// Execute tools and get results
		toolResults := d.executeTools(toolCalls)
		
		// Add tool results to context
		messages = append(messages, map[string]string{
			"role":    "assistant",
			"content": response,
		})
		messages = append(messages, map[string]string{
			"role":    "user",
			"content": "Tool execution results:\n" + toolResults,
		})
		
		// Get final response with tool results
		finalResponse, err := d.ollama.Chat(messages, model)
		if err != nil {
			return response, nil // Return original if final fails
		}
		return finalResponse, nil
	}
	
	return response, nil
}

// buildSystemPrompt creates the system prompt with tool descriptions
func (d *DeveloperAgent) buildSystemPrompt() string {
	repo, _ := models.GetRepositoryByID(d.repoID)
	repoName := "repository"
	if repo != nil {
		repoName = repo.Name
	}
	
	return fmt.Sprintf(`You are a developer assistant AI with direct access to the %s repository and database.

You have access to the following tools:

1. **search_files** - Search for files in the repository
   Parameters: pattern (glob pattern like "*.go" or "**/*.js")

2. **read_file** - Read a file's content
   Parameters: path (file path relative to repository root)

3. **list_commits** - Get recent commits
   Parameters: limit (number of commits, default 10), branch (default HEAD)

4. **search_code** - Search for code patterns
   Parameters: pattern (search term or regex), file_type (optional file extension)

5. **get_issues** - List repository issues
   Parameters: status (open/closed/all), limit (default 10)

6. **analyze_structure** - Analyze repository structure
   Parameters: none

7. **database_query** - Query the database
   Parameters: table (repositories/issues/users/etc), conditions (SQL WHERE conditions), limit

When you need to use a tool, respond with:
[TOOL: tool_name]
[PARAMS]
param1: value1
param2: value2
[/PARAMS]

You can use multiple tools in one response. After I provide the tool results, give a comprehensive answer.

Current repository context: %s (ID: %s)`, repoName, repoName, d.repoID)
}

// parseToolCalls extracts tool calls from the response
func (d *DeveloperAgent) parseToolCalls(response string) []map[string]interface{} {
	var toolCalls []map[string]interface{}
	
	lines := strings.Split(response, "\n")
	i := 0
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])
		
		if strings.HasPrefix(line, "[TOOL:") && strings.HasSuffix(line, "]") {
			toolName := strings.TrimSpace(line[6 : len(line)-1])
			params := make(map[string]string)
			
			// Look for [PARAMS]
			i++
			if i < len(lines) && strings.TrimSpace(lines[i]) == "[PARAMS]" {
				i++
				// Parse parameters until [/PARAMS]
				for i < len(lines) && strings.TrimSpace(lines[i]) != "[/PARAMS]" {
					paramLine := strings.TrimSpace(lines[i])
					if colonIdx := strings.Index(paramLine, ":"); colonIdx > 0 {
						key := strings.TrimSpace(paramLine[:colonIdx])
						value := strings.TrimSpace(paramLine[colonIdx+1:])
						params[key] = value
					}
					i++
				}
			}
			
			toolCalls = append(toolCalls, map[string]interface{}{
				"tool":   toolName,
				"params": params,
			})
		}
		i++
	}
	
	return toolCalls
}

// executeTools executes the requested tools
func (d *DeveloperAgent) executeTools(toolCalls []map[string]interface{}) string {
	var results []string
	
	for _, call := range toolCalls {
		toolName := call["tool"].(string)
		params := call["params"].(map[string]string)
		
		result := d.executeTool(toolName, params)
		results = append(results, fmt.Sprintf("Tool: %s\nResult: %s", toolName, result))
	}
	
	return strings.Join(results, "\n\n---\n\n")
}

// executeTool executes a single tool
func (d *DeveloperAgent) executeTool(toolName string, params map[string]string) string {
	switch toolName {
	case "search_files":
		return d.toolSearchFiles(params)
	case "read_file":
		return d.toolReadFile(params)
	case "list_commits":
		return d.toolListCommits(params)
	case "search_code":
		return d.toolSearchCode(params)
	case "get_issues":
		return d.toolGetIssues(params)
	case "analyze_structure":
		return d.toolAnalyzeStructure(params)
	case "database_query":
		return d.toolDatabaseQuery(params)
	default:
		return fmt.Sprintf("Unknown tool: %s", toolName)
	}
}

// toolSearchFiles searches for files in the repository
func (d *DeveloperAgent) toolSearchFiles(params map[string]string) string {
	pattern := params["pattern"]
	if pattern == "" {
		pattern = "*"
	}
	
	repo, err := models.GetRepositoryByID(d.repoID)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	
	// Use git to list files
	stdout, _, err := repo.Git("ls-tree", "-r", "--name-only", "HEAD")
	if err != nil {
		return fmt.Sprintf("Error listing files: %v", err)
	}
	
	files := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	var matchedFiles []string
	
	for _, file := range files {
		if matched, _ := filepath.Match(pattern, filepath.Base(file)); matched {
			matchedFiles = append(matchedFiles, file)
		}
	}
	
	if len(matchedFiles) == 0 {
		return "No files found matching pattern: " + pattern
	}
	
	return fmt.Sprintf("Found %d files:\n%s", len(matchedFiles), strings.Join(matchedFiles, "\n"))
}

// toolReadFile reads a file from the repository
func (d *DeveloperAgent) toolReadFile(params map[string]string) string {
	path := params["path"]
	if path == "" {
		return "Error: path parameter is required"
	}
	
	repo, err := models.GetRepositoryByID(d.repoID)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	
	// Use git to read file
	stdout, _, err := repo.Git("show", "HEAD:"+path)
	if err != nil {
		return fmt.Sprintf("Error reading file: %v", err)
	}
	
	content := stdout.String()
	if len(content) > 5000 {
		content = content[:5000] + "\n... (truncated)"
	}
	
	return fmt.Sprintf("File: %s\n\n%s", path, content)
}

// toolListCommits lists recent commits
func (d *DeveloperAgent) toolListCommits(params map[string]string) string {
	limit := params["limit"]
	if limit == "" {
		limit = "10"
	}
	
	branch := params["branch"]
	if branch == "" {
		branch = "HEAD"
	}
	
	repo, err := models.GetRepositoryByID(d.repoID)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	
	stdout, _, err := repo.Git("log", "--oneline", "-n", limit, branch)
	if err != nil {
		return fmt.Sprintf("Error getting commits: %v", err)
	}
	
	return fmt.Sprintf("Recent commits on %s:\n%s", branch, stdout.String())
}

// toolSearchCode searches for code patterns
func (d *DeveloperAgent) toolSearchCode(params map[string]string) string {
	pattern := params["pattern"]
	if pattern == "" {
		return "Error: pattern parameter is required"
	}
	
	fileType := params["file_type"]
	
	repo, err := models.GetRepositoryByID(d.repoID)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	
	// Build grep command
	args := []string{"grep", "-r", "-n", "--max-count=20", pattern}
	if fileType != "" {
		args = append(args, "--include=*"+fileType)
	}
	
	stdout, stderr, err := repo.Git(args...)
	if err != nil {
		// Check if no matches found
		if strings.Contains(stderr.String(), "No such") || stdout.Len() == 0 {
			return fmt.Sprintf("No matches found for pattern: %s", pattern)
		}
		return fmt.Sprintf("Error searching: %v", err)
	}
	
	results := stdout.String()
	if results == "" {
		return fmt.Sprintf("No matches found for pattern: %s", pattern)
	}
	
	lines := strings.Split(results, "\n")
	if len(lines) > 20 {
		lines = lines[:20]
		results = strings.Join(lines, "\n") + "\n... (more results truncated)"
	}
	
	return fmt.Sprintf("Search results for '%s':\n%s", pattern, results)
}

// toolGetIssues lists repository issues
func (d *DeveloperAgent) toolGetIssues(params map[string]string) string {
	status := params["status"]
	if status == "" {
		status = "open"
	}
	
	limit := params["limit"]
	if limit == "" {
		limit = "10"
	}
	
	// Query issues from database
	var issues []*models.Issue
	var err error
	
	if status == "all" {
		issues, err = models.Issues.Search("WHERE RepoID = ? LIMIT ?", d.repoID, limit)
	} else {
		issues, err = models.Issues.Search("WHERE RepoID = ? AND Status = ? LIMIT ?", d.repoID, status, limit)
	}
	
	if err != nil {
		return fmt.Sprintf("Error getting issues: %v", err)
	}
	
	if len(issues) == 0 {
		return fmt.Sprintf("No %s issues found", status)
	}
	
	var issueList []string
	for _, issue := range issues {
		issueList = append(issueList, fmt.Sprintf("#%s: %s (%s)", issue.ID, issue.Title, issue.Status))
	}
	
	return fmt.Sprintf("Issues (%s):\n%s", status, strings.Join(issueList, "\n"))
}

// toolAnalyzeStructure analyzes repository structure
func (d *DeveloperAgent) toolAnalyzeStructure(params map[string]string) string {
	repo, err := models.GetRepositoryByID(d.repoID)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	
	// Get file count
	fileCount, _ := repo.GetFileCount()
	
	// Get language statistics
	langStats, _ := repo.GetLanguageStats()
	
	// Get contributors
	contributors, _ := repo.GetContributors()
	
	// Get commit count
	commitCount, _ := repo.GetCommitCount("HEAD")
	
	// Build structure analysis
	var languages []string
	for lang, lines := range langStats {
		languages = append(languages, fmt.Sprintf("%s: %d lines", lang, lines))
	}
	
	return fmt.Sprintf(`Repository Structure Analysis:
- Name: %s
- Files: %d
- Commits: %d  
- Contributors: %d
- Languages:
  %s
- Description: %s`,
		repo.Name,
		fileCount,
		commitCount,
		len(contributors),
		strings.Join(languages, "\n  "),
		repo.Description)
}

// toolDatabaseQuery queries the database
func (d *DeveloperAgent) toolDatabaseQuery(params map[string]string) string {
	table := params["table"]
	if table == "" {
		return "Error: table parameter is required"
	}
	
	conditions := params["conditions"]
	limit := params["limit"]
	if limit == "" {
		limit = "10"
	}
	
	// Build query
	query := ""
	if conditions != "" {
		query = fmt.Sprintf("WHERE %s", conditions)
	}
	query += fmt.Sprintf(" LIMIT %s", limit)
	
	// Execute query based on table
	switch table {
	case "repositories":
		repos, err := models.Repositories.Search(query)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		var results []string
		for _, repo := range repos {
			results = append(results, fmt.Sprintf("%s: %s (%s)", repo.ID, repo.Name, repo.Visibility))
		}
		return fmt.Sprintf("Repositories:\n%s", strings.Join(results, "\n"))
		
	case "issues":
		issues, err := models.Issues.Search(query)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		var results []string
		for _, issue := range issues {
			results = append(results, fmt.Sprintf("#%s: %s (%s)", issue.ID, issue.Title, issue.Status))
		}
		return fmt.Sprintf("Issues:\n%s", strings.Join(results, "\n"))
		
	case "users":
		users, err := models.Auth.Users.Search(query)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		var results []string
		for _, user := range users {
			role := "user"
			if user.IsAdmin {
				role = "admin"
			}
			results = append(results, fmt.Sprintf("%s: %s (%s)", user.ID, user.Name, role))
		}
		return fmt.Sprintf("Users:\n%s", strings.Join(results, "\n"))
		
	default:
		return fmt.Sprintf("Unknown table: %s", table)
	}
}

// SummarizeRepository generates a comprehensive repository summary
func (d *DeveloperAgent) SummarizeRepository() (string, error) {
	repo, err := models.GetRepositoryByID(d.repoID)
	if err != nil {
		return "", err
	}
	
	// Gather repository information
	fileCount, _ := repo.GetFileCount()
	commitCount, _ := repo.GetCommitCount("HEAD")
	langStats, _ := repo.GetLanguageStats()
	contributors, _ := repo.GetContributors()
	
	// Read README if exists
	readme := ""
	stdout, _, err := repo.Git("show", "HEAD:README.md")
	if err == nil {
		readme = stdout.String()
		if len(readme) > 500 {
			readme = readme[:500] + "..."
		}
	}
	
	// Build context for AI
	var languages []string
	for lang := range langStats {
		languages = append(languages, lang)
	}
	
	prompt := fmt.Sprintf(`Generate a comprehensive summary for this repository:

Repository: %s
Description: %s
Files: %d
Commits: %d
Contributors: %d
Languages: %s

README excerpt:
%s

Please provide:
1. A brief overview of what this repository is
2. The main technologies and frameworks used
3. The repository's purpose and key features
4. Any notable patterns or architecture observations`,
		repo.Name,
		repo.Description,
		fileCount,
		commitCount,
		len(contributors),
		strings.Join(languages, ", "),
		readme)
	
	// Use a capable model for summarization
	model := "llama3.1:8b"
	if !d.isModelAvailable(model) {
		model = "llama3.2:3b"
	}
	
	return d.ollama.Generate(prompt, model)
}


// selectModel chooses the appropriate model based on query complexity
func (d *DeveloperAgent) selectModel(query string) string {
	// Check query characteristics
	lowerQuery := strings.ToLower(query)
	
	// Use specialized models for specific tasks
	if strings.Contains(lowerQuery, "code") || 
	   strings.Contains(lowerQuery, "function") ||
	   strings.Contains(lowerQuery, "implement") ||
	   strings.Contains(lowerQuery, "debug") ||
	   strings.Contains(lowerQuery, "fix") {
		return "codellama:7b" // Best for code-related tasks
	}
	
	// Use larger models for complex reasoning
	if strings.Contains(lowerQuery, "analyze") ||
	   strings.Contains(lowerQuery, "explain") ||
	   strings.Contains(lowerQuery, "compare") ||
	   strings.Contains(lowerQuery, "design") ||
	   strings.Contains(lowerQuery, "architecture") ||
	   len(query) > 200 { // Longer queries often need more reasoning
		// Use more capable models for complex tasks
		// Try models in order of preference
		if d.isModelAvailable("llama3.1:8b") {
			return "llama3.1:8b"
		}
		if d.isModelAvailable("gemma2:9b") {
			return "gemma2:9b"
		}
		if d.isModelAvailable("qwen2.5:7b") {
			return "qwen2.5:7b"
		}
	}
	
	// Use lightweight models for simple queries
	if len(query) < 50 && !strings.Contains(lowerQuery, "?") {
		if d.isModelAvailable("phi3:mini") {
			return "phi3:mini"
		}
	}
	
	// Default to llama3.2:3b for general tasks
	return "llama3.2:3b"
}

// isModelAvailable checks if a model is available locally
func (d *DeveloperAgent) isModelAvailable(model string) bool {
	models, err := d.ollama.ListModels()
	if err != nil {
		return false
	}
	
	for _, m := range models {
		if m == model {
			return true
		}
	}
	return false
}

// getLanguageFromExt returns language name from file extension
func (d *DeveloperAgent) getLanguageFromExt(ext string) string {
	langMap := map[string]string{
		".go":   "Go",
		".js":   "JavaScript",
		".jsx":  "React",
		".ts":   "TypeScript",
		".tsx":  "React TypeScript",
		".py":   "Python",
		".java": "Java",
		".c":    "C",
		".cpp":  "C++",
		".cs":   "C#",
		".rb":   "Ruby",
		".rs":   "Rust",
		".php":  "PHP",
		".swift": "Swift",
		".kt":   "Kotlin",
		".html": "HTML",
		".css":  "CSS",
		".sql":  "SQL",
		".sh":   "Shell",
		".yml":  "YAML",
		".json": "JSON",
		".xml":  "XML",
		".md":   "Markdown",
	}
	
	if lang, ok := langMap[strings.ToLower(ext)]; ok {
		return lang
	}
	return "code"
}