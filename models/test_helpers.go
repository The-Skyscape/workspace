package models

import (
	"os"
	"testing"
	
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/database"
	"github.com/The-Skyscape/devtools/pkg/testutils"
)

// Global test workspace for the current test
var testWorkspace *testutils.TestWorkspace

// SetupTestDB creates a test database and initializes all models
func SetupTestDB(t *testing.T) *database.DynamicDB {
	// Create test workspace with temp directories
	testWorkspace = testutils.SetupTestWorkspace(t)
	
	// Set DataDir environment variable for any code that needs it
	os.Setenv("DATADIR", testWorkspace.DataDir)
	
	// Re-initialize all model collections with test database
	DB = testWorkspace.DB
	Auth = testWorkspace.Auth
	Users = Auth.Users
	
	// Initialize all collections
	Repositories = database.Manage(DB, new(Repository))
	Repos = Repositories
	AccessTokens = database.Manage(DB, new(AccessToken))
	Issues = database.Manage(DB, new(Issue))
	IssueTags = database.Manage(DB, new(IssueTag))
	PullRequests = database.Manage(DB, new(PullRequest))
	Comments = database.Manage(DB, new(Comment))
	Actions = database.Manage(DB, new(Action))
	ActionRuns = database.Manage(DB, new(ActionRun))
	ActionArtifacts = database.Manage(DB, new(ActionArtifact))
	Activities = database.Manage(DB, new(Activity))
	GlobalSettings = database.Manage(DB, new(Settings))
	Profiles = database.Manage(DB, new(Profile))
	SSHKeys = database.Manage(DB, new(SSHKey))
	GitHubUsers = database.Manage(DB, new(UserGitHub))
	Conversations = database.Manage(DB, new(Conversation))
	Messages = database.Manage(DB, new(Message))
	Todos = database.Manage(DB, new(Todo))
	AIActivities = database.Manage(DB, new(AIActivity))
	TagDefinitions = database.Manage(DB, new(TagDefinition))
	IssueLabels = database.Manage(DB, new(IssueLabel))
	Events = database.Manage(DB, new(Event))
	EventMetadataEntries = database.Manage(DB, new(EventMetadata))
	
	// Initialize system tags
	if err := CreateSystemTags(); err != nil {
		t.Logf("Warning: Failed to create system tags: %v", err)
	}
	
	return testWorkspace.DB
}

// CleanupTestDB cleans up the test database
func CleanupTestDB(t *testing.T, db *database.DynamicDB) {
	if testWorkspace != nil {
		testWorkspace.Cleanup()
		testWorkspace = nil
	}
}

// CreateTestUser creates a test user
func CreateTestUser(t *testing.T, db *database.DynamicDB, email string) *authentication.User {
	if testWorkspace == nil {
		t.Fatal("Test workspace not initialized. Call SetupTestDB first.")
	}
	return testWorkspace.CreateTestUser(email)
}

// GetTestDataDir returns the test data directory
func GetTestDataDir() string {
	if testWorkspace != nil {
		return testWorkspace.DataDir
	}
	return ""
}