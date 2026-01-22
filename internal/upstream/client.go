package upstream

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/agentfacts/mcp-proxy/internal/config"
	"github.com/rs/zerolog/log"
)

// Client manages connections to the upstream MCP server.
type Client struct {
	cfg        config.UpstreamConfig
	httpClient *http.Client

	// Connection state
	mu           sync.RWMutex
	connected    bool
	messageURL   string
	sseConn      *http.Response
	responseChan chan *Response

	// Pending requests waiting for responses
	pending   map[interface{}]chan *Response
	pendingMu sync.RWMutex

	// Lifecycle
	done   chan struct{}
	ctx    context.Context
	cancel context.CancelFunc
}

// Response represents a response from the upstream server.
type Response struct {
	SessionID string
	Data      []byte
	Error     error
}

// NewClient creates a new upstream client.
func NewClient(cfg config.UpstreamConfig) *Client {
	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
			Transport: &http.Transport{
				MaxIdleConns:        cfg.ConnectionPool.MaxIdle,
				MaxIdleConnsPerHost: cfg.ConnectionPool.MaxIdle,
				IdleConnTimeout:     cfg.ConnectionPool.IdleTimeout,
			},
		},
		pending:      make(map[interface{}]chan *Response),
		responseChan: make(chan *Response, 100),
		done:         make(chan struct{}),
	}
}

// Connect establishes an SSE connection to the upstream server.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	if c.connected {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	c.ctx, c.cancel = context.WithCancel(ctx)

	// Create SSE request
	req, err := http.NewRequestWithContext(c.ctx, "GET", c.cfg.URL, nil)
	if err != nil {
		return fmt.Errorf("failed to create SSE request: %w", err)
	}

	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	log.Info().Str("url", c.cfg.URL).Msg("Connecting to upstream MCP server")

	// Establish SSE connection
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to upstream: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	c.mu.Lock()
	c.sseConn = resp
	c.connected = true
	c.mu.Unlock()

	// Start reading SSE events
	go c.readEvents()

	log.Info().Str("url", c.cfg.URL).Msg("Connected to upstream MCP server")

	return nil
}

// Disconnect closes the upstream connection.
func (c *Client) Disconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return
	}

	c.connected = false
	close(c.done)

	if c.cancel != nil {
		c.cancel()
	}

	if c.sseConn != nil {
		c.sseConn.Body.Close()
		c.sseConn = nil
	}

	log.Info().Msg("Disconnected from upstream MCP server")
}

// Send sends a message to the upstream server and waits for a response.
func (c *Client) Send(ctx context.Context, message []byte) ([]byte, error) {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return nil, fmt.Errorf("not connected to upstream")
	}
	messageURL := c.messageURL
	c.mu.RUnlock()

	if messageURL == "" {
		return nil, fmt.Errorf("upstream message URL not yet received")
	}

	// Extract request ID for response matching
	var parsed map[string]interface{}
	if err := json.Unmarshal(message, &parsed); err != nil {
		return nil, fmt.Errorf("invalid JSON message: %w", err)
	}
	requestID := parsed["id"]

	// Create response channel for this request
	respChan := make(chan *Response, 1)
	c.pendingMu.Lock()
	c.pending[requestID] = respChan
	c.pendingMu.Unlock()

	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, requestID)
		c.pendingMu.Unlock()
	}()

	// Send message to upstream
	req, err := http.NewRequestWithContext(ctx, "POST", messageURL, bytes.NewReader(message))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send to upstream: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	// Wait for response via SSE
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case response := <-respChan:
		if response.Error != nil {
			return nil, response.Error
		}
		return response.Data, nil
	case <-time.After(c.cfg.Timeout):
		return nil, fmt.Errorf("timeout waiting for upstream response")
	}
}

// SendAsync sends a message without waiting for a response.
func (c *Client) SendAsync(ctx context.Context, message []byte) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("not connected to upstream")
	}
	messageURL := c.messageURL
	c.mu.RUnlock()

	if messageURL == "" {
		return fmt.Errorf("upstream message URL not yet received")
	}

	req, err := http.NewRequestWithContext(ctx, "POST", messageURL, bytes.NewReader(message))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send to upstream: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	return nil
}

// readEvents reads SSE events from the upstream connection.
func (c *Client) readEvents() {
	c.mu.RLock()
	conn := c.sseConn
	c.mu.RUnlock()

	if conn == nil {
		return
	}

	reader := bufio.NewReader(conn.Body)
	var event, data string

	for {
		select {
		case <-c.done:
			return
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				log.Error().Err(err).Msg("Error reading from upstream SSE")
			}
			c.handleDisconnect()
			return
		}

		line = strings.TrimSpace(line)

		// Empty line marks end of event
		if line == "" {
			if event != "" || data != "" {
				c.handleEvent(event, data)
				event = ""
				data = ""
			}
			continue
		}

		// Parse SSE fields
		if strings.HasPrefix(line, "event:") {
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			data = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		}
	}
}

// handleEvent processes a received SSE event.
func (c *Client) handleEvent(event, data string) {
	switch event {
	case "endpoint":
		// Store the message URL
		c.mu.Lock()
		// Convert relative URL to absolute
		if strings.HasPrefix(data, "/") {
			// Parse base URL and append path
			c.messageURL = strings.TrimSuffix(c.cfg.URL, "/") + data
		} else {
			c.messageURL = data
		}
		c.mu.Unlock()
		log.Debug().Str("message_url", c.messageURL).Msg("Received upstream message endpoint")

	case "message":
		// Parse response to find matching request
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(data), &parsed); err != nil {
			log.Warn().Err(err).Msg("Failed to parse upstream message")
			return
		}

		requestID := parsed["id"]
		c.pendingMu.RLock()
		respChan, ok := c.pending[requestID]
		c.pendingMu.RUnlock()

		if ok {
			select {
			case respChan <- &Response{Data: []byte(data)}:
			default:
				log.Warn().Interface("id", requestID).Msg("Response channel full")
			}
		} else {
			log.Debug().Interface("id", requestID).Msg("Received response for unknown request")
		}

	case "ping":
		// Heartbeat, ignore
		log.Trace().Msg("Received upstream ping")

	default:
		log.Debug().Str("event", event).Msg("Received unknown SSE event type")
	}
}

// handleDisconnect handles upstream disconnection.
func (c *Client) handleDisconnect() {
	c.mu.Lock()
	wasConnected := c.connected
	c.connected = false
	c.mu.Unlock()

	if wasConnected {
		log.Warn().Msg("Upstream connection lost")

		// Fail all pending requests
		c.pendingMu.Lock()
		for id, ch := range c.pending {
			select {
			case ch <- &Response{Error: fmt.Errorf("upstream disconnected")}:
			default:
			}
			delete(c.pending, id)
		}
		c.pendingMu.Unlock()

		// TODO: Implement reconnection logic with backoff
	}
}

// IsConnected returns true if connected to upstream.
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// GetMessageURL returns the upstream message URL.
func (c *Client) GetMessageURL() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.messageURL
}
