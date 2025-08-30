package ai

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
)

// MessageType represents the type of Claude message
type MessageType string

const (
	MessageTypeUser      MessageType = "user"
	MessageTypeAssistant MessageType = "assistant"
	MessageTypeTool      MessageType = "tool"
	MessageTypeError     MessageType = "error"
	MessageTypeSystem    MessageType = "system"
)

// ClaudeMessage represents a message in the Claude JSON stream format
type ClaudeMessage struct {
	Type      MessageType            `json:"type"`
	Content   string                 `json:"content,omitempty"`
	ToolName  string                 `json:"tool_name,omitempty"`
	ToolInput map[string]interface{} `json:"tool_input,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// StreamHandler manages the persistent Claude process with JSON streaming
type StreamHandler struct {
	sandbox    SandboxInterface
	inputPipe  io.WriteCloser
	outputPipe io.ReadCloser
	outputChan chan ClaudeMessage
	errorChan  chan error
	done       chan bool
	mu         sync.RWMutex
	isRunning  bool
}

// NewStreamHandler creates a new Claude stream handler
func NewStreamHandler(sandbox SandboxInterface) (*StreamHandler, error) {
	if !sandbox.IsRunning() {
		return nil, errors.New("sandbox is not running")
	}

	handler := &StreamHandler{
		sandbox:    sandbox,
		outputChan: make(chan ClaudeMessage, 100),
		errorChan:  make(chan error, 10),
		done:       make(chan bool),
		isRunning:  false,
	}

	return handler, nil
}

// Start initializes the streaming connection to Claude
func (h *StreamHandler) Start() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.isRunning {
		return errors.New("stream handler is already running")
	}

	// Check if Claude process is running
	checkCmd := "kill -0 $(cat /tmp/claude.pid 2>/dev/null) 2>/dev/null && echo 'running' || echo 'stopped'"
	output, _, err := h.sandbox.Execute(checkCmd)
	if err != nil || strings.TrimSpace(output) != "running" {
		return errors.New("Claude process is not running in sandbox")
	}

	// Start reading from the output pipe
	go h.readOutputStream()

	h.isRunning = true
	log.Println("Claude stream handler started")
	return nil
}

// Stop gracefully stops the stream handler
func (h *StreamHandler) Stop() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.isRunning {
		return nil
	}

	// Signal shutdown
	close(h.done)

	// Close pipes if they exist
	if h.inputPipe != nil {
		h.inputPipe.Close()
	}
	if h.outputPipe != nil {
		h.outputPipe.Close()
	}

	h.isRunning = false
	log.Println("Claude stream handler stopped")
	return nil
}

// SendMessage sends a message to Claude via the input pipe
func (h *StreamHandler) SendMessage(message string) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if !h.isRunning {
		return errors.New("stream handler is not running")
	}

	// Create a JSON message for Claude
	msg := ClaudeMessage{
		Type:      MessageTypeUser,
		Content:   message,
		Timestamp: time.Now(),
	}

	jsonData, err := json.Marshal(msg)
	if err != nil {
		return errors.Wrap(err, "failed to marshal message")
	}

	// Send to Claude via named pipe
	sendCmd := fmt.Sprintf(`echo '%s' > /tmp/claude_input`, string(jsonData))
	_, _, err = h.sandbox.Execute(sendCmd)
	if err != nil {
		return errors.Wrap(err, "failed to send message to Claude")
	}

	return nil
}

// GetOutputChannel returns the channel for receiving Claude's responses
func (h *StreamHandler) GetOutputChannel() <-chan ClaudeMessage {
	return h.outputChan
}

// GetErrorChannel returns the channel for receiving errors
func (h *StreamHandler) GetErrorChannel() <-chan error {
	return h.errorChan
}

// readOutputStream continuously reads from Claude's output pipe
func (h *StreamHandler) readOutputStream() {
	defer close(h.outputChan)
	defer close(h.errorChan)

	// Start a goroutine to continuously read from the output pipe
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-h.done:
			return
		case <-ticker.C:
			// Try to read from the output pipe
			readCmd := "timeout 0.1 cat /tmp/claude_output 2>/dev/null || true"
			output, _, err := h.sandbox.Execute(readCmd)
			if err != nil {
				continue // Ignore read errors, just try again
			}

			if output == "" {
				continue
			}

			// Parse each line as a JSON message
			scanner := bufio.NewScanner(strings.NewReader(output))
			for scanner.Scan() {
				line := scanner.Text()
				if line == "" {
					continue
				}

				var msg ClaudeMessage
				if err := json.Unmarshal([]byte(line), &msg); err != nil {
					// If not valid JSON, treat as plain text assistant message
					msg = ClaudeMessage{
						Type:      MessageTypeAssistant,
						Content:   line,
						Timestamp: time.Now(),
					}
				}

				select {
				case h.outputChan <- msg:
				case <-h.done:
					return
				}
			}
		}
	}
}

// ExecuteStreamingCommand sends a command and streams the response
func (h *StreamHandler) ExecuteStreamingCommand(command string, outputChan chan<- string) error {
	// Send the command
	if err := h.SendMessage(command); err != nil {
		return err
	}

	// Create a timeout for the response
	timeout := time.After(30 * time.Second)
	messageReceived := false

	// Listen for responses
	for {
		select {
		case msg := <-h.outputChan:
			messageReceived = true
			
			switch msg.Type {
			case MessageTypeAssistant:
				outputChan <- msg.Content
			case MessageTypeTool:
				outputChan <- fmt.Sprintf("[Tool: %s] %v", msg.ToolName, msg.ToolInput)
			case MessageTypeError:
				return errors.New(msg.Error)
			}

			// Check if this is the end of the response
			if strings.Contains(msg.Content, "[DONE]") {
				return nil
			}

		case err := <-h.errorChan:
			return err

		case <-timeout:
			if !messageReceived {
				return errors.New("timeout waiting for Claude response")
			}
			// Reset timeout if we're still receiving messages
			timeout = time.After(30 * time.Second)
		}
	}
}

// StreamToHTTP streams Claude output to an HTTP response writer (SSE)
func (h *StreamHandler) StreamToHTTP(w io.Writer, flusher interface{ Flush() }) {
	for msg := range h.outputChan {
		var data string
		
		switch msg.Type {
		case MessageTypeAssistant:
			data = msg.Content
		case MessageTypeTool:
			data = fmt.Sprintf("[%s] Running...", msg.ToolName)
		case MessageTypeError:
			data = fmt.Sprintf("Error: %s", msg.Error)
		default:
			continue
		}

		// Write SSE formatted data
		fmt.Fprintf(w, "data: %s\n\n", strings.ReplaceAll(data, "\n", "\\n"))
		flusher.Flush()

		// Check for completion
		if strings.Contains(data, "[DONE]") {
			break
		}
	}
}