package ai

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// ParseToolCalls extracts tool calls from AI response text
// Supports multiple formats for GPT-OSS compatibility
func ParseToolCalls(text string) ([]ToolCall, string) {
	var toolCalls []ToolCall
	cleanedText := text
	
	// First try XML format: <tool_call>...</tool_call>
	// Updated regex to handle multi-line JSON and nested objects
	xmlRe := regexp.MustCompile(`<tool_call>\s*({[\s\S]*?})\s*</tool_call>`)
	xmlMatches := xmlRe.FindAllStringSubmatch(text, -1)
	
	if len(xmlMatches) > 0 {
		// Found XML-wrapped tool calls
		positions := xmlRe.FindAllStringIndex(text, -1)
		
		// Parse each tool call
		for _, match := range xmlMatches {
			if len(match) > 1 {
				jsonStr := match[1]
				var tc ToolCall
				if err := json.Unmarshal([]byte(jsonStr), &tc); err == nil {
					toolCalls = append(toolCalls, tc)
				}
			}
		}
		
		// Remove tool calls from the original text (from end to start)
		for i := len(positions) - 1; i >= 0; i-- {
			pos := positions[i]
			cleanedText = cleanedText[:pos[0]] + cleanedText[pos[1]:]
		}
	} else {
		// Try to detect plain JSON tool calls or function call format
		// GPT-OSS might use different formats including function calls
		// Updated regex to handle nested objects in params
		jsonRe := regexp.MustCompile(`\{[^{}]*"(?:tool|function|name)"\s*:\s*"[^"]+"\s*(?:,\s*"(?:params|arguments|parameters)"\s*:\s*\{[^{}]*(?:\{[^{}]*\}[^{}]*)?\})?\s*\}`)
		jsonMatches := jsonRe.FindAllString(text, -1)
		
		if len(jsonMatches) > 0 {
			positions := jsonRe.FindAllStringIndex(text, -1)
			
			// Parse each potential tool call
			for _, jsonStr := range jsonMatches {
				// Try standard format first
				var tc ToolCall
				if err := json.Unmarshal([]byte(jsonStr), &tc); err == nil && tc.Tool != "" {
					toolCalls = append(toolCalls, tc)
				} else {
					// Try alternative formats (function call format)
					var altFormat struct {
						Name      string                 `json:"name"`
						Function  string                 `json:"function"`
						Arguments map[string]interface{} `json:"arguments"`
						Parameters map[string]interface{} `json:"parameters"`
					}
					if err := json.Unmarshal([]byte(jsonStr), &altFormat); err == nil {
						// Convert to standard format
						toolName := altFormat.Name
						if toolName == "" {
							toolName = altFormat.Function
						}
						params := altFormat.Arguments
						if params == nil {
							params = altFormat.Parameters
						}
						if toolName != "" {
							toolCalls = append(toolCalls, ToolCall{
								Tool:   toolName,
								Params: params,
							})
						}
					}
				}
			}
			
			// If we found valid tool calls, remove them from text
			if len(toolCalls) > 0 {
				// Remove from end to start to maintain positions
				for i := len(positions) - 1; i >= 0; i-- {
					pos := positions[i]
					cleanedText = cleanedText[:pos[0]] + cleanedText[pos[1]:]
				}
			}
		}
	}
	
	// Clean up any extra whitespace
	cleanedText = strings.TrimSpace(cleanedText)
	
	return toolCalls, cleanedText
}

// FormatToolCallForAI formats a tool call for the AI to understand
func FormatToolCallForAI(toolName string, params map[string]interface{}) string {
	tc := ToolCall{
		Tool:   toolName,
		Params: params,
	}
	
	jsonStr, err := json.Marshal(tc)
	if err != nil {
		return ""
	}
	
	return fmt.Sprintf("<tool_call>\n%s\n</tool_call>", string(jsonStr))
}

// ExtractToolResults extracts the results after tool execution for context
func ExtractToolResults(results []string) string {
	if len(results) == 0 {
		return ""
	}
	
	var formatted strings.Builder
	formatted.WriteString("\n\n=== Tool Execution Results ===\n")
	for i, result := range results {
		formatted.WriteString(fmt.Sprintf("\n[Tool Result %d]:\n%s\n", i+1, result))
	}
	formatted.WriteString("\n=== End of Tool Results ===\n")
	
	return formatted.String()
}