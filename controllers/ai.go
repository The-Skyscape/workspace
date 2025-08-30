package controllers

import (
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
	
	// Check for search query
	searchQuery := strings.TrimSpace(r.URL.Query().Get("q"))
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
		
		// Return partial for search
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
	model := "qwen2.5-coder:1.5b" // Use the model that's already loaded
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
	
	// Process tool calls if any
	processedContent, toolResults := c.processToolCalls(response.Message.Content, conversationID, user.ID)
	
	// Save tool execution messages
	for _, result := range toolResults {
		toolMsg := &models.Message{
			ConversationID: conversationID,
			Role:           models.MessageRoleTool,
			Content:        result,
			CreatedAt:      time.Now(),
		}
		models.Messages.Insert(toolMsg)
	}
	
	// If tools were used, get a follow-up response
	if len(toolResults) > 0 {
		// Add tool results to context
		for _, result := range toolResults {
			ollamaMessages = append(ollamaMessages, services.OllamaMessage{
				Role:    "system",
				Content: "Tool result: " + result,
			})
		}
		
		// Get new response with tool results
		response, err = services.Ollama.Chat(model, ollamaMessages, false)
		if err == nil {
			processedContent = response.Message.Content
		}
	}
	
	// Save assistant response
	assistantMsg := &models.Message{
		ConversationID: conversationID,
		Role:           models.MessageRoleAssistant,
		Content:        processedContent,
		CreatedAt:      time.Now(),
	}
	assistantMsg, err = models.Messages.Insert(assistantMsg)
	if err != nil {
		log.Printf("AIController: Failed to save assistant message: %v", err)
	}
	
	// Update conversation's last message
	conversation.UpdateLastMessage(processedContent, models.MessageRoleAssistant)
	
	// Get all messages and render
	messages, _ = conversation.GetMessages()
	c.Render(w, r, "ai-messages.html", messages)
}

// buildSystemPrompt creates the system prompt for the AI
func (c *AIController) buildSystemPrompt() string {
	prompt := `You are an AI assistant integrated into the Skyscape development platform. 
You help users with coding, debugging, documentation, and development tasks.
Be concise, helpful, and accurate in your responses.
When users ask about code, provide clear examples and explanations.`
	
	// Add tool information if available
	if c.toolRegistry != nil {
		prompt += c.toolRegistry.GenerateToolPrompt()
	}
	
	return prompt
}

// processToolCalls processes any tool calls in the AI response
func (c *AIController) processToolCalls(response, conversationID, userID string) (string, []string) {
	if c.toolRegistry == nil {
		return response, nil
	}
	
	// Parse tool calls from the response
	toolCalls, cleanedText := ai.ParseToolCalls(response)
	
	if len(toolCalls) == 0 {
		return response, nil
	}
	
	var toolResults []string
	
	// Execute each tool call
	for _, tc := range toolCalls {
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