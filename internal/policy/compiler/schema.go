// Package compiler provides JSON to Rego policy compilation.
package compiler

// PolicyDefinition is the root JSON policy structure.
type PolicyDefinition struct {
	Schema      string           `json:"$schema,omitempty"`
	Version     string           `json:"version"`
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Rules       []RuleDefinition `json:"rules"`
	Defaults    DefaultsConfig   `json:"defaults,omitempty"`
}

// RuleDefinition represents a single policy rule.
type RuleDefinition struct {
	ID         string                 `json:"id"`
	Type       RuleType               `json:"type"`
	Priority   int                    `json:"priority,omitempty"`
	Enabled    *bool                  `json:"enabled,omitempty"`
	Conditions map[string]interface{} `json:"conditions"`
	Action     Action                 `json:"action"`
	Message    string                 `json:"message,omitempty"`
}

// IsEnabled returns whether the rule is enabled (defaults to true).
func (r *RuleDefinition) IsEnabled() bool {
	if r.Enabled == nil {
		return true
	}
	return *r.Enabled
}

// RuleType defines the types of rules supported.
type RuleType string

const (
	RuleTypeCapability RuleType = "capability"
	RuleTypeBlocklist  RuleType = "blocklist"
	RuleTypeRateLimit  RuleType = "rate_limit"
	RuleTypeCustom     RuleType = "custom"
)

// Action defines the policy action.
type Action string

const (
	ActionAllow Action = "allow"
	ActionDeny  Action = "deny"
)

// DefaultsConfig holds default policy settings.
type DefaultsConfig struct {
	Action    Action `json:"action,omitempty"`
	RateLimit int    `json:"rate_limit,omitempty"`
}

// CapabilityConditions represents conditions for capability rules.
type CapabilityConditions struct {
	Tool               string `json:"tool"`
	RequiresCapability string `json:"requires_capability"`
}

// BlocklistConditions represents conditions for blocklist rules.
type BlocklistConditions struct {
	MatchType string   `json:"match_type"` // tool, agent, did
	Values    []string `json:"values"`
}

// RateLimitConditions represents conditions for rate limit rules.
type RateLimitConditions struct {
	AgentID      string `json:"agent_id,omitempty"`
	AgentPattern string `json:"agent_pattern,omitempty"`
	Limit        int    `json:"limit"`
	Window       string `json:"window,omitempty"` // session, minute, hour
}

// Expression represents a condition expression for custom rules.
type Expression struct {
	// Logical operators
	All []Expression `json:"all,omitempty"`
	Any []Expression `json:"any,omitempty"`
	Not *Expression  `json:"not,omitempty"`

	// Field operations
	Field string      `json:"field,omitempty"`
	Op    Operator    `json:"op,omitempty"`
	Value interface{} `json:"value,omitempty"`

	// Shorthand conditions
	FieldEquals  map[string]interface{} `json:"field_equals,omitempty"`
	FieldIn      map[string][]string    `json:"field_in,omitempty"`
	FieldMatches map[string]string      `json:"field_matches,omitempty"`
	ToolIn       []string               `json:"tool_in,omitempty"`
	AgentIn      []string               `json:"agent_in,omitempty"`
}

// Operator defines comparison operators.
type Operator string

const (
	OpEquals      Operator = "eq"
	OpNotEquals   Operator = "ne"
	OpGreaterThan Operator = "gt"
	OpGreaterEq   Operator = "gte"
	OpLessThan    Operator = "lt"
	OpLessEq      Operator = "lte"
	OpContains    Operator = "contains"
	OpStartsWith  Operator = "startswith"
	OpEndsWith    Operator = "endswith"
	OpMatches     Operator = "matches"
	OpIn          Operator = "in"
	OpNotIn       Operator = "not_in"
)

// CompileResult contains the compiled Rego output.
type CompileResult struct {
	// Modules maps filename to Rego content
	Modules map[string]string

	// Warnings during compilation (non-fatal)
	Warnings []string

	// Source policy for reference
	Source *PolicyDefinition
}
