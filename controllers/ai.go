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

	"workspace/internal/ai"
	"workspace/internal/ai/tools"
	"workspace/models"
	"workspace/services"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

// AIController handles AI chat conversations
type AIController struct {
	application.BaseController
	toolRegistry *ai.ToolRegistry
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
	registry := ai.NewToolRegistry()
	
	// Register ONLY essential tools (6 tools total for maximum efficiency)
	// This minimal set covers 95% of use cases while reducing decision time
	
	registry.Register(&tools.TodoUpdateTool{})    // Task management for complex operations
	registry.Register(&tools.ListReposTool{})     // Find and list repositories  
	registry.Register(&tools.GetRepoTool{})       // Get repository details
	registry.Register(&tools.ListFilesTool{})     // Browse directory structure
	registry.Register(&tools.ReadFileTool{})      // Read file contents
	registry.Register(&tools.RunCommandTool{})    // Universal tool for everything else (git, npm, edit, write, etc.)
	
	return "ai", &AIController{
		toolRegistry: registry,
	}
}

// Setup initializes the AI controller
func (c *AIController) Setup(app *application.App) {
	c.App = app
	auth := app.Use("auth").(*authentication.Controller)

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
	
	user, _, err := c.App.Use("auth").(*authentication.Controller).Authenticate(c.Request)
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
	
	user, _, err := c.App.Use("auth").(*authentication.Controller).Authenticate(c.Request)
	if err != nil || !user.IsAdmin {
		return false
	}
	
	return services.Ollama != nil && services.Ollama.IsRunning()
}

// Handler methods

// panel renders the main AI panel with conversation list (admin only)
func (c *AIController) panel(w http.ResponseWriter, r *http.Request) {
	user, _, err := c.App.Use("auth").(*authentication.Controller).Authenticate(r)
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
	user, _, err := c.App.Use("auth").(*authentication.Controller).Authenticate(r)
	if err != nil || !user.IsAdmin {
		c.RenderErrorMsg(w, r, "Admin access required")
		return
	}
	
	// Create new conversation
	conversation := &models.Conversation{
		UserID:    user.ID,
		Title:     "New Conversation",
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
	user, _, err := c.App.Use("auth").(*authentication.Controller).Authenticate(r)
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
	user, _, err := c.App.Use("auth").(*authentication.Controller).Authenticate(r)
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
	user, _, err := c.App.Use("auth").(*authentication.Controller).Authenticate(r)
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
	user, _, err := c.App.Use("auth").(*authentication.Controller).Authenticate(r)
	if err != nil || !user.IsAdmin {
		c.RenderErrorMsg(w, r, "Admin access required")
		return
	}
	
	// Initialize metrics
	metrics := &AIMetrics{
		StartTime: time.Now(),
		ModelUsed: "llama3.2:3b",
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
		c.Render(w, r, "ai-messages-streaming.html", map[string]interface{}{
			"Messages":       messages,
			"ConversationID": conversationID,
		})
		return
	}
	
	// Non-streaming fallback (shouldn't normally happen)
	messages, _ := conversation.GetMessages()
	
	// Use proper system prompt for llama3.2:3b with tool support
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
	
	// Check if Ollama service is ready
	if !services.Ollama.IsRunning() {
		log.Printf("AIController: Ollama service not running")
		
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
	
	// Get tool definitions in Ollama format
	toolDefinitions := c.convertToOllamaTools(c.toolRegistry.GenerateOllamaTools())
	
	// Always provide tools to Llama 3.2 - let it decide when to use them
	model := "llama3.2:3b"
	thinkingStart := time.Now()
	log.Printf("AIController: Sending request to Llama 3.2 with %d tools available", len(toolDefinitions))
	response, err := services.Ollama.ChatWithTools(model, ollamaMessages, toolDefinitions, false)
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
	finalResponse := response.Message.Content
	
	// Check if model decided to use tools
	if len(response.Message.ToolCalls) == 0 {
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
		
		if len(response.Message.ToolCalls) > 0 {
			// Log tool usage
			toolNames := []string{}
			for _, tc := range response.Message.ToolCalls {
				toolNames = append(toolNames, tc.Function.Name)
			}
			log.Printf("AIController: Executing %d tools (iteration %d): %v", len(response.Message.ToolCalls), iteration+1, toolNames)
			
			// Process native tool calls (without streaming in sendMessage)
			toolResults = c.processNativeToolCalls(response.Message.ToolCalls, conversationID, user.ID, nil, nil)
			
			toolDuration := time.Since(toolStart)
			metrics.ToolDuration += toolDuration
			metrics.ToolCallCount += len(response.Message.ToolCalls)
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
				Role:    "tool",  // Use "tool" role for tool results
				Content: result,
			})
			
			log.Printf("AIController: Added tool result %d/%d to context", i+1, len(toolResults))
		}
		
		// Get new response with tool results
		followUpStart := time.Now()
		log.Printf("AIController: Getting follow-up response after tool execution (iteration %d)", iteration+1)
		response, err = services.Ollama.ChatWithTools(model, ollamaMessages, toolDefinitions, false)
		metrics.ThinkingDuration += time.Since(followUpStart)
		if err != nil {
			log.Printf("AIController: Failed to get follow-up response: %v", err)
			// If we can't get a follow-up, save what we have with the tool results
			finalResponse = finalResponse + "\n\n" + strings.Join(toolResults, "\n")
			break
		}
		
		// Update response for next iteration check
		finalResponse = response.Message.Content
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

// buildSystemPrompt creates the system prompt optimized for llama3.2:3b
func (c *AIController) buildSystemPrompt(conversationID string) string {
	// Prompt optimized for incremental thinking and faster responses
	prompt := `You are an AI coding assistant in the Skyscape development platform.

**Think Step-by-Step:**
Break down tasks into small, manageable steps. Think out loud about what you need to do:
- First, understand what the user is asking
- Then, identify the immediate next action
- Execute that action
- Based on results, decide the next step
- Continue until the task is complete

**Available Tools (6 essential tools):**
• todo_update - Manage task lists for complex operations
• list_repos - Find repositories
• get_repo - Get repository details  
• list_files - Browse directory structure
• read_file - Read file contents
• run_command - Execute ANY command (git, npm, edit files, write files, etc.)

**Decision Framework:**
1. Simple greetings/chat → Respond conversationally, no tools
2. "Show me" / "What's in" → Start with list_repos or list_files
3. "How does X work" → Use list_files then read_file
4. "Create/modify/fix" → Use run_command (can edit, write, create files)
5. "Run/execute/build/test" → Use run_command
6. Complex multi-step tasks → Use todo_update to track progress

**Incremental Approach:**
Don't try to plan everything upfront. Instead:
- Take one step at a time
- Use tool results to inform next action
- Provide updates as you work
- Example: "explore repo" → list_repos (find ID) → list_files (see structure) → read_file (examine code)

Be concise and focus on the immediate next action.`
	
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
			extension.GFM,        // GitHub Flavored Markdown
			extension.Linkify,    // Auto-linkify URLs
			extension.TaskList,   // Task list support
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

// categorizeTools returns relevant tools based on the user's message
func (c *AIController) categorizeTools(message string) []string {
	messageLower := strings.ToLower(message)
	
	// Quick responses that don't need tools
	greetings := []string{"hello", "hi", "hey", "good morning", "good afternoon", "good evening", "how are you", "thanks", "thank you"}
	for _, greeting := range greetings {
		if strings.Contains(messageLower, greeting) && len(messageLower) < 30 {
			return []string{} // No tools needed
		}
	}
	
	// With only 6 tools, we can provide all of them for most queries
	// The AI can easily decide from this minimal set
	
	// Exploration/browsing patterns - all tools useful
	if strings.Contains(messageLower, "explore") || strings.Contains(messageLower, "show me") || 
	   strings.Contains(messageLower, "what's in") || strings.Contains(messageLower, "browse") {
		return []string{"list_repos", "get_repo", "list_files", "read_file"}
	}
	
	// File/code operations - focus on file and command tools
	if strings.Contains(messageLower, "create") || strings.Contains(messageLower, "write") || 
	   strings.Contains(messageLower, "edit") || strings.Contains(messageLower, "modify") ||
	   strings.Contains(messageLower, "fix") || strings.Contains(messageLower, "update") ||
	   strings.Contains(messageLower, "change") || strings.Contains(messageLower, "add") {
		return []string{"list_files", "read_file", "run_command", "todo_update"}
	}
	
	// Code understanding - reading tools
	if strings.Contains(messageLower, "how does") || strings.Contains(messageLower, "explain") ||
	   strings.Contains(messageLower, "what does") || strings.Contains(messageLower, "understand") ||
	   strings.Contains(messageLower, "show me the code") {
		return []string{"list_files", "read_file"}
	}
	
	// Terminal/command operations
	if strings.Contains(messageLower, "run") || strings.Contains(messageLower, "execute") ||
	   strings.Contains(messageLower, "npm") || strings.Contains(messageLower, "git") ||
	   strings.Contains(messageLower, "build") || strings.Contains(messageLower, "test") ||
	   strings.Contains(messageLower, "install") || strings.Contains(messageLower, "deploy") {
		return []string{"run_command"}
	}
	
	// Default: provide all 6 tools (it's a small enough set)
	return []string{"todo_update", "list_repos", "get_repo", "list_files", "read_file", "run_command"}
}

// streamResponse handles SSE streaming of AI responses
func (c *AIController) streamResponse(w http.ResponseWriter, r *http.Request) {
	conversationID := r.PathValue("id")
	user, _, err := c.App.Use("auth").(*authentication.Controller).Authenticate(r)
	if err != nil || !user.IsAdmin {
		http.Error(w, "Admin access required", http.StatusForbidden)
		return
	}
	
	// Initialize metrics for tracking
	metrics := &AIMetrics{
		StartTime: time.Now(),
		ModelUsed: "llama3.2:3b",
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
	
	// Build message history for Ollama
	ollamaMessages := []services.OllamaMessage{
		{
			Role:    "system",
			Content: c.buildSystemPrompt(conversationID),
		},
	}
	
	for _, msg := range messages {
		if msg.Role == models.MessageRoleUser || msg.Role == models.MessageRoleAssistant || msg.Role == models.MessageRoleTool {
			role := msg.Role
			if msg.Role == models.MessageRoleTool {
				role = "tool" // Ollama expects "tool" role for tool results
			}
			ollamaMessages = append(ollamaMessages, services.OllamaMessage{
				Role:    role,
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
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == models.MessageRoleUser {
			lastUserMessage = messages[i].Content
			break
		}
	}
	
	// Categorize and filter tools based on the user's message
	relevantToolNames := c.categorizeTools(lastUserMessage)
	log.Printf("AIController: Last user message: %s", lastUserMessage)
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
	
	// Track thinking time
	thinkingStart := time.Now()
	
	// Always use Llama 3.2 with tools available - let the model decide when to use them
	model := "llama3.2:3b"
	var initialResponse *services.OllamaChatResponse
	
	log.Printf("AIController: Streaming response with Llama 3.2")
	
	// Filter the tool registry to only include relevant tools
	var toolDefinitions []services.OllamaTool
	if len(relevantToolNames) > 0 {
		// Get all tool definitions from registry
		allTools := c.toolRegistry.GenerateOllamaTools()
		var filteredTools []map[string]interface{}
		
		// Filter to only include relevant tools
		for _, toolDef := range allTools {
			if funcDef, ok := toolDef["function"].(map[string]interface{}); ok {
				toolName := funcDef["name"].(string)
				for _, name := range relevantToolNames {
					if toolName == name {
						filteredTools = append(filteredTools, toolDef)
						break
					}
				}
			}
		}
		
		// Convert filtered tools to Ollama format
		toolDefinitions = c.convertToOllamaTools(filteredTools)
	}
	
	// Log tool names for debugging
	toolNames := []string{}
	for _, td := range toolDefinitions {
		toolNames = append(toolNames, td.Function.Name)
	}
	log.Printf("AIController: Providing %d tools to model: %v", len(toolDefinitions), toolNames)
	
	initialResponse, err = services.Ollama.ChatWithTools(model, ollamaMessages, toolDefinitions, false)
	
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
	
	// Process tool calls with agentic loop
	finalResponse := initialResponse.Message.Content
	iteration := 0
	maxIterations := 5
	
	// Check if there are tool calls to process
	if len(initialResponse.Message.ToolCalls) == 0 {
		log.Printf("AIController: No tool calls detected, proceeding with direct response")
		// Skip directly to streaming the response
		goto streamResponse
	}
	
	log.Printf("AIController: Tool calls detected, entering agentic loop")
	
	for iteration < maxIterations {
		var toolResults []string
		toolStart := time.Now()
		
		// Native tool calls from Ollama/Llama 3.2
		if len(initialResponse.Message.ToolCalls) > 0 {
			// Send status about tool usage
			toolNames := []string{}
			for _, tc := range initialResponse.Message.ToolCalls {
				toolNames = append(toolNames, tc.Function.Name)
			}
			
			// Provide initial status indicating tools will be used
			statusMsg := ""
			if len(toolNames) == 1 {
				statusMsg = "🤖 Preparing to use tool..."
			} else {
				statusMsg = fmt.Sprintf("🤖 Preparing to use %d tools...", len(toolNames))
			}
			fmt.Fprintf(w, "event: status\ndata: %s\n\n", statusMsg)
			flusher.Flush()
			
			// Process native tool calls with streaming
			log.Printf("AIController: Processing %d tool calls (iteration %d): %v", len(initialResponse.Message.ToolCalls), iteration+1, toolNames)
			toolResults = c.processNativeToolCalls(initialResponse.Message.ToolCalls, conversationID, user.ID, w, flusher)
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
		thinkingMsg := "🤔 Analyzing results and planning next step..."
		if iteration > 0 {
			thinkingMsg = fmt.Sprintf("🤔 Iteration %d: Thinking about what to do next...", iteration+1)
		}
		fmt.Fprintf(w, "event: status\ndata: <span class='loading loading-spinner loading-xs'></span> %s\n\n", thinkingMsg)
		flusher.Flush()
		
		// Get new response with tool results context
		response, err := services.Ollama.ChatWithTools(model, ollamaMessages, toolDefinitions, false)
		if err != nil {
			finalResponse = finalResponse + "\n\n" + strings.Join(toolResults, "\n")
			break
		}
		
		finalResponse = response.Message.Content
		initialResponse = response  // Update for next iteration
		iteration++
		
		log.Printf("AIController: Stream iteration %d complete", iteration)
	}
	
streamResponse:
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

// convertToOllamaTools converts tool definitions to Ollama's native format
func (c *AIController) convertToOllamaTools(toolDefs []map[string]interface{}) []services.OllamaTool {
	var tools []services.OllamaTool
	
	for _, def := range toolDefs {
		if funcDef, ok := def["function"].(map[string]interface{}); ok {
			tool := services.OllamaTool{
				Type: "function",
				Function: services.OllamaToolFunction{
					Name:        funcDef["name"].(string),
					Description: funcDef["description"].(string),
				},
			}
			
			// Add parameters if available
			if params, ok := funcDef["parameters"].(map[string]interface{}); ok {
				tool.Function.Parameters = params
			}
			
			tools = append(tools, tool)
		}
	}
	
	return tools
}

// processNativeToolCalls processes tool calls from Ollama's native response format
func (c *AIController) processNativeToolCalls(toolCalls []services.OllamaToolCall, conversationID, userID string, w http.ResponseWriter, flusher http.Flusher) []string {
	if c.toolRegistry == nil {
		log.Printf("AIController: ERROR - Tool registry is nil")
		return nil
	}
	
	var toolResults []string
	startTime := time.Now()
	streaming := w != nil && flusher != nil  // Check if streaming is enabled
	
	log.Printf("AIController: Processing %d tool calls", len(toolCalls))
	
	for i, tc := range toolCalls {
		toolStart := time.Now()
		
		// Stream thought/planning message (only if streaming enabled)
		if streaming {
			thoughtMsg := fmt.Sprintf("🤔 Planning: %s", tc.Function.Name)
			if len(toolCalls) > 1 {
				thoughtMsg = fmt.Sprintf("🤔 Planning tool %d/%d: %s", i+1, len(toolCalls), tc.Function.Name)
			}
			fmt.Fprintf(w, "event: status\ndata: %s\n\n", thoughtMsg)
			flusher.Flush()
			time.Sleep(200 * time.Millisecond) // Brief pause for visibility
		}
		
		log.Printf("AIController: [Tool %d/%d] Executing '%s'", i+1, len(toolCalls), tc.Function.Name)
		
		// Parse arguments from json.RawMessage
		var params map[string]interface{}
		if err := json.Unmarshal(tc.Function.Arguments, &params); err != nil {
			log.Printf("AIController: [Tool %d/%d] ERROR - Failed to parse arguments for '%s': %v", 
				i+1, len(toolCalls), tc.Function.Name, err)
			errorResult := ai.FormatToolResult(tc.Function.Name, "", fmt.Errorf("invalid arguments: %v", err))
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
			errorResult := ai.FormatToolResult(tc.Function.Name, "", err)
			toolResults = append(toolResults, errorResult)
			log.Printf("AIController: [Tool %d/%d] '%s' FAILED in %.3fs: %v", 
				i+1, len(toolCalls), tc.Function.Name, toolDuration.Seconds(), err)
			
			// Stream error result immediately (only if streaming enabled)
			if streaming {
				c.streamToolResult(w, flusher, tc.Function.Name, errorResult, i+1, len(toolCalls))
			}
		} else {
			successResult := ai.FormatToolResult(tc.Function.Name, result, nil)
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
			completeMsg := fmt.Sprintf("✅ Completed: %s", tc.Function.Name)
			if len(toolCalls) > 1 {
				completeMsg = fmt.Sprintf("✅ Completed tool %d/%d: %s", i+1, len(toolCalls), tc.Function.Name)
			}
			fmt.Fprintf(w, "event: status\ndata: %s\n\n", completeMsg)
			flusher.Flush()
			time.Sleep(300 * time.Millisecond) // Brief pause between tools for visibility
		}
	}
	
	totalDuration := time.Since(startTime)
	log.Printf("AIController: All %d tools completed in %.3fs", len(toolCalls), totalDuration.Seconds())
	
	return toolResults
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
	user, _, err := c.App.Use("auth").(*authentication.Controller).Authenticate(r)
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
	user, _, err := c.App.Use("auth").(*authentication.Controller).Authenticate(r)
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
	user, _, err := c.App.Use("auth").(*authentication.Controller).Authenticate(r)
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