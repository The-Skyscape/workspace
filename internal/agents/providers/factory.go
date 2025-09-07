package providers

import (
	"fmt"
	"log"
	"os"
	"workspace/internal/agents"
	"workspace/services"
)

// NewProvider creates a provider based on the AI_MODEL environment variable
func NewProvider() (agents.Provider, error) {
	// First check if AI is enabled
	aiEnabled := os.Getenv("AI_ENABLED")
	if aiEnabled != "true" {
		return nil, fmt.Errorf("AI features are disabled (AI_ENABLED != true)")
	}
	
	modelName := os.Getenv("AI_MODEL")
	if modelName == "" {
		modelName = "llama3.2:1b" // Default to small model
		log.Printf("AI_MODEL not set, defaulting to %s", modelName)
	}
	
	// Ensure Ollama service is available
	if !services.Ollama.IsRunning() {
		return nil, fmt.Errorf("Ollama service is not running")
	}
	
	switch modelName {
	case "llama3.2:1b":
		log.Printf("Initializing Llama 3.2:1b provider for CPU environment")
		return NewLlama32Provider(services.Ollama), nil
		
	case "gpt-oss":
		log.Printf("Initializing GPT-OSS provider for GPU environment")
		return NewGPTOSSProvider(services.Ollama), nil
		
	default:
		return nil, fmt.Errorf("unsupported model: %s (supported: llama3.2:1b, gpt-oss)", modelName)
	}
}

// GetProviderForModel returns a provider for a specific model name
// This is useful for testing or when you need to override the environment
func GetProviderForModel(modelName string) (agents.Provider, error) {
	// Ensure Ollama service is available
	if !services.Ollama.IsRunning() {
		return nil, fmt.Errorf("Ollama service is not running")
	}
	
	switch modelName {
	case "llama3.2:1b":
		return NewLlama32Provider(services.Ollama), nil
		
	case "gpt-oss":
		return NewGPTOSSProvider(services.Ollama), nil
		
	default:
		return nil, fmt.Errorf("unsupported model: %s", modelName)
	}
}