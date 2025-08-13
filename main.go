package main

import (
	"cmp"
	"embed"
	"os"

	"github.com/The-Skyscape/devtools/pkg/application"

	"workspace/controllers"
	"workspace/models"
)

//go:embed all:views
var views embed.FS

func main() {
	// Start application immediately
	application.Serve(views,
		application.WithController("auth", models.Auth.Controller()),
		application.WithController(controllers.Home()),
		application.WithController(controllers.Repos()),
		application.WithController(controllers.Workspaces()),
		application.WithController(controllers.Public()),
		application.WithController(controllers.Settings()),
		application.WithController(controllers.Monitoring()),
		application.WithHostPrefix(cmp.Or(os.Getenv("PREFIX"), "")),
		application.WithDaisyTheme(cmp.Or(os.Getenv("THEME"), "corporate")),
	)
}
