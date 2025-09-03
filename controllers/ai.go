package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
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
	
	// OPTIMIZED: Register only essential tools (reduced from 11 to 5)
	// Repository tools
	registry.Register(&tools.ListReposTool{})
	registry.Register(&tools.GetRepoTool{})
	
	// File tools (read-only)
	registry.Register(&tools.ListFilesTool{})
	registry.Register(&tools.ReadFileTool{})
	registry.Register(&tools.SearchFilesTool{})
	
	// Commented out for performance - can be re-enabled if needed
	// registry.Register(&tools.CreateRepoTool{})
	// registry.Register(&tools.GetRepoLinkTool{})
	// registry.Register(&tools.EditFileTool{})
	// registry.Register(&tools.WriteFileTool{})
	// registry.Register(&tools.DeleteFileTool{})
	// registry.Register(&tools.MoveFileTool{})
	
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
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
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
		CreatedAt:      time.Now(),
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
	
	// Use proper system prompt for llama3.2 with tool support
	systemPrompt := c.buildSystemPrompt()
	
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
	
	// Check if Ollama service is ready
	if !services.Ollama.IsRunning() {
		log.Printf("AIController: Ollama service not running")
		
		// Save error message with helpful information
		errorMsg := &models.Message{
			ConversationID: conversationID,
			Role:           models.MessageRoleError,
			Content:        "AI service is initializing. This may take a few minutes while the model is being loaded. Please try again shortly.",
			CreatedAt:      time.Now(),
		}
		models.Messages.Insert(errorMsg)
		
		// Render messages with error
		messages, _ = conversation.GetMessages()
		c.Render(w, r, "ai-messages.html", messages)
		return
	}
	
	// Get tool definitions in Ollama format
	toolDefinitions := c.convertToOllamaTools(c.toolRegistry.GenerateOllamaTools())
	
	// Get response from Ollama WITH TOOLS
	model := "llama3.2:3b"
	thinkingStart := time.Now()
	log.Printf("AIController: Getting AI response with tools for message: %s", content)
	response, err := services.Ollama.ChatWithTools(model, ollamaMessages, toolDefinitions, false)
	metrics.ThinkingDuration = time.Since(thinkingStart)
	log.Printf("AIController: Initial response received in %.1fs", metrics.ThinkingDuration.Seconds())
	
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
			CreatedAt:      time.Now(),
		}
		models.Messages.Insert(errorMsg)
		
		// Render messages with error
		messages, _ = conversation.GetMessages()
		c.Render(w, r, "ai-messages.html", messages)
		return
	}
	
	// Process tool calls and implement agentic loop
	finalResponse := response.Message.Content
	
	// Early exit if no tools are needed
	if len(response.ToolCalls) == 0 && !strings.Contains(finalResponse, "<tool_call>") {
		log.Printf("AIController: No tool calls detected, returning direct response")
		// Save and return the response immediately
		assistantMsg := &models.Message{
			ConversationID: conversationID,
			Role:           models.MessageRoleAssistant,
			Content:        finalResponse,
			CreatedAt:      time.Now(),
		}
		models.Messages.Insert(assistantMsg)
		conversation.UpdateLastMessage(finalResponse, models.MessageRoleAssistant)
		
		metrics.TotalDuration = time.Since(metrics.StartTime)
		log.Printf("AIController: Response metrics - Time: %.1fs, Model: %s", 
			metrics.TotalDuration.Seconds(), metrics.ModelUsed)
		
		messages, _ = conversation.GetMessages()
		c.Render(w, r, "ai-messages.html", messages)
		return
	}
	
	maxIterations := 5 // Prevent infinite loops
	iteration := 0
	
	for iteration < maxIterations {
		// Check for native tool calls first, then fall back to XML parsing
		var toolResults []string
		
		if len(response.ToolCalls) > 0 {
			// Process native tool calls
			log.Printf("AIController: Processing %d native tool calls (iteration %d)", len(response.ToolCalls), iteration+1)
			toolResults = c.processNativeToolCalls(response.ToolCalls, conversationID, user.ID)
		} else {
			// Fall back to XML parsing for compatibility
			var processedContent string
			processedContent, toolResults = c.processToolCalls(finalResponse, conversationID, user.ID)
			if len(toolResults) == 0 {
				finalResponse = processedContent
				break
			}
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
				CreatedAt:      time.Now(),
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
		log.Printf("AIController: Getting follow-up response after tool execution (iteration %d)", iteration+1)
		response, err = services.Ollama.ChatWithTools(model, ollamaMessages, toolDefinitions, false)
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
		CreatedAt:      time.Now(),
	}
	assistantMsg, err = models.Messages.Insert(assistantMsg)
	if err != nil {
		log.Printf("AIController: Failed to save assistant message: %v", err)
	}
	
	// Update conversation's last message
	conversation.UpdateLastMessage(finalResponse, models.MessageRoleAssistant)
	
	// Get all messages and render
	messages, _ = conversation.GetMessages()
	c.Render(w, r, "ai-messages.html", messages)
}

// buildSystemPrompt creates the system prompt for the AI
func (c *AIController) buildSystemPrompt() string {
	// Clear prompt for llama3.2:3b
	prompt := `You are a helpful AI assistant for Skyscape development platform.

When users greet you or have general questions, respond naturally and friendly.

You have tools available to help with code and repositories, but only use them when specifically needed:
- Use tools ONLY when users ask about their repositories, files, or code
- For greetings and general chat, just respond normally without tools
- Be concise and helpful`
	
	return prompt
	
	/*
	// Original verbose prompt (kept for reference)
	prompt := `You are an AI assistant powered by GPT-OSS, integrated into the Skyscape development platform.
- Repository management and code analysis
- File operations with Git integration
- Automatic tool selection and intelligent chaining

## Important Guidelines
1. Use the provided tools to interact with the system - never guess or make up information
2. When users ask about repositories, files, or code, call the appropriate functions to get real data
3. You can make multiple tool calls in sequence to complete complex tasks
4. After receiving tool results, analyze them and determine if additional tools are needed
5. Provide clear, actionable responses based on actual tool results

## How Tool Calling Works
You have native function calling capabilities. When you need information or to perform actions:
- The system detects when you need to call tools
- You can call multiple tools in one response when they're independent
- After tool execution, you'll receive the results
- Based on results, you can call additional tools if needed
- The system handles up to 5 iterations of tool calls automatically

## Multi-Tool Examples

**Example 1: Code Analysis**
User: "Analyze the main.go file and tell me what it does"
→ Call read_file(repo_id="app", path="main.go")
→ Receive file content
→ Provide analysis based on actual code

**Example 2: Project Setup**
User: "Create a new Python project with a README"
→ Call create_repo(name="python-app", description="New Python project")
→ Call write_file(repo_id="python-app", path="README.md", content="# Python App\n...")
→ Call write_file(repo_id="python-app", path="main.py", content="def main():\n...")
→ Confirm creation with actual results

**Example 3: Code Modification**
User: "Fix the typo in config.json where it says 'debuf' instead of 'debug'"
→ Call read_file(repo_id="app", path="config.json")
→ Analyze content to find the typo
→ Call edit_file(repo_id="app", path="config.json", content="[corrected]", message="Fix typo")
→ Confirm the fix was applied

## Tool Categories

**Repository Management**: list_repos, get_repo, create_repo, delete_repo, get_repo_link
**File Operations**: list_files, read_file, write_file, edit_file, delete_file, move_file, search_files
**Git Operations**: git_status, git_history, git_diff, git_commit, git_push
**Issues & PRs**: create_issue, get_issue
**Project Management**: create_milestone, create_project_card

## Error Handling
- If a tool fails, explain the error clearly to the user
- Suggest alternative approaches when possible
- Never pretend a tool succeeded if it failed
- Provide partial results when some tools succeed and others fail

Remember: Your strength is in using real data from tools, not making assumptions. Always verify with tools before providing information about the system state.`
	
	return prompt
	*/
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

// processToolCalls processes any tool calls in the AI response
func (c *AIController) processToolCalls(response, conversationID, userID string) (string, []string) {
	if c.toolRegistry == nil {
		log.Printf("AIController: Tool registry is nil")
		return response, nil
	}
	
	log.Printf("AIController: Processing response for tool calls: %s", response)
	
	// Parse tool calls from the response
	toolCalls, cleanedText := ai.ParseToolCalls(response)
	
	log.Printf("AIController: Found %d tool calls", len(toolCalls))
	
	if len(toolCalls) == 0 {
		return response, nil
	}
	
	var toolResults []string
	
	// Execute each tool call
	for _, tc := range toolCalls {
		log.Printf("AIController: Executing tool '%s' with params: %v", tc.Tool, tc.Params)
		result, err := c.toolRegistry.ExecuteTool(tc.Tool, tc.Params, userID)
		if err != nil {
			toolResults = append(toolResults, ai.FormatToolResult(tc.Tool, "", err))
			log.Printf("AIController: Tool execution failed: %v", err)
		} else {
			toolResults = append(toolResults, ai.FormatToolResult(tc.Tool, result, nil))
			log.Printf("AIController: Tool '%s' executed successfully", tc.Tool)
		}
	}
	
	return cleanedText, toolResults
}

// streamResponse handles SSE streaming of AI responses
func (c *AIController) streamResponse(w http.ResponseWriter, r *http.Request) {
	conversationID := r.PathValue("id")
	user, _, err := c.App.Use("auth").(*authentication.Controller).Authenticate(r)
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
			Content: c.buildSystemPrompt(),
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
	
	// Check if Ollama service is ready
	if !services.Ollama.IsRunning() {
		fmt.Fprintf(w, "event: error\ndata: AI service is not running\n\n")
		flusher.Flush()
		return
	}
	
	// Send initial status
	fmt.Fprintf(w, "event: status\ndata: Generating response...\n\n")
	flusher.Flush()
	
	// Check if message might need tools
	model := "llama3.2:3b"
	var initialResponse *services.OllamaChatResponse
	var err error
	
	// Get the latest user message
	lastUserMsg := ollamaMessages[len(ollamaMessages)-1].Content
	needsTools := strings.Contains(strings.ToLower(lastUserMsg), "repo") || 
		strings.Contains(strings.ToLower(lastUserMsg), "file") ||
		strings.Contains(strings.ToLower(lastUserMsg), "code") ||
		strings.Contains(strings.ToLower(lastUserMsg), "search") ||
		strings.Contains(strings.ToLower(lastUserMsg), "list")
	
	if needsTools {
		// Get tool definitions and call with tools
		log.Printf("AIController: Message needs tools, calling ChatWithTools")
		toolDefinitions := c.convertToOllamaTools(c.toolRegistry.GenerateOllamaTools())
		initialResponse, err = services.Ollama.ChatWithTools(model, ollamaMessages, toolDefinitions, false)
	} else {
		// Simple chat without tools
		log.Printf("AIController: Simple message, calling Chat without tools")
		initialResponse, err = services.Ollama.Chat(model, ollamaMessages, false)
	}
	if err != nil {
		fmt.Fprintf(w, "event: error\ndata: Failed to get AI response: %s\n\n", err.Error())
		flusher.Flush()
		return
	}
	
	// Process tool calls with agentic loop
	finalResponse := initialResponse.Message.Content
	iteration := 0
	maxIterations := 5
	
	// Early exit if no tools are needed
	if len(initialResponse.ToolCalls) == 0 && !strings.Contains(finalResponse, "<tool_call>") {
		log.Printf("AIController: No tool calls in streaming response")
		// Skip directly to streaming the response
		goto streamResponse
	}
	
	for iteration < maxIterations {
		// Check for native tool calls first, then fall back to XML parsing
		var toolResults []string
		
		if len(initialResponse.ToolCalls) > 0 {
			// Send thinking status
			fmt.Fprintf(w, "event: status\ndata: <span class='loading loading-spinner loading-xs'></span> Thinking...\n\n")
			flusher.Flush()
			
			// Process native tool calls
			log.Printf("AIController: Processing %d native tool calls in stream (iteration %d)", len(initialResponse.ToolCalls), iteration+1)
			toolResults = c.processNativeToolCalls(initialResponse.ToolCalls, conversationID, user.ID)
		} else {
			// Fall back to XML parsing for compatibility
			var processedContent string
			processedContent, toolResults = c.processToolCalls(finalResponse, conversationID, user.ID)
			if len(toolResults) == 0 {
				finalResponse = processedContent
				break
			}
		}
		
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
		
		// Then add each tool result
		for i, result := range toolResults {
			toolMsg := &models.Message{
				ConversationID: conversationID,
				Role:           models.MessageRoleTool,
				Content:        result,
				CreatedAt:      time.Now(),
			}
			models.Messages.Insert(toolMsg)
			
			// Add tool result to conversation context
			ollamaMessages = append(ollamaMessages, services.OllamaMessage{
				Role:    "tool",
				Content: result,
			})
			
			log.Printf("AIController: Added streaming tool result %d/%d", i+1, len(toolResults))
		}
		
		// Get follow-up response
		fmt.Fprintf(w, "event: status\ndata: <span class='loading loading-spinner loading-xs'></span> Interpreting results...\n\n")
		flusher.Flush()
		
		// Need to use the toolDefinitions from earlier
		toolDefinitions := c.convertToOllamaTools(c.toolRegistry.GenerateOllamaTools())
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
	// Track timing for metrics
	streamStart := time.Now()
	
	// Clear status
	fmt.Fprintf(w, "event: status\ndata: \n\n")
	flusher.Flush()
	
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
	
	// Calculate metrics
	duration := time.Since(streamStart).Seconds()
	
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
			<div class="text-xs text-base-content/60 mt-2">📊 Response: %.1fs</div>
		</div>
	</div>`, htmlContent, duration)
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
			CreatedAt:      time.Now(),
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
func (c *AIController) processNativeToolCalls(toolCalls []services.OllamaToolCall, conversationID, userID string) []string {
	if c.toolRegistry == nil {
		log.Printf("AIController: Tool registry is nil")
		return nil
	}
	
	var toolResults []string
	startTime := time.Now()
	
	log.Printf("AIController: Processing %d native tool calls", len(toolCalls))
	
	for i, tc := range toolCalls {
		toolStart := time.Now()
		log.Printf("AIController: [%d/%d] Executing tool '%s' with arguments: %s", 
			i+1, len(toolCalls), tc.Function.Name, tc.Function.Arguments)
		
		// Parse arguments from json.RawMessage
		var params map[string]interface{}
		if err := json.Unmarshal(tc.Function.Arguments, &params); err != nil {
			log.Printf("AIController: [%d/%d] Failed to parse arguments for '%s': %v", 
				i+1, len(toolCalls), tc.Function.Name, err)
			toolResults = append(toolResults, ai.FormatToolResult(tc.Function.Name, "", err))
			continue
		}
		
		// Execute the tool
		result, err := c.toolRegistry.ExecuteTool(tc.Function.Name, params, userID)
		toolDuration := time.Since(toolStart)
		
		if err != nil {
			toolResults = append(toolResults, ai.FormatToolResult(tc.Function.Name, "", err))
			log.Printf("AIController: [%d/%d] Tool '%s' failed after %v: %v", 
				i+1, len(toolCalls), tc.Function.Name, toolDuration, err)
		} else {
			toolResults = append(toolResults, ai.FormatToolResult(tc.Function.Name, result, nil))
			resultPreview := result
			if len(resultPreview) > 100 {
				resultPreview = resultPreview[:100] + "..."
			}
			log.Printf("AIController: [%d/%d] Tool '%s' succeeded in %v (result: %s)", 
				i+1, len(toolCalls), tc.Function.Name, toolDuration, resultPreview)
		}
	}
	
	totalDuration := time.Since(startTime)
	log.Printf("AIController: Completed %d tool calls in %v", len(toolCalls), totalDuration)
	
	return toolResults
}