package models

import (
	"encoding/json"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
)

// Conversation represents an AI chat conversation
type Conversation struct {
	application.Model
	UserID         string // Owner of the conversation
	Title          string // Conversation title (auto-generated from first message)
	LastMessage    string // Preview of the last message
	LastRole       string // Role of last message (user/assistant)
	WorkingContext string // JSON context for tracking state between messages
	Settings       string // JSON settings for conversation behavior
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

// GetWorkingContext returns the parsed working context
func (c *Conversation) GetWorkingContext() map[string]any {
	if c.WorkingContext == "" {
		return make(map[string]any)
	}

	var context map[string]any
	if err := json.Unmarshal([]byte(c.WorkingContext), &context); err != nil {
		return make(map[string]any)
	}
	return context
}

// UpdateWorkingContext updates a key in the working context
func (c *Conversation) UpdateWorkingContext(key string, value any) error {
	context := c.GetWorkingContext()
	context[key] = value

	contextJSON, err := json.Marshal(context)
	if err != nil {
		return err
	}

	c.WorkingContext = string(contextJSON)
	c.UpdatedAt = time.Now()
	return Conversations.Update(c)
}

// ClearWorkingContext clears the working context
func (c *Conversation) ClearWorkingContext() error {
	c.WorkingContext = "{}"
	c.UpdatedAt = time.Now()
	return Conversations.Update(c)
}

// GetSettings returns the parsed settings
func (c *Conversation) GetSettings() map[string]any {
	if c.Settings == "" {
		// Default settings
		return map[string]any{
			"showThinking":  false,
			"maxIterations": 10,
			"autoMode":      true,
		}
	}

	var settings map[string]any
	if err := json.Unmarshal([]byte(c.Settings), &settings); err != nil {
		return map[string]any{
			"showThinking":  false,
			"maxIterations": 10,
			"autoMode":      true,
		}
	}
	return settings
}
