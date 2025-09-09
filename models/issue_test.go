package models

import (
	"testing"
	
	"github.com/The-Skyscape/devtools/pkg/testutils"
)

func TestIssueModel(t *testing.T) {
	// Setup test database
	db := SetupTestDB(t)
	defer CleanupTestDB(t, db)
	
	t.Run("CreateIssue", func(t *testing.T) {
		// Create test user and repo
		user := CreateTestUser(t, db, "test@example.com")
		repo := createTestRepository(t, "test-repo", user.ID)
		
		// Create issue
		issue, err := CreateIssue("Test Issue", "This is a test issue", user.ID, repo.ID)
		testutils.AssertNoError(t, err)
		testutils.AssertNotNil(t, issue)
		testutils.AssertEqual(t, "Test Issue", issue.Title)
		testutils.AssertEqual(t, "This is a test issue", issue.Body)
		testutils.AssertEqual(t, IssueStatusOpen, issue.Status)
		testutils.AssertEqual(t, "todo", issue.Column)
		testutils.AssertEqual(t, PriorityMedium, issue.Priority)
		
		// Verify event was created
		events, err := GetEntityEvents("issue", issue.ID, 10)
		testutils.AssertNoError(t, err)
		testutils.AssertEqual(t, 1, len(events))
		testutils.AssertEqual(t, EventIssueCreated, events[0].Type)
	})
	
	t.Run("IssueLabels", func(t *testing.T) {
		// Create test data
		user := CreateTestUser(t, db, "labels@example.com")
		repo := createTestRepository(t, "label-repo", user.ID)
		issue, err := CreateIssue("Labeled Issue", "Issue with labels", user.ID, repo.ID)
		testutils.AssertNoError(t, err)
		
		// Create tag definitions
		bugTag, err := GetOrCreateTag("bug", repo.ID)
		testutils.AssertNoError(t, err)
		testutils.AssertNotNil(t, bugTag)
		
		enhancementTag, err := GetOrCreateTag("enhancement", repo.ID)
		testutils.AssertNoError(t, err)
		testutils.AssertNotNil(t, enhancementTag)
		
		// Add labels to issue
		err = AddLabelToIssue(issue.ID, bugTag.ID, user.ID)
		testutils.AssertNoError(t, err)
		
		err = AddLabelToIssue(issue.ID, enhancementTag.ID, user.ID)
		testutils.AssertNoError(t, err)
		
		// Verify labels
		labels, err := issue.Labels()
		testutils.AssertNoError(t, err)
		testutils.AssertEqual(t, 2, len(labels))
		
		// Check HasLabel method
		testutils.AssertTrue(t, issue.HasLabel("bug"))
		testutils.AssertTrue(t, issue.HasLabel("enhancement"))
		testutils.AssertFalse(t, issue.HasLabel("security"))
		
		// Check LabelNames method
		labelNames := issue.LabelNames()
		testutils.AssertContains(t, labelNames, "bug")
		testutils.AssertContains(t, labelNames, "enhancement")
		
		// Remove a label
		err = RemoveLabelFromIssue(issue.ID, bugTag.ID)
		testutils.AssertNoError(t, err)
		
		labels, err = issue.Labels()
		testutils.AssertNoError(t, err)
		testutils.AssertEqual(t, 1, len(labels))
		testutils.AssertEqual(t, "enhancement", labels[0].Name)
	})
	
	t.Run("IssuePrioritySorting", func(t *testing.T) {
		user := CreateTestUser(t, db, "priority@example.com")
		repo := createTestRepository(t, "priority-repo", user.ID)
		
		// Create issues with different priorities
		critical, err := CreateIssue("Critical", "Critical issue", user.ID, repo.ID)
		testutils.AssertNoError(t, err)
		critical.Priority = PriorityCritical
		err = Issues.Update(critical)
		testutils.AssertNoError(t, err)
		
		low, err := CreateIssue("Low", "Low priority", user.ID, repo.ID)
		testutils.AssertNoError(t, err)
		low.Priority = PriorityLow
		err = Issues.Update(low)
		testutils.AssertNoError(t, err)
		
		high, err := CreateIssue("High", "High priority", user.ID, repo.ID)
		testutils.AssertNoError(t, err)
		high.Priority = PriorityHigh
		err = Issues.Update(high)
		testutils.AssertNoError(t, err)
		
		// Get issues sorted by priority
		issues, count, err := GetRepoIssuesPaginated(repo.ID, true, 10, 0)
		testutils.AssertNoError(t, err)
		testutils.AssertEqual(t, 3, count)
		
		// Verify priority ordering (1 is highest priority)
		testutils.AssertEqual(t, PriorityCritical, issues[0].Priority)
		testutils.AssertEqual(t, PriorityHigh, issues[1].Priority)
		testutils.AssertEqual(t, PriorityLow, issues[2].Priority)
	})
	
	t.Run("IssueStatusFiltering", func(t *testing.T) {
		user := CreateTestUser(t, db, "status@example.com")
		repo := createTestRepository(t, "status-repo", user.ID)
		
		// Create open and closed issues
		open1, err := CreateIssue("Open 1", "First open", user.ID, repo.ID)
		testutils.AssertNoError(t, err)
		
		closed, err := CreateIssue("Closed", "Closed issue", user.ID, repo.ID)
		testutils.AssertNoError(t, err)
		closed.Status = IssueStatusClosed
		err = Issues.Update(closed)
		testutils.AssertNoError(t, err)
		
		open2, err := CreateIssue("Open 2", "Second open", user.ID, repo.ID)
		testutils.AssertNoError(t, err)
		
		// Get only open issues
		openIssues, count, err := GetRepoIssuesPaginated(repo.ID, false, 10, 0)
		testutils.AssertNoError(t, err)
		testutils.AssertEqual(t, 2, count)
		
		// Get all issues
		_, count, err = GetRepoIssuesPaginated(repo.ID, true, 10, 0)
		testutils.AssertNoError(t, err)
		testutils.AssertEqual(t, 3, count)
		
		// Verify specific issues
		_ = open1
		_ = open2
		for _, issue := range openIssues {
			testutils.AssertEqual(t, IssueStatusOpen, issue.Status)
		}
	})
}

// Helper function to create test repository
func createTestRepository(t *testing.T, name string, userID string) *Repository {
	repo := &Repository{
		Name:        name,
		Description: "Test repository",
		UserID:      userID,
		Visibility:  "public",
	}
	
	inserted, err := Repos.Insert(repo)
	testutils.AssertNoError(t, err)
	return inserted
}

