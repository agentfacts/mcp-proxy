package stdio

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/agentfacts/mcp-proxy/internal/config"
	"github.com/agentfacts/mcp-proxy/internal/session"
	"github.com/agentfacts/mcp-proxy/internal/transport"
	"github.com/rs/zerolog/log"
)

// MessageHandler is an alias for the transport.MessageHandler type.
type MessageHandler = transport.MessageHandler

// Server implements the stdio transport for MCP.
// It reads JSON-RPC messages from stdin and writes responses to stdout.
type Server struct {
	agentCfg       config.AgentConfig
	sessionManager *session.Manager
	messageHandler MessageHandler
	session        *session.Session // Single session for stdio

	// I/O streams (configurable for testing)
	stdin  io.Reader
	stdout io.Writer

	// Lifecycle
	mu      sync.RWMutex
	started bool
	done    chan struct{}
	wg      sync.WaitGroup
}

// NewServer creates a new stdio transport server.
func NewServer(agentCfg config.AgentConfig, sessionMgr *session.Manager) *Server {
	return &Server{
		agentCfg:       agentCfg,
		sessionManager: sessionMgr,
		stdin:          os.Stdin,
		stdout:         os.Stdout,
		done:           make(chan struct{}),
	}
}

// NewServerWithIO creates a new stdio transport server with custom I/O streams.
// This is primarily useful for testing.
func NewServerWithIO(agentCfg config.AgentConfig, sessionMgr *session.Manager, stdin io.Reader, stdout io.Writer) *Server {
	return &Server{
		agentCfg:       agentCfg,
		sessionManager: sessionMgr,
		stdin:          stdin,
		stdout:         stdout,
		done:           make(chan struct{}),
	}
}

// SetMessageHandler sets the callback for processing incoming messages.
func (s *Server) SetMessageHandler(h MessageHandler) {
	s.messageHandler = h
}

// Start begins reading from stdin and processing messages.
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return fmt.Errorf("server already started")
	}
	s.started = true
	s.mu.Unlock()

	// Create a single session for the entire process lifetime
	sess, err := s.sessionManager.Create(ctx)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	s.session = sess

	// Set default agent info from config
	s.session.SetAgent(s.agentCfg.ID, s.agentCfg.Name, s.agentCfg.Capabilities)
	s.session.SetClientInfo("stdio", "stdio-client")

	log.Info().
		Str("session_id", s.session.ID).
		Str("transport", "stdio").
		Msg("Stdio server started")

	// Start the read loop in a goroutine
	s.wg.Add(1)
	go s.readLoop(ctx)

	return nil
}

// Stop gracefully shuts down the server.
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return nil
	}
	s.started = false
	s.mu.Unlock()

	// Signal shutdown
	close(s.done)

	// Close session
	if s.session != nil {
		s.session.Close()
		s.sessionManager.Delete(s.session.ID)
	}

	// Wait for read loop to finish with timeout
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Info().Msg("Stdio server stopped")
	case <-ctx.Done():
		log.Warn().Msg("Stdio server stop timed out")
	}

	return nil
}

// Name returns the transport name.
func (s *Server) Name() string {
	return "stdio"
}

// readLoop continuously reads messages from stdin and processes them.
func (s *Server) readLoop(ctx context.Context) {
	defer s.wg.Done()

	reader := NewReader(s.stdin)
	writer := NewWriter(s.stdout)

	for {
		select {
		case <-s.done:
			return
		case <-ctx.Done():
			return
		default:
		}

		// Read next message
		msg, err := reader.ReadMessage()
		if err != nil {
			if err == io.EOF {
				log.Info().Msg("Stdin closed (EOF), shutting down")
				return
			}
			log.Error().Err(err).Msg("Error reading message")
			s.writeError(writer, nil, -32700, "Parse error")
			continue
		}

		// Increment request count
		s.session.IncrementRequestCount()

		log.Debug().
			Str("session_id", s.session.ID).
			Int("body_size", len(msg)).
			Int("request_count", s.session.GetRequestCount()).
			Msg("Received MCP message")

		// Process message through handler
		var response []byte
		if s.messageHandler != nil {
			response, err = s.messageHandler(ctx, s.session, msg)
			if err != nil {
				log.Error().Err(err).Str("session_id", s.session.ID).Msg("Message handler error")
				// Try to extract request ID for error response
				id := extractRequestID(msg)
				s.writeError(writer, id, -32603, "Internal error")
				continue
			}
		} else {
			// No handler configured - echo back for testing
			response = msg
		}

		// Write response
		if response != nil {
			if err := writer.Write(response); err != nil {
				log.Error().Err(err).Msg("Error writing response")
			}
		}
	}
}

// writeError writes a JSON-RPC error response to stdout.
func (s *Server) writeError(writer *Writer, id interface{}, code int, message string) {
	errResp := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
		},
	}

	data, err := json.Marshal(errResp)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal error response")
		return
	}

	if err := writer.Write(data); err != nil {
		log.Error().Err(err).Msg("Failed to write error response")
	}
}

// extractRequestID attempts to extract the request ID from a JSON-RPC message.
func extractRequestID(msg []byte) interface{} {
	var req struct {
		ID interface{} `json:"id"`
	}
	if err := json.Unmarshal(msg, &req); err != nil {
		return nil
	}
	return req.ID
}
