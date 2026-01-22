package router

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/agentfacts/mcp-proxy/internal/session"
)

// TestNewRouter tests router creation.
func TestNewRouter(t *testing.T) {
	r := NewRouter()
	if r == nil {
		t.Fatal("NewRouter() returned nil")
	}
	if r.parser == nil {
		t.Error("Router parser is nil")
	}
	if r.response == nil {
		t.Error("Router response builder is nil")
	}
}

// TestRouterCallbacks tests setting router callbacks.
func TestRouterCallbacks(t *testing.T) {
	r := NewRouter()

	policyEvalCalled := false
	upstreamCalled := false
	auditCalled := false

	r.SetPolicyEvaluator(func(ctx context.Context, sess *session.Session, reqCtx *RequestContext) (*PolicyDecision, error) {
		policyEvalCalled = true
		return &PolicyDecision{Allow: true, PolicyMode: "test"}, nil
	})

	r.SetUpstreamSender(func(ctx context.Context, message []byte) ([]byte, error) {
		upstreamCalled = true
		return message, nil
	})

	r.SetAuditLogger(func(ctx context.Context, sess *session.Session, reqCtx *RequestContext, decision *PolicyDecision, response []byte, latency time.Duration) {
		auditCalled = true
	})

	// Use initialize which has LogMetadata (not LogNone like ping)
	req := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`
	sess := session.NewSession("test_sess")

	_, err := r.Route(context.Background(), sess, []byte(req))
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}

	// Initialize is passthrough, so only upstream and audit should be called
	if policyEvalCalled {
		t.Error("Policy evaluator was called for passthrough method")
	}
	if !upstreamCalled {
		t.Error("Upstream sender was not called")
	}
	if !auditCalled {
		t.Error("Audit logger was not called")
	}
}

// TestParseValidRequest tests parsing valid JSON-RPC requests.
func TestParseValidRequest(t *testing.T) {
	tests := []struct {
		name    string
		message string
		wantErr bool
	}{
		{
			name:    "valid ping",
			message: `{"jsonrpc":"2.0","id":1,"method":"ping"}`,
			wantErr: false,
		},
		{
			name:    "valid tools/call",
			message: `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"test_tool"}}`,
			wantErr: false,
		},
		{
			name:    "valid initialize",
			message: `{"jsonrpc":"2.0","id":3,"method":"initialize","params":{}}`,
			wantErr: false,
		},
		{
			name:    "notification (no id)",
			message: `{"jsonrpc":"2.0","method":"notifications/initialized"}`,
			wantErr: false,
		},
	}

	r := NewRouter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := r.parser.Parse([]byte(tt.message))
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && req == nil {
				t.Error("Parse() returned nil request without error")
			}
		})
	}
}

// TestParseMalformedMessages tests error handling for malformed JSON-RPC messages.
func TestParseMalformedMessages(t *testing.T) {
	tests := []struct {
		name        string
		message     string
		expectedErr int // JSON-RPC error code
	}{
		{
			name:        "empty message",
			message:     "",
			expectedErr: CodeParseError,
		},
		{
			name:        "invalid json",
			message:     `{"jsonrpc":"2.0"`,
			expectedErr: CodeParseError,
		},
		{
			name:        "wrong jsonrpc version",
			message:     `{"jsonrpc":"1.0","id":1,"method":"test"}`,
			expectedErr: CodeInvalidRequest,
		},
		{
			name:        "missing method",
			message:     `{"jsonrpc":"2.0","id":1}`,
			expectedErr: CodeInvalidRequest,
		},
		{
			name:        "reserved method name",
			message:     `{"jsonrpc":"2.0","id":1,"method":"rpc.test"}`,
			expectedErr: CodeInvalidRequest,
		},
	}

	r := NewRouter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := r.parser.Parse([]byte(tt.message))
			if err == nil {
				t.Error("Parse() should have returned error")
				return
			}

			parseErr, ok := err.(*ParseError)
			if !ok {
				t.Errorf("Error type = %T, want *ParseError", err)
				return
			}

			if parseErr.Code != tt.expectedErr {
				t.Errorf("Error code = %d, want %d", parseErr.Code, tt.expectedErr)
			}
		})
	}
}

// TestToolsCallParsing tests parsing tools/call method parameters.
func TestToolsCallParsing(t *testing.T) {
	tests := []struct {
		name     string
		message  string
		wantErr  bool
		wantTool string
	}{
		{
			name:     "valid tool call",
			message:  `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"/test"}}}`,
			wantErr:  false,
			wantTool: "read_file",
		},
		{
			name:    "missing params",
			message: `{"jsonrpc":"2.0","id":1,"method":"tools/call"}`,
			wantErr: true,
		},
		{
			name:    "missing tool name",
			message: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"arguments":{}}}`,
			wantErr: true,
		},
		{
			name:     "tool call with meta",
			message:  `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test_tool","_meta":{"agentfacts":"token123"}}}`,
			wantErr:  false,
			wantTool: "test_tool",
		},
	}

	r := NewRouter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, reqCtx, err := r.ParseAndValidate([]byte(tt.message))
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseAndValidate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if req.Method != "tools/call" {
					t.Errorf("Method = %s, want tools/call", req.Method)
				}
				if reqCtx.Tool != tt.wantTool {
					t.Errorf("Tool = %s, want %s", reqCtx.Tool, tt.wantTool)
				}
			}
		})
	}
}

// TestResourcesReadParsing tests parsing resources/read method parameters.
func TestResourcesReadParsing(t *testing.T) {
	tests := []struct {
		name    string
		message string
		wantErr bool
		wantURI string
	}{
		{
			name:    "valid resource read",
			message: `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"file:///test.txt"}}`,
			wantErr: false,
			wantURI: "file:///test.txt",
		},
		{
			name:    "missing params",
			message: `{"jsonrpc":"2.0","id":1,"method":"resources/read"}`,
			wantErr: true,
		},
		{
			name:    "missing uri",
			message: `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{}}`,
			wantErr: true,
		},
	}

	r := NewRouter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, reqCtx, err := r.ParseAndValidate([]byte(tt.message))
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseAndValidate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if req.Method != "resources/read" {
					t.Errorf("Method = %s, want resources/read", req.Method)
				}
				if reqCtx.ResourceURI != tt.wantURI {
					t.Errorf("ResourceURI = %s, want %s", reqCtx.ResourceURI, tt.wantURI)
				}
			}
		})
	}
}

// TestPolicyEvaluationIntegration tests routing with policy evaluation.
func TestPolicyEvaluationIntegration(t *testing.T) {
	tests := []struct {
		name           string
		policyDecision *PolicyDecision
		policyError    error
		expectBlocked  bool
	}{
		{
			name: "policy allows",
			policyDecision: &PolicyDecision{
				Allow:       true,
				PolicyMode:  "enforce",
				MatchedRule: "allow_all",
			},
			policyError:   nil,
			expectBlocked: false,
		},
		{
			name: "policy denies in enforce mode",
			policyDecision: &PolicyDecision{
				Allow:       false,
				PolicyMode:  "enforce",
				Violations:  []string{"missing_capability"},
				MatchedRule: "deny_rule",
			},
			policyError:   nil,
			expectBlocked: true,
		},
		{
			name: "policy denies in audit mode",
			policyDecision: &PolicyDecision{
				Allow:       false,
				PolicyMode:  "audit",
				Violations:  []string{"missing_capability"},
				MatchedRule: "deny_rule",
			},
			policyError:   nil,
			expectBlocked: false, // Audit mode allows through
		},
		{
			name:           "policy evaluation error",
			policyDecision: nil,
			policyError:    errors.New("policy evaluation failed"),
			expectBlocked:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRouter()

			// Set up policy evaluator
			r.SetPolicyEvaluator(func(ctx context.Context, sess *session.Session, reqCtx *RequestContext) (*PolicyDecision, error) {
				return tt.policyDecision, tt.policyError
			})

			// Set up upstream (should only be called if not blocked)
			upstreamCalled := false
			r.SetUpstreamSender(func(ctx context.Context, message []byte) ([]byte, error) {
				upstreamCalled = true
				return []byte(`{"jsonrpc":"2.0","id":1,"result":"ok"}`), nil
			})

			// Create a tools/call request (enforced method)
			msg := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test_tool"}}`
			sess := session.NewSession("test_sess")

			resp, err := r.Route(context.Background(), sess, []byte(msg))
			if err != nil {
				t.Fatalf("Route() error = %v", err)
			}

			// Parse response
			var jsonResp Response
			if err := json.Unmarshal(resp, &jsonResp); err != nil {
				t.Fatalf("Failed to unmarshal response: %v", err)
			}

			// Check if blocked
			if tt.expectBlocked {
				if upstreamCalled {
					t.Error("Upstream was called when request should be blocked")
				}
				if jsonResp.Error == nil {
					t.Error("Response has no error when request should be blocked")
				}
			} else {
				if !upstreamCalled {
					t.Error("Upstream was not called when request should be allowed")
				}
			}
		})
	}
}

// TestAuditLogging tests that audit logger is called with correct parameters.
func TestAuditLogging(t *testing.T) {
	r := NewRouter()

	var capturedCtx context.Context
	var capturedSess *session.Session
	var capturedReqCtx *RequestContext
	var capturedDecision *PolicyDecision
	var capturedResponse []byte
	var capturedLatency time.Duration

	r.SetAuditLogger(func(ctx context.Context, sess *session.Session, reqCtx *RequestContext, decision *PolicyDecision, response []byte, latency time.Duration) {
		capturedCtx = ctx
		capturedSess = sess
		capturedReqCtx = reqCtx
		capturedDecision = decision
		capturedResponse = response
		capturedLatency = latency
	})

	r.SetPolicyEvaluator(func(ctx context.Context, sess *session.Session, reqCtx *RequestContext) (*PolicyDecision, error) {
		return &PolicyDecision{
			Allow:       true,
			PolicyMode:  "enforce",
			MatchedRule: "test_rule",
		}, nil
	})

	r.SetUpstreamSender(func(ctx context.Context, message []byte) ([]byte, error) {
		return []byte(`{"jsonrpc":"2.0","id":1,"result":"ok"}`), nil
	})

	msg := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test_tool"}}`
	sess := session.NewSession("test_sess")
	ctx := context.Background()

	_, err := r.Route(ctx, sess, []byte(msg))
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}

	// Verify audit logger was called
	if capturedCtx == nil {
		t.Error("Audit logger was not called")
	}
	if capturedSess != sess {
		t.Error("Audit logger received wrong session")
	}
	if capturedReqCtx == nil {
		t.Error("Audit logger received nil request context")
	} else if capturedReqCtx.Method != "tools/call" {
		t.Errorf("Audit logger received wrong method: %s", capturedReqCtx.Method)
	}
	if capturedDecision == nil {
		t.Error("Audit logger received nil decision")
	} else if !capturedDecision.Allow {
		t.Error("Audit logger received wrong decision")
	}
	if capturedResponse == nil {
		t.Error("Audit logger received nil response")
	}
	if capturedLatency == 0 {
		t.Error("Audit logger received zero latency")
	}
}

// TestPassthroughHandler tests passthrough routing without policy check.
func TestPassthroughHandler(t *testing.T) {
	r := NewRouter()

	policyEvalCalled := false
	r.SetPolicyEvaluator(func(ctx context.Context, sess *session.Session, reqCtx *RequestContext) (*PolicyDecision, error) {
		policyEvalCalled = true
		return nil, nil
	})

	upstreamCalled := false
	r.SetUpstreamSender(func(ctx context.Context, message []byte) ([]byte, error) {
		upstreamCalled = true
		return message, nil
	})

	// Ping is a passthrough method
	msg := `{"jsonrpc":"2.0","id":1,"method":"ping"}`
	sess := session.NewSession("test_sess")

	_, err := r.Route(context.Background(), sess, []byte(msg))
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}

	if policyEvalCalled {
		t.Error("Policy evaluator should not be called for passthrough methods")
	}
	if !upstreamCalled {
		t.Error("Upstream should be called for passthrough methods")
	}
}

// TestEnforceHandler tests full enforcement routing.
func TestEnforceHandler(t *testing.T) {
	r := NewRouter()

	policyEvalCalled := false
	r.SetPolicyEvaluator(func(ctx context.Context, sess *session.Session, reqCtx *RequestContext) (*PolicyDecision, error) {
		policyEvalCalled = true
		return &PolicyDecision{Allow: true, PolicyMode: "enforce"}, nil
	})

	upstreamCalled := false
	r.SetUpstreamSender(func(ctx context.Context, message []byte) ([]byte, error) {
		upstreamCalled = true
		return []byte(`{"jsonrpc":"2.0","id":1,"result":"ok"}`), nil
	})

	// tools/call is an enforce method
	msg := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test_tool"}}`
	sess := session.NewSession("test_sess")

	_, err := r.Route(context.Background(), sess, []byte(msg))
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}

	if !policyEvalCalled {
		t.Error("Policy evaluator should be called for enforce methods")
	}
	if !upstreamCalled {
		t.Error("Upstream should be called when policy allows")
	}
}

// TestFilterHandler tests filter routing (currently implemented as passthrough).
func TestFilterHandler(t *testing.T) {
	r := NewRouter()

	upstreamCalled := false
	r.SetUpstreamSender(func(ctx context.Context, message []byte) ([]byte, error) {
		upstreamCalled = true
		return []byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`), nil
	})

	// tools/list is a filter method
	msg := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	sess := session.NewSession("test_sess")

	_, err := r.Route(context.Background(), sess, []byte(msg))
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}

	if !upstreamCalled {
		t.Error("Upstream should be called for filter methods")
	}
}

// TestUpstreamError tests handling of upstream errors.
func TestUpstreamError(t *testing.T) {
	r := NewRouter()

	r.SetPolicyEvaluator(func(ctx context.Context, sess *session.Session, reqCtx *RequestContext) (*PolicyDecision, error) {
		return &PolicyDecision{Allow: true, PolicyMode: "enforce"}, nil
	})

	r.SetUpstreamSender(func(ctx context.Context, message []byte) ([]byte, error) {
		return nil, errors.New("upstream connection failed")
	})

	msg := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test_tool"}}`
	sess := session.NewSession("test_sess")

	resp, err := r.Route(context.Background(), sess, []byte(msg))
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}

	// Parse response
	var jsonResp Response
	if err := json.Unmarshal(resp, &jsonResp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	// Should return an error response
	if jsonResp.Error == nil {
		t.Error("Expected error response for upstream failure")
	}
	if jsonResp.Error.Code != CodeUpstreamError {
		t.Errorf("Error code = %d, want %d", jsonResp.Error.Code, CodeUpstreamError)
	}
}

// TestNoUpstream tests routing without upstream sender (echo mode).
func TestNoUpstream(t *testing.T) {
	r := NewRouter()
	// No upstream sender set

	msg := `{"jsonrpc":"2.0","id":1,"method":"ping"}`
	sess := session.NewSession("test_sess")

	resp, err := r.Route(context.Background(), sess, []byte(msg))
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}

	// Should echo back the request
	if string(resp) != msg {
		t.Error("Response does not match request in echo mode")
	}
}

// TestBuildErrorResponse tests building custom error responses.
func TestBuildErrorResponse(t *testing.T) {
	r := NewRouter()

	resp, err := r.BuildErrorResponse(1, CodeMethodNotFound, "Method not found")
	if err != nil {
		t.Fatalf("BuildErrorResponse() error = %v", err)
	}

	var jsonResp Response
	if err := json.Unmarshal(resp, &jsonResp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	// ID can be int or float64 when unmarshaled from JSON
	if jsonResp.ID == nil {
		t.Error("Response ID is nil")
	} else {
		// JSON numbers unmarshal as float64
		if idFloat, ok := jsonResp.ID.(float64); ok {
			if idFloat != 1.0 {
				t.Errorf("Response ID = %v, want 1", jsonResp.ID)
			}
		} else if idInt, ok := jsonResp.ID.(int); ok {
			if idInt != 1 {
				t.Errorf("Response ID = %v, want 1", jsonResp.ID)
			}
		} else {
			t.Errorf("Response ID type = %T, want int or float64", jsonResp.ID)
		}
	}

	if jsonResp.Error == nil {
		t.Error("Response error is nil")
	} else {
		if jsonResp.Error.Code != CodeMethodNotFound {
			t.Errorf("Error code = %d, want %d", jsonResp.Error.Code, CodeMethodNotFound)
		}
		if jsonResp.Error.Message != "Method not found" {
			t.Errorf("Error message = %s, want 'Method not found'", jsonResp.Error.Message)
		}
	}
}

// TestMethodRegistry tests the method configuration registry.
func TestMethodRegistry(t *testing.T) {
	tests := []struct {
		method          string
		expectedHandler HandlerType
		expectedLog     LogLevel
	}{
		{"tools/call", HandlerFullEnforce, LogFull},
		{"tools/list", HandlerFilter, LogMetadata},
		{"resources/read", HandlerFullEnforce, LogFull},
		{"resources/list", HandlerFilter, LogMetadata},
		{"ping", HandlerPassthrough, LogNone},
		{"initialize", HandlerPassthrough, LogMetadata},
		{"notifications/initialized", HandlerPassthrough, LogNone},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			cfg, ok := MethodRegistry[tt.method]
			if !ok {
				t.Errorf("Method %s not found in registry", tt.method)
				return
			}
			if cfg.Handler != tt.expectedHandler {
				t.Errorf("Handler = %v, want %v", cfg.Handler, tt.expectedHandler)
			}
			if cfg.LogLevel != tt.expectedLog {
				t.Errorf("LogLevel = %v, want %v", cfg.LogLevel, tt.expectedLog)
			}
		})
	}
}

// TestUnknownMethod tests routing of unknown methods (should default to passthrough).
func TestUnknownMethod(t *testing.T) {
	r := NewRouter()

	upstreamCalled := false
	r.SetUpstreamSender(func(ctx context.Context, message []byte) ([]byte, error) {
		upstreamCalled = true
		return message, nil
	})

	msg := `{"jsonrpc":"2.0","id":1,"method":"unknown/method"}`
	sess := session.NewSession("test_sess")

	_, err := r.Route(context.Background(), sess, []byte(msg))
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}

	// Unknown methods should be passed through
	if !upstreamCalled {
		t.Error("Upstream should be called for unknown methods")
	}
}

// TestAgentFactsTokenExtraction tests extraction of AgentFacts token from metadata.
func TestAgentFactsTokenExtraction(t *testing.T) {
	r := NewRouter()

	msg := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test_tool","_meta":{"agentfacts":"token123"}}}`

	req, reqCtx, err := r.ParseAndValidate([]byte(msg))
	if err != nil {
		t.Fatalf("ParseAndValidate() error = %v", err)
	}

	if req.Method != "tools/call" {
		t.Errorf("Method = %s, want tools/call", req.Method)
	}

	if reqCtx.AgentFactsToken != "token123" {
		t.Errorf("AgentFactsToken = %s, want 'token123'", reqCtx.AgentFactsToken)
	}
}
