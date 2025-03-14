package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
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

func TestChatServer_AutoGeneratedUsername(t *testing.T) {
	server := NewChatServer()
	server.Run()

	s := httptest.NewServer(http.HandlerFunc(server.handleConnection))
	defer s.Close()

	wsURL := "ws" + strings.TrimPrefix(s.URL, "http")

	// Connect without username
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	c, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{})
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	// Read welcome message to verify auto-generated username
	var msg Message
	ctx, cancel = context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	if err := wsjson.Read(ctx, c, &msg); err != nil {
		t.Fatalf("Failed to read message: %v", err)
	}

	if msg.Type != "system" || !strings.Contains(msg.Content, "User-") {
		t.Errorf("Expected auto-generated username message, got: %+v", msg)
	}
}

func TestChatServer_InvalidMessage(t *testing.T) {
	server := NewChatServer()
	server.Run()

	s := httptest.NewServer(http.HandlerFunc(server.handleConnection))
	defer s.Close()

	wsURL := "ws" + strings.TrimPrefix(s.URL, "http")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	c, _, err := websocket.Dial(ctx, wsURL+"?username=testuser", &websocket.DialOptions{})
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	// Skip welcome message
	var msg Message
	ctx, cancel = context.WithTimeout(context.Background(), time.Second*5)
	if err := wsjson.Read(ctx, c, &msg); err != nil {
		t.Fatalf("Failed to read welcome message: %v", err)
	}
	cancel()

	// Send invalid JSON
	ctx, cancel = context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	err = c.Write(ctx, websocket.MessageText, []byte("invalid json"))
	if err != nil {
		t.Fatalf("Failed to send invalid message: %v", err)
	}
}

func TestChatServer_ConcurrentBroadcast(t *testing.T) {
	server := NewChatServer()
	server.Run()

	s := httptest.NewServer(http.HandlerFunc(server.handleConnection))
	defer s.Close()

	wsURL := "ws" + strings.TrimPrefix(s.URL, "http")

	// Connect multiple clients concurrently
	numClients := 5
	clients := make([]*websocket.Conn, numClients)

	for i := 0; i < numClients; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		c, _, err := websocket.Dial(ctx, wsURL+fmt.Sprintf("?username=user%d", i), &websocket.DialOptions{})
		cancel()
		if err != nil {
			t.Fatalf("Failed to connect client %d: %v", i, err)
		}
		defer c.Close(websocket.StatusNormalClosure, "")
		clients[i] = c

		// Skip welcome message
		var msg Message
		ctx, cancel = context.WithTimeout(context.Background(), time.Second*5)
		if err := wsjson.Read(ctx, c, &msg); err != nil {
			t.Fatalf("Failed to read welcome message for client %d: %v", i, err)
		}
		cancel()
	}

	// Send messages concurrently
	var wg sync.WaitGroup
	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			msg := Message{
				Type:    "message",
				Content: fmt.Sprintf("Message from client %d", idx),
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
			defer cancel()
			err := wsjson.Write(ctx, clients[idx], msg)
			if err != nil {
				t.Errorf("Failed to send message from client %d: %v", idx, err)
			}
		}(i)
	}
	wg.Wait()

	// Give time for messages to be broadcast
	time.Sleep(time.Millisecond * 100)
}

func TestStaticFileServer(t *testing.T) {
	// Create a temporary static file for testing
	tmpDir := t.TempDir()
	if err := os.WriteFile(tmpDir+"/index.html", []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create test server with the temporary directory
	http.Handle("/", http.FileServer(http.Dir(tmpDir)))

	s := httptest.NewServer(http.DefaultServeMux)
	defer s.Close()

	// Test file serving
	resp, err := http.Get(s.URL + "/index.html")
	if err != nil {
		t.Fatalf("Failed to get static file: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	if string(body) != "test content" {
		t.Errorf("Expected body 'test content', got '%s'", string(body))
	}
}

func TestChatServer_BroadcastErrors(t *testing.T) {
	server := NewChatServer()
	server.Run()

	s := httptest.NewServer(http.HandlerFunc(server.handleConnection))
	defer s.Close()

	wsURL := "ws" + strings.TrimPrefix(s.URL, "http")

	// Connect first client (this one will stay healthy)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	c1, _, err := websocket.Dial(ctx, wsURL+"?username=user1", &websocket.DialOptions{})
	cancel()
	if err != nil {
		t.Fatalf("Failed to connect client 1: %v", err)
	}
	defer c1.Close(websocket.StatusNormalClosure, "")

	// Skip welcome message for first client
	var msg Message
	ctx, cancel = context.WithTimeout(context.Background(), time.Second*5)
	if err := wsjson.Read(ctx, c1, &msg); err != nil {
		t.Fatalf("Failed to read welcome message for client 1: %v", err)
	}
	cancel()

	// Connect second client (this one will be closed to simulate error)
	ctx, cancel = context.WithTimeout(context.Background(), time.Second*5)
	c2, _, err := websocket.Dial(ctx, wsURL+"?username=user2", &websocket.DialOptions{})
	cancel()
	if err != nil {
		t.Fatalf("Failed to connect client 2: %v", err)
	}

	// Skip welcome message for second client
	ctx, cancel = context.WithTimeout(context.Background(), time.Second*5)
	if err := wsjson.Read(ctx, c2, &msg); err != nil {
		t.Fatalf("Failed to read welcome message for client 2: %v", err)
	}
	cancel()

	// Read join message on first client for second client's join
	ctx, cancel = context.WithTimeout(context.Background(), time.Second*5)
	if err := wsjson.Read(ctx, c1, &msg); err != nil {
		t.Fatalf("Failed to read join message on client 1: %v", err)
	}
	cancel()

	// Force close the second client's connection to simulate an error condition
	c2.Close(websocket.StatusInternalError, "simulated error")

	// Send a message from the first client
	testMessage := Message{
		Type:    "message",
		Content: "This message should trigger cleanup of dead connection",
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Second*5)
	if err := wsjson.Write(ctx, c1, testMessage); err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}
	cancel()

	// Give time for the server to process the error and clean up
	time.Sleep(time.Millisecond * 100)

	// Verify that the dead client was removed
	server.clientsMtx.Lock()
	clientCount := len(server.clients)
	server.clientsMtx.Unlock()

	if clientCount != 1 {
		t.Errorf("Expected 1 client after error cleanup, got %d", clientCount)
	}

	// Verify the first client is still connected by sending another message
	testMessage = Message{
		Type:    "message",
		Content: "Checking connection is still alive",
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Second*5)
	if err := wsjson.Write(ctx, c1, testMessage); err != nil {
		t.Fatalf("Failed to send follow-up message: %v", err)
	}
	cancel()
}

func TestChatServer_BroadcastConcurrentModification(t *testing.T) {
	server := NewChatServer()
	server.Run()

	s := httptest.NewServer(http.HandlerFunc(server.handleConnection))
	defer s.Close()

	wsURL := "ws" + strings.TrimPrefix(s.URL, "http")

	// Connect multiple clients
	numClients := 5
	clients := make([]*websocket.Conn, numClients)

	for i := 0; i < numClients; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		c, _, err := websocket.Dial(ctx, wsURL+fmt.Sprintf("?username=user%d", i), &websocket.DialOptions{})
		cancel()
		if err != nil {
			t.Fatalf("Failed to connect client %d: %v", i, err)
		}
		clients[i] = c
		defer c.Close(websocket.StatusNormalClosure, "")

		// Skip welcome message
		var msg Message
		ctx, cancel = context.WithTimeout(context.Background(), time.Second*5)
		if err := wsjson.Read(ctx, c, &msg); err != nil {
			t.Fatalf("Failed to read welcome message for client %d: %v", i, err)
		}
		cancel()
	}

	// Concurrently send messages and close connections
	var wg sync.WaitGroup
	for i := 0; i < numClients; i++ {
		// Half of the clients will send messages
		if i%2 == 0 {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				msg := Message{
					Type:    "message",
					Content: fmt.Sprintf("Message from client %d", idx),
				}
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
				defer cancel()
				wsjson.Write(ctx, clients[idx], msg)
			}(i)
		} else {
			// The other half will close their connections
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				time.Sleep(time.Millisecond * 10) // Small delay to ensure some messages are sent
				clients[idx].Close(websocket.StatusNormalClosure, "")
			}(i)
		}
	}
	wg.Wait()

	// Give time for the server to process everything
	time.Sleep(time.Millisecond * 100)

	// Verify client count is correct (should be number of clients that were sending messages)
	server.clientsMtx.Lock()
	clientCount := len(server.clients)
	server.clientsMtx.Unlock()

	expectedClients := (numClients + 1) / 2 // Round up division
	if clientCount > expectedClients {
		t.Errorf("Expected at most %d clients after concurrent operations, got %d", expectedClients, clientCount)
	}
}
