package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
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

// WorkerSession represents an active Claude session
type WorkerSession struct {
	ID           string
	WorkerID     string
	ContainerID  string
	ClaudeProcess *exec.Cmd
	InputPipe    *bytes.Buffer
	OutputBuffer *bytes.Buffer
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
	
	log.Println("WorkerService: Initializing AI worker service (docker exec mode)")
	
	// Check if Claude is available in the main container
	if !ws.isClaudeAvailable() {
		log.Println("WorkerService: Claude CLI not available in container")
		return errors.New("Claude CLI not installed")
	}
	
	log.Println("WorkerService: Service initialized successfully")
	return nil
}

// isClaudeAvailable checks if Claude CLI is available
func (ws *WorkerService) isClaudeAvailable() bool {
	cmd := exec.Command("which", "claude")
	return cmd.Run() == nil
}

// CreateWorker creates a new AI worker instance for a user
func (ws *WorkerService) CreateWorker(userID string) (*models.Worker, error) {
	return ws.CreateWorkerWithDetails(userID, "", "")
}

// CreateWorkerWithDetails creates a new AI worker instance with name and description
func (ws *WorkerService) CreateWorkerWithDetails(userID, name, description string) (*models.Worker, error) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	
	// Check if API key is configured
	apiKey := ws.getAPIKey()
	if apiKey == "" {
		return nil, errors.New("Anthropic API key not configured")
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
		Port:         0, // No port needed for docker exec
		ContainerID:  "sky-app", // Using main container
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	}
	
	// Save to database
	worker, err := models.Workers.Insert(worker)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create worker record")
	}
	
	// Simulate startup time, then set to running
	go func() {
		time.Sleep(3 * time.Second)
		worker.Status = "running"
		models.Workers.Update(worker)
		log.Printf("WorkerService: Worker %s is now running", worker.ID)
	}()
	
	log.Printf("WorkerService: Created worker %s (%s) for user %s", worker.ID, name, userID)
	return worker, nil
}

// CreateSession creates a new Claude session for a worker
func (ws *WorkerService) CreateSession(workerID string) (*WorkerSession, error) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	
	// Get worker from database
	worker, err := models.Workers.Get(workerID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get worker")
	}
	
	// Create session
	session := &WorkerSession{
		ID:           fmt.Sprintf("session-%d", time.Now().UnixNano()),
		WorkerID:     workerID,
		ContainerID:  worker.ContainerID,
		InputPipe:    &bytes.Buffer{},
		OutputBuffer: &bytes.Buffer{},
		Created:      time.Now(),
		LastActive:   time.Now(),
	}
	
	// Start Claude process using docker exec
	session.ClaudeProcess = exec.Command("docker", "exec", "-i", session.ContainerID, 
		"claude", 
		"--input-format=stream-json",
		"--output-format=stream-json",
		"--replay-user-messages",
		"--allowed-tools", "Bash(git:*),Bash(cd:*),Bash(ls:*),Edit,Read,Write",
		"--dangerously-skip-permissions")
	
	session.ClaudeProcess.Stdin = session.InputPipe
	session.ClaudeProcess.Stdout = session.OutputBuffer
	session.ClaudeProcess.Stderr = session.OutputBuffer
	
	// Set environment variables
	session.ClaudeProcess.Env = append(session.ClaudeProcess.Env, 
		fmt.Sprintf("ANTHROPIC_API_KEY=%s", ws.getAPIKey()))
	
	// Start the process
	if err := session.ClaudeProcess.Start(); err != nil {
		return nil, errors.Wrap(err, "failed to start Claude process")
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

// SendMessage sends a message to a Claude session
func (ws *WorkerService) SendMessage(sessionID, message string) error {
	ws.mu.RLock()
	session, ok := ws.sessions[sessionID]
	ws.mu.RUnlock()
	
	if !ok {
		return errors.New("session not found")
	}
	
	session.mu.Lock()
	defer session.mu.Unlock()
	
	// Prepare JSON message
	msgData := map[string]interface{}{
		"type": "message",
		"content": message,
	}
	
	msgJSON, err := json.Marshal(msgData)
	if err != nil {
		return errors.Wrap(err, "failed to marshal message")
	}
	
	// Send to Claude
	_, err = session.InputPipe.Write(msgJSON)
	if err != nil {
		return errors.Wrap(err, "failed to send message")
	}
	
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
	
	session.LastActive = time.Now()
	return nil
}

// GetSessionOutput gets the output from a Claude session
func (ws *WorkerService) GetSessionOutput(sessionID string) (string, error) {
	ws.mu.RLock()
	session, ok := ws.sessions[sessionID]
	ws.mu.RUnlock()
	
	if !ok {
		return "", errors.New("session not found")
	}
	
	session.mu.Lock()
	defer session.mu.Unlock()
	
	output := session.OutputBuffer.String()
	session.OutputBuffer.Reset()
	
	// If we have output, save it as assistant message
	if output != "" {
		dbMessage := &models.WorkerMessage{
			SessionID: sessionID,
			Role:      "assistant",
			Content:   output,
			CreatedAt: time.Now(),
		}
		if _, err := models.WorkerMessages.Insert(dbMessage); err != nil {
			log.Printf("WorkerService: Failed to save assistant message: %v", err)
		}
	}
	
	return output, nil
}

// StopWorker stops a worker and all its sessions
func (ws *WorkerService) StopWorker(workerID string) error {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	
	// Find and stop all sessions for this worker
	for sessionID, session := range ws.sessions {
		if session.WorkerID == workerID {
			if session.ClaudeProcess != nil && session.ClaudeProcess.Process != nil {
				session.ClaudeProcess.Process.Kill()
			}
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

// getAPIKey retrieves the Anthropic API key from environment or vault
func (ws *WorkerService) getAPIKey() string {
	// Try environment variable first
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		return key
	}
	
	// Try vault/secrets
	secret, err := models.Secrets.GetSecret("integrations/anthropic")
	if err == nil {
		if apiKey, ok := secret["api_key"].(string); ok {
			return apiKey
		}
	}
	
	return ""
}

// CleanupInactiveSessions removes sessions that have been inactive for too long
func (ws *WorkerService) CleanupInactiveSessions() {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	
	cutoff := time.Now().Add(-30 * time.Minute)
	
	for sessionID, session := range ws.sessions {
		if session.LastActive.Before(cutoff) {
			if session.ClaudeProcess != nil && session.ClaudeProcess.Process != nil {
				session.ClaudeProcess.Process.Kill()
			}
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

// ExecuteCommand executes a simple command in the container (for testing)
func (ws *WorkerService) ExecuteCommand(command string) (string, error) {
	cmd := exec.Command("docker", "exec", "sky-app", "sh", "-c", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", errors.Wrapf(err, "command failed: %s", string(output))
	}
	return string(output), nil
}

// GetServiceInfo returns information about the worker service
func (ws *WorkerService) GetServiceInfo() map[string]interface{} {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	
	return map[string]interface{}{
		"configured":      ws.getAPIKey() != "",
		"claude_available": ws.isClaudeAvailable(),
		"active_sessions": len(ws.sessions),
		"mode":           "docker-exec",
	}
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

// Mock implementation for testing without Claude
func (ws *WorkerService) SendMockMessage(sessionID, message string) (string, error) {
	// Simulate Claude response
	time.Sleep(500 * time.Millisecond)
	
	responses := []string{
		"I'll help you with that. Let me analyze the codebase first.",
		"Based on my analysis, here's what I found...",
		"I've completed the task. The changes have been applied.",
		"Let me search for that information in the repository.",
	}
	
	return responses[len(message)%len(responses)], nil
}