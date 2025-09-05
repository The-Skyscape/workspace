package providers

import (
	"fmt"
	"log"
	"workspace/internal/agents"
	"workspace/services"
)

// Llama32Provider implements the Provider interface for llama3.2:1b model
// Optimized for CPU-limited environments with basic tool support
type Llama32Provider struct {
	ollamaService *services.OllamaService
}

// NewLlama32Provider creates a new Llama 3.2:1b provider
func NewLlama32Provider(ollamaService *services.OllamaService) *Llama32Provider {
	return &Llama32Provider{
		ollamaService: ollamaService,
	}
}

// Name returns the provider name
func (p *Llama32Provider) Name() string {
	return "Llama 3.2:1b Provider"
}

// Model returns the model identifier
func (p *Llama32Provider) Model() string {
	return "llama3.2:1b"
}

// MaxContextTokens returns the maximum context size
func (p *Llama32Provider) MaxContextTokens() int {
	return 8192 // 8K context window
}

// SupportedTools returns the list of tools this model can effectively use
// Limited to basic operations that work well on CPU-constrained hardware
func (p *Llama32Provider) SupportedTools() []string {
	return []string{
		// Basic repository operations
		"list_repos",
		"get_repo",
		
		// File reading operations
		"list_files",
		"read_file",
		
		// Git status operations (read-only)
		"git_status",
		"git_history",
		
		// Issue viewing
		"get_issue",
	}
}

// SupportsToolCalling returns whether this model supports tool calling
func (p *Llama32Provider) SupportsToolCalling() bool {
	return true // Llama 3.2 has basic tool calling support
}

// SupportsStreaming returns whether this model supports streaming
func (p *Llama32Provider) SupportsStreaming() bool {
	return true
}

// RequiresGPU returns whether this model requires GPU
func (p *Llama32Provider) RequiresGPU() bool {
	return false // Optimized for CPU
}

// FormatMessages formats messages according to Llama's preferences
// Llama models often work better with tools described in the system message
func (p *Llama32Provider) FormatMessages(messages []agents.Message, tools []agents.Tool) []agents.Message {
	// For Llama, we might want to add tool descriptions to a system message
	// But for now, keep it simple and let Ollama handle it
	return messages
}

// ParseResponse processes the response for any provider-specific parsing
func (p *Llama32Provider) ParseResponse(response *agents.Response) *agents.Response {
	// Llama returns tool calls in the standard format
	// No special parsing needed
	return response
}

// RequiresToolsInMessages returns whether tools should be embedded in messages
func (p *Llama32Provider) RequiresToolsInMessages() bool {
	// Llama models often prefer tools in the system message
	// But we're using Ollama's native tool calling which handles this
	return false
}

// PrefersSeparateTools returns whether tools should be passed separately
func (p *Llama32Provider) PrefersSeparateTools() bool {
	// Use Ollama's native tool parameter
	return true
}

// Chat sends a chat request to the model
func (p *Llama32Provider) Chat(messages []agents.Message, options agents.ChatOptions) (*agents.Response, error) {
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
func (p *Llama32Provider) ChatWithTools(messages []agents.Message, tools []agents.Tool, options agents.ChatOptions) (*agents.Response, error) {
	if !p.ollamaService.IsRunning() {
		return nil, fmt.Errorf("Ollama service is not running")
	}
	
	// Filter tools to only include supported ones
	supportedTools := p.filterSupportedTools(tools)
	
	// Convert messages to Ollama format
	ollamaMessages := make([]services.OllamaMessage, len(messages))
	for i, msg := range messages {
		ollamaMessages[i] = agents.ConvertToOllamaMessage(msg)
	}
	
	// Convert tools to Ollama format
	ollamaTools := make([]services.OllamaTool, len(supportedTools))
	for i, tool := range supportedTools {
		ollamaTools[i] = services.OllamaTool{
			Type: tool.Type,
			Function: services.OllamaToolFunction{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  tool.Function.Parameters,
			},
		}
	}
	
	log.Printf("Llama32Provider: Sending request with %d messages and %d tools", len(messages), len(supportedTools))
	
	// Send chat request with tools
	response, err := p.ollamaService.ChatWithTools(p.Model(), ollamaMessages, ollamaTools, false)
	if err != nil {
		return nil, fmt.Errorf("chat with tools failed: %w", err)
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
func (p *Llama32Provider) StreamChat(messages []agents.Message, options agents.ChatOptions, callback agents.StreamCallback) error {
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

// filterSupportedTools returns only the tools this model supports
func (p *Llama32Provider) filterSupportedTools(tools []agents.Tool) []agents.Tool {
	supported := make(map[string]bool)
	for _, name := range p.SupportedTools() {
		supported[name] = true
	}
	
	filtered := []agents.Tool{}
	for _, tool := range tools {
		if supported[tool.Function.Name] {
			filtered = append(filtered, tool)
		}
	}
	
	return filtered
}

// convertToolCalls converts Ollama tool calls to agent format
func (p *Llama32Provider) convertToolCalls(ollamaCalls []services.OllamaToolCall) []agents.ToolCall {
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