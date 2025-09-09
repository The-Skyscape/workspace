package models

import (
	"testing"
	
	"github.com/The-Skyscape/devtools/pkg/testutils"
)

func TestCommentModel(t *testing.T) {
	// Setup test database
	db := SetupTestDB(t)
	defer CleanupTestDB(t, db)
	
	
	t.Run("CreateComment", func(t *testing.T) {
		// Create test data
		user := CreateTestUser(t, db, "commenter@example.com")
		repo := createTestRepository(t, "comment-repo", user.ID)
		issue, err := CreateIssue("Test Issue", "Issue body", user.ID, repo.ID)
		testutils.AssertNoError(t, err)
		
		// Create comment using generic method
		comment, err := CreateComment("issue", issue.ID, repo.ID, user.ID, "This is a test comment")
		testutils.AssertNoError(t, err)
		testutils.AssertNotNil(t, comment)
		testutils.AssertEqual(t, "This is a test comment", comment.Body)
		testutils.AssertEqual(t, user.ID, comment.AuthorID)
		testutils.AssertEqual(t, repo.ID, comment.RepoID)
		testutils.AssertEqual(t, "issue", comment.EntityType)
		testutils.AssertEqual(t, issue.ID, comment.EntityID)
	})
	
	t.Run("IssueComments", func(t *testing.T) {
		// Create test data
		user := CreateTestUser(t, db, "issue-commenter@example.com")
		repo := createTestRepository(t, "issue-comment-repo", user.ID)
		issue, err := CreateIssue("Commented Issue", "Issue with comments", user.ID, repo.ID)
		testutils.AssertNoError(t, err)
		
		// Create multiple comments
		comment1, err := CreateIssueComment(issue.ID, repo.ID, user.ID, "First comment")
		testutils.AssertNoError(t, err)
		testutils.AssertNotNil(t, comment1)
		
		comment2, err := CreateIssueComment(issue.ID, repo.ID, user.ID, "Second comment")
		testutils.AssertNoError(t, err)
		testutils.AssertNotNil(t, comment2)
		
		comment3, err := CreateIssueComment(issue.ID, repo.ID, user.ID, "Third comment")
		testutils.AssertNoError(t, err)
		testutils.AssertNotNil(t, comment3)
		
		// Get issue comments
		comments, err := GetIssueComments(issue.ID)
		testutils.AssertNoError(t, err)
		testutils.AssertEqual(t, 3, len(comments))
		
		// Verify order (should be chronological)
		testutils.AssertEqual(t, "First comment", comments[0].Body)
		testutils.AssertEqual(t, "Second comment", comments[1].Body)
		testutils.AssertEqual(t, "Third comment", comments[2].Body)
	})
	
	t.Run("PRComments", func(t *testing.T) {
		// Create test data
		user := CreateTestUser(t, db, "pr-commenter@example.com")
		repo := createTestRepository(t, "pr-comment-repo", user.ID)
		prID := "pr-123"
		
		// Create PR comments
		comment1, err := CreatePRComment(prID, repo.ID, user.ID, "PR review comment")
		testutils.AssertNoError(t, err)
		testutils.AssertNotNil(t, comment1)
		testutils.AssertEqual(t, "pr", comment1.EntityType)
		testutils.AssertEqual(t, prID, comment1.EntityID)
		
		comment2, err := CreatePRComment(prID, repo.ID, user.ID, "Another PR comment")
		testutils.AssertNoError(t, err)
		testutils.AssertNotNil(t, comment2)
		
		// Get PR comments
		comments, err := GetPRComments(prID)
		testutils.AssertNoError(t, err)
		testutils.AssertEqual(t, 2, len(comments))
	})
	
	t.Run("LineComments", func(t *testing.T) {
		// Create test data
		user := CreateTestUser(t, db, "line-commenter@example.com")
		repo := createTestRepository(t, "line-comment-repo", user.ID)
		prID := "pr-456"
		
		// Create line comment
		lineComment, err := CreateLineComment(
			"pr",
			prID,
			repo.ID,
			user.ID,
			"This line needs refactoring",
			"src/main.go",
			42,
		)
		testutils.AssertNoError(t, err)
		testutils.AssertNotNil(t, lineComment)
		testutils.AssertEqual(t, "src/main.go", lineComment.FilePath)
		testutils.AssertEqual(t, 42, lineComment.LineNumber)
		testutils.AssertEqual(t, "This line needs refactoring", lineComment.Body)
		
		// Create another line comment on different line
		lineComment2, err := CreateLineComment(
			"pr",
			prID,
			repo.ID,
			user.ID,
			"Consider using constants here",
			"src/main.go",
			55,
		)
		testutils.AssertNoError(t, err)
		testutils.AssertNotNil(t, lineComment2)
		
		// Get all PR comments (including line comments)
		allComments, err := GetPRComments(prID)
		testutils.AssertNoError(t, err)
		testutils.AssertEqual(t, 2, len(allComments))
		
		// Verify line comment details
		for _, comment := range allComments {
			testutils.AssertEqual(t, "src/main.go", comment.FilePath)
			testutils.AssertTrue(t, comment.LineNumber > 0)
		}
	})
	
	t.Run("EntityComments", func(t *testing.T) {
		// Create test data
		user := CreateTestUser(t, db, "entity-commenter@example.com")
		repo := createTestRepository(t, "entity-comment-repo", user.ID)
		
		// Create comments on different entity types
		issueID := "issue-789"
		prID := "pr-789"
		commitID := "commit-abc123"
		
		// Issue comment
		_, err := CreateComment("issue", issueID, repo.ID, user.ID, "Issue comment")
		testutils.AssertNoError(t, err)
		
		// PR comments
		_, err = CreateComment("pr", prID, repo.ID, user.ID, "PR comment 1")
		testutils.AssertNoError(t, err)
		_, err = CreateComment("pr", prID, repo.ID, user.ID, "PR comment 2")
		testutils.AssertNoError(t, err)
		
		// Commit comment
		_, err = CreateComment("commit", commitID, repo.ID, user.ID, "Commit comment")
		testutils.AssertNoError(t, err)
		
		// Get comments by entity
		issueComments, err := GetEntityComments("issue", issueID)
		testutils.AssertNoError(t, err)
		testutils.AssertEqual(t, 1, len(issueComments))
		
		prComments, err := GetEntityComments("pr", prID)
		testutils.AssertNoError(t, err)
		testutils.AssertEqual(t, 2, len(prComments))
		
		commitComments, err := GetEntityComments("commit", commitID)
		testutils.AssertNoError(t, err)
		testutils.AssertEqual(t, 1, len(commitComments))
	})
	
	t.Run("CommentRelationships", func(t *testing.T) {
		// Create test data
		user := CreateTestUser(t, db, "relation-commenter@example.com")
		repo := createTestRepository(t, "relation-repo", user.ID)
		
		// Create comment
		comment, err := CreateComment("issue", "issue-999", repo.ID, user.ID, "Test comment")
		testutils.AssertNoError(t, err)
		
		// Test Author relationship
		author, err := comment.Author()
		testutils.AssertNoError(t, err)
		testutils.AssertNotNil(t, author)
		testutils.AssertEqual(t, user.ID, author.ID)
		testutils.AssertEqual(t, user.Email, author.Email)
		
		// Test Repository relationship
		commentRepo, err := comment.Repository()
		testutils.AssertNoError(t, err)
		testutils.AssertNotNil(t, commentRepo)
		testutils.AssertEqual(t, repo.ID, commentRepo.ID)
		testutils.AssertEqual(t, repo.Name, commentRepo.Name)
	})
	
	t.Run("CommentWithCommitSHA", func(t *testing.T) {
		// Create test data
		user := CreateTestUser(t, db, "commit-commenter@example.com")
		repo := createTestRepository(t, "commit-comment-repo", user.ID)
		
		// Create comment on a commit
		comment := &Comment{
			Body:       "This commit looks good",
			AuthorID:   user.ID,
			RepoID:     repo.ID,
			EntityType: "commit",
			EntityID:   "commit-def456",
			CommitSHA:  "def456789abcdef123456789abcdef123456789a",
		}
		
		inserted, err := Comments.Insert(comment)
		testutils.AssertNoError(t, err)
		comment = inserted
		
		testutils.AssertEqual(t, "def456789abcdef123456789abcdef123456789a", comment.CommitSHA)
		testutils.AssertEqual(t, "commit", comment.EntityType)
		testutils.AssertEqual(t, "commit-def456", comment.EntityID)
	})
	
	t.Run("EmptyRelationships", func(t *testing.T) {
		// Create comment with minimal data
		comment := &Comment{
			Body:       "Orphan comment",
			EntityType: "test",
			EntityID:   "test-1",
		}
		
		// Test Author with empty AuthorID
		author, err := comment.Author()
		testutils.AssertNoError(t, err)
		testutils.AssertNil(t, author)
		
		// Test Repository with empty RepoID
		repo, err := comment.Repository()
		testutils.AssertNoError(t, err)
		testutils.AssertNil(t, repo)
	})
}