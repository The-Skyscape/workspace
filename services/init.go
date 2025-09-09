package services

import (
	"log"
	"os"
	"strings"
)

// init automatically starts required services during package initialization.
// This ensures services are ready when the application starts, not when controllers setup.
// Services start in background goroutines to prevent blocking the main startup.
func init() {
	// Skip initialization during tests
	if strings.HasSuffix(os.Args[0], ".test") {
		log.Println("Services: Skipping initialization during tests")
		return
	}

	log.Println("Services: Starting service initialization...")

	// Initialize Ollama service if AI is enabled
	aiEnabled := os.Getenv("AI_ENABLED") == "true"
	if aiEnabled {
		go func() {
			log.Println("Services: Initializing Ollama service (AI_ENABLED=true)...")
			if err := Ollama.Init(); err != nil {
				log.Printf("Services: Warning - Ollama service initialization failed: %v", err)
				// Continue anyway - the application can run without AI
			} else {
				log.Println("Services: Ollama service initialization started successfully")
			}
		}()
	} else {
		log.Println("Services: Ollama service disabled (AI_ENABLED != true)")
	}

	// Note: Vault is managed through models.Secrets using devtools security package
	// Note: Actions service starts on-demand when actions are run
	// Note: Notebook service starts on-demand when notebooks are accessed
	// Note: Coder proxy is handled differently (not a container service in workspace)

	log.Println("Services: Service initialization scheduled")
}