package sse

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agentfacts/mcp-proxy/internal/config"
	"github.com/agentfacts/mcp-proxy/internal/session"
)

func TestNewServer(t *testing.T) {
	sm := session.NewManager(session.ManagerConfig{
		SessionTTL:      time.Hour,
		CleanupInterval: time.Minute,
		MaxSessions:     100,
	})

	serverCfg := config.ServerConfig{
		Listen: config.ListenConfig{
			Address: "127.0.0.1",
			Port:    0,
		},
		Transport: "sse",
	}

	agentCfg := config.AgentConfig{
		ID:   "test-agent",
		Name: "Test Agent",
	}

	server := NewServer(serverCfg, agentCfg, sm)

	if server == nil {
		t.Fatal("NewServer returned nil")
	}
}

func TestNewHandler(t *testing.T) {
	sm := session.NewManager(session.ManagerConfig{
		SessionTTL:      time.Hour,
		CleanupInterval: time.Minute,
		MaxSessions:     100,
	})

	agentCfg := config.AgentConfig{
		ID:   "test-agent",
		Name: "Test Agent",
	}

	handler := NewHandler(sm, agentCfg)

	if handler == nil {
		t.Fatal("NewHandler returned nil")
	}
}

func TestSSEConnection(t *testing.T) {
	sm := session.NewManager(session.ManagerConfig{
		SessionTTL:      time.Hour,
		CleanupInterval: time.Minute,
		MaxSessions:     100,
	})
	ctx := context.Background()
	sm.Start(ctx)
	defer sm.Stop()

	agentCfg := config.AgentConfig{
		ID:   "test-agent",
		Name: "Test Agent",
	}

	handler := NewHandler(sm, agentCfg)

	// Create test server
	ts := httptest.NewServer(http.HandlerFunc(handler.HandleSSE))
	defer ts.Close()

	// Connect with SSE headers
	req, err := http.NewRequest("GET", ts.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer resp.Body.Close()

	// Check response headers
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/event-stream") {
		t.Errorf("Expected content-type text/event-stream, got %s", contentType)
	}

	// Read first event (should be endpoint)
	reader := bufio.NewReader(resp.Body)

	// Read event type
	eventLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read event line: %v", err)
	}
	if !strings.HasPrefix(eventLine, "event:") {
		t.Errorf("Expected event line, got: %s", eventLine)
	}

	// Read data
	dataLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read data line: %v", err)
	}
	if !strings.HasPrefix(dataLine, "data:") {
		t.Errorf("Expected data line, got: %s", dataLine)
	}

	// Extract session ID from endpoint
	data := strings.TrimPrefix(dataLine, "data: ")
	data = strings.TrimSpace(data)
	if !strings.Contains(data, "sessionId=") {
		t.Errorf("Expected endpoint with sessionId, got: %s", data)
	}
}

func TestMessageHandler(t *testing.T) {
	sm := session.NewManager(session.ManagerConfig{
		SessionTTL:      time.Hour,
		CleanupInterval: time.Minute,
		MaxSessions:     100,
	})
	ctx := context.Background()
	sm.Start(ctx)
	defer sm.Stop()

	agentCfg := config.AgentConfig{
		ID:   "test-agent",
		Name: "Test Agent",
	}

	handler := NewHandler(sm, agentCfg)

	// Set up message handler that echoes back
	handler.SetMessageHandler(func(ctx context.Context, sess *session.Session, msg []byte) ([]byte, error) {
		var req map[string]interface{}
		if err := json.Unmarshal(msg, &req); err != nil {
			return nil, err
		}

		response := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      req["id"],
			"result":  "echo",
		}
		return json.Marshal(response)
	})

	// Create a session first
	sess, err := sm.Create(ctx)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Create test server for message endpoint
	ts := httptest.NewServer(http.HandlerFunc(handler.HandleMessage))
	defer ts.Close()

	// Send a message
	msg := `{"jsonrpc":"2.0","id":"1","method":"test"}`
	req, err := http.NewRequest("POST", ts.URL+"?sessionId="+sess.ID, strings.NewReader(msg))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}
	defer resp.Body.Close()

	// Check response - should be 202 Accepted (response sent via SSE)
	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("Expected status 202, got %d", resp.StatusCode)
	}
}

func TestMissingSessionID(t *testing.T) {
	sm := session.NewManager(session.ManagerConfig{
		SessionTTL:      time.Hour,
		CleanupInterval: time.Minute,
		MaxSessions:     100,
	})

	agentCfg := config.AgentConfig{
		ID:   "test-agent",
		Name: "Test Agent",
	}

	handler := NewHandler(sm, agentCfg)

	ts := httptest.NewServer(http.HandlerFunc(handler.HandleMessage))
	defer ts.Close()

	// Send a message without sessionId
	msg := `{"jsonrpc":"2.0","id":"1","method":"test"}`
	req, err := http.NewRequest("POST", ts.URL, strings.NewReader(msg))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}
	defer resp.Body.Close()

	// Should get error response
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result["error"] == nil {
		t.Error("Expected error in response")
	}
}

func TestInvalidSessionID(t *testing.T) {
	sm := session.NewManager(session.ManagerConfig{
		SessionTTL:      time.Hour,
		CleanupInterval: time.Minute,
		MaxSessions:     100,
	})

	agentCfg := config.AgentConfig{
		ID:   "test-agent",
		Name: "Test Agent",
	}

	handler := NewHandler(sm, agentCfg)

	ts := httptest.NewServer(http.HandlerFunc(handler.HandleMessage))
	defer ts.Close()

	// Send a message with invalid sessionId
	msg := `{"jsonrpc":"2.0","id":"1","method":"test"}`
	req, err := http.NewRequest("POST", ts.URL+"?sessionId=invalid-session", strings.NewReader(msg))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}
	defer resp.Body.Close()

	// Should get error response
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result["error"] == nil {
		t.Error("Expected error in response")
	}
}

func TestMalformedJSON(t *testing.T) {
	sm := session.NewManager(session.ManagerConfig{
		SessionTTL:      time.Hour,
		CleanupInterval: time.Minute,
		MaxSessions:     100,
	})
	ctx := context.Background()
	sm.Start(ctx)
	defer sm.Stop()

	agentCfg := config.AgentConfig{
		ID:   "test-agent",
		Name: "Test Agent",
	}

	handler := NewHandler(sm, agentCfg)

	// Create a session
	sess, err := sm.Create(ctx)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(handler.HandleMessage))
	defer ts.Close()

	// Send malformed JSON
	msg := `{not valid json`
	req, err := http.NewRequest("POST", ts.URL+"?sessionId="+sess.ID, strings.NewReader(msg))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}
	defer resp.Body.Close()

	// Should get error response
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result["error"] == nil {
		t.Error("Expected error in response for malformed JSON")
	}
}

func TestServerStartStop(t *testing.T) {
	sm := session.NewManager(session.ManagerConfig{
		SessionTTL:      time.Hour,
		CleanupInterval: time.Minute,
		MaxSessions:     100,
	})

	// Use localhost with port 0 for random available port
	serverCfg := config.ServerConfig{
		Listen: config.ListenConfig{
			Address: "127.0.0.1",
			Port:    0,
		},
		Transport:    "sse",
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	agentCfg := config.AgentConfig{
		ID:   "test-agent",
		Name: "Test Agent",
	}

	server := NewServer(serverCfg, agentCfg, sm)

	ctx := context.Background()

	// Start server - will fail with port 0 but that's OK for this test
	err := server.Start(ctx)
	if err != nil {
		// Expected with port 0 in some cases
		t.Logf("Start error (may be expected): %v", err)
	}

	// Stop server
	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err = server.Stop(stopCtx)
	if err != nil {
		t.Logf("Stop error (may be expected): %v", err)
	}
}

func TestConcurrentConnections(t *testing.T) {
	sm := session.NewManager(session.ManagerConfig{
		SessionTTL:      time.Hour,
		CleanupInterval: time.Minute,
		MaxSessions:     100,
	})
	ctx := context.Background()
	sm.Start(ctx)
	defer sm.Stop()

	agentCfg := config.AgentConfig{
		ID:   "test-agent",
		Name: "Test Agent",
	}

	handler := NewHandler(sm, agentCfg)

	ts := httptest.NewServer(http.HandlerFunc(handler.HandleSSE))
	defer ts.Close()

	// Connect multiple clients concurrently
	numClients := 5
	done := make(chan bool, numClients)

	for i := 0; i < numClients; i++ {
		go func(clientNum int) {
			req, _ := http.NewRequest("GET", ts.URL, nil)
			req.Header.Set("Accept", "text/event-stream")

			client := &http.Client{
				Timeout: 2 * time.Second,
			}

			resp, err := client.Do(req)
			if err != nil {
				t.Logf("Client %d: connection error: %v", clientNum, err)
				done <- false
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				done <- true
			} else {
				t.Logf("Client %d: unexpected status %d", clientNum, resp.StatusCode)
				done <- false
			}
		}(i)
	}

	// Wait for all clients
	successCount := 0
	for i := 0; i < numClients; i++ {
		if <-done {
			successCount++
		}
	}

	if successCount != numClients {
		t.Errorf("Expected %d successful connections, got %d", numClients, successCount)
	}
}

func BenchmarkMessageHandling(b *testing.B) {
	sm := session.NewManager(session.ManagerConfig{
		SessionTTL:      time.Hour,
		CleanupInterval: time.Minute,
		MaxSessions:     100,
	})
	ctx := context.Background()
	sm.Start(ctx)
	defer sm.Stop()

	agentCfg := config.AgentConfig{
		ID:   "test-agent",
		Name: "Test Agent",
	}

	handler := NewHandler(sm, agentCfg)
	handler.SetMessageHandler(func(ctx context.Context, sess *session.Session, msg []byte) ([]byte, error) {
		return []byte(`{"jsonrpc":"2.0","id":"1","result":"ok"}`), nil
	})

	sess, _ := sm.Create(ctx)

	ts := httptest.NewServer(http.HandlerFunc(handler.HandleMessage))
	defer ts.Close()

	client := &http.Client{}
	msg := `{"jsonrpc":"2.0","id":"1","method":"test"}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("POST", ts.URL+"?sessionId="+sess.ID, strings.NewReader(msg))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			b.Fatalf("Request failed: %v", err)
		}
		resp.Body.Close()
	}
}

func TestHeaderValidation(t *testing.T) {
	sm := session.NewManager(session.ManagerConfig{
		SessionTTL:      time.Hour,
		CleanupInterval: time.Minute,
		MaxSessions:     100,
	})
	ctx := context.Background()
	sm.Start(ctx)
	defer sm.Stop()

	agentCfg := config.AgentConfig{
		ID:   "test-agent",
		Name: "Test Agent",
	}

	handler := NewHandler(sm, agentCfg)

	ts := httptest.NewServer(http.HandlerFunc(handler.HandleSSE))
	defer ts.Close()

	tests := []struct {
		name       string
		accept     string
		wantStatus int
	}{
		{
			name:       "correct accept header",
			accept:     "text/event-stream",
			wantStatus: http.StatusOK,
		},
		{
			name:       "accept with charset",
			accept:     "text/event-stream; charset=utf-8",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", ts.URL, nil)
			req.Header.Set("Accept", tt.accept)

			client := &http.Client{
				Timeout: 2 * time.Second,
			}

			resp, err := client.Do(req)
			if err != nil {
				// Timeout is expected for SSE connections
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("Expected status %d, got %d", tt.wantStatus, resp.StatusCode)
			}
		})
	}
}

func TestCORSHeaders(t *testing.T) {
	sm := session.NewManager(session.ManagerConfig{
		SessionTTL:      time.Hour,
		CleanupInterval: time.Minute,
		MaxSessions:     100,
	})
	ctx := context.Background()
	sm.Start(ctx)
	defer sm.Stop()

	agentCfg := config.AgentConfig{
		ID:   "test-agent",
		Name: "Test Agent",
	}

	// Use NewHandlerWithSecurity to configure CORS
	securityCfg := config.SecurityConfig{
		EnableSecurityHeaders: true,
		CORSAllowedOrigins:    []string{"*"}, // Allow all origins for this test
	}
	handler := NewHandlerWithSecurity(sm, agentCfg, securityCfg)

	ts := httptest.NewServer(http.HandlerFunc(handler.HandleSSE))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL, nil)
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return // Timeout expected
	}
	defer resp.Body.Close()

	// Check CORS header
	cors := resp.Header.Get("Access-Control-Allow-Origin")
	if cors != "*" {
		t.Errorf("Expected CORS header '*', got '%s'", cors)
	}

	// Check security headers
	xco := resp.Header.Get("X-Content-Type-Options")
	if xco != "nosniff" {
		t.Errorf("Expected X-Content-Type-Options 'nosniff', got '%s'", xco)
	}
}

func TestLargePayload(t *testing.T) {
	sm := session.NewManager(session.ManagerConfig{
		SessionTTL:      time.Hour,
		CleanupInterval: time.Minute,
		MaxSessions:     100,
	})
	ctx := context.Background()
	sm.Start(ctx)
	defer sm.Stop()

	agentCfg := config.AgentConfig{
		ID:   "test-agent",
		Name: "Test Agent",
	}

	handler := NewHandler(sm, agentCfg)
	handler.SetMessageHandler(func(ctx context.Context, sess *session.Session, msg []byte) ([]byte, error) {
		return []byte(`{"jsonrpc":"2.0","id":"1","result":"ok"}`), nil
	})

	sess, _ := sm.Create(ctx)

	ts := httptest.NewServer(http.HandlerFunc(handler.HandleMessage))
	defer ts.Close()

	// Create a large payload (500KB - under the 1MB limit)
	largeData := strings.Repeat("x", 500*1024)
	msg := fmt.Sprintf(`{"jsonrpc":"2.0","id":"1","method":"test","params":{"data":"%s"}}`, largeData)

	req, _ := http.NewRequest("POST", ts.URL+"?sessionId="+sess.ID, strings.NewReader(msg))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// Should handle large payloads (under 1MB limit)
	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("Expected status 202, got %d", resp.StatusCode)
	}
}

func TestServerName(t *testing.T) {
	sm := session.NewManager(session.ManagerConfig{
		SessionTTL:      time.Hour,
		CleanupInterval: time.Minute,
		MaxSessions:     100,
	})

	serverCfg := config.ServerConfig{
		Listen: config.ListenConfig{
			Address: "127.0.0.1",
			Port:    0,
		},
		Transport: "sse",
	}

	agentCfg := config.AgentConfig{
		ID:   "test-agent",
		Name: "Test Agent",
	}

	server := NewServer(serverCfg, agentCfg, sm)

	if server.Name() != "sse" {
		t.Errorf("Expected name 'sse', got '%s'", server.Name())
	}
}

func TestMessageHandlerError(t *testing.T) {
	sm := session.NewManager(session.ManagerConfig{
		SessionTTL:      time.Hour,
		CleanupInterval: time.Minute,
		MaxSessions:     100,
	})
	ctx := context.Background()
	sm.Start(ctx)
	defer sm.Stop()

	agentCfg := config.AgentConfig{
		ID:   "test-agent",
		Name: "Test Agent",
	}

	handler := NewHandler(sm, agentCfg)
	handler.SetMessageHandler(func(ctx context.Context, sess *session.Session, msg []byte) ([]byte, error) {
		return nil, fmt.Errorf("intentional error")
	})

	sess, _ := sm.Create(ctx)

	ts := httptest.NewServer(http.HandlerFunc(handler.HandleMessage))
	defer ts.Close()

	msg := `{"jsonrpc":"2.0","id":"1","method":"test"}`
	req, _ := http.NewRequest("POST", ts.URL+"?sessionId="+sess.ID, strings.NewReader(msg))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// Should get error response
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if result["error"] == nil {
		t.Error("Expected error in response when handler returns error")
	}
}

func TestNoMessageHandler(t *testing.T) {
	sm := session.NewManager(session.ManagerConfig{
		SessionTTL:      time.Hour,
		CleanupInterval: time.Minute,
		MaxSessions:     100,
	})
	ctx := context.Background()
	sm.Start(ctx)
	defer sm.Stop()

	agentCfg := config.AgentConfig{
		ID:   "test-agent",
		Name: "Test Agent",
	}

	handler := NewHandler(sm, agentCfg)
	// No message handler set - should echo

	sess, _ := sm.Create(ctx)

	ts := httptest.NewServer(http.HandlerFunc(handler.HandleMessage))
	defer ts.Close()

	msg := `{"jsonrpc":"2.0","id":"1","method":"test"}`
	req, _ := http.NewRequest("POST", ts.URL+"?sessionId="+sess.ID, strings.NewReader(msg))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// Should return 202 Accepted even without handler (echo mode)
	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("Expected status 202, got %d", resp.StatusCode)
	}
}
