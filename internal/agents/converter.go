package agents

import (
	"encoding/json"
	"workspace/services"
)

// ConvertOllamaToAgentMessages converts Ollama format messages to agent format
func ConvertOllamaToAgentMessages(ollamaMessages []services.OllamaMessage) []Message {
	messages := make([]Message, len(ollamaMessages))
	for i, msg := range ollamaMessages {
		messages[i] = Message{
			Role:    msg.Role,
			Content: msg.Content,
		}
		
		// Convert tool calls if present
		if len(msg.ToolCalls) > 0 {
			messages[i].ToolCalls = make([]ToolCall, len(msg.ToolCalls))
			for j, tc := range msg.ToolCalls {
				messages[i].ToolCalls[j] = ToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: FunctionCall{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			}
		}
	}
	return messages
}

// ConvertAgentToOllamaMessages converts agent format messages to Ollama format
func ConvertAgentToOllamaMessages(agentMessages []Message) []services.OllamaMessage {
	ollamaMessages := make([]services.OllamaMessage, len(agentMessages))
	for i, msg := range agentMessages {
		ollamaMessages[i] = services.OllamaMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
		
		// Convert tool calls if present
		if len(msg.ToolCalls) > 0 {
			ollamaMessages[i].ToolCalls = make([]services.OllamaToolCall, len(msg.ToolCalls))
			for j, tc := range msg.ToolCalls {
				ollamaMessages[i].ToolCalls[j] = services.OllamaToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: services.OllamaFunctionCall{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			}
		}
	}
	return ollamaMessages
}

// ConvertOllamaToAgentToolCall converts a single Ollama tool call to agent format
func ConvertOllamaToAgentToolCall(ollamaCall *services.OllamaToolCall) ToolCall {
	return ToolCall{
		ID:   ollamaCall.ID,
		Type: ollamaCall.Type,
		Function: FunctionCall{
			Name:      ollamaCall.Function.Name,
			Arguments: ollamaCall.Function.Arguments,
		},
	}
}

// ConvertAgentToOllamaToolCall converts a single agent tool call to Ollama format
func ConvertAgentToOllamaToolCall(agentCall ToolCall) services.OllamaToolCall {
	return services.OllamaToolCall{
		ID:   agentCall.ID,
		Type: agentCall.Type,
		Function: services.OllamaFunctionCall{
			Name:      agentCall.Function.Name,
			Arguments: agentCall.Function.Arguments,
		},
	}
}

// ConvertRegistryToAgentTools converts tool registry definitions to agent tool format
func ConvertRegistryToAgentTools(registry *ToolRegistry, supportedTools []string) []Tool {
	var tools []Tool
	
	// Create a map for quick lookup
	supportedMap := make(map[string]bool)
	for _, name := range supportedTools {
		supportedMap[name] = true
	}
	
	// Get tool definitions from registry
	toolDefs := registry.GenerateOllamaTools()
	
	for _, def := range toolDefs {
		if funcDef, ok := def["function"].(map[string]interface{}); ok {
			name := funcDef["name"].(string)
			
			// Only include if supported
			if !supportedMap[name] {
				continue
			}
			
			tool := Tool{
				Type: "function",
				Function: ToolFunction{
					Name:        name,
					Description: funcDef["description"].(string),
					Parameters:  funcDef["parameters"].(map[string]interface{}),
				},
			}
			tools = append(tools, tool)
		}
	}
	
	return tools
}

// ConvertAgentToOllamaTools converts agent tools to Ollama format
func ConvertAgentToOllamaTools(tools []Tool) []services.OllamaTool {
	ollamaTools := make([]services.OllamaTool, len(tools))
	for i, tool := range tools {
		ollamaTools[i] = services.OllamaTool{
			Type: tool.Type,
			Function: services.OllamaToolFunction{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  tool.Function.Parameters,
			},
		}
	}
	return ollamaTools
}

// ExtractToolCallsFromMessage checks if a message contains tool calls and extracts them
// This is useful for models that embed tool calls within the message content
func ExtractToolCallsFromMessage(message *Message) []ToolCall {
	// For now, we rely on native tool calling
	// Models should return tool calls in the proper format
	return message.ToolCalls
}

// FormatToolResultMessage creates a properly formatted tool result message
func FormatToolResultMessage(toolName string, result string) Message {
	return Message{
		Role:    "tool",
		Content: result,
	}
}

// ParseJSONArguments safely parses tool call arguments
func ParseJSONArguments(args json.RawMessage) (map[string]interface{}, error) {
	var params map[string]interface{}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	return params, nil
}