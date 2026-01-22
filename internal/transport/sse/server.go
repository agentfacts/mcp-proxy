package sse

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/agentfacts/mcp-proxy/internal/config"
	"github.com/agentfacts/mcp-proxy/internal/session"
)

// Server implements the SSE transport for MCP.
type Server struct {
	cfg            config.ServerConfig
	agentCfg       config.AgentConfig
	sessionManager *session.Manager
	httpServer     *http.Server
	handler        *Handler

	// Lifecycle
	mu      sync.RWMutex
	started bool
	done    chan struct{}
}

// NewServer creates a new SSE transport server.
func NewServer(cfg config.ServerConfig, agentCfg config.AgentConfig, sessionMgr *session.Manager) *Server {
	s := &Server{
		cfg:            cfg,
		agentCfg:       agentCfg,
		sessionManager: sessionMgr,
		done:           make(chan struct{}),
	}

	// Create the handler
	s.handler = NewHandler(s.sessionManager, agentCfg)

	return s
}

// SetMessageHandler sets the callback for processing incoming messages.
func (s *Server) SetMessageHandler(h MessageHandler) {
	s.handler.SetMessageHandler(h)
}

// Start begins accepting SSE connections.
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return fmt.Errorf("server already started")
	}
	s.started = true
	s.mu.Unlock()

	// Create HTTP mux
	mux := http.NewServeMux()

	// SSE stream endpoint - establishes the event stream
	mux.HandleFunc("GET /", s.handler.HandleSSE)

	// Message endpoint - receives MCP messages
	mux.HandleFunc("POST /message", s.handler.HandleMessage)

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", s.cfg.Listen.Address, s.cfg.Listen.Port)
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      s.requestLogger(mux),
		ReadTimeout:  s.cfg.ReadTimeout,
		WriteTimeout: s.cfg.WriteTimeout,
		IdleTimeout:  s.cfg.IdleTimeout,
		BaseContext: func(l net.Listener) context.Context {
			return ctx
		},
	}

	// Start listening
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	log.Info().
		Str("address", addr).
		Str("transport", "sse").
		Msg("SSE server listening")

	// Start serving in goroutine
	go func() {
		if err := s.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("SSE server error")
		}
	}()

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

	close(s.done)

	log.Info().Msg("Shutting down SSE server...")

	// Gracefully shutdown HTTP server
	if s.httpServer != nil {
		if err := s.httpServer.Shutdown(ctx); err != nil {
			return fmt.Errorf("server shutdown error: %w", err)
		}
	}

	log.Info().Msg("SSE server stopped")
	return nil
}

// Name returns the transport name.
func (s *Server) Name() string {
	return "sse"
}

// requestLogger is middleware that logs HTTP requests.
func (s *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		log.Debug().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Str("remote_addr", r.RemoteAddr).
			Int("status", wrapped.statusCode).
			Dur("duration", time.Since(start)).
			Msg("HTTP request")
	})
}

// responseWriter wraps http.ResponseWriter to capture status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Flush implements http.Flusher for SSE support.
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
