package router

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	json "github.com/goccy/go-json"
)

// randBytePool provides reusable byte slices for random generation.
var randBytePool = sync.Pool{
	New: func() interface{} {
		// Most common case: 8 hex chars = 4 bytes
		b := make([]byte, 8)
		return &b
	},
}

// requestContextPool provides reusable RequestContext objects.
var requestContextPool = sync.Pool{
	New: func() interface{} {
		return &RequestContext{}
	},
}

// requestPool provides reusable Request objects.
var requestPool = sync.Pool{
	New: func() interface{} {
		return &Request{}
	},
}

// GetRequest retrieves a Request from the pool.
func GetRequest() *Request {
	req := requestPool.Get().(*Request)
	req.JSONRPC = ""
	req.ID = nil
	req.Method = ""
	req.Params = nil
	return req
}

// PutRequest returns a Request to the pool.
func PutRequest(req *Request) {
	req.ID = nil
	req.Params = nil
	requestPool.Put(req)
}

// JSON-RPC 2.0 Error Codes
const (
	// Standard JSON-RPC errors
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603

	// MCP-specific errors
	CodePolicyViolation = -32001
	CodeIdentityError   = -32002
	CodeRateLimited     = -32003
	CodeUpstreamError   = -32004
)

// Request represents a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"` // Can be string, number, or null
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response represents a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *Error      `json:"error,omitempty"`
}

// Error represents a JSON-RPC 2.0 error object.
type Error struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// PolicyViolationData contains details about a policy violation.
type PolicyViolationData struct {
	RequestID          string   `json:"request_id"`
	AgentID            string   `json:"agent_id"`
	Tool               string   `json:"tool,omitempty"`
	RequiredCapability string   `json:"required_capability,omitempty"`
	AgentCapabilities  []string `json:"agent_capabilities,omitempty"`
	Violations         []string `json:"violations"`
	PolicyMode         string   `json:"policy_mode"`
	Timestamp          string   `json:"timestamp"`
}

// ToolCallParams represents parameters for tools/call method.
type ToolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
	Meta      *MetaParams            `json:"_meta,omitempty"`
}

// ResourceReadParams represents parameters for resources/read method.
type ResourceReadParams struct {
	URI  string      `json:"uri"`
	Meta *MetaParams `json:"_meta,omitempty"`
}

// MetaParams contains metadata fields like AgentFacts token.
type MetaParams struct {
	AgentFacts string `json:"agentfacts,omitempty"`
}

// HandlerType defines how a method should be handled.
type HandlerType int

const (
	// HandlerPassthrough - Forward without policy check (ping, initialize)
	HandlerPassthrough HandlerType = iota
	// HandlerFullEnforce - Full policy enforcement (tools/call, resources/read)
	HandlerFullEnforce
	// HandlerFilter - Filter results based on policy (tools/list, resources/list)
	HandlerFilter
)

// MethodConfig defines how to handle a specific MCP method.
type MethodConfig struct {
	Handler     HandlerType
	LogLevel    LogLevel
	Description string
}

// LogLevel defines the logging detail for a method.
type LogLevel int

const (
	LogNone     LogLevel = iota // Don't log
	LogMetadata                 // Log metadata only
	LogFull                     // Log full request and response
)

// MethodRegistry maps MCP methods to their handling configuration.
var MethodRegistry = map[string]MethodConfig{
	// Tool methods
	"tools/call": {
		Handler:     HandlerFullEnforce,
		LogLevel:    LogFull,
		Description: "Invoke a tool",
	},
	"tools/list": {
		Handler:     HandlerFilter,
		LogLevel:    LogMetadata,
		Description: "List available tools",
	},

	// Resource methods
	"resources/read": {
		Handler:     HandlerFullEnforce,
		LogLevel:    LogFull,
		Description: "Read a resource",
	},
	"resources/list": {
		Handler:     HandlerFilter,
		LogLevel:    LogMetadata,
		Description: "List available resources",
	},
	"resources/subscribe": {
		Handler:     HandlerFullEnforce,
		LogLevel:    LogFull,
		Description: "Subscribe to resource updates",
	},

	// Prompt methods
	"prompts/get": {
		Handler:     HandlerPassthrough,
		LogLevel:    LogMetadata,
		Description: "Get a prompt template",
	},
	"prompts/list": {
		Handler:     HandlerPassthrough,
		LogLevel:    LogMetadata,
		Description: "List available prompts",
	},

	// Lifecycle methods
	"initialize": {
		Handler:     HandlerPassthrough,
		LogLevel:    LogMetadata,
		Description: "Initialize MCP session",
	},
	"ping": {
		Handler:     HandlerPassthrough,
		LogLevel:    LogNone,
		Description: "Health check ping",
	},

	// Notification methods (client-initiated, no response expected)
	"notifications/initialized": {
		Handler:     HandlerPassthrough,
		LogLevel:    LogNone,
		Description: "Client initialization complete",
	},
	"notifications/cancelled": {
		Handler:     HandlerPassthrough,
		LogLevel:    LogMetadata,
		Description: "Request cancelled",
	},
}

// RequestContext holds parsed information about a request for policy evaluation.
type RequestContext struct {
	// Original request
	Request *Request

	// Parsed request details
	RequestID   string
	Method      string
	Tool        string // For tools/call
	ResourceURI string // For resources/read
	Arguments   map[string]interface{}

	// Handler configuration
	Config MethodConfig

	// Timing
	ReceivedAt time.Time

	// AgentFacts token if present
	AgentFactsToken string
}

// NewRequestContext creates a RequestContext from a parsed request.
// Uses object pooling to reduce allocations.
// Calls time.Now() internally - use NewRequestContextAt for hot paths where time is already known.
func NewRequestContext(req *Request) *RequestContext {
	return NewRequestContextAt(req, time.Now())
}

// NewRequestContextAt creates a RequestContext with a provided timestamp.
// Use this in hot paths where time.Now() has already been called.
func NewRequestContextAt(req *Request, receivedAt time.Time) *RequestContext {
	ctx := requestContextPool.Get().(*RequestContext)

	// Initialize fields
	ctx.Request = req
	ctx.RequestID = generateRequestID()
	ctx.Method = req.Method
	ctx.ReceivedAt = receivedAt
	ctx.Tool = ""
	ctx.ResourceURI = ""
	ctx.Arguments = nil
	ctx.AgentFactsToken = ""

	// Get method configuration
	if cfg, ok := MethodRegistry[req.Method]; ok {
		ctx.Config = cfg
	} else {
		// Unknown method - default to passthrough
		ctx.Config = MethodConfig{
			Handler:     HandlerPassthrough,
			LogLevel:    LogMetadata,
			Description: "Unknown method",
		}
	}

	return ctx
}

// Release returns the RequestContext to the pool for reuse.
// Also releases the underlying Request if present.
// Call this when done with the context to reduce allocations.
func (ctx *RequestContext) Release() {
	// Release the underlying request if present
	if ctx.Request != nil {
		PutRequest(ctx.Request)
	}
	// Clear references to help GC
	ctx.Request = nil
	ctx.Arguments = nil
	requestContextPool.Put(ctx)
}

// generateRequestID creates a unique request identifier.
func generateRequestID() string {
	return "req_" + randomHex(8)
}

// randomHex generates a cryptographically secure random hex string.
// Uses crypto/rand for unpredictable values suitable for security contexts.
// Optimized with buffer pooling to reduce allocations.
func randomHex(n int) string {
	// We need n/2 bytes since each byte produces 2 hex characters
	byteLen := (n + 1) / 2

	// Use pooled buffer for common case (8 hex = 4 bytes)
	if byteLen <= 8 {
		bp := randBytePool.Get().(*[]byte)
		b := (*bp)[:byteLen]
		if _, err := rand.Read(b); err != nil {
			randBytePool.Put(bp)
			panic("crypto/rand failed: " + err.Error())
		}
		result := hex.EncodeToString(b)[:n]
		randBytePool.Put(bp)
		return result
	}

	// Fallback for larger sizes (rare)
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)[:n]
}
