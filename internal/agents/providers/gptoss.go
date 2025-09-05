package providers

import (
	"fmt"
	"log"
	"workspace/internal/agents"
	"workspace/services"
)

// GPTOSSProvider implements the Provider interface for gpt-oss model
// Optimized for GPU-accelerated environments with full tool support
type GPTOSSProvider struct {
	ollamaService *services.OllamaService
}

// NewGPTOSSProvider creates a new GPT-OSS provider
func NewGPTOSSProvider(ollamaService *services.OllamaService) *GPTOSSProvider {
	return &GPTOSSProvider{
		ollamaService: ollamaService,
	}
}

// Name returns the provider name
func (p *GPTOSSProvider) Name() string {
	return "GPT-OSS Provider"
}

// Model returns the model identifier
func (p *GPTOSSProvider) Model() string {
	return "gpt-oss"
}

// MaxContextTokens returns the maximum context size
func (p *GPTOSSProvider) MaxContextTokens() int {
	return 128000 // 128K context window
}

// SupportedTools returns all available tools - GPT-OSS can handle everything
func (p *GPTOSSProvider) SupportedTools() []string {
	return []string{
		// Repository management
		"list_repos",
		"get_repo",
		"create_repo",
		"delete_repo",
		"get_repo_link",
		
		// File operations
		"list_files",
		"read_file",
		"write_file",
		"edit_file",
		"delete_file",
		"move_file",
		"search_files",
		
		// Git operations
		"git_status",
		"git_history",
		"git_diff",
		"git_commit",
		"git_push",
		
		// Issue and project management
		"create_issue",
		"get_issue",
		"create_milestone",
		"create_project_card",
		
		// Advanced operations
		"terminal_execute",
		"create_todo",
		"list_todos",
		"update_todo",
	}
}

// SupportsToolCalling returns whether this model supports tool calling
func (p *GPTOSSProvider) SupportsToolCalling() bool {
	return true // GPT-OSS has advanced tool calling support
}

// SupportsStreaming returns whether this model supports streaming
func (p *GPTOSSProvider) SupportsStreaming() bool {
	return true
}

// RequiresGPU returns whether this model requires GPU
func (p *GPTOSSProvider) RequiresGPU() bool {
	return true // Requires GPU for optimal performance
}

// FormatMessages formats messages according to GPT-OSS preferences
// GPT-OSS follows OpenAI's format closely
func (p *GPTOSSProvider) FormatMessages(messages []agents.Message, tools []agents.Tool) []agents.Message {
	// GPT-OSS uses standard OpenAI format
	// Tools are passed separately, not in messages
	return messages
}

// ParseResponse processes the response for any provider-specific parsing
func (p *GPTOSSProvider) ParseResponse(response *agents.Response) *agents.Response {
	// GPT-OSS returns tool calls in OpenAI format
	// No special parsing needed
	return response
}

// RequiresToolsInMessages returns whether tools should be embedded in messages
func (p *GPTOSSProvider) RequiresToolsInMessages() bool {
	// GPT-OSS follows OpenAI spec - tools are separate
	return false
}

// PrefersSeparateTools returns whether tools should be passed separately
func (p *GPTOSSProvider) PrefersSeparateTools() bool {
	// GPT-OSS prefers tools as a separate parameter per OpenAI spec
	return true
}

// Chat sends a chat request to the model
func (p *GPTOSSProvider) Chat(messages []agents.Message, options agents.ChatOptions) (*agents.Response, error) {
	if !p.ollamaService.IsRunning() {
		return nil, fmt.Errorf("Ollama service is not running")
	}
	
	// Convert messages to Ollama format
	ollamaMessages := make([]services.OllamaMessage, len(messages))
	for i, msg := range messages {
		ollamaMessages[i] = agents.ConvertToOllamaMessage(msg)
	}
	
	// Send chat request
	response, err := p.ollamaService.Chat(p.Model(), ollamaMessages, false)
	if err != nil {
		return nil, fmt.Errorf("chat request failed: %w", err)
	}
	
	// Convert response
	return &agents.Response{
		Content: response.Message.Content,
		ToolCalls: p.convertToolCalls(response.Message.ToolCalls),
		Metadata: agents.ResponseMetadata{
			Model:           response.Model,
			TotalDuration:   response.TotalDuration,
			EvalCount:       response.EvalCount,
			PromptEvalCount: response.PromptEvalCount,
		},
	}, nil
}

// ChatWithTools sends a chat request with tool definitions
func (p *GPTOSSProvider) ChatWithTools(messages []agents.Message, tools []agents.Tool, options agents.ChatOptions) (*agents.Response, error) {
	if !p.ollamaService.IsRunning() {
		return nil, fmt.Errorf("Ollama service is not running")
	}
	
	// Convert messages to Ollama format
	ollamaMessages := make([]services.OllamaMessage, len(messages))
	for i, msg := range messages {
		ollamaMessages[i] = agents.ConvertToOllamaMessage(msg)
	}
	
	// Convert all tools to Ollama format (no filtering needed for GPT-OSS)
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
	
	log.Printf("GPTOSSProvider: Sending request with %d messages and %d tools", len(messages), len(tools))
	
	// Send chat request with tools
	response, err := p.ollamaService.ChatWithTools(p.Model(), ollamaMessages, ollamaTools, false)
	if err != nil {
		return nil, fmt.Errorf("chat with tools failed: %w", err)
	}
	
	// Log if we got tool calls
	if len(response.Message.ToolCalls) > 0 {
		log.Printf("GPTOSSProvider: Received %d tool calls", len(response.Message.ToolCalls))
		for _, tc := range response.Message.ToolCalls {
			log.Printf("GPTOSSProvider: Tool call: %s", tc.Function.Name)
		}
	}
	
	// Convert response
	return &agents.Response{
		Content: response.Message.Content,
		ToolCalls: p.convertToolCalls(response.Message.ToolCalls),
		Metadata: agents.ResponseMetadata{
			Model:           response.Model,
			TotalDuration:   response.TotalDuration,
			EvalCount:       response.EvalCount,
			PromptEvalCount: response.PromptEvalCount,
		},
	}, nil
}

// StreamChat sends a streaming chat request
func (p *GPTOSSProvider) StreamChat(messages []agents.Message, options agents.ChatOptions, callback agents.StreamCallback) error {
	if !p.ollamaService.IsRunning() {
		return fmt.Errorf("Ollama service is not running")
	}
	
	// Convert messages to Ollama format
	ollamaMessages := make([]services.OllamaMessage, len(messages))
	for i, msg := range messages {
		ollamaMessages[i] = agents.ConvertToOllamaMessage(msg)
	}
	
	// Stream chat with callback wrapper
	return p.ollamaService.StreamChat(p.Model(), ollamaMessages, func(chunk *services.OllamaChatResponse) error {
		// Convert chunk to agent response
		response := &agents.Response{
			Content: chunk.Message.Content,
			ToolCalls: p.convertToolCalls(chunk.Message.ToolCalls),
			Metadata: agents.ResponseMetadata{
				Model: chunk.Model,
			},
		}
		return callback(response)
	})
}

// convertToolCalls converts Ollama tool calls to agent format
func (p *GPTOSSProvider) convertToolCalls(ollamaCalls []services.OllamaToolCall) []agents.ToolCall {
	if len(ollamaCalls) == 0 {
		return nil
	}
	
	calls := make([]agents.ToolCall, len(ollamaCalls))
	for i, oc := range ollamaCalls {
		calls[i] = agents.ToolCall{
			ID:   oc.ID,
			Type: oc.Type,
			Function: agents.FunctionCall{
				Name:      oc.Function.Name,
				Arguments: oc.Function.Arguments,
			},
		}
	}
	
	return calls
}