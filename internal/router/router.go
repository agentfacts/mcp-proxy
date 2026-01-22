package router

import (
	"context"
	"time"

	"github.com/agentfacts/mcp-proxy/internal/session"
	json "github.com/goccy/go-json"
	"github.com/rs/zerolog/log"
)

// Router handles MCP message routing and processing.
type Router struct {
	parser   *Parser
	response *ResponseBuilder

	// Callbacks for different stages
	policyEvaluator PolicyEvaluator
	upstreamSender  UpstreamSender
	auditLogger     AuditLogger
}

// PolicyEvaluator is called to evaluate policy for a request.
type PolicyEvaluator func(ctx context.Context, sess *session.Session, reqCtx *RequestContext) (*PolicyDecision, error)

// PolicyDecision contains the result of policy evaluation.
type PolicyDecision struct {
	Allow       bool
	Violations  []string
	MatchedRule string
	PolicyMode  string // "audit" or "enforce"
}

// UpstreamSender is called to forward requests to upstream.
type UpstreamSender func(ctx context.Context, message []byte) ([]byte, error)

// AuditLogger is called to log requests and decisions.
type AuditLogger func(ctx context.Context, sess *session.Session, reqCtx *RequestContext, decision *PolicyDecision, response []byte, latency time.Duration)

// NewRouter creates a new message router.
func NewRouter() *Router {
	return &Router{
		parser:   NewParser(),
		response: NewResponseBuilder(),
	}
}

// SetPolicyEvaluator sets the policy evaluation callback.
func (r *Router) SetPolicyEvaluator(fn PolicyEvaluator) {
	r.policyEvaluator = fn
}

// SetUpstreamSender sets the upstream forwarding callback.
func (r *Router) SetUpstreamSender(fn UpstreamSender) {
	r.upstreamSender = fn
}

// SetAuditLogger sets the audit logging callback.
func (r *Router) SetAuditLogger(fn AuditLogger) {
	r.auditLogger = fn
}

// Route processes an incoming MCP message and returns a response.
func (r *Router) Route(ctx context.Context, sess *session.Session, message []byte) ([]byte, error) {
	start := time.Now()

	// Parse the message
	req, err := r.parser.Parse(message)
	if err != nil {
		if parseErr, ok := err.(*ParseError); ok {
			resp := r.response.FromParseError(parseErr, nil)
			return r.response.Marshal(resp)
		}
		resp := r.response.ParseError(err.Error())
		return r.response.Marshal(resp)
	}

	// Create request context (pooled) - reuse start time to avoid second time.Now() call
	reqCtx := NewRequestContextAt(req, start)
	defer reqCtx.Release()

	// Extract tool/resource information based on method
	if err := r.extractRequestDetails(req, reqCtx); err != nil {
		if parseErr, ok := err.(*ParseError); ok {
			resp := r.response.FromParseError(parseErr, req.ID)
			return r.response.Marshal(resp)
		}
	}

	// Extract AgentFacts token if present
	if meta, _ := r.parser.ExtractMeta(req.Params); meta != nil {
		reqCtx.AgentFactsToken = meta.AgentFacts
	}

	log.Debug().
		Str("request_id", reqCtx.RequestID).
		Str("session_id", sess.ID).
		Str("method", req.Method).
		Str("tool", reqCtx.Tool).
		Str("handler", handlerTypeName(reqCtx.Config.Handler)).
		Msg("Routing request")

	// Handle based on method configuration
	var response []byte
	var decision *PolicyDecision

	switch reqCtx.Config.Handler {
	case HandlerPassthrough:
		response, err = r.handlePassthrough(ctx, sess, reqCtx, message)

	case HandlerFullEnforce:
		response, decision, err = r.handleEnforce(ctx, sess, reqCtx, message)

	case HandlerFilter:
		response, decision, err = r.handleFilter(ctx, sess, reqCtx, message)

	default:
		response, err = r.handlePassthrough(ctx, sess, reqCtx, message)
	}

	latency := time.Since(start)

	// Audit log
	if r.auditLogger != nil && reqCtx.Config.LogLevel != LogNone {
		r.auditLogger(ctx, sess, reqCtx, decision, response, latency)
	}

	log.Debug().
		Str("request_id", reqCtx.RequestID).
		Str("method", req.Method).
		Dur("latency", latency).
		Bool("allowed", decision == nil || decision.Allow).
		Msg("Request completed")

	return response, err
}

// extractRequestDetails parses method-specific details from the request.
func (r *Router) extractRequestDetails(req *Request, reqCtx *RequestContext) error {
	switch req.Method {
	case "tools/call":
		params, err := r.parser.ParseToolCall(req)
		if err != nil {
			return err
		}
		reqCtx.Tool = params.Name
		reqCtx.Arguments = params.Arguments
		if params.Meta != nil {
			reqCtx.AgentFactsToken = params.Meta.AgentFacts
		}

	case "resources/read":
		params, err := r.parser.ParseResourceRead(req)
		if err != nil {
			return err
		}
		reqCtx.ResourceURI = params.URI
		if params.Meta != nil {
			reqCtx.AgentFactsToken = params.Meta.AgentFacts
		}
	}

	return nil
}

// handlePassthrough forwards the request without policy check.
func (r *Router) handlePassthrough(ctx context.Context, sess *session.Session, reqCtx *RequestContext, message []byte) ([]byte, error) {
	if r.upstreamSender != nil {
		return r.upstreamSender(ctx, message)
	}
	// No upstream - echo back
	return message, nil
}

// handleEnforce applies full policy enforcement before forwarding.
func (r *Router) handleEnforce(ctx context.Context, sess *session.Session, reqCtx *RequestContext, message []byte) ([]byte, *PolicyDecision, error) {
	// Evaluate policy
	var decision *PolicyDecision
	if r.policyEvaluator != nil {
		var err error
		decision, err = r.policyEvaluator(ctx, sess, reqCtx)
		if err != nil {
			log.Error().Err(err).Str("request_id", reqCtx.RequestID).Msg("Policy evaluation error")
			resp := r.response.InternalError(reqCtx.Request.ID, "Policy evaluation failed")
			data, _ := r.response.Marshal(resp)
			return data, decision, nil
		}

		// Check decision
		if !decision.Allow {
			if decision.PolicyMode == "enforce" {
				// Block the request
				resp := r.response.PolicyViolation(
					reqCtx.Request.ID,
					reqCtx,
					sess.AgentID,
					sess.Capabilities,
					decision.Violations,
					decision.PolicyMode,
				)
				data, _ := r.response.Marshal(resp)
				return data, decision, nil
			}
			// Audit mode - log but continue
			log.Warn().
				Str("request_id", reqCtx.RequestID).
				Str("agent_id", sess.AgentID).
				Strs("violations", decision.Violations).
				Msg("Policy violation (audit mode)")
		}
	} else {
		// No policy evaluator - default allow
		decision = &PolicyDecision{
			Allow:       true,
			PolicyMode:  "disabled",
			MatchedRule: "no_policy",
		}
	}

	// Forward to upstream
	var response []byte
	var err error
	if r.upstreamSender != nil {
		response, err = r.upstreamSender(ctx, message)
		if err != nil {
			resp := r.response.UpstreamError(reqCtx.Request.ID, err.Error())
			data, _ := r.response.Marshal(resp)
			return data, decision, nil
		}
	} else {
		// No upstream - echo back
		response = message
	}

	return response, decision, nil
}

// handleFilter applies policy filtering to list responses.
func (r *Router) handleFilter(ctx context.Context, sess *session.Session, reqCtx *RequestContext, message []byte) ([]byte, *PolicyDecision, error) {
	// For now, treat filter same as passthrough
	// TODO: Implement response filtering based on capabilities

	decision := &PolicyDecision{
		Allow:       true,
		PolicyMode:  "filter",
		MatchedRule: "passthrough",
	}

	var response []byte
	var err error
	if r.upstreamSender != nil {
		response, err = r.upstreamSender(ctx, message)
	} else {
		response = message
	}

	// TODO: Filter the response to remove unauthorized tools/resources

	return response, decision, err
}

// handlerTypeName returns a string name for the handler type.
func handlerTypeName(h HandlerType) string {
	switch h {
	case HandlerPassthrough:
		return "passthrough"
	case HandlerFullEnforce:
		return "enforce"
	case HandlerFilter:
		return "filter"
	default:
		return "unknown"
	}
}

// ParseAndValidate parses a message and returns the request context.
// Useful for external validation without full routing.
func (r *Router) ParseAndValidate(message []byte) (*Request, *RequestContext, error) {
	req, err := r.parser.Parse(message)
	if err != nil {
		return nil, nil, err
	}

	reqCtx := NewRequestContext(req)
	if err := r.extractRequestDetails(req, reqCtx); err != nil {
		return req, reqCtx, err
	}

	return req, reqCtx, nil
}

// BuildErrorResponse creates an error response for the given request.
func (r *Router) BuildErrorResponse(id interface{}, code int, message string) ([]byte, error) {
	resp := r.response.Error(id, code, message)
	return json.Marshal(resp)
}
