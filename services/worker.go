package services

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"workspace/models"

	"github.com/pkg/errors"
)

// WorkerService manages AI worker instances using docker exec
type WorkerService struct {
	sessions map[string]*WorkerSession
	mu       sync.RWMutex
}

// WorkerSession represents an active AI chat session
type WorkerSession struct {
	ID           string
	WorkerID     string
	Model        string
	Messages     []OllamaMessage
	Created      time.Time
	LastActive   time.Time
	mu           sync.Mutex
}

// WorkerStatus represents the status of a worker
type WorkerStatus struct {
	ID          string    `json:"id"`
	Status      string    `json:"status"`
	Port        int       `json:"port"`
	Sessions    int       `json:"sessions"`
	LastActive  time.Time `json:"last_active"`
}

var (
	// Worker is the global worker service instance
	Worker = &WorkerService{
		sessions: make(map[string]*WorkerSession),
	}
)

// Init initializes the worker service
func (ws *WorkerService) Init() error {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	
	log.Println("WorkerService: Initializing AI worker service (Ollama mode)")
	
	// Initialize Ollama service
	if err := Ollama.Init(); err != nil {
		log.Printf("WorkerService: Failed to initialize Ollama: %v", err)
		// Don't fail completely, service can retry later
	}
	
	// Start cleanup routine for inactive sessions
	ws.StartCleanupRoutine()
	
	log.Println("WorkerService: Service initialized successfully")
	return nil
}

// IsConfigured checks if the worker service is configured and ready
func (ws *WorkerService) IsConfigured() bool {
	return Ollama.IsConfigured()
}

// CreateWorker creates a new AI worker instance for a user
func (ws *WorkerService) CreateWorker(userID string) (*models.Worker, error) {
	return ws.CreateWorkerWithDetails(userID, "", "")
}

// CreateWorkerWithDetails creates a new AI worker instance with name and description
func (ws *WorkerService) CreateWorkerWithDetails(userID, name, description string) (*models.Worker, error) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	
	// Check if Ollama is running
	if !Ollama.IsRunning() {
		log.Println("WorkerService: Ollama not running, attempting to start...")
		if err := Ollama.Start(); err != nil {
			return nil, errors.New("Ollama service not available")
		}
	}
	
	// Default name if not provided
	if name == "" {
		name = "AI Assistant"
	}
	
	// Create worker model
	worker := &models.Worker{
		Name:         name,
		Description:  description,
		UserID:       userID,
		Status:       "starting", // Start in "starting" status
		Port:         0, // Using Ollama REST API
		ContainerID:  "ollama", // Using Ollama service
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	}
	
	// Save to database
	worker, err := models.Workers.Insert(worker)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create worker record")
	}
	
	// Simulate realistic startup sequence
	go func() {
		workerID := worker.ID
		
		// Phase 1: Container initialization (1.5 seconds)
		time.Sleep(1500 * time.Millisecond)
		log.Printf("WorkerService: Worker %s - initializing container...", workerID)
		
		// Phase 2: Loading AI model (2 seconds)
		time.Sleep(2 * time.Second)
		log.Printf("WorkerService: Worker %s - loading AI model...", workerID)
		
		// Phase 3: Ready for use
		time.Sleep(500 * time.Millisecond)
		
		// Update to running status
		w, _ := models.Workers.Get(workerID)
		if w != nil && w.Status == "starting" {
			w.Status = "running"
			w.LastActiveAt = time.Now()
			models.Workers.Update(w)
			log.Printf("WorkerService: Worker %s is now running", workerID)
		}
	}()
	
	log.Printf("WorkerService: Created worker %s (%s) for user %s", worker.ID, name, userID)
	return worker, nil
}

// CreateSession creates a new AI chat session for a worker
func (ws *WorkerService) CreateSession(workerID string) (*WorkerSession, error) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	
	// Get worker from database
	worker, err := models.Workers.Get(workerID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get worker")
	}
	
	// Determine which model to use
	model := Ollama.config.DefaultModel
	if worker.AIModel != "" {
		model = worker.AIModel
	}
	
	// Create session with initial system message
	session := &WorkerSession{
		ID:           fmt.Sprintf("session-%d", time.Now().UnixNano()),
		WorkerID:     workerID,
		Model:        model,
		Messages: []OllamaMessage{
			{
				Role: "system",
				Content: "You are a helpful AI assistant specializing in software development. " +
					"You have access to the full context of the user's repository and can help with " +
					"coding, debugging, documentation, and architecture decisions.",
			},
		},
		Created:      time.Now(),
		LastActive:   time.Now(),
	}
	
	// Store session in memory
	ws.sessions[session.ID] = session
	
	// Save session to database
	dbSession := &models.WorkerSession{
		WorkerID:     workerID,
		Name:         "New Chat",
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	}
	dbSession, err = models.WorkerSessions.Insert(dbSession)
	if err != nil {
		log.Printf("WorkerService: Failed to save session to database: %v", err)
	} else {
		// Update session ID to match database ID
		session.ID = dbSession.ID
		ws.sessions[dbSession.ID] = session
		delete(ws.sessions, fmt.Sprintf("session-%d", session.Created.UnixNano()))
	}
	
	// Update worker last active time
	worker.LastActiveAt = time.Now()
	models.Workers.Update(worker)
	
	log.Printf("WorkerService: Created session %s for worker %s", session.ID, workerID)
	return session, nil
}

// SendMessage sends a message to an AI session and gets a response
func (ws *WorkerService) SendMessage(sessionID, message string) (string, error) {
	ws.mu.RLock()
	session, ok := ws.sessions[sessionID]
	ws.mu.RUnlock()
	
	if !ok {
		return "", errors.New("session not found")
	}
	
	session.mu.Lock()
	defer session.mu.Unlock()
	
	// Add user message to history
	session.Messages = append(session.Messages, OllamaMessage{
		Role:    "user",
		Content: message,
	})
	
	// Send to Ollama and get response
	response, err := Ollama.Chat(session.Model, session.Messages, false)
	if err != nil {
		// If Ollama is not available, return a helpful error message
		if !Ollama.IsRunning() {
			return "AI service is currently unavailable. Please try again later.", nil
		}
		return "", errors.Wrap(err, "failed to get AI response")
	}
	
	// Add assistant response to history
	session.Messages = append(session.Messages, response.Message)
	
	// Save message to database
	dbMessage := &models.WorkerMessage{
		SessionID: sessionID,
		Role:      "user",
		Content:   message,
		CreatedAt: time.Now(),
	}
	if _, err := models.WorkerMessages.Insert(dbMessage); err != nil {
		log.Printf("WorkerService: Failed to save message to database: %v", err)
	}
	
	// Update session activity
	session.LastActive = time.Now()
	
	// Return the assistant's response
	return response.Message.Content, nil
}

// GetSessionHistory gets the message history from a session
func (ws *WorkerService) GetSessionHistory(sessionID string) ([]OllamaMessage, error) {
	ws.mu.RLock()
	session, ok := ws.sessions[sessionID]
	ws.mu.RUnlock()
	
	if !ok {
		return nil, errors.New("session not found")
	}
	
	session.mu.Lock()
	defer session.mu.Unlock()
	
	// Return a copy of the messages (skip system message)
	var history []OllamaMessage
	for _, msg := range session.Messages {
		if msg.Role != "system" {
			history = append(history, msg)
		}
	}
	
	return history, nil
}

// StopWorker stops a worker and all its sessions
func (ws *WorkerService) StopWorker(workerID string) error {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	
	// Find and remove all sessions for this worker
	for sessionID, session := range ws.sessions {
		if session.WorkerID == workerID {
			delete(ws.sessions, sessionID)
		}
	}
	
	// Update worker status in database
	worker, err := models.Workers.Get(workerID)
	if err != nil {
		return errors.Wrap(err, "failed to get worker")
	}
	
	worker.Status = "stopped"
	if err := models.Workers.Update(worker); err != nil {
		return errors.Wrap(err, "failed to update worker status")
	}
	
	log.Printf("WorkerService: Stopped worker %s", workerID)
	return nil
}

// GetWorkerStatus returns the status of a worker
func (ws *WorkerService) GetWorkerStatus(workerID string) (*WorkerStatus, error) {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	
	worker, err := models.Workers.Get(workerID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get worker")
	}
	
	// Count active sessions
	sessionCount := 0
	for _, session := range ws.sessions {
		if session.WorkerID == workerID {
			sessionCount++
		}
	}
	
	return &WorkerStatus{
		ID:         worker.ID,
		Status:     worker.Status,
		Port:       worker.Port,
		Sessions:   sessionCount,
		LastActive: worker.LastActiveAt,
	}, nil
}

// GetAvailableModels returns a list of available AI models
func (ws *WorkerService) GetAvailableModels() ([]string, error) {
	if !Ollama.IsRunning() {
		return []string{}, errors.New("Ollama service not running")
	}
	return Ollama.ListModels()
}

// StreamMessage sends a message and streams the response
func (ws *WorkerService) StreamMessage(sessionID, message string, callback func(chunk string) error) error {
	ws.mu.RLock()
	session, ok := ws.sessions[sessionID]
	ws.mu.RUnlock()
	
	if !ok {
		return errors.New("session not found")
	}
	
	session.mu.Lock()
	defer session.mu.Unlock()
	
	// Add user message to history
	session.Messages = append(session.Messages, OllamaMessage{
		Role:    "user",
		Content: message,
	})
	
	// Save user message to database
	dbMessage := &models.WorkerMessage{
		SessionID: sessionID,
		Role:      "user",
		Content:   message,
		CreatedAt: time.Now(),
	}
	if _, err := models.WorkerMessages.Insert(dbMessage); err != nil {
		log.Printf("WorkerService: Failed to save user message: %v", err)
	}
	
	// Build assistant response incrementally
	var fullResponse strings.Builder
	
	// Stream response from Ollama
	err := Ollama.StreamChat(session.Model, session.Messages, func(chunk *OllamaChatResponse) error {
		if chunk.Message.Content != "" {
			fullResponse.WriteString(chunk.Message.Content)
			return callback(chunk.Message.Content)
		}
		return nil
	})
	
	if err != nil {
		return errors.Wrap(err, "failed to stream response")
	}
	
	// Add complete response to history
	session.Messages = append(session.Messages, OllamaMessage{
		Role:    "assistant",
		Content: fullResponse.String(),
	})
	
	// Save assistant message to database
	assistantMsg := &models.WorkerMessage{
		SessionID: sessionID,
		Role:      "assistant",
		Content:   fullResponse.String(),
		CreatedAt: time.Now(),
	}
	if _, err := models.WorkerMessages.Insert(assistantMsg); err != nil {
		log.Printf("WorkerService: Failed to save assistant message: %v", err)
	}
	
	session.LastActive = time.Now()
	return nil
}

// CleanupInactiveSessions removes sessions that have been inactive for too long
func (ws *WorkerService) CleanupInactiveSessions() {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	
	cutoff := time.Now().Add(-30 * time.Minute)
	
	for sessionID, session := range ws.sessions {
		if session.LastActive.Before(cutoff) {
			delete(ws.sessions, sessionID)
			log.Printf("WorkerService: Cleaned up inactive session %s", sessionID)
		}
	}
}

// StartCleanupRoutine starts a background routine to clean up inactive sessions
func (ws *WorkerService) StartCleanupRoutine() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		
		for range ticker.C {
			ws.CleanupInactiveSessions()
		}
	}()
}

// GetAllWorkers returns all workers for a user
func (ws *WorkerService) GetAllWorkers(userID string) ([]*models.Worker, error) {
	workers, err := models.Workers.Search("WHERE UserID = ? ORDER BY CreatedAt DESC", userID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get workers")
	}
	return workers, nil
}


// GetServiceInfo returns information about the worker service
func (ws *WorkerService) GetServiceInfo() map[string]interface{} {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	
	info := map[string]interface{}{
		"configured":      Ollama.IsConfigured(),
		"ollama_running":  Ollama.IsRunning(),
		"active_sessions": len(ws.sessions),
		"mode":           "ollama",
	}
	
	// Add Ollama service info if available
	if Ollama.IsRunning() {
		info["ollama"] = Ollama.GetServiceInfo()
	}
	
	return info
}

// GetSessions returns all sessions for a worker from the database
func (ws *WorkerService) GetSessions(workerID string) ([]*models.WorkerSession, error) {
	sessions, err := models.WorkerSessions.Search("WHERE WorkerID = ? ORDER BY UpdatedAt DESC", workerID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get sessions")
	}
	return sessions, nil
}

// GetMessages returns all messages for a session from the database
func (ws *WorkerService) GetMessages(sessionID string) ([]*models.WorkerMessage, error) {
	messages, err := models.WorkerMessages.Search("WHERE SessionID = ? ORDER BY CreatedAt ASC", sessionID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get messages")
	}
	return messages, nil
}

