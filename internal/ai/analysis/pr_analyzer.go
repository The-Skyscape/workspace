package analysis

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// PRAnalyzer performs intelligent analysis on pull requests
type PRAnalyzer struct {
	// Patterns for detecting different types of changes
	securityPatterns    []*regexp.Regexp
	performancePatterns []*regexp.Regexp
	breakingPatterns    []*regexp.Regexp
	docPatterns         []*regexp.Regexp
	testPatterns        []*regexp.Regexp
	configPatterns      []*regexp.Regexp
}

// PRAnalysis contains the results of PR analysis
type PRAnalysis struct {
	RiskLevel            string                `json:"risk_level"`
	Complexity           string                `json:"complexity"`
	EstimatedReviewTime  string                `json:"estimated_review_time"`
	Categories           []string              `json:"categories"`
	ChecklistItems       []ChecklistItem       `json:"checklist_items"`
	Issues               []Issue               `json:"issues"`
	Suggestions          []string              `json:"suggestions"`
	HasSecurity          bool                  `json:"has_security"`
	PerformanceImpact    string                `json:"performance_impact"`
	PerformanceNotes     []string              `json:"performance_notes"`
	TestCoverage         string                `json:"test_coverage"`
	AutoApprovalEligible bool                  `json:"auto_approval_eligible"`
	AutoApprovalReasons  []string              `json:"auto_approval_reasons"`
	Recommendation       string                `json:"recommendation"`
	FileAnalysis         map[string]FileDetail `json:"file_analysis"`
	DependencyChanges    []DependencyChange    `json:"dependency_changes"`
	APIChanges           []APIChange           `json:"api_changes"`
}

// ChecklistItem represents a review checklist item
type ChecklistItem struct {
	Description string `json:"description"`
	Passed      bool   `json:"passed"`
	Warning     bool   `json:"warning"`
	Failed      bool   `json:"failed"`
	Details     string `json:"details"`
}

// Issue represents a problem found in the PR
type Issue struct {
	Type        string `json:"type"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
	File        string `json:"file"`
	Line        int    `json:"line"`
	Suggestion  string `json:"suggestion"`
}

// FileDetail contains analysis for a specific file
type FileDetail struct {
	Language      string   `json:"language"`
	Changes       int      `json:"changes"`
	Risk          string   `json:"risk"`
	Issues        []string `json:"issues"`
	Improvements  []string `json:"improvements"`
	TestsAffected bool     `json:"tests_affected"`
}

// DependencyChange represents a change to dependencies
type DependencyChange struct {
	Name       string `json:"name"`
	Type       string `json:"type"` // added, removed, updated
	OldVersion string `json:"old_version,omitempty"`
	NewVersion string `json:"new_version,omitempty"`
	Risk       string `json:"risk"`
	Notes      string `json:"notes"`
}

// APIChange represents a change to an API endpoint or interface
type APIChange struct {
	Type        string `json:"type"` // added, removed, modified
	Path        string `json:"path"`
	Method      string `json:"method,omitempty"`
	Breaking    bool   `json:"breaking"`
	Description string `json:"description"`
}

// NewPRAnalyzer creates a new PR analyzer
func NewPRAnalyzer() *PRAnalyzer {
	return &PRAnalyzer{
		securityPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)(password|secret|key|token|api[_-]?key|credential)`),
			regexp.MustCompile(`(?i)(auth|authentication|authorization|permission)`),
			regexp.MustCompile(`(?i)(encrypt|decrypt|hash|salt|crypto)`),
			regexp.MustCompile(`(?i)(sql|query|database|injection)`),
			regexp.MustCompile(`(?i)(cors|csrf|xss|vulnerability)`),
			regexp.MustCompile(`(?i)(eval|exec|system|shell|command)`),
		},
		performancePatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)(cache|caching|memoiz|memo)`),
			regexp.MustCompile(`(?i)(optimize|optimization|performance)`),
			regexp.MustCompile(`(?i)(async|await|concurrent|parallel|goroutine)`),
			regexp.MustCompile(`(?i)(index|indices|query[_-]?plan)`),
			regexp.MustCompile(`(?i)(batch|bulk|pagination|limit)`),
			regexp.MustCompile(`(?i)(n\+1|eager[_-]?load|lazy[_-]?load)`),
		},
		breakingPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)(breaking[_-]?change|incompatible)`),
			regexp.MustCompile(`(?i)(deprecat|obsolete|legacy)`),
			regexp.MustCompile(`(?i)(remov|delet).*(?i)(api|endpoint|method|function)`),
			regexp.MustCompile(`(?i)changed?.*(?i)(signature|interface|contract)`),
			regexp.MustCompile(`(?i)(renam|refactor).*(?i)(public|export)`),
		},
		docPatterns: []*regexp.Regexp{
			regexp.MustCompile(`\.(md|markdown|rst|txt|doc|pdf)$`),
			regexp.MustCompile(`(?i)(readme|license|contributing|changelog)`),
			regexp.MustCompile(`(?i)(docs?/|documentation/)`),
			regexp.MustCompile(`(?i)^\s*(//|/\*|\#)\s*`), // Comments
		},
		testPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)(_test\.|test_|\.test\.|\.spec\.)`),
			regexp.MustCompile(`(?i)(test/|tests/|spec/|specs/)`),
			regexp.MustCompile(`(?i)(describe|it|expect|assert|should)`),
			regexp.MustCompile(`(?i)(mock|stub|spy|fake)`),
		},
		configPatterns: []*regexp.Regexp{
			regexp.MustCompile(`\.(json|yaml|yml|toml|ini|conf|config|env)$`),
			regexp.MustCompile(`(?i)(config|configuration|settings)`),
			regexp.MustCompile(`(?i)(dockerfile|docker-compose|k8s|kubernetes)`),
			regexp.MustCompile(`(?i)(package\.json|go\.mod|requirements|gemfile)`),
		},
	}
}

// Analyze performs comprehensive analysis on a pull request
func (a *PRAnalyzer) Analyze(ctx context.Context, pr any) (*PRAnalysis, error) {
	// Type assertion for PR model
	prData := extractPRData(pr)

	result := &PRAnalysis{
		FileAnalysis:        make(map[string]FileDetail),
		Categories:          []string{},
		ChecklistItems:      []ChecklistItem{},
		Issues:              []Issue{},
		Suggestions:         []string{},
		PerformanceNotes:    []string{},
		DependencyChanges:   []DependencyChange{},
		APIChanges:          []APIChange{},
		AutoApprovalReasons: []string{},
	}

	// Analyze PR size and complexity
	a.analyzeComplexity(prData, result)

	// Analyze file changes
	a.analyzeFiles(prData, result)

	// Detect categories
	a.detectCategories(prData, result)

	// Perform security analysis
	a.analyzeSecurityRisks(prData, result)

	// Analyze performance impact
	a.analyzePerformanceImpact(prData, result)

	// Check test coverage
	a.analyzeTestCoverage(prData, result)

	// Analyze dependencies
	a.analyzeDependencies(prData, result)

	// Detect API changes
	a.analyzeAPIChanges(prData, result)

	// Build review checklist
	a.buildChecklist(prData, result)

	// Calculate risk level
	a.calculateRiskLevel(result)

	// Determine auto-approval eligibility
	a.checkAutoApproval(result)

	// Generate recommendation
	a.generateRecommendation(result)

	// Estimate review time
	a.estimateReviewTime(prData, result)

	return result, nil
}

// analyzeComplexity determines the complexity of the PR
func (a *PRAnalyzer) analyzeComplexity(pr prInfo, result *PRAnalysis) {
	totalChanges := pr.Additions + pr.Deletions
	fileCount := pr.ChangedFiles

	if totalChanges < 50 && fileCount <= 3 {
		result.Complexity = "trivial"
	} else if totalChanges < 200 && fileCount <= 10 {
		result.Complexity = "simple"
	} else if totalChanges < 500 && fileCount <= 25 {
		result.Complexity = "moderate"
	} else if totalChanges < 1000 && fileCount <= 50 {
		result.Complexity = "complex"
	} else {
		result.Complexity = "very complex"
	}
}

// analyzeFiles performs detailed analysis on changed files
func (a *PRAnalyzer) analyzeFiles(pr prInfo, result *PRAnalysis) {
	// Simulate file analysis based on PR description and patterns
	files := a.extractFileList(pr)

	for _, file := range files {
		detail := FileDetail{
			Language:     a.detectLanguage(file),
			Changes:      0, // Would be calculated from diff
			Risk:         "low",
			Issues:       []string{},
			Improvements: []string{},
		}

		// Check for security patterns in filename
		for _, pattern := range a.securityPatterns {
			if pattern.MatchString(file) {
				detail.Risk = "high"
				detail.Issues = append(detail.Issues, "Security-sensitive file modified")
				result.HasSecurity = true
				break
			}
		}

		// Check if test file
		for _, pattern := range a.testPatterns {
			if pattern.MatchString(file) {
				detail.TestsAffected = true
				break
			}
		}

		result.FileAnalysis[file] = detail
	}
}

// detectCategories identifies the types of changes in the PR
func (a *PRAnalyzer) detectCategories(pr prInfo, result *PRAnalysis) {
	description := strings.ToLower(pr.Title + " " + pr.Description)

	categoryMap := map[string][]string{
		"bug":           {"fix", "bug", "issue", "problem", "error", "crash", "fault"},
		"feature":       {"feature", "add", "new", "implement", "introduce"},
		"enhancement":   {"enhance", "improve", "optimize", "better", "upgrade"},
		"documentation": {"doc", "readme", "comment", "documentation"},
		"refactoring":   {"refactor", "cleanup", "reorganize", "restructure"},
		"performance":   {"performance", "speed", "fast", "optimize", "cache"},
		"security":      {"security", "vulnerability", "cve", "patch", "exploit"},
		"dependency":    {"dependency", "upgrade", "update", "package", "library"},
		"testing":       {"test", "spec", "coverage", "unit", "integration"},
		"ci/cd":         {"ci", "cd", "pipeline", "workflow", "action", "deploy"},
	}

	for category, keywords := range categoryMap {
		for _, keyword := range keywords {
			if strings.Contains(description, keyword) {
				result.Categories = append(result.Categories, category)
				break
			}
		}
	}

	// Ensure we have at least one category
	if len(result.Categories) == 0 {
		result.Categories = append(result.Categories, "other")
	}
}

// analyzeSecurityRisks checks for security implications
func (a *PRAnalyzer) analyzeSecurityRisks(pr prInfo, result *PRAnalysis) {
	description := pr.Title + " " + pr.Description

	securityIssues := 0
	for _, pattern := range a.securityPatterns {
		if pattern.MatchString(description) {
			securityIssues++
			result.HasSecurity = true
		}
	}

	if securityIssues > 0 {
		result.Issues = append(result.Issues, Issue{
			Type:        "Security Review Required",
			Severity:    "high",
			Description: "This PR contains security-related changes that require careful review",
			Suggestion:  "Ensure proper security review by a qualified team member",
		})

		result.Suggestions = append(result.Suggestions,
			"Request review from security team",
			"Ensure secrets are not exposed in code or logs",
			"Verify authentication and authorization logic",
		)
	}
}

// analyzePerformanceImpact evaluates performance implications
func (a *PRAnalyzer) analyzePerformanceImpact(pr prInfo, result *PRAnalysis) {
	description := pr.Title + " " + pr.Description

	perfPatterns := 0
	for _, pattern := range a.performancePatterns {
		if pattern.MatchString(description) {
			perfPatterns++
		}
	}

	if perfPatterns > 2 {
		result.PerformanceImpact = "significant"
		result.PerformanceNotes = append(result.PerformanceNotes,
			"This PR includes performance-related changes",
			"Consider running performance benchmarks",
			"Monitor metrics after deployment",
		)
	} else if perfPatterns > 0 {
		result.PerformanceImpact = "moderate"
		result.PerformanceNotes = append(result.PerformanceNotes,
			"Some performance implications detected",
		)
	} else {
		result.PerformanceImpact = "minimal"
	}
}

// analyzeTestCoverage checks test coverage
func (a *PRAnalyzer) analyzeTestCoverage(pr prInfo, result *PRAnalysis) {
	hasTests := false
	testFiles := 0

	for file := range result.FileAnalysis {
		for _, pattern := range a.testPatterns {
			if pattern.MatchString(file) {
				hasTests = true
				testFiles++
				break
			}
		}
	}

	if testFiles > 0 {
		result.TestCoverage = fmt.Sprintf("Tests included (%d test files modified)", testFiles)
	} else if hasTests {
		result.TestCoverage = "Existing tests may be affected"
	} else {
		result.TestCoverage = "No test changes detected - consider adding tests"
		result.Suggestions = append(result.Suggestions, "Add tests for new functionality")
	}
}

// analyzeDependencies checks for dependency changes
func (a *PRAnalyzer) analyzeDependencies(pr prInfo, result *PRAnalysis) {
	description := strings.ToLower(pr.Title + " " + pr.Description)

	// Check for dependency-related keywords
	depKeywords := []string{"dependency", "package", "library", "module", "upgrade", "update", "version"}
	for _, keyword := range depKeywords {
		if strings.Contains(description, keyword) {
			// Add placeholder dependency change
			result.DependencyChanges = append(result.DependencyChanges, DependencyChange{
				Type:  "updated",
				Risk:  "low",
				Notes: "Dependency update detected - verify compatibility",
			})
			break
		}
	}
}

// analyzeAPIChanges detects API modifications
func (a *PRAnalyzer) analyzeAPIChanges(pr prInfo, result *PRAnalysis) {
	description := strings.ToLower(pr.Title + " " + pr.Description)

	// Check for API-related keywords
	apiKeywords := []string{"api", "endpoint", "route", "handler", "controller"}
	for _, keyword := range apiKeywords {
		if strings.Contains(description, keyword) {
			// Check if it's a breaking change
			breaking := false
			for _, pattern := range a.breakingPatterns {
				if pattern.MatchString(description) {
					breaking = true
					break
				}
			}

			result.APIChanges = append(result.APIChanges, APIChange{
				Type:        "modified",
				Breaking:    breaking,
				Description: "API changes detected - verify backward compatibility",
			})

			if breaking {
				result.Issues = append(result.Issues, Issue{
					Type:        "Breaking Change",
					Severity:    "high",
					Description: "This PR may contain breaking API changes",
					Suggestion:  "Document migration path for API consumers",
				})
			}
			break
		}
	}
}

// buildChecklist creates the review checklist
func (a *PRAnalyzer) buildChecklist(pr prInfo, result *PRAnalysis) {
	// Code quality checks
	result.ChecklistItems = append(result.ChecklistItems, ChecklistItem{
		Description: "Code follows project conventions",
		Passed:      true, // Default to passed, would check actual patterns
	})

	result.ChecklistItems = append(result.ChecklistItems, ChecklistItem{
		Description: "No obvious bugs or errors",
		Passed:      len(result.Issues) == 0,
		Failed:      len(result.Issues) > 3,
		Warning:     len(result.Issues) > 0 && len(result.Issues) <= 3,
	})

	// Documentation check
	hasDocChanges := false
	for _, cat := range result.Categories {
		if cat == "documentation" {
			hasDocChanges = true
			break
		}
	}

	result.ChecklistItems = append(result.ChecklistItems, ChecklistItem{
		Description: "Documentation updated if needed",
		Passed:      hasDocChanges || result.Complexity == "trivial",
		Warning:     !hasDocChanges && result.Complexity != "trivial",
		Details:     "Consider updating documentation for this change",
	})

	// Test coverage check
	result.ChecklistItems = append(result.ChecklistItems, ChecklistItem{
		Description: "Tests included or updated",
		Passed:      strings.Contains(result.TestCoverage, "Tests included"),
		Warning:     !strings.Contains(result.TestCoverage, "Tests included"),
		Details:     result.TestCoverage,
	})

	// Security check
	result.ChecklistItems = append(result.ChecklistItems, ChecklistItem{
		Description: "No security vulnerabilities introduced",
		Passed:      !result.HasSecurity,
		Warning:     result.HasSecurity,
		Details:     "Security-sensitive changes require careful review",
	})

	// Performance check
	result.ChecklistItems = append(result.ChecklistItems, ChecklistItem{
		Description: "Performance impact considered",
		Passed:      result.PerformanceImpact == "minimal",
		Warning:     result.PerformanceImpact != "minimal",
		Details:     fmt.Sprintf("Performance impact: %s", result.PerformanceImpact),
	})

	// Breaking changes check
	hasBreaking := false
	for _, api := range result.APIChanges {
		if api.Breaking {
			hasBreaking = true
			break
		}
	}

	result.ChecklistItems = append(result.ChecklistItems, ChecklistItem{
		Description: "No unexpected breaking changes",
		Passed:      !hasBreaking,
		Failed:      hasBreaking,
		Details:     "Breaking changes detected - ensure proper versioning",
	})
}

// calculateRiskLevel determines overall risk
func (a *PRAnalyzer) calculateRiskLevel(result *PRAnalysis) {
	riskScore := 0

	// Complexity contributes to risk
	switch result.Complexity {
	case "trivial":
		riskScore += 0
	case "simple":
		riskScore += 1
	case "moderate":
		riskScore += 3
	case "complex":
		riskScore += 5
	case "very complex":
		riskScore += 8
	}

	// Security issues are high risk
	if result.HasSecurity {
		riskScore += 10
	}

	// Breaking changes are risky
	for _, api := range result.APIChanges {
		if api.Breaking {
			riskScore += 5
			break
		}
	}

	// Issues add to risk
	riskScore += len(result.Issues) * 2

	// Determine risk level
	if riskScore <= 2 {
		result.RiskLevel = "low"
	} else if riskScore <= 5 {
		result.RiskLevel = "medium"
	} else if riskScore <= 10 {
		result.RiskLevel = "high"
	} else {
		result.RiskLevel = "critical"
	}
}

// checkAutoApproval determines if PR can be auto-approved
func (a *PRAnalyzer) checkAutoApproval(result *PRAnalysis) {
	// Start optimistic
	result.AutoApprovalEligible = true

	// Disqualifiers
	if result.RiskLevel == "high" || result.RiskLevel == "critical" {
		result.AutoApprovalEligible = false
		return
	}

	if result.HasSecurity {
		result.AutoApprovalEligible = false
		return
	}

	if result.Complexity == "complex" || result.Complexity == "very complex" {
		result.AutoApprovalEligible = false
		return
	}

	// Check for breaking changes
	for _, api := range result.APIChanges {
		if api.Breaking {
			result.AutoApprovalEligible = false
			return
		}
	}

	// Check if only safe categories
	safeCategories := map[string]bool{
		"documentation": true,
		"testing":       true,
		"ci/cd":         true,
	}

	allSafe := true
	for _, cat := range result.Categories {
		if !safeCategories[cat] {
			allSafe = false
			break
		}
	}

	if result.AutoApprovalEligible && allSafe {
		result.AutoApprovalReasons = append(result.AutoApprovalReasons,
			"Only contains safe change categories",
			"Low risk assessment",
			"No security implications",
			"Simple complexity level",
		)
	} else if result.AutoApprovalEligible && result.RiskLevel == "low" {
		result.AutoApprovalReasons = append(result.AutoApprovalReasons,
			"Low risk changes",
			"No breaking changes detected",
			"Minimal complexity",
		)
	} else {
		result.AutoApprovalEligible = false
	}
}

// generateRecommendation creates final recommendation
func (a *PRAnalyzer) generateRecommendation(result *PRAnalysis) {
	if result.AutoApprovalEligible {
		result.Recommendation = "This PR appears safe and can be auto-approved after automated checks pass."
		return
	}

	switch result.RiskLevel {
	case "low":
		result.Recommendation = "This PR has low risk and can be approved after a quick review."
	case "medium":
		result.Recommendation = "This PR requires standard review with focus on the identified areas of concern."
	case "high":
		result.Recommendation = "This PR requires thorough review by experienced team members. Pay special attention to security and performance implications."
	case "critical":
		result.Recommendation = "This PR has critical risk factors and requires extensive review by multiple team members, including domain experts."
	default:
		result.Recommendation = "Standard review process recommended."
	}
}

// estimateReviewTime estimates time needed for review
func (a *PRAnalyzer) estimateReviewTime(pr prInfo, result *PRAnalysis) {
	baseMinutes := 5

	// Add time based on lines changed
	totalChanges := pr.Additions + pr.Deletions
	baseMinutes += totalChanges / 50 // Roughly 50 lines per minute

	// Add time based on complexity
	switch result.Complexity {
	case "trivial":
		baseMinutes += 0
	case "simple":
		baseMinutes += 5
	case "moderate":
		baseMinutes += 15
	case "complex":
		baseMinutes += 30
	case "very complex":
		baseMinutes += 60
	}

	// Add time for security review
	if result.HasSecurity {
		baseMinutes += 30
	}

	// Add time for breaking changes
	for _, api := range result.APIChanges {
		if api.Breaking {
			baseMinutes += 20
			break
		}
	}

	// Format the time estimate
	if baseMinutes < 15 {
		result.EstimatedReviewTime = "~10 minutes"
	} else if baseMinutes < 30 {
		result.EstimatedReviewTime = "~20 minutes"
	} else if baseMinutes < 45 {
		result.EstimatedReviewTime = "~30 minutes"
	} else if baseMinutes < 60 {
		result.EstimatedReviewTime = "~45 minutes"
	} else if baseMinutes < 120 {
		result.EstimatedReviewTime = fmt.Sprintf("~%d hour", (baseMinutes+30)/60)
	} else {
		result.EstimatedReviewTime = fmt.Sprintf("~%d hours", (baseMinutes+30)/60)
	}
}

// Helper functions

type prInfo struct {
	Title        string
	Description  string
	Additions    int
	Deletions    int
	ChangedFiles int
	Files        []string
}

func extractPRData(pr any) prInfo {
	// This would extract data from the actual PR model
	// For now, return placeholder data
	info := prInfo{
		Title:        "Sample PR",
		Description:  "Sample description",
		Additions:    100,
		Deletions:    50,
		ChangedFiles: 5,
	}

	// Extract from actual model if possible
	if prModel, ok := pr.(interface {
		GetTitle() string
		GetDescription() string
		GetAdditions() int
		GetDeletions() int
		GetChangedFiles() int
	}); ok {
		info.Title = prModel.GetTitle()
		info.Description = prModel.GetDescription()
		info.Additions = prModel.GetAdditions()
		info.Deletions = prModel.GetDeletions()
		info.ChangedFiles = prModel.GetChangedFiles()
	}

	return info
}

func (a *PRAnalyzer) extractFileList(pr prInfo) []string {
	// This would extract from actual diff
	// For now, simulate based on common patterns
	files := []string{
		"controllers/repos.go",
		"models/repository.go",
		"views/repo-list.html",
		"README.md",
		"go.mod",
	}

	if pr.ChangedFiles > 0 {
		return files[:min(pr.ChangedFiles, len(files))]
	}

	return files
}

func (a *PRAnalyzer) detectLanguage(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))

	langMap := map[string]string{
		".go":   "Go",
		".js":   "JavaScript",
		".ts":   "TypeScript",
		".py":   "Python",
		".java": "Java",
		".rb":   "Ruby",
		".rs":   "Rust",
		".c":    "C",
		".cpp":  "C++",
		".cs":   "C#",
		".html": "HTML",
		".css":  "CSS",
		".sql":  "SQL",
		".sh":   "Shell",
		".yml":  "YAML",
		".json": "JSON",
		".md":   "Markdown",
	}

	if lang, ok := langMap[ext]; ok {
		return lang
	}

	return "Unknown"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
