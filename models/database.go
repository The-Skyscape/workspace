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
	Repositories = database.Manage(DB, new(Repository))
	AccessTokens = database.Manage(DB, new(AccessToken))

	// Application-specific collections
	Issues          = database.Manage(DB, new(Issue))
	PullRequests    = database.Manage(DB, new(PullRequest))
	Comments        = database.Manage(DB, new(Comment))
	Permissions     = database.Manage(DB, new(Permission))
	Actions         = database.Manage(DB, new(Action))
	ActionRuns      = database.Manage(DB, new(ActionRun))
	ActionArtifacts = database.Manage(DB, new(ActionArtifact))
	Activities      = database.Manage(DB, new(Activity))
	
	// Global settings
	GlobalSettings = database.Manage(DB, new(Settings))
)

func init() {
	// Initialize FTS5 search
	// Note: FTS5 initialization happens separately when needed
	// since we need the underlying sql.DB, not the DynamicDB wrapper
}