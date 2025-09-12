package processors

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"workspace/internal/ai/queue"
	"workspace/models"
)

// SecurityProcessor handles security scanning tasks
type SecurityProcessor struct {
	secretPatterns     []*regexp.Regexp
	vulnerablePatterns []*regexp.Regexp
	suspiciousPatterns []*regexp.Regexp
}

// NewSecurityProcessor creates a new security processor
func NewSecurityProcessor() *SecurityProcessor {
	return &SecurityProcessor{
		secretPatterns: []*regexp.Regexp{
			// API Keys
			regexp.MustCompile(`(?i)(api[_-]?key|apikey|api_secret)[\s]*[:=][\s]*['"]?([a-zA-Z0-9_\-]{20,})['"]?`),
			// AWS
			regexp.MustCompile(`(?i)(aws[_-]?access[_-]?key[_-]?id|aws[_-]?secret[_-]?access[_-]?key)[\s]*[:=][\s]*['"]?([a-zA-Z0-9/+=]{20,})['"]?`),
			// GitHub
			regexp.MustCompile(`ghp_[a-zA-Z0-9]{36}`),
			regexp.MustCompile(`gho_[a-zA-Z0-9]{36}`),
			regexp.MustCompile(`ghu_[a-zA-Z0-9]{36}`),
			regexp.MustCompile(`ghs_[a-zA-Z0-9]{36}`),
			regexp.MustCompile(`ghr_[a-zA-Z0-9]{36}`),
			// Generic secrets
			regexp.MustCompile(`(?i)(password|passwd|pwd|secret|token)[\s]*[:=][\s]*['"]?([^'"\s]{8,})['"]?`),
			// Private keys
			regexp.MustCompile(`-----BEGIN (RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----`),
			// JWT
			regexp.MustCompile(`eyJ[a-zA-Z0-9_-]+\.eyJ[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+`),
		},
		vulnerablePatterns: []*regexp.Regexp{
			// SQL Injection
			regexp.MustCompile(`(?i)(select|insert|update|delete|drop|union|exec|execute).*from.*where`),
			regexp.MustCompile(`(?i)('\s*or\s*'|"\s*or\s*"|--\s*$|#\s*$|/\*.*\*/)`),
			// Command Injection
			regexp.MustCompile(`(?i)(exec|system|eval|passthru|shell_exec|proc_open|popen)`),
			regexp.MustCompile(`\$\(.*\)|\||\&\&|\|\|`),
			// Path Traversal
			regexp.MustCompile(`\.\./|\.\.\\`),
			// XXE
			regexp.MustCompile(`<!DOCTYPE[^>]*\[<!ENTITY`),
			// Unsafe deserialization
			regexp.MustCompile(`(?i)(pickle\.loads|yaml\.load\s*\(|deserialize\s*\(|unserialize\s*\()`),
		},
		suspiciousPatterns: []*regexp.Regexp{
			// Hardcoded IPs
			regexp.MustCompile(`\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`),
			// Hardcoded ports
			regexp.MustCompile(`(?i)(port|listen)[\s]*[:=][\s]*[0-9]{2,5}`),
			// Unsafe functions
			regexp.MustCompile(`(?i)(eval|exec|system|passthru|shell_exec)`),
			// Debug/test code
			regexp.MustCompile(`(?i)(todo|fixme|hack|xxx|debug|test)`),
			// Weak crypto
			regexp.MustCompile(`(?i)(md5|sha1|des|rc4)`),
		},
	}
}

// Process performs security scanning
func (p *SecurityProcessor) Process(ctx context.Context, task *queue.Task) error {
	repoID, ok := task.Data["repo_id"].(string)
	if !ok {
		return fmt.Errorf("invalid repo ID in task data")
	}

	// Get repository
	repo, err := models.Repos.Get(repoID)
	if err != nil {
		return fmt.Errorf("failed to get repo: %w", err)
	}

	// Perform security scan
	findings, err := p.scanRepository(ctx, repo)
	if err != nil {
		return fmt.Errorf("security scan failed: %w", err)
	}

	// Create security report
	if len(findings) > 0 {
		if err := p.createSecurityReport(repo, findings); err != nil {
			return fmt.Errorf("failed to create security report: %w", err)
		}
	}

	task.Result = map[string]any{
		"findings": findings,
		"severity": p.calculateSeverity(findings),
	}

	return nil
}

// CanHandle checks if this processor can handle the given task type
func (p *SecurityProcessor) CanHandle(taskType queue.TaskType) bool {
	return taskType == queue.TaskSecurityScan || taskType == queue.TaskPerformanceCheck
}

// SecurityFinding represents a security issue found
type SecurityFinding struct {
	Type        string `json:"type"`
	Severity    string `json:"severity"`
	File        string `json:"file"`
	Line        int    `json:"line"`
	Description string `json:"description"`
	Pattern     string `json:"pattern"`
	Remediation string `json:"remediation"`
}

// scanRepository performs security scanning on repository files
func (p *SecurityProcessor) scanRepository(ctx context.Context, repo *models.Repository) ([]SecurityFinding, error) {
	var findings []SecurityFinding

	// This is a simplified scan - in production, would scan actual files
	// For now, scan recent commits and PR descriptions

	// Check recent issues for security keywords
	issues, _ := models.Issues.Search("WHERE RepoID = ? AND Status = 'open' LIMIT 10", repo.ID)
	for _, issue := range issues {
		content := issue.Title + " " + issue.Body

		// Check for exposed secrets
		for _, pattern := range p.secretPatterns {
			if pattern.MatchString(content) {
				findings = append(findings, SecurityFinding{
					Type:        "exposed_secret",
					Severity:    "critical",
					File:        fmt.Sprintf("issue-%s", issue.ID),
					Description: "Potential secret or credential exposed in issue",
					Remediation: "Remove sensitive data and rotate credentials immediately",
				})
			}
		}

		// Check for vulnerability patterns
		for _, pattern := range p.vulnerablePatterns {
			if pattern.MatchString(content) {
				findings = append(findings, SecurityFinding{
					Type:        "vulnerability_pattern",
					Severity:    "high",
					File:        fmt.Sprintf("issue-%s", issue.ID),
					Description: "Potential security vulnerability pattern detected",
					Remediation: "Review code for security vulnerabilities",
				})
			}
		}
	}

	// Check recent PRs
	prs, _ := models.PullRequests.Search("WHERE RepoID = ? AND Status = 'open' LIMIT 10", repo.ID)
	for _, pr := range prs {
		content := pr.Title + " " + pr.Description

		// Check for suspicious patterns
		for _, pattern := range p.suspiciousPatterns {
			if pattern.MatchString(content) {
				findings = append(findings, SecurityFinding{
					Type:        "suspicious_pattern",
					Severity:    "medium",
					File:        fmt.Sprintf("pr-%s", pr.ID),
					Description: "Suspicious pattern detected that may indicate security issue",
					Remediation: "Review for security best practices",
				})
			}
		}
	}

	log.Printf("SecurityProcessor: Found %d security findings in repo %s", len(findings), repo.Name)
	return findings, nil
}

// createSecurityReport creates an issue with security findings
func (p *SecurityProcessor) createSecurityReport(repo *models.Repository, findings []SecurityFinding) error {
	var report strings.Builder

	report.WriteString("# 🔒 Security Scan Report\n\n")
	report.WriteString(fmt.Sprintf("**Repository:** %s\n", repo.Name))
	report.WriteString(fmt.Sprintf("**Scan Date:** %s\n", time.Now().Format("January 2, 2006 15:04 MST")))
	report.WriteString(fmt.Sprintf("**Total Findings:** %d\n\n", len(findings)))

	// Group findings by severity
	critical := []SecurityFinding{}
	high := []SecurityFinding{}
	medium := []SecurityFinding{}
	low := []SecurityFinding{}

	for _, finding := range findings {
		switch finding.Severity {
		case "critical":
			critical = append(critical, finding)
		case "high":
			high = append(high, finding)
		case "medium":
			medium = append(medium, finding)
		default:
			low = append(low, finding)
		}
	}

	// Critical findings
	if len(critical) > 0 {
		report.WriteString("## 🔴 Critical Findings\n\n")
		for _, f := range critical {
			report.WriteString(fmt.Sprintf("### %s\n", f.Type))
			report.WriteString(fmt.Sprintf("- **Location:** %s\n", f.File))
			report.WriteString(fmt.Sprintf("- **Description:** %s\n", f.Description))
			report.WriteString(fmt.Sprintf("- **Remediation:** %s\n\n", f.Remediation))
		}
	}

	// High findings
	if len(high) > 0 {
		report.WriteString("## 🟠 High Priority Findings\n\n")
		for _, f := range high {
			report.WriteString(fmt.Sprintf("- **%s** in %s: %s\n", f.Type, f.File, f.Description))
		}
		report.WriteString("\n")
	}

	// Medium findings
	if len(medium) > 0 {
		report.WriteString("## 🟡 Medium Priority Findings\n\n")
		for _, f := range medium {
			report.WriteString(fmt.Sprintf("- **%s** in %s: %s\n", f.Type, f.File, f.Description))
		}
		report.WriteString("\n")
	}

	// Low findings
	if len(low) > 0 {
		report.WriteString("## 🟢 Low Priority Findings\n\n")
		for _, f := range low {
			report.WriteString(fmt.Sprintf("- %s in %s\n", f.Type, f.File))
		}
		report.WriteString("\n")
	}

	// Recommendations
	report.WriteString("## 💡 Recommendations\n\n")
	report.WriteString("1. Address all critical findings immediately\n")
	report.WriteString("2. Review and remediate high priority issues\n")
	report.WriteString("3. Implement security best practices\n")
	report.WriteString("4. Consider using automated security tools in CI/CD\n")
	report.WriteString("5. Conduct regular security audits\n\n")

	report.WriteString("---\n")
	report.WriteString("*This report was generated automatically by AI security scanning.*\n")

	// Convert severity to priority
	var priority models.IssuePriority
	switch p.calculateSeverity(findings) {
	case "critical":
		priority = models.PriorityCritical
	case "high":
		priority = models.PriorityHigh
	case "medium":
		priority = models.PriorityMedium
	default:
		priority = models.PriorityLow
	}

	// Create issue
	issue := &models.Issue{
		Title:    fmt.Sprintf("🔒 Security Scan - %d findings", len(findings)),
		Body:     report.String(),
		Status:   models.IssueStatusOpen,
		Priority: priority,
		RepoID:   repo.ID,
		AuthorID: "system",
	}

	_, err := models.Issues.Insert(issue)
	return err
}

// calculateSeverity determines overall severity from findings
func (p *SecurityProcessor) calculateSeverity(findings []SecurityFinding) string {
	hasCritical := false
	hasHigh := false
	hasMedium := false

	for _, f := range findings {
		switch f.Severity {
		case "critical":
			hasCritical = true
		case "high":
			hasHigh = true
		case "medium":
			hasMedium = true
		}
	}

	if hasCritical {
		return "critical"
	}
	if hasHigh {
		return "high"
	}
	if hasMedium {
		return "medium"
	}
	return "low"
}
