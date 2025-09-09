package main

import (
	"cmp"
	"embed"
	"os"

	"github.com/The-Skyscape/devtools/pkg/application"

	"workspace/controllers"
	"workspace/internal/ai"
	"workspace/models"
)

//go:embed all:views
var views embed.FS

func main() {
	// Load theme from database settings, fallback to env or corporate
	settings, err := models.GetSettings()
	theme := "corporate"
	if err == nil && settings.DefaultTheme != "" {
		theme = settings.DefaultTheme
	} else if envTheme := os.Getenv("THEME"); envTheme != "" {
		theme = envTheme
	}

	// Initialize AI system if enabled
	ai.InitializeAISystem()

	// Start application immediately
	application.Serve(views,
		application.WithController(controllers.Auth()),       // Use custom auth controller
		application.WithController(controllers.Logs()),       // Add logs controller
		application.WithController(controllers.Home()),
		application.WithController(controllers.Repos()),
		application.WithController(controllers.Issues()),
		application.WithController(controllers.PullRequests()),
		application.WithController(controllers.Actions()),
		application.WithController(controllers.Integrations()),
		application.WithController(controllers.AI()),
		application.WithController(controllers.Workspaces()),
		application.WithController(controllers.Settings()),
		application.WithController(controllers.Monitoring()),
		application.WithController(controllers.Users()),
		application.WithController(controllers.Health()),
		application.WithHostPrefix(cmp.Or(os.Getenv("PREFIX"), "")),
		application.WithDaisyTheme(theme),
	)
}
