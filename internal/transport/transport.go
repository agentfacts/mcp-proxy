package transport

import (
	"context"
)

// Transport defines the interface for MCP transport implementations.
// Supported transports: SSE, stdio, HTTP
type Transport interface {
	// Start begins accepting connections and processing messages.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the transport, closing all connections.
	Stop(ctx context.Context) error

	// Name returns the transport type name (e.g., "sse", "stdio", "http")
	Name() string
}

// MessageHandler is called when a message is received from a client.
// It should process the message and return a response.
type MessageHandler func(ctx context.Context, sessionID string, message []byte) ([]byte, error)

// ConnectionHandler is called when a new connection is established or closed.
type ConnectionHandler interface {
	OnConnect(ctx context.Context, sessionID string)
	OnDisconnect(ctx context.Context, sessionID string)
}

// TransportConfig holds common transport configuration.
type TransportConfig struct {
	Address        string
	Port           int
	ReadTimeout    int // seconds
	WriteTimeout   int // seconds
	MaxConnections int
}
