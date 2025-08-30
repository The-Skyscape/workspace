package models

import (
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
)

// WorkerSession represents a chat session with a worker
type WorkerSession struct {
	application.Model
	WorkerID     string    // Worker this session belongs to
	Name         string    // Session name (e.g., "Debug Session", "Feature Development")
	CreatedAt    time.Time
	LastActiveAt time.Time
}

// Table returns the database table name
func (*WorkerSession) Table() string { return "worker_sessions" }

// MarkActive updates the last active timestamp
func (s *WorkerSession) MarkActive() {
	s.LastActiveAt = time.Now()
	WorkerSessions.Update(s)
}

// GetMessages returns all messages for this session
func (s *WorkerSession) GetMessages() ([]*WorkerMessage, error) {
	return WorkerMessages.Search("WHERE SessionID = ? ORDER BY CreatedAt ASC", s.ID)
}

// GetRecentMessages returns messages from the last N minutes
func (s *WorkerSession) GetRecentMessages(minutes int) ([]*WorkerMessage, error) {
	cutoff := time.Now().Add(-time.Duration(minutes) * time.Minute)
	return WorkerMessages.Search("WHERE SessionID = ? AND CreatedAt > ? ORDER BY CreatedAt ASC", 
		s.ID, cutoff)
}

// GetLastMessage returns the most recent message in the session
func (s *WorkerSession) GetLastMessage() (*WorkerMessage, error) {
	messages, err := WorkerMessages.Search("WHERE SessionID = ? ORDER BY CreatedAt DESC LIMIT 1", s.ID)
	if err != nil || len(messages) == 0 {
		return nil, err
	}
	return messages[0], nil
}

// AddMessage adds a new message to the session
func (s *WorkerSession) AddMessage(role, content string) (*WorkerMessage, error) {
	message := &WorkerMessage{
		SessionID: s.ID,
		Role:      role,
		Content:   content,
		CreatedAt: time.Now(),
	}
	
	// Mark session as active
	s.MarkActive()
	
	return WorkerMessages.Insert(message)
}

// ClearMessages removes all messages from the session
func (s *WorkerSession) ClearMessages() error {
	messages, err := s.GetMessages()
	if err != nil {
		return err
	}
	
	for _, msg := range messages {
		if err := WorkerMessages.Delete(msg); err != nil {
			return err
		}
	}
	
	return nil
}

// GetMessageCount returns the number of messages in the session
func (s *WorkerSession) GetMessageCount() int {
	messages, err := s.GetMessages()
	if err != nil {
		return 0
	}
	return len(messages)
}