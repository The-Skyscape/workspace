package models

import (
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/database"
	"github.com/The-Skyscape/devtools/pkg/database/local"
)

// User is a type alias for the authentication user
type User = authentication.User

var (
	// DB is the application's database
	DB = local.Database("workspace")

	// Auth is the DB's authentication collection
	Auth = authentication.Manage(DB)
	
	// Users is a type alias for convenience in models
	Users = Auth.Users

	// Git-related collections
	Repositories = database.Manage(DB, new(Repository))
	Repos        = Repositories // Alias for convenience
	AccessTokens = database.Manage(DB, new(AccessToken))

	// Application-specific collections
	Issues          = database.Manage(DB, new(Issue))
	IssueTags       = database.Manage(DB, new(IssueTag)) // Deprecated: use IssueLabels
	PullRequests    = database.Manage(DB, new(PullRequest))
	Comments        = database.Manage(DB, new(Comment))
	Actions         = database.Manage(DB, new(Action))
	ActionRuns      = database.Manage(DB, new(ActionRun))
	ActionArtifacts = database.Manage(DB, new(ActionArtifact))
	Activities      = database.Manage(DB, new(Activity))
	
	// Normalized tag system
	TagDefinitions = database.Manage(DB, new(TagDefinition))
	IssueLabels    = database.Manage(DB, new(IssueLabel))
	
	// Event system
	Events               = database.Manage(DB, new(Event))
	EventMetadataEntries = database.Manage(DB, new(EventMetadata))
	
	// Global settings
	GlobalSettings = database.Manage(DB, new(Settings))
	
	// User profiles
	Profiles = database.Manage(DB, new(Profile))
	
	// SSH Keys for Git authentication
	SSHKeys = database.Manage(DB, new(SSHKey))
	
	// GitHub integration
	GitHubUsers = database.Manage(DB, new(UserGitHub))
	
	// AI Conversations
	Conversations = database.Manage(DB, new(Conversation))
	Messages      = database.Manage(DB, new(Message))
	Todos         = database.Manage(DB, new(Todo))
	AIActivities  = database.Manage(DB, new(AIActivity))
)

func init() {
	// Create database indexes for common queries
	createIndexes()
}

// createIndexes creates database indexes for common queries
func createIndexes() {
	// Repository-related indexes
	Repositories.Index("UserID")
	Issues.Index("RepoID")
	Issues.Index("Status")
	Issues.Index("Priority")
	Issues.Index("AssigneeID")
	PullRequests.Index("RepoID")
	PullRequests.Index("Status")
	PullRequests.Index("ReviewStatus")
	Comments.Index("IssueID")
	Comments.Index("PullRequestID")
	
	// Actions and activities
	Actions.Index("RepoID")
	ActionRuns.Index("ActionID")
	ActionRuns.Index("Status")
	Activities.Index("UserID")
	Activities.Index("RepoID")
	
	// AI-related indexes
	Conversations.Index("UserID")
	Messages.Index("ConversationID")
	AIActivities.Index("Status")
	AIActivities.Index("Priority")
	
	// Sorting indexes
	Issues.Index("CreatedAt")
	Issues.Index("UpdatedAt")
	PullRequests.Index("CreatedAt")
	Activities.Index("CreatedAt")
	Conversations.Index("UpdatedAt")
	
	// Composite indexes for complex queries
	Issues.Index("Status", "Priority") // For critical issue queries
	Issues.Index("RepoID", "Status")   // For repo-specific open issues
}

