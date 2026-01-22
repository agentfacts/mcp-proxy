package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/storage/inmem"
)

// Engine provides policy evaluation using embedded OPA.
type Engine struct {
	// Compiled policy query
	query rego.PreparedEvalQuery
	mu    sync.RWMutex

	// Policy modules (kept for recompilation when data changes)
	modules map[string]string

	// Policy data (tool_capabilities, rate_limits, etc.)
	policyData map[string]interface{}
	dataMu     sync.RWMutex

	// Decision cache
	cache *DecisionCache

	// Configuration
	mode    string // "enforce" or "audit"
	enabled bool

	// Metrics
	evaluations   int64
	evalErrors    int64
	avgEvalTimeNs int64
}

// EngineConfig holds configuration for the policy engine.
type EngineConfig struct {
	Mode        string // "enforce" or "audit"
	Enabled     bool
	CacheConfig CacheConfig
}

// NewEngine creates a new policy engine.
func NewEngine(cfg EngineConfig) *Engine {
	if cfg.Mode == "" {
		cfg.Mode = "enforce"
	}

	return &Engine{
		policyData: make(map[string]interface{}),
		cache:      NewDecisionCache(cfg.CacheConfig),
		mode:       cfg.Mode,
		enabled:    cfg.Enabled,
	}
}

// LoadPolicies compiles and loads Rego policies.
func (e *Engine) LoadPolicies(ctx context.Context, modules map[string]string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Store modules for later recompilation
	e.modules = modules

	// Compile with current policy data
	return e.compileWithData(ctx)
}

// compileWithData compiles policies with the current policy data.
// Must be called with e.mu held.
func (e *Engine) compileWithData(ctx context.Context) error {
	// Build rego options with all modules
	opts := []func(*rego.Rego){
		rego.Query("data.mcp.policy.decision"),
	}

	for name, content := range e.modules {
		opts = append(opts, rego.Module(name, content))
	}

	// Add data store if we have policy data
	e.dataMu.RLock()
	if len(e.policyData) > 0 {
		store := inmem.NewFromObject(e.policyData)
		opts = append(opts, rego.Store(store))
	}
	e.dataMu.RUnlock()

	// Compile the query
	r := rego.New(opts...)
	query, err := r.PrepareForEval(ctx)
	if err != nil {
		return fmt.Errorf("failed to compile policies: %w", err)
	}

	e.query = query
	return nil
}

// SetPolicyData updates the runtime policy data.
func (e *Engine) SetPolicyData(data map[string]interface{}) error {
	e.dataMu.Lock()
	e.policyData = data
	e.dataMu.Unlock()

	// Invalidate cache when data changes
	e.cache.Invalidate()

	// Recompile with new data
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.modules) > 0 {
		ctx := context.Background()
		return e.compileWithData(ctx)
	}
	return nil
}

// Evaluate evaluates a policy decision for the given input.
func (e *Engine) Evaluate(ctx context.Context, input *PolicyInput) (*EvaluationResult, error) {
	start := time.Now()

	result := &EvaluationResult{
		Input:      input,
		PolicyMode: e.mode,
	}

	// If disabled, allow everything
	if !e.enabled {
		result.Decision = &PolicyDecision{
			Allow:       true,
			MatchedRule: "policy_disabled",
		}
		result.EvalTime = time.Since(start)
		return result, nil
	}

	// Check cache first
	cacheKey := e.cache.ComputeKey(input)
	if cached, hit, tier := e.cache.Get(cacheKey); hit {
		result.Decision = cached
		result.CacheHit = true
		result.CacheTier = tier
		result.EvalTime = time.Since(start)
		return result, nil
	}

	// Evaluate policy
	decision, err := e.evaluatePolicy(ctx, input)
	if err != nil {
		e.evalErrors++
		return nil, fmt.Errorf("policy evaluation failed: %w", err)
	}

	result.Decision = decision
	result.EvalTime = time.Since(start)

	// Update metrics
	e.evaluations++
	e.updateAvgEvalTime(result.EvalTime)

	// Cache the result
	e.cache.Set(cacheKey, decision)

	return result, nil
}

// evaluatePolicy runs the OPA evaluation.
func (e *Engine) evaluatePolicy(ctx context.Context, input *PolicyInput) (*PolicyDecision, error) {
	e.mu.RLock()
	query := e.query
	e.mu.RUnlock()

	// Convert input to map for OPA
	inputMap, err := structToMap(input)
	if err != nil {
		return nil, fmt.Errorf("failed to convert input: %w", err)
	}

	// Evaluate with input (data is already in the compiled store)
	results, err := query.Eval(ctx, rego.EvalInput(inputMap))
	if err != nil {
		return nil, fmt.Errorf("evaluation error: %w", err)
	}

	if len(results) == 0 {
		return &PolicyDecision{
			Allow:       false,
			Violations:  []string{"No policy decision returned"},
			MatchedRule: "no_result",
		}, nil
	}

	// Parse decision from results
	decision, err := parseDecision(results[0].Expressions[0].Value)
	if err != nil {
		return nil, fmt.Errorf("failed to parse decision: %w", err)
	}

	return decision, nil
}

// parseDecision converts OPA output to PolicyDecision.
func parseDecision(value interface{}) (*PolicyDecision, error) {
	decisionMap, ok := value.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected decision type: %T", value)
	}

	decision := &PolicyDecision{}

	// Parse allow
	if allow, ok := decisionMap["allow"].(bool); ok {
		decision.Allow = allow
	}

	// Parse violations
	if violations, ok := decisionMap["violations"].([]interface{}); ok {
		for _, v := range violations {
			if s, ok := v.(string); ok {
				decision.Violations = append(decision.Violations, s)
			}
		}
	}

	// Parse matched_rule
	if rule, ok := decisionMap["matched_rule"].(string); ok {
		decision.MatchedRule = rule
	}

	// Parse obligations if present
	if obligations, ok := decisionMap["obligations"].([]interface{}); ok {
		for _, o := range obligations {
			if oblMap, ok := o.(map[string]interface{}); ok {
				obl := PolicyObligation{
					Params: make(map[string]string),
				}
				if action, ok := oblMap["action"].(string); ok {
					obl.Action = action
				}
				if params, ok := oblMap["params"].(map[string]interface{}); ok {
					for k, v := range params {
						if s, ok := v.(string); ok {
							obl.Params[k] = s
						}
					}
				}
				decision.Obligations = append(decision.Obligations, obl)
			}
		}
	}

	return decision, nil
}

// structToMap converts a struct to a map using JSON marshaling.
func structToMap(v interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// updateAvgEvalTime updates the rolling average evaluation time.
func (e *Engine) updateAvgEvalTime(d time.Duration) {
	// Simple exponential moving average
	alpha := int64(10) // Weight for new value
	if e.avgEvalTimeNs == 0 {
		e.avgEvalTimeNs = d.Nanoseconds()
	} else {
		e.avgEvalTimeNs = (e.avgEvalTimeNs*(100-alpha) + d.Nanoseconds()*alpha) / 100
	}
}

// Mode returns the current policy mode.
func (e *Engine) Mode() string {
	return e.mode
}

// Stats returns engine statistics.
func (e *Engine) Stats() EngineStats {
	cacheStats := e.cache.Stats()
	return EngineStats{
		Evaluations:   e.evaluations,
		EvalErrors:    e.evalErrors,
		AvgEvalTimeMs: float64(e.avgEvalTimeNs) / 1e6,
		CacheStats:    cacheStats,
	}
}

// EngineStats contains policy engine statistics.
type EngineStats struct {
	Evaluations   int64
	EvalErrors    int64
	AvgEvalTimeMs float64
	CacheStats    CacheStats
}

// IsAllowed is a convenience method to check if a request is allowed.
func (e *Engine) IsAllowed(ctx context.Context, input *PolicyInput) (bool, *EvaluationResult, error) {
	result, err := e.Evaluate(ctx, input)
	if err != nil {
		return false, nil, err
	}

	// In audit mode, always return true but still log the decision
	if e.mode == "audit" {
		return true, result, nil
	}

	return result.Decision.Allow, result, nil
}

// IsReady returns true if the policy engine is initialized and ready.
func (e *Engine) IsReady() bool {
	if !e.enabled {
		return true // If disabled, always ready
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.modules) > 0
}
