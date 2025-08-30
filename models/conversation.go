package models

import (
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
)

// Conversation represents an AI chat conversation
type Conversation struct {
	application.Model
	UserID       string    // Owner of the conversation
	Title        string    // Conversation title (auto-generated from first message)
	LastMessage  string    // Preview of the last message
	LastRole     string    // Role of last message (user/assistant)
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Table returns the database table name
func (*Conversation) Table() string { return "conversations" }

// GetMessages returns all messages for this conversation
func (c *Conversation) GetMessages() ([]*Message, error) {
	return Messages.Search("WHERE ConversationID = ? ORDER BY CreatedAt ASC", c.ID)
}

// GetMessageCount returns the number of messages in the conversation
func (c *Conversation) GetMessageCount() int {
	messages, err := c.GetMessages()
	if err != nil {
		return 0
	}
	return len(messages)
}

// UpdateLastMessage updates the last message preview
func (c *Conversation) UpdateLastMessage(content, role string) error {
	c.LastMessage = content
	c.LastRole = role
	c.UpdatedAt = time.Now()
	
	// Truncate message for preview
	if len(c.LastMessage) > 100 {
		c.LastMessage = c.LastMessage[:97] + "..."
	}
	
	return Conversations.Update(c)
}

// GenerateTitle generates a title from the first user message
func (c *Conversation) GenerateTitle() error {
	messages, err := c.GetMessages()
	if err != nil || len(messages) == 0 {
		return err
	}
	
	// Find first user message
	for _, msg := range messages {
		if msg.Role == "user" {
			title := msg.Content
			if len(title) > 50 {
				title = title[:47] + "..."
			}
			c.Title = title
			return Conversations.Update(c)
		}
	}
	
	return nil
}