package models

import (
	"strings"
	"testing"
	
	"github.com/The-Skyscape/devtools/pkg/testutils"
)

func TestTagSystem(t *testing.T) {
	// Setup test database
	db := SetupTestDB(t)
	defer CleanupTestDB(t, db)
	
	t.Run("CreateSystemTags", func(t *testing.T) {
		// Create system tags
		err := CreateSystemTags()
		testutils.AssertNoError(t, err)
		
		// Verify some system tags exist
		bugTag, err := TagDefinitions.Search("WHERE Name = ? AND IsSystem = ?", "bug", true)
		testutils.AssertNoError(t, err)
		testutils.AssertEqual(t, 1, len(bugTag))
		testutils.AssertEqual(t, "bug", bugTag[0].Name)
		testutils.AssertEqual(t, TagCategoryBug, bugTag[0].Category)
		testutils.AssertEqual(t, "#d73a4a", bugTag[0].Color)
		testutils.AssertTrue(t, bugTag[0].IsSystem)
		
		// Verify priority tags
		criticalTag, err := TagDefinitions.Search("WHERE Name = ? AND Category = ?", "critical", TagCategoryPriority)
		testutils.AssertNoError(t, err)
		testutils.AssertEqual(t, 1, len(criticalTag))
		testutils.AssertEqual(t, "#b91c1c", criticalTag[0].Color)
		
		// Test idempotency - should not create duplicates
		err = CreateSystemTags()
		testutils.AssertNoError(t, err)
		
		bugTagsAfter, err := TagDefinitions.Search("WHERE Name = ? AND IsSystem = ?", "bug", true)
		testutils.AssertNoError(t, err)
		testutils.AssertEqual(t, 1, len(bugTagsAfter))
	})
	
	t.Run("GetOrCreateTag", func(t *testing.T) {
		repoID := "test-repo-1"
		
		// Create new custom tag
		customTag, err := GetOrCreateTag("custom-feature", repoID)
		testutils.AssertNoError(t, err)
		testutils.AssertNotNil(t, customTag)
		testutils.AssertEqual(t, "custom-feature", customTag.Name)
		testutils.AssertEqual(t, TagCategoryCustom, customTag.Category)
		testutils.AssertEqual(t, "#808080", customTag.Color)
		testutils.AssertEqual(t, repoID, customTag.RepoID)
		testutils.AssertFalse(t, customTag.IsSystem)
		
		// Get existing tag
		existingTag, err := GetOrCreateTag("custom-feature", repoID)
		testutils.AssertNoError(t, err)
		testutils.AssertNotNil(t, existingTag)
		testutils.AssertEqual(t, customTag.ID, existingTag.ID)
		
		// Test case insensitivity and trimming
		sameTag, err := GetOrCreateTag("  CUSTOM-FEATURE  ", repoID)
		testutils.AssertNoError(t, err)
		testutils.AssertNotNil(t, sameTag)
		testutils.AssertEqual(t, customTag.ID, sameTag.ID)
		
		// Test empty tag name
		emptyTag, err := GetOrCreateTag("  ", repoID)
		testutils.AssertNoError(t, err)
		testutils.AssertNil(t, emptyTag)
	})
	
	t.Run("RepoSpecificVsGlobalTags", func(t *testing.T) {
		repo1 := "repo-1"
		repo2 := "repo-2"
		
		// Create global tag (empty RepoID)
		globalTag, err := GetOrCreateTag("global-tag", "")
		testutils.AssertNoError(t, err)
		testutils.AssertNotNil(t, globalTag)
		testutils.AssertEqual(t, "", globalTag.RepoID)
		
		// Create repo-specific tag with same name
		repo1Tag, err := GetOrCreateTag("global-tag", repo1)
		testutils.AssertNoError(t, err)
		testutils.AssertNotNil(t, repo1Tag)
		testutils.AssertEqual(t, repo1, repo1Tag.RepoID)
		testutils.AssertTrue(t, globalTag.ID != repo1Tag.ID)
		
		// Get tag for repo2 should return global tag
		repo2Tag, err := GetOrCreateTag("global-tag", repo2)
		testutils.AssertNoError(t, err)
		testutils.AssertNotNil(t, repo2Tag)
		// Since there's no repo2-specific tag, it should return the repo1-specific or create new
		// Based on the logic, it will create a new one for repo2
		testutils.AssertEqual(t, repo2, repo2Tag.RepoID)
	})
	
	t.Run("IssueLabelManagement", func(t *testing.T) {
		// Create test data
		user := CreateTestUser(t, db, "label-user@example.com")
		repo := createTestRepository(t, "label-test-repo", user.ID)
		issue, err := CreateIssue("Labeled Issue", "Issue for testing labels", user.ID, repo.ID)
		testutils.AssertNoError(t, err)
		
		// Create tags
		bugTag, err := GetOrCreateTag("bug", repo.ID)
		testutils.AssertNoError(t, err)
		securityTag, err := GetOrCreateTag("security", repo.ID)
		testutils.AssertNoError(t, err)
		
		// Add labels to issue
		err = AddLabelToIssue(issue.ID, bugTag.ID, user.ID)
		testutils.AssertNoError(t, err)
		
		err = AddLabelToIssue(issue.ID, securityTag.ID, user.ID)
		testutils.AssertNoError(t, err)
		
		// Test duplicate prevention
		err = AddLabelToIssue(issue.ID, bugTag.ID, user.ID)
		testutils.AssertNoError(t, err) // Should not error on duplicate
		
		// Get issue labels
		labels, err := GetIssueLabels(issue.ID)
		testutils.AssertNoError(t, err)
		testutils.AssertEqual(t, 2, len(labels))
		
		// Verify label names
		labelNames := make([]string, len(labels))
		for i, label := range labels {
			labelNames[i] = label.Name
		}
		testutils.AssertContains(t, strings.Join(labelNames, ","), "bug")
		testutils.AssertContains(t, strings.Join(labelNames, ","), "security")
		
		// Remove a label
		err = RemoveLabelFromIssue(issue.ID, bugTag.ID)
		testutils.AssertNoError(t, err)
		
		// Verify removal
		labelsAfter, err := GetIssueLabels(issue.ID)
		testutils.AssertNoError(t, err)
		testutils.AssertEqual(t, 1, len(labelsAfter))
		testutils.AssertEqual(t, "security", labelsAfter[0].Name)
		
		// Test removing non-existent label
		err = RemoveLabelFromIssue(issue.ID, "nonexistent-id")
		testutils.AssertNoError(t, err) // Should not error
	})
	
	t.Run("GetIssuesByTagID", func(t *testing.T) {
		// Create test data
		user := CreateTestUser(t, db, "tag-search@example.com")
		repo := createTestRepository(t, "tag-search-repo", user.ID)
		
		// Create issues
		issue1, err := CreateIssue("Bug Issue 1", "First bug", user.ID, repo.ID)
		testutils.AssertNoError(t, err)
		issue2, err := CreateIssue("Bug Issue 2", "Second bug", user.ID, repo.ID)
		testutils.AssertNoError(t, err)
		issue3, err := CreateIssue("Feature Issue", "New feature", user.ID, repo.ID)
		testutils.AssertNoError(t, err)
		
		// Create tags
		bugTag, err := GetOrCreateTag("bug", repo.ID)
		testutils.AssertNoError(t, err)
		featureTag, err := GetOrCreateTag("feature", repo.ID)
		testutils.AssertNoError(t, err)
		
		// Label issues
		err = AddLabelToIssue(issue1.ID, bugTag.ID, user.ID)
		testutils.AssertNoError(t, err)
		err = AddLabelToIssue(issue2.ID, bugTag.ID, user.ID)
		testutils.AssertNoError(t, err)
		err = AddLabelToIssue(issue3.ID, featureTag.ID, user.ID)
		testutils.AssertNoError(t, err)
		
		// Get issues by tag
		bugIssues, err := GetIssuesByTagID(bugTag.ID)
		testutils.AssertNoError(t, err)
		testutils.AssertEqual(t, 2, len(bugIssues))
		testutils.AssertContains(t, strings.Join(bugIssues, ","), issue1.ID)
		testutils.AssertContains(t, strings.Join(bugIssues, ","), issue2.ID)
		
		featureIssues, err := GetIssuesByTagID(featureTag.ID)
		testutils.AssertNoError(t, err)
		testutils.AssertEqual(t, 1, len(featureIssues))
		testutils.AssertEqual(t, issue3.ID, featureIssues[0])
	})
	
	t.Run("IssueLabelRelationships", func(t *testing.T) {
		// Create test data
		user := CreateTestUser(t, db, "rel-user@example.com")
		repo := createTestRepository(t, "rel-repo", user.ID)
		issue, err := CreateIssue("Related Issue", "Issue with relationships", user.ID, repo.ID)
		testutils.AssertNoError(t, err)
		
		// Create and add tag
		tag, err := GetOrCreateTag("test-tag", repo.ID)
		testutils.AssertNoError(t, err)
		err = AddLabelToIssue(issue.ID, tag.ID, user.ID)
		testutils.AssertNoError(t, err)
		
		// Get the IssueLabel entry
		labels, err := IssueLabels.Search("WHERE IssueID = ? AND TagID = ?", issue.ID, tag.ID)
		testutils.AssertNoError(t, err)
		testutils.AssertEqual(t, 1, len(labels))
		
		label := labels[0]
		
		// Test Issue relationship
		relatedIssue, err := label.Issue()
		testutils.AssertNoError(t, err)
		testutils.AssertNotNil(t, relatedIssue)
		testutils.AssertEqual(t, issue.ID, relatedIssue.ID)
		testutils.AssertEqual(t, issue.Title, relatedIssue.Title)
		
		// Test Tag relationship
		relatedTag, err := label.Tag()
		testutils.AssertNoError(t, err)
		testutils.AssertNotNil(t, relatedTag)
		testutils.AssertEqual(t, tag.ID, relatedTag.ID)
		testutils.AssertEqual(t, tag.Name, relatedTag.Name)
		
		// Test AddedByUser relationship
		addedBy, err := label.AddedByUser()
		testutils.AssertNoError(t, err)
		testutils.AssertNotNil(t, addedBy)
		testutils.AssertEqual(t, user.ID, addedBy.ID)
		testutils.AssertEqual(t, user.Email, addedBy.Email)
	})
	
	t.Run("TagSortOrder", func(t *testing.T) {
		// Create tags with different sort orders
		tags := []struct {
			name      string
			category  TagCategory
			sortOrder int
		}{
			{"high-priority", TagCategoryPriority, 1},
			{"medium-priority", TagCategoryPriority, 5},
			{"low-priority", TagCategoryPriority, 10},
			{"urgent", TagCategoryStatus, 2},
		}
		
		for _, tagData := range tags {
			tag := &TagDefinition{
				Name:      tagData.name,
				Category:  tagData.category,
				Color:     "#000000",
				RepoID:    "sort-test-repo",
				IsSystem:  false,
				SortOrder: tagData.sortOrder,
			}
			_, err := TagDefinitions.Insert(tag)
			testutils.AssertNoError(t, err)
		}
		
		// Query tags ordered by SortOrder
		sortedTags, err := TagDefinitions.Search("WHERE RepoID = ? ORDER BY SortOrder", "sort-test-repo")
		testutils.AssertNoError(t, err)
		testutils.AssertEqual(t, 4, len(sortedTags))
		
		// Verify order
		testutils.AssertEqual(t, "high-priority", sortedTags[0].Name)
		testutils.AssertEqual(t, "urgent", sortedTags[1].Name)
		testutils.AssertEqual(t, "medium-priority", sortedTags[2].Name)
		testutils.AssertEqual(t, "low-priority", sortedTags[3].Name)
	})
	
	t.Run("EmptyLabelRelationships", func(t *testing.T) {
		// Create IssueLabel with minimal data
		label := &IssueLabel{
			IssueID: "",
			TagID:   "",
			AddedBy: "",
		}
		
		// Test empty relationships
		issue, err := label.Issue()
		testutils.AssertNoError(t, err)
		testutils.AssertNil(t, issue)
		
		tag, err := label.Tag()
		testutils.AssertNoError(t, err)
		testutils.AssertNil(t, tag)
		
		user, err := label.AddedByUser()
		testutils.AssertNoError(t, err)
		testutils.AssertNil(t, user)
	})
}