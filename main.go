package main

import (
	"cmp"
	"embed"
	"os"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"

	"workspace/controllers"
	"workspace/internal/ai"
	"workspace/internal/backup"
	"workspace/internal/middleware"
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
	
	// Initialize backup scheduler
	backup.InitializeBackupScheduler()

	// Configure rate limiting for production environment
	rateLimitConfig := &middleware.RateLimitConfig{
		// API endpoints: 60 requests per minute
		APIRate:   60,
		APIWindow: time.Minute,
		
		// AI endpoints: 10 requests per minute (expensive)
		AIRate:    10,
		AIWindow:  time.Minute,
		
		// Auth endpoints: 5 requests per minute (prevent brute force)
		AuthRate:   5,
		AuthWindow: time.Minute,
		
		// Search: 30 requests per minute
		SearchRate:   30,
		SearchWindow: time.Minute,
		
		// General: 120 requests per minute
		GeneralRate:   120,
		GeneralWindow: time.Minute,
	}

	// Create rate limiters
	limiters := middleware.CreateRateLimiters(rateLimitConfig)
	routeLimiter := middleware.NewRouteRateLimiter(limiters)

	// Start application immediately
	application.Serve(views,
		application.WithMiddleware(routeLimiter),
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
		application.WithController(controllers.Backup()),
		application.WithHostPrefix(cmp.Or(os.Getenv("PREFIX"), "")),
		application.WithDaisyTheme(theme),
	)
}
