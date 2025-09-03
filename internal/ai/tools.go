package ai

import (
	"encoding/json"
	"fmt"
)

// Tool represents an AI tool that can be called
type Tool interface {
	// Name returns the tool's name (used in tool calls)
	Name() string
	
	// Description returns a description for the AI to understand when to use this tool
	Description() string
	
	// Execute runs the tool with the given parameters
	Execute(params map[string]interface{}, userID string) (string, error)
	
	// ValidateParams checks if the parameters are valid
	ValidateParams(params map[string]interface{}) error
	
	// Schema returns the parameter schema for structured outputs (optional)
	Schema() map[string]interface{}
}

// ToolCall represents a parsed tool call from the AI
type ToolCall struct {
	Tool   string                 `json:"tool"`
	Params map[string]interface{} `json:"params,omitempty"`
}

// ToolRegistry manages available tools
type ToolRegistry struct {
	tools map[string]Tool
}

// NewToolRegistry creates a new tool registry
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry
func (r *ToolRegistry) Register(tool Tool) {
	r.tools[tool.Name()] = tool
}

// Get retrieves a tool by name
func (r *ToolRegistry) Get(name string) (Tool, bool) {
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
func (r *ToolRegistry) ExecuteTool(name string, params map[string]interface{}, userID string) (string, error) {
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
		
		// Include schema if available for GPT-OSS structured outputs
		if schema := tool.Schema(); schema != nil {
			if params, ok := schema["parameters"].(map[string]interface{}); ok {
				prompt += "  Parameters:\n"
				for paramName, paramInfo := range params {
					if info, ok := paramInfo.(map[string]interface{}); ok {
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
	
	prompt += `
To use a tool, respond with a tool call in this format:
<tool_call>
{"tool": "tool_name", "params": {"param1": "value1", "param2": "value2"}}
</tool_call>

You can make multiple tool calls in a single response. The GPT-OSS model has native function calling support, so you can also use structured outputs for more reliable tool usage. After I execute the tools, I'll share the results with you and you can continue with additional tool calls if needed.`
	
	return prompt
}

// GenerateStructuredToolsSchema generates a schema for all tools (for GPT-OSS structured outputs)
func (r *ToolRegistry) GenerateStructuredToolsSchema() []map[string]interface{} {
	var schemas []map[string]interface{}
	
	for name, tool := range r.tools {
		schema := map[string]interface{}{
			"name":        name,
			"description": tool.Description(),
		}
		
		// Add parameter schema if available
		if toolSchema := tool.Schema(); toolSchema != nil {
			schema["parameters"] = toolSchema
		} else {
			// Default schema for tools without explicit schemas
			schema["parameters"] = map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			}
		}
		
		schemas = append(schemas, schema)
	}
	
	return schemas
}

// GenerateOllamaTools generates tool definitions in Ollama's native format
func (r *ToolRegistry) GenerateOllamaTools() []map[string]interface{} {
	var tools []map[string]interface{}
	
	for name, tool := range r.tools {
		// Build the tool definition in OpenAI-compatible format
		toolDef := map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        name,
				"description": tool.Description(),
			},
		}
		
		// Add parameter schema if available
		if toolSchema := tool.Schema(); toolSchema != nil {
			toolDef["function"].(map[string]interface{})["parameters"] = toolSchema
		} else {
			// Default schema for tools without explicit schemas
			toolDef["function"].(map[string]interface{})["parameters"] = map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
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