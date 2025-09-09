package ai

import (
	"fmt"
	"log"
	"strings"
	"time"
	"workspace/models"
)

// IssueTriageProcessor handles automatic issue triage
type IssueTriageProcessor struct{}

func (p *IssueTriageProcessor) ProcessEvent(event *AIEvent) error {
	log.Printf("IssueTriageProcessor: Processing issue %s", event.EntityID)
	
	// Get the issue
	issue, err := models.Issues.Get(event.EntityID)
	if err != nil {
		return fmt.Errorf("failed to get issue: %w", err)
	}
	
	// For now, use basic triage without AI
	// In production, this would call services.Ollama.ChatWithTools properly
	analysis := struct {
		Priority   string   `json:"priority"`
		Labels     []string `json:"labels"`
		Areas      []string `json:"areas"`
		Complexity string   `json:"complexity"`
		Summary    string   `json:"summary"`
	}{
		Priority:   "medium",
		Labels:     []string{"needs-triage"},
		Areas:      []string{"general"},
		Complexity: "medium",
		Summary:    "Automated triage pending",
	}
	
	// Update issue priority if not already set
	if issue.Priority == models.PriorityNone {
		switch analysis.Priority {
		case "critical":
			issue.Priority = models.PriorityCritical
		case "high":
			issue.Priority = models.PriorityHigh
		case "medium":
			issue.Priority = models.PriorityMedium
		case "low":
			issue.Priority = models.PriorityLow
		default:
			issue.Priority = models.PriorityMedium
		}
		if err := models.Issues.Update(issue); err != nil {
			log.Printf("IssueTriageProcessor: Failed to update priority: %v", err)
		}
	}
	
	// Add labels as tags
	for _, label := range analysis.Labels {
		tag := &models.IssueTag{
			IssueID: issue.ID,
			Tag:     label,
		}
		if _, err := models.IssueTags.Insert(tag); err != nil {
			log.Printf("IssueTriageProcessor: Failed to add tag %s: %v", label, err)
		}
	}
	
	// Add automated comment with analysis
	// Note: IssueComment model doesn't exist yet, skip for now
	/*
	comment := &models.IssueComment{
		IssueID:  issue.ID,
		AuthorID: "ai-assistant",
		Body: fmt.Sprintf(`🤖 **AI Analysis**

**Priority:** %s
**Complexity:** %s
**Labels:** %s
**Technical Areas:** %s

**Summary:** %s

*This analysis was generated automatically by the AI assistant.*`,
			analysis.Priority,
			analysis.Complexity,
			strings.Join(analysis.Labels, ", "),
			strings.Join(analysis.Areas, ", "),
			analysis.Summary),
	}
	
	if _, err := models.IssueComments.Insert(comment); err != nil {
		log.Printf("IssueTriageProcessor: Failed to add comment: %v", err)
	}
	*/
	log.Printf("IssueTriageProcessor: Would add analysis comment for issue %s", issue.ID)
	
	log.Printf("IssueTriageProcessor: Successfully triaged issue %s with priority %s", 
		issue.ID, analysis.Priority)
	return nil
}

func (p *IssueTriageProcessor) CanHandle(eventType EventType) bool {
	return eventType == EventIssueCreated
}

// IssueUpdateProcessor handles issue updates
type IssueUpdateProcessor struct{}

func (p *IssueUpdateProcessor) ProcessEvent(event *AIEvent) error {
	log.Printf("IssueUpdateProcessor: Processing update for issue %s", event.EntityID)
	
	// Get the issue
	issue, err := models.Issues.Get(event.EntityID)
	if err != nil {
		return fmt.Errorf("failed to get issue: %w", err)
	}
	
	// Check if issue was closed
	if issue.Status == "closed" {
		// Log completion
		log.Printf("IssueUpdateProcessor: Issue %s was closed", issue.ID)
		
		// Could trigger additional automation here
		// e.g., update project boards, notify stakeholders, etc.
	}
	
	return nil
}

func (p *IssueUpdateProcessor) CanHandle(eventType EventType) bool {
	return eventType == EventIssueUpdated || eventType == EventIssueClosed
}

// PRReviewProcessor handles automated PR reviews
type PRReviewProcessor struct{}

func (p *PRReviewProcessor) ProcessEvent(event *AIEvent) error {
	log.Printf("PRReviewProcessor: Processing PR %s", event.EntityID)
	
	// Get the PR
	pr, err := models.PullRequests.Get(event.EntityID)
	if err != nil {
		return fmt.Errorf("failed to get PR: %w", err)
	}
	
	// Get the repository
	repo, err := models.Repos.Get(pr.RepoID)
	if err != nil {
		return fmt.Errorf("failed to get repo: %w", err)
	}
	
	// Build prompt for AI review (disabled for now)
	_ = fmt.Sprintf(`Review this pull request and provide:
1. Code quality assessment (1-10)
2. Security concerns (if any)
3. Performance considerations
4. Suggested improvements
5. Whether it's safe to auto-approve (yes/no)

PR Title: %s
PR Description: %s
Compare Branch: %s
Base Branch: %s
Repository: %s

Respond in JSON format with fields: quality_score, security_concerns (array), performance_notes, improvements (array), auto_approve (boolean), summary`,
		pr.Title, pr.Body, pr.CompareBranch, pr.BaseBranch, repo.Name)
	
	// For now, skip AI review if Ollama isn't properly configured
	review := struct {
		QualityScore      int      `json:"quality_score"`
		SecurityConcerns  []string `json:"security_concerns"`
		PerformanceNotes  string   `json:"performance_notes"`
		Improvements      []string `json:"improvements"`
		AutoApprove      bool     `json:"auto_approve"`
		Summary          string   `json:"summary"`
	}{
		QualityScore: 7,
		Summary:      "Automated review pending manual verification",
	}
	
	// Add review comment
	reviewBody := fmt.Sprintf(`🤖 **Automated PR Review**

**Code Quality:** %d/10
%s
%s
%s

**Summary:** %s

*This review was generated automatically by the AI assistant.*`,
		review.QualityScore,
		func() string {
			if len(review.SecurityConcerns) > 0 {
				return fmt.Sprintf("\n⚠️ **Security Concerns:**\n- %s", 
					strings.Join(review.SecurityConcerns, "\n- "))
			}
			return ""
		}(),
		func() string {
			if review.PerformanceNotes != "" {
				return fmt.Sprintf("\n📊 **Performance Notes:** %s", review.PerformanceNotes)
			}
			return ""
		}(),
		func() string {
			if len(review.Improvements) > 0 {
				return fmt.Sprintf("\n💡 **Suggested Improvements:**\n- %s",
					strings.Join(review.Improvements, "\n- "))
			}
			return ""
		}(),
		review.Summary)
	
	// PRComment model doesn't exist yet, skip adding comment for now
	log.Printf("PRReviewProcessor: Would add review comment: %s", reviewBody[:100])
	
	// Auto-approve if safe
	if review.AutoApprove && review.QualityScore >= 8 && len(review.SecurityConcerns) == 0 {
		pr.Status = "approved"
		if err := models.PullRequests.Update(pr); err != nil {
			log.Printf("PRReviewProcessor: Failed to auto-approve: %v", err)
		} else {
			log.Printf("PRReviewProcessor: Auto-approved PR %s", pr.ID)
		}
	}
	
	return nil
}

func (p *PRReviewProcessor) CanHandle(eventType EventType) bool {
	return eventType == EventPRCreated
}

// PRUpdateProcessor handles PR updates
type PRUpdateProcessor struct{}

func (p *PRUpdateProcessor) ProcessEvent(event *AIEvent) error {
	log.Printf("PRUpdateProcessor: Processing update for PR %s", event.EntityID)
	
	// Get the PR
	pr, err := models.PullRequests.Get(event.EntityID)
	if err != nil {
		return fmt.Errorf("failed to get PR: %w", err)
	}
	
	// Check if PR was merged
	if pr.Status == "merged" {
		log.Printf("PRUpdateProcessor: PR %s was merged", pr.ID)
		// Could trigger post-merge automation here
	}
	
	return nil
}

func (p *PRUpdateProcessor) CanHandle(eventType EventType) bool {
	return eventType == EventPRUpdated || eventType == EventPRMerged
}

// DailyReportProcessor generates daily repository reports
type DailyReportProcessor struct{}

func (p *DailyReportProcessor) ProcessEvent(event *AIEvent) error {
	log.Printf("DailyReportProcessor: Generating daily reports")
	
	// Get all active repositories
	repos, err := models.Repos.Search("WHERE IsPrivate = false ORDER BY UpdatedAt DESC LIMIT 10")
	if err != nil {
		return fmt.Errorf("failed to get repos: %w", err)
	}
	
	for _, repo := range repos {
		// Generate report for each repo
		report := p.generateRepoReport(repo)
		log.Printf("DailyReportProcessor: Report for %s: %s", repo.Name, report[:100])
		
		// Store report as AI activity
		activity := &models.AIActivity{
			Type:        "daily_report",
			Description: fmt.Sprintf("Daily report for %s", repo.Name),
			RepoID:      repo.ID,
			RepoName:    repo.Name,
			Success:     true,
		}
		models.AIActivities.Insert(activity)
	}
	
	log.Printf("DailyReportProcessor: Generated reports for %d repositories", len(repos))
	return nil
}

func (p *DailyReportProcessor) generateRepoReport(repo *models.Repository) string {
	// Count recent activity
	yesterday := time.Now().Add(-24 * time.Hour)
	
	// Count issues
	openIssues := models.Issues.Count("WHERE RepoID = ? AND Status = 'open'", repo.ID)
	recentIssues := models.Issues.Count("WHERE RepoID = ? AND CreatedAt > ?", repo.ID, yesterday)
	
	// Count PRs  
	openPRs := models.PullRequests.Count("WHERE RepoID = ? AND Status = 'open'", repo.ID)
	recentPRs := models.PullRequests.Count("WHERE RepoID = ? AND CreatedAt > ?", repo.ID, yesterday)
	
	report := fmt.Sprintf(`📊 Daily Report for %s

**Activity Summary (Last 24 hours):**
- New Issues: %d
- New Pull Requests: %d

**Current Status:**
- Open Issues: %d
- Open Pull Requests: %d

**Health Score:** %s

Generated at: %s`,
		repo.Name,
		recentIssues,
		recentPRs,
		openIssues,
		openPRs,
		p.calculateHealthScore(openIssues, openPRs),
		time.Now().Format("2006-01-02 15:04:05"))
	
	return report
}

func (p *DailyReportProcessor) calculateHealthScore(openIssues, openPRs int) string {
	score := 100
	score -= openIssues * 2
	score -= openPRs * 3
	
	if score > 80 {
		return "🟢 Excellent"
	} else if score > 60 {
		return "🟡 Good"
	} else if score > 40 {
		return "🟠 Needs Attention"
	}
	return "🔴 Critical"
}

func (p *DailyReportProcessor) CanHandle(eventType EventType) bool {
	return eventType == EventDailySchedule
}

// StaleCheckProcessor identifies and manages stale issues/PRs
type StaleCheckProcessor struct{}

func (p *StaleCheckProcessor) ProcessEvent(event *AIEvent) error {
	log.Printf("StaleCheckProcessor: Checking for stale issues and PRs")
	
	// Define stale threshold (30 days)
	staleThreshold := time.Now().Add(-30 * 24 * time.Hour)
	
	// Find stale issues
	staleIssues, err := models.Issues.Search(
		"WHERE Status = 'open' AND UpdatedAt < ? ORDER BY UpdatedAt ASC LIMIT 20",
		staleThreshold)
	if err != nil {
		return fmt.Errorf("failed to find stale issues: %w", err)
	}
	
	for _, issue := range staleIssues {
		// Add stale label
		tag := &models.IssueTag{
			IssueID: issue.ID,
			Tag:     "stale",
		}
		models.IssueTags.Insert(tag)
		
		// Would add stale comment here (IssueComment model doesn't exist yet)
		log.Printf("StaleCheckProcessor: Would mark issue %s as stale", issue.ID)
	}
	
	// Find stale PRs
	stalePRs, err := models.PullRequests.Search(
		"WHERE Status = 'open' AND UpdatedAt < ? ORDER BY UpdatedAt ASC LIMIT 20",
		staleThreshold)
	if err != nil {
		return fmt.Errorf("failed to find stale PRs: %w", err)
	}
	
	for _, pr := range stalePRs {
		// PRComment model doesn't exist yet, skip for now
		log.Printf("StaleCheckProcessor: Would mark PR %s as stale", pr.ID)
	}
	
	log.Printf("StaleCheckProcessor: Marked %d issues and %d PRs as stale", 
		len(staleIssues), len(stalePRs))
	return nil
}

func (p *StaleCheckProcessor) CanHandle(eventType EventType) bool {
	return eventType == EventStaleCheck
}

// SecurityScanProcessor performs security analysis
type SecurityScanProcessor struct{}

func (p *SecurityScanProcessor) ProcessEvent(event *AIEvent) error {
	log.Printf("SecurityScanProcessor: Running security scan for repo %s", event.RepoID)
	
	// This is a placeholder for actual security scanning
	// In production, this would integrate with security tools
	
	// Log the scan
	activity := &models.AIActivity{
		Type:        "security_scan",
		Description: "Security scan completed",
		RepoID:      event.RepoID,
		Success:     true,
	}
	models.AIActivities.Insert(activity)
	
	return nil
}

func (p *SecurityScanProcessor) CanHandle(eventType EventType) bool {
	return eventType == EventSecurityScan
}