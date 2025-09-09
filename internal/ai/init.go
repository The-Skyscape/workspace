// Package ai provides AI service initialization
package ai

import (
	"log"
	"os"
)

// InitializeAISystem initializes the AI system at startup
func InitializeAISystem() {
	// Check if AI is enabled
	aiEnabled := os.Getenv("AI_ENABLED") == "true"
	
	if !aiEnabled {
		log.Println("AI System: Disabled (AI_ENABLED != true)")
		return
	}
	
	// Initialize with default configuration
	config := DefaultConfig()
	config.Enabled = true
	
	// Initialize the AI service
	if err := Initialize(config); err != nil {
		log.Printf("AI System: Failed to initialize: %v", err)
		return
	}
	
	// Initialize the event queue for proactive AI
	if err := InitializeEventQueue(3); err != nil {
		log.Printf("AI Event Queue: Failed to initialize: %v", err)
		// Continue anyway - the chat-based AI will still work
	}
	
	log.Println("AI System: Initialized successfully")
}