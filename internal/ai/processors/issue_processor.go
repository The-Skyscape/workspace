// Package processors implements AI task processors for various automation tasks
package processors

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"workspace/internal/ai/analysis"
	"workspace/internal/ai/queue"
	"workspace/models"
)

// IssueProcessor handles issue triage and analysis
type IssueProcessor struct {
	analyzer *analysis.IssueAnalyzer
}

// NewIssueProcessor creates a new issue processor
func NewIssueProcessor() *IssueProcessor {
	return &IssueProcessor{
		analyzer: analysis.NewIssueAnalyzer(),
	}
}

// Process handles issue triage tasks
func (p *IssueProcessor) Process(ctx context.Context, task *queue.Task) error {
	issueID, ok := task.Data["issue_id"].(string)
	if !ok || issueID == "" {
		return fmt.Errorf("invalid issue ID in task data")
	}
	
	// Get the issue
	issue, err := models.Issues.Get(issueID)
	if err != nil {
		return fmt.Errorf("failed to get issue %s: %w", issueID, err)
	}
	
	// Analyze the issue
	result, err := p.analyzer.Analyze(ctx, issue)
	if err != nil {
		return fmt.Errorf("failed to analyze issue: %w", err)
	}
	
	// Update issue with analysis results
	if err := p.applyAnalysis(issue, result); err != nil {
		return fmt.Errorf("failed to apply analysis: %w", err)
	}
	
	// Post analysis comment
	if err := p.postComment(task, issue, result); err != nil {
		return fmt.Errorf("failed to post comment: %w", err)
	}
	
	task.Result = result
	return nil
}

// CanHandle checks if this processor can handle the given task type
func (p *IssueProcessor) CanHandle(taskType queue.TaskType) bool {
	return taskType == queue.TaskIssueTriage
}

// applyAnalysis applies the analysis results to the issue
func (p *IssueProcessor) applyAnalysis(issue *models.Issue, result *analysis.IssueAnalysis) error {
	updated := false
	
	// Set priority if not already set
	if issue.Priority == "" && result.Priority != "" {
		issue.Priority = result.Priority
		updated = true
	}
	
	// Add labels as metadata
	if len(result.Labels) > 0 {
		metadata := map[string]interface{}{
			"labels":     result.Labels,
			"categories": result.Categories,
			"sentiment":  result.Sentiment,
		}
		
		metadataJSON, err := json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
		
		issue.Metadata = string(metadataJSON)
		updated = true
	}
	
	// Update issue if changes were made
	if updated {
		if err := models.Issues.Update(issue); err != nil {
			return fmt.Errorf("failed to update issue: %w", err)
		}
		
		log.Printf("IssueProcessor: Updated issue %s with priority=%s, labels=%v",
			issue.ID, issue.Priority, result.Labels)
	}
	
	return nil
}

// postComment posts an analysis comment on the issue
func (p *IssueProcessor) postComment(task *queue.Task, issue *models.Issue, result *analysis.IssueAnalysis) error {
	commentBody := p.formatComment(issue, result)
	
	comment := &models.Comment{
		Body:       commentBody,
		AuthorID:   task.UserID,
		IssueID:    issue.ID,
		EntityType: "issue",
		EntityID:   issue.ID,
		RepoID:     task.RepoID,
	}
	
	if _, err := models.Comments.Insert(comment); err != nil {
		return fmt.Errorf("failed to insert comment: %w", err)
	}
	
	log.Printf("IssueProcessor: Posted analysis comment on issue %s", issue.ID)
	return nil
}

// formatComment formats the analysis results as a comment
func (p *IssueProcessor) formatComment(issue *models.Issue, result *analysis.IssueAnalysis) string {
	var b strings.Builder
	
	b.WriteString("## 🤖 AI Analysis\n\n")
	
	// Priority section
	b.WriteString("### Priority Assessment\n")
	b.WriteString(fmt.Sprintf("**Assigned Priority:** `%s`\n", result.Priority))
	if result.PriorityReason != "" {
		b.WriteString(fmt.Sprintf("**Reasoning:** %s\n", result.PriorityReason))
	}
	b.WriteString("\n")
	
	// Labels section
	if len(result.Labels) > 0 {
		b.WriteString("### Suggested Labels\n")
		for _, label := range result.Labels {
			b.WriteString(fmt.Sprintf("- `%s`\n", label))
		}
		b.WriteString("\n")
	}
	
	// Categories section
	if len(result.Categories) > 0 {
		b.WriteString("### Categories\n")
		for category, confidence := range result.Categories {
			emoji := getCategoryEmoji(category)
			b.WriteString(fmt.Sprintf("%s **%s** (%.0f%% confidence)\n", 
				emoji, category, confidence*100))
		}
		b.WriteString("\n")
	}
	
	// Analysis insights
	if len(result.Insights) > 0 {
		b.WriteString("### Insights\n")
		for _, insight := range result.Insights {
			b.WriteString(fmt.Sprintf("- %s\n", insight))
		}
		b.WriteString("\n")
	}
	
	// Recommendations
	if len(result.Recommendations) > 0 {
		b.WriteString("### Recommended Actions\n")
		for i, rec := range result.Recommendations {
			b.WriteString(fmt.Sprintf("%d. %s\n", i+1, rec))
		}
		b.WriteString("\n")
	}
	
	// Similar issues
	if len(result.SimilarIssues) > 0 {
		b.WriteString("### Similar Issues\n")
		b.WriteString("These existing issues might be related:\n")
		for _, similarID := range result.SimilarIssues {
			b.WriteString(fmt.Sprintf("- #%s\n", similarID))
		}
		b.WriteString("\n")
	}
	
	// Security notice if applicable
	if result.HasSecurity {
		b.WriteString("---\n")
		b.WriteString("⚠️ **Security Notice:** This issue may have security implications. ")
		b.WriteString("Please handle with appropriate care and avoid exposing sensitive details.\n")
	}
	
	// Footer
	b.WriteString("\n---\n")
	b.WriteString("*This analysis was performed automatically. ")
	b.WriteString("Human review is recommended for critical decisions.*\n")
	
	return b.String()
}

// getCategoryEmoji returns an emoji for a category
func getCategoryEmoji(category string) string {
	emojiMap := map[string]string{
		"bug":           "🐛",
		"enhancement":   "✨",
		"documentation": "📚",
		"performance":   "⚡",
		"security":      "🔒",
		"ui/ux":         "🎨",
		"testing":       "🧪",
		"refactoring":   "♻️",
		"infrastructure":"🏗️",
		"question":      "❓",
		"discussion":    "💬",
	}
	
	if emoji, ok := emojiMap[strings.ToLower(category)]; ok {
		return emoji
	}
	return "📌"
}