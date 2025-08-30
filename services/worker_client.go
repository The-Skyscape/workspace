package services

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
)

// WorkerClient provides API access to the worker service
type WorkerClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
	mu      sync.RWMutex
}

// WorkerInfo represents a worker instance
type WorkerInfo struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// WorkerMessage represents a Claude message
type WorkerMessage struct {
	Type      string                 `json:"type"`
	Role      string                 `json:"role,omitempty"`
	Content   interface{}            `json:"content,omitempty"`
	ToolName  string                 `json:"tool_name,omitempty"`
	ToolInput map[string]interface{} `json:"tool_input,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// CreateWorkerRequest represents the request to create a worker
type CreateWorkerRequest struct {
	Repos  []string `json:"repos"`
	UserID string   `json:"user_id"`
}

// SendMessageRequest represents a message to send
type SendMessageRequest struct {
	Content   string `json:"content"`
	SessionID string `json:"session_id"`
}

var (
	// Global worker client instance
	Worker *WorkerClient
)

// InitWorkerClient initializes the global worker client
func InitWorkerClient() {
	baseURL := os.Getenv("WORKER_URL")
	if baseURL == "" {
		// Default to localhost for development
		baseURL = "http://localhost:8080"
	}

	apiKey := os.Getenv("WORKER_API_KEY")
	if apiKey == "" {
		apiKey = "dev-secret"
	}

	Worker = NewWorkerClient(baseURL, apiKey)
	log.Printf("Worker client initialized with URL: %s", baseURL)
}

// NewWorkerClient creates a new worker client
func NewWorkerClient(baseURL, apiKey string) *WorkerClient {
	return &WorkerClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		apiKey:  apiKey,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// CreateWorker creates a new worker instance
func (c *WorkerClient) CreateWorker(repos []string, userID string) (*WorkerInfo, error) {
	reqBody := CreateWorkerRequest{
		Repos:  repos,
		UserID: userID,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal request")
	}

	req, err := http.NewRequest("POST", c.baseURL+"/api/workers", bytes.NewBuffer(body))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create request")
	}

	c.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to make request")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, errors.Errorf("worker creation failed: %s", string(body))
	}

	var worker WorkerInfo
	if err := json.NewDecoder(resp.Body).Decode(&worker); err != nil {
		return nil, errors.Wrap(err, "failed to decode response")
	}

	return &worker, nil
}

// GetWorker retrieves worker information
func (c *WorkerClient) GetWorker(workerID string) (*WorkerInfo, error) {
	req, err := http.NewRequest("GET", c.baseURL+"/api/workers/"+workerID, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create request")
	}

	c.setHeaders(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to make request")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, errors.Errorf("failed to get worker: %s", string(body))
	}

	var worker WorkerInfo
	if err := json.NewDecoder(resp.Body).Decode(&worker); err != nil {
		return nil, errors.Wrap(err, "failed to decode response")
	}

	return &worker, nil
}

// DeleteWorker removes a worker instance
func (c *WorkerClient) DeleteWorker(workerID string) error {
	req, err := http.NewRequest("DELETE", c.baseURL+"/api/workers/"+workerID, nil)
	if err != nil {
		return errors.Wrap(err, "failed to create request")
	}

	c.setHeaders(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return errors.Wrap(err, "failed to make request")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return errors.Errorf("failed to delete worker: %s", string(body))
	}

	return nil
}

// SendMessage sends a message to a worker
func (c *WorkerClient) SendMessage(workerID, sessionID, content string) error {
	reqBody := SendMessageRequest{
		Content:   content,
		SessionID: sessionID,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return errors.Wrap(err, "failed to marshal request")
	}

	req, err := http.NewRequest("POST", c.baseURL+"/api/workers/"+workerID+"/message", bytes.NewBuffer(body))
	if err != nil {
		return errors.Wrap(err, "failed to create request")
	}

	c.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return errors.Wrap(err, "failed to make request")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return errors.Errorf("failed to send message: %s", string(body))
	}

	return nil
}

// StreamMessages returns a channel for receiving messages via SSE
func (c *WorkerClient) StreamMessages(workerID, sessionID string) (<-chan WorkerMessage, error) {
	url := fmt.Sprintf("%s/api/workers/%s/stream?session_id=%s", c.baseURL, workerID, sessionID)
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create request")
	}

	c.setHeaders(req)
	req.Header.Set("Accept", "text/event-stream")

	// Use a client without timeout for streaming
	streamClient := &http.Client{}
	resp, err := streamClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to stream")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, errors.Errorf("failed to start stream: %s", string(body))
	}

	messages := make(chan WorkerMessage, 100)

	// Start goroutine to read SSE stream
	go func() {
		defer close(messages)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			
			// SSE format: "data: {json}"
			if strings.HasPrefix(line, "data: ") {
				data := line[6:] // Remove "data: " prefix
				
				var msg WorkerMessage
				if err := json.Unmarshal([]byte(data), &msg); err != nil {
					log.Printf("Failed to parse SSE message: %v", err)
					continue
				}
				
				messages <- msg
			}
			// Ignore heartbeats and other SSE lines
		}

		if err := scanner.Err(); err != nil {
			log.Printf("SSE stream error: %v", err)
		}
	}()

	return messages, nil
}

// GetWorkerStatus gets the status of a worker
func (c *WorkerClient) GetWorkerStatus(workerID string) (map[string]interface{}, error) {
	req, err := http.NewRequest("GET", c.baseURL+"/api/workers/"+workerID+"/status", nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create request")
	}

	c.setHeaders(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to make request")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, errors.Errorf("failed to get status: %s", string(body))
	}

	var status map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, errors.Wrap(err, "failed to decode response")
	}

	return status, nil
}

// IsHealthy checks if the worker service is healthy
func (c *WorkerClient) IsHealthy() bool {
	req, err := http.NewRequest("GET", c.baseURL+"/api/health", nil)
	if err != nil {
		return false
	}

	// Health check doesn't require API key
	resp, err := c.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// setHeaders adds authentication headers to the request
func (c *WorkerClient) setHeaders(req *http.Request) {
	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
}

// UpdateEndpoint updates the base URL and API key for the client
func (c *WorkerClient) UpdateEndpoint(baseURL, apiKey string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.baseURL = strings.TrimSuffix(baseURL, "/")
	c.apiKey = apiKey
}