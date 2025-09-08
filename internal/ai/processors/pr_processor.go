package processors

import (
	"context"
	"fmt"
	"log"
	"strings"

	"workspace/internal/ai/analysis"
	"workspace/internal/ai/queue"
	"workspace/models"
)

// PRProcessor handles pull request review and analysis
type PRProcessor struct {
	analyzer *analysis.PRAnalyzer
}

// NewPRProcessor creates a new PR processor
func NewPRProcessor() *PRProcessor {
	return &PRProcessor{
		analyzer: analysis.NewPRAnalyzer(),
	}
}

// Process handles PR review tasks
func (p *PRProcessor) Process(ctx context.Context, task *queue.Task) error {
	prID, ok := task.Data["pr_id"].(string)
	if !ok || prID == "" {
		return fmt.Errorf("invalid PR ID in task data")
	}
	
	// Get the pull request
	pr, err := models.PullRequests.Get(prID)
	if err != nil {
		return fmt.Errorf("failed to get PR %s: %w", prID, err)
	}
	
	// Analyze the PR
	result, err := p.analyzer.Analyze(ctx, pr)
	if err != nil {
		return fmt.Errorf("failed to analyze PR: %w", err)
	}
	
	// Check for auto-approval
	if result.AutoApprovalEligible && result.RiskLevel == "low" {
		if err := p.autoApprovePR(task, pr); err != nil {
			log.Printf("PRProcessor: Failed to auto-approve PR %s: %v", prID, err)
		}
	}
	
	// Post review comment
	if err := p.postReview(task, pr, result); err != nil {
		return fmt.Errorf("failed to post review: %w", err)
	}
	
	// Update PR metadata if needed
	if err := p.updatePRMetadata(pr, result); err != nil {
		log.Printf("PRProcessor: Failed to update PR metadata: %v", err)
	}
	
	task.Result = result
	return nil
}

// CanHandle checks if this processor can handle the given task type
func (p *PRProcessor) CanHandle(taskType queue.TaskType) bool {
	return taskType == queue.TaskPRReview || taskType == queue.TaskAutoApprove
}

// autoApprovePR automatically approves a PR if eligible
func (p *PRProcessor) autoApprovePR(task *queue.Task, pr *models.PullRequest) error {
	// Update PR status
	pr.Status = "approved"
	if err := models.PullRequests.Update(pr); err != nil {
		return fmt.Errorf("failed to update PR status: %w", err)
	}
	
	// Post approval comment
	comment := &models.Comment{
		Body: `## ✅ Auto-Approved

This pull request has been automatically approved as it contains only safe changes:
- Documentation updates
- Configuration file changes
- Non-critical dependency updates

The changes have been reviewed and determined to have minimal risk.

*Auto-approval by AI Assistant*`,
		AuthorID:   task.UserID,
		EntityType: "pull_request",
		EntityID:   pr.ID,
		RepoID:     task.RepoID,
	}
	
	if _, err := models.Comments.Insert(comment); err != nil {
		return fmt.Errorf("failed to post approval comment: %w", err)
	}
	
	log.Printf("PRProcessor: Auto-approved PR %s", pr.ID)
	return nil
}

// postReview posts a review comment on the PR
func (p *PRProcessor) postReview(task *queue.Task, pr *models.PullRequest, result *analysis.PRAnalysis) error {
	reviewBody := p.formatReview(pr, result)
	
	comment := &models.Comment{
		Body:       reviewBody,
		AuthorID:   task.UserID,
		EntityType: "pull_request",
		EntityID:   pr.ID,
		RepoID:     task.RepoID,
	}
	
	if _, err := models.Comments.Insert(comment); err != nil {
		return fmt.Errorf("failed to insert review comment: %w", err)
	}
	
	log.Printf("PRProcessor: Posted review on PR %s", pr.ID)
	return nil
}

// formatReview formats the analysis results as a review comment
func (p *PRProcessor) formatReview(pr *models.PullRequest, result *analysis.PRAnalysis) string {
	var b strings.Builder
	
	// Header with overall assessment
	b.WriteString("## 🤖 AI Code Review\n\n")
	
	// Risk assessment badge
	riskBadge := p.getRiskBadge(result.RiskLevel)
	b.WriteString(fmt.Sprintf("**Risk Level:** %s\n", riskBadge))
	b.WriteString(fmt.Sprintf("**Complexity:** %s\n", result.Complexity))
	b.WriteString(fmt.Sprintf("**Estimated Review Time:** %s\n\n", result.EstimatedReviewTime))
	
	// Change summary
	if pr.Additions > 0 || pr.Deletions > 0 || pr.ChangedFiles > 0 {
		b.WriteString("### 📊 Change Summary\n")
		b.WriteString(fmt.Sprintf("- **Files Changed:** %d\n", pr.ChangedFiles))
		b.WriteString(fmt.Sprintf("- **Lines Added:** +%d\n", pr.Additions))
		b.WriteString(fmt.Sprintf("- **Lines Deleted:** -%d\n", pr.Deletions))
		b.WriteString(fmt.Sprintf("- **Net Change:** %+d lines\n\n", pr.Additions-pr.Deletions))
	}
	
	// Categories detected
	if len(result.Categories) > 0 {
		b.WriteString("### 🏷️ Change Categories\n")
		for _, category := range result.Categories {
			emoji := getCategoryEmoji(category)
			b.WriteString(fmt.Sprintf("- %s %s\n", emoji, category))
		}
		b.WriteString("\n")
	}
	
	// Review checklist
	b.WriteString("### ✅ Review Checklist\n")
	for _, item := range result.ChecklistItems {
		status := "⬜"
		if item.Passed {
			status = "✅"
		} else if item.Warning {
			status = "⚠️"
		} else if item.Failed {
			status = "❌"
		}
		b.WriteString(fmt.Sprintf("%s %s\n", status, item.Description))
		if item.Details != "" {
			b.WriteString(fmt.Sprintf("   - %s\n", item.Details))
		}
	}
	b.WriteString("\n")
	
	// Issues found
	if len(result.Issues) > 0 {
		b.WriteString("### ⚠️ Issues Found\n")
		for i, issue := range result.Issues {
			severityEmoji := p.getSeverityEmoji(issue.Severity)
			b.WriteString(fmt.Sprintf("%d. %s **%s** - %s\n", 
				i+1, severityEmoji, issue.Type, issue.Description))
			if issue.File != "" {
				b.WriteString(fmt.Sprintf("   - File: `%s`", issue.File))
				if issue.Line > 0 {
					b.WriteString(fmt.Sprintf(":%d", issue.Line))
				}
				b.WriteString("\n")
			}
			if issue.Suggestion != "" {
				b.WriteString(fmt.Sprintf("   - Suggestion: %s\n", issue.Suggestion))
			}
		}
		b.WriteString("\n")
	}
	
	// Suggestions
	if len(result.Suggestions) > 0 {
		b.WriteString("### 💡 Suggestions\n")
		for _, suggestion := range result.Suggestions {
			b.WriteString(fmt.Sprintf("- %s\n", suggestion))
		}
		b.WriteString("\n")
	}
	
	// Security considerations
	if result.HasSecurity {
		b.WriteString("### 🔒 Security Considerations\n")
		b.WriteString("This PR may have security implications. Please ensure:\n")
		b.WriteString("- Sensitive data is not exposed\n")
		b.WriteString("- Authentication/authorization is properly implemented\n")
		b.WriteString("- Input validation is in place\n")
		b.WriteString("- Dependencies are from trusted sources\n\n")
	}
	
	// Performance impact
	if result.PerformanceImpact != "none" {
		b.WriteString("### ⚡ Performance Impact\n")
		b.WriteString(fmt.Sprintf("Expected impact: **%s**\n", result.PerformanceImpact))
		if len(result.PerformanceNotes) > 0 {
			b.WriteString("Considerations:\n")
			for _, note := range result.PerformanceNotes {
				b.WriteString(fmt.Sprintf("- %s\n", note))
			}
		}
		b.WriteString("\n")
	}
	
	// Test coverage
	if result.TestCoverage != "" {
		b.WriteString("### 🧪 Test Coverage\n")
		b.WriteString(fmt.Sprintf("%s\n\n", result.TestCoverage))
	}
	
	// Auto-approval status
	if result.AutoApprovalEligible {
		b.WriteString("### ✅ Auto-Approval Status\n")
		b.WriteString("This PR is eligible for auto-approval based on:\n")
		for _, reason := range result.AutoApprovalReasons {
			b.WriteString(fmt.Sprintf("- %s\n", reason))
		}
		b.WriteString("\n")
	}
	
	// Overall recommendation
	b.WriteString("### 📝 Recommendation\n")
	b.WriteString(fmt.Sprintf("%s\n\n", result.Recommendation))
	
	// Footer
	b.WriteString("---\n")
	b.WriteString("*This review was generated automatically by AI. ")
	b.WriteString("Human review is still recommended for critical changes.*\n")
	
	return b.String()
}

// updatePRMetadata updates PR metadata based on analysis
func (p *PRProcessor) updatePRMetadata(pr *models.PullRequest, result *analysis.PRAnalysis) error {
	// Update status if auto-approved
	if result.AutoApprovalEligible && pr.Status == "open" {
		pr.Status = "approved"
		return models.PullRequests.Update(pr)
	}
	
	// Could add more metadata updates here (labels, etc.)
	return nil
}

// getRiskBadge returns a formatted risk level badge
func (p *PRProcessor) getRiskBadge(risk string) string {
	switch risk {
	case "low":
		return "🟢 Low Risk"
	case "medium":
		return "🟡 Medium Risk"
	case "high":
		return "🔴 High Risk"
	case "critical":
		return "🔴 **CRITICAL RISK**"
	default:
		return "⚪ Unknown"
	}
}

// getSeverityEmoji returns an emoji for issue severity
func (p *PRProcessor) getSeverityEmoji(severity string) string {
	switch severity {
	case "critical":
		return "🔴"
	case "high":
		return "🟠"
	case "medium":
		return "🟡"
	case "low":
		return "🔵"
	default:
		return "⚪"
	}
}