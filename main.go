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
		application.WithHostPrefix(cmp.Or(os.Getenv("PREFIX"), "")),
		application.WithDaisyTheme(cmp.Or(os.Getenv("THEME"), "corporate")),
	)
}

// startServices initializes global services
func startServices() {
	log.Println("Initializing global services...")
	
	// Start coder service if ENABLE_CODER is set
	if os.Getenv("ENABLE_CODER") == "true" {
		log.Println("Starting global coder service...")
		if err := services.Coder.Start(); err != nil {
			log.Printf("Warning: Failed to start coder service: %v", err)
			// Don't fail the application if coder fails to start
		}
	} else {
		log.Println("Coder service disabled (set ENABLE_CODER=true to enable)")
	}
}
