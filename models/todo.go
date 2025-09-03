package models

import (
	"fmt"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
)

// Todo represents a task item in an AI conversation
type Todo struct {
	application.Model
	ConversationID string    // Associated conversation (UUID)
	Content        string    // Task description  
	Status         string    // pending, in_progress, completed
	Position       int       // Order in the list
}

// Table returns the database table name
func (*Todo) Table() string { return "todos" }

// TodoStatus constants
const (
	TodoStatusPending    = "pending"
	TodoStatusInProgress = "in_progress"
	TodoStatusCompleted  = "completed"
)

// GetConversation returns the associated conversation
func (t *Todo) GetConversation() (*Conversation, error) {
	return Conversations.Get(t.ConversationID)
}

// UpdateStatus updates the todo status
func (t *Todo) UpdateStatus(status string) error {
	t.Status = status
	t.UpdatedAt = time.Now()
	return Todos.Update(t)
}

// UpdateContent updates the todo content
func (t *Todo) UpdateContent(content string) error {
	t.Content = content
	t.UpdatedAt = time.Now()
	return Todos.Update(t)
}

// GetTodosByConversation returns all todos for a conversation
func GetTodosByConversation(conversationID string) ([]*Todo, error) {
	return Todos.Search("WHERE ConversationID = ? ORDER BY Position ASC, CreatedAt ASC", conversationID)
}

// GetActiveTodos returns non-completed todos for a conversation
func GetActiveTodos(conversationID string) ([]*Todo, error) {
	return Todos.Search("WHERE ConversationID = ? AND Status != ? ORDER BY Position ASC, CreatedAt ASC", 
		conversationID, TodoStatusCompleted)
}

// GetNextPosition returns the next available position for a todo
func GetNextTodoPosition(conversationID string) int {
	todos, err := GetTodosByConversation(conversationID)
	if err != nil || len(todos) == 0 {
		return 1
	}
	
	maxPos := 0
	for _, todo := range todos {
		if todo.Position > maxPos {
			maxPos = todo.Position
		}
	}
	return maxPos + 1
}

// FormatForPrompt formats todos for inclusion in AI prompts
func FormatTodosForPrompt(todos []*Todo) string {
	if len(todos) == 0 {
		return ""
	}
	
	result := "\nCurrent Task List:\n"
	for i, todo := range todos {
		status := "⬜"
		if todo.Status == TodoStatusInProgress {
			status = "🔵"
		} else if todo.Status == TodoStatusCompleted {
			status = "✅"
		}
		result += fmt.Sprintf("%d. %s %s (%s)\n", i+1, status, todo.Content, todo.Status)
	}
	return result
}