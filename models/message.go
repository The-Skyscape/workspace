package models

import (
	"github.com/The-Skyscape/devtools/pkg/application"
)

// Message represents a single message in a conversation
type Message struct {
	application.Model
	ConversationID string // Conversation this message belongs to
	Role           string // user, assistant, tool, error, system
	Content        string // Message content
	Metadata       string // JSON metadata for tool executions
}

// Table returns the database table name
func (*Message) Table() string { return "messages" }

// Message role constants
const (
	MessageRoleUser      = "user"
	MessageRoleAssistant = "assistant"
	MessageRoleTool      = "tool"
	MessageRoleError     = "error"
	MessageRoleSystem    = "system"
)

// IsFromUser checks if the message is from the user
func (m *Message) IsFromUser() bool {
	return m.Role == MessageRoleUser
}

// IsFromAssistant checks if the message is from the assistant
func (m *Message) IsFromAssistant() bool {
	return m.Role == MessageRoleAssistant
}

// IsToolExecution checks if the message is a tool execution
func (m *Message) IsToolExecution() bool {
	return m.Role == MessageRoleTool
}

// IsError checks if the message is an error
func (m *Message) IsError() bool {
	return m.Role == MessageRoleError
}
