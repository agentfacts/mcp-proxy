package policy

import (
	"context"
	"testing"
	"time"
)

// TestNewEngine tests policy engine creation with various configurations.
func TestNewEngine(t *testing.T) {
	tests := []struct {
		name         string
		config       EngineConfig
		expectedMode string
	}{
		{
			name: "enforce mode",
			config: EngineConfig{
				Mode:    "enforce",
				Enabled: true,
			},
			expectedMode: "enforce",
		},
		{
			name: "audit mode",
			config: EngineConfig{
				Mode:    "audit",
				Enabled: true,
			},
			expectedMode: "audit",
		},
		{
			name: "default mode",
			config: EngineConfig{
				Enabled: true,
			},
			expectedMode: "enforce",
		},
		{
			name: "disabled",
			config: EngineConfig{
				Enabled: false,
			},
			expectedMode: "enforce",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := NewEngine(tt.config)
			if engine == nil {
				t.Fatal("NewEngine() returned nil")
			}
			if engine.mode != tt.expectedMode {
				t.Errorf("mode = %s, want %s", engine.mode, tt.expectedMode)
			}
			if engine.enabled != tt.config.Enabled {
				t.Errorf("enabled = %v, want %v", engine.enabled, tt.config.Enabled)
			}
			if engine.cache == nil {
				t.Error("cache is nil")
			}
		})
	}
}

// TestLoadPolicies tests loading and compiling Rego policies.
func TestLoadPolicies(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Mode:    "enforce",
		Enabled: true,
	})

	// Simple test policy
	modules := map[string]string{
		"test.rego": `
package mcp.policy

decision = {
	"allow": true,
	"matched_rule": "allow_all",
	"violations": []
}
`,
	}

	ctx := context.Background()
	err := engine.LoadPolicies(ctx, modules)
	if err != nil {
		t.Fatalf("LoadPolicies() error = %v", err)
	}

	// Verify modules are stored
	if len(engine.modules) != 1 {
		t.Errorf("modules count = %d, want 1", len(engine.modules))
	}
}

// TestLoadPoliciesWithSyntaxError tests handling of invalid Rego syntax.
func TestLoadPoliciesWithSyntaxError(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Mode:    "enforce",
		Enabled: true,
	})

	// Invalid Rego syntax
	modules := map[string]string{
		"bad.rego": `
package mcp.policy
this is not valid rego
`,
	}

	ctx := context.Background()
	err := engine.LoadPolicies(ctx, modules)
	if err == nil {
		t.Error("LoadPolicies() should return error for invalid syntax")
	}
}

// TestEvaluateWithDisabledEngine tests that disabled engine allows everything.
func TestEvaluateWithDisabledEngine(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Mode:    "enforce",
		Enabled: false, // Disabled
	})

	input := NewInputBuilder().
		WithAgent("agent1", "Test Agent", []string{"read"}).
		WithRequest("tools/call", "test_tool", nil).
		Build()

	ctx := context.Background()
	result, err := engine.Evaluate(ctx, input)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	if !result.Decision.Allow {
		t.Error("Disabled engine should allow everything")
	}
	if result.Decision.MatchedRule != "policy_disabled" {
		t.Errorf("MatchedRule = %s, want 'policy_disabled'", result.Decision.MatchedRule)
	}
}

// TestPolicyEvaluationAllow tests policy that allows a request.
func TestPolicyEvaluationAllow(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Mode:    "enforce",
		Enabled: true,
	})

	// Policy that allows everything
	modules := map[string]string{
		"allow.rego": `
package mcp.policy

decision = {
	"allow": true,
	"matched_rule": "allow_all",
	"violations": []
}
`,
	}

	ctx := context.Background()
	err := engine.LoadPolicies(ctx, modules)
	if err != nil {
		t.Fatalf("LoadPolicies() error = %v", err)
	}

	input := NewInputBuilder().
		WithAgent("agent1", "Test Agent", []string{"read"}).
		WithRequest("tools/call", "read_file", nil).
		Build()

	result, err := engine.Evaluate(ctx, input)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	if !result.Decision.Allow {
		t.Error("Decision should allow")
	}
	if result.Decision.MatchedRule != "allow_all" {
		t.Errorf("MatchedRule = %s, want 'allow_all'", result.Decision.MatchedRule)
	}
	if len(result.Decision.Violations) != 0 {
		t.Errorf("Violations count = %d, want 0", len(result.Decision.Violations))
	}
}

// TestPolicyEvaluationDeny tests policy that denies a request.
func TestPolicyEvaluationDeny(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Mode:    "enforce",
		Enabled: true,
	})

	// Policy that denies everything
	modules := map[string]string{
		"deny.rego": `
package mcp.policy

decision = {
	"allow": false,
	"matched_rule": "deny_all",
	"violations": ["access denied"]
}
`,
	}

	ctx := context.Background()
	err := engine.LoadPolicies(ctx, modules)
	if err != nil {
		t.Fatalf("LoadPolicies() error = %v", err)
	}

	input := NewInputBuilder().
		WithAgent("agent1", "Test Agent", []string{"read"}).
		WithRequest("tools/call", "delete_file", nil).
		Build()

	result, err := engine.Evaluate(ctx, input)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	if result.Decision.Allow {
		t.Error("Decision should deny")
	}
	if result.Decision.MatchedRule != "deny_all" {
		t.Errorf("MatchedRule = %s, want 'deny_all'", result.Decision.MatchedRule)
	}
	if len(result.Decision.Violations) == 0 {
		t.Error("Violations should not be empty")
	}
}

// TestPolicyWithData tests policy evaluation with runtime data.
func TestPolicyWithData(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Mode:    "enforce",
		Enabled: true,
	})

	// Policy that checks capabilities from data
	modules := map[string]string{
		"check_caps.rego": `
package mcp.policy

import future.keywords.if

decision = {
	"allow": allow,
	"matched_rule": matched_rule,
	"violations": violations
}

default allow = false
default matched_rule = "no_match"
violations = []

allow if {
	input.agent.capabilities[_] == "admin"
}

matched_rule = "admin_allowed" if {
	input.agent.capabilities[_] == "admin"
}
`,
	}

	ctx := context.Background()
	err := engine.LoadPolicies(ctx, modules)
	if err != nil {
		t.Fatalf("LoadPolicies() error = %v", err)
	}

	// Test with admin capability - should allow
	input1 := NewInputBuilder().
		WithAgent("agent1", "Admin Agent", []string{"admin", "read"}).
		WithRequest("tools/call", "admin_tool", nil).
		Build()

	result1, err := engine.Evaluate(ctx, input1)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if !result1.Decision.Allow {
		t.Error("Admin agent should be allowed")
	}

	// Test without admin capability - should deny
	input2 := NewInputBuilder().
		WithAgent("agent2", "User Agent", []string{"read"}).
		WithRequest("tools/call", "admin_tool", nil).
		Build()

	result2, err := engine.Evaluate(ctx, input2)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if result2.Decision.Allow {
		t.Error("Non-admin agent should be denied")
	}
}

// TestSetPolicyData tests updating runtime policy data.
func TestSetPolicyData(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Mode:    "enforce",
		Enabled: true,
	})

	// Policy that uses data
	modules := map[string]string{
		"data_policy.rego": `
package mcp.policy

import future.keywords.if

decision = {
	"allow": allow,
	"matched_rule": "default",
	"violations": []
}

default allow = true
`,
	}

	ctx := context.Background()
	err := engine.LoadPolicies(ctx, modules)
	if err != nil {
		t.Fatalf("LoadPolicies() error = %v", err)
	}

	// Set policy data
	data := map[string]interface{}{
		"tool_capabilities": map[string]string{
			"read_file":  "read",
			"write_file": "write",
		},
		"blocked_tools": []string{"delete_all"},
	}

	err = engine.SetPolicyData(data)
	if err != nil {
		t.Fatalf("SetPolicyData() error = %v", err)
	}

	// Verify data was set
	engine.dataMu.RLock()
	if len(engine.policyData) == 0 {
		t.Error("Policy data not set")
	}
	engine.dataMu.RUnlock()
}

// TestCacheBehavior tests decision caching.
func TestCacheBehavior(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Mode:    "enforce",
		Enabled: true,
		CacheConfig: CacheConfig{
			Enabled:    true,
			TTL:        1 * time.Minute,
			MaxEntries: 100,
		},
	})

	modules := map[string]string{
		"cache_test.rego": `
package mcp.policy

decision = {
	"allow": true,
	"matched_rule": "allow_all",
	"violations": []
}
`,
	}

	ctx := context.Background()
	err := engine.LoadPolicies(ctx, modules)
	if err != nil {
		t.Fatalf("LoadPolicies() error = %v", err)
	}

	input := NewInputBuilder().
		WithAgent("agent1", "Test Agent", []string{"read"}).
		WithRequest("tools/call", "test_tool", nil).
		Build()

	// First evaluation - should miss cache
	result1, err := engine.Evaluate(ctx, input)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if result1.CacheHit {
		t.Error("First evaluation should not be a cache hit")
	}

	// Second evaluation with same input - should hit cache
	result2, err := engine.Evaluate(ctx, input)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if !result2.CacheHit {
		t.Error("Second evaluation should be a cache hit")
	}
	if result2.CacheTier != "L2" {
		t.Errorf("CacheTier = %s, want L2", result2.CacheTier)
	}

	// Verify both results are the same
	if result1.Decision.Allow != result2.Decision.Allow {
		t.Error("Cached result differs from original")
	}
}

// TestCacheInvalidation tests cache invalidation on data update.
func TestCacheInvalidation(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Mode:    "enforce",
		Enabled: true,
		CacheConfig: CacheConfig{
			Enabled:    true,
			TTL:        1 * time.Minute,
			MaxEntries: 100,
		},
	})

	modules := map[string]string{
		"test.rego": `
package mcp.policy

decision = {
	"allow": true,
	"matched_rule": "allow_all",
	"violations": []
}
`,
	}

	ctx := context.Background()
	err := engine.LoadPolicies(ctx, modules)
	if err != nil {
		t.Fatalf("LoadPolicies() error = %v", err)
	}

	input := NewInputBuilder().
		WithAgent("agent1", "Test Agent", []string{"read"}).
		WithRequest("tools/call", "test_tool", nil).
		Build()

	// First evaluation
	result1, err := engine.Evaluate(ctx, input)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if result1.CacheHit {
		t.Error("First evaluation should not be a cache hit")
	}

	// Second evaluation - should hit cache
	result2, err := engine.Evaluate(ctx, input)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if !result2.CacheHit {
		t.Error("Second evaluation should be a cache hit")
	}

	// Update policy data - should invalidate cache
	data := map[string]interface{}{
		"test": "data",
	}
	err = engine.SetPolicyData(data)
	if err != nil {
		t.Fatalf("SetPolicyData() error = %v", err)
	}

	// Third evaluation - should miss cache after invalidation
	result3, err := engine.Evaluate(ctx, input)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if result3.CacheHit {
		t.Error("Evaluation after data update should not be a cache hit")
	}
}

// TestAuditModeVsEnforceMode tests behavior difference between modes.
func TestAuditModeVsEnforceMode(t *testing.T) {
	tests := []struct {
		name          string
		mode          string
		allow         bool
		expectAllowed bool
	}{
		{"enforce allow", "enforce", true, true},
		{"enforce deny", "enforce", false, false},
		{"audit allow", "audit", true, true},
		{"audit deny", "audit", false, true}, // Audit mode always allows
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := NewEngine(EngineConfig{
				Mode:    tt.mode,
				Enabled: true,
			})

			allowStr := "false"
			if tt.allow {
				allowStr = "true"
			}

			modules := map[string]string{
				"test.rego": `
package mcp.policy

decision = {
	"allow": ` + allowStr + `,
	"matched_rule": "test",
	"violations": []
}
`,
			}

			ctx := context.Background()
			err := engine.LoadPolicies(ctx, modules)
			if err != nil {
				t.Fatalf("LoadPolicies() error = %v", err)
			}

			input := NewInputBuilder().
				WithAgent("agent1", "Test Agent", []string{"read"}).
				WithRequest("tools/call", "test_tool", nil).
				Build()

			allowed, result, err := engine.IsAllowed(ctx, input)
			if err != nil {
				t.Fatalf("IsAllowed() error = %v", err)
			}

			if allowed != tt.expectAllowed {
				t.Errorf("IsAllowed() = %v, want %v", allowed, tt.expectAllowed)
			}

			if result.PolicyMode != tt.mode {
				t.Errorf("PolicyMode = %s, want %s", result.PolicyMode, tt.mode)
			}
		})
	}
}

// TestEngineStats tests statistics collection.
func TestEngineStats(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Mode:    "enforce",
		Enabled: true,
		CacheConfig: CacheConfig{
			Enabled: false, // Disable cache for predictable stats
		},
	})

	modules := map[string]string{
		"test.rego": `
package mcp.policy

decision = {
	"allow": true,
	"matched_rule": "allow_all",
	"violations": []
}
`,
	}

	ctx := context.Background()
	err := engine.LoadPolicies(ctx, modules)
	if err != nil {
		t.Fatalf("LoadPolicies() error = %v", err)
	}

	input := NewInputBuilder().
		WithAgent("agent1", "Test Agent", []string{"read"}).
		WithRequest("tools/call", "test_tool", nil).
		Build()

	// Initial stats
	stats1 := engine.Stats()
	if stats1.Evaluations != 0 {
		t.Errorf("Initial evaluations = %d, want 0", stats1.Evaluations)
	}

	// Evaluate a few times
	for i := 0; i < 5; i++ {
		_, err := engine.Evaluate(ctx, input)
		if err != nil {
			t.Fatalf("Evaluate() error = %v", err)
		}
	}

	// Check stats
	stats2 := engine.Stats()
	if stats2.Evaluations != 5 {
		t.Errorf("Evaluations = %d, want 5", stats2.Evaluations)
	}
	if stats2.AvgEvalTimeMs <= 0 {
		t.Error("AvgEvalTimeMs should be > 0")
	}
}

// TestIsReady tests engine readiness check.
func TestIsReady(t *testing.T) {
	// Disabled engine is always ready
	engine1 := NewEngine(EngineConfig{
		Enabled: false,
	})
	if !engine1.IsReady() {
		t.Error("Disabled engine should be ready")
	}

	// Enabled engine without policies is not ready
	engine2 := NewEngine(EngineConfig{
		Enabled: true,
	})
	if engine2.IsReady() {
		t.Error("Engine without policies should not be ready")
	}

	// Enabled engine with policies is ready
	modules := map[string]string{
		"test.rego": `
package mcp.policy

decision = {
	"allow": true,
	"matched_rule": "allow_all",
	"violations": []
}
`,
	}

	ctx := context.Background()
	err := engine2.LoadPolicies(ctx, modules)
	if err != nil {
		t.Fatalf("LoadPolicies() error = %v", err)
	}

	if !engine2.IsReady() {
		t.Error("Engine with policies should be ready")
	}
}

// TestPolicyWithObligations tests parsing of policy obligations.
func TestPolicyWithObligations(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Mode:    "enforce",
		Enabled: true,
	})

	// Policy with obligations
	modules := map[string]string{
		"obligations.rego": `
package mcp.policy

decision = {
	"allow": true,
	"matched_rule": "allow_with_logging",
	"violations": [],
	"obligations": [
		{
			"action": "log",
			"params": {"level": "info"}
		},
		{
			"action": "alert",
			"params": {"channel": "security"}
		}
	]
}
`,
	}

	ctx := context.Background()
	err := engine.LoadPolicies(ctx, modules)
	if err != nil {
		t.Fatalf("LoadPolicies() error = %v", err)
	}

	input := NewInputBuilder().
		WithAgent("agent1", "Test Agent", []string{"read"}).
		WithRequest("tools/call", "test_tool", nil).
		Build()

	result, err := engine.Evaluate(ctx, input)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	if len(result.Decision.Obligations) != 2 {
		t.Errorf("Obligations count = %d, want 2", len(result.Decision.Obligations))
	}

	if result.Decision.Obligations[0].Action != "log" {
		t.Errorf("First obligation action = %s, want 'log'", result.Decision.Obligations[0].Action)
	}
	if result.Decision.Obligations[1].Action != "alert" {
		t.Errorf("Second obligation action = %s, want 'alert'", result.Decision.Obligations[1].Action)
	}
}

// TestInputBuilder tests the PolicyInput builder.
func TestInputBuilder(t *testing.T) {
	input := NewInputBuilder().
		WithAgent("agent1", "Test Agent", []string{"read", "write"}).
		WithAgentDetails("gpt-4", "OpenAI", []string{"production"}).
		WithRequest("tools/call", "test_tool", map[string]interface{}{"key": "value"}).
		WithSession("sess_123", 5, time.Now().Add(-1*time.Hour)).
		WithIdentity(true, "did:example:123").
		WithEnvironment("192.168.1.1", "production", "us-east-1").
		Build()

	if input.Agent.ID != "agent1" {
		t.Errorf("Agent.ID = %s, want 'agent1'", input.Agent.ID)
	}
	if input.Agent.Name != "Test Agent" {
		t.Errorf("Agent.Name = %s, want 'Test Agent'", input.Agent.Name)
	}
	if len(input.Agent.Capabilities) != 2 {
		t.Errorf("Agent.Capabilities length = %d, want 2", len(input.Agent.Capabilities))
	}
	if input.Agent.Model != "gpt-4" {
		t.Errorf("Agent.Model = %s, want 'gpt-4'", input.Agent.Model)
	}
	if input.Request.Method != "tools/call" {
		t.Errorf("Request.Method = %s, want 'tools/call'", input.Request.Method)
	}
	if input.Request.Tool != "test_tool" {
		t.Errorf("Request.Tool = %s, want 'test_tool'", input.Request.Tool)
	}
	if input.Session.ID != "sess_123" {
		t.Errorf("Session.ID = %s, want 'sess_123'", input.Session.ID)
	}
	if input.Session.RequestCount != 5 {
		t.Errorf("Session.RequestCount = %d, want 5", input.Session.RequestCount)
	}
	if !input.Identity.Verified {
		t.Error("Identity.Verified should be true")
	}
	if input.Identity.DID != "did:example:123" {
		t.Errorf("Identity.DID = %s, want 'did:example:123'", input.Identity.DID)
	}
	if input.Context.SourceIP != "192.168.1.1" {
		t.Errorf("Context.SourceIP = %s, want '192.168.1.1'", input.Context.SourceIP)
	}
	if input.Context.Environment != "production" {
		t.Errorf("Context.Environment = %s, want 'production'", input.Context.Environment)
	}
}

// TestModeGetter tests the Mode() getter.
func TestModeGetter(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Mode:    "audit",
		Enabled: true,
	})

	if engine.Mode() != "audit" {
		t.Errorf("Mode() = %s, want 'audit'", engine.Mode())
	}
}
