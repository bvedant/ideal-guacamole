package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestChatServer_NewConnection(t *testing.T) {
	server := NewChatServer()
	server.Run()

	// Create test server
	s := httptest.NewServer(http.HandlerFunc(server.handleConnection))
	defer s.Close()

	// Convert http URL to ws URL
	wsURL := "ws" + strings.TrimPrefix(s.URL, "http")

	// Connect a test client
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	c, _, err := websocket.Dial(ctx, wsURL+"?username=testuser", &websocket.DialOptions{})
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	// Verify client was added
	if len(server.clients) != 1 {
		t.Errorf("Expected 1 client, got %d", len(server.clients))
	}

	// Read welcome message
	var msg Message
	ctx, cancel = context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	if err := wsjson.Read(ctx, c, &msg); err != nil {
		t.Fatalf("Failed to read message: %v", err)
	}

	if msg.Type != "system" || !strings.Contains(msg.Content, "testuser has joined") {
		t.Errorf("Unexpected welcome message: %+v", msg)
	}
}

func TestChatServer_MessageBroadcast(t *testing.T) {
	server := NewChatServer()
	server.Run()

	s := httptest.NewServer(http.HandlerFunc(server.handleConnection))
	defer s.Close()

	wsURL := "ws" + strings.TrimPrefix(s.URL, "http")

	// Connect first client
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	c1, _, err := websocket.Dial(ctx, wsURL+"?username=user1", &websocket.DialOptions{})
	if err != nil {
		t.Fatalf("Failed to connect client 1: %v", err)
	}
	defer c1.Close(websocket.StatusNormalClosure, "")

	// Read the welcome message for first client
	var msg Message
	ctx, cancel = context.WithTimeout(context.Background(), time.Second*5)
	if err := wsjson.Read(ctx, c1, &msg); err != nil {
		t.Fatalf("Failed to read welcome message for client 1: %v", err)
	}
	cancel()
	if msg.Type != "system" || !strings.Contains(msg.Content, "user1 has joined") {
		t.Fatalf("Unexpected welcome message for client 1: %+v", msg)
	}

	// Connect second client
	ctx, cancel = context.WithTimeout(context.Background(), time.Second*5)
	c2, _, err := websocket.Dial(ctx, wsURL+"?username=user2", &websocket.DialOptions{})
	if err != nil {
		t.Fatalf("Failed to connect client 2: %v", err)
	}
	cancel()
	defer c2.Close(websocket.StatusNormalClosure, "")

	// Read join message on client 1 for client 2's join
	ctx, cancel = context.WithTimeout(context.Background(), time.Second*5)
	if err := wsjson.Read(ctx, c1, &msg); err != nil {
		t.Fatalf("Failed to read join message on client 1: %v", err)
	}
	cancel()
	if msg.Type != "system" || !strings.Contains(msg.Content, "user2 has joined") {
		t.Fatalf("Unexpected join message on client 1: %+v", msg)
	}

	// Read own welcome message on client 2
	ctx, cancel = context.WithTimeout(context.Background(), time.Second*5)
	if err := wsjson.Read(ctx, c2, &msg); err != nil {
		t.Fatalf("Failed to read welcome message on client 2: %v", err)
	}
	cancel()
	if msg.Type != "system" || !strings.Contains(msg.Content, "user2 has joined") {
		t.Fatalf("Unexpected welcome message on client 2: %+v", msg)
	}

	// Send a message from client 1
	testMessage := Message{
		Type:    "message",
		Content: "Hello, World!",
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Second*5)
	if err := wsjson.Write(ctx, c1, testMessage); err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}
	cancel()

	// Read the message on client 2
	ctx, cancel = context.WithTimeout(context.Background(), time.Second*5)
	if err := wsjson.Read(ctx, c2, &msg); err != nil {
		t.Fatalf("Client 2 failed to receive message: %v", err)
	}
	cancel()

	if msg.Type != "message" || msg.Content != testMessage.Content || msg.Username != "user1" {
		t.Errorf("Unexpected message received: %+v", msg)
	}
}

func TestChatServer_ClientDisconnect(t *testing.T) {
	server := NewChatServer()
	server.Run()

	s := httptest.NewServer(http.HandlerFunc(server.handleConnection))
	defer s.Close()

	wsURL := "ws" + strings.TrimPrefix(s.URL, "http")

	// Connect a client
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	c, _, err := websocket.Dial(ctx, wsURL+"?username=disconnectuser", &websocket.DialOptions{})
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	// Skip welcome message
	var msg Message
	ctx, cancel = context.WithTimeout(context.Background(), time.Second*5)
	if err := wsjson.Read(ctx, c, &msg); err != nil {
		t.Fatalf("Failed to read welcome message: %v", err)
	}
	cancel()

	// Close the connection
	if err := c.Close(websocket.StatusNormalClosure, ""); err != nil {
		t.Fatalf("Failed to close connection: %v", err)
	}

	// Give the server time to process the disconnection
	time.Sleep(time.Millisecond * 100)

	// Verify client was removed
	server.clientsMtx.Lock()
	clientCount := len(server.clients)
	server.clientsMtx.Unlock()

	if clientCount != 0 {
		t.Errorf("Expected 0 clients after disconnect, got %d", clientCount)
	}
}
