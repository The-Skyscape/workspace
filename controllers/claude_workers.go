package controllers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/The-Skyscape/devtools/pkg/authentication"
	"workspace/internal/claude"
	"workspace/models"
	"workspace/services"
)

// createWorker handles creating a new AI worker
func (c *ClaudeController) createWorker(w http.ResponseWriter, r *http.Request) {
	user, _, err := c.App.Use("auth").(*authentication.Controller).Authenticate(r)
	if err != nil {
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}

	// Parse form to get selected repositories
	if err := r.ParseForm(); err != nil {
		c.RenderErrorMsg(w, r, "invalid form data")
		return
	}

	// Collect repository IDs from form
	var repoIDs []string
	for key, values := range r.Form {
		if strings.HasPrefix(key, "repo_") && len(values) > 0 {
			repoID := values[0]
			// Verify user has access to this repo
			repo, err := models.Repositories.Get(repoID)
			if err == nil && repo != nil && (repo.UserID == user.ID || user.IsAdmin) {
				repoIDs = append(repoIDs, repoID)
			}
		}
	}

	if len(repoIDs) == 0 {
		c.RenderErrorMsg(w, r, "please select at least one repository")
		return
	}

	// Create worker record
	worker := &models.AIWorker{
		UserID:       user.ID,
		Name:         fmt.Sprintf("Assistant-%d", time.Now().Unix()),
		Status:       models.WorkerStatusCreating,
		LastActiveAt: time.Now(),
	}
	worker, err = models.AIWorkers.Insert(worker)
	if err != nil {
		log.Printf("Claude: Failed to create worker: %v", err)
		c.RenderErrorMsg(w, r, "failed to create worker")
		return
	}

	// Associate repositories with worker
	for _, repoID := range repoIDs {
		if err := models.AddRepositoryToWorker(worker.ID, repoID); err != nil {
			log.Printf("Claude: Failed to add repo %s to worker: %v", repoID, err)
		}
	}

	// Start async worker initialization
	go initializeWorker(worker, repoIDs)

	// Return to workers list showing the new creating worker
	c.Render(w, r, "claude-workers-list.html", nil)
}

// initializeWorker initializes a worker in the background
func initializeWorker(worker *models.AIWorker, repoIDs []string) {
	// Prepare repository names
	var repoNames []string
	for _, repoID := range repoIDs {
		repo, err := models.Repositories.Get(repoID)
		if err == nil && repo != nil {
			repoNames = append(repoNames, repo.Name)
		}
	}

	// Create worker manager
	authManager := claude.NewAuthManager(models.Secrets)
	workerManager := claude.NewWorkerManager(authManager, services.SandboxAdapter{})
	
	// Initialize worker with repositories
	config := claude.WorkerConfig{
		WorkerID:  worker.ID,
		RepoIDs:   repoIDs,
		RepoNames: repoNames,
		UserID:    worker.UserID,
	}
	
	sandbox, err := workerManager.InitializeWorker(config)
	if err != nil {
		log.Printf("Claude: Failed to initialize worker %s: %v", worker.ID, err)
		worker.Status = models.WorkerStatusError
		models.AIWorkers.Update(worker)
		return
	}

	// Wait for initialization to complete
	time.Sleep(10 * time.Second)

	// Update worker status
	sandboxName := fmt.Sprintf("claude-worker-%s", worker.ID)
	worker.Status = models.WorkerStatusReady
	worker.SandboxID = sandboxName
	if err := models.AIWorkers.Update(worker); err != nil {
		log.Printf("Claude: Failed to update worker status: %v", err)
	}

	log.Printf("Claude: Worker %s initialized successfully with sandbox %v", worker.ID, sandbox)
}

// getWorkerChat loads the chat interface for a specific worker
func (c *ClaudeController) getWorkerChat(w http.ResponseWriter, r *http.Request) {
	user, _, err := c.App.Use("auth").(*authentication.Controller).Authenticate(r)
	if err != nil {
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}

	workerID := r.PathValue("id")
	worker, err := models.AIWorkers.Get(workerID)
	if err != nil || worker == nil {
		c.RenderErrorMsg(w, r, "worker not found")
		return
	}

	// Check ownership
	if worker.UserID != user.ID && !user.IsAdmin {
		c.RenderErrorMsg(w, r, "access denied")
		return
	}
	
	// Render appropriate view based on status
	if worker.Status == models.WorkerStatusCreating {
		c.Render(w, r, "claude-creating.html", map[string]interface{}{
			"Worker": worker,
		})
	} else {
		// Use enhanced chat view for streaming support
		c.Render(w, r, "claude-chat-enhanced.html", map[string]interface{}{
			"Worker": worker,
		})
	}
}

// deleteWorker handles deleting a worker
func (c *ClaudeController) deleteWorker(w http.ResponseWriter, r *http.Request) {
	user, _, err := c.App.Use("auth").(*authentication.Controller).Authenticate(r)
	if err != nil {
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}

	workerID := r.PathValue("id")
	worker, err := models.AIWorkers.Get(workerID)
	if err != nil || worker == nil {
		c.RenderErrorMsg(w, r, "worker not found")
		return
	}

	// Check ownership
	if worker.UserID != user.ID && !user.IsAdmin {
		c.RenderErrorMsg(w, r, "access denied")
		return
	}

	// Stop sandbox if running
	if worker.SandboxID != "" {
		authManager := claude.NewAuthManager(models.Secrets)
		workerManager := claude.NewWorkerManager(authManager, services.SandboxAdapter{})
		workerManager.CleanupWorker(worker.SandboxID)
	}

	// Clear conversation history
	models.ClearConversationHistory(worker.ID)

	// Mark worker as stopped
	worker.Status = models.WorkerStatusStopped
	models.AIWorkers.Update(worker)

	// Return to workers list
	c.Render(w, r, "claude-workers-list.html", nil)
}

// getChatHistory returns the chat history for a worker
func (c *ClaudeController) getChatHistory(w http.ResponseWriter, r *http.Request) {
	workerID := r.PathValue("id")
	
	// Verify ownership
	worker, err := models.AIWorkers.Get(workerID)
	if err != nil || worker == nil {
		c.RenderErrorMsg(w, r, "worker not found")
		return
	}

	user, _, _ := c.App.Use("auth").(*authentication.Controller).Authenticate(r)
	if worker.UserID != user.ID && !user.IsAdmin {
		c.RenderErrorMsg(w, r, "access denied")
		return
	}

	// Render chat history
	c.Render(w, r, "chat-history.html", nil)
}

// sendMessage handles sending a message to Claude
func (c *ClaudeController) sendMessage(w http.ResponseWriter, r *http.Request) {
	user, _, err := c.App.Use("auth").(*authentication.Controller).Authenticate(r)
	if err != nil {
		c.RenderErrorMsg(w, r, "authentication required")
		return
	}

	workerID := r.PathValue("id")
	worker, err := models.AIWorkers.Get(workerID)
	if err != nil || worker == nil {
		c.RenderErrorMsg(w, r, "worker not found")
		return
	}

	// Check ownership
	if worker.UserID != user.ID && !user.IsAdmin {
		c.RenderErrorMsg(w, r, "access denied")
		return
	}

	// Parse message
	if err := r.ParseForm(); err != nil {
		c.RenderErrorMsg(w, r, "invalid form data")
		return
	}

	message := r.FormValue("message")
	if message == "" {
		c.RenderErrorMsg(w, r, "message is required")
		return
	}

	// Save user message
	userMsg, err := models.AddMessage(worker.ID, models.RoleUser, message)
	if err != nil {
		log.Printf("Claude: Failed to save user message: %v", err)
	}

	// Render user message immediately
	c.Render(w, r, "chat-message.html", userMsg)

	// Update worker activity
	worker.MarkActive()

	// Trigger Claude response via SSE
	w.Header().Set("HX-Trigger", fmt.Sprintf(`{"claude-respond":{"workerId":"%s","message":"%s"}}`, 
		worker.ID, strings.ReplaceAll(message, `"`, `\"`)))
}

// streamResponse handles streaming Claude's response using JSON streaming
func (c *ClaudeController) streamResponse(w http.ResponseWriter, r *http.Request) {
	workerID := r.PathValue("id")
	worker, err := models.AIWorkers.Get(workerID)
	if err != nil || worker == nil {
		http.Error(w, "worker not found", http.StatusNotFound)
		return
	}

	// Get sandbox
	sandbox, err := services.GetSandbox(worker.SandboxID)
	if err != nil || sandbox == nil {
		http.Error(w, "sandbox not found", http.StatusInternalServerError)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	// Get message from query params
	message := r.URL.Query().Get("message")
	if message == "" {
		fmt.Fprintf(w, "data: {\"type\":\"error\",\"content\":\"No message provided\"}\n\n")
		w.(http.Flusher).Flush()
		return
	}

	// Create worker manager
	authManager := claude.NewAuthManager(models.Secrets)
	workerManager := claude.NewWorkerManager(authManager, services.SandboxAdapter{})
	
	// Get or create stream handler for this worker
	handler, err := workerManager.ExecuteStreamingMessage(sandbox, message)
	if err != nil {
		// Fallback to non-streaming mode if streaming fails
		log.Printf("Streaming failed, falling back to regular mode: %v", err)
		
		output, err := workerManager.ExecuteMessage(sandbox, message)
		if err != nil {
			fmt.Fprintf(w, "data: {\"type\":\"error\",\"content\":\"%s\"}\n\n", 
				strings.ReplaceAll(err.Error(), "\"", "\\\""))
			w.(http.Flusher).Flush()
			return
		}
		
		// Send the response as chunks
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			fmt.Fprintf(w, "data: {\"type\":\"assistant\",\"content\":\"%s\"}\n\n", 
				strings.ReplaceAll(line, "\"", "\\\""))
			w.(http.Flusher).Flush()
			time.Sleep(10 * time.Millisecond)
		}
		
		// Save response
		models.AddMessage(worker.ID, models.RoleAssistant, output)
	} else {
		// Use streaming mode
		defer handler.Stop()
		
		// Collect full response for saving
		var fullResponse strings.Builder
		
		// Create a timeout for the entire streaming operation
		timeout := time.After(2 * time.Minute)
		done := make(chan bool)
		
		go func() {
			// Stream messages from handler to HTTP response
			for {
				select {
				case msg := <-handler.GetOutputChannel():
					// Format message as JSON for SSE
					jsonData, _ := json.Marshal(msg)
					fmt.Fprintf(w, "data: %s\n\n", string(jsonData))
					w.(http.Flusher).Flush()
					
					// Accumulate response
					if msg.Type == claude.MessageTypeAssistant {
						fullResponse.WriteString(msg.Content)
						fullResponse.WriteString("\n")
					}
					
					// Check for completion
					if strings.Contains(msg.Content, "[DONE]") {
						done <- true
						return
					}
					
				case err := <-handler.GetErrorChannel():
					fmt.Fprintf(w, "data: {\"type\":\"error\",\"content\":\"%s\"}\n\n", 
						strings.ReplaceAll(err.Error(), "\"", "\\\""))
					w.(http.Flusher).Flush()
					done <- true
					return
					
				case <-timeout:
					done <- true
					return
				}
			}
		}()
		
		// Wait for completion
		<-done
		
		// Save the full response
		if response := fullResponse.String(); response != "" {
			models.AddMessage(worker.ID, models.RoleAssistant, response)
		}
	}

	// Send completion signal
	fmt.Fprintf(w, "data: {\"type\":\"done\"}\n\n")
	w.(http.Flusher).Flush()
}

