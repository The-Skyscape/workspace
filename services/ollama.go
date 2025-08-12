package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/The-Skyscape/devtools/pkg/containers"
	"github.com/The-Skyscape/devtools/pkg/database"
	"github.com/pkg/errors"
)

// OllamaService manages the Ollama AI service
type OllamaService struct {
	host    containers.Host
	port    int
	running bool
}

var (
	// Global Ollama instance
	Ollama = &OllamaService{
		host: containers.Local(),
		port: 11434, // Standard Ollama port
	}
)

// Start launches the Ollama service
func (o *OllamaService) Start() error {
	if o.running {
		log.Println("Ollama service already running")
		return nil
	}

	log.Println("Starting Ollama service on port", o.port)

	// Prepare directories for models and data
	dataDir := fmt.Sprintf("%s/services/ollama", database.DataDir())
	modelsDir := fmt.Sprintf("%s/models", dataDir)

	// Create directories
	prepareScript := fmt.Sprintf(`
		mkdir -p %s
		chmod -R 777 %s
	`, modelsDir, dataDir)

	if err := o.host.Exec("bash", "-c", prepareScript); err != nil {
		return errors.Wrap(err, "failed to prepare Ollama directories")
	}

	// Create service configuration
	service := &containers.Service{
		Host:          o.host,
		Name:          "skyscape-ollama",
		Image:         "ollama/ollama:latest",
		Network:       "host", // Use host network for easy access
		RestartPolicy: "always",
		Mounts: map[string]string{
			modelsDir: "/root/.ollama/models", // Persist downloaded models
		},
		Env: map[string]string{
			"OLLAMA_HOST":       fmt.Sprintf("0.0.0.0:%d", o.port),
			"OLLAMA_ORIGINS":    "*", // Allow all origins for internal use
			"OLLAMA_KEEP_ALIVE": "5m",
		},
	}

	// Launch the service
	if err := containers.Launch(o.host, service); err != nil {
		return errors.Wrap(err, "failed to launch Ollama service")
	}

	o.running = true
	log.Println("Ollama service started successfully")

	// Wait for service to be ready
	if err := o.WaitForReady(30 * time.Second); err != nil {
		log.Printf("Warning: Ollama service may not be fully ready: %v", err)
	}

	// Pull default models
	go o.pullDefaultModels()

	return nil
}

// Stop stops the Ollama service
func (o *OllamaService) Stop() error {
	if !o.running {
		log.Println("Ollama service not running")
		return nil
	}

	service := o.getService()
	if err := service.Stop(); err != nil {
		return errors.Wrap(err, "failed to stop Ollama service")
	}

	o.running = false
	log.Println("Ollama service stopped")
	return nil
}

// IsRunning checks if the service is running
func (o *OllamaService) IsRunning() bool {
	service := o.getService()
	return service.IsRunning()
}

// GetPort returns the port the service is running on
func (o *OllamaService) GetPort() int {
	return o.port
}

// getService returns the container service configuration
func (o *OllamaService) getService() *containers.Service {
	return &containers.Service{
		Host: o.host,
		Name: "skyscape-ollama",
	}
}

// HTTPRequest makes an HTTP request to the Ollama service
func (o *OllamaService) HTTPRequest(method, path string, body io.Reader, timeout time.Duration) (*http.Response, error) {
	if !o.IsRunning() {
		return nil, errors.New("Ollama service is not running")
	}

	url := fmt.Sprintf("http://localhost:%d%s", o.port, path)
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create request")
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: timeout}
	return client.Do(req)
}

// HealthCheck performs a health check on the service
func (o *OllamaService) HealthCheck() error {
	resp, err := o.HTTPRequest("GET", "/", nil, 2*time.Second)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		return nil
	}

	return fmt.Errorf("unhealthy response: %d", resp.StatusCode)
}

// WaitForReady waits for the Ollama service to be ready
func (o *OllamaService) WaitForReady(timeout time.Duration) error {
	start := time.Now()
	for {
		if o.IsRunning() {
			// Check if the service is actually responding
			if err := o.HealthCheck(); err == nil {
				return nil
			}

			// Keep trying for the timeout duration
			if time.Since(start) < timeout {
				time.Sleep(1 * time.Second)
				continue
			}

			return errors.New("timeout waiting for Ollama service to be ready")
		}

		if time.Since(start) > timeout {
			return errors.New("Ollama service is not running")
		}

		time.Sleep(1 * time.Second)
	}
}

// Init initializes the Ollama service if not already running
func (o *OllamaService) Init() error {
	// Check if service already exists and is running
	if o.IsRunning() {
		log.Println("Ollama service already running")
		o.running = true
		return nil
	}

	// Start the service
	log.Println("Initializing Ollama service...")
	if err := o.Start(); err != nil {
		return errors.Wrap(err, "failed to initialize Ollama service")
	}

	return nil
}

// pullDefaultModels pulls the default models for the service
func (o *OllamaService) pullDefaultModels() {
	models := []string{
		// Existing models
		"llama3.2:3b",     // Fast, efficient model for general tasks
		"codellama:7b",    // Specialized for code generation
		
		// High-performance OSS models
		"llama3.1:8b",     // Meta's latest, excellent for general tasks
		"gemma2:9b",       // Google's efficient model, good balance
		"qwen2.5:7b",      // Alibaba's multilingual model
		"phi3:mini",       // Microsoft's lightweight but capable
		
		// Note: GPT-OSS models (20b/120b) are large and require significant resources
		// Uncomment if you have sufficient hardware:
		// "gpt-oss:20b",  // OpenAI's OSS model with reasoning capabilities
	}

	for _, model := range models {
		log.Printf("Pulling Ollama model: %s", model)
		if err := o.PullModel(model); err != nil {
			log.Printf("Warning: Failed to pull model %s: %v", model, err)
		} else {
			log.Printf("Successfully pulled model: %s", model)
		}
	}
}

// PullModel pulls a specific model with streaming progress
func (o *OllamaService) PullModel(model string) error {
	reqBody := map[string]interface{}{
		"name":   model,
		"stream": true, // Enable streaming for progress updates
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return errors.Wrap(err, "failed to marshal request")
	}

	// Pull can take a long time, so use a longer timeout
	// Note: We use a very long timeout for large models
	resp, err := o.HTTPRequest("POST", "/api/pull", bytes.NewReader(body), 60*time.Minute)
	if err != nil {
		return errors.Wrap(err, "failed to pull model")
	}
	defer resp.Body.Close()

	// Read the streaming response
	decoder := json.NewDecoder(resp.Body)
	lastStatus := ""
	lastPercent := -1
	
	for {
		var result map[string]interface{}
		if err := decoder.Decode(&result); err != nil {
			if err == io.EOF {
				break
			}
			// Don't fail on decode errors, just log them
			log.Printf("Warning: decode error during pull (continuing): %v", err)
			continue
		}

		// Check for errors in the response
		if errMsg, ok := result["error"].(string); ok {
			return errors.New(errMsg)
		}

		// Extract and log progress
		status, _ := result["status"].(string)
		
		// Calculate percentage if total and completed are available
		if total, ok := result["total"].(float64); ok && total > 0 {
			if completed, ok := result["completed"].(float64); ok {
				percent := int((completed / total) * 100)
				// Only log if percentage changed significantly
				if percent != lastPercent && percent%10 == 0 {
					lastPercent = percent
					log.Printf("Model %s: %s (%d%%)", model, status, percent)
				}
			}
		} else if status != lastStatus && status != "" {
			// Log status changes
			lastStatus = status
			log.Printf("Model %s: %s", model, status)
		}
		
		// Check for success
		if status == "success" {
			log.Printf("Successfully pulled model: %s", model)
			return nil
		}
	}

	return nil
}

// ListModels lists available models
func (o *OllamaService) ListModels() ([]string, error) {
	resp, err := o.HTTPRequest("GET", "/api/tags", nil, 5*time.Second)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list models")
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
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

// Generate generates a completion using Ollama
func (o *OllamaService) Generate(prompt string, model string) (string, error) {
	if model == "" {
		model = "llama3.2:3b" // Default model
	}

	reqBody := map[string]interface{}{
		"model":  model,
		"prompt": prompt,
		"stream": false,
		"options": map[string]interface{}{
			"temperature": 0.7,
			"num_predict": 2048,
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal request")
	}

	resp, err := o.HTTPRequest("POST", "/api/generate", bytes.NewReader(body), 60*time.Second)
	if err != nil {
		return "", errors.Wrap(err, "failed to generate")
	}
	defer resp.Body.Close()

	var result struct {
		Response string `json:"response"`
		Error    string `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", errors.Wrap(err, "failed to decode response")
	}

	if result.Error != "" {
		return "", errors.New(result.Error)
	}

	return result.Response, nil
}

// Chat performs a chat completion with context
func (o *OllamaService) Chat(messages []map[string]string, model string) (string, error) {
	if model == "" {
		model = "llama3.2:3b"
	}

	reqBody := map[string]interface{}{
		"model":    model,
		"messages": messages,
		"stream":   false,
		"options": map[string]interface{}{
			"temperature": 0.7,
			"num_predict": 2048,
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal request")
	}

	resp, err := o.HTTPRequest("POST", "/api/chat", bytes.NewReader(body), 60*time.Second)
	if err != nil {
		return "", errors.Wrap(err, "failed to chat")
	}
	defer resp.Body.Close()

	var result struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Error string `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", errors.Wrap(err, "failed to decode response")
	}

	if result.Error != "" {
		return "", errors.New(result.Error)
	}

	return result.Message.Content, nil
}