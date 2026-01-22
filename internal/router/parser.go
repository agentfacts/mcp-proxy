package router

import (
	"fmt"
	"regexp"
	"strings"

	json "github.com/goccy/go-json"
)

// Method name validation constants
const (
	maxMethodLength = 256 // Maximum allowed method name length
)

// methodPattern validates method names: alphanumeric, underscores, forward slashes
// Must start with a letter
var methodPattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_/]*$`)

// Parser handles JSON-RPC message parsing and validation.
type Parser struct{}

// NewParser creates a new message parser.
func NewParser() *Parser {
	return &Parser{}
}

// Parse parses and validates a JSON-RPC 2.0 message.
// Returns a pooled Request that should be released via PutRequest when done.
func (p *Parser) Parse(data []byte) (*Request, error) {
	// Check for empty input
	if len(data) == 0 {
		return nil, &ParseError{
			Code:    CodeParseError,
			Message: "Empty message",
		}
	}

	// Get pooled request
	req := GetRequest()

	// Parse JSON
	if err := json.Unmarshal(data, req); err != nil {
		PutRequest(req)
		return nil, &ParseError{
			Code:    CodeParseError,
			Message: fmt.Sprintf("Invalid JSON: %v", err),
		}
	}

	// Validate JSON-RPC version
	if req.JSONRPC != "2.0" {
		PutRequest(req)
		return nil, &ParseError{
			Code:    CodeInvalidRequest,
			Message: fmt.Sprintf("Invalid JSON-RPC version: expected '2.0', got '%s'", req.JSONRPC),
		}
	}

	// Validate method
	if req.Method == "" {
		PutRequest(req)
		return nil, &ParseError{
			Code:    CodeInvalidRequest,
			Message: "Missing 'method' field",
		}
	}

	// Validate method name length to prevent DoS
	if len(req.Method) > maxMethodLength {
		PutRequest(req)
		return nil, &ParseError{
			Code:    CodeInvalidRequest,
			Message: "Method name too long",
		}
	}

	// Validate method name format to prevent log injection and other attacks
	if !methodPattern.MatchString(req.Method) {
		PutRequest(req)
		return nil, &ParseError{
			Code:    CodeInvalidRequest,
			Message: "Invalid method name format",
		}
	}

	// Method must not start with "rpc." (reserved per JSON-RPC spec)
	if strings.HasPrefix(req.Method, "rpc.") {
		PutRequest(req)
		return nil, &ParseError{
			Code:    CodeInvalidRequest,
			Message: "Method names starting with 'rpc.' are reserved",
		}
	}

	// For requests (not notifications), ID must be present
	// Notifications have no ID field
	// We'll treat missing ID as notification

	return req, nil
}

// ParseToolCall extracts tool call parameters from a request.
func (p *Parser) ParseToolCall(req *Request) (*ToolCallParams, error) {
	if req.Params == nil {
		return nil, &ParseError{
			Code:    CodeInvalidParams,
			Message: "Missing 'params' for tools/call",
		}
	}

	var params ToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil, &ParseError{
			Code:    CodeInvalidParams,
			Message: fmt.Sprintf("Invalid tools/call params: %v", err),
		}
	}

	if params.Name == "" {
		return nil, &ParseError{
			Code:    CodeInvalidParams,
			Message: "Missing 'name' in tools/call params",
		}
	}

	return &params, nil
}

// ParseResourceRead extracts resource read parameters from a request.
func (p *Parser) ParseResourceRead(req *Request) (*ResourceReadParams, error) {
	if req.Params == nil {
		return nil, &ParseError{
			Code:    CodeInvalidParams,
			Message: "Missing 'params' for resources/read",
		}
	}

	var params ResourceReadParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil, &ParseError{
			Code:    CodeInvalidParams,
			Message: fmt.Sprintf("Invalid resources/read params: %v", err),
		}
	}

	if params.URI == "" {
		return nil, &ParseError{
			Code:    CodeInvalidParams,
			Message: "Missing 'uri' in resources/read params",
		}
	}

	return &params, nil
}

// ExtractMeta extracts the _meta field from params if present.
func (p *Parser) ExtractMeta(params json.RawMessage) (*MetaParams, error) {
	if params == nil {
		return nil, nil
	}

	// Parse params to extract _meta
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(params, &raw); err != nil {
		return nil, nil // Params might not be an object
	}

	metaRaw, ok := raw["_meta"]
	if !ok {
		return nil, nil
	}

	var meta MetaParams
	if err := json.Unmarshal(metaRaw, &meta); err != nil {
		return nil, fmt.Errorf("invalid _meta field: %w", err)
	}

	return &meta, nil
}

// IsNotification returns true if the request is a notification (no ID).
func (p *Parser) IsNotification(req *Request) bool {
	return req.ID == nil
}

// IsRequest returns true if the request expects a response (has ID).
func (p *Parser) IsRequest(req *Request) bool {
	return req.ID != nil
}

// ParseError represents a parsing error with JSON-RPC error code.
type ParseError struct {
	Code    int
	Message string
}

func (e *ParseError) Error() string {
	return e.Message
}

// ToResponse converts a ParseError to a JSON-RPC error response.
func (e *ParseError) ToResponse(id interface{}) *Response {
	return &Response{
		JSONRPC: "2.0",
		ID:      id,
		Error: &Error{
			Code:    e.Code,
			Message: e.Message,
		},
	}
}
