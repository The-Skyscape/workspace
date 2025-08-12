package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AIManager provides AI capabilities using Ollama
type AIManager struct {
	ollama *OllamaService
}

var (
	// Global AI manager instance
	AI = &AIManager{
		ollama: Ollama,
	}
)

// AnalyzeCode analyzes code and provides insights
func (m *AIManager) AnalyzeCode(filePath string, content string) (string, error) {
	ext := filepath.Ext(filePath)
	language := m.getLanguageFromExt(ext)

	prompt := fmt.Sprintf(`Analyze this %s code and provide insights:
File: %s

%s

Provide:
1. A brief summary of what the code does
2. Any potential issues or improvements
3. Code quality observations`, language, filePath, content)

	return m.ollama.Generate(prompt, "codellama:7b")
}

// SummarizeRepository generates a summary of a repository
func (m *AIManager) SummarizeRepository(repoID string, repoName string, repoDescription string) (string, error) {
	prompt := fmt.Sprintf(`Generate a comprehensive summary for this repository:

Repository: %s
Description: %s

Please provide:
1. A brief overview of what this repository appears to be
2. The main technologies and frameworks used (based on the name and description)
3. The repository's purpose and potential use cases
4. Any suggestions for development`, 
		repoName,
		repoDescription)

	return m.ollama.Generate(prompt, "llama3.2:3b")
}

// GeneratePRDescription generates a pull request description from diff
func (m *AIManager) GeneratePRDescription(diff string) (string, error) {
	prompt := fmt.Sprintf(`Based on the following git diff, generate a professional pull request description:

%s

Include:
1. A concise title
2. Summary of changes
3. Type of change (feature, bugfix, refactor, etc.)
4. Any breaking changes
5. Testing notes`, diff)

	return m.ollama.Generate(prompt, "codellama:7b")
}

// ExplainCode explains what a piece of code does
func (m *AIManager) ExplainCode(code string, language string) (string, error) {
	prompt := fmt.Sprintf(`Explain what this %s code does in simple terms:

%s

Provide a clear explanation that a developer who is not familiar with this code could understand.`, language, code)

	return m.ollama.Generate(prompt, "codellama:7b")
}

// ChatWithContext handles contextual chat about a repository
func (m *AIManager) ChatWithContext(repoID string, messages []map[string]string) (string, error) {
	// Simple chat without repository context (context will be added by agent)
	return m.ollama.Chat(messages, "llama3.2:3b")
}

// ChatWithAgent handles contextual chat with agent capabilities
// This is a placeholder that can be enhanced with agent functionality
func (m *AIManager) ChatWithAgent(repoID string, messages []map[string]string) (string, error) {
	// For now, use regular chat
	// In the future, this can use the developer agent for enhanced capabilities
	return m.ChatWithContext(repoID, messages)
}

// SearchSimilarCode finds similar code patterns in the repository
func (m *AIManager) SearchSimilarCode(repoID string, codeSnippet string) ([]string, error) {
	prompt := fmt.Sprintf(`Given this code snippet:

%s

What patterns, functions, or structures should I search for to find similar code? 
List specific search terms, function names, or patterns (one per line).`, codeSnippet)

	response, err := m.ollama.Generate(prompt, "codellama:7b")
	if err != nil {
		return nil, err
	}

	// Parse response into search terms
	lines := strings.Split(response, "\n")
	var searchTerms []string
	for _, line := range lines {
		term := strings.TrimSpace(line)
		// Skip empty lines and explanatory text
		if term != "" && !strings.HasPrefix(term, "-") && !strings.Contains(term, ":") {
			searchTerms = append(searchTerms, term)
		}
	}

	return searchTerms, nil
}

// GenerateCommitMessage generates a commit message from staged changes
func (m *AIManager) GenerateCommitMessage(diff string) (string, error) {
	prompt := fmt.Sprintf(`Generate a concise, conventional commit message for these changes:

%s

Follow conventional commit format (type: description). Keep it under 72 characters.
Types: feat, fix, docs, style, refactor, test, chore`, diff)

	response, err := m.ollama.Generate(prompt, "codellama:7b")
	if err != nil {
		return "", err
	}

	// Extract just the commit message line
	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			return line, nil
		}
	}

	return response, nil
}

// ReviewCode performs an AI code review
func (m *AIManager) ReviewCode(filePath string, content string) (string, error) {
	ext := filepath.Ext(filePath)
	language := m.getLanguageFromExt(ext)

	prompt := fmt.Sprintf(`Review this %s code for potential issues:
File: %s

%s

Provide a code review focusing on:
1. Bugs or potential runtime errors
2. Security concerns
3. Performance issues
4. Code style and best practices
5. Suggestions for improvement

Format as a professional code review.`, language, filePath, content)

	return m.ollama.Generate(prompt, "codellama:7b")
}

// GenerateDocumentation generates documentation for code
func (m *AIManager) GenerateDocumentation(code string, language string) (string, error) {
	prompt := fmt.Sprintf(`Generate comprehensive documentation for this %s code:

%s

Include:
1. Purpose and overview
2. Function/method descriptions
3. Parameters and return values
4. Usage examples
5. Any important notes or warnings`, language, code)

	return m.ollama.Generate(prompt, "codellama:7b")
}

// GenerateTests generates test cases for code
func (m *AIManager) GenerateTests(code string, language string) (string, error) {
	prompt := fmt.Sprintf(`Generate comprehensive test cases for this %s code:

%s

Include:
1. Unit tests for each function/method
2. Edge cases
3. Error scenarios
4. Example test data
5. Assertions to verify correct behavior

Use the appropriate testing framework for %s.`, language, code, language)

	return m.ollama.Generate(prompt, "codellama:7b")
}

// getLanguageFromExt returns the language name from file extension
func (m *AIManager) getLanguageFromExt(ext string) string {
	langMap := map[string]string{
		".go":   "Go",
		".js":   "JavaScript",
		".jsx":  "React/JSX",
		".ts":   "TypeScript",
		".tsx":  "React/TSX",
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

// IsAvailable checks if the AI service is available
func (m *AIManager) IsAvailable() bool {
	return m.ollama != nil && m.ollama.IsRunning()
}

// GetAvailableModels returns the list of available models
func (m *AIManager) GetAvailableModels() ([]string, error) {
	if !m.IsAvailable() {
		return nil, fmt.Errorf("AI service is not available")
	}
	return m.ollama.ListModels()
}

// AnalyzeFile reads and analyzes a file
func (m *AIManager) AnalyzeFile(filePath string) (string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// Limit content size for analysis
	contentStr := string(content)
	if len(contentStr) > 10000 {
		contentStr = contentStr[:10000] + "\n... (truncated)"
	}

	return m.AnalyzeCode(filePath, contentStr)
}