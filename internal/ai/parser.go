package ai

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// ParseToolCalls extracts tool calls from AI response text
func ParseToolCalls(text string) ([]ToolCall, string) {
	var toolCalls []ToolCall
	
	// Regular expression to match tool calls in XML format
	// Matches: <tool_call>...</tool_call>
	re := regexp.MustCompile(`<tool_call>\s*({[^}]+})\s*</tool_call>`)
	matches := re.FindAllStringSubmatch(text, -1)
	
	// Track positions of tool calls for removal
	var positions [][]int
	if len(matches) > 0 {
		positions = re.FindAllStringIndex(text, -1)
	}
	
	// Parse each tool call
	for _, match := range matches {
		if len(match) > 1 {
			jsonStr := match[1]
			var tc ToolCall
			if err := json.Unmarshal([]byte(jsonStr), &tc); err == nil {
				toolCalls = append(toolCalls, tc)
			}
		}
	}
	
	// Remove tool calls from the original text
	cleanedText := text
	if len(positions) > 0 {
		// Remove from end to start to maintain correct positions
		for i := len(positions) - 1; i >= 0; i-- {
			pos := positions[i]
			cleanedText = cleanedText[:pos[0]] + cleanedText[pos[1]:]
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