package models

import (
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
)

// ClaudeUsage tracks Claude API usage statistics
type ClaudeUsage struct {
	application.Model
	TotalRequests   int       `json:"total_requests"`
	TotalTokens     int       `json:"total_tokens"`
	InputTokens     int       `json:"input_tokens"`
	OutputTokens    int       `json:"output_tokens"`
	EstimatedCost   float64   `json:"estimated_cost"`
	LastUsed        time.Time `json:"last_used"`
	MonthlyRequests int       `json:"monthly_requests"`
	MonthlyTokens   int       `json:"monthly_tokens"`
	MonthlyCost     float64   `json:"monthly_cost"`
	MonthReset      time.Time `json:"month_reset"`
}

// Table returns the database table name
func (*ClaudeUsage) Table() string { return "claude_usage" }


// GetOrCreateUsage returns the usage stats, creating if necessary
func GetOrCreateUsage() (*ClaudeUsage, error) {
	// Try to get existing usage record (there should only be one)
	usages, err := ClaudeUsages.Search("ORDER BY ID LIMIT 1")
	if err != nil {
		return nil, err
	}
	
	if len(usages) > 0 {
		usage := usages[0]
		// Check if we need to reset monthly stats
		now := time.Now()
		if usage.MonthReset.Before(time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)) {
			usage.MonthlyRequests = 0
			usage.MonthlyTokens = 0
			usage.MonthlyCost = 0
			usage.MonthReset = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
			if err := ClaudeUsages.Update(usage); err != nil {
				return nil, err
			}
		}
		return usage, nil
	}
	
	// Create new usage record
	usage := &ClaudeUsage{
		MonthReset: time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.UTC),
	}
	return ClaudeUsages.Insert(usage)
}

// UpdateUsage increments usage statistics atomically
func UpdateUsage(inputTokens, outputTokens int) error {
	usage, err := GetOrCreateUsage()
	if err != nil {
		return err
	}
	
	// Update stats
	usage.TotalRequests++
	usage.TotalTokens += inputTokens + outputTokens
	usage.InputTokens += inputTokens
	usage.OutputTokens += outputTokens
	
	// Estimate costs (based on Claude 3 Sonnet pricing)
	// Input: $3 per million tokens, Output: $15 per million tokens
	inputCost := float64(inputTokens) * 0.000003
	outputCost := float64(outputTokens) * 0.000015
	usage.EstimatedCost += inputCost + outputCost
	
	// Update monthly stats
	usage.MonthlyRequests++
	usage.MonthlyTokens += inputTokens + outputTokens
	usage.MonthlyCost += inputCost + outputCost
	usage.LastUsed = time.Now()
	
	// Save to database (atomic operation)
	return ClaudeUsages.Update(usage)
}

// GetUsageStats returns the current usage statistics
func GetUsageStats() (*ClaudeUsage, error) {
	return GetOrCreateUsage()
}