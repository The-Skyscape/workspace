package main

import (
	"cmp"
	"embed"
	"log"
	"os"

	"github.com/The-Skyscape/devtools/pkg/application"

	"workspace/controllers"
	"workspace/models"
	"workspace/services"
)

//go:embed all:views
var views embed.FS

func main() {
	// Start global services
	startServices()

	// Start application
	application.Serve(views,
		application.WithController("auth", models.Auth.Controller()),
		application.WithController(controllers.Home()),
		application.WithController(controllers.Repos()),
		application.WithController(controllers.Services()),
		application.WithController(controllers.Public()),
		application.WithController(controllers.Settings()),
		application.WithController(controllers.AI()),
		application.WithHostPrefix(cmp.Or(os.Getenv("PREFIX"), "")),
		application.WithDaisyTheme(cmp.Or(os.Getenv("THEME"), "corporate")),
	)
}

// startServices initializes global services
func startServices() {
	log.Println("Initializing global services...")

	// Initialize coder service (will check if already running)
	log.Println("Initializing coder service...")
	if err := services.Coder.Init(); err != nil {
		log.Printf("Warning: Failed to initialize coder service: %v", err)
		// Don't fail the application if coder fails to start
	}

	// Initialize Ollama AI service
	log.Println("Initializing Ollama AI service...")
	if err := services.Ollama.Init(); err != nil {
		log.Printf("Warning: Failed to initialize Ollama service: %v", err)
		// Don't fail the application if Ollama fails to start
	}
}
