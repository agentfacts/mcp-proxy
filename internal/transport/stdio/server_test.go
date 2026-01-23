package stdio

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/agentfacts/mcp-proxy/internal/config"
	"github.com/agentfacts/mcp-proxy/internal/session"
)

func newTestSessionManager() *session.Manager {
	return session.NewManager(session.ManagerConfig{
		SessionTTL:      time.Hour,
		CleanupInterval: time.Minute,
		MaxSessions:     100,
	})
}

func TestServerStartStop(t *testing.T) {
	sessionMgr := newTestSessionManager()
	agentCfg := config.AgentConfig{
		ID:           "test-agent",
		Name:         "Test Agent",
		Capabilities: []string{"tools"},
	}

	stdin := strings.NewReader("")
	stdout := &bytes.Buffer{}

	server := NewServerWithIO(agentCfg, sessionMgr, stdin, stdout)

	ctx := context.Background()

	// Start should succeed
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Starting again should fail
	if err := server.Start(ctx); err == nil {
		t.Fatal("Expected error on second Start, got nil")
	}

	// Stop should succeed
	stopCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := server.Stop(stopCtx); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Stopping again should be a no-op
	if err := server.Stop(stopCtx); err != nil {
		t.Fatalf("Second Stop failed: %v", err)
	}
}

func TestServerName(t *testing.T) {
	sessionMgr := newTestSessionManager()
	server := NewServer(config.AgentConfig{}, sessionMgr)

	if got := server.Name(); got != "stdio" {
		t.Errorf("Name() = %q, want %q", got, "stdio")
	}
}

func TestServerMessageProcessing(t *testing.T) {
	sessionMgr := newTestSessionManager()
	agentCfg := config.AgentConfig{
		ID:   "test-agent",
		Name: "Test Agent",
	}

	// Create a pipe to simulate stdin
	stdinReader, stdinWriter := io.Pipe()
	stdout := &bytes.Buffer{}

	server := NewServerWithIO(agentCfg, sessionMgr, stdinReader, stdout)

	// Set up a message handler that returns a response
	server.SetMessageHandler(func(ctx context.Context, sess *session.Session, msg []byte) ([]byte, error) {
		// Parse request and create response
		var req map[string]interface{}
		if err := json.Unmarshal(msg, &req); err != nil {
			return nil, err
		}

		response := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      req["id"],
			"result":  map[string]interface{}{"status": "ok"},
		}
		return json.Marshal(response)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Send a message
	request := `{"jsonrpc":"2.0","method":"test","id":1}`
	go func() {
		stdinWriter.Write([]byte(request + "\n"))
		// Give time for processing then close
		time.Sleep(100 * time.Millisecond)
		stdinWriter.Close()
	}()

	// Wait for response
	time.Sleep(200 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(ctx, time.Second)
	defer stopCancel()
	server.Stop(stopCtx)

	// Check response
	output := stdout.String()
	if output == "" {
		t.Fatal("Expected output, got empty string")
	}

	var response map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &response); err != nil {
		t.Fatalf("Failed to parse response: %v, output was: %s", err, output)
	}

	if response["jsonrpc"] != "2.0" {
		t.Errorf("Expected jsonrpc 2.0, got %v", response["jsonrpc"])
	}

	if response["id"].(float64) != 1 {
		t.Errorf("Expected id 1, got %v", response["id"])
	}
}

func TestServerEchoWithoutHandler(t *testing.T) {
	sessionMgr := newTestSessionManager()
	agentCfg := config.AgentConfig{
		ID:   "test-agent",
		Name: "Test Agent",
	}

	stdinReader, stdinWriter := io.Pipe()
	stdout := &bytes.Buffer{}

	server := NewServerWithIO(agentCfg, sessionMgr, stdinReader, stdout)
	// Don't set a message handler - should echo

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	request := `{"jsonrpc":"2.0","method":"test","id":1}`
	go func() {
		stdinWriter.Write([]byte(request + "\n"))
		time.Sleep(100 * time.Millisecond)
		stdinWriter.Close()
	}()

	time.Sleep(200 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(ctx, time.Second)
	defer stopCancel()
	server.Stop(stopCtx)

	output := strings.TrimSpace(stdout.String())
	if output != request {
		t.Errorf("Expected echo of request, got %s", output)
	}
}

func TestServerInvalidJSON(t *testing.T) {
	sessionMgr := newTestSessionManager()
	agentCfg := config.AgentConfig{
		ID:   "test-agent",
		Name: "Test Agent",
	}

	stdinReader, stdinWriter := io.Pipe()
	stdout := &bytes.Buffer{}

	server := NewServerWithIO(agentCfg, sessionMgr, stdinReader, stdout)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Send invalid JSON
	go func() {
		stdinWriter.Write([]byte("not valid json\n"))
		time.Sleep(100 * time.Millisecond)
		stdinWriter.Close()
	}()

	time.Sleep(200 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(ctx, time.Second)
	defer stopCancel()
	server.Stop(stopCtx)

	output := stdout.String()
	if !strings.Contains(output, "error") {
		t.Errorf("Expected error response for invalid JSON, got: %s", output)
	}

	var response map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &response); err != nil {
		t.Fatalf("Failed to parse error response: %v", err)
	}

	errObj, ok := response["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected error object in response")
	}

	if errObj["code"].(float64) != -32700 {
		t.Errorf("Expected error code -32700, got %v", errObj["code"])
	}
}

func TestServerEOFShutdown(t *testing.T) {
	sessionMgr := newTestSessionManager()
	agentCfg := config.AgentConfig{
		ID:   "test-agent",
		Name: "Test Agent",
	}

	// Use a simple reader that immediately returns EOF
	stdin := strings.NewReader("")
	stdout := &bytes.Buffer{}

	server := NewServerWithIO(agentCfg, sessionMgr, stdin, stdout)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Server should detect EOF and be ready to stop
	time.Sleep(100 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(ctx, time.Second)
	defer stopCancel()
	if err := server.Stop(stopCtx); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestReaderBasic(t *testing.T) {
	input := `{"jsonrpc":"2.0","method":"test","id":1}
{"jsonrpc":"2.0","method":"test2","id":2}
`
	reader := NewReader(strings.NewReader(input))

	msg1, err := reader.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage() error: %v", err)
	}

	var req1 map[string]interface{}
	if err := json.Unmarshal(msg1, &req1); err != nil {
		t.Fatalf("Failed to parse message 1: %v", err)
	}
	if req1["id"].(float64) != 1 {
		t.Errorf("Expected id 1, got %v", req1["id"])
	}

	msg2, err := reader.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage() error: %v", err)
	}

	var req2 map[string]interface{}
	if err := json.Unmarshal(msg2, &req2); err != nil {
		t.Fatalf("Failed to parse message 2: %v", err)
	}
	if req2["id"].(float64) != 2 {
		t.Errorf("Expected id 2, got %v", req2["id"])
	}

	// Third read should return EOF
	_, err = reader.ReadMessage()
	if err != io.EOF {
		t.Errorf("Expected io.EOF, got %v", err)
	}
}

func TestReaderSkipsEmptyLines(t *testing.T) {
	input := `
{"jsonrpc":"2.0","id":1}

{"jsonrpc":"2.0","id":2}
`
	reader := NewReader(strings.NewReader(input))

	msg1, err := reader.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage() error: %v", err)
	}
	var req1 map[string]interface{}
	json.Unmarshal(msg1, &req1)
	if req1["id"].(float64) != 1 {
		t.Errorf("Expected id 1, got %v", req1["id"])
	}

	msg2, err := reader.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage() error: %v", err)
	}
	var req2 map[string]interface{}
	json.Unmarshal(msg2, &req2)
	if req2["id"].(float64) != 2 {
		t.Errorf("Expected id 2, got %v", req2["id"])
	}
}

func TestReaderInvalidJSON(t *testing.T) {
	input := "not valid json\n"
	reader := NewReader(strings.NewReader(input))

	_, err := reader.ReadMessage()
	if err == nil {
		t.Fatal("Expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "invalid JSON") {
		t.Errorf("Expected 'invalid JSON' error, got: %v", err)
	}
}

func TestWriterBasic(t *testing.T) {
	buf := &bytes.Buffer{}
	writer := NewWriter(buf)

	data := []byte(`{"jsonrpc":"2.0","result":"ok","id":1}`)
	if err := writer.Write(data); err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	expected := string(data) + "\n"
	if got := buf.String(); got != expected {
		t.Errorf("Write() output = %q, want %q", got, expected)
	}
}

func TestWriterMultiple(t *testing.T) {
	buf := &bytes.Buffer{}
	writer := NewWriter(buf)

	msg1 := []byte(`{"id":1}`)
	msg2 := []byte(`{"id":2}`)

	writer.Write(msg1)
	writer.Write(msg2)

	expected := string(msg1) + "\n" + string(msg2) + "\n"
	if got := buf.String(); got != expected {
		t.Errorf("Output = %q, want %q", got, expected)
	}
}

func TestServerSessionInfo(t *testing.T) {
	sessionMgr := newTestSessionManager()
	agentCfg := config.AgentConfig{
		ID:           "custom-agent-id",
		Name:         "Custom Agent",
		Capabilities: []string{"tools", "resources"},
	}

	stdin := strings.NewReader("")
	stdout := &bytes.Buffer{}

	server := NewServerWithIO(agentCfg, sessionMgr, stdin, stdout)

	ctx := context.Background()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Verify session was created with correct info
	if server.session == nil {
		t.Fatal("Session not created")
	}

	if server.session.AgentID != "custom-agent-id" {
		t.Errorf("AgentID = %q, want %q", server.session.AgentID, "custom-agent-id")
	}

	if server.session.AgentName != "Custom Agent" {
		t.Errorf("AgentName = %q, want %q", server.session.AgentName, "Custom Agent")
	}

	if len(server.session.Capabilities) != 2 {
		t.Errorf("Capabilities length = %d, want 2", len(server.session.Capabilities))
	}

	if server.session.SourceIP != "stdio" {
		t.Errorf("SourceIP = %q, want %q", server.session.SourceIP, "stdio")
	}

	stopCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	server.Stop(stopCtx)
}
