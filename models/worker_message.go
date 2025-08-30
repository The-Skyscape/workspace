package models

import (
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
)

// WorkerMessage represents a message in a worker session
type WorkerMessage struct {
	application.Model
	SessionID string    // Session this message belongs to
	Role      string    // user, assistant, system, tool
	Content   string    // Message content
	Formatted string    `sql:"-"` // Formatted HTML (not stored, computed at runtime)
	Metadata  string    // JSON metadata for tool usage, etc.
	CreatedAt time.Time
}

// Table returns the database table name
func (*WorkerMessage) Table() string { return "worker_messages" }

// Role constants
const (
	MessageRoleUser      = "user"
	MessageRoleAssistant = "assistant"
	MessageRoleSystem    = "system"
	MessageRoleTool      = "tool"
	MessageRoleError     = "error"
)

// IsUserMessage checks if this is a user message
func (m *WorkerMessage) IsUserMessage() bool {
	return m.Role == MessageRoleUser
}

// IsAssistantMessage checks if this is an assistant message
func (m *WorkerMessage) IsAssistantMessage() bool {
	return m.Role == MessageRoleAssistant
}

// IsToolMessage checks if this is a tool usage message
func (m *WorkerMessage) IsToolMessage() bool {
	return m.Role == MessageRoleTool
}

// GetSession returns the session this message belongs to
func (m *WorkerMessage) GetSession() (*WorkerSession, error) {
	return WorkerSessions.Get(m.SessionID)
}