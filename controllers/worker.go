package controllers

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"workspace/models"
	"workspace/services"

	"github.com/The-Skyscape/devtools/pkg/application"
	"github.com/The-Skyscape/devtools/pkg/authentication"
)

// WorkerController handles AI worker management
type WorkerController struct {
	application.BaseController
	// Store chat messages in memory for demo (maps workerID to messages)
	chatHistory map[string][]map[string]interface{}
	chatMutex   sync.RWMutex
}

// Worker returns the controller factory
func Worker() (string, *WorkerController) {
	return "worker", &WorkerController{
		chatHistory: make(map[string][]map[string]interface{}),
	}
}

// Setup initializes the Worker controller
func (c *WorkerController) Setup(app *application.App) {
	c.App = app
	auth := app.Use("auth").(*authentication.Controller)

	// Main drawer routes
	http.Handle("GET /worker/panel", app.ProtectFunc(c.panel, auth.Required))  // Main drawer content
	http.Handle("POST /worker/create", app.ProtectFunc(c.createWorker, auth.Required))
	
	// Chat routes
	http.Handle("GET /worker/chat/{id}", app.ProtectFunc(c.loadChat, auth.Required))
	http.Handle("GET /worker/chat/{id}/messages", app.ProtectFunc(c.getMessages, auth.Required))
	http.Handle("POST /worker/chat/{id}/send", app.ProtectFunc(c.sendMessage, auth.Required))
	
	// Worker management routes
	http.Handle("POST /worker/{id}/stop", app.ProtectFunc(c.stopWorker, auth.Required))
	http.Handle("POST /worker/{id}/start", app.ProtectFunc(c.startWorker, auth.Required))
	http.Handle("DELETE /worker/{id}", app.ProtectFunc(c.deleteWorker, auth.Required))

	// Initialize worker service in background
	go func() {
		log.Println("WorkerController: Initializing worker service...")
		if err := services.Worker.Init(); err != nil {
			log.Printf("WorkerController: Warning - Worker service not available: %v", err)
		}
		// Start cleanup routine
		services.Worker.StartCleanupRoutine()
	}()
}

// Handle prepares the controller for request handling
func (c WorkerController) Handle(req *http.Request) application.Controller {
	c.Request = req
	return &c
}

// Template helper methods

// IsConfigured checks if the worker service is configured
func (c *WorkerController) IsConfigured() bool {
	// For now, always return true since we're using mock data
	return true
}

// GetWorkers returns all workers for the current user
func (c *WorkerController) GetWorkers() []*models.Worker {
	if c.Request == nil {
		return nil
	}
	
	user, _, err := c.App.Use("auth").(*authentication.Controller).Authenticate(c.Request)
	if err != nil {
		return nil
	}
	
	workers, _ := models.Workers.Search("WHERE UserID = ? AND Status != 'stopped' ORDER BY LastActiveAt DESC", user.ID)
	return workers
}

// GetServiceStatus returns the worker service status
func (c *WorkerController) GetServiceStatus() map[string]interface{} {
	return services.Worker.GetServiceInfo()
}

// Mock data methods for UI development

// calculateResponseDelay calculates an appropriate delay based on message complexity
func calculateResponseDelay(message string) time.Duration {
	msgLen := len(message)
	lowerMsg := strings.ToLower(message)
	
	// Base delay
	baseDelay := 1 * time.Second
	
	// Add delay based on message length
	if msgLen > 100 {
		baseDelay += 500 * time.Millisecond
	}
	if msgLen > 200 {
		baseDelay += 1 * time.Second
	}
	
	// Add delay for code-related queries
	if strings.Contains(lowerMsg, "code") || strings.Contains(lowerMsg, "debug") || 
	   strings.Contains(lowerMsg, "error") || strings.Contains(lowerMsg, "implement") {
		baseDelay += 1500 * time.Millisecond
	}
	
	// Add delay for complex queries
	if strings.Contains(lowerMsg, "explain") || strings.Contains(lowerMsg, "how") ||
	   strings.Contains(lowerMsg, "why") || strings.Contains(lowerMsg, "analyze") {
		baseDelay += 1 * time.Second
	}
	
	// Add some randomness (±500ms)
	randomDelay := time.Duration(time.Now().UnixNano()%1000-500) * time.Millisecond
	totalDelay := baseDelay + randomDelay
	
	// Cap at 4 seconds max
	if totalDelay > 4*time.Second {
		totalDelay = 4 * time.Second
	}
	
	// Minimum 800ms
	if totalDelay < 800*time.Millisecond {
		totalDelay = 800 * time.Millisecond
	}
	
	return totalDelay
}

// generateMockResponse generates contextual responses based on user input
func generateMockResponse(message string) string {
	lowerMsg := strings.ToLower(message)
	
	// Check for specific keywords and provide contextual responses
	switch {
	case strings.Contains(lowerMsg, "hello") || strings.Contains(lowerMsg, "hi") || strings.Contains(lowerMsg, "hey"):
		return "Hello! I'm Claude, your AI coding assistant. I can help you with:\n\n• Writing and debugging code\n• Understanding codebases\n• Implementing features\n• Fixing bugs\n• Writing tests\n• Code reviews\n\nWhat would you like to work on today?"
		
	case strings.Contains(lowerMsg, "help") || strings.Contains(lowerMsg, "what can you"):
		return "I can assist you with a wide range of programming tasks:\n\n**Code Development:**\n• Write functions, classes, and modules\n• Implement algorithms and data structures\n• Create REST APIs and web services\n\n**Debugging & Testing:**\n• Debug errors and fix bugs\n• Write unit and integration tests\n• Optimize performance\n\n**Code Understanding:**\n• Explain how code works\n• Review and suggest improvements\n• Document your code\n\n**Languages & Frameworks:**\nI'm proficient in Go, Python, JavaScript, TypeScript, React, and many more!\n\nJust describe what you need help with!"
		
	case strings.Contains(lowerMsg, "bug") || strings.Contains(lowerMsg, "error") || strings.Contains(lowerMsg, "fix"):
		return "I can help you debug that issue! To provide the best assistance, please share:\n\n1. **The error message** you're seeing (if any)\n2. **The relevant code** where the issue occurs\n3. **What you expected** to happen\n4. **What actually happened**\n\nYou can paste your code here and I'll analyze it for potential issues and suggest fixes."
		
	case strings.Contains(lowerMsg, "test") || strings.Contains(lowerMsg, "testing"):
		return "I can help you write comprehensive tests! I can create:\n\n• **Unit tests** for individual functions\n• **Integration tests** for API endpoints\n• **Test fixtures** and mock data\n• **Test coverage** improvements\n\nWhich component or function would you like to test? Share the code and I'll write appropriate test cases."
		
	case strings.Contains(lowerMsg, "feature") || strings.Contains(lowerMsg, "implement"):
		return "I'd be happy to help you implement that feature! Let's break it down:\n\n1. **What's the feature?** Describe what it should do\n2. **Where does it fit?** Which part of your codebase\n3. **Any constraints?** Performance, compatibility, etc.\n\nOnce you provide these details, I can help you design and implement the feature with clean, maintainable code."
		
	case strings.Contains(lowerMsg, "explain") || strings.Contains(lowerMsg, "understand") || strings.Contains(lowerMsg, "how does"):
		return "I'd be glad to explain that code! Please share:\n\n• The code snippet you'd like explained\n• Any specific parts that are confusing\n• The context (what the code is supposed to do)\n\nI'll provide a detailed explanation of how it works, line by line if needed."
		
	case strings.Contains(lowerMsg, "review") || strings.Contains(lowerMsg, "feedback"):
		return "I'll review your code and provide constructive feedback! I'll look for:\n\n✓ **Code quality** and best practices\n✓ **Potential bugs** or edge cases\n✓ **Performance** optimizations\n✓ **Security** considerations\n✓ **Readability** and maintainability\n\nShare your code and I'll provide a thorough review with suggestions for improvement."
		
	case strings.Contains(lowerMsg, "refactor") || strings.Contains(lowerMsg, "improve"):
		return "I can help refactor your code to make it cleaner and more maintainable! I'll focus on:\n\n• **Simplifying** complex logic\n• **Extracting** reusable functions\n• **Improving** naming and structure\n• **Reducing** duplication\n• **Applying** design patterns\n\nShare the code you'd like to refactor and I'll suggest improvements."
		
	case strings.Contains(lowerMsg, "thank") || strings.Contains(lowerMsg, "thanks"):
		return "You're welcome! I'm here to help anytime. Feel free to ask me anything else about your code or development tasks. Happy coding! 🚀"
		
	default:
		// Generic helpful response for unmatched queries
		return fmt.Sprintf("I understand you're asking about: \"%s\"\n\nI'm here to help with that! Could you provide a bit more context or code examples? This will help me give you a more specific and useful response.\n\nFor example, you could:\n• Share the relevant code snippet\n• Describe what you're trying to achieve\n• Mention any errors you're encountering\n\nI'm ready to assist with coding, debugging, testing, or explaining concepts!", message)
	}
}

// GetMockSessions returns mock chat sessions
func (c *WorkerController) GetMockSessions() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"ID":          "session-1",
			"Title":       "Debug React App",
			"LastMessage": "I'll help you debug that React component...",
			"Time":        "2 min ago",
			"Active":      true,
		},
		{
			"ID":          "session-2", 
			"Title":       "Write Python Script",
			"LastMessage": "Here's a Python script that processes CSV files...",
			"Time":        "1 hour ago",
			"Active":      false,
		},
		{
			"ID":          "session-3",
			"Title":       "SQL Query Help",
			"LastMessage": "The JOIN query would look like this...",
			"Time":        "Yesterday",
			"Active":      false,
		},
	}
}

// GetMockMessages returns mock chat messages for a session
func (c *WorkerController) GetMockMessages(sessionID string) []map[string]interface{} {
	messages := []map[string]interface{}{
		{
			"Role":    "user",
			"Content": "Can you help me debug this React component that's not rendering properly?",
			"Time":    "2:30 PM",
		},
		{
			"Role":    "assistant",
			"Content": "I'll help you debug that React component. Can you share the code that's causing issues?",
			"Time":    "2:31 PM",
		},
		{
			"Role":    "user",
			"Content": "Here's my component:\n```jsx\nfunction MyComponent() {\n  const [data, setData] = useState()\n  return <div>{data.name}</div>\n}\n```",
			"Time":    "2:32 PM",
		},
		{
			"Role":    "assistant",
			"Content": "I can see the issue. The `data` state is initialized as `undefined`, but you're trying to access `data.name` which will cause an error. Here's the fix:\n\n```jsx\nfunction MyComponent() {\n  const [data, setData] = useState({ name: '' })\n  return <div>{data.name}</div>\n}\n```\n\nOr you can use optional chaining:\n```jsx\nreturn <div>{data?.name}</div>\n```",
			"Time":    "2:33 PM",
		},
	}
	
	return messages
}

// Handler methods

// listWorkers returns the worker list view
func (c *WorkerController) listWorkers(w http.ResponseWriter, r *http.Request) {
	c.Render(w, r, "worker-list.html", nil)
}

// getWorkerStatusBadge returns the worker status badge
func (c *WorkerController) getWorkerStatusBadge(w http.ResponseWriter, r *http.Request) {
	workerID := r.PathValue("id")
	worker, err := models.Workers.Get(workerID)
	if err != nil || worker == nil {
		fmt.Fprintf(w, `<span class="badge badge-error badge-xs">Not Found</span>`)
		return
	}

	// Render status badge
	switch worker.Status {
	case models.WorkerStatusRunning:
		fmt.Fprintf(w, `<span class="badge badge-success badge-xs">Running</span>`)
	case models.WorkerStatusStarting:
		fmt.Fprintf(w, `<span class="badge badge-warning badge-xs">Starting</span>`)
	case models.WorkerStatusError:
		fmt.Fprintf(w, `<span class="badge badge-error badge-xs" title="%s">Error</span>`, worker.ErrorMessage)
	default:
		fmt.Fprintf(w, `<span class="badge badge-xs">%s</span>`, worker.Status)
	}
}

// listSessions returns sessions for a worker
func (c *WorkerController) listSessions(w http.ResponseWriter, r *http.Request) {
	user, _, err := c.App.Use("auth").(*authentication.Controller).Authenticate(r)
	if err != nil {
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}

	workerID := r.PathValue("id")
	worker, err := models.Workers.Get(workerID)
	if err != nil || worker == nil {
		c.RenderErrorMsg(w, r, "worker not found")
		return
	}

	// Check ownership
	if worker.UserID != user.ID && !user.IsAdmin {
		c.RenderErrorMsg(w, r, "access denied")
		return
	}

	// Get sessions
	sessions, _ := worker.GetSessions()
	
	data := struct {
		Worker   *models.Worker
		Sessions []*models.WorkerSession
	}{
		Worker:   worker,
		Sessions: sessions,
	}

	c.Render(w, r, "worker-sessions.html", data)
}

// createSession creates a new chat session
func (c *WorkerController) createSession(w http.ResponseWriter, r *http.Request) {
	user, _, err := c.App.Use("auth").(*authentication.Controller).Authenticate(r)
	if err != nil {
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}

	workerID := r.PathValue("id")
	worker, err := models.Workers.Get(workerID)
	if err != nil || worker == nil {
		c.RenderErrorMsg(w, r, "worker not found")
		return
	}

	// Check ownership
	if worker.UserID != user.ID && !user.IsAdmin {
		c.RenderErrorMsg(w, r, "access denied")
		return
	}

	// Parse form
	if err := r.ParseForm(); err != nil {
		c.RenderErrorMsg(w, r, "invalid form data")
		return
	}

	name := r.FormValue("name")
	if name == "" {
		name = fmt.Sprintf("Session %d", time.Now().Unix())
	}

	// Create session
	session := &models.WorkerSession{
		WorkerID:     workerID,
		Name:         name,
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	}

	session, err = models.WorkerSessions.Insert(session)
	if err != nil {
		log.Printf("WorkerController: Failed to create session: %v", err)
		c.RenderErrorMsg(w, r, "Failed to create session")
		return
	}

	// Load chat interface
	c.loadSession(w, r)
}

// loadSession loads the chat interface for a session
func (c *WorkerController) loadSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	session, err := models.WorkerSessions.Get(sessionID)
	if err != nil || session == nil {
		c.RenderErrorMsg(w, r, "session not found")
		return
	}

	// Get worker
	worker, err := models.Workers.Get(session.WorkerID)
	if err != nil || worker == nil {
		c.RenderErrorMsg(w, r, "worker not found")
		return
	}

	// Check ownership
	user, _, _ := c.App.Use("auth").(*authentication.Controller).Authenticate(r)
	if worker.UserID != user.ID && !user.IsAdmin {
		c.RenderErrorMsg(w, r, "access denied")
		return
	}

	// Get messages
	messages, _ := session.GetMessages()

	data := struct {
		Worker   *models.Worker
		Session  *models.WorkerSession
		Messages []*models.WorkerMessage
	}{
		Worker:   worker,
		Session:  session,
		Messages: messages,
	}

	c.Render(w, r, "worker-chat.html", data)
}

// deleteSession removes a session and its messages
func (c *WorkerController) deleteSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	session, err := models.WorkerSessions.Get(sessionID)
	if err != nil || session == nil {
		c.RenderErrorMsg(w, r, "session not found")
		return
	}

	// Get worker for ownership check
	worker, err := models.Workers.Get(session.WorkerID)
	if err != nil || worker == nil {
		c.RenderErrorMsg(w, r, "worker not found")
		return
	}

	// Check ownership
	user, _, _ := c.App.Use("auth").(*authentication.Controller).Authenticate(r)
	if worker.UserID != user.ID && !user.IsAdmin {
		c.RenderErrorMsg(w, r, "access denied")
		return
	}

	// Clear messages
	session.ClearMessages()

	// Delete session
	if err := models.WorkerSessions.Delete(session); err != nil {
		log.Printf("WorkerController: Failed to delete session: %v", err)
		c.RenderErrorMsg(w, r, "Failed to delete session")
		return
	}

	// Return to sessions list
	c.listSessions(w, r)
}

// Removed old sendMessage and getMessages - using new simplified versions below

// Main UI handlers

// panel renders the drawer panel content
func (c *WorkerController) panel(w http.ResponseWriter, r *http.Request) {
	user, _, err := c.App.Use("auth").(*authentication.Controller).Authenticate(r)
	if err != nil {
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}
	
	// Get search query if provided
	searchQuery := strings.TrimSpace(r.URL.Query().Get("q"))
	
	// Get user's workers
	workers, err := services.Worker.GetAllWorkers(user.ID)
	if err != nil {
		workers = []*models.Worker{}
	}
	
	// Transform workers for template with additional data
	workerData := []map[string]interface{}{}
	for _, worker := range workers {
		// Use a default name if empty
		name := "AI Assistant"
		if worker.Name != "" {
			name = worker.Name
		}
		
		// Filter by search query if provided
		if searchQuery != "" {
			lowerQuery := strings.ToLower(searchQuery)
			if !strings.Contains(strings.ToLower(name), lowerQuery) && 
			   !strings.Contains(strings.ToLower(worker.Description), lowerQuery) {
				continue
			}
		}
		
		// Get session count for this worker
		sessions, _ := services.Worker.GetSessions(worker.ID)
		sessionCount := len(sessions)
		
		// Format last active time
		lastActive := "never"
		if !worker.LastActiveAt.IsZero() {
			duration := time.Since(worker.LastActiveAt)
			if duration < time.Minute {
				lastActive = "just now"
			} else if duration < time.Hour {
				lastActive = fmt.Sprintf("%d min ago", int(duration.Minutes()))
			} else if duration < 24*time.Hour {
				lastActive = fmt.Sprintf("%d hours ago", int(duration.Hours()))
			} else {
				lastActive = fmt.Sprintf("%d days ago", int(duration.Hours()/24))
			}
		}
		
		workerData = append(workerData, map[string]interface{}{
			"ID":           worker.ID,
			"Name":         name,
			"Description":  worker.Description,
			"Status":       worker.Status,
			"SessionCount": sessionCount,
			"LastActive":   lastActive,
		})
	}
	
	// If searching, only return the worker list partial
	if searchQuery != "" {
		// Return just the worker list HTML for HTMX update
		w.Write([]byte(`<div class="flex flex-col gap-3" id="worker-list">`))
		for _, worker := range workerData {
			c.renderWorkerCard(w, worker)
		}
		w.Write([]byte(`</div>`))
		return
	}
	
	data := map[string]interface{}{
		"HasWorkers": len(workers) > 0,
		"Workers":    workerData,
	}
	
	c.Render(w, r, "worker-drawer-panel.html", data)
}

// renderWorkerCard is a helper to render a single worker card
func (c *WorkerController) renderWorkerCard(w io.Writer, worker map[string]interface{}) {
	// This is a simplified version - in production you'd use a template
	// Get host from request
	host := "http://" + c.Request.Host
	if c.Request.TLS != nil {
		host = "https://" + c.Request.Host
	}
	
	fmt.Fprintf(w, `<div class="card bg-base-100 shadow-sm border border-base-300 hover:shadow-md transition-all cursor-pointer"
	     hx-get="%s/worker/chat/%s"
	     hx-target="#worker-panel-content"
	     hx-swap="innerHTML">
	    <div class="card-body p-4">
	        <div class="flex items-start justify-between gap-3">
	            <div class="flex-1 min-w-0">
	                <h4 class="font-semibold truncate">%s</h4>`, 
		host, worker["ID"], worker["Name"])
	
	if desc, ok := worker["Description"].(string); ok && desc != "" {
		fmt.Fprintf(w, `<p class="text-sm text-base-content/70 line-clamp-2">%s</p>`, desc)
	}
	
	fmt.Fprintf(w, `</div></div></div></div>`)
}

// loadChat loads the chat interface for a worker
func (c *WorkerController) loadChat(w http.ResponseWriter, r *http.Request) {
	workerID := r.PathValue("id")
	
	// Verify ownership
	user, _, err := c.App.Use("auth").(*authentication.Controller).Authenticate(r)
	if err != nil {
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}
	
	// Get worker
	worker, err := models.Workers.Get(workerID)
	if err != nil || worker.UserID != user.ID {
		c.RenderErrorMsg(w, r, "worker not found")
		return
	}
	
	// Get or create session for this worker
	sessions, _ := services.Worker.GetSessions(worker.ID)
	var sessionID string
	if len(sessions) > 0 {
		sessionID = sessions[0].ID // Use most recent session
	} else {
		// Create new session
		newSession, _ := services.Worker.CreateSession(worker.ID)
		if newSession != nil {
			sessionID = newSession.ID
		}
	}
	
	// Render chat interface
	data := map[string]interface{}{
		"ChatMode": true,
		"Worker":   worker,
		"SessionID": sessionID,
	}
	c.Render(w, r, "worker-drawer-panel.html", data)
}

// getMessages returns chat messages for a worker
func (c *WorkerController) getMessages(w http.ResponseWriter, r *http.Request) {
	workerID := r.PathValue("id")
	
	// Verify ownership
	user, _, err := c.App.Use("auth").(*authentication.Controller).Authenticate(r)
	if err != nil {
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}
	
	// Get worker
	worker, err := models.Workers.Get(workerID)
	if err != nil || worker.UserID != user.ID {
		c.RenderErrorMsg(w, r, "worker not found")
		return
	}
	
	// Get messages from memory store
	c.chatMutex.RLock()
	messages, exists := c.chatHistory[workerID]
	c.chatMutex.RUnlock()
	
	// Add welcome message if no messages
	if !exists || len(messages) == 0 {
		messages = []map[string]interface{}{
			{
				"Content":   "Hello! I'm Claude, your AI coding assistant. I can help you with:\n\n• Writing and debugging code\n• Understanding codebases\n• Implementing features\n• Fixing bugs\n• Writing tests\n• Code reviews\n\nWhat would you like to work on today?",
				"IsUser":    false,
				"Timestamp": time.Now().Format("3:04 PM"),
			},
		}
		// Store the welcome message
		c.chatMutex.Lock()
		c.chatHistory[workerID] = messages
		c.chatMutex.Unlock()
	}
	
	data := map[string]interface{}{
		"Messages": messages,
	}
	c.Render(w, r, "worker-chat-messages.html", data)
}

// sendMessage sends a message to the worker
func (c *WorkerController) sendMessage(w http.ResponseWriter, r *http.Request) {
	workerID := r.PathValue("id")
	message := r.FormValue("message")
	
	// Verify ownership
	user, _, err := c.App.Use("auth").(*authentication.Controller).Authenticate(r)
	if err != nil {
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}
	
	// Get worker
	worker, err := models.Workers.Get(workerID)
	if err != nil || worker.UserID != user.ID {
		c.RenderErrorMsg(w, r, "worker not found")
		return
	}
	
	// Get existing messages
	c.chatMutex.RLock()
	messages, exists := c.chatHistory[workerID]
	c.chatMutex.RUnlock()
	
	if !exists {
		messages = []map[string]interface{}{}
	}
	
	// Add user message immediately
	userMsg := map[string]interface{}{
		"Content":   message,
		"IsUser":    true,
		"Timestamp": time.Now().Format("3:04 PM"),
	}
	messages = append(messages, userMsg)
	
	// Add typing indicator message
	typingMsg := map[string]interface{}{
		"Content":   "Claude is typing...",
		"IsUser":    false,
		"IsTyping":  true,
		"Timestamp": time.Now().Add(100 * time.Millisecond).Format("3:04 PM"),
	}
	messages = append(messages, typingMsg)
	
	// Store messages with typing indicator
	c.chatMutex.Lock()
	c.chatHistory[workerID] = messages
	c.chatMutex.Unlock()
	
	// Render immediately with typing indicator
	data := map[string]interface{}{
		"Messages": messages,
	}
	c.Render(w, r, "worker-chat-messages.html", data)
	
	// Process response asynchronously with realistic delays
	go func() {
		// Calculate delay based on message complexity
		delay := calculateResponseDelay(message)
		time.Sleep(delay)
		
		// Generate contextual mock response
		assistantResponse := generateMockResponse(message)
		
		// Get messages again to replace typing indicator
		c.chatMutex.Lock()
		messages, _ := c.chatHistory[workerID]
		
		// Remove typing indicator (last message)
		if len(messages) > 0 {
			lastMsg := messages[len(messages)-1]
			if isTyping, exists := lastMsg["IsTyping"]; exists && isTyping == true {
				messages = messages[:len(messages)-1]
			}
		}
		
		// Add actual assistant response
		assistantMsg := map[string]interface{}{
			"Content":   assistantResponse,
			"IsUser":    false,
			"Timestamp": time.Now().Format("3:04 PM"),
		}
		messages = append(messages, assistantMsg)
		
		// Store updated messages
		c.chatHistory[workerID] = messages
		c.chatMutex.Unlock()
		
		// Update worker last active time
		if w, err := models.Workers.Get(workerID); err == nil {
			w.LastActiveAt = time.Now()
			models.Workers.Update(w)
		}
		
		log.Printf("WorkerController: Async response generated for worker %s after %v", workerID, delay)
	}()
}

// startWorker starts a stopped worker
func (c *WorkerController) startWorker(w http.ResponseWriter, r *http.Request) {
	workerID := r.PathValue("id")
	
	// Verify ownership
	user, _, err := c.App.Use("auth").(*authentication.Controller).Authenticate(r)
	if err != nil {
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}
	
	// Get worker
	worker, err := models.Workers.Get(workerID)
	if err != nil || worker.UserID != user.ID {
		c.RenderErrorMsg(w, r, "worker not found")
		return
	}
	
	// Start worker with realistic startup sequence
	worker.Status = "starting"
	models.Workers.Update(worker)
	
	// Simulate realistic startup sequence
	go func() {
		// Wait a moment for container to initialize
		time.Sleep(1 * time.Second)
		
		// Update to running status after "startup"
		time.Sleep(2 * time.Second)
		w, _ := models.Workers.Get(workerID)
		if w != nil && w.Status == "starting" {
			w.Status = "running"
			w.LastActiveAt = time.Now()
			models.Workers.Update(w)
		}
	}()
	
	// Re-render panel
	c.panel(w, r)
}

// deleteWorker deletes a worker
func (c *WorkerController) deleteWorker(w http.ResponseWriter, r *http.Request) {
	workerID := r.PathValue("id")
	
	// Verify ownership
	user, _, err := c.App.Use("auth").(*authentication.Controller).Authenticate(r)
	if err != nil {
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}
	
	// Get worker
	worker, err := models.Workers.Get(workerID)
	if err != nil || worker.UserID != user.ID {
		c.RenderErrorMsg(w, r, "worker not found")
		return
	}
	
	// Delete worker
	// Stop and delete worker
	if err := services.Worker.StopWorker(workerID); err != nil {
		log.Printf("WorkerController: Failed to stop worker: %v", err)
	}
	// Delete from database
	if err := models.Workers.Delete(worker); err != nil {
		log.Printf("WorkerController: Failed to delete worker from database: %v", err)
	}
	
	// Re-render panel
	c.panel(w, r)
}

// getMockResponse returns a mock assistant response (kept for legacy support)
func (c *WorkerController) getMockResponse(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	var response string
	
	// Try to get real response from worker
	user, _, err := c.App.Use("auth").(*authentication.Controller).Authenticate(r)
	if err == nil && user != nil && sessionID != "" && !strings.HasPrefix(sessionID, "session-") {
		// Try to get output from real session
		output, err := services.Worker.GetSessionOutput(sessionID)
		if err == nil && output != "" {
			response = output
		} else {
			// Use mock response
			response, _ = services.Worker.SendMockMessage(sessionID, "")
		}
	} else {
		// Mock responses
		responses := []string{
			"I'll help you with that. Let me analyze your code and provide a solution.",
			"That's an interesting question! Here's what I found...",
			"Based on your requirements, I recommend the following approach:",
			"I've identified the issue. Here's how to fix it:",
		}
		response = responses[time.Now().UnixNano()%int64(len(responses))]
	}
	
	data := map[string]interface{}{
		"Role":    "assistant",
		"Content": response,
	}
	c.Render(w, r, "worker-message.html", data)
}

// Real worker management methods

// createWorker creates a new worker for the current user
func (c *WorkerController) createWorker(w http.ResponseWriter, r *http.Request) {
	user, _, err := c.App.Use("auth").(*authentication.Controller).Authenticate(r)
	if err != nil {
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}
	
	// Parse form data
	if err := r.ParseForm(); err != nil {
		c.RenderErrorMsg(w, r, "Invalid form data")
		return
	}
	
	name := r.FormValue("name")
	description := r.FormValue("description")
	
	// Create worker with name and description
	worker, err := services.Worker.CreateWorkerWithDetails(user.ID, name, description)
	if err != nil {
		log.Printf("WorkerController: Failed to create worker: %v", err)
		c.RenderErrorMsg(w, r, fmt.Sprintf("Failed to create worker: %v", err))
		return
	}
	
	// Create initial session
	newSession, _ := services.Worker.CreateSession(worker.ID)
	var sessionID string
	if newSession != nil {
		sessionID = newSession.ID
	}
	
	// Load the chat interface
	data := map[string]interface{}{
		"ChatMode": true,
		"Worker":   worker,
		"SessionID": sessionID,
	}
	c.Render(w, r, "worker-drawer-panel.html", data)
}

// stopWorker stops a worker
func (c *WorkerController) stopWorker(w http.ResponseWriter, r *http.Request) {
	workerID := r.PathValue("id")
	
	// Verify ownership
	user, _, err := c.App.Use("auth").(*authentication.Controller).Authenticate(r)
	if err != nil {
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}
	
	// Get worker to check ownership
	worker, err := models.Workers.Get(workerID)
	if err != nil || worker.UserID != user.ID {
		c.RenderErrorMsg(w, r, "worker not found")
		return
	}
	
	// Stop worker
	if err := services.Worker.StopWorker(workerID); err != nil {
		log.Printf("WorkerController: Failed to stop worker: %v", err)
		c.RenderErrorMsg(w, r, "Failed to stop worker")
		return
	}
	
	c.Render(w, r, "success-alert.html", map[string]string{
		"Message": "Worker stopped successfully",
	})
}

// getWorkerStatus returns the status of a worker
func (c *WorkerController) getWorkerStatus(w http.ResponseWriter, r *http.Request) {
	workerID := r.PathValue("id")
	
	// Verify ownership
	user, _, err := c.App.Use("auth").(*authentication.Controller).Authenticate(r)
	if err != nil {
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}
	
	// Get worker to check ownership
	worker, err := models.Workers.Get(workerID)
	if err != nil || worker.UserID != user.ID {
		c.RenderErrorMsg(w, r, "worker not found")
		return
	}
	
	// Get status
	status, err := services.Worker.GetWorkerStatus(workerID)
	if err != nil {
		c.RenderErrorMsg(w, r, "Failed to get status")
		return
	}
	
	c.Render(w, r, "worker-status.html", status)
}

// streamResponse streams responses from a Claude session using Server-Sent Events
func (c *WorkerController) streamResponse(w http.ResponseWriter, r *http.Request) {
	_ = r.PathValue("sessionID") // TODO: Use for real streaming
	
	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}
	
	// Stream mock data for now
	messages := []string{
		"I'll help you with that.",
		"Let me analyze the code...",
		"Here's what I found:",
		"The solution is implemented.",
	}
	
	for _, msg := range messages {
		// Send SSE event
		fmt.Fprintf(w, "data: %s\n\n", msg)
		flusher.Flush()
		time.Sleep(500 * time.Millisecond)
	}
	
	// Send done event
	fmt.Fprintf(w, "event: done\ndata: complete\n\n")
	flusher.Flush()
}

// statusCheck checks worker status and redirects if it changes from starting to running
func (c *WorkerController) statusCheck(w http.ResponseWriter, r *http.Request) {
	workerID := r.PathValue("id")
	
	// Get worker
	worker, err := models.Workers.Get(workerID)
	if err != nil {
		// Worker not found, no action
		w.WriteHeader(http.StatusNoContent)
		return
	}
	
	// Check if status is now running
	if worker.Status == "running" {
		// Use built-in redirect that handles HTMX properly
		c.Redirect(w, r, fmt.Sprintf("/worker/%s", worker.ID))
		return
	}
	
	// No change, return empty response
	w.WriteHeader(http.StatusNoContent)
}