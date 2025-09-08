package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"workspace/internal/agents"
	"workspace/internal/agents/providers"
	"workspace/internal/agents/tools"
	"workspace/models"
	"workspace/services"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

// AIController handles AI chat conversations
type AIController struct {
	application.BaseController
	toolRegistry *agents.ToolRegistry
	provider     agents.Provider
}

// AIMetrics tracks performance metrics for AI responses
type AIMetrics struct {
	StartTime        time.Time
	ThinkingDuration time.Duration
	ToolDuration     time.Duration
	TotalDuration    time.Duration
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	ToolCallCount    int
	ModelUsed        string
	Error            string
}

// AI returns the controller factory
func AI() (string, *AIController) {
	// Initialize tool registry
	registry := agents.NewToolRegistry()

	// Note: Tools will be registered in Setup based on provider capabilities

	return "ai", &AIController{
		toolRegistry: registry,
	}
}

// Setup initializes the AI controller
func (c *AIController) Setup(app *application.App) {
	c.App = app
	auth := app.Use("auth").(*AuthController)

	// Initialize provider based on AI_MODEL environment variable
	provider, err := providers.NewProvider()
	if err != nil {
		log.Printf("AIController: Failed to initialize provider: %v", err)
		// Continue without provider - AI features will be disabled
	} else {
		c.provider = provider
		log.Printf("AIController: Initialized %s with model %s", provider.Name(), provider.Model())

		// Register only the tools supported by this provider
		c.registerSupportedTools(provider)
	}

	// Conversation routes - Admin only
	http.Handle("GET /ai/panel", app.ProtectFunc(c.panel, auth.AdminOnly))
	http.Handle("POST /ai/conversations", app.ProtectFunc(c.createConversation, auth.AdminOnly))
	http.Handle("DELETE /ai/conversations/{id}", app.ProtectFunc(c.deleteConversation, auth.AdminOnly))

	// Chat routes - Admin only
	http.Handle("GET /ai/chat/{id}", app.ProtectFunc(c.loadChat, auth.AdminOnly))
	http.Handle("GET /ai/chat/{id}/messages", app.ProtectFunc(c.getMessages, auth.AdminOnly))
	http.Handle("POST /ai/chat/{id}/send", app.ProtectFunc(c.sendMessage, auth.AdminOnly))
	http.Handle("GET /ai/chat/{id}/stream", app.ProtectFunc(c.streamResponse, auth.AdminOnly))

	// Todo routes - Admin only
	http.Handle("GET /ai/chat/{id}/todos/panel", app.ProtectFunc(c.getTodoPanel, auth.AdminOnly))
	http.Handle("GET /ai/chat/{id}/todos", app.ProtectFunc(c.getTodos, auth.AdminOnly))
	http.Handle("GET /ai/chat/{id}/todos/stream", app.ProtectFunc(c.streamTodos, auth.AdminOnly))

	// Control routes - Admin only
	http.Handle("POST /ai/chat/{id}/stop", app.ProtectFunc(c.stopExecution, auth.AdminOnly))

	// Initialize Ollama service in background
	go func() {
		log.Println("AIController: Initializing Ollama service...")
		if !services.Ollama.IsConfigured() {
			if err := services.Ollama.Init(); err != nil {
				log.Printf("AIController: Warning - Ollama service not available: %v", err)
			}
		}
	}()
}

// Handle prepares the controller for request handling
func (c AIController) Handle(req *http.Request) application.Controller {
	c.Request = req
	return &c
}

// registerSupportedTools registers only the tools supported by the provider
func (c *AIController) registerSupportedTools(provider agents.Provider) {
	// Map of all available tools
	allTools := map[string]agents.ToolImplementation{
		// Repository tools
		"list_repos":  &tools.ListReposTool{},
		"get_repo":    &tools.GetRepoTool{},
		"create_repo": &tools.CreateRepoTool{},
		// "delete_repo":  &tools.DeleteRepoTool{}, // Not implemented yet
		"get_repo_link": &tools.GetRepoLinkTool{},

		// File tools
		"list_files":   &tools.ListFilesTool{},
		"read_file":    &tools.ReadFileTool{},
		"write_file":   &tools.WriteFileTool{},
		"edit_file":    &tools.EditFileTool{},
		"delete_file":  &tools.DeleteFileTool{},
		"move_file":    &tools.MoveFileTool{},
		"search_files": &tools.SearchFilesTool{},

		// Git tools
		"git_status":  &tools.GitStatusTool{},
		"git_history": &tools.GitLogTool{}, // Use GitLogTool
		"git_diff":    &tools.GitDiffTool{},
		"git_commit":  &tools.GitCommitTool{},
		// "git_push":    &tools.GitPushTool{}, // Not implemented

		// Issue tools
		"create_issue": &tools.CreateIssueTool{},
		"get_issue":    &tools.ListIssuesTool{}, // Use ListIssuesTool

		// Project tools
		// "create_milestone":     &tools.CreateMilestoneTool{}, // Not implemented
		// "create_project_card":  &tools.CreateProjectCardTool{}, // Not implemented

		// Terminal tool
		"terminal_execute": &tools.RunCommandTool{},

		// Todo tools
		// "create_todo": &tools.TodoCreateTool{}, // Not implemented
		"list_todos":  &tools.TodoListTool{},
		"update_todo": &tools.TodoUpdateTool{},
	}

	// Register only the tools this provider supports
	supportedTools := provider.SupportedTools()
	registeredCount := 0

	for _, toolName := range supportedTools {
		if tool, exists := allTools[toolName]; exists {
			c.toolRegistry.Register(tool)
			registeredCount++
		} else {
			log.Printf("AIController: Warning - provider claims support for unknown tool: %s", toolName)
		}
	}

	log.Printf("AIController: Registered %d/%d tools for %s", registeredCount, len(supportedTools), provider.Name())
}

// Template helper methods

// IsAIEnabled returns whether AI features are enabled (checks environment variable)
func (c *AIController) IsAIEnabled() bool {
	return services.Ollama.IsRunning()
}

// GetConversations returns all conversations for the current user (admin only)
func (c *AIController) GetConversations() []*models.Conversation {
	if c.Request == nil {
		return nil
	}

	user, _, err := c.App.Use("auth").(*AuthController).Authenticate(c.Request)
	if err != nil || !user.IsAdmin {
		return nil
	}

	conversations, _ := models.Conversations.Search("WHERE UserID = ? ORDER BY UpdatedAt DESC", user.ID)
	return conversations
}

// IsOllamaReady checks if Ollama service is ready and user is admin
func (c *AIController) IsOllamaReady() bool {
	if c.Request == nil {
		return false
	}

	user, _, err := c.App.Use("auth").(*AuthController).Authenticate(c.Request)
	if err != nil || !user.IsAdmin {
		return false
	}

	return services.Ollama != nil && services.Ollama.IsRunning()
}

// Handler methods

// panel renders the main AI panel with conversation list (admin only)
func (c *AIController) panel(w http.ResponseWriter, r *http.Request) {
	user, _, err := c.App.Use("auth").(*AuthController).Authenticate(r)
	if err != nil || !user.IsAdmin {
		c.RenderErrorMsg(w, r, "Admin access required")
		return
	}

	// Get user's conversations
	conversations, err := models.Conversations.Search("WHERE UserID = ? ORDER BY UpdatedAt DESC LIMIT 50", user.ID)
	if err != nil {
		conversations = []*models.Conversation{}
	}

	// Check if this is a search request (even if query is empty)
	_, hasSearchParam := r.URL.Query()["q"]
	searchQuery := strings.TrimSpace(r.URL.Query().Get("q"))

	if hasSearchParam {
		// Filter conversations if search query is not empty
		if searchQuery != "" {
			filtered := []*models.Conversation{}
			lowerQuery := strings.ToLower(searchQuery)
			for _, conv := range conversations {
				if strings.Contains(strings.ToLower(conv.Title), lowerQuery) ||
					strings.Contains(strings.ToLower(conv.LastMessage), lowerQuery) {
					filtered = append(filtered, conv)
				}
			}
			conversations = filtered
		}

		// Return partial for search (even when query is empty)
		c.Render(w, r, "ai-conversations-list.html", conversations)
		return
	}

	c.Render(w, r, "ai-panel.html", conversations)
}

// createConversation creates a new conversation (admin only)
func (c *AIController) createConversation(w http.ResponseWriter, r *http.Request) {
	user, _, err := c.App.Use("auth").(*AuthController).Authenticate(r)
	if err != nil || !user.IsAdmin {
		c.RenderErrorMsg(w, r, "Admin access required")
		return
	}

	// Create new conversation
	conversation := &models.Conversation{
		UserID: user.ID,
		Title:  "New Conversation",
	}

	conversation, err = models.Conversations.Insert(conversation)
	if err != nil {
		log.Printf("AIController: Failed to create conversation: %v", err)
		c.RenderErrorMsg(w, r, "Failed to create conversation")
		return
	}

	// Load chat interface
	c.Render(w, r, "ai-chat.html", conversation)
}

// deleteConversation deletes a conversation and its messages (admin only)
func (c *AIController) deleteConversation(w http.ResponseWriter, r *http.Request) {
	conversationID := r.PathValue("id")
	user, _, err := c.App.Use("auth").(*AuthController).Authenticate(r)
	if err != nil || !user.IsAdmin {
		c.RenderErrorMsg(w, r, "Admin access required")
		return
	}

	// Get conversation and verify ownership
	conversation, err := models.Conversations.Get(conversationID)
	if err != nil || conversation.UserID != user.ID {
		c.RenderErrorMsg(w, r, "Conversation not found")
		return
	}

	// Delete all messages
	messages, _ := conversation.GetMessages()
	for _, msg := range messages {
		models.Messages.Delete(msg)
	}

	// Delete conversation
	if err := models.Conversations.Delete(conversation); err != nil {
		log.Printf("AIController: Failed to delete conversation: %v", err)
		c.RenderErrorMsg(w, r, "Failed to delete conversation")
		return
	}

	// Re-render panel
	c.panel(w, r)
}

// loadChat loads the chat interface for a conversation (admin only)
func (c *AIController) loadChat(w http.ResponseWriter, r *http.Request) {
	conversationID := r.PathValue("id")
	user, _, err := c.App.Use("auth").(*AuthController).Authenticate(r)
	if err != nil || !user.IsAdmin {
		c.RenderErrorMsg(w, r, "Admin access required")
		return
	}

	// Get conversation and verify ownership
	conversation, err := models.Conversations.Get(conversationID)
	if err != nil || conversation.UserID != user.ID {
		c.RenderErrorMsg(w, r, "Conversation not found")
		return
	}

	c.Render(w, r, "ai-chat.html", conversation)
}

// getMessages returns messages for a conversation (admin only)
func (c *AIController) getMessages(w http.ResponseWriter, r *http.Request) {
	conversationID := r.PathValue("id")
	user, _, err := c.App.Use("auth").(*AuthController).Authenticate(r)
	if err != nil || !user.IsAdmin {
		c.RenderErrorMsg(w, r, "Admin access required")
		return
	}

	// Verify ownership
	conversation, err := models.Conversations.Get(conversationID)
	if err != nil || conversation.UserID != user.ID {
		c.RenderErrorMsg(w, r, "Conversation not found")
		return
	}

	// Get messages
	messages, err := conversation.GetMessages()
	if err != nil {
		log.Printf("AIController: Failed to get messages: %v", err)
		messages = []*models.Message{}
	}

	c.Render(w, r, "ai-messages.html", messages)
}

// sendMessage sends a message to the AI and gets a response
func (c *AIController) sendMessage(w http.ResponseWriter, r *http.Request) {
	conversationID := r.PathValue("id")
	content := strings.TrimSpace(r.FormValue("message"))
	user, _, err := c.App.Use("auth").(*AuthController).Authenticate(r)
	if err != nil || !user.IsAdmin {
		c.RenderErrorMsg(w, r, "Admin access required")
		return
	}

	// Initialize metrics
	metrics := &AIMetrics{
		StartTime: time.Now(),
		ModelUsed: services.Ollama.GetDefaultModel(),
	}

	// Validate input
	if content == "" {
		c.RenderErrorMsg(w, r, "Message cannot be empty")
		return
	}

	// Verify ownership
	conversation, err := models.Conversations.Get(conversationID)
	if err != nil || conversation.UserID != user.ID {
		c.RenderErrorMsg(w, r, "Conversation not found")
		return
	}

	// Save user message
	userMsg := &models.Message{
		ConversationID: conversationID,
		Role:           models.MessageRoleUser,
		Content:        content,
	}
	userMsg, err = models.Messages.Insert(userMsg)
	if err != nil {
		log.Printf("AIController: Failed to save user message: %v", err)
	}

	// Update conversation title if it's the first message
	if conversation.Title == "New Conversation" {
		conversation.Title = content
		if len(conversation.Title) > 50 {
			conversation.Title = conversation.Title[:47] + "..."
		}
		models.Conversations.Update(conversation)
	}

	// Check if this is an HTMX request (should always be true for our UI)
	if r.Header.Get("HX-Request") == "true" {
		// Return the streaming template immediately
		// The template will connect to /stream endpoint which does the actual work
		messages, _ := conversation.GetMessages()
		c.Render(w, r, "ai-messages-enhanced.html", map[string]interface{}{
			"Messages":       messages,
			"ConversationID": conversationID,
		})
		return
	}

	// Non-streaming fallback (shouldn't normally happen)
	messages, _ := conversation.GetMessages()

	// Use proper system prompt for gpt-oss with tool support
	systemPrompt := c.buildSystemPrompt(conversationID)

	ollamaMessages := []services.OllamaMessage{
		{
			Role:    "system",
			Content: systemPrompt,
		},
	}

	for _, msg := range messages {
		if msg.Role == models.MessageRoleUser || msg.Role == models.MessageRoleAssistant {
			ollamaMessages = append(ollamaMessages, services.OllamaMessage{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}

	// Add todos to context if any exist
	todos, err := models.GetActiveTodos(conversationID)
	if err == nil && len(todos) > 0 {
		todoContext := models.FormatTodosForPrompt(todos)
		todoContext += "\n\nUse todo_update tool to manage these tasks as you work."
		ollamaMessages = append(ollamaMessages, services.OllamaMessage{
			Role:    "system",
			Content: todoContext,
		})
	}

	// Check if provider is ready
	if c.provider == nil {
		log.Printf("AIController: AI provider not initialized")

		// Save error message with helpful information
		errorMsg := &models.Message{
			ConversationID: conversationID,
			Role:           models.MessageRoleError,
			Content:        "AI service is initializing. This may take a few minutes while the model is being loaded. Please try again shortly.",
		}
		models.Messages.Insert(errorMsg)

		// Render messages with error
		messages, _ = conversation.GetMessages()
		c.Render(w, r, "ai-messages.html", messages)
		return
	}

	// Convert messages to agent format
	agentMessages := agents.ConvertOllamaToAgentMessages(ollamaMessages)

	// Get tools in agent format
	tools := agents.ConvertRegistryToAgentTools(c.toolRegistry, c.provider.SupportedTools())

	// Use provider to send request with tools
	thinkingStart := time.Now()
	log.Printf("AIController: Sending request to %s with %d tools available", c.provider.Model(), len(tools))
	response, err := c.provider.ChatWithTools(agentMessages, tools, agents.ChatOptions{})
	metrics.ThinkingDuration = time.Since(thinkingStart)
	log.Printf("AIController: Initial response received in %.2fs", metrics.ThinkingDuration.Seconds())

	if err != nil {
		log.Printf("AIController: Failed to get AI response: %v", err)

		// Determine error message based on error type
		errorMessage := "Unable to get AI response. Please try again."
		if strings.Contains(err.Error(), "model not found") {
			errorMessage = "AI model is being downloaded. This may take several minutes on first use. Please try again shortly."
		} else if strings.Contains(err.Error(), "connection refused") {
			errorMessage = "AI service is not responding. Please contact support if this persists."
		} else if strings.Contains(err.Error(), "timeout") {
			errorMessage = "AI service timed out. The model may be loading. Please try again in a moment."
		}

		// Save error message
		errorMsg := &models.Message{
			ConversationID: conversationID,
			Role:           models.MessageRoleError,
			Content:        errorMessage,
		}
		models.Messages.Insert(errorMsg)

		// Render messages with error
		messages, _ = conversation.GetMessages()
		c.Render(w, r, "ai-messages.html", messages)
		return
	}

	// Process tool calls and implement agentic loop
	finalResponse := response.Content

	// Check if model decided to use tools
	if len(response.ToolCalls) == 0 {
		// Native tool calling only - no text parsing needed
		log.Printf("AIController: Model chose not to use tools for this query")
		// Save and return the response immediately
		assistantMsg := &models.Message{
			ConversationID: conversationID,
			Role:           models.MessageRoleAssistant,
			Content:        finalResponse,
		}
		models.Messages.Insert(assistantMsg)
		conversation.UpdateLastMessage(finalResponse, models.MessageRoleAssistant)

		metrics.TotalDuration = time.Since(metrics.StartTime)
		log.Printf("AIController: Direct response completed - Total: %.2fs, Thinking: %.2fs",
			metrics.TotalDuration.Seconds(), metrics.ThinkingDuration.Seconds())

		messages, _ = conversation.GetMessages()
		c.Render(w, r, "ai-messages.html", messages)
		return
	}

	maxIterations := 5 // Prevent infinite loops
	iteration := 0

	log.Printf("AIController: Model decided to use tools, entering agentic loop")

	for iteration < maxIterations {
		var toolResults []string
		toolStart := time.Now()

		if len(response.ToolCalls) > 0 {
			// Log tool usage
			toolNames := []string{}
			for _, tc := range response.ToolCalls {
				toolNames = append(toolNames, tc.Function.Name)
			}
			log.Printf("AIController: Executing %d tools (iteration %d): %v", len(response.ToolCalls), iteration+1, toolNames)

			// Process native tool calls (without streaming in sendMessage)
			toolResults = c.processNativeAgentToolCalls(response.ToolCalls, conversationID, user.ID, nil, nil)

			toolDuration := time.Since(toolStart)
			metrics.ToolDuration += toolDuration
			metrics.ToolCallCount += len(response.ToolCalls)
			log.Printf("AIController: Tools executed in %.2fs", toolDuration.Seconds())
		} else {
			// No more tool calls, exit loop
			break
		}

		if len(toolResults) == 0 {
			// No tool calls found
			break
		}

		// Save tool execution messages
		// First add the assistant message that triggered the tools
		if iteration == 0 || finalResponse != "" {
			ollamaMessages = append(ollamaMessages, services.OllamaMessage{
				Role:    "assistant",
				Content: finalResponse, // Include the assistant's message/tool call
			})
		}

		// Then add each tool result
		for i, result := range toolResults {
			toolMsg := &models.Message{
				ConversationID: conversationID,
				Role:           models.MessageRoleTool,
				Content:        result,
			}
			models.Messages.Insert(toolMsg)

			// Add tool result to conversation context
			ollamaMessages = append(ollamaMessages, services.OllamaMessage{
				Role:    "tool", // Use "tool" role for tool results
				Content: result,
			})

			log.Printf("AIController: Added tool result %d/%d to context", i+1, len(toolResults))
		}

		// Get new response with tool results
		followUpStart := time.Now()
		log.Printf("AIController: Getting follow-up response after tool execution (iteration %d)", iteration+1)
		agentMessages = agents.ConvertOllamaToAgentMessages(ollamaMessages)
		tools = agents.ConvertRegistryToAgentTools(c.toolRegistry, c.provider.SupportedTools())
		response, err = c.provider.ChatWithTools(agentMessages, tools, agents.ChatOptions{})
		metrics.ThinkingDuration += time.Since(followUpStart)
		if err != nil {
			log.Printf("AIController: Failed to get follow-up response: %v", err)
			// If we can't get a follow-up, save what we have with the tool results
			finalResponse = finalResponse + "\n\n" + strings.Join(toolResults, "\n")
			break
		}

		// Update response for next iteration check
		finalResponse = response.Content
		iteration++

		// The loop will continue if the new response contains tool calls
		log.Printf("AIController: Iteration %d complete, checking for more tool calls", iteration)
	}

	// Save the final assistant response
	assistantMsg := &models.Message{
		ConversationID: conversationID,
		Role:           models.MessageRoleAssistant,
		Content:        finalResponse,
	}
	assistantMsg, err = models.Messages.Insert(assistantMsg)
	if err != nil {
		log.Printf("AIController: Failed to save assistant message: %v", err)
	}

	// Update conversation's last message
	conversation.UpdateLastMessage(finalResponse, models.MessageRoleAssistant)

	// Calculate final metrics
	metrics.TotalDuration = time.Since(metrics.StartTime)

	// Log comprehensive metrics
	if metrics.ToolCallCount > 0 {
		log.Printf("AIController: Response complete - Total: %.2fs, Thinking: %.2fs, Tools: %d calls in %.2fs",
			metrics.TotalDuration.Seconds(),
			metrics.ThinkingDuration.Seconds(),
			metrics.ToolCallCount,
			metrics.ToolDuration.Seconds())
	} else {
		log.Printf("AIController: Response complete - Total: %.2fs, Thinking: %.2fs",
			metrics.TotalDuration.Seconds(),
			metrics.ThinkingDuration.Seconds())
	}

	// Get all messages and render
	messages, _ = conversation.GetMessages()
	c.Render(w, r, "ai-messages.html", messages)
}

// buildSystemPrompt creates the system prompt optimized for gpt-oss
func (c *AIController) buildSystemPrompt(conversationID string) string {
	// Prompt optimized for native tool calling with gpt-oss
	prompt := `You are an AI coding assistant in the Skyscape development platform, similar to Claude Code but integrated into a web interface.

**YOUR PERSONALITY & APPROACH:**
- Be proactive and intelligent in your exploration
- Provide insights and observations, not just raw data
- Think like a developer exploring a codebase
- Make connections between what you discover
- Suggest interesting areas to explore based on what you find

**CHAIN OF THOUGHT PROCESS:**
Share your thinking process as quick, natural thoughts:
- When understanding: "Let me see what you're asking for..." or "This looks like an exploration task..."
- Before using tools: "I should start by listing repositories..." or "Now I need to check the file structure..."
- Analyzing results: "Interesting, I found X..." or "This appears to be a Y project..."
- Planning next steps: "I should dive deeper into..." or "Let me examine the main files..."

IMPORTANT PATH RULES:
- Always use relative paths: "." for root, "controllers/main.go" for files
- Never use absolute paths like "/src/main" or "/root/..."
- Repository files are relative to the repo root

**TOOL USAGE - CRITICAL RULE:**
⚠️ **USE ONLY ONE TOOL AT A TIME** ⚠️
NEVER call multiple tools in a single response. This is MANDATORY.

The correct pattern is:
1. Call ONE tool
2. Wait for results
3. Analyze what you found
4. THEN decide on the next tool

❌ WRONG: Calling list_repos, get_repo, and read_file together
✅ RIGHT: Call list_repos → analyze → call get_repo → analyze → call read_file

**AFTER EVERY SINGLE TOOL:**
You MUST provide a response that:
1. Explains what you discovered
2. Highlights interesting findings
3. Decides what to explore next
4. Then calls the NEXT tool (if needed)

**GOOD vs BAD RESPONSES after tool execution:**

❌ BAD: "I've successfully executed the requested action. The results are shown above. What would you like to do next?"
❌ BAD: "I found 38 repositories."
❌ BAD: "The tool completed successfully."

✅ GOOD: "I found the SkyCastle repository! It appears to be a private repository. Looking at the name, it might be a game or fantasy-themed project. Would you like me to explore its file structure to understand what it does?"
✅ GOOD: "Interesting! I can see 38 repositories here, with a mix of public and private ones. I notice 'sky-castle' at the top - is this the main project? There are also several test repositories and what looks like interview projects. Let me explore SkyCastle's structure to understand the codebase better."
✅ GOOD: "The repository structure shows this is a Go application with models, controllers, and views - looks like an MVC architecture. I can see auth and database packages. Should I examine the main.go file to understand the application's entry point?"

**AVAILABLE TOOLS:**
1. todo_update - Track multi-step tasks (actions: add, update, remove, clear)
2. list_repos - Discover all repositories (use visibility:"all" to see both public and private repos)
3. get_repo - Get repository details (use repo_id like "sky-castle")
4. list_files - Explore directory structure (use path="." for root, or "controllers" for subdir - NO leading slashes)
5. read_file - Examine code (use path like "README.md" or "controllers/main.go" - relative paths only)
6. run_command - Execute git, edit files, run tests

**EXPLORATION PATTERNS - STEP BY STEP:**
When exploring, take it ONE STEP at a time:

Step 1: list_repos → "I found X repos, let me look at SkyCastle..."
Step 2: get_repo(repo_id="sky-castle") → "It's a private Go project, let me see the structure..."
Step 3: list_files(repo_id="sky-castle", path=".") → "I see MVC pattern with controllers and models..."
Step 4: read_file(repo_id="sky-castle", path="README.md") → "This explains the project purpose..."

REMEMBER: ONE tool, analyze results, THEN next tool.
Never jump ahead - explore methodically and share insights at each step.

**IMPORTANT BEHAVIORS:**
- If a tool fails, explain the error and try an alternative approach
- When listing many items, highlight the interesting ones
- After reading code, explain what it does and why it's significant
- Connect findings to build understanding of the whole system
- Be conversational and engaging, not robotic`

	// Check for project context file (SKYSCAPE.md) and append if exists
	contextFile := c.loadProjectContext()
	if contextFile != "" {
		prompt += "\n\n## Project Context\n" + contextFile
	}

	return prompt
}

// loadProjectContext loads the SKYSCAPE.md file if it exists
func (c *AIController) loadProjectContext() string {
	// Try to find SKYSCAPE.md in common locations
	possiblePaths := []string{
		"/home/coder/skyscape/workspace/SKYSCAPE.md",
		"/home/coder/skyscape/SKYSCAPE.md",
		"./SKYSCAPE.md",
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			content, err := os.ReadFile(path)
			if err == nil {
				return string(content)
			}
		}
	}

	return ""
}

// RenderMessageMarkdown converts message content to HTML with markdown formatting
func (c *AIController) RenderMessageMarkdown(content string) template.HTML {
	// Create goldmark markdown processor with GitHub Flavored Markdown
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,      // GitHub Flavored Markdown
			extension.Linkify,  // Auto-linkify URLs
			extension.TaskList, // Task list support
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithXHTML(),
		),
	)

	// Convert markdown to HTML
	var buf bytes.Buffer
	if err := md.Convert([]byte(content), &buf); err != nil {
		// If conversion fails, return escaped content
		return template.HTML(template.HTMLEscapeString(content))
	}

	htmlStr := buf.String()

	// Add Tailwind/DaisyUI classes for styling
	// Code blocks with better chat-appropriate styling
	htmlStr = strings.ReplaceAll(htmlStr, "<pre>", `<pre class="bg-base-200 p-3 rounded-lg overflow-x-auto my-2 text-xs">`)
	htmlStr = strings.ReplaceAll(htmlStr, "<code>", `<code class="bg-base-200 px-1 py-0.5 rounded text-xs">`)

	// Tables
	htmlStr = strings.ReplaceAll(htmlStr, "<table>", `<table class="table table-xs table-zebra my-2">`)

	// Headings (smaller for chat context)
	htmlStr = strings.ReplaceAll(htmlStr, "<h1", `<h1 class="text-lg font-bold mt-3 mb-2"`)
	htmlStr = strings.ReplaceAll(htmlStr, "<h2", `<h2 class="text-base font-semibold mt-2 mb-1"`)
	htmlStr = strings.ReplaceAll(htmlStr, "<h3", `<h3 class="text-sm font-semibold mt-2 mb-1"`)

	// Lists
	htmlStr = strings.ReplaceAll(htmlStr, "<ul>", `<ul class="list-disc pl-4 my-1 space-y-0.5 text-sm">`)
	htmlStr = strings.ReplaceAll(htmlStr, "<ol>", `<ol class="list-decimal pl-4 my-1 space-y-0.5 text-sm">`)

	// Blockquotes
	htmlStr = strings.ReplaceAll(htmlStr, "<blockquote>", `<blockquote class="border-l-2 border-info pl-3 my-2 text-sm italic">`)

	// Paragraphs
	htmlStr = strings.ReplaceAll(htmlStr, "<p>", `<p class="my-1">`)

	// Links
	htmlStr = strings.ReplaceAll(htmlStr, "<a href=", `<a class="link link-info text-sm" href=`)

	// Strong and emphasis
	htmlStr = strings.ReplaceAll(htmlStr, "<strong>", `<strong class="font-semibold">`)
	htmlStr = strings.ReplaceAll(htmlStr, "<em>", `<em class="italic">`)

	return template.HTML(htmlStr)
}

// categorizeTools returns relevant tools based on the user's message and conversation state
func (c *AIController) categorizeTools(message string, isFirstMessage bool, lastToolUsed string) []string {
	messageLower := strings.ToLower(message)

	// Quick responses that don't need tools
	greetings := []string{"hello", "hi", "hey", "good morning", "good afternoon", "good evening", "how are you", "thanks", "thank you"}
	for _, greeting := range greetings {
		if strings.Contains(messageLower, greeting) && len(messageLower) < 30 {
			return []string{} // No tools needed
		}
	}

	// PROGRESSIVE TOOL AVAILABILITY - Start with minimal tools to prevent batching
	if isFirstMessage {
		// For exploration requests, start with ONLY list_repos
		if strings.Contains(messageLower, "explore") ||
			strings.Contains(messageLower, "repo") ||
			strings.Contains(messageLower, "skycastle") ||
			strings.Contains(messageLower, "sky-castle") {
			// Start with just list_repos - this forces step-by-step exploration
			log.Printf("AIController: First exploration message - providing only list_repos tool")
			return []string{"list_repos"}
		}
		// For other queries, also start minimal
		if strings.Contains(messageLower, "show") ||
			strings.Contains(messageLower, "what") ||
			strings.Contains(messageLower, "list") {
			return []string{"list_repos"}
		}
		// For task planning
		if strings.Contains(messageLower, "help") || strings.Contains(messageLower, "create") {
			return []string{"todo_update"}
		}
		// Default first message - just list repos
		return []string{"list_repos"}
	}

	// After tool execution, provide next logical tools based on what was just done
	switch lastToolUsed {
	case "list_repos":
		// After listing repos, allow getting details
		log.Printf("AIController: After list_repos - providing get_repo tool")
		return []string{"get_repo"}

	case "get_repo":
		// After getting repo details, allow exploring files
		log.Printf("AIController: After get_repo - providing list_files and read_file")
		return []string{"list_files", "read_file"}

	case "list_files":
		// After listing files, allow reading them or listing more directories
		log.Printf("AIController: After list_files - providing read_file and list_files")
		return []string{"read_file", "list_files"}

	case "read_file":
		// After reading a file, allow reading more or exploring directories
		log.Printf("AIController: After read_file - providing read_file and list_files")
		return []string{"read_file", "list_files"}

	case "todo_update":
		// After updating todos, provide more general tools
		log.Printf("AIController: After todo_update - providing general tools")
		return []string{"list_repos", "todo_update"}

	default:
		// If we don't know the last tool, provide a safe default set
		log.Printf("AIController: Unknown last tool '%s' - providing default exploration tools", lastToolUsed)
		return []string{"get_repo", "list_files", "read_file"}
	}
}

// streamResponse handles SSE streaming of AI responses
func (c *AIController) streamResponse(w http.ResponseWriter, r *http.Request) {
	conversationID := r.PathValue("id")
	user, _, err := c.App.Use("auth").(*AuthController).Authenticate(r)
	if err != nil || !user.IsAdmin {
		http.Error(w, "Admin access required", http.StatusForbidden)
		return
	}

	// Initialize metrics for tracking
	metrics := &AIMetrics{
		StartTime: time.Now(),
		ModelUsed: services.Ollama.GetDefaultModel(),
	}

	// Verify ownership
	conversation, err := models.Conversations.Get(conversationID)
	if err != nil || conversation.UserID != user.ID {
		http.Error(w, "Conversation not found", http.StatusNotFound)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable Nginx buffering

	// Create flusher for real-time updates
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Get the latest pending message (should be created by sendMessage)
	messages, _ := conversation.GetMessages()
	if len(messages) == 0 {
		fmt.Fprintf(w, "event: error\ndata: No messages in conversation\n\n")
		flusher.Flush()
		return
	}

	// Get working context and settings
	workingContext := conversation.GetWorkingContext()
	settings := conversation.GetSettings()

	// Enhance user message with context if needed
	if len(messages) > 0 {
		lastMsg := messages[len(messages)-1]
		if lastMsg.Role == models.MessageRoleUser {
			enhancedContent := c.resolveContextualReferences(lastMsg.Content, workingContext)
			if enhancedContent != lastMsg.Content {
				// Don't save the enhanced content, just use it for context
				messages[len(messages)-1].Content = enhancedContent
			}
		}
	}

	// Build optimized context window
	ollamaMessages := c.buildContextWindow(conversation, 30)

	// Add todos to context if any exist
	todos, err := models.GetActiveTodos(conversationID)
	if err == nil && len(todos) > 0 {
		todoContext := models.FormatTodosForPrompt(todos)
		todoContext += "\n\nUse todo_update tool to manage these tasks as you work."
		ollamaMessages = append(ollamaMessages, services.OllamaMessage{
			Role:    "system",
			Content: todoContext,
		})
	}

	// Check if Ollama service is ready
	if !services.Ollama.IsRunning() {
		log.Printf("AIController: Ollama service is not running")

		// Save error as message in conversation
		errorMsg := &models.Message{
			ConversationID: conversationID,
			Role:           models.MessageRoleError,
			Content:        "AI service is initializing. Please wait a moment and try again.",
		}
		models.Messages.Insert(errorMsg)

		// Stream error message
		errorHTML := `<div class="alert alert-warning my-2">
			<svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5 shrink-0 stroke-current" fill="none" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
			</svg>
			<span class="text-sm">AI service is initializing. Please wait a moment and try again.</span>
		</div>`
		errorHTMLEscaped := strings.ReplaceAll(errorHTML, "\n", "")
		errorHTMLEscaped = strings.ReplaceAll(errorHTMLEscaped, "\t", "")
		fmt.Fprintf(w, "event: complete\ndata: %s\n\n", errorHTMLEscaped)
		fmt.Fprintf(w, "event: done\ndata: \n\n")
		flusher.Flush()
		return
	}

	// Get the last user message for tool categorization
	var lastUserMessage string
	userMessageCount := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == models.MessageRoleUser {
			if lastUserMessage == "" {
				lastUserMessage = messages[i].Content
			}
			userMessageCount++
		}
	}

	// Check if this is the first user message in the conversation
	isFirstMessage := userMessageCount <= 1

	// Categorize and filter tools based on the user's message and conversation state
	relevantToolNames := c.categorizeTools(lastUserMessage, isFirstMessage, "")
	log.Printf("AIController: Last user message: %s", lastUserMessage)
	log.Printf("AIController: First message: %v, User messages: %d", isFirstMessage, userMessageCount)
	log.Printf("AIController: Selected %d relevant tools based on query: %v", len(relevantToolNames), relevantToolNames)

	// Send initial thinking status based on query type
	initialStatus := "🤔 Understanding your request..."
	if len(relevantToolNames) == 0 {
		initialStatus = "💬 Preparing response..."
	} else if strings.Contains(strings.ToLower(lastUserMessage), "explore") {
		initialStatus = "🔍 Planning exploration strategy..."
	} else if strings.Contains(strings.ToLower(lastUserMessage), "create") || strings.Contains(strings.ToLower(lastUserMessage), "write") {
		initialStatus = "✏️ Planning implementation..."
	} else {
		initialStatus = "🤔 Analyzing request and selecting approach..."
	}
	fmt.Fprintf(w, "event: status\ndata: <span class='loading loading-spinner loading-xs'></span> %s\n\n", initialStatus)
	flusher.Flush()

	// Send initial thinking thoughts
	messageLower := strings.ToLower(lastUserMessage)
	if strings.Contains(messageLower, "explore") {
		c.streamThought(w, flusher, "I see you want to explore something. Let me gather information...")
		if strings.Contains(messageLower, "skycastle") || strings.Contains(messageLower, "sky-castle") {
			c.streamThought(w, flusher, "Looking for the SkyCastle repository specifically...")
		}
	} else if strings.Contains(messageLower, "what") || strings.Contains(messageLower, "show") {
		c.streamThought(w, flusher, "Let me check what information I can find...")
	} else if strings.Contains(messageLower, "create") || strings.Contains(messageLower, "write") {
		c.streamThought(w, flusher, "I'll need to understand the requirements and plan the implementation...")
	}

	// Track thinking time
	thinkingStart := time.Now()

	// Convert messages to agent format
	agentMessages := agents.ConvertOllamaToAgentMessages(ollamaMessages)
	var initialResponse *agents.Response

	log.Printf("AIController: Streaming response with %s", c.provider.Model())

	// Get tools in agent format - provider will filter to supported ones
	tools := agents.ConvertRegistryToAgentTools(c.toolRegistry, c.provider.SupportedTools())

	// Log tool names for debugging
	toolNames := []string{}
	for _, t := range tools {
		toolNames = append(toolNames, t.Function.Name)
	}
	log.Printf("AIController: Providing %d tools to model: %v", len(tools), toolNames)

	initialResponse, err = c.provider.ChatWithTools(agentMessages, tools, agents.ChatOptions{})

	metrics.ThinkingDuration = time.Since(thinkingStart)
	log.Printf("AIController: Initial response received in %.2fs", metrics.ThinkingDuration.Seconds())

	if err != nil {
		log.Printf("AIController: Failed to get initial AI response: %v", err)

		// Save error as message in conversation
		errorMsg := &models.Message{
			ConversationID: conversationID,
			Role:           models.MessageRoleError,
			Content:        fmt.Sprintf("Failed to get AI response: %v", err),
		}
		models.Messages.Insert(errorMsg)

		// Stream error message with proper formatting
		errorHTML := fmt.Sprintf(`<div class="alert alert-error my-2">
			<svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5 shrink-0 stroke-current" fill="none" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2m7-2a9 9 0 11-18 0 9 9 0 0118 0z" />
			</svg>
			<span class="text-sm">Failed to get AI response. Please try again.</span>
		</div>`)
		errorHTMLEscaped := strings.ReplaceAll(errorHTML, "\n", "")
		errorHTMLEscaped = strings.ReplaceAll(errorHTMLEscaped, "\t", "")
		fmt.Fprintf(w, "event: complete\ndata: %s\n\n", errorHTMLEscaped)
		fmt.Fprintf(w, "event: done\ndata: \n\n")
		flusher.Flush()
		return
	}

	// Autonomous execution loop
	finalResponse := initialResponse.Content
	iteration := 0
	maxIterations := 10 // Allow more iterations for complex tasks
	taskComplete := false

	// Check settings for max iterations
	if maxIter, ok := settings["maxIterations"].(float64); ok {
		maxIterations = int(maxIter)
	}

	// Check if there are tool calls to process
	if len(initialResponse.ToolCalls) == 0 {
		// Native tool calling only - no text parsing needed
		log.Printf("AIController: No tool calls detected, proceeding with direct response")
		// Skip directly to streaming the response
		goto streamResponse
	}

	log.Printf("AIController: Entering autonomous execution mode")

	// Send initial thinking message
	c.streamThought(w, flusher, "Analyzing the task and planning approach...")

	for !taskComplete && iteration < maxIterations {
		// Check for cancellation
		select {
		case <-r.Context().Done():
			log.Printf("AIController: Execution cancelled by user")
			fmt.Fprintf(w, "event: status\ndata: ❌ Execution cancelled\n\n")
			flusher.Flush()
			return
		default:
		}
		var toolResults []string
		toolStart := time.Now()

		// Native tool calls from Ollama/Llama 3.2
		if len(initialResponse.ToolCalls) > 0 {
			// Send status about tool usage
			toolNames := []string{}
			for _, tc := range initialResponse.ToolCalls {
				toolNames = append(toolNames, tc.Function.Name)
			}

			// ENFORCE SINGLE TOOL EXECUTION
			if len(initialResponse.ToolCalls) > 1 {
				log.Printf("AIController: WARNING - Model attempted to call %d tools at once: %v. Taking only the first one.", len(toolNames), toolNames)
				c.streamThought(w, flusher, fmt.Sprintf("I was about to use multiple tools, but I should focus on one at a time. Starting with %s...", toolNames[0]))
				// Take only the first tool call
				initialResponse.ToolCalls = initialResponse.ToolCalls[:1]
				toolNames = toolNames[:1]
			}

			// Provide initial status indicating tools will be used
			statusMsg := "🤖 Preparing to use tool..."
			fmt.Fprintf(w, "event: status\ndata: %s\n\n", statusMsg)
			flusher.Flush()

			// Process native tool calls with streaming
			log.Printf("AIController: Processing %d tool call (iteration %d): %v", len(initialResponse.ToolCalls), iteration+1, toolNames)
			toolResults = c.processNativeAgentToolCalls(initialResponse.ToolCalls, conversationID, user.ID, w, flusher)

			// Extract and update working context from tool calls
			for i, tc := range initialResponse.ToolCalls {
				var params map[string]interface{}
				json.Unmarshal(tc.Function.Arguments, &params)
				if i < len(toolResults) {
					contextUpdate := c.extractContextFromToolCall(tc.Function.Name, params, toolResults[i])
					c.updateWorkingContext(conversationID, contextUpdate)
				}
			}
		} else {
			// No more tool calls, exit loop
			break
		}

		toolDuration := time.Since(toolStart)
		metrics.ToolDuration += toolDuration
		metrics.ToolCallCount += len(toolResults)
		log.Printf("AIController: Tools executed in %.2fs", toolDuration.Seconds())

		if len(toolResults) == 0 {
			// No tool calls found
			break
		}

		// Save tool results
		// First add the assistant message that triggered the tools
		if iteration == 0 || finalResponse != "" {
			ollamaMessages = append(ollamaMessages, services.OllamaMessage{
				Role:    "assistant",
				Content: finalResponse,
			})
		}

		// Save tool results to the database and add to conversation context
		for _, result := range toolResults {
			toolMsg := &models.Message{
				ConversationID: conversationID,
				Role:           models.MessageRoleTool,
				Content:        result,
			}
			models.Messages.Insert(toolMsg)

			// Add tool result to conversation context
			ollamaMessages = append(ollamaMessages, services.OllamaMessage{
				Role:    "tool",
				Content: result,
			})
		}

		// Get follow-up response
		if iteration > 0 {
			c.streamThought(w, flusher, fmt.Sprintf("Iteration %d: Analyzing what to do next...", iteration+1))
		} else {
			c.streamThought(w, flusher, "Analyzing results and planning next step...")
		}
		fmt.Fprintf(w, "event: status\ndata: <span class='loading loading-spinner loading-xs'></span> Processing...\n\n")
		flusher.Flush()

		// Track the last tool used for context
		var lastToolUsed string
		if len(initialResponse.ToolCalls) > 0 {
			lastToolUsed = initialResponse.ToolCalls[len(initialResponse.ToolCalls)-1].Function.Name
		}

		// Add thinking about what we found
		if len(toolResults) > 0 && lastToolUsed != "" {
			switch lastToolUsed {
			case "list_repos":
				c.streamThought(w, flusher, "I can see the available repositories. Let me identify interesting ones...")
			case "get_repo":
				c.streamThought(w, flusher, "Now I have details about the repository. Let me analyze what this tells us...")
			case "list_files":
				c.streamThought(w, flusher, "The file structure reveals the project organization. Let me identify key files...")
			case "read_file":
				c.streamThought(w, flusher, "I've examined the code. Let me understand what it does...")
			default:
				c.streamThought(w, flusher, "Let me analyze what we discovered...")
			}
		}

		// Check if any tools failed and prompt appropriately
		hasFailure := false
		for _, result := range toolResults {
			if strings.Contains(result, "❌ Tool") || strings.Contains(result, "failed:") || strings.Contains(result, "error:") {
				hasFailure = true
				break
			}
		}

		// Update available tools based on what was just used
		if lastToolUsed != "" {
			// Re-categorize tools based on the last tool used
			relevantToolNames := c.categorizeTools("", false, lastToolUsed)
			log.Printf("AIController: Updating tool availability after %s - providing: %v", lastToolUsed, relevantToolNames)

			// Tools are dynamically filtered by the provider
		}

		if hasFailure {
			// Prompt to retry or work around the failure
			ollamaMessages = append(ollamaMessages, services.OllamaMessage{
				Role:    "system",
				Content: "One or more tools failed. Analyze the error and either: 1) Retry with corrected parameters, 2) Try an alternative tool/approach, or 3) Explain to the user why it failed and what they can do. Be proactive - don't just report the error.",
			})
		} else if len(toolResults) > 0 {
			// Create tool-specific follow-up prompts
			var contextPrompt string
			switch lastToolUsed {
			case "list_repos":
				contextPrompt = "You just listed repositories. Analyze what you found. If the user asked about a specific repo but it's not in the list, explain that you couldn't find it and suggest alternatives (maybe it's named differently or is private). If you did find relevant repos, use get_repo to explore them. Always provide a helpful response explaining what you found."
			case "get_repo":
				contextPrompt = "You got repository details. Explain what this tells you about the project. Now use list_files to explore the structure. Continue the exploration autonomously."
			case "list_files":
				contextPrompt = "You explored the file structure. Explain what kind of project this is based on the structure. Pick an important file to read next (like README.md or main.go) and use read_file to examine it."
			case "read_file":
				contextPrompt = "You read a file. Explain what this code/content does. Continue exploring other important files or directories to build a complete understanding."
			case "run_command":
				contextPrompt = "You executed a command. Explain what it accomplished. Continue with the next logical step in your exploration or task."
			default:
				contextPrompt = "Analyze the tool results above. Explain what you discovered and continue with the next tool to explore further. Remember you're exploring autonomously - keep going until you have a good understanding."
			}

			ollamaMessages = append(ollamaMessages, services.OllamaMessage{
				Role:    "system",
				Content: contextPrompt + " Remember: Never give generic responses like 'I successfully executed the action'. Always provide insights and be conversational.",
			})
		}

		// Get new response with tool results context
		agentMessages = agents.ConvertOllamaToAgentMessages(ollamaMessages)
		tools = agents.ConvertRegistryToAgentTools(c.toolRegistry, c.provider.SupportedTools())
		response, err := c.provider.ChatWithTools(agentMessages, tools, agents.ChatOptions{})
		if err != nil {
			finalResponse = finalResponse + "\n\n" + strings.Join(toolResults, "\n")
			break
		}

		// Log the response for debugging
		log.Printf("AIController: Follow-up response content length: %d", len(response.Content))
		if response.Content == "" && len(toolResults) > 0 {
			log.Printf("AIController: WARNING - Empty response after tool execution, requesting regeneration")
			// Don't force a generic response - instead ask the model to try again with stronger prompting
			retryMessages := append(ollamaMessages, services.OllamaMessage{
				Role:    "system",
				Content: "You must provide a response analyzing the tool results above. Look at what was discovered and provide insights. What's interesting about these results? What should we explore next? Be specific and helpful, not generic.",
			})

			retryAgentMessages := agents.ConvertOllamaToAgentMessages(retryMessages)
			retryResponse, retryErr := c.provider.ChatWithTools(retryAgentMessages, tools, agents.ChatOptions{})
			if retryErr == nil && retryResponse.Content != "" {
				response = retryResponse
				log.Printf("AIController: Regenerated response successfully")
			} else {
				// If still empty, provide a fallback response based on what was found
				log.Printf("AIController: Both attempts returned empty, generating fallback response")
				switch lastToolUsed {
				case "list_repos":
					// Parse the tool results to find the requested repo
					resultStr := strings.Join(toolResults, "\n")
					if strings.Contains(resultStr, "sky-castle") {
						finalResponse = "I found the sky-castle repository! It's listed as a private repository. Let me explore it further to provide you with a summary.\n\nWould you like me to continue exploring the sky-castle repository?"
					} else {
						finalResponse = "I found " + resultStr + "\n\nLet me know which repository you'd like me to explore in detail."
					}
				default:
					finalResponse = "Here's what I discovered:\n" + strings.Join(toolResults, "\n")
				}
				taskComplete = true // Stop the loop since we can't get proper responses
				break
			}
		}

		finalResponse = response.Content
		initialResponse = response // Update for next iteration

		// Check if task is complete based on response content and tool calls
		lowerResponse := strings.ToLower(finalResponse)
		noMoreTools := len(response.ToolCalls) == 0

		// Only mark complete if:
		// 1. Model explicitly says task is complete, OR
		// 2. No more tools AND we've done at least 2 iterations, OR
		// 3. User's original request appears satisfied (for simple queries)
		if strings.Contains(lowerResponse, "task complete") ||
			strings.Contains(lowerResponse, "all done") ||
			strings.Contains(lowerResponse, "finished successfully") ||
			strings.Contains(lowerResponse, "completed successfully") ||
			(noMoreTools && iteration >= 2) ||
			(noMoreTools && strings.Contains(lowerResponse, "would you like") && iteration > 0) {
			taskComplete = true
			log.Printf("AIController: Task marked as complete after %d iterations", iteration+1)
		} else if noMoreTools && iteration == 0 && strings.Contains(strings.ToLower(lastUserMessage), "explore") {
			// For exploration tasks, don't stop after just one iteration
			// Encourage the model to continue exploring
			log.Printf("AIController: Exploration task detected, encouraging continuation")
			ollamaMessages = append(ollamaMessages, services.OllamaMessage{
				Role:    "assistant",
				Content: finalResponse,
			})
			ollamaMessages = append(ollamaMessages, services.OllamaMessage{
				Role:    "system",
				Content: "The user asked you to explore. Continue exploring by using more tools to discover interesting aspects of the repository. Don't stop after just listing - dive deeper into the structure and code.",
			})

			// Request continuation with updated tools for exploration
			// Update tools for exploration continuation
			if lastToolUsed != "" {
				_ = c.categorizeTools("", false, lastToolUsed) // For future use
				// Tools are dynamically filtered by the provider
			}
			agentMessages = agents.ConvertOllamaToAgentMessages(ollamaMessages)
			response, err = c.provider.ChatWithTools(agentMessages, tools, agents.ChatOptions{})
			if err == nil && (len(response.ToolCalls) > 0 || response.Content != "") {
				initialResponse = response
				if response.Content != "" {
					finalResponse = finalResponse + "\n\n" + response.Content
				}
			} else {
				taskComplete = true
			}
		}

		iteration++
		log.Printf("AIController: Autonomous iteration %d complete", iteration)
	}

streamResponse:
	// Add final thinking before response
	if iteration > 0 {
		c.streamThought(w, flusher, fmt.Sprintf("Completed exploration after %d iterations. Preparing response...", iteration))
	} else {
		c.streamThought(w, flusher, "Preparing my response...")
	}

	// Clear status and prepare for response streaming
	fmt.Fprintf(w, "event: status\ndata: \n\n")
	flusher.Flush()

	log.Printf("AIController: Beginning response streaming, content length: %d", len(finalResponse))

	// Send initial message structure (replaces typing indicator)
	startHTML := `<div class="chat chat-start my-2" id="streaming-message">
		<div class="chat-image avatar">
			<div class="w-8 h-8 rounded-full flex-shrink-0">
				<div class="bg-base-300 text-base-content w-8 h-8 flex items-center justify-center rounded-full">
					<svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
					</svg>
				</div>
			</div>
		</div>
		<div class="chat-bubble max-w-[85%] sm:max-w-[70%] break-words text-sm">
			<span id="streaming-content"></span>
		</div>
	</div>`
	// SSE data must be on a single line - replace newlines
	startHTMLEscaped := strings.ReplaceAll(startHTML, "\n", "")
	startHTMLEscaped = strings.ReplaceAll(startHTMLEscaped, "\t", "")
	fmt.Fprintf(w, "event: start\ndata: %s\n\n", startHTMLEscaped)
	flusher.Flush()

	// Stream plain text chunks for immediate feedback
	chunkSize := 50 // Smaller chunks for smoother streaming
	for i := 0; i < len(finalResponse); i += chunkSize {
		end := i + chunkSize
		if end > len(finalResponse) {
			end = len(finalResponse)
		}

		// Send plain text chunk (will be escaped by browser)
		chunk := finalResponse[i:end]
		fmt.Fprintf(w, "event: chunk\ndata: %s\n\n", template.HTMLEscapeString(chunk))
		flusher.Flush()
		time.Sleep(20 * time.Millisecond) // Smooth streaming effect
	}

	// Calculate final metrics
	metrics.TotalDuration = time.Since(metrics.StartTime)

	// Build performance summary
	perfSummary := fmt.Sprintf("⚡ %.1fs total", metrics.TotalDuration.Seconds())
	if metrics.ToolCallCount > 0 {
		perfSummary = fmt.Sprintf("⚡ %.1fs total | 🤔 %.1fs thinking | 🔧 %d tools in %.1fs",
			metrics.TotalDuration.Seconds(),
			metrics.ThinkingDuration.Seconds(),
			metrics.ToolCallCount,
			metrics.ToolDuration.Seconds())
	}

	log.Printf("AIController: Response complete - %s", perfSummary)

	// Send the complete formatted message with metrics
	htmlContent := c.RenderMessageMarkdown(finalResponse)
	completeHTML := fmt.Sprintf(`<div class="chat chat-start my-2">
		<div class="chat-image avatar">
			<div class="w-8 h-8 rounded-full flex-shrink-0">
				<div class="bg-base-300 text-base-content w-8 h-8 flex items-center justify-center rounded-full">
					<svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
					</svg>
				</div>
			</div>
		</div>
		<div class="chat-bubble max-w-[85%%] sm:max-w-[70%%] break-words text-sm">
			%s
			<div class="text-xs text-base-content/60 mt-2">%s</div>
		</div>
	</div>`, htmlContent, perfSummary)
	// SSE data must be on a single line - replace newlines
	completeHTMLEscaped := strings.ReplaceAll(completeHTML, "\n", "")
	completeHTMLEscaped = strings.ReplaceAll(completeHTMLEscaped, "\t", "")
	fmt.Fprintf(w, "event: complete\ndata: %s\n\n", completeHTMLEscaped)
	flusher.Flush()

	// Signal completion
	fmt.Fprintf(w, "event: done\ndata: complete\n\n")
	flusher.Flush()

	// Save the final response to database
	if finalResponse != "" {
		assistantMsg := &models.Message{
			ConversationID: conversationID,
			Role:           models.MessageRoleAssistant,
			Content:        finalResponse,
		}

		models.Messages.Insert(assistantMsg)
		conversation.UpdateLastMessage(finalResponse, models.MessageRoleAssistant)
	}
}

// processNativeAgentToolCalls processes native tool calls from agent provider
func (c *AIController) processNativeAgentToolCalls(toolCalls []agents.ToolCall, conversationID, userID string, w http.ResponseWriter, flusher http.Flusher) []string {
	if c.toolRegistry == nil {
		log.Printf("AIController: ERROR - Tool registry is nil")
		return nil
	}

	var toolResults []string
	startTime := time.Now()
	streaming := w != nil && flusher != nil // Check if streaming is enabled

	log.Printf("AIController: Processing %d tool calls", len(toolCalls))

	for i, tc := range toolCalls {
		toolStart := time.Now()

		// Stream thought/planning message (only if streaming enabled)
		if streaming {
			// Add contextual thinking based on tool type
			switch tc.Function.Name {
			case "list_repos":
				c.streamThought(w, flusher, "I need to see what repositories are available...")
			case "get_repo":
				c.streamThought(w, flusher, "Let me get details about this repository...")
			case "list_files":
				c.streamThought(w, flusher, "I'll explore the file structure...")
			case "read_file":
				c.streamThought(w, flusher, "Let me examine this file...")
			case "write_file":
				c.streamThought(w, flusher, "I'm writing the file with the requested changes...")
			case "edit_file":
				c.streamThought(w, flusher, "I'm making the requested edits...")
			case "run_command":
				c.streamThought(w, flusher, "Running command...")
			case "git_status":
				c.streamThought(w, flusher, "Checking git status...")
			case "git_history":
				c.streamThought(w, flusher, "Looking at commit history...")
			case "git_diff":
				c.streamThought(w, flusher, "Examining changes...")
			case "git_commit":
				c.streamThought(w, flusher, "Creating commit...")
			case "todo_update":
				c.streamThought(w, flusher, "Updating task list...")
			}
		}

		// Parse arguments from JSON
		var params map[string]interface{}
		if err := json.Unmarshal(tc.Function.Arguments, &params); err != nil {
			log.Printf("AIController: Failed to parse tool arguments: %v", err)
			result := fmt.Sprintf("❌ Tool %s failed: Invalid arguments", tc.Function.Name)
			toolResults = append(toolResults, result)
			continue
		}

		// Get the tool instance
		tool, exists := c.toolRegistry.Get(tc.Function.Name)
		if !exists {
			log.Printf("AIController: Tool %s not found", tc.Function.Name)
			result := fmt.Sprintf("❌ Tool %s not found", tc.Function.Name)
			toolResults = append(toolResults, result)
			continue
		}

		// Validate parameters
		if err := tool.ValidateParams(params); err != nil {
			log.Printf("AIController: Invalid parameters for tool %s: %v", tc.Function.Name, err)
			result := fmt.Sprintf("❌ Tool %s: Invalid parameters - %v", tc.Function.Name, err)
			toolResults = append(toolResults, result)
			continue
		}

		// Execute the tool
		log.Printf("AIController: Executing tool %s [%d/%d] with params: %v", tc.Function.Name, i+1, len(toolCalls), params)

		// Update status (only if streaming)
		if streaming {
			statusMsg := fmt.Sprintf("🔧 Using %s...", tc.Function.Name)
			fmt.Fprintf(w, "event: status\ndata: %s\n\n", statusMsg)
			flusher.Flush()
		}

		result, err := tool.Execute(params, userID)

		toolDuration := time.Since(toolStart)
		if err != nil {
			log.Printf("AIController: Tool %s failed after %.2fs: %v", tc.Function.Name, toolDuration.Seconds(), err)
			result = fmt.Sprintf("❌ Tool %s failed: %v", tc.Function.Name, err)
		} else {
			log.Printf("AIController: Tool %s succeeded in %.2fs", tc.Function.Name, toolDuration.Seconds())

			// Compress output if too verbose
			result = c.compressToolOutput(tc.Function.Name, result)
		}

		toolResults = append(toolResults, result)
	}

	totalDuration := time.Since(startTime)
	log.Printf("AIController: All %d tools executed in %.2fs", len(toolCalls), totalDuration.Seconds())

	return toolResults
}

// processNativeToolCalls processes tool calls from Ollama's native response format
func (c *AIController) processNativeToolCalls(toolCalls []services.OllamaToolCall, conversationID, userID string, w http.ResponseWriter, flusher http.Flusher) []string {
	if c.toolRegistry == nil {
		log.Printf("AIController: ERROR - Tool registry is nil")
		return nil
	}

	var toolResults []string
	startTime := time.Now()
	streaming := w != nil && flusher != nil // Check if streaming is enabled

	log.Printf("AIController: Processing %d tool calls", len(toolCalls))

	for i, tc := range toolCalls {
		toolStart := time.Now()

		// Stream thought/planning message (only if streaming enabled)
		if streaming {
			// Add contextual thinking based on tool type
			switch tc.Function.Name {
			case "list_repos":
				c.streamThought(w, flusher, "I need to see what repositories are available...")
			case "get_repo":
				c.streamThought(w, flusher, "Let me get more details about this repository...")
			case "list_files":
				c.streamThought(w, flusher, "I should explore the file structure to understand the project layout...")
			case "read_file":
				c.streamThought(w, flusher, "Let me examine this file to understand the code...")
			case "run_command":
				c.streamThought(w, flusher, "I'll execute a command to perform this action...")
			case "todo_update":
				c.streamThought(w, flusher, "Let me update the task list...")
			default:
				c.streamThought(w, flusher, fmt.Sprintf("I'll use %s to help with this...", tc.Function.Name))
			}

			// Also send planning thought
			if len(toolCalls) > 1 {
				c.streamThought(w, flusher, fmt.Sprintf("Planning tool %d of %d: %s", i+1, len(toolCalls), tc.Function.Name))
			} else {
				c.streamThought(w, flusher, fmt.Sprintf("Planning to use: %s", tc.Function.Name))
			}
			time.Sleep(100 * time.Millisecond) // Brief pause for readability
		}

		log.Printf("AIController: [Tool %d/%d] Executing '%s'", i+1, len(toolCalls), tc.Function.Name)

		// Parse arguments from json.RawMessage
		var params map[string]interface{}
		if err := json.Unmarshal(tc.Function.Arguments, &params); err != nil {
			log.Printf("AIController: [Tool %d/%d] ERROR - Failed to parse arguments for '%s': %v",
				i+1, len(toolCalls), tc.Function.Name, err)
			errorResult := agents.FormatToolResult(tc.Function.Name, "", fmt.Errorf("invalid arguments: %v", err))
			toolResults = append(toolResults, errorResult)

			// Stream error immediately (only if streaming enabled)
			if streaming {
				c.streamToolResult(w, flusher, tc.Function.Name, errorResult, i+1, len(toolCalls))
			}
			continue
		}

		// Inject conversation ID for todo_update tool
		if tc.Function.Name == "todo_update" {
			params["_conversation_id"] = conversationID
		}

		// Log parsed parameters for debugging
		log.Printf("AIController: [Tool %d/%d] Parameters: %v", i+1, len(toolCalls), params)

		// Stream execution status (only if streaming enabled)
		if streaming {
			execMsg := fmt.Sprintf("🔧 Executing: %s", tc.Function.Name)
			if len(toolCalls) > 1 {
				execMsg = fmt.Sprintf("🔧 Executing tool %d/%d: %s", i+1, len(toolCalls), tc.Function.Name)
			}
			fmt.Fprintf(w, "event: status\ndata: <span class='loading loading-spinner loading-xs'></span> %s\n\n", execMsg)
			flusher.Flush()
		}

		// Execute the tool
		result, err := c.toolRegistry.ExecuteTool(tc.Function.Name, params, userID)
		toolDuration := time.Since(toolStart)

		if err != nil {
			// Format error result with clear indication of failure
			errorResult := fmt.Sprintf("❌ Tool '%s' failed: %v\nThe tool encountered an error and could not complete. You may want to try again with different parameters or use an alternative approach.", tc.Function.Name, err)
			toolResults = append(toolResults, errorResult)
			log.Printf("AIController: [Tool %d/%d] '%s' FAILED in %.3fs: %v",
				i+1, len(toolCalls), tc.Function.Name, toolDuration.Seconds(), err)

			// Stream error result immediately (only if streaming enabled)
			if streaming {
				c.streamToolResult(w, flusher, tc.Function.Name, errorResult, i+1, len(toolCalls))
			}
		} else {
			successResult := agents.FormatToolResult(tc.Function.Name, result, nil)
			toolResults = append(toolResults, successResult)

			// Better result preview for logging
			lines := strings.Split(result, "\n")
			preview := result
			if len(lines) > 3 {
				preview = fmt.Sprintf("%s... (%d lines total)", strings.Join(lines[:3], "\n"), len(lines))
			} else if len(result) > 200 {
				preview = result[:200] + "..."
			}
			log.Printf("AIController: [Tool %d/%d] '%s' SUCCESS in %.3fs. Result preview: %s",
				i+1, len(toolCalls), tc.Function.Name, toolDuration.Seconds(), preview)

			// Stream success result immediately (only if streaming enabled)
			if streaming {
				c.streamToolResult(w, flusher, tc.Function.Name, successResult, i+1, len(toolCalls))
			}
		}

		// Stream completion status (only if streaming enabled)
		if streaming {
			c.streamThought(w, flusher, fmt.Sprintf("Completed %s successfully", tc.Function.Name))
			fmt.Fprintf(w, "event: status\ndata: Ready\n\n")
			flusher.Flush()
			time.Sleep(200 * time.Millisecond) // Brief pause between tools for visibility
		}
	}

	totalDuration := time.Since(startTime)
	log.Printf("AIController: All %d tools completed in %.3fs", len(toolCalls), totalDuration.Seconds())

	return toolResults
}

// streamThought sends a thinking event via SSE
func (c *AIController) streamThought(w http.ResponseWriter, flusher http.Flusher, thought string) {
	if w == nil || flusher == nil {
		log.Printf("AIController: streamThought - Skipping, w or flusher is nil")
		return // Skip if not streaming
	}

	log.Printf("AIController: streamThought - Sending: %s", thought)

	// Format thinking as HTML for HTMX to insert
	thinkingHTML := fmt.Sprintf(`<div class="text-xs italic text-base-content/50 pl-4 border-l-2 border-base-300">%s</div>`, template.HTMLEscapeString(thought))

	// Send as dedicated thinking event
	fmt.Fprintf(w, "event: thinking\ndata: %s\n\n", thinkingHTML)
	flusher.Flush()

	// Small pause for readability
	time.Sleep(100 * time.Millisecond)
}

// streamToolResult streams a single tool result via SSE
func (c *AIController) streamToolResult(w http.ResponseWriter, flusher http.Flusher, toolName string, result string, current int, total int) {
	// Create the tool result HTML
	toolHTML := fmt.Sprintf(`<div class="collapse collapse-arrow bg-base-200/30 my-2">
		<input type="checkbox" class="peer" />
		<div class="collapse-title min-h-0 py-2 px-3 peer-checked:pb-0">
			<div class="flex items-center gap-2">
				<svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4 text-info flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
				</svg>
				<div class="flex-1">
					<span class="text-xs font-semibold">%s</span>`, toolName)

	// Add progress indicator if multiple tools
	if total > 1 {
		toolHTML += fmt.Sprintf(`<span class="text-xs text-base-content/60 ml-2">Tool %d/%d</span>`, current, total)
	}

	toolHTML += `<span class="text-xs text-info ml-2">Click for details</span>
				</div>
			</div>
		</div>
		<div class="collapse-content px-3 pt-2">
			<div class="text-xs max-h-96 overflow-y-auto">
				<pre class="whitespace-pre-wrap font-mono bg-base-300/50 p-2 rounded">` + template.HTMLEscapeString(result) + `</pre>
			</div>
		</div>
	</div>`

	// SSE data must be on a single line - replace newlines
	toolHTMLEscaped := strings.ReplaceAll(toolHTML, "\n", "")
	toolHTMLEscaped = strings.ReplaceAll(toolHTMLEscaped, "\t", "")
	fmt.Fprintf(w, "event: tool\ndata: %s\n\n", toolHTMLEscaped)
	flusher.Flush()

	log.Printf("AIController: Streamed tool result %d/%d via SSE", current, total)
}

// getTodoPanel renders the todo panel for a conversation
func (c *AIController) getTodoPanel(w http.ResponseWriter, r *http.Request) {
	conversationID := r.PathValue("id")

	// Get conversation
	conversation, err := models.Conversations.Get(conversationID)
	if err != nil {
		c.RenderErrorMsg(w, r, "Conversation not found")
		return
	}

	// Check ownership
	user, _, err := c.App.Use("auth").(*AuthController).Authenticate(r)
	if err != nil || conversation.UserID != user.ID {
		c.RenderErrorMsg(w, r, "Unauthorized")
		return
	}

	// Get todos
	todos, err := models.GetTodosByConversation(conversationID)
	if err != nil {
		todos = []*models.Todo{}
	}

	// Calculate stats
	completedCount := 0
	for _, todo := range todos {
		if todo.Status == models.TodoStatusCompleted {
			completedCount++
		}
	}

	totalCount := len(todos)
	percentComplete := 0
	if totalCount > 0 {
		percentComplete = (completedCount * 100) / totalCount
	}

	// Render panel
	c.Render(w, r, "ai-todos.html", map[string]interface{}{
		"ConversationID":  conversationID,
		"Todos":           todos,
		"HasTodos":        len(todos) > 0,
		"TotalCount":      totalCount,
		"CompletedCount":  completedCount,
		"PercentComplete": percentComplete,
	})
}

// getTodos returns just the todo list items (for HTMX refresh)
func (c *AIController) getTodos(w http.ResponseWriter, r *http.Request) {
	conversationID := r.PathValue("id")

	// Get conversation
	conversation, err := models.Conversations.Get(conversationID)
	if err != nil {
		c.RenderErrorMsg(w, r, "Conversation not found")
		return
	}

	// Check ownership
	user, _, err := c.App.Use("auth").(*AuthController).Authenticate(r)
	if err != nil || conversation.UserID != user.ID {
		c.RenderErrorMsg(w, r, "Unauthorized")
		return
	}

	// Get todos
	todos, err := models.GetTodosByConversation(conversationID)
	if err != nil {
		todos = []*models.Todo{}
	}

	// Render just the todo items
	w.Header().Set("Content-Type", "text/html")
	for _, todo := range todos {
		statusIcon := ""
		contentClass := "text-sm text-base-content/80"

		switch todo.Status {
		case models.TodoStatusCompleted:
			statusIcon = `<input type="checkbox" checked disabled class="checkbox checkbox-xs checkbox-success mt-0.5" />`
			contentClass = "text-sm line-through text-base-content/50"
		case models.TodoStatusInProgress:
			statusIcon = `<span class="loading loading-spinner loading-xs text-primary mt-0.5"></span>`
			contentClass = "text-sm text-primary font-medium"
		default:
			statusIcon = `<input type="checkbox" disabled class="checkbox checkbox-xs mt-0.5" />`
		}

		fmt.Fprintf(w, `<div class="flex items-start gap-2 py-1 group">
			%s
			<span class="%s">%s</span>
		</div>`, statusIcon, contentClass, template.HTMLEscapeString(todo.Content))
	}
}

// streamTodos provides SSE endpoint for todo updates
func (c *AIController) streamTodos(w http.ResponseWriter, r *http.Request) {
	conversationID := r.PathValue("id")

	// Get conversation
	conversation, err := models.Conversations.Get(conversationID)
	if err != nil {
		http.Error(w, "Conversation not found", http.StatusNotFound)
		return
	}

	// Check ownership
	user, _, err := c.App.Use("auth").(*AuthController).Authenticate(r)
	if err != nil || conversation.UserID != user.ID {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Send initial connection event
	fmt.Fprintf(w, "event: connected\ndata: Todo stream connected\n\n")
	flusher.Flush()

	// Keep connection alive with periodic pings
	// In a real implementation, this would watch for todo changes
	// For now, just keep the connection open
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			fmt.Fprintf(w, "event: ping\ndata: keepalive\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// stopExecution handles cancellation of AI execution
func (c *AIController) stopExecution(w http.ResponseWriter, r *http.Request) {
	conversationID := r.PathValue("id")
	user, _, err := c.App.Use("auth").(*AuthController).Authenticate(r)
	if err != nil || !user.IsAdmin {
		http.Error(w, "Admin access required", http.StatusForbidden)
		return
	}

	// Verify ownership
	conversation, err := models.Conversations.Get(conversationID)
	if err != nil || conversation.UserID != user.ID {
		http.Error(w, "Conversation not found", http.StatusNotFound)
		return
	}

	// The actual cancellation is handled by context.Done() in streamResponse
	// This endpoint just acknowledges the request
	log.Printf("AIController: Stop execution requested for conversation %s", conversationID)

	// For HTMX requests, use c.Refresh to properly handle the response
	// This will trigger the appropriate HTMX behavior
	c.Refresh(w, r)
}

// compressToolOutput compresses verbose tool outputs to save context window space
func (c *AIController) compressToolOutput(toolName string, output string) string {
	lines := strings.Split(output, "\n")

	switch toolName {
	case "list_repos":
		// Extract just repo names and IDs
		repoCount := strings.Count(output, "Repository:")
		if repoCount > 0 {
			// Parse and summarize
			summary := fmt.Sprintf("Found %d repositories", repoCount)
			// Extract first few repo names if possible
			var repoNames []string
			for _, line := range lines {
				if strings.Contains(line, "Name:") {
					parts := strings.Split(line, "Name:")
					if len(parts) > 1 {
						name := strings.TrimSpace(parts[1])
						repoNames = append(repoNames, name)
						if len(repoNames) >= 3 {
							break
						}
					}
				}
			}
			if len(repoNames) > 0 {
				summary += ": " + strings.Join(repoNames, ", ")
				if repoCount > len(repoNames) {
					summary += fmt.Sprintf(" (and %d more)", repoCount-len(repoNames))
				}
			}
			return summary
		}

	case "list_files":
		fileCount := len(lines)
		dirCount := 0
		for _, line := range lines {
			if strings.HasSuffix(line, "/") {
				dirCount++
			}
		}
		summary := fmt.Sprintf("Found %d files in %d directories", fileCount-dirCount, dirCount)

		// Show first few important files
		var importantFiles []string
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasSuffix(line, "README.md") ||
				strings.HasSuffix(line, "main.go") ||
				strings.HasSuffix(line, "Makefile") ||
				strings.HasSuffix(line, "package.json") {
				importantFiles = append(importantFiles, line)
			}
		}
		if len(importantFiles) > 0 {
			summary += ". Key files: " + strings.Join(importantFiles, ", ")
		}
		return summary

	case "read_file":
		if len(lines) > 50 {
			// For long files, show summary
			return fmt.Sprintf("Read %d lines. Shows file structure and implementation details.", len(lines))
		}

	case "run_command":
		if len(output) > 500 {
			// Compress long command outputs
			preview := output
			if len(output) > 200 {
				preview = output[:200] + "..."
			}
			return fmt.Sprintf("Command output (%d chars): %s", len(output), preview)
		}
	}

	// For short outputs or unrecognized tools, return as-is
	if len(output) < 200 {
		return output
	}

	// Default compression for long outputs
	return fmt.Sprintf("Output (%d lines, %d chars) - content available in full context", len(lines), len(output))
}

// updateWorkingContext updates the conversation's working context with new information
func (c *AIController) updateWorkingContext(conversationID string, updates map[string]interface{}) {
	conversation, err := models.Conversations.Get(conversationID)
	if err != nil {
		log.Printf("AIController: Failed to get conversation for context update: %v", err)
		return
	}

	for key, value := range updates {
		if err := conversation.UpdateWorkingContext(key, value); err != nil {
			log.Printf("AIController: Failed to update working context: %v", err)
		}
	}
}

// extractContextFromToolCall extracts contextual information from tool calls
func (c *AIController) extractContextFromToolCall(toolName string, params map[string]interface{}, result string) map[string]interface{} {
	context := make(map[string]interface{})

	switch toolName {
	case "get_repo", "list_repos":
		// Extract repo information
		if repoID, ok := params["repo_id"]; ok {
			context["current_repo_id"] = repoID
		}
		if strings.Contains(result, "Name:") {
			lines := strings.Split(result, "\n")
			for _, line := range lines {
				if strings.Contains(line, "Name:") {
					parts := strings.Split(line, "Name:")
					if len(parts) > 1 {
						context["current_repo_name"] = strings.TrimSpace(parts[1])
					}
					break
				}
			}
		}

	case "read_file", "write_file", "edit_file":
		// Track current file
		if filePath, ok := params["file_path"]; ok {
			context["current_file_path"] = filePath
			// Extract directory
			if pathStr, ok := filePath.(string); ok {
				lastSlash := strings.LastIndex(pathStr, "/")
				if lastSlash > 0 {
					context["current_directory"] = pathStr[:lastSlash]
				}
			}
		}

	case "list_files":
		// Track current directory
		if path, ok := params["path"]; ok {
			context["current_directory"] = path
		}
	}

	return context
}

// resolveContextualReferences enhances user messages with context
func (c *AIController) resolveContextualReferences(message string, workingContext map[string]interface{}) string {
	lowerMessage := strings.ToLower(message)
	contextHints := []string{}

	// Check for contextual references
	if strings.Contains(lowerMessage, "that repo") ||
		strings.Contains(lowerMessage, "the repo") ||
		strings.Contains(lowerMessage, "this repo") {
		if repoName, ok := workingContext["current_repo_name"]; ok {
			contextHints = append(contextHints, fmt.Sprintf("Repository context: %v", repoName))
		}
		if repoID, ok := workingContext["current_repo_id"]; ok {
			contextHints = append(contextHints, fmt.Sprintf("Repository ID: %v", repoID))
		}
	}

	if strings.Contains(lowerMessage, "that file") ||
		strings.Contains(lowerMessage, "the file") ||
		strings.Contains(lowerMessage, "this file") {
		if filePath, ok := workingContext["current_file_path"]; ok {
			contextHints = append(contextHints, fmt.Sprintf("Current file: %v", filePath))
		}
	}

	if strings.Contains(lowerMessage, "that directory") ||
		strings.Contains(lowerMessage, "this directory") ||
		strings.Contains(lowerMessage, "the directory") ||
		strings.Contains(lowerMessage, "current directory") {
		if dir, ok := workingContext["current_directory"]; ok {
			contextHints = append(contextHints, fmt.Sprintf("Current directory: %v", dir))
		}
	}

	// Add context hints to message if any were found
	if len(contextHints) > 0 {
		return message + "\n\n[Context: " + strings.Join(contextHints, ", ") + "]"
	}

	return message
}

// buildContextWindow creates an optimized context window for the AI
func (c *AIController) buildContextWindow(conversation *models.Conversation, maxMessages int) []services.OllamaMessage {
	messages, _ := conversation.GetMessages()
	context := []services.OllamaMessage{
		{
			Role:    "system",
			Content: c.buildSystemPromptWithAutonomy(conversation.ID),
		},
	}

	// Add working context as system message if it exists
	workingContext := conversation.GetWorkingContext()
	if len(workingContext) > 0 {
		contextJSON, _ := json.Marshal(workingContext)
		context = append(context, services.OllamaMessage{
			Role:    "system",
			Content: fmt.Sprintf("Working Context: %s", contextJSON),
		})
	}

	// Smart message selection - prioritize recent and important messages
	startIdx := 0
	if len(messages) > maxMessages {
		startIdx = len(messages) - maxMessages
	}

	for i := startIdx; i < len(messages); i++ {
		msg := messages[i]

		// Skip old thinking messages
		if msg.Role == models.MessageRoleThinking && i < len(messages)-5 {
			continue
		}

		// Skip old status messages
		if msg.Role == models.MessageRoleStatus && i < len(messages)-10 {
			continue
		}

		// Compress tool outputs
		content := msg.Content
		if msg.Role == models.MessageRoleTool && msg.ToolName != "" {
			content = c.compressToolOutput(msg.ToolName, content)
		}

		role := msg.Role
		// Map custom roles to standard Ollama roles
		switch msg.Role {
		case models.MessageRoleThinking, models.MessageRoleStatus, models.MessageRolePlan:
			role = "assistant"
		case models.MessageRoleTool:
			role = "tool"
		}

		context = append(context, services.OllamaMessage{
			Role:    role,
			Content: content,
		})
	}

	return context
}

// buildSystemPromptWithAutonomy creates an enhanced system prompt for autonomous execution
func (c *AIController) buildSystemPromptWithAutonomy(conversationID string) string {
	basePrompt := c.buildSystemPrompt(conversationID)

	autonomousInstructions := `

**AUTONOMOUS EXECUTION MODE:**
You can explore and work autonomously, taking initiative to discover interesting things.

For exploration tasks - FOLLOW THIS PATTERN:
1. Use ONE tool (e.g., list_repos)
2. Analyze and explain what you found
3. Decide what to explore next
4. Use the NEXT tool (e.g., get_repo)
5. Repeat this cycle

NEVER use multiple tools at once. Always:
- One tool → Analyze → Explain → Next tool
- Share insights after EACH tool
- Build understanding step by step

After each action, decide:
- **CONTINUE**: Keep exploring/working (default for exploration)
- **COMPLETE**: Only when you've thoroughly explored or finished the task
- **CLARIFY**: Only if you genuinely need user input

Be curious and thorough. For "explore" requests, don't stop after just listing - actually explore the codebase by examining files, understanding architecture, and finding interesting patterns

**CONTEXT AWARENESS:**
- Previous discoveries are preserved in your working context
- "that repo" or "the repo" refers to the last mentioned repository
- "that file" or "the file" refers to the current file being discussed
- "it" refers to the last mentioned entity
- Use context to resolve ambiguous references without asking for clarification

**EXECUTION STYLE:**
- Be proactive and take initiative
- Break complex tasks into steps and execute them sequentially
- Provide updates as you progress through the task
- Only stop when the task is complete or you genuinely need user input`

	return basePrompt + autonomousInstructions
}
