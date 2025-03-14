package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const (
	maxUsernameLength = 50
	maxMessageLength  = 5000
)

var (
	validUsernameRegex = regexp.MustCompile("^[a-zA-Z0-9_-]+$")
)

// Message represents a chat message
type Message struct {
	Type     string `json:"type"`
	Username string `json:"username"`
	Content  string `json:"content"`
	Time     string `json:"time"`
}

// Validate checks if the message is valid
func (m *Message) Validate() error {
	if m.Content == "" {
		return fmt.Errorf("message content cannot be empty")
	}
	if len(m.Content) > maxMessageLength {
		return fmt.Errorf("message content too long (max %d characters)", maxMessageLength)
	}
	if m.Type != "" && m.Type != "message" && m.Type != "system" {
		return fmt.Errorf("invalid message type: %s", m.Type)
	}
	return nil
}

// Client represents a connected chat client
type Client struct {
	conn     *websocket.Conn
	username string
}

// ChatServer manages the chat service
type ChatServer struct {
	clients    map[*Client]bool
	clientsMtx sync.Mutex
	broadcast  chan Message
}

// NewChatServer creates a new chat server instance
func NewChatServer() *ChatServer {
	return &ChatServer{
		clients:   make(map[*Client]bool),
		broadcast: make(chan Message),
	}
}

// Run starts the broadcast goroutine
func (cs *ChatServer) Run() {
	go cs.handleBroadcasts()
}

// handleBroadcasts sends messages to all connected clients
func (cs *ChatServer) handleBroadcasts() {
	for msg := range cs.broadcast {
		cs.clientsMtx.Lock()
		for client := range cs.clients {
			// Create a context with timeout for each write
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
			err := wsjson.Write(ctx, client.conn, msg)
			cancel()

			if err != nil {
				log.Printf("Error sending message to client: %v", err)
				client.conn.Close(websocket.StatusInternalError, "Failed to send message")
				delete(cs.clients, client)
			}
		}
		cs.clientsMtx.Unlock()
	}
}

// validateUsername checks if a username is valid
func (cs *ChatServer) validateUsername(username string) error {
	if username == "" {
		return nil // Empty username will be auto-generated
	}
	if len(username) > maxUsernameLength {
		return fmt.Errorf("username too long (max %d characters)", maxUsernameLength)
	}
	if !validUsernameRegex.MatchString(username) {
		return fmt.Errorf("username contains invalid characters (only letters, numbers, underscore, and hyphen allowed)")
	}
	return nil
}

// handleConnection manages a WebSocket connection
func (cs *ChatServer) handleConnection(w http.ResponseWriter, r *http.Request) {
	// Validate username before upgrading connection
	username := r.URL.Query().Get("username")
	if err := cs.validateUsername(username); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Allow connections from any origin for development purposes
		InsecureSkipVerify: true,
	})
	if err != nil {
		if websocket.CloseStatus(err) == websocket.StatusProtocolError {
			http.Error(w, "Upgrade Required", http.StatusUpgradeRequired)
		} else {
			http.Error(w, "Bad Request", http.StatusBadRequest)
		}
		log.Printf("WebSocket accept error: %v", err)
		return
	}
	defer c.CloseNow()

	// Auto-generate username if not provided
	if username == "" {
		username = fmt.Sprintf("User-%d", time.Now().UnixNano()%10000)
	}

	// Create a new client
	client := &Client{
		conn:     c,
		username: username,
	}

	// Register client
	cs.clientsMtx.Lock()
	cs.clients[client] = true
	cs.clientsMtx.Unlock()

	// Send welcome message
	joinMsg := Message{
		Type:     "system",
		Username: "Server",
		Content:  fmt.Sprintf("%s has joined the chat", username),
		Time:     time.Now().Format(time.RFC3339),
	}
	cs.broadcast <- joinMsg

	// Handle messages in a loop
	for {
		var msg Message
		ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
		err := wsjson.Read(ctx, c, &msg)
		cancel()

		if websocket.CloseStatus(err) == websocket.StatusGoingAway ||
			websocket.CloseStatus(err) == websocket.StatusNormalClosure {
			log.Printf("Client %s disconnected gracefully", client.username)
			break
		} else if err != nil {
			log.Printf("WebSocket read error: %v", err)
			break
		}

		// Add metadata to message
		msg.Username = client.username
		msg.Time = time.Now().Format(time.RFC3339)
		if msg.Type == "" {
			msg.Type = "message"
		}

		// Validate message
		if err := msg.Validate(); err != nil {
			log.Printf("Invalid message from %s: %v", client.username, err)
			continue
		}

		// Broadcast message to all clients
		cs.broadcast <- msg
	}

	// Remove client on disconnect
	cs.clientsMtx.Lock()
	delete(cs.clients, client)
	cs.clientsMtx.Unlock()

	// Send leave message
	leaveMsg := Message{
		Type:     "system",
		Username: "Server",
		Content:  fmt.Sprintf("%s has left the chat", username),
		Time:     time.Now().Format(time.RFC3339),
	}
	cs.broadcast <- leaveMsg
}

func main() {
	// Create and run chat server
	chatServer := NewChatServer()
	chatServer.Run()

	// Create static file server
	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/", fs)

	// WebSocket endpoint
	http.HandleFunc("/ws", chatServer.handleConnection)

	// Start HTTP server
	port := "8080"
	log.Printf("Server starting at http://localhost:%s", port)
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
