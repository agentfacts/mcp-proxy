package router

import (
	"bytes"
	"sync"
	"time"

	json "github.com/goccy/go-json"
)

// bufferPool provides reusable buffers for JSON encoding.
var bufferPool = sync.Pool{
	New: func() interface{} {
		return bytes.NewBuffer(make([]byte, 0, 512))
	},
}

// getBuffer retrieves a buffer from the pool.
func getBuffer() *bytes.Buffer {
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

// putBuffer returns a buffer to the pool.
func putBuffer(buf *bytes.Buffer) {
	// Only return reasonably-sized buffers to avoid memory bloat
	if buf.Cap() <= 4096 {
		bufferPool.Put(buf)
	}
}

// ResponseBuilder helps construct JSON-RPC responses.
type ResponseBuilder struct{}

// NewResponseBuilder creates a new response builder.
func NewResponseBuilder() *ResponseBuilder {
	return &ResponseBuilder{}
}

// Success creates a successful response with the given result.
func (b *ResponseBuilder) Success(id interface{}, result interface{}) *Response {
	return &Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
}

// Error creates an error response.
func (b *ResponseBuilder) Error(id interface{}, code int, message string) *Response {
	return &Response{
		JSONRPC: "2.0",
		ID:      id,
		Error: &Error{
			Code:    code,
			Message: message,
		},
	}
}

// ErrorWithData creates an error response with additional data.
func (b *ResponseBuilder) ErrorWithData(id interface{}, code int, message string, data interface{}) *Response {
	return &Response{
		JSONRPC: "2.0",
		ID:      id,
		Error: &Error{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
}

// ParseError creates a parse error response (-32700).
func (b *ResponseBuilder) ParseError(message string) *Response {
	return b.Error(nil, CodeParseError, message)
}

// InvalidRequest creates an invalid request error response (-32600).
func (b *ResponseBuilder) InvalidRequest(id interface{}, message string) *Response {
	return b.Error(id, CodeInvalidRequest, message)
}

// MethodNotFound creates a method not found error response (-32601).
func (b *ResponseBuilder) MethodNotFound(id interface{}, method string) *Response {
	return b.Error(id, CodeMethodNotFound, "Method not found: "+method)
}

// InvalidParams creates an invalid params error response (-32602).
func (b *ResponseBuilder) InvalidParams(id interface{}, message string) *Response {
	return b.Error(id, CodeInvalidParams, message)
}

// InternalError creates an internal error response (-32603).
func (b *ResponseBuilder) InternalError(id interface{}, message string) *Response {
	return b.Error(id, CodeInternalError, message)
}

// PolicyViolation creates a policy violation error response (-32001).
func (b *ResponseBuilder) PolicyViolation(id interface{}, reqCtx *RequestContext, agentID string, capabilities []string, violations []string, policyMode string) *Response {
	data := PolicyViolationData{
		RequestID:         reqCtx.RequestID,
		AgentID:           agentID,
		Tool:              reqCtx.Tool,
		AgentCapabilities: capabilities,
		Violations:        violations,
		PolicyMode:        policyMode,
		Timestamp:         time.Now().UTC().Format(time.RFC3339Nano),
	}

	message := "Policy violation"
	if len(violations) > 0 {
		message = violations[0] // Use first violation as message
	}

	return b.ErrorWithData(id, CodePolicyViolation, message, data)
}

// IdentityError creates an identity verification error response (-32002).
func (b *ResponseBuilder) IdentityError(id interface{}, errorCode string, message string) *Response {
	data := map[string]string{
		"error_code": errorCode,
	}
	return b.ErrorWithData(id, CodeIdentityError, message, data)
}

// RateLimited creates a rate limit error response (-32003).
func (b *ResponseBuilder) RateLimited(id interface{}, agentID string, limit int, current int) *Response {
	data := map[string]interface{}{
		"agent_id": agentID,
		"limit":    limit,
		"current":  current,
	}
	return b.ErrorWithData(id, CodeRateLimited, "Rate limit exceeded", data)
}

// UpstreamError creates an upstream error response (-32004).
func (b *ResponseBuilder) UpstreamError(id interface{}, message string) *Response {
	return b.Error(id, CodeUpstreamError, message)
}

// FromParseError converts a ParseError to a Response.
func (b *ResponseBuilder) FromParseError(err *ParseError, id interface{}) *Response {
	return b.Error(id, err.Code, err.Message)
}

// Marshal serializes a response to JSON using pooled buffers.
func (b *ResponseBuilder) Marshal(resp *Response) ([]byte, error) {
	buf := getBuffer()
	defer putBuffer(buf)

	enc := json.NewEncoder(buf)
	if err := enc.Encode(resp); err != nil {
		return nil, err
	}

	// Remove trailing newline added by Encoder
	data := buf.Bytes()
	if len(data) > 0 && data[len(data)-1] == '\n' {
		data = data[:len(data)-1]
	}

	// Copy to new slice to avoid returning pooled buffer
	result := make([]byte, len(data))
	copy(result, data)
	return result, nil
}

// MustMarshal serializes a response to JSON, panicking on error.
func (b *ResponseBuilder) MustMarshal(resp *Response) []byte {
	data, err := json.Marshal(resp)
	if err != nil {
		panic(err)
	}
	return data
}
