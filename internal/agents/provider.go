package agents

import (
	"encoding/json"
	"workspace/services"
)

// Provider represents an AI agent provider with specific capabilities
type Provider interface {
	// Core identification
	Name() string
	Model() string
	
	// Capabilities (built into each provider)
	MaxContextTokens() int
	SupportedTools() []string
	SupportsToolCalling() bool
	SupportsStreaming() bool
	RequiresGPU() bool
	
	// Message formatting for provider-specific needs
	FormatMessages(messages []Message, tools []Tool) []Message
	ParseResponse(response *Response) *Response
	
	// Tool handling preferences
	RequiresToolsInMessages() bool  // Some models want tools in system message
	PrefersSeparateTools() bool      // Some models want tools as separate parameter
	
	// Chat methods
	Chat(messages []Message, options ChatOptions) (*Response, error)
	ChatWithTools(messages []Message, tools []Tool, options ChatOptions) (*Response, error)
	StreamChat(messages []Message, options ChatOptions, callback StreamCallback) error
}

// Message represents a chat message
type Message struct {
	Role      string     `json:"role"` // "user", "assistant", "system", "tool"
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// ChatOptions configures chat behavior
type ChatOptions struct {
	Temperature float64
	MaxTokens   int
	Stream      bool
}

// Response represents a chat response from the AI
type Response struct {
	Content   string
	ToolCalls []ToolCall
	Metadata  ResponseMetadata
}

// ResponseMetadata contains additional information about the response
type ResponseMetadata struct {
	Model           string
	TotalDuration   int64
	EvalCount       int
	PromptEvalCount int
}

// ToolCall represents a tool invocation
type ToolCall struct {
	ID       string          `json:"id,omitempty"`
	Type     string          `json:"type"` // Usually "function"
	Function FunctionCall    `json:"function"`
}

// FunctionCall contains the function call details
type FunctionCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// StreamCallback is called for each chunk in streaming responses
type StreamCallback func(chunk *Response) error

// Tool represents a tool definition for function calling
type Tool struct {
	Type     string       `json:"type"` // Usually "function"
	Function ToolFunction `json:"function"`
}

// ToolFunction defines a callable function
type ToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ConvertToOllamaMessage converts an agent Message to Ollama format
func ConvertToOllamaMessage(msg Message) services.OllamaMessage {
	ollamaMsg := services.OllamaMessage{
		Role:    msg.Role,
		Content: msg.Content,
	}
	
	// Convert tool calls if present
	if len(msg.ToolCalls) > 0 {
		ollamaMsg.ToolCalls = make([]services.OllamaToolCall, len(msg.ToolCalls))
		for i, tc := range msg.ToolCalls {
			ollamaMsg.ToolCalls[i] = services.OllamaToolCall{
				ID:   tc.ID,
				Type: tc.Type,
				Function: services.OllamaFunctionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			}
		}
	}
	
	return ollamaMsg
}

// ConvertFromOllamaMessage converts an Ollama message to agent format
func ConvertFromOllamaMessage(msg services.OllamaMessage) Message {
	agentMsg := Message{
		Role:    msg.Role,
		Content: msg.Content,
	}
	
	// Convert tool calls if present
	if len(msg.ToolCalls) > 0 {
		agentMsg.ToolCalls = make([]ToolCall, len(msg.ToolCalls))
		for i, tc := range msg.ToolCalls {
			agentMsg.ToolCalls[i] = ToolCall{
				ID:   tc.ID,
				Type: tc.Type,
				Function: FunctionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			}
		}
	}
	
	return agentMsg
}