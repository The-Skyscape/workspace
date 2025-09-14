package models

import (
	"fmt"
	"strings"
	"testing"
	"time"
	
	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/database"
	"github.com/The-Skyscape/devtools/pkg/database/engines/sqlite3"
	"github.com/The-Skyscape/devtools/pkg/testutils"
)

func init() {
	// Don't override global DB in init - let each test create its own
	// This ensures test isolation
}

// initializeCollections initializes all model collections
func initializeCollections() {
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
}

// Global test workspace for the current test
var testWorkspace *testutils.TestWorkspace

// SetupTestDB creates a test database and initializes all models
func SetupTestDB(t *testing.T) *database.DynamicDB {
	// Create a fresh in-memory database for this test
	testDB := sqlite3.Open(":memory:", nil).Dynamic()
	
	// Save current global state
	oldDB := DB
	oldAuth := Auth
	oldUsers := Users
	
	// Override globals for this test
	DB = testDB
	Auth = authentication.Manage(DB)
	Users = Auth.Users
	
	// Re-initialize all collections with test database
	initializeCollections()
	
	// Initialize system tags
	if err := CreateSystemTags(); err != nil {
		t.Logf("Warning: Failed to create system tags: %v", err)
	}
	
	// Register cleanup to restore globals
	t.Cleanup(func() {
		DB = oldDB
		Auth = oldAuth
		Users = oldUsers
		initializeCollections()
	})
	
	return testDB
}

// CleanupTestDB cleans up the test database
func CleanupTestDB(t *testing.T, db *database.DynamicDB) {
	// Nothing to clean up for in-memory database
	// It will be garbage collected when test ends
}

// CreateTestUser creates a test user
func CreateTestUser(t *testing.T, db *database.DynamicDB, email string) *authentication.User {
	// Extract handle from email (e.g., "user@example.com" -> "user")
	handle := email
	if idx := strings.Index(email, "@"); idx > 0 {
		handle = email[:idx]
	}
	
	// Make handle unique by appending timestamp if needed
	handle = fmt.Sprintf("%s-%d", handle, time.Now().UnixNano())
	
	user := &authentication.User{
		Email:  email,
		Handle: handle,
		Name:   handle,
	}
	
	created, err := Users.Insert(user)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	
	return created
}

// GetTestDataDir returns the test data directory
func GetTestDataDir() string {
	if testWorkspace != nil {
		return testWorkspace.DataDir
	}
	return ""
}

// createTestRepository creates a test repository for testing
func createTestRepository(t *testing.T, name string, userID string) *Repository {
	t.Helper()
	
	repo := &Repository{
		Name:        name,
		Description: "Test repository",
		UserID:      userID,
		Visibility:  "public",
	}
	
	// Insert into database
	created, err := Repositories.Insert(repo)
	if err != nil {
		t.Fatalf("Failed to create test repository: %v", err)
	}
	
	return created
}