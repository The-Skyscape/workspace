package processors

import (
	"context"
	"fmt"
	"log"
	"time"

	"workspace/internal/ai/queue"
	"workspace/models"
)

// StaleProcessor handles stale issue and PR management
type StaleProcessor struct{}

// NewStaleProcessor creates a new stale management processor
func NewStaleProcessor() *StaleProcessor {
	return &StaleProcessor{}
}

// Process handles stale management tasks
func (p *StaleProcessor) Process(ctx context.Context, task *queue.Task) error {
	// Process stale issues
	if err := p.processStaleIssues(ctx); err != nil {
		log.Printf("StaleProcessor: Error processing stale issues: %v", err)
	}
	
	// Process stale PRs
	if err := p.processStalePRs(ctx); err != nil {
		log.Printf("StaleProcessor: Error processing stale PRs: %v", err)
	}
	
	return nil
}

// CanHandle checks if this processor can handle the given task type
func (p *StaleProcessor) CanHandle(taskType queue.TaskType) bool {
	return taskType == queue.TaskStaleManagement
}

// processStaleIssues handles stale issues
func (p *StaleProcessor) processStaleIssues(ctx context.Context) error {
	// Find issues older than 30 days with no activity
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	
	issues, err := models.Issues.Search("WHERE Status = 'open' AND UpdatedAt < ?", cutoff.Unix())
	if err != nil {
		return fmt.Errorf("failed to find stale issues: %w", err)
	}
	
	for _, issue := range issues {
		// Add stale label comment
		comment := &models.Comment{
			Body: `## ⏰ Stale Issue Notice

This issue has been automatically marked as stale because it has not had recent activity for 30 days.

**What happens next?**
- If there is no activity within the next 7 days, this issue will be closed automatically
- To keep this issue open, please add a comment or update
- If this issue is still relevant, please add the "keep-open" label

*This is an automated message to help maintain repository health.*`,
			AuthorID:   "system",
			EntityType: "issue",
			EntityID:   issue.ID,
			RepoID:     issue.RepoID,
		}
		
		if _, err := models.Comments.Insert(comment); err != nil {
			log.Printf("StaleProcessor: Failed to add stale comment to issue %s: %v", issue.ID, err)
			continue
		}
		
		log.Printf("StaleProcessor: Marked issue %s as stale", issue.ID)
	}
	
	// Close issues that have been stale for 7+ days
	closeCutoff := time.Now().Add(-37 * 24 * time.Hour)
	oldStale, err := models.Issues.Search("WHERE Status = 'open' AND UpdatedAt < ?", closeCutoff.Unix())
	if err != nil {
		return fmt.Errorf("failed to find old stale issues: %w", err)
	}
	
	for _, issue := range oldStale {
		issue.Status = "closed"
		if err := models.Issues.Update(issue); err != nil {
			log.Printf("StaleProcessor: Failed to close stale issue %s: %v", issue.ID, err)
			continue
		}
		
		// Add closing comment
		comment := &models.Comment{
			Body: `## 🔒 Issue Closed

This issue has been automatically closed due to inactivity.

If this issue is still relevant, please:
1. Reopen the issue
2. Add a comment explaining why it should remain open
3. Add the "keep-open" label to prevent automatic closure

*Closed by AI automation to maintain repository health.*`,
			AuthorID:   "system",
			EntityType: "issue",
			EntityID:   issue.ID,
			RepoID:     issue.RepoID,
		}
		
		models.Comments.Insert(comment)
		log.Printf("StaleProcessor: Closed stale issue %s", issue.ID)
	}
	
	return nil
}

// processStalePRs handles stale pull requests
func (p *StaleProcessor) processStalePRs(ctx context.Context) error {
	// Find PRs older than 14 days with no activity
	cutoff := time.Now().Add(-14 * 24 * time.Hour)
	
	prs, err := models.PullRequests.Search("WHERE Status = 'open' AND UpdatedAt < ?", cutoff.Unix())
	if err != nil {
		return fmt.Errorf("failed to find stale PRs: %w", err)
	}
	
	for _, pr := range prs {
		// Add stale notice comment
		comment := &models.Comment{
			Body: `## ⏰ Stale Pull Request Notice

This pull request has been automatically flagged as stale because it has not had recent activity for 14 days.

**Recommended actions:**
- Resolve any merge conflicts
- Address review feedback
- Request reviews from maintainers
- Consider breaking into smaller PRs if too large

**What happens next?**
- After 30 days of inactivity, this PR may be closed
- To prevent closure, please update the PR or add a comment

*This message helps maintain PR velocity and code freshness.*`,
			AuthorID:   "system",
			EntityType: "pull_request",
			EntityID:   pr.ID,
			RepoID:     pr.RepoID,
		}
		
		if _, err := models.Comments.Insert(comment); err != nil {
			log.Printf("StaleProcessor: Failed to add stale comment to PR %s: %v", pr.ID, err)
			continue
		}
		
		log.Printf("StaleProcessor: Marked PR %s as stale", pr.ID)
	}
	
	return nil
}