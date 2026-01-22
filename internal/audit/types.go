package audit

import (
	"time"
)

// Record represents a single audit log entry.
type Record struct {
	// Identifiers
	ID        int64  `json:"id"`
	RequestID string `json:"request_id"`
	SessionID string `json:"session_id"`

	// Timing
	Timestamp time.Time `json:"timestamp"`
	Latency   float64   `json:"latency_ms"`

	// Agent info
	AgentID      string `json:"agent_id"`
	AgentName    string `json:"agent_name,omitempty"`
	Capabilities string `json:"capabilities,omitempty"` // JSON array as string

	// Request info
	Method      string `json:"method"`
	Tool        string `json:"tool,omitempty"`
	ResourceURI string `json:"resource_uri,omitempty"`
	Arguments   string `json:"arguments,omitempty"` // JSON as string

	// Identity info
	IdentityVerified bool   `json:"identity_verified"`
	DID              string `json:"did,omitempty"`

	// Policy decision
	Allowed     bool   `json:"allowed"`
	MatchedRule string `json:"matched_rule,omitempty"`
	Violations  string `json:"violations,omitempty"` // JSON array as string
	PolicyMode  string `json:"policy_mode"`

	// Environment
	SourceIP    string `json:"source_ip,omitempty"`
	Environment string `json:"environment,omitempty"`
}

// RecordBuilder helps construct audit records.
type RecordBuilder struct {
	record Record
}

// NewRecordBuilder creates a new record builder.
func NewRecordBuilder() *RecordBuilder {
	return &RecordBuilder{
		record: Record{
			Timestamp: time.Now(),
		},
	}
}

// WithRequest sets request identifiers.
func (b *RecordBuilder) WithRequest(requestID, sessionID string) *RecordBuilder {
	b.record.RequestID = requestID
	b.record.SessionID = sessionID
	return b
}

// WithTiming sets timing information.
func (b *RecordBuilder) WithTiming(latencyMs float64) *RecordBuilder {
	b.record.Latency = latencyMs
	return b
}

// WithAgent sets agent information.
func (b *RecordBuilder) WithAgent(agentID, agentName, capabilities string) *RecordBuilder {
	b.record.AgentID = agentID
	b.record.AgentName = agentName
	b.record.Capabilities = capabilities
	return b
}

// WithMethod sets the request method and details.
func (b *RecordBuilder) WithMethod(method, tool, resourceURI, arguments string) *RecordBuilder {
	b.record.Method = method
	b.record.Tool = tool
	b.record.ResourceURI = resourceURI
	b.record.Arguments = arguments
	return b
}

// WithIdentity sets identity information.
func (b *RecordBuilder) WithIdentity(verified bool, did string) *RecordBuilder {
	b.record.IdentityVerified = verified
	b.record.DID = did
	return b
}

// WithDecision sets the policy decision.
func (b *RecordBuilder) WithDecision(allowed bool, matchedRule, violations, policyMode string) *RecordBuilder {
	b.record.Allowed = allowed
	b.record.MatchedRule = matchedRule
	b.record.Violations = violations
	b.record.PolicyMode = policyMode
	return b
}

// WithEnvironment sets environment context.
func (b *RecordBuilder) WithEnvironment(sourceIP, environment string) *RecordBuilder {
	b.record.SourceIP = sourceIP
	b.record.Environment = environment
	return b
}

// Build returns the constructed record.
func (b *RecordBuilder) Build() *Record {
	return &b.record
}

// QueryOptions for filtering audit records.
type QueryOptions struct {
	// Time range
	StartTime *time.Time
	EndTime   *time.Time

	// Filters
	AgentID   string
	SessionID string
	Method    string
	Tool      string
	Allowed   *bool

	// Pagination
	Limit  int
	Offset int

	// Ordering
	OrderBy   string // "timestamp", "agent_id", etc.
	OrderDesc bool
}

// Stats contains aggregate statistics.
type Stats struct {
	TotalRequests   int64   `json:"total_requests"`
	AllowedRequests int64   `json:"allowed_requests"`
	DeniedRequests  int64   `json:"denied_requests"`
	UniqueAgents    int64   `json:"unique_agents"`
	UniqueSessions  int64   `json:"unique_sessions"`
	AvgLatencyMs    float64 `json:"avg_latency_ms"`
}
