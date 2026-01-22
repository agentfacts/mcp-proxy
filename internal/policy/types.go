package policy

import (
	"time"
)

// PolicyInput is the input structure sent to OPA for policy evaluation.
// This matches the schema defined in the spec section 4.2.
type PolicyInput struct {
	Agent    AgentContext      `json:"agent"`
	Request  RequestContext    `json:"request"`
	Session  SessionContext    `json:"session"`
	Identity IdentityContext   `json:"identity"`
	Context  EnvironmentContext `json:"context"`
}

// AgentContext contains information about the agent making the request.
type AgentContext struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Capabilities []string `json:"capabilities"`
	Model        string   `json:"model"`
	Publisher    string   `json:"publisher"`
	Tags         []string `json:"tags"`
}

// RequestContext contains information about the request being made.
type RequestContext struct {
	Method    string                 `json:"method"`
	Tool      string                 `json:"tool"`
	Arguments map[string]interface{} `json:"arguments"`
	Intent    string                 `json:"intent"`
}

// SessionContext contains information about the current session.
type SessionContext struct {
	ID               string    `json:"id"`
	RequestCount     int       `json:"request_count"`
	StartedAt        time.Time `json:"started_at"`
	CumulativeReads  int       `json:"cumulative_reads"`
	CumulativeWrites int       `json:"cumulative_writes"`
}

// IdentityContext contains verified identity information from AgentFacts.
type IdentityContext struct {
	Verified     bool      `json:"verified"`
	DID          string    `json:"did"`
	SignatureAlg string    `json:"signature_alg"`
	IssuedAt     time.Time `json:"issued_at"`
	HasLogProof  bool      `json:"has_log_proof"`
}

// EnvironmentContext contains information about the execution environment.
type EnvironmentContext struct {
	Timestamp   time.Time `json:"timestamp"`
	SourceIP    string    `json:"source_ip"`
	Environment string    `json:"environment"`
	ProxyRegion string    `json:"proxy_region"`
}

// PolicyDecision is the output from OPA policy evaluation.
type PolicyDecision struct {
	Allow       bool               `json:"allow"`
	Violations  []string           `json:"violations"`
	MatchedRule string             `json:"matched_rule"`
	Obligations []PolicyObligation `json:"obligations,omitempty"`
}

// PolicyObligation represents an action that must be taken (e.g., log, alert).
type PolicyObligation struct {
	Action string            `json:"action"` // "log", "alert", "rate_limit"
	Params map[string]string `json:"params"`
}

// PolicyData contains runtime policy data loaded from JSON.
type PolicyData struct {
	ToolCapabilities      map[string]string `json:"tool_capabilities"`
	RateLimits            map[string]int    `json:"rate_limits"`
	BlockedTools          []string          `json:"blocked_tools"`
	BlockedAgents         []string          `json:"blocked_agents"`
	BlockedDIDs           []string          `json:"blocked_dids"`
	AllowedDIDs           []string          `json:"allowed_dids"`
	TrustedPublishers     []string          `json:"trusted_publishers"`
	IdentityRequiredTools []string          `json:"identity_required_tools"`
	PIITools              []string          `json:"pii_tools"`
	BlockedModelsForPII   []string          `json:"blocked_models_for_pii"`
}

// EvaluationResult contains the full result of a policy evaluation.
type EvaluationResult struct {
	Decision    *PolicyDecision
	Input       *PolicyInput
	EvalTime    time.Duration
	CacheHit    bool
	CacheTier   string // "L1", "L2", or ""
	PolicyMode  string // "audit" or "enforce"
}

// InputBuilder helps construct PolicyInput from various sources.
type InputBuilder struct {
	input PolicyInput
}

// NewInputBuilder creates a new InputBuilder with defaults.
func NewInputBuilder() *InputBuilder {
	return &InputBuilder{
		input: PolicyInput{
			Context: EnvironmentContext{
				Timestamp: time.Now(),
			},
		},
	}
}

// WithAgent sets the agent context.
func (b *InputBuilder) WithAgent(id, name string, capabilities []string) *InputBuilder {
	b.input.Agent = AgentContext{
		ID:           id,
		Name:         name,
		Capabilities: capabilities,
	}
	return b
}

// WithAgentDetails sets additional agent details.
func (b *InputBuilder) WithAgentDetails(model, publisher string, tags []string) *InputBuilder {
	b.input.Agent.Model = model
	b.input.Agent.Publisher = publisher
	b.input.Agent.Tags = tags
	return b
}

// WithRequest sets the request context.
func (b *InputBuilder) WithRequest(method, tool string, arguments map[string]interface{}) *InputBuilder {
	b.input.Request = RequestContext{
		Method:    method,
		Tool:      tool,
		Arguments: arguments,
	}
	return b
}

// WithSession sets the session context.
func (b *InputBuilder) WithSession(id string, requestCount int, startedAt time.Time) *InputBuilder {
	b.input.Session = SessionContext{
		ID:           id,
		RequestCount: requestCount,
		StartedAt:    startedAt,
	}
	return b
}

// WithIdentity sets the identity context.
func (b *InputBuilder) WithIdentity(verified bool, did string) *InputBuilder {
	b.input.Identity = IdentityContext{
		Verified: verified,
		DID:      did,
	}
	return b
}

// WithEnvironment sets the environment context.
func (b *InputBuilder) WithEnvironment(sourceIP, environment, region string) *InputBuilder {
	b.input.Context.SourceIP = sourceIP
	b.input.Context.Environment = environment
	b.input.Context.ProxyRegion = region
	return b
}

// Build returns the constructed PolicyInput.
func (b *InputBuilder) Build() *PolicyInput {
	return &b.input
}
