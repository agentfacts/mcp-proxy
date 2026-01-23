package sse

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/agentfacts/mcp-proxy/internal/config"
	"github.com/agentfacts/mcp-proxy/internal/session"
	"github.com/agentfacts/mcp-proxy/internal/transport"
	"github.com/rs/zerolog/log"
)

// MessageHandler is an alias for the transport.MessageHandler type.
type MessageHandler = transport.MessageHandler

// Handler handles SSE connections and messages.
type Handler struct {
	sessionManager *session.Manager
	agentCfg       config.AgentConfig
	securityCfg    config.SecurityConfig
	messageHandler MessageHandler
}

// NewHandler creates a new SSE handler with default security settings.
func NewHandler(sessionMgr *session.Manager, agentCfg config.AgentConfig) *Handler {
	return &Handler{
		sessionManager: sessionMgr,
		agentCfg:       agentCfg,
		securityCfg: config.SecurityConfig{
			EnableSecurityHeaders: true,
			CORSAllowedOrigins:    []string{}, // Empty = same-origin only (secure default)
		},
	}
}

// NewHandlerWithSecurity creates a new SSE handler with custom security configuration.
func NewHandlerWithSecurity(sessionMgr *session.Manager, agentCfg config.AgentConfig, securityCfg config.SecurityConfig) *Handler {
	return &Handler{
		sessionManager: sessionMgr,
		agentCfg:       agentCfg,
		securityCfg:    securityCfg,
	}
}

// setSecurityHeaders adds security headers to the response.
func (h *Handler) setSecurityHeaders(w http.ResponseWriter) {
	if !h.securityCfg.EnableSecurityHeaders {
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
}

// setCORSHeaders sets CORS headers based on configuration.
func (h *Handler) setCORSHeaders(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")

	// If no allowed origins configured, only allow same-origin (no CORS header)
	if len(h.securityCfg.CORSAllowedOrigins) == 0 {
		return
	}

	// Check if wildcard is allowed
	for _, allowed := range h.securityCfg.CORSAllowedOrigins {
		if allowed == "*" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			return
		}
	}

	// Check if the origin is in the allowed list
	for _, allowed := range h.securityCfg.CORSAllowedOrigins {
		if strings.EqualFold(origin, allowed) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			return
		}
	}
}

// SetMessageHandler sets the callback for processing messages.
func (h *Handler) SetMessageHandler(handler MessageHandler) {
	h.messageHandler = handler
}

// HandleSSE handles the SSE stream connection (GET /).
func (h *Handler) HandleSSE(w http.ResponseWriter, r *http.Request) {
	// Check if client supports SSE
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Create new session
	sess, err := h.sessionManager.Create(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("Failed to create session")
		http.Error(w, "Failed to create session", http.StatusServiceUnavailable)
		return
	}

	// Set default agent info from config
	sess.SetAgent(h.agentCfg.ID, h.agentCfg.Name, h.agentCfg.Capabilities)

	// Set client info
	sess.SetClientInfo(r.RemoteAddr, r.UserAgent())

	log.Info().
		Str("session_id", sess.ID).
		Str("remote_addr", r.RemoteAddr).
		Msg("SSE connection established")

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	// Set security headers
	h.setSecurityHeaders(w)
	h.setCORSHeaders(w, r)

	// Send endpoint event with message URL
	messageURL := fmt.Sprintf("/message?sessionId=%s", sess.ID)
	h.sendEvent(w, flusher, "endpoint", messageURL)

	// Create done channel for cleanup
	clientGone := r.Context().Done()

	// Heartbeat ticker
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	// Main event loop
	for {
		select {
		case <-clientGone:
			// Client disconnected
			log.Info().
				Str("session_id", sess.ID).
				Int("request_count", sess.GetRequestCount()).
				Msg("SSE client disconnected")
			h.sessionManager.Delete(sess.ID)
			return

		case <-sess.Done:
			// Session was closed externally
			log.Debug().Str("session_id", sess.ID).Msg("Session closed")
			return

		case msg := <-sess.MessageChan:
			// Send message to client
			h.sendEvent(w, flusher, "message", string(msg))

		case <-heartbeat.C:
			// Send heartbeat to keep connection alive
			h.sendEvent(w, flusher, "ping", "")
		}
	}
}

// HandleMessage handles incoming MCP messages (POST /message).
func (h *Handler) HandleMessage(w http.ResponseWriter, r *http.Request) {
	// Get session ID from query parameter
	sessionID := r.URL.Query().Get("sessionId")
	if sessionID == "" {
		h.sendError(w, http.StatusBadRequest, -32600, "Missing sessionId parameter")
		return
	}

	// Get session
	sess, ok := h.sessionManager.Get(sessionID)
	if !ok {
		h.sendError(w, http.StatusNotFound, -32600, "Session not found or expired")
		return
	}

	// Read request body
	body, err := io.ReadAll(io.LimitReader(r.Body, 1*1024*1024)) // 1MB limit
	if err != nil {
		h.sendError(w, http.StatusBadRequest, -32700, "Failed to read request body")
		return
	}
	defer r.Body.Close()

	// Validate JSON
	if !json.Valid(body) {
		h.sendError(w, http.StatusBadRequest, -32700, "Invalid JSON")
		return
	}

	// Increment request count
	sess.IncrementRequestCount()

	log.Debug().
		Str("session_id", sessionID).
		Int("body_size", len(body)).
		Int("request_count", sess.GetRequestCount()).
		Msg("Received MCP message")

	// Process message through handler
	var response []byte
	if h.messageHandler != nil {
		response, err = h.messageHandler(r.Context(), sess, body)
		if err != nil {
			// Log full error internally but return sanitized message to client
			log.Error().Err(err).Str("session_id", sessionID).Msg("Message handler error")
			h.sendError(w, http.StatusInternalServerError, -32603, "Internal server error")
			return
		}
	} else {
		// No handler configured - echo back for testing
		response = body
	}

	// Send response via SSE stream
	if response != nil {
		if !sess.SendMessage(response) {
			log.Warn().Str("session_id", sessionID).Msg("Failed to send response - session closed or buffer full")
		}
	}

	// Return 202 Accepted (response will be sent via SSE)
	w.WriteHeader(http.StatusAccepted)
}

// sendEvent sends an SSE event to the client.
func (h *Handler) sendEvent(w http.ResponseWriter, flusher http.Flusher, event, data string) {
	if event != "" {
		fmt.Fprintf(w, "event: %s\n", event)
	}
	if data != "" {
		fmt.Fprintf(w, "data: %s\n", data)
	}
	fmt.Fprintf(w, "\n")
	flusher.Flush()
}

// sendError sends a JSON-RPC error response with security headers.
func (h *Handler) sendError(w http.ResponseWriter, httpStatus int, code int, message string) {
	h.setSecurityHeaders(w)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)

	response := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      nil,
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
		},
	}

	json.NewEncoder(w).Encode(response)
}
