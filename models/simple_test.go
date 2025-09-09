package models

import (
	"testing"
	
	"github.com/The-Skyscape/devtools/pkg/testutils"
)

func TestSimpleIssue(t *testing.T) {
	// Create a simple issue without database
	issue := &Issue{
		Title:    "Test Issue",
		Body:     "Test Body",
		Status:   IssueStatusOpen,
		Priority: PriorityMedium,
		Column:   "todo",
	}
	
	testutils.AssertEqual(t, "Test Issue", issue.Title)
	testutils.AssertEqual(t, "Test Body", issue.Body)
	testutils.AssertEqual(t, IssueStatusOpen, issue.Status)
	testutils.AssertEqual(t, PriorityMedium, issue.Priority)
}

func TestIssueLabels(t *testing.T) {
	// Test the HasLabel method with mock data
	issue := &Issue{
		Title: "Test Issue",
	}
	
	// The HasLabel method will return false since we don't have a database
	testutils.AssertFalse(t, issue.HasLabel("bug"))
}

func TestEventMetadata(t *testing.T) {
	// Test event without database
	event := &Event{
		Type:       EventIssueCreated,
		Status:     EventStatusPending,
		Priority:   5,
		EntityType: "issue",
		EntityID:   "test-123",
	}
	
	testutils.AssertEqual(t, EventIssueCreated, event.Type)
	testutils.AssertEqual(t, EventStatusPending, event.Status)
	testutils.AssertEqual(t, 5, event.Priority)
	
	// Test retry logic
	testutils.AssertTrue(t, event.ShouldRetry())
	
	event.RetryCount = 3
	event.MaxRetries = 3
	testutils.AssertFalse(t, event.ShouldRetry())
}

func TestCommentPolymorphism(t *testing.T) {
	// Test comment with different entity types
	issueComment := &Comment{
		Body:       "Issue comment",
		EntityType: "issue",
		EntityID:   "issue-123",
	}
	
	prComment := &Comment{
		Body:       "PR comment",
		EntityType: "pr", 
		EntityID:   "pr-456",
		FilePath:   "main.go",
		LineNumber: 42,
	}
	
	testutils.AssertEqual(t, "issue", issueComment.EntityType)
	testutils.AssertEqual(t, "issue-123", issueComment.EntityID)
	
	testutils.AssertEqual(t, "pr", prComment.EntityType)
	testutils.AssertEqual(t, "pr-456", prComment.EntityID)
	testutils.AssertEqual(t, "main.go", prComment.FilePath)
	testutils.AssertEqual(t, 42, prComment.LineNumber)
}

func TestTagCategories(t *testing.T) {
	// Test tag definition without database
	bugTag := &TagDefinition{
		Name:        "bug",
		Category:    TagCategoryBug,
		Color:       "#d73a4a",
		Description: "Something isn't working",
		IsSystem:    true,
		SortOrder:   1,
	}
	
	customTag := &TagDefinition{
		Name:        "custom-feature",
		Category:    TagCategoryCustom,
		Color:       "#808080",
		IsSystem:    false,
		SortOrder:   100,
	}
	
	testutils.AssertEqual(t, "bug", bugTag.Name)
	testutils.AssertEqual(t, TagCategoryBug, bugTag.Category)
	testutils.AssertTrue(t, bugTag.IsSystem)
	
	testutils.AssertEqual(t, "custom-feature", customTag.Name)
	testutils.AssertEqual(t, TagCategoryCustom, customTag.Category)
	testutils.AssertFalse(t, customTag.IsSystem)
}