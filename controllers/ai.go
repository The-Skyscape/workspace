package controllers

import (
	"bytes"
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

// AI returns the controller factory
func AI() (string, *AIController) {
	// Initialize tool registry
	registry := ai.NewToolRegistry()
	
	// Register repository tools
	registry.Register(&tools.ListReposTool{})
	registry.Register(&tools.GetRepoTool{})
	registry.Register(&tools.CreateRepoTool{})
	registry.Register(&tools.GetRepoLinkTool{})
	
	// Register file tools
	registry.Register(&tools.ListFilesTool{})
	registry.Register(&tools.ReadFileTool{})
	registry.Register(&tools.SearchFilesTool{})
	
	// Register file modification tools
	registry.Register(&tools.EditFileTool{})
	registry.Register(&tools.WriteFileTool{})
	registry.Register(&tools.DeleteFileTool{})
	registry.Register(&tools.MoveFileTool{})
	
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
	
	// Check if client supports streaming (can check for HX-Request header)
	useStreaming := r.Header.Get("HX-Request") == "true"
	
	if useStreaming {
		// Return the user message and streaming setup
		messages, _ := conversation.GetMessages()
		
		// Render a template that sets up SSE
		c.Render(w, r, "ai-messages-streaming.html", map[string]interface{}{
			"Messages":       messages,
			"ConversationID": conversationID,
		})
		return
	}
	
	// Build message history for Ollama
	messages, _ := conversation.GetMessages()
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
	
	// Get response from Ollama
	model := "gpt-oss:20b" // Use GPT-OSS with native tool calling support
	response, err := services.Ollama.Chat(model, ollamaMessages, false)
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
	maxIterations := 5 // Prevent infinite loops
	iteration := 0
	
	for iteration < maxIterations {
		// Check for tool calls in the response
		processedContent, toolResults := c.processToolCalls(finalResponse, conversationID, user.ID)
		
		if len(toolResults) == 0 {
			// No tool calls found, use the processed content as final response
			finalResponse = processedContent
			break
		}
		
		// Save tool execution messages
		for _, result := range toolResults {
			toolMsg := &models.Message{
				ConversationID: conversationID,
				Role:           models.MessageRoleTool,
				Content:        result,
				CreatedAt:      time.Now(),
			}
			models.Messages.Insert(toolMsg)
			
			// Add tool result to conversation context
			ollamaMessages = append(ollamaMessages, services.OllamaMessage{
				Role:    "assistant",
				Content: finalResponse, // Include the assistant's tool call
			})
			ollamaMessages = append(ollamaMessages, services.OllamaMessage{
				Role:    "system",
				Content: "Tool execution result:\n" + result + "\n\nBased on this result, please provide a helpful response to the user's original request.",
			})
		}
		
		// Get new response with tool results
		log.Printf("AIController: Getting follow-up response after tool execution (iteration %d)", iteration+1)
		response, err = services.Ollama.Chat(model, ollamaMessages, false)
		if err != nil {
			log.Printf("AIController: Failed to get follow-up response: %v", err)
			// If we can't get a follow-up, save what we have with the tool results
			finalResponse = processedContent + "\n\n" + strings.Join(toolResults, "\n")
			break
		}
		
		// Use the new response for the next iteration
		finalResponse = response.Message.Content
		iteration++
		
		// Check if this response also contains tool calls
		// The loop will continue if it does
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
	// Generate dynamic tool list from registry
	toolPrompt := c.toolRegistry.GenerateToolPrompt()
	
	prompt := `You are an AI assistant powered by GPT-OSS, integrated into the Skyscape development platform. 
You help users with coding, debugging, documentation, and development tasks.

## Core Capabilities
- Native function calling and tool usage
- Advanced reasoning for complex development tasks
- Repository management and code analysis
- File operations with Git integration

## Important Guidelines
1. ALWAYS use tools to interact with the system - never guess or make up information
2. When users ask about repositories, files, or code, use the appropriate tools to get real data
3. You can chain multiple tool calls to complete complex tasks
4. Provide clear, actionable responses based on tool results
5. Use your reasoning capabilities to plan multi-step operations

## Tool Usage
When you need to use tools, the GPT-OSS model supports native function calling. You can make multiple tool calls in sequence to accomplish complex tasks. After each tool execution, analyze the results and determine if additional tools are needed.

CRITICAL: When you need to use a tool, your ENTIRE response must be ONLY this format - nothing else:
<tool_call>
{"tool": "tool_name", "params": {"param": "value"}}
</tool_call>

DO NOT explain what you're going to do. DO NOT describe your plan. Just execute the tool immediately.

Examples:

User: "Summarize the README.md of sky-castle"
You: <tool_call>
{"tool": "read_file", "params": {"repo_id": "sky-castle", "path": "README.md"}}
</tool_call>

User: "What files are in the workspace repo?"
You: <tool_call>
{"tool": "list_files", "params": {"repo_id": "workspace"}}
</tool_call>

User: "List all repositories"
You: <tool_call>
{"tool": "list_repos", "params": {}}
</tool_call>

User: "Create a new Python script hello.py in the test-repo that prints Hello World"
You: <tool_call>
{"tool": "write_file", "params": {"repo_id": "test-repo", "path": "hello.py", "content": "print('Hello World')", "message": "Add hello.py script"}}
</tool_call>

User: "Fix the typo in config.json where it says 'debuf' instead of 'debug'"
You: <tool_call>
{"tool": "read_file", "params": {"repo_id": "myapp", "path": "config.json"}}
</tool_call>
[After getting content, then:]
You: <tool_call>
{"tool": "edit_file", "params": {"repo_id": "myapp", "path": "config.json", "content": "[corrected content]", "message": "Fix typo: debuf -> debug"}}
</tool_call>

IMPORTANT RULES:
1. When asked about data, immediately execute the appropriate tool - DO NOT explain your plan
2. Your response must be ONLY the XML tool call, nothing else
3. After I provide tool results, then you can explain and summarize for the user
4. If you need multiple tools, execute them one at a time
5. For file operations, always specify the repo_id parameter`
	
	// Append the dynamically generated tool documentation
	prompt += toolPrompt
	
	return prompt
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
	
	// First get the complete response without streaming to handle tool calls
	model := "gpt-oss:20b"
	initialResponse, err := services.Ollama.Chat(model, ollamaMessages, false)
	if err != nil {
		fmt.Fprintf(w, "event: error\ndata: Failed to get AI response: %s\n\n", err.Error())
		flusher.Flush()
		return
	}
	
	// Process tool calls with agentic loop
	finalResponse := initialResponse.Message.Content
	iteration := 0
	maxIterations := 5
	
	for iteration < maxIterations {
		// Check for tool calls
		processedContent, toolResults := c.processToolCalls(finalResponse, conversationID, user.ID)
		
		if len(toolResults) == 0 {
			// No tool calls, use the processed content
			finalResponse = processedContent
			break
		}
		
		// Save tool results
		for _, result := range toolResults {
			toolMsg := &models.Message{
				ConversationID: conversationID,
				Role:           models.MessageRoleTool,
				Content:        result,
				CreatedAt:      time.Now(),
			}
			models.Messages.Insert(toolMsg)
		}
		
		// Build new context with tool results
		ollamaMessages = append(ollamaMessages, services.OllamaMessage{
			Role:    "assistant",
			Content: finalResponse,
		})
		ollamaMessages = append(ollamaMessages, services.OllamaMessage{
			Role:    "system",
			Content: "Tool execution result:\n" + strings.Join(toolResults, "\n") + "\n\nBased on this result, please provide a helpful response to the user's original request.",
		})
		
		// Get follow-up response
		response, err := services.Ollama.Chat(model, ollamaMessages, false)
		if err != nil {
			finalResponse = processedContent + "\n\n" + strings.Join(toolResults, "\n")
			break
		}
		
		finalResponse = response.Message.Content
		iteration++
	}
	
	// Send initial message structure (replaces typing indicator)
	startHTML, err := c.RenderString("ai-streaming-start.html", nil)
	if err != nil {
		fmt.Fprintf(w, "event: error\ndata: Failed to render template\n\n")
		flusher.Flush()
		return
	}
	fmt.Fprintf(w, "event: start\ndata: %s\n\n", startHTML)
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
	
	// Send the complete formatted message
	htmlContent := c.RenderMessageMarkdown(finalResponse)
	completeHTML, err := c.RenderString("ai-streaming-complete.html", map[string]interface{}{
		"Content": htmlContent,
	})
	if err != nil {
		fmt.Fprintf(w, "event: error\ndata: Failed to render final message\n\n")
		flusher.Flush()
		return
	}
	fmt.Fprintf(w, "event: complete\ndata: %s\n\n", completeHTML)
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