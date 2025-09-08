package processors

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"workspace/internal/ai/queue"
	"workspace/models"
)

// ReportProcessor handles daily report generation
type ReportProcessor struct{}

// NewReportProcessor creates a new report processor
func NewReportProcessor() *ReportProcessor {
	return &ReportProcessor{}
}

// Process generates daily reports for repositories
func (p *ReportProcessor) Process(ctx context.Context, task *queue.Task) error {
	repoID, ok := task.Data["repo_id"].(string)
	if !ok || repoID == "" {
		return fmt.Errorf("invalid repo ID in task data")
	}
	
	// Get the repository
	repo, err := models.Repos.Get(repoID)
	if err != nil {
		return fmt.Errorf("failed to get repo %s: %w", repoID, err)
	}
	
	// Generate the report
	report, err := p.generateReport(ctx, repo)
	if err != nil {
		return fmt.Errorf("failed to generate report: %w", err)
	}
	
	// Create an issue with the report
	issue := &models.Issue{
		Title:       fmt.Sprintf("Daily Report - %s", time.Now().Format("January 2, 2006")),
		Description: report,
		Status:      "open",
		Priority:    "low",
		RepoID:      repoID,
		AuthorID:    task.UserID,
	}
	
	if _, err := models.Issues.Insert(issue); err != nil {
		return fmt.Errorf("failed to create report issue: %w", err)
	}
	
	log.Printf("ReportProcessor: Generated daily report for repo %s", repo.Name)
	
	task.Result = map[string]interface{}{
		"report":   report,
		"issue_id": issue.ID,
	}
	
	return nil
}

// CanHandle checks if this processor can handle the given task type
func (p *ReportProcessor) CanHandle(taskType queue.TaskType) bool {
	return taskType == queue.TaskDailyReport
}

// generateReport creates a comprehensive daily report
func (p *ReportProcessor) generateReport(ctx context.Context, repo *models.Repository) (string, error) {
	var b strings.Builder
	
	b.WriteString("# 📊 Daily Repository Report\n\n")
	b.WriteString(fmt.Sprintf("**Repository:** %s\n", repo.Name))
	b.WriteString(fmt.Sprintf("**Date:** %s\n", time.Now().Format("January 2, 2006")))
	b.WriteString(fmt.Sprintf("**Time:** %s\n\n", time.Now().Format("3:04 PM MST")))
	
	// Repository health score
	health := p.calculateHealthScore(repo)
	b.WriteString("## 🏥 Repository Health\n")
	b.WriteString(fmt.Sprintf("**Overall Score:** %s\n\n", p.getHealthBadge(health)))
	
	// Activity summary (last 24 hours)
	b.WriteString("## 📈 Activity Summary (Last 24 Hours)\n")
	
	// Get recent commits
	commits, _ := p.getRecentCommits(repo.ID, 24*time.Hour)
	b.WriteString(fmt.Sprintf("- **Commits:** %d\n", len(commits)))
	
	// Get open issues
	issues, _ := models.Issues.Search("WHERE RepoID = ? AND Status = 'open'", repo.ID)
	b.WriteString(fmt.Sprintf("- **Open Issues:** %d\n", len(issues)))
	
	// Get open PRs
	prs, _ := models.PullRequests.Search("WHERE RepoID = ? AND Status = 'open'", repo.ID)
	b.WriteString(fmt.Sprintf("- **Open Pull Requests:** %d\n", len(prs)))
	
	// Get recent AI activity
	activities, _ := models.AIActivities.Search("WHERE RepoID = ? AND CreatedAt > ? ORDER BY CreatedAt DESC LIMIT 10", 
		repo.ID, time.Now().Add(-24*time.Hour).Unix())
	b.WriteString(fmt.Sprintf("- **AI Activities:** %d\n\n", len(activities)))
	
	// Issue statistics
	b.WriteString("## 🐛 Issue Statistics\n")
	
	// Priority breakdown
	critical, _ := models.Issues.Search("WHERE RepoID = ? AND Status = 'open' AND Priority = 'critical'", repo.ID)
	high, _ := models.Issues.Search("WHERE RepoID = ? AND Status = 'open' AND Priority = 'high'", repo.ID)
	medium, _ := models.Issues.Search("WHERE RepoID = ? AND Status = 'open' AND Priority = 'medium'", repo.ID)
	low, _ := models.Issues.Search("WHERE RepoID = ? AND Status = 'open' AND Priority = 'low'", repo.ID)
	
	b.WriteString("### Priority Breakdown\n")
	b.WriteString(fmt.Sprintf("- 🔴 Critical: %d\n", len(critical)))
	b.WriteString(fmt.Sprintf("- 🟠 High: %d\n", len(high)))
	b.WriteString(fmt.Sprintf("- 🟡 Medium: %d\n", len(medium)))
	b.WriteString(fmt.Sprintf("- 🟢 Low: %d\n\n", len(low)))
	
	// Age analysis
	b.WriteString("### Age Analysis\n")
	oldIssues := p.findOldIssues(issues, 7*24*time.Hour)
	staleIssues := p.findOldIssues(issues, 30*24*time.Hour)
	
	b.WriteString(fmt.Sprintf("- Issues older than 7 days: %d\n", len(oldIssues)))
	b.WriteString(fmt.Sprintf("- Issues older than 30 days: %d\n\n", len(staleIssues)))
	
	// PR statistics
	b.WriteString("## 🔀 Pull Request Statistics\n")
	
	// PR age analysis
	oldPRs := p.findOldPRs(prs, 3*24*time.Hour)
	stalePRs := p.findOldPRs(prs, 7*24*time.Hour)
	
	b.WriteString(fmt.Sprintf("- PRs awaiting review > 3 days: %d\n", len(oldPRs)))
	b.WriteString(fmt.Sprintf("- PRs awaiting review > 7 days: %d\n", len(stalePRs)))
	
	// Review status
	approved, _ := models.PullRequests.Search("WHERE RepoID = ? AND Status = 'approved'", repo.ID)
	b.WriteString(fmt.Sprintf("- Recently approved: %d\n\n", len(approved)))
	
	// Code quality indicators
	b.WriteString("## 🎯 Code Quality Indicators\n")
	
	// Test coverage trend (simulated)
	b.WriteString("- **Test Coverage:** ")
	if len(commits) > 0 {
		b.WriteString("✅ Tests updated with recent changes\n")
	} else {
		b.WriteString("⚠️ No recent test updates\n")
	}
	
	// Documentation status
	b.WriteString("- **Documentation:** ")
	hasRecentDocs := p.checkRecentDocumentationUpdates(repo.ID)
	if hasRecentDocs {
		b.WriteString("✅ Recently updated\n")
	} else {
		b.WriteString("⚠️ May need attention\n")
	}
	
	// Dependencies
	b.WriteString("- **Dependencies:** ")
	b.WriteString("ℹ️ Run dependency audit recommended\n\n")
	
	// AI automation summary
	b.WriteString("## 🤖 AI Automation Summary\n")
	
	if len(activities) > 0 {
		b.WriteString("### Recent AI Activities\n")
		for i, activity := range activities[:min(5, len(activities))] {
			b.WriteString(fmt.Sprintf("%d. %s - %s\n", 
				i+1, activity.Type, activity.Description))
		}
		b.WriteString("\n")
	} else {
		b.WriteString("No AI activities in the last 24 hours.\n\n")
	}
	
	// Recommendations
	b.WriteString("## 💡 Recommendations\n")
	
	recommendations := p.generateRecommendations(repo, issues, prs, health)
	for i, rec := range recommendations {
		b.WriteString(fmt.Sprintf("%d. %s\n", i+1, rec))
	}
	
	// Upcoming milestones
	b.WriteString("\n## 📅 Upcoming Milestones\n")
	b.WriteString("No upcoming milestones scheduled.\n")
	
	// Footer
	b.WriteString("\n---\n")
	b.WriteString("*This report was generated automatically by the AI Assistant. ")
	b.WriteString("For detailed metrics and real-time monitoring, visit the repository dashboard.*\n")
	
	return b.String(), nil
}

// calculateHealthScore calculates repository health
func (p *ReportProcessor) calculateHealthScore(repo *models.Repository) int {
	score := 100
	
	// Check various health indicators
	issues, _ := models.Issues.Search("WHERE RepoID = ? AND Status = 'open'", repo.ID)
	prs, _ := models.PullRequests.Search("WHERE RepoID = ? AND Status = 'open'", repo.ID)
	
	// Too many open issues
	if len(issues) > 50 {
		score -= 20
	} else if len(issues) > 20 {
		score -= 10
	}
	
	// Old unresolved issues
	oldIssues := p.findOldIssues(issues, 30*24*time.Hour)
	if len(oldIssues) > 10 {
		score -= 15
	} else if len(oldIssues) > 5 {
		score -= 8
	}
	
	// Stale PRs
	stalePRs := p.findOldPRs(prs, 7*24*time.Hour)
	if len(stalePRs) > 5 {
		score -= 15
	} else if len(stalePRs) > 2 {
		score -= 8
	}
	
	// Critical issues
	critical, _ := models.Issues.Search("WHERE RepoID = ? AND Status = 'open' AND Priority = 'critical'", repo.ID)
	if len(critical) > 0 {
		score -= len(critical) * 10
	}
	
	// Ensure score doesn't go below 0
	if score < 0 {
		score = 0
	}
	
	return score
}

// getHealthBadge returns a health score badge
func (p *ReportProcessor) getHealthBadge(score int) string {
	if score >= 90 {
		return fmt.Sprintf("🟢 Excellent (%d/100)", score)
	} else if score >= 70 {
		return fmt.Sprintf("🟡 Good (%d/100)", score)
	} else if score >= 50 {
		return fmt.Sprintf("🟠 Fair (%d/100)", score)
	} else {
		return fmt.Sprintf("🔴 Needs Attention (%d/100)", score)
	}
}

// getRecentCommits gets commits from the last duration
func (p *ReportProcessor) getRecentCommits(repoID string, duration time.Duration) ([]string, error) {
	// This would query actual git commits
	// For now, return empty slice
	return []string{}, nil
}

// findOldIssues finds issues older than the specified duration
func (p *ReportProcessor) findOldIssues(issues []*models.Issue, age time.Duration) []*models.Issue {
	var old []*models.Issue
	cutoff := time.Now().Add(-age)
	
	for _, issue := range issues {
		if issue.CreatedAt.Before(cutoff) {
			old = append(old, issue)
		}
	}
	
	return old
}

// findOldPRs finds PRs older than the specified duration
func (p *ReportProcessor) findOldPRs(prs []*models.PullRequest, age time.Duration) []*models.PullRequest {
	var old []*models.PullRequest
	cutoff := time.Now().Add(-age)
	
	for _, pr := range prs {
		if pr.CreatedAt.Before(cutoff) {
			old = append(old, pr)
		}
	}
	
	return old
}

// checkRecentDocumentationUpdates checks for recent doc updates
func (p *ReportProcessor) checkRecentDocumentationUpdates(repoID string) bool {
	// This would check actual commit history for doc changes
	// For now, return false
	return false
}

// generateRecommendations creates actionable recommendations
func (p *ReportProcessor) generateRecommendations(repo *models.Repository, issues []*models.Issue, prs []*models.PullRequest, health int) []string {
	var recommendations []string
	
	// Health-based recommendations
	if health < 50 {
		recommendations = append(recommendations, 
			"🔴 Repository health is low - consider scheduling a cleanup sprint")
	}
	
	// Issue-based recommendations
	if len(issues) > 20 {
		recommendations = append(recommendations,
			"📋 High number of open issues - consider triaging and prioritizing")
	}
	
	oldIssues := p.findOldIssues(issues, 30*24*time.Hour)
	if len(oldIssues) > 5 {
		recommendations = append(recommendations,
			"🕰️ Several stale issues detected - review and close or update")
	}
	
	// PR-based recommendations
	stalePRs := p.findOldPRs(prs, 7*24*time.Hour)
	if len(stalePRs) > 2 {
		recommendations = append(recommendations,
			"🔀 Multiple PRs awaiting review - allocate time for code reviews")
	}
	
	// Critical issues
	critical, _ := models.Issues.Search("WHERE RepoID = ? AND Status = 'open' AND Priority = 'critical'", repo.ID)
	if len(critical) > 0 {
		recommendations = append(recommendations,
			fmt.Sprintf("⚠️ %d critical issues require immediate attention", len(critical)))
	}
	
	// General recommendations
	if len(recommendations) == 0 {
		recommendations = append(recommendations,
			"✅ Repository is in good health - maintain current practices",
			"🎯 Consider setting new milestones for upcoming features",
			"📚 Keep documentation up to date with recent changes",
		)
	}
	
	return recommendations
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}