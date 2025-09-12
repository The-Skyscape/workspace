// Package websocket provides real-time updates for AI activity
package websocket

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// MessageType represents the type of WebSocket message
type MessageType string

const (
	MessageTypeActivity   MessageType = "activity"
	MessageTypeQueueStats MessageType = "queue_stats"
	MessageTypeTaskUpdate MessageType = "task_update"
	MessageTypeHeartbeat  MessageType = "heartbeat"
)

// Message represents a WebSocket message
type Message struct {
	Type      MessageType `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Data      any         `json:"data"`
}

// Client represents a WebSocket client
type Client struct {
	ID     string
	Conn   *websocket.Conn
	Send   chan []byte
	Hub    *Hub
	UserID string
	RepoID string // Optional: filter by repository
}

// Hub maintains active WebSocket connections
type Hub struct {
	// Registered clients
	clients map[*Client]bool

	// Broadcast channel for messages
	broadcast chan []byte

	// Register requests from clients
	register chan *Client

	// Unregister requests from clients
	unregister chan *Client

	// Mutex for thread safety
	mu sync.RWMutex
}

// NewHub creates a new WebSocket hub
func NewHub() *Hub {
	return &Hub{
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
	}
}

// Run starts the hub's event loop
func (h *Hub) Run() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()

			log.Printf("WebSocket: Client %s connected", client.ID)

			// Send welcome message
			welcome := Message{
				Type:      MessageTypeHeartbeat,
				Timestamp: time.Now(),
				Data: map[string]any{
					"message":   "Connected to AI activity stream",
					"client_id": client.ID,
				},
			}
			h.sendToClient(client, welcome)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.Send)
				h.mu.Unlock()
				log.Printf("WebSocket: Client %s disconnected", client.ID)
			} else {
				h.mu.Unlock()
			}

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.Send <- message:
				default:
					// Client's send channel is full, close it
					close(client.Send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()

		case <-ticker.C:
			// Send heartbeat to all clients
			heartbeat := Message{
				Type:      MessageTypeHeartbeat,
				Timestamp: time.Now(),
				Data: map[string]any{
					"message": "ping",
				},
			}
			h.BroadcastMessage(heartbeat)
		}
	}
}

// BroadcastMessage sends a message to all connected clients
func (h *Hub) BroadcastMessage(msg Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("WebSocket: Failed to marshal message: %v", err)
		return
	}

	h.broadcast <- data
}

// BroadcastActivity sends an activity update to all clients
func (h *Hub) BroadcastActivity(activity any) {
	msg := Message{
		Type:      MessageTypeActivity,
		Timestamp: time.Now(),
		Data:      activity,
	}
	h.BroadcastMessage(msg)
}

// BroadcastQueueStats sends queue statistics to all clients
func (h *Hub) BroadcastQueueStats(stats map[string]any) {
	msg := Message{
		Type:      MessageTypeQueueStats,
		Timestamp: time.Now(),
		Data:      stats,
	}
	h.BroadcastMessage(msg)
}

// BroadcastTaskUpdate sends a task update to all clients
func (h *Hub) BroadcastTaskUpdate(task any) {
	msg := Message{
		Type:      MessageTypeTaskUpdate,
		Timestamp: time.Now(),
		Data:      task,
	}
	h.BroadcastMessage(msg)
}

// SendToUser sends a message to all clients of a specific user
func (h *Hub) SendToUser(userID string, msg Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("WebSocket: Failed to marshal message: %v", err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		if client.UserID == userID {
			select {
			case client.Send <- data:
			default:
				// Client's send channel is full
			}
		}
	}
}

// SendToRepo sends a message to all clients watching a specific repository
func (h *Hub) SendToRepo(repoID string, msg Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("WebSocket: Failed to marshal message: %v", err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		if client.RepoID == repoID || client.RepoID == "" {
			select {
			case client.Send <- data:
			default:
				// Client's send channel is full
			}
		}
	}
}

// sendToClient sends a message to a specific client
func (h *Hub) sendToClient(client *Client, msg Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("WebSocket: Failed to marshal message: %v", err)
		return
	}

	select {
	case client.Send <- data:
	default:
		// Client's send channel is full
	}
}

// GetClientCount returns the number of connected clients
func (h *Hub) GetClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// Client methods

// ReadPump pumps messages from the WebSocket connection to the hub
func (c *Client) ReadPump() {
	defer func() {
		c.Hub.unregister <- c
		c.Conn.Close()
	}()

	c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		// Handle incoming messages (e.g., filter requests)
		var msg map[string]any
		if err := json.Unmarshal(message, &msg); err == nil {
			c.handleMessage(msg)
		}
	}
}

// WritePump pumps messages from the hub to the WebSocket connection
func (c *Client) WritePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued messages to the current WebSocket message
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.Send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleMessage processes incoming messages from the client
func (c *Client) handleMessage(msg map[string]any) {
	// Handle filter changes, subscriptions, etc.
	if action, ok := msg["action"].(string); ok {
		switch action {
		case "subscribe_repo":
			if repoID, ok := msg["repo_id"].(string); ok {
				c.RepoID = repoID
				log.Printf("WebSocket: Client %s subscribed to repo %s", c.ID, repoID)
			}
		case "unsubscribe_repo":
			c.RepoID = ""
			log.Printf("WebSocket: Client %s unsubscribed from repo", c.ID)
		}
	}
}
