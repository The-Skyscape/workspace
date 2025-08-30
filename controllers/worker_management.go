package controllers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/The-Skyscape/devtools/pkg/authentication"
	"workspace/internal/ai"
	"workspace/models"
	"workspace/services"
)

// createWorker handles creating a new AI worker
func (c *WorkerController) createWorker(w http.ResponseWriter, r *http.Request) {
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

	// Check if Claude API key is configured
	authManager := ai.NewAuthManager(models.Secrets)
	if !authManager.IsConfigured() {
		c.RenderErrorMsg(w, r, "Claude API key not configured. Please ask an admin to configure it in settings.")
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
		log.Printf("Worker: Failed to create worker: %v", err)
		c.RenderErrorMsg(w, r, "failed to create worker")
		return
	}

	// Associate repositories with worker
	for _, repoID := range repoIDs {
		if err := models.AddRepositoryToWorker(worker.ID, repoID); err != nil {
			log.Printf("Worker: Failed to add repo %s to worker: %v", repoID, err)
		}
	}

	// Start async worker initialization
	go initializeWorker(worker, repoIDs)

	// Show the creating state for the new worker
	c.Render(w, r, "worker-creating.html", worker)
}

// initializeWorker initializes a worker in the background
func initializeWorker(worker *models.AIWorker, repoIDs []string) {
	// Launch a local worker container
	manager := services.GetWorkerManager()
	workerContainer, err := manager.LaunchWorker(worker.ID)
	if err != nil {
		log.Printf("Worker: Failed to launch worker container %s: %v", worker.ID, err)
		worker.Status = models.WorkerStatusError
		worker.ErrorMessage = fmt.Sprintf("Failed to launch container: %v", err)
		models.AIWorkers.Update(worker)
		return
	}

	// Get the client for this specific worker
	client, err := manager.GetClient(worker.ID)
	if err != nil {
		log.Printf("Worker: Failed to get client for worker %s: %v", worker.ID, err)
		manager.StopWorker(worker.ID)
		worker.Status = models.WorkerStatusError
		worker.ErrorMessage = fmt.Sprintf("Failed to get worker client: %v", err)
		models.AIWorkers.Update(worker)
		return
	}

	// Now create the worker via the API
	workerInfo, err := client.CreateWorker(repoIDs, worker.UserID)
	if err != nil {
		log.Printf("Worker: Failed to initialize worker %s: %v", worker.ID, err)
		manager.StopWorker(worker.ID)
		worker.Status = models.WorkerStatusError
		worker.ErrorMessage = fmt.Sprintf("Failed to initialize worker: %v", err)
		models.AIWorkers.Update(worker)
		return
	}

	// Update worker with container info
	worker.SandboxID = workerInfo.ID
	worker.Status = models.WorkerStatusReady
	if err := models.AIWorkers.Update(worker); err != nil {
		log.Printf("Worker: Failed to update worker status: %v", err)
	}

	log.Printf("Worker: Worker %s initialized successfully on port %d", worker.ID, workerContainer.Port)
}

// getWorkerChat loads the chat interface for a specific worker
func (c *WorkerController) getWorkerChat(w http.ResponseWriter, r *http.Request) {
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
		c.Render(w, r, "worker-creating.html", worker)
	} else if worker.Status == models.WorkerStatusError {
		// Show error view with details
		c.Render(w, r, "worker-error.html", worker)
	} else {
		// Use enhanced chat view for streaming support
		c.Render(w, r, "worker-chat-enhanced.html", worker)
	}
}

// deleteWorker handles deleting a worker
func (c *WorkerController) deleteWorker(w http.ResponseWriter, r *http.Request) {
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

	// Stop the local worker container and remove client
	manager := services.GetWorkerManager()
	if err := manager.StopWorker(worker.ID); err != nil {
		log.Printf("Failed to stop worker container: %v", err)
	}
	
	// Remove worker from worker service via API if it has a sandbox ID
	if worker.SandboxID != "" {
		if client, err := manager.GetClient(worker.ID); err == nil {
			if err := client.DeleteWorker(worker.SandboxID); err != nil {
				log.Printf("Failed to remove worker from service: %v", err)
			}
		}
	}

	// Clear conversation history
	models.ClearConversationHistory(worker.ID)

	// Mark worker as stopped
	worker.Status = models.WorkerStatusStopped
	models.AIWorkers.Update(worker)

	// Return to workers list
	c.Render(w, r, "worker-list.html", nil)
}

// getChatHistory returns the chat history for a worker
func (c *WorkerController) getChatHistory(w http.ResponseWriter, r *http.Request) {
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
func (c *WorkerController) sendMessage(w http.ResponseWriter, r *http.Request) {
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
		log.Printf("Worker: Failed to save user message: %v", err)
	}

	// Render user message immediately
	c.Render(w, r, "chat-message.html", userMsg)

	// Update worker activity
	worker.MarkActive()

	// Trigger Claude response via SSE
	w.Header().Set("HX-Trigger", fmt.Sprintf(`{"claude-respond":{"workerId":"%s","message":"%s"}}`, 
		worker.ID, strings.ReplaceAll(message, `"`, `\"`)))
}

// streamResponse handles streaming Claude's response using the worker API
func (c *WorkerController) streamResponse(w http.ResponseWriter, r *http.Request) {
	workerID := r.PathValue("id")
	worker, err := models.AIWorkers.Get(workerID)
	if err != nil || worker == nil {
		http.Error(w, "worker not found", http.StatusNotFound)
		return
	}

	// Get the client for this specific worker
	manager := services.GetWorkerManager()
	client, err := manager.GetClient(workerID)
	if err != nil {
		http.Error(w, "worker client not available", http.StatusServiceUnavailable)
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

	// Generate session ID for this connection
	sessionID := fmt.Sprintf("%s-%d", workerID, time.Now().UnixNano())

	// Send message to worker service
	if err := client.SendMessage(worker.SandboxID, sessionID, message); err != nil {
		fmt.Fprintf(w, "data: {\"type\":\"error\",\"content\":\"%s\"}\n\n", 
			strings.ReplaceAll(err.Error(), "\"", "\\\""))
		w.(http.Flusher).Flush()
		return
	}

	// Stream messages from worker service
	messages, err := client.StreamMessages(worker.SandboxID, sessionID)
	if err != nil {
		fmt.Fprintf(w, "data: {\"type\":\"error\",\"content\":\"%s\"}\n\n", 
			strings.ReplaceAll(err.Error(), "\"", "\\\""))
		w.(http.Flusher).Flush()
		return
	}

	// Save user message
	models.AddMessage(worker.ID, models.RoleUser, message)

	// Collect full response for saving
	var fullResponse strings.Builder
	
	// Create a timeout for the entire streaming operation
	timeout := time.After(2 * time.Minute)
	
	// Stream messages from worker service to client
	for {
		select {
		case msg, ok := <-messages:
			if !ok {
				// Channel closed, streaming ended
				goto done
			}
			
			// Format message as JSON for SSE
			jsonData, _ := json.Marshal(msg)
			fmt.Fprintf(w, "data: %s\n\n", string(jsonData))
			w.(http.Flusher).Flush()
			
			// Accumulate assistant responses
			if msg.Type == "assistant" {
				if content, ok := msg.Content.(string); ok {
					fullResponse.WriteString(content)
					fullResponse.WriteString("\n")
				}
			}
			
			// Check for completion markers
			if content, ok := msg.Content.(string); ok {
				if strings.Contains(content, "[DONE]") || msg.Type == "result" {
					goto done
				}
			}
			
		case <-timeout:
			// Timeout reached
			goto done
			
		case <-r.Context().Done():
			// Client disconnected
			return
		}
	}
	
done:
	
	// Save the full response
	if response := fullResponse.String(); response != "" {
		models.AddMessage(worker.ID, models.RoleAssistant, response)
	}

	// Send completion signal
	fmt.Fprintf(w, "data: {\"type\":\"done\"}\n\n")
	w.(http.Flusher).Flush()
}

