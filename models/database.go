package models

import (
	"workspace/internal/coding"

	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/database"
	"github.com/The-Skyscape/devtools/pkg/database/local"
)

var (
	// DB is the application's database
	DB = local.Database("workspace")

	// Auth is the DB's authentication collection
	Auth = authentication.Manage(DB)

	// Coding manager for Git repos and workspaces (uses existing models)
	Coding = coding.Manage(DB)

	// Application-specific collections
	Todos        = database.Manage(DB, new(Todo))
	Issues       = database.Manage(DB, new(Issue))
	PullRequests = database.Manage(DB, new(PullRequest))
	Permissions  = database.Manage(DB, new(Permission))
	Actions      = database.Manage(DB, new(Action))
	Activities   = database.Manage(DB, new(Activity))
)