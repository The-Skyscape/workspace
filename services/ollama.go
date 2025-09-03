package services

import (
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

	"github.com/The-Skyscape/devtools/pkg/containers"
	"github.com/The-Skyscape/devtools/pkg/database"
	"github.com/pkg/errors"
)

// OllamaConfig holds configuration for the Ollama service
type OllamaConfig struct {
	Port          int
	ContainerName string
	DataDir       string
	DefaultModel  string
	GPUEnabled    bool
}

// OllamaService manages the Ollama container for AI models
type OllamaService struct {
	config  *OllamaConfig
	service *containers.Service
	client  *http.Client
	mu      sync.RWMutex
}

// OllamaStatus represents the current status of the Ollama service
type OllamaStatus struct {
	Running      bool
	Port         int
	Health       string
	Models       []string
	DefaultModel string
}

// OllamaMessage represents a chat message
type OllamaMessage struct {
	Role    string `json:"role"`    // "user", "assistant", "system"
	Content string `json:"content"`
}

// OllamaChatRequest represents a chat completion request
type OllamaChatRequest struct {
	Model    string           `json:"model"`
	Messages []OllamaMessage  `json:"messages"`
	Stream   bool             `json:"stream"`
	Options  map[string]interface{} `json:"options,omitempty"`
}

// OllamaChatResponse represents a chat completion response
type OllamaChatResponse struct {
	Model     string         `json:"model"`
	CreatedAt string         `json:"created_at"`
	Message   OllamaMessage  `json:"message"`
	Done      bool          `json:"done"`
	TotalDuration   int64   `json:"total_duration,omitempty"`
	LoadDuration    int64   `json:"load_duration,omitempty"`
	PromptEvalCount int     `json:"prompt_eval_count,omitempty"`
	EvalCount       int     `json:"eval_count,omitempty"`
	EvalDuration    int64   `json:"eval_duration,omitempty"`
}

// OllamaModelInfo represents model information
type OllamaModelInfo struct {
	Name       string    `json:"name"`
	ModifiedAt time.Time `json:"modified_at"`
	Size       int64     `json:"size"`
}

var (
	// Ollama is the global Ollama service instance
	Ollama = NewOllamaService()
)

// NewOllamaService creates a new Ollama service with default configuration
func NewOllamaService() *OllamaService {
	return &OllamaService{
		config: &OllamaConfig{
			Port:          11434,
			ContainerName: "skyscape-ollama",
			DataDir:       fmt.Sprintf("%s/ollama", database.DataDir()),
			DefaultModel:  "gpt-oss:20b",         // GPT-OSS 20B model with native tool calling
			GPUEnabled:    false,                 // CPU mode by default
		},
		client: &http.Client{
			Timeout: 5 * time.Minute, // Increased timeout for model loading
		},
	}
}

// Init initializes the Ollama service if not already running
func (o *OllamaService) Init() error {
	// Check if AI is enabled via environment variable
	aiEnabled := os.Getenv("AI_ENABLED")
	if aiEnabled != "true" {
		log.Println("OllamaService: AI features disabled (AI_ENABLED != true)")
		return nil
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	// Check if service already exists and is running
	existing := containers.Local().Service(o.config.ContainerName)
	if existing != nil && existing.IsRunning() {
		log.Println("OllamaService: Already running")
		o.service = existing
		
		// Pull default model if not already present
		go o.ensureDefaultModel()
		return nil
	}

	// Start the service asynchronously to prevent blocking
	go func() {
		log.Println("OllamaService: Starting initialization in background...")
		if err := o.startAsync(); err != nil {
			log.Printf("OllamaService: Failed to start: %v", err)
		}
	}()

	return nil
}

// startAsync starts the Ollama service asynchronously
func (o *OllamaService) startAsync() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	// Check again if already running (race condition prevention)
	if o.service != nil && o.service.IsRunning() {
		log.Println("OllamaService: Already running")
		return nil
	}

	log.Printf("OllamaService: Starting on port %d", o.config.Port)

	// Prepare data directory
	prepareScript := fmt.Sprintf(`
		mkdir -p %s
		chmod -R 777 %s
	`, o.config.DataDir, o.config.DataDir)

	host := containers.Local()
	if err := host.Exec("bash", "-c", prepareScript); err != nil {
		return errors.Wrap(err, "failed to prepare Ollama directories")
	}

	// Create service configuration
	o.service = o.createServiceConfig()

	// Launch the service with progress tracking
	log.Println("OllamaService: Pulling Docker image (this may take a few minutes)...")
	if err := containers.Launch(host, o.service); err != nil {
		return errors.Wrap(err, "failed to launch Ollama service")
	}

	// Wait for service to be ready
	log.Println("OllamaService: Waiting for service to be ready...")
	if err := o.service.WaitForReady(60*time.Second, o.healthCheck); err != nil {
		log.Printf("OllamaService: Warning - service may not be fully ready: %v", err)
		// Still try to pull model, it might work
	}

	log.Println("OllamaService: Container started, initializing models...")
	
	// Pull default model in background with retry logic
	go o.ensureDefaultModel()
	
	return nil
}

// Start launches the Ollama service
func (o *OllamaService) Start() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	
	return o.start()
}

// start is the internal start method (must be called with lock held)
func (o *OllamaService) start() error {
	// Check if already running
	if o.service != nil && o.service.IsRunning() {
		log.Println("OllamaService: Already running")
		return nil
	}

	log.Printf("OllamaService: Starting on port %d", o.config.Port)

	// Prepare directories
	if o.config.DataDir == "" {
		o.config.DataDir = fmt.Sprintf("%s/ollama", database.DataDir())
	}

	prepareScript := fmt.Sprintf(`
		mkdir -p %s
		chmod -R 777 %s
	`, o.config.DataDir, o.config.DataDir)

	host := containers.Local()
	if err := host.Exec("bash", "-c", prepareScript); err != nil {
		return errors.Wrap(err, "failed to prepare Ollama directories")
	}

	// Create service configuration
	o.service = o.createServiceConfig()

	// Launch the service
	if err := containers.Launch(host, o.service); err != nil {
		return errors.Wrap(err, "failed to launch Ollama service")
	}

	// Wait for service to be ready
	if err := o.service.WaitForReady(60*time.Second, o.healthCheck); err != nil {
		log.Printf("OllamaService: Warning - service may not be fully ready: %v", err)
		// Still try to pull model, it might work
	}

	log.Println("OllamaService: Container started, initializing models...")
	
	// Pull default model in background with retry logic
	go o.ensureDefaultModel()
	
	return nil
}

// Stop stops the Ollama service
func (o *OllamaService) Stop() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.service == nil {
		log.Println("OllamaService: Not initialized")
		return nil
	}

	if !o.service.IsRunning() {
		log.Println("OllamaService: Not running")
		return nil
	}

	if err := o.service.Stop(); err != nil {
		return errors.Wrap(err, "failed to stop Ollama service")
	}

	log.Println("OllamaService: Stopped")
	return nil
}

// Restart restarts the Ollama service
func (o *OllamaService) Restart() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.service == nil {
		return errors.New("Ollama service not initialized")
	}

	if err := o.service.Restart(); err != nil {
		return errors.Wrap(err, "failed to restart Ollama service")
	}

	// Wait for service to be ready after restart
	if err := o.service.WaitForReady(60*time.Second, o.healthCheck); err != nil {
		log.Printf("Warning: Ollama service may not be fully ready after restart: %v", err)
	}

	log.Println("OllamaService: Restarted")
	return nil
}

// IsRunning checks if the service is running
func (o *OllamaService) IsRunning() bool {
	// Check if AI is enabled
	if os.Getenv("AI_ENABLED") != "true" {
		return false
	}

	o.mu.RLock()
	defer o.mu.RUnlock()

	if o.service == nil {
		// Try to get existing service
		existing := containers.Local().Service(o.config.ContainerName)
		if existing != nil {
			o.service = existing
		}
	}

	return o.service != nil && o.service.IsRunning()
}

// IsConfigured checks if Ollama is ready to use
func (o *OllamaService) IsConfigured() bool {
	// Check if AI is enabled
	if os.Getenv("AI_ENABLED") != "true" {
		return false
	}
	return o.IsRunning()
}

// GetStatus returns the current status of the Ollama service
func (o *OllamaService) GetStatus() *OllamaStatus {
	o.mu.RLock()
	defer o.mu.RUnlock()

	status := &OllamaStatus{
		Running:      o.IsRunning(),
		Port:         o.config.Port,
		Health:       "unknown",
		DefaultModel: o.config.DefaultModel,
		Models:       []string{},
	}

	if status.Running {
		if err := o.healthCheck(); err == nil {
			status.Health = "healthy"
			// Try to get installed models
			if models, err := o.ListModels(); err == nil {
				status.Models = models
			}
		} else {
			status.Health = "unhealthy"
		}
	} else {
		status.Health = "stopped"
	}

	return status
}

// createServiceConfig creates the container service configuration
func (o *OllamaService) createServiceConfig() *containers.Service {
	if o.config.DataDir == "" {
		o.config.DataDir = fmt.Sprintf("%s/ollama", database.DataDir())
	}

	return &containers.Service{
		Host:          containers.Local(),
		Name:          o.config.ContainerName,
		Image:         "ollama/ollama:latest",
		Network:       "host",
		RestartPolicy: "always",
		Mounts: map[string]string{
			o.config.DataDir: "/root/.ollama",
		},
		Env: map[string]string{
			"OLLAMA_HOST": fmt.Sprintf("0.0.0.0:%d", o.config.Port),
		},
		// Note: GPU support would need to be added via Docker runtime flags
		// For now, CPU-only mode is sufficient for development
	}
}

// healthCheck performs a health check on the service
func (o *OllamaService) healthCheck() error {
	resp, err := o.httpRequest("GET", "/api/tags", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unhealthy response: %d", resp.StatusCode)
	}

	return nil
}

// httpRequest makes an HTTP request to the Ollama service
func (o *OllamaService) httpRequest(method, path string, body io.Reader) (*http.Response, error) {
	if !o.IsRunning() {
		return nil, errors.New("Ollama service is not running")
	}

	url := fmt.Sprintf("http://localhost:%d%s", o.config.Port, path)
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create request")
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return o.client.Do(req)
}

// ListModels returns a list of installed models
func (o *OllamaService) ListModels() ([]string, error) {
	resp, err := o.httpRequest("GET", "/api/tags", nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list models")
	}
	defer resp.Body.Close()

	var result struct {
		Models []OllamaModelInfo `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, errors.Wrap(err, "failed to decode response")
	}

	models := make([]string, len(result.Models))
	for i, model := range result.Models {
		models[i] = model.Name
	}

	return models, nil
}

// PullModel pulls a model from the Ollama registry with streaming progress
func (o *OllamaService) PullModel(modelName string) error {
	log.Printf("OllamaService: Pulling model %s...", modelName)
	
	payload := map[string]interface{}{
		"name": modelName,
		"stream": true,
	}
	
	body, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "failed to marshal request")
	}

	resp, err := o.httpRequest("POST", "/api/pull", bytes.NewReader(body))
	if err != nil {
		return errors.Wrap(err, "failed to pull model")
	}
	defer resp.Body.Close()

	// IMPORTANT: Must read streaming response to completion
	// Otherwise the pull will hang/fail
	decoder := json.NewDecoder(resp.Body)
	var lastStatus string
	progressCount := 0
	
	for {
		var status map[string]interface{}
		if err := decoder.Decode(&status); err != nil {
			if err == io.EOF {
				break
			}
			// Don't fail on decode errors, just log and continue
			log.Printf("OllamaService: Warning - decode error: %v", err)
			continue
		}
		
		// Log progress periodically to avoid spam
		if statusMsg, ok := status["status"].(string); ok {
			if statusMsg != lastStatus {
				log.Printf("OllamaService: %s", statusMsg)
				lastStatus = statusMsg
				progressCount = 0
			} else {
				progressCount++
				// Show dots for same status to indicate progress
				if progressCount%10 == 0 {
					log.Printf("OllamaService: ... still %s", statusMsg)
				}
			}
		}
		
		// Check for completion
		if completed, ok := status["completed"].(bool); ok && completed {
			log.Printf("OllamaService: Model %s pull completed", modelName)
			break
		}
		
		// Check for errors in response
		if errMsg, ok := status["error"].(string); ok && errMsg != "" {
			return fmt.Errorf("pull failed: %s", errMsg)
		}
	}

	log.Printf("OllamaService: Model %s pulled successfully", modelName)
	return nil
}

// RemoveModel removes a model
func (o *OllamaService) RemoveModel(modelName string) error {
	payload := map[string]string{
		"name": modelName,
	}
	
	body, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "failed to marshal request")
	}

	resp, err := o.httpRequest("DELETE", "/api/delete", bytes.NewReader(body))
	if err != nil {
		return errors.Wrap(err, "failed to remove model")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to remove model: status %d", resp.StatusCode)
	}

	log.Printf("OllamaService: Model %s removed", modelName)
	return nil
}

// Chat sends a chat request to Ollama
func (o *OllamaService) Chat(modelName string, messages []OllamaMessage, stream bool) (*OllamaChatResponse, error) {
	if modelName == "" {
		modelName = o.config.DefaultModel
	}

	request := OllamaChatRequest{
		Model:    modelName,
		Messages: messages,
		Stream:   stream,
		Options: map[string]interface{}{
			"temperature":      0.7,
			"top_p":           0.9,
			"reasoning_effort": "medium", // GPT-OSS specific: low/medium/high
			"num_ctx":         128000,    // GPT-OSS supports 128K context
		},
	}

	body, err := json.Marshal(request)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal request")
	}

	resp, err := o.httpRequest("POST", "/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, errors.Wrap(err, "failed to send chat request")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("chat request failed: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var response OllamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, errors.Wrap(err, "failed to decode response")
	}

	return &response, nil
}

// StreamChat sends a streaming chat request to Ollama
func (o *OllamaService) StreamChat(modelName string, messages []OllamaMessage, callback func(chunk *OllamaChatResponse) error) error {
	if modelName == "" {
		modelName = o.config.DefaultModel
	}

	request := OllamaChatRequest{
		Model:    modelName,
		Messages: messages,
		Stream:   true,
		Options: map[string]interface{}{
			"temperature":      0.7,
			"top_p":           0.9,
			"reasoning_effort": "medium", // GPT-OSS specific: low/medium/high
			"num_ctx":         128000,    // GPT-OSS supports 128K context
		},
	}

	body, err := json.Marshal(request)
	if err != nil {
		return errors.Wrap(err, "failed to marshal request")
	}

	resp, err := o.httpRequest("POST", "/api/chat", bytes.NewReader(body))
	if err != nil {
		return errors.Wrap(err, "failed to send chat request")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("chat request failed: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	// Read streaming response
	decoder := json.NewDecoder(resp.Body)
	for {
		var chunk OllamaChatResponse
		if err := decoder.Decode(&chunk); err != nil {
			if err == io.EOF {
				break
			}
			return errors.Wrap(err, "failed to decode streaming response")
		}

		if err := callback(&chunk); err != nil {
			return errors.Wrap(err, "callback failed")
		}

		if chunk.Done {
			break
		}
	}

	return nil
}

// ensureDefaultModel ensures the default model is pulled
func (o *OllamaService) ensureDefaultModel() {
	// Wait for service to be fully ready
	time.Sleep(10 * time.Second)
	
	// Check health first
	if err := o.healthCheck(); err != nil {
		log.Printf("OllamaService: Service not ready yet, will retry model pull later: %v", err)
		// Retry in background
		go func() {
			time.Sleep(30 * time.Second)
			o.ensureDefaultModel()
		}()
		return
	}
	
	models, err := o.ListModels()
	if err != nil {
		log.Printf("OllamaService: Failed to list models: %v", err)
		// Retry later
		go func() {
			time.Sleep(30 * time.Second)
			o.ensureDefaultModel()
		}()
		return
	}

	// Check if default model is already installed
	hasDefault := false
	modelBase := strings.Split(o.config.DefaultModel, ":")[0]
	for _, model := range models {
		if strings.HasPrefix(model, modelBase) {
			hasDefault = true
			log.Printf("OllamaService: Found existing model %s", model)
			break
		}
	}

	if !hasDefault {
		log.Printf("OllamaService: Default model %s not found, pulling now...", o.config.DefaultModel)
		if err := o.PullModel(o.config.DefaultModel); err != nil {
			log.Printf("OllamaService: Failed to pull default model %s: %v", o.config.DefaultModel, err)
			// Try a smaller GPT-OSS variant or compatible model as fallback
			fallbackModel := "gpt-oss:latest" // Try the default/latest tag
			log.Printf("OllamaService: Trying fallback model %s...", fallbackModel)
			if err := o.PullModel(fallbackModel); err != nil {
				// If GPT-OSS fails completely, fall back to a smaller coding model
				lastResortModel := "qwen2.5-coder:1.5b"
				log.Printf("OllamaService: Trying last resort model %s...", lastResortModel)
				if err := o.PullModel(lastResortModel); err != nil {
					log.Printf("OllamaService: Failed to pull any models: %v", err)
				}
			}
		}
	} else {
		log.Printf("OllamaService: Model %s is already available", o.config.DefaultModel)
	}
}

// GetServiceInfo returns information about the Ollama service
func (o *OllamaService) GetServiceInfo() map[string]interface{} {
	o.mu.RLock()
	defer o.mu.RUnlock()
	
	info := map[string]interface{}{
		"configured":   o.IsConfigured(),
		"running":      o.IsRunning(),
		"port":         o.config.Port,
		"default_model": o.config.DefaultModel,
		"gpu_enabled":  o.config.GPUEnabled,
	}
	
	if o.IsRunning() {
		if models, err := o.ListModels(); err == nil {
			info["models"] = models
		}
	}
	
	return info
}