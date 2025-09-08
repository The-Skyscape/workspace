// Package analysis provides intelligent analysis capabilities for AI automation
package analysis

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"workspace/models"
)

// IssueAnalysis contains the results of issue analysis
type IssueAnalysis struct {
	Priority          string                 `json:"priority"`
	PriorityReason    string                 `json:"priority_reason"`
	Labels            []string               `json:"labels"`
	Categories        map[string]float64     `json:"categories"`
	Sentiment         string                 `json:"sentiment"`
	Urgency           int                    `json:"urgency"`
	Impact            int                    `json:"impact"`
	Insights          []string               `json:"insights"`
	Recommendations   []string               `json:"recommendations"`
	SimilarIssues     []string               `json:"similar_issues"`
	HasSecurity       bool                   `json:"has_security"`
	EstimatedEffort   string                 `json:"estimated_effort"`
}

// IssueAnalyzer performs intelligent issue analysis
type IssueAnalyzer struct {
	patterns      map[string][]*regexp.Regexp
	priorityRules []PriorityRule
	labelRules    []LabelRule
}

// PriorityRule defines a rule for priority assignment
type PriorityRule struct {
	Priority   string
	Keywords   []string
	Patterns   []*regexp.Regexp
	MinUrgency int
	MinImpact  int
	Reason     string
}

// LabelRule defines a rule for label assignment
type LabelRule struct {
	Label      string
	Keywords   []string
	Patterns   []*regexp.Regexp
	Confidence float64
}

// NewIssueAnalyzer creates a new issue analyzer
func NewIssueAnalyzer() *IssueAnalyzer {
	analyzer := &IssueAnalyzer{
		patterns: make(map[string][]*regexp.Regexp),
	}
	
	analyzer.initializeRules()
	return analyzer
}

// Analyze performs comprehensive issue analysis
func (a *IssueAnalyzer) Analyze(ctx context.Context, issue *models.Issue) (*IssueAnalysis, error) {
	result := &IssueAnalysis{
		Categories:      make(map[string]float64),
		Labels:          []string{},
		Insights:        []string{},
		Recommendations: []string{},
		SimilarIssues:   []string{},
	}
	
	// Combine title and description for analysis
	content := strings.ToLower(issue.Title + " " + issue.Description)
	
	// Analyze sentiment
	result.Sentiment = a.analyzeSentiment(content)
	
	// Calculate urgency and impact
	result.Urgency = a.calculateUrgency(content)
	result.Impact = a.calculateImpact(content)
	
	// Detect categories and labels
	a.detectCategories(content, result)
	a.detectLabels(content, result)
	
	// Determine priority
	result.Priority, result.PriorityReason = a.determinePriority(content, result)
	
	// Check for security implications
	result.HasSecurity = a.hasSecurityImplications(content)
	
	// Estimate effort
	result.EstimatedEffort = a.estimateEffort(issue, result)
	
	// Generate insights
	a.generateInsights(issue, result)
	
	// Generate recommendations
	a.generateRecommendations(issue, result)
	
	// Find similar issues
	if err := a.findSimilarIssues(ctx, issue, result); err != nil {
		// Log but don't fail
		fmt.Printf("Failed to find similar issues: %v\n", err)
	}
	
	return result, nil
}

// initializeRules sets up analysis rules
func (a *IssueAnalyzer) initializeRules() {
	// Priority rules
	a.priorityRules = []PriorityRule{
		{
			Priority: "critical",
			Keywords: []string{"critical", "urgent", "emergency", "data loss", "security vulnerability", 
				"production down", "major outage", "breaking", "crash"},
			Patterns: []*regexp.Regexp{
				regexp.MustCompile(`(system|app|service)\s+(is\s+)?down`),
				regexp.MustCompile(`can'?t\s+(access|login|connect)`),
				regexp.MustCompile(`(losing|lost)\s+data`),
			},
			MinUrgency: 8,
			MinImpact:  8,
			Reason:     "Contains critical keywords or patterns indicating severe issues",
		},
		{
			Priority: "high",
			Keywords: []string{"high priority", "important", "blocking", "regression", "broken", 
				"cannot", "failure", "error"},
			Patterns: []*regexp.Regexp{
				regexp.MustCompile(`block(ing|ed|s)`),
				regexp.MustCompile(`(feature|function)\s+not\s+work`),
			},
			MinUrgency: 6,
			MinImpact:  6,
			Reason:     "Issue is blocking work or represents a regression",
		},
		{
			Priority: "low",
			Keywords: []string{"minor", "trivial", "nice to have", "someday", "cosmetic", 
				"typo", "polish"},
			MinUrgency: 0,
			MinImpact:  3,
			Reason:     "Issue is minor or cosmetic in nature",
		},
		{
			Priority:   "medium",
			Keywords:   []string{},
			MinUrgency: 3,
			MinImpact:  3,
			Reason:     "Standard priority for typical issues",
		},
	}
	
	// Label rules
	a.labelRules = []LabelRule{
		{
			Label:    "bug",
			Keywords: []string{"bug", "error", "crash", "fail", "broken", "not working", "issue", "problem"},
			Patterns: []*regexp.Regexp{
				regexp.MustCompile(`doesn'?t\s+work`),
				regexp.MustCompile(`(should|supposed\s+to)\s+but`),
			},
			Confidence: 0.8,
		},
		{
			Label:    "enhancement",
			Keywords: []string{"feature", "request", "add", "implement", "improve", "enhance", "would be nice"},
			Patterns: []*regexp.Regexp{
				regexp.MustCompile(`would\s+be\s+(nice|good|great)`),
				regexp.MustCompile(`(add|implement|create)\s+.*\s+feature`),
			},
			Confidence: 0.7,
		},
		{
			Label:    "documentation",
			Keywords: []string{"documentation", "docs", "readme", "tutorial", "guide", "explain"},
			Patterns: []*regexp.Regexp{
				regexp.MustCompile(`update\s+.*\s+doc`),
				regexp.MustCompile(`document\s+`),
			},
			Confidence: 0.9,
		},
		{
			Label:    "performance",
			Keywords: []string{"slow", "performance", "optimize", "speed", "fast", "lag", "latency"},
			Patterns: []*regexp.Regexp{
				regexp.MustCompile(`(too|very|extremely)\s+slow`),
				regexp.MustCompile(`takes\s+.*\s+(long|time)`),
			},
			Confidence: 0.75,
		},
		{
			Label:    "security",
			Keywords: []string{"security", "vulnerability", "exploit", "injection", "xss", "csrf", "auth"},
			Patterns: []*regexp.Regexp{
				regexp.MustCompile(`(sql|script)\s+injection`),
				regexp.MustCompile(`(auth|authentication|authorization)\s+`),
			},
			Confidence: 0.95,
		},
		{
			Label:    "ui/ux",
			Keywords: []string{"ui", "ux", "interface", "design", "layout", "style", "css", "visual"},
			Patterns: []*regexp.Regexp{
				regexp.MustCompile(`user\s+interface`),
				regexp.MustCompile(`look\s+and\s+feel`),
			},
			Confidence: 0.7,
		},
		{
			Label:    "testing",
			Keywords: []string{"test", "testing", "unit test", "integration test", "coverage"},
			Patterns: []*regexp.Regexp{
				regexp.MustCompile(`(add|write|create)\s+test`),
				regexp.MustCompile(`test\s+coverage`),
			},
			Confidence: 0.8,
		},
		{
			Label:    "refactoring",
			Keywords: []string{"refactor", "cleanup", "technical debt", "reorganize", "restructure"},
			Patterns: []*regexp.Regexp{
				regexp.MustCompile(`clean\s+up`),
				regexp.MustCompile(`code\s+smell`),
			},
			Confidence: 0.75,
		},
	}
}

// analyzeSentiment determines the overall sentiment of the issue
func (a *IssueAnalyzer) analyzeSentiment(content string) string {
	positiveWords := []string{"great", "good", "nice", "excellent", "love", "awesome", "perfect", "thanks"}
	negativeWords := []string{"bad", "terrible", "hate", "awful", "horrible", "broken", "fail", "error", "bug"}
	urgentWords := []string{"urgent", "asap", "immediately", "critical", "emergency"}
	
	positiveCount := countWords(content, positiveWords)
	negativeCount := countWords(content, negativeWords)
	urgentCount := countWords(content, urgentWords)
	
	if urgentCount > 0 {
		return "urgent"
	}
	if negativeCount > positiveCount*2 {
		return "negative"
	}
	if positiveCount > negativeCount*2 {
		return "positive"
	}
	return "neutral"
}

// calculateUrgency determines how urgent the issue is (1-10)
func (a *IssueAnalyzer) calculateUrgency(content string) int {
	urgency := 5 // Default medium urgency
	
	// Increase for urgent keywords
	urgentKeywords := []string{"urgent", "asap", "immediately", "critical", "emergency", "blocking"}
	for _, keyword := range urgentKeywords {
		if strings.Contains(content, keyword) {
			urgency += 2
		}
	}
	
	// Check for production issues
	if strings.Contains(content, "production") || strings.Contains(content, "prod") {
		urgency += 3
	}
	
	// Check for data loss
	if strings.Contains(content, "data loss") || strings.Contains(content, "lost data") {
		urgency = 10
	}
	
	// Cap at 10
	if urgency > 10 {
		urgency = 10
	}
	
	return urgency
}

// calculateImpact determines the impact level (1-10)
func (a *IssueAnalyzer) calculateImpact(content string) int {
	impact := 5 // Default medium impact
	
	// High impact keywords
	highImpactKeywords := []string{"all users", "everyone", "system", "platform", "core", "critical"}
	for _, keyword := range highImpactKeywords {
		if strings.Contains(content, keyword) {
			impact += 2
		}
	}
	
	// Low impact keywords
	lowImpactKeywords := []string{"minor", "small", "edge case", "rare", "specific"}
	for _, keyword := range lowImpactKeywords {
		if strings.Contains(content, keyword) {
			impact -= 2
		}
	}
	
	// Security issues have high impact
	if strings.Contains(content, "security") || strings.Contains(content, "vulnerability") {
		impact = 9
	}
	
	// Bound between 1 and 10
	if impact < 1 {
		impact = 1
	}
	if impact > 10 {
		impact = 10
	}
	
	return impact
}

// detectCategories identifies issue categories
func (a *IssueAnalyzer) detectCategories(content string, result *IssueAnalysis) {
	for _, rule := range a.labelRules {
		confidence := 0.0
		matches := 0
		
		// Check keywords
		for _, keyword := range rule.Keywords {
			if strings.Contains(content, keyword) {
				matches++
			}
		}
		
		// Check patterns
		for _, pattern := range rule.Patterns {
			if pattern.MatchString(content) {
				matches += 2 // Patterns are weighted more
			}
		}
		
		if matches > 0 {
			confidence = rule.Confidence * (float64(matches) / float64(len(rule.Keywords)+len(rule.Patterns)*2))
			if confidence > 0.3 { // Threshold for category detection
				result.Categories[rule.Label] = confidence
			}
		}
	}
}

// detectLabels assigns labels based on content analysis
func (a *IssueAnalyzer) detectLabels(content string, result *IssueAnalysis) {
	labelSet := make(map[string]bool)
	
	for category, confidence := range result.Categories {
		if confidence > 0.5 { // Higher threshold for actual labels
			labelSet[category] = true
		}
	}
	
	// Convert to slice
	for label := range labelSet {
		result.Labels = append(result.Labels, label)
	}
}

// determinePriority determines the issue priority
func (a *IssueAnalyzer) determinePriority(content string, result *IssueAnalysis) (string, string) {
	for _, rule := range a.priorityRules {
		// Check urgency and impact thresholds
		if result.Urgency >= rule.MinUrgency && result.Impact >= rule.MinImpact {
			return rule.Priority, rule.Reason
		}
		
		// Check keywords
		for _, keyword := range rule.Keywords {
			if strings.Contains(content, keyword) {
				return rule.Priority, rule.Reason
			}
		}
		
		// Check patterns
		for _, pattern := range rule.Patterns {
			if pattern.MatchString(content) {
				return rule.Priority, rule.Reason
			}
		}
	}
	
	// Default to medium
	return "medium", "Standard priority based on content analysis"
}

// hasSecurityImplications checks for security-related content
func (a *IssueAnalyzer) hasSecurityImplications(content string) bool {
	securityKeywords := []string{
		"security", "vulnerability", "exploit", "injection", "xss", "csrf",
		"authentication", "authorization", "password", "token", "credential",
		"sensitive", "privacy", "leak", "exposure",
	}
	
	for _, keyword := range securityKeywords {
		if strings.Contains(content, keyword) {
			return true
		}
	}
	return false
}

// estimateEffort estimates the effort required
func (a *IssueAnalyzer) estimateEffort(issue *models.Issue, result *IssueAnalysis) string {
	// Simple estimation based on categories and content length
	contentLength := len(issue.Title) + len(issue.Description)
	
	if contentLength < 100 && len(result.Labels) <= 1 {
		return "small"
	}
	if contentLength < 500 && len(result.Labels) <= 2 {
		return "medium"
	}
	if result.HasSecurity || len(result.Labels) > 3 {
		return "large"
	}
	
	return "medium"
}

// generateInsights generates analytical insights
func (a *IssueAnalyzer) generateInsights(issue *models.Issue, result *IssueAnalysis) {
	// Sentiment-based insights
	if result.Sentiment == "urgent" {
		result.Insights = append(result.Insights, 
			"This issue appears to be time-sensitive and may require immediate attention")
	}
	
	// Category-based insights
	if confidence, ok := result.Categories["bug"]; ok && confidence > 0.7 {
		result.Insights = append(result.Insights,
			"This appears to be a bug report that may affect system functionality")
	}
	
	if confidence, ok := result.Categories["security"]; ok && confidence > 0.5 {
		result.Insights = append(result.Insights,
			"Security implications detected - consider private discussion and responsible disclosure")
	}
	
	// Effort insights
	if result.EstimatedEffort == "large" {
		result.Insights = append(result.Insights,
			"This issue may require significant effort and should be carefully planned")
	}
	
	// Impact insights
	if result.Impact >= 8 {
		result.Insights = append(result.Insights,
			"High impact issue affecting multiple users or core functionality")
	}
}

// generateRecommendations creates actionable recommendations
func (a *IssueAnalyzer) generateRecommendations(issue *models.Issue, result *IssueAnalysis) {
	// Priority-based recommendations
	switch result.Priority {
	case "critical":
		result.Recommendations = append(result.Recommendations,
			"Assign to senior team member immediately",
			"Create incident response if in production",
			"Consider hotfix deployment process")
	case "high":
		result.Recommendations = append(result.Recommendations,
			"Schedule for current sprint",
			"Assign to available developer",
			"Request additional details if needed")
	case "low":
		result.Recommendations = append(result.Recommendations,
			"Add to backlog for future consideration",
			"Consider combining with related improvements",
			"Document as known issue if not fixing immediately")
	default:
		result.Recommendations = append(result.Recommendations,
			"Review in next planning session",
			"Gather more information if needed",
			"Assign appropriate milestone")
	}
	
	// Category-specific recommendations
	if _, ok := result.Categories["bug"]; ok {
		result.Recommendations = append(result.Recommendations,
			"Request steps to reproduce",
			"Verify in latest version",
			"Add test coverage after fix")
	}
	
	if _, ok := result.Categories["enhancement"]; ok {
		result.Recommendations = append(result.Recommendations,
			"Define acceptance criteria",
			"Consider user impact and adoption",
			"Plan implementation approach")
	}
	
	if result.HasSecurity {
		result.Recommendations = append(result.Recommendations,
			"Conduct security review",
			"Keep discussion private until patched",
			"Prepare security advisory if needed")
	}
}

// findSimilarIssues finds potentially related issues
func (a *IssueAnalyzer) findSimilarIssues(ctx context.Context, issue *models.Issue, result *IssueAnalysis) error {
	// Get recent issues from the same repository
	recentIssues, err := models.Issues.Search("WHERE RepoID = ? AND ID != ? ORDER BY CreatedAt DESC LIMIT 20", 
		issue.RepoID, issue.ID)
	if err != nil {
		return err
	}
	
	// Simple similarity check based on keywords
	issueWords := extractKeywords(issue.Title + " " + issue.Description)
	
	for _, other := range recentIssues {
		otherWords := extractKeywords(other.Title + " " + other.Description)
		similarity := calculateSimilarity(issueWords, otherWords)
		
		if similarity > 0.3 { // 30% similarity threshold
			result.SimilarIssues = append(result.SimilarIssues, other.ID)
			if len(result.SimilarIssues) >= 3 {
				break // Limit to 3 similar issues
			}
		}
	}
	
	return nil
}

// Helper functions

func countWords(content string, words []string) int {
	count := 0
	for _, word := range words {
		count += strings.Count(content, word)
	}
	return count
}

func extractKeywords(text string) map[string]int {
	keywords := make(map[string]int)
	words := strings.Fields(strings.ToLower(text))
	
	for _, word := range words {
		// Clean the word
		word = strings.TrimFunc(word, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsNumber(r)
		})
		
		// Skip short words and common words
		if len(word) > 3 && !isCommonWord(word) {
			keywords[word]++
		}
	}
	
	return keywords
}

func calculateSimilarity(words1, words2 map[string]int) float64 {
	if len(words1) == 0 || len(words2) == 0 {
		return 0
	}
	
	intersection := 0
	total := len(words1) + len(words2)
	
	for word := range words1 {
		if _, exists := words2[word]; exists {
			intersection++
		}
	}
	
	return float64(intersection*2) / float64(total)
}

func isCommonWord(word string) bool {
	commonWords := map[string]bool{
		"the": true, "and": true, "is": true, "it": true, "to": true,
		"of": true, "in": true, "for": true, "on": true, "with": true,
		"as": true, "at": true, "by": true, "an": true, "be": true,
		"this": true, "that": true, "from": true, "or": true, "but": true,
		"not": true, "are": true, "was": true, "were": true, "been": true,
		"have": true, "has": true, "had": true, "will": true, "would": true,
		"can": true, "could": true, "should": true, "may": true, "might": true,
	}
	return commonWords[word]
}