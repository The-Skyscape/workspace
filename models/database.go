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
	Actions         = database.Manage(DB, new(Action))
	ActionRuns      = database.Manage(DB, new(ActionRun))
	ActionArtifacts = database.Manage(DB, new(ActionArtifact))
	Activities      = database.Manage(DB, new(Activity))
	
	// Global settings
	GlobalSettings = database.Manage(DB, new(Settings))
	
	// User profiles
	Profiles = database.Manage(DB, new(Profile))
	
	// GitHub integration
	GitHubUsers = database.Manage(DB, new(UserGitHub))
	
	// AI Workers
	AIWorkers      = database.Manage(DB, new(AIWorker))
	AIWorkerRepos  = database.Manage(DB, new(AIWorkerRepo))
	ChatMessages   = database.Manage(DB, new(ChatMessage))
	
	// Worker usage tracking
	WorkerUsages   = database.Manage(DB, new(WorkerUsage))
)

