package agents

import (
	"encoding/json"
	"fmt"
)

// ToolImplementation represents an AI tool that can be called
type ToolImplementation interface {
	// Name returns the tool's name (used in tool calls)
	Name() string

	// Description returns a description for the AI to understand when to use this tool
	Description() string

	// Execute runs the tool with the given parameters
	Execute(params map[string]any, userID string) (string, error)

	// ValidateParams checks if the parameters are valid
	ValidateParams(params map[string]any) error

	// Schema returns the parameter schema for structured outputs (optional)
	Schema() map[string]any
}

// ToolCallRequest represents a parsed tool call from the AI
type ToolCallRequest struct {
	Tool   string         `json:"tool"`
	Params map[string]any `json:"params,omitempty"`
}

// ToolRegistry manages available tools
type ToolRegistry struct {
	tools map[string]ToolImplementation
}

// NewToolRegistry creates a new tool registry
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]ToolImplementation),
	}
}

// Register adds a tool to the registry
func (r *ToolRegistry) Register(tool ToolImplementation) {
	r.tools[tool.Name()] = tool
}

// Get retrieves a tool by name
func (r *ToolRegistry) Get(name string) (ToolImplementation, bool) {
	tool, exists := r.tools[name]
	return tool, exists
}

// ListTools returns all available tool names and descriptions
func (r *ToolRegistry) ListTools() map[string]string {
	list := make(map[string]string)
	for name, tool := range r.tools {
		list[name] = tool.Description()
	}
	return list
}

// ExecuteTool executes a tool by name with given parameters
func (r *ToolRegistry) ExecuteTool(name string, params map[string]any, userID string) (string, error) {
	tool, exists := r.tools[name]
	if !exists {
		return "", fmt.Errorf("tool '%s' not found", name)
	}

	// Validate parameters
	if err := tool.ValidateParams(params); err != nil {
		return "", fmt.Errorf("invalid parameters for tool '%s': %w", name, err)
	}

	// Execute the tool
	result, err := tool.Execute(params, userID)
	if err != nil {
		return "", fmt.Errorf("tool '%s' execution failed: %w", name, err)
	}

	return result, nil
}

// FormatToolResult formats a tool execution result for display
func FormatToolResult(toolName string, result string, err error) string {
	if err != nil {
		return fmt.Sprintf("❌ Tool '%s' failed: %s", toolName, err.Error())
	}
	return fmt.Sprintf("✅ Tool '%s' result:\n%s", toolName, result)
}

// GenerateToolPrompt generates a prompt that describes available tools to the AI
func (r *ToolRegistry) GenerateToolPrompt() string {
	if len(r.tools) == 0 {
		return ""
	}

	prompt := "\n\nYou have access to the following tools:\n\n"
	for name, tool := range r.tools {
		prompt += fmt.Sprintf("- **%s**: %s\n", name, tool.Description())

		// Include schema if available for Ollama structured outputs
		if schema := tool.Schema(); schema != nil {
			if params, ok := schema["parameters"].(map[string]any); ok {
				prompt += "  Parameters:\n"
				for paramName, paramInfo := range params {
					if info, ok := paramInfo.(map[string]any); ok {
						required := ""
						if info["required"] == true {
							required = " (required)"
						}
						prompt += fmt.Sprintf("    - %s: %s%s\n", paramName, info["description"], required)
					}
				}
			}
		}
	}

	// Note: gpt-oss uses native Ollama tool calling, not XML format
	prompt += `
These tools will be available to you through native function calling. The GPT-OSS model will automatically select and use the appropriate tools based on your request.`

	return prompt
}

// GenerateStructuredToolsSchema generates a schema for all tools (for Ollama structured outputs)
func (r *ToolRegistry) GenerateStructuredToolsSchema() []map[string]any {
	var schemas []map[string]any

	for name, tool := range r.tools {
		schema := map[string]any{
			"name":        name,
			"description": tool.Description(),
		}

		// Add parameter schema if available
		if toolSchema := tool.Schema(); toolSchema != nil {
			schema["parameters"] = toolSchema
		} else {
			// Default schema for tools without explicit schemas
			schema["parameters"] = map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}
		}

		schemas = append(schemas, schema)
	}

	return schemas
}

// GenerateOllamaTools generates tool definitions in Ollama's native format
func (r *ToolRegistry) GenerateOllamaTools() []map[string]any {
	var tools []map[string]any

	for name, tool := range r.tools {
		// Build the tool definition in OpenAI-compatible format
		toolDef := map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        name,
				"description": tool.Description(),
			},
		}

		// Add parameter schema if available
		if toolSchema := tool.Schema(); toolSchema != nil {
			toolDef["function"].(map[string]any)["parameters"] = toolSchema
		} else {
			// Default schema for tools without explicit schemas
			toolDef["function"].(map[string]any)["parameters"] = map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}
		}

		tools = append(tools, toolDef)
	}

	return tools
}

// MarshalToolCall converts a ToolCall to JSON string
func MarshalToolCall(tc ToolCall) (string, error) {
	data, err := json.Marshal(tc)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// UnmarshalToolCall parses a JSON string into a ToolCall
func UnmarshalToolCall(data string) (*ToolCall, error) {
	var tc ToolCall
	if err := json.Unmarshal([]byte(data), &tc); err != nil {
		return nil, err
	}
	return &tc, nil
}
