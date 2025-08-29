package models

import (
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
)

// ChatMessage represents a message in a Claude conversation
type ChatMessage struct {
	application.Model
	WorkerID  string    // AI Worker this message belongs to
	Role      string    // "user" or "assistant"
	Content   string    // Message content
	CreatedAt time.Time // When the message was created
}

// Table returns the database table name
func (*ChatMessage) Table() string { return "chat_messages" }

// Role constants
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
)

// GetConversationHistory returns all messages for a worker in chronological order
func GetConversationHistory(workerID string) ([]*ChatMessage, error) {
	return ChatMessages.Search("WHERE WorkerID = ? ORDER BY CreatedAt ASC", workerID)
}

// AddMessage adds a new message to the conversation
func AddMessage(workerID, role, content string) (*ChatMessage, error) {
	message := &ChatMessage{
		WorkerID:  workerID,
		Role:      role,
		Content:   content,
		CreatedAt: time.Now(),
	}
	return ChatMessages.Insert(message)
}

// ClearConversationHistory removes all messages for a worker
func ClearConversationHistory(workerID string) error {
	messages, err := ChatMessages.Search("WHERE WorkerID = ?", workerID)
	if err != nil {
		return err
	}
	
	for _, msg := range messages {
		if err := ChatMessages.Delete(msg); err != nil {
			return err
		}
	}
	
	return nil
}