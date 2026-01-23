package compiler

import (
	"strings"
	"testing"
)

func TestCompileCapabilityRule(t *testing.T) {
	compiler := NewCompiler()

	def := &PolicyDefinition{
		Version: "1.0",
		Name:    "test-capability",
		Rules: []RuleDefinition{
			{
				ID:   "require-read",
				Type: RuleTypeCapability,
				Conditions: map[string]interface{}{
					"tool":                "customer_lookup",
					"requires_capability": "read:customers",
				},
				Action:  ActionDeny,
				Message: "Missing capability",
			},
		},
	}

	result, err := compiler.Compile(def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Modules) != 1 {
		t.Errorf("expected 1 module, got %d", len(result.Modules))
	}

	// Check module name
	moduleName := "json_test_capability.rego"
	rego, ok := result.Modules[moduleName]
	if !ok {
		t.Fatalf("expected module %s not found", moduleName)
	}

	// Verify generated Rego contains expected content
	if !strings.Contains(rego, "customer_lookup") {
		t.Error("generated Rego should contain tool name")
	}
	if !strings.Contains(rego, "read:customers") {
		t.Error("generated Rego should contain capability")
	}
	if !strings.Contains(rego, "capability_matches") {
		t.Error("generated Rego should contain capability_matches helper")
	}
}

func TestCompileBlocklistRule(t *testing.T) {
	compiler := NewCompiler()

	def := &PolicyDefinition{
		Version: "1.0",
		Name:    "test-blocklist",
		Rules: []RuleDefinition{
			{
				ID:   "block-tools",
				Type: RuleTypeBlocklist,
				Conditions: map[string]interface{}{
					"match_type": "tool",
					"values":     []interface{}{"rm_rf", "shell_exec"},
				},
				Action:  ActionDeny,
				Message: "Tool is blocked",
			},
		},
	}

	result, err := compiler.Compile(def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	moduleName := "json_test_blocklist.rego"
	rego, ok := result.Modules[moduleName]
	if !ok {
		t.Fatalf("expected module %s not found", moduleName)
	}

	if !strings.Contains(rego, "rm_rf") {
		t.Error("generated Rego should contain blocked tool")
	}
	if !strings.Contains(rego, "shell_exec") {
		t.Error("generated Rego should contain blocked tool")
	}
	if !strings.Contains(rego, "blocked") {
		t.Error("generated Rego should define 'blocked' rule")
	}
}

func TestCompileRateLimitRule(t *testing.T) {
	compiler := NewCompiler()

	def := &PolicyDefinition{
		Version: "1.0",
		Name:    "test-ratelimit",
		Rules: []RuleDefinition{
			{
				ID:   "agent-limit",
				Type: RuleTypeRateLimit,
				Conditions: map[string]interface{}{
					"agent_pattern": "support-.*",
					"limit":         float64(500), // JSON numbers are float64
					"window":        "session",
				},
				Action:  ActionDeny,
				Message: "Rate limit exceeded",
			},
		},
	}

	result, err := compiler.Compile(def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	moduleName := "json_test_ratelimit.rego"
	rego, ok := result.Modules[moduleName]
	if !ok {
		t.Fatalf("expected module %s not found", moduleName)
	}

	if !strings.Contains(rego, "500") {
		t.Error("generated Rego should contain rate limit")
	}
	if !strings.Contains(rego, "rate_limit_ok") {
		t.Error("generated Rego should define 'rate_limit_ok' rule")
	}
}

func TestCompileCustomRule(t *testing.T) {
	compiler := NewCompiler()

	def := &PolicyDefinition{
		Version: "1.0",
		Name:    "test-custom",
		Rules: []RuleDefinition{
			{
				ID:   "pii-check",
				Type: RuleTypeCustom,
				Conditions: map[string]interface{}{
					"all": []interface{}{
						map[string]interface{}{
							"tool_in": []interface{}{"customer_lookup", "payment_history"},
						},
						map[string]interface{}{
							"not": map[string]interface{}{
								"identity.verified": true,
							},
						},
					},
				},
				Action:  ActionDeny,
				Message: "PII requires identity",
			},
		},
	}

	result, err := compiler.Compile(def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	moduleName := "json_test_custom.rego"
	rego, ok := result.Modules[moduleName]
	if !ok {
		t.Fatalf("expected module %s not found", moduleName)
	}

	if !strings.Contains(rego, "customer_lookup") {
		t.Error("generated Rego should contain tool name")
	}
	if !strings.Contains(rego, "violations") {
		t.Error("generated Rego should define 'violations' rule")
	}
}

func TestValidationErrors(t *testing.T) {
	compiler := NewCompiler()

	tests := []struct {
		name string
		def  *PolicyDefinition
		err  string
	}{
		{
			name: "missing version",
			def: &PolicyDefinition{
				Name:  "test",
				Rules: []RuleDefinition{{ID: "r1", Type: RuleTypeBlocklist, Conditions: map[string]interface{}{"match_type": "tool", "values": []interface{}{"x"}}}},
			},
			err: "version is required",
		},
		{
			name: "missing name",
			def: &PolicyDefinition{
				Version: "1.0",
				Rules:   []RuleDefinition{{ID: "r1", Type: RuleTypeBlocklist, Conditions: map[string]interface{}{"match_type": "tool", "values": []interface{}{"x"}}}},
			},
			err: "name is required",
		},
		{
			name: "no rules",
			def: &PolicyDefinition{
				Version: "1.0",
				Name:    "test",
				Rules:   []RuleDefinition{},
			},
			err: "at least one rule is required",
		},
		{
			name: "duplicate rule ID",
			def: &PolicyDefinition{
				Version: "1.0",
				Name:    "test",
				Rules: []RuleDefinition{
					{ID: "r1", Type: RuleTypeBlocklist, Conditions: map[string]interface{}{"match_type": "tool", "values": []interface{}{"x"}}},
					{ID: "r1", Type: RuleTypeBlocklist, Conditions: map[string]interface{}{"match_type": "tool", "values": []interface{}{"y"}}},
				},
			},
			err: "duplicate rule id",
		},
		{
			name: "capability missing tool",
			def: &PolicyDefinition{
				Version: "1.0",
				Name:    "test",
				Rules: []RuleDefinition{
					{ID: "r1", Type: RuleTypeCapability, Conditions: map[string]interface{}{"requires_capability": "read:x"}},
				},
			},
			err: "'tool' condition",
		},
		{
			name: "blocklist invalid match_type",
			def: &PolicyDefinition{
				Version: "1.0",
				Name:    "test",
				Rules: []RuleDefinition{
					{ID: "r1", Type: RuleTypeBlocklist, Conditions: map[string]interface{}{"match_type": "invalid", "values": []interface{}{"x"}}},
				},
			},
			err: "must be one of: tool, agent, did",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := compiler.Compile(tc.def)
			if err == nil {
				t.Errorf("expected error containing %q", tc.err)
				return
			}
			if !strings.Contains(err.Error(), tc.err) {
				t.Errorf("expected error containing %q, got %q", tc.err, err.Error())
			}
		})
	}
}

func TestExpressionCompiler(t *testing.T) {
	ec := NewExpressionCompiler()

	tests := []struct {
		name     string
		expr     map[string]interface{}
		contains []string
	}{
		{
			name: "field equals",
			expr: map[string]interface{}{
				"field_equals": map[string]interface{}{
					"agent.id": "test-agent",
				},
			},
			contains: []string{"input.agent.id", "test-agent"},
		},
		{
			name: "tool_in shorthand",
			expr: map[string]interface{}{
				"tool_in": []interface{}{"tool1", "tool2"},
			},
			contains: []string{"input.request.tool", "tool1", "tool2"},
		},
		{
			name: "not operator",
			expr: map[string]interface{}{
				"not": map[string]interface{}{
					"identity.verified": true,
				},
			},
			contains: []string{"not", "input.identity.verified"},
		},
		{
			name: "field with operator",
			expr: map[string]interface{}{
				"field": "session.request_count",
				"op":    "gte",
				"value": float64(100),
			},
			contains: []string{"input.session.request_count", ">=", "100"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ec.Compile(tc.expr)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			for _, expected := range tc.contains {
				if !strings.Contains(result, expected) {
					t.Errorf("expected result to contain %q, got:\n%s", expected, result)
				}
			}
		})
	}
}

func TestSanitizeRuleID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"with-hyphen", "with_hyphen"},
		{"with_underscore", "with_underscore"},
		{"CamelCase", "CamelCase"},
		{"rule123", "rule123"},
		{"123start", "start"}, // Numbers can't start identifiers
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := sanitizeRuleID(tc.input)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}
