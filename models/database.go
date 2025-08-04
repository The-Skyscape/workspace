package models

import (
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/database"
	"github.com/The-Skyscape/devtools/pkg/database/local"
)

var (
	// DB is the application's database
	DB = local.Database("workspace")

	// Auth is the DB's authentication collection
	Auth = authentication.Manage(DB)

	// Git-related collections
	GitRepos     = database.Manage(DB, new(GitRepo))
	Workspaces   = database.Manage(DB, new(Workspace))
	AccessTokens = database.Manage(DB, new(AccessToken))

	// Application-specific collections
	Issues       = database.Manage(DB, new(Issue))
	PullRequests = database.Manage(DB, new(PullRequest))
	Comments     = database.Manage(DB, new(Comment))
	Permissions  = database.Manage(DB, new(Permission))
	Actions      = database.Manage(DB, new(Action))
	Activities   = database.Manage(DB, new(Activity))
)