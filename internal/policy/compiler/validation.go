package compiler

import (
	"fmt"
	"regexp"
	"strings"
)

// Validator validates policy definitions.
type Validator struct {
	ruleIDs map[string]bool
}

// NewValidator creates a new policy validator.
func NewValidator() *Validator {
	return &Validator{
		ruleIDs: make(map[string]bool),
	}
}

// Validate validates a policy definition.
func (v *Validator) Validate(def *PolicyDefinition) error {
	v.ruleIDs = make(map[string]bool) // Reset for each validation

	if def.Version == "" {
		return fmt.Errorf("version is required")
	}

	if def.Name == "" {
		return fmt.Errorf("name is required")
	}

	if !isValidIdentifier(def.Name) {
		return fmt.Errorf("name must be a valid identifier (alphanumeric and hyphens): %s", def.Name)
	}

	if len(def.Rules) == 0 {
		return fmt.Errorf("at least one rule is required")
	}

	for i, rule := range def.Rules {
		if err := v.validateRule(&rule, i); err != nil {
			return fmt.Errorf("rule[%d]: %w", i, err)
		}
	}

	return nil
}

func (v *Validator) validateRule(rule *RuleDefinition, index int) error {
	if rule.ID == "" {
		return fmt.Errorf("id is required")
	}

	if !isValidIdentifier(rule.ID) {
		return fmt.Errorf("id must be a valid identifier: %s", rule.ID)
	}

	if v.ruleIDs[rule.ID] {
		return fmt.Errorf("duplicate rule id: %s", rule.ID)
	}
	v.ruleIDs[rule.ID] = true

	if rule.Type == "" {
		return fmt.Errorf("type is required")
	}

	switch rule.Type {
	case RuleTypeCapability:
		return v.validateCapabilityRule(rule)
	case RuleTypeBlocklist:
		return v.validateBlocklistRule(rule)
	case RuleTypeRateLimit:
		return v.validateRateLimitRule(rule)
	case RuleTypeCustom:
		return v.validateCustomRule(rule)
	default:
		return fmt.Errorf("unknown rule type: %s", rule.Type)
	}
}

func (v *Validator) validateCapabilityRule(rule *RuleDefinition) error {
	tool, ok := rule.Conditions["tool"]
	if !ok {
		return fmt.Errorf("capability rule requires 'tool' condition")
	}
	if _, ok := tool.(string); !ok {
		return fmt.Errorf("'tool' must be a string")
	}

	cap, ok := rule.Conditions["requires_capability"]
	if !ok {
		return fmt.Errorf("capability rule requires 'requires_capability' condition")
	}
	if _, ok := cap.(string); !ok {
		return fmt.Errorf("'requires_capability' must be a string")
	}

	return nil
}

func (v *Validator) validateBlocklistRule(rule *RuleDefinition) error {
	matchType, ok := rule.Conditions["match_type"]
	if !ok {
		return fmt.Errorf("blocklist rule requires 'match_type' condition")
	}

	mt, ok := matchType.(string)
	if !ok {
		return fmt.Errorf("'match_type' must be a string")
	}

	validTypes := map[string]bool{"tool": true, "agent": true, "did": true}
	if !validTypes[mt] {
		return fmt.Errorf("'match_type' must be one of: tool, agent, did")
	}

	values, ok := rule.Conditions["values"]
	if !ok {
		return fmt.Errorf("blocklist rule requires 'values' condition")
	}

	valuesSlice, ok := values.([]interface{})
	if !ok {
		return fmt.Errorf("'values' must be an array")
	}

	if len(valuesSlice) == 0 {
		return fmt.Errorf("'values' must not be empty")
	}

	for i, val := range valuesSlice {
		if _, ok := val.(string); !ok {
			return fmt.Errorf("'values[%d]' must be a string", i)
		}
	}

	return nil
}

func (v *Validator) validateRateLimitRule(rule *RuleDefinition) error {
	limit, ok := rule.Conditions["limit"]
	if !ok {
		return fmt.Errorf("rate_limit rule requires 'limit' condition")
	}

	switch l := limit.(type) {
	case float64:
		if l <= 0 {
			return fmt.Errorf("'limit' must be positive")
		}
	case int:
		if l <= 0 {
			return fmt.Errorf("'limit' must be positive")
		}
	default:
		return fmt.Errorf("'limit' must be a number")
	}

	// Either agent_id or agent_pattern should be specified
	_, hasID := rule.Conditions["agent_id"]
	_, hasPattern := rule.Conditions["agent_pattern"]

	if !hasID && !hasPattern {
		// Allow default rate limit without agent specification
	}

	if window, ok := rule.Conditions["window"]; ok {
		w, ok := window.(string)
		if !ok {
			return fmt.Errorf("'window' must be a string")
		}
		validWindows := map[string]bool{"session": true, "minute": true, "hour": true}
		if !validWindows[w] {
			return fmt.Errorf("'window' must be one of: session, minute, hour")
		}
	}

	return nil
}

func (v *Validator) validateCustomRule(rule *RuleDefinition) error {
	// Custom rules must have at least one condition
	if len(rule.Conditions) == 0 {
		return fmt.Errorf("custom rule requires at least one condition")
	}

	// Validate that conditions can be parsed as expressions
	return v.validateExpression(rule.Conditions)
}

func (v *Validator) validateExpression(expr map[string]interface{}) error {
	// Check for logical operators
	if all, ok := expr["all"]; ok {
		allSlice, ok := all.([]interface{})
		if !ok {
			return fmt.Errorf("'all' must be an array")
		}
		for i, item := range allSlice {
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				return fmt.Errorf("'all[%d]' must be an object", i)
			}
			if err := v.validateExpression(itemMap); err != nil {
				return fmt.Errorf("all[%d]: %w", i, err)
			}
		}
	}

	if any, ok := expr["any"]; ok {
		anySlice, ok := any.([]interface{})
		if !ok {
			return fmt.Errorf("'any' must be an array")
		}
		for i, item := range anySlice {
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				return fmt.Errorf("'any[%d]' must be an object", i)
			}
			if err := v.validateExpression(itemMap); err != nil {
				return fmt.Errorf("any[%d]: %w", i, err)
			}
		}
	}

	if not, ok := expr["not"]; ok {
		notMap, ok := not.(map[string]interface{})
		if !ok {
			return fmt.Errorf("'not' must be an object")
		}
		if err := v.validateExpression(notMap); err != nil {
			return fmt.Errorf("not: %w", err)
		}
	}

	// Validate field operations
	if field, ok := expr["field"]; ok {
		if _, ok := field.(string); !ok {
			return fmt.Errorf("'field' must be a string")
		}

		if op, ok := expr["op"]; ok {
			opStr, ok := op.(string)
			if !ok {
				return fmt.Errorf("'op' must be a string")
			}
			if !isValidOperator(Operator(opStr)) {
				return fmt.Errorf("invalid operator: %s", opStr)
			}
		}
	}

	return nil
}

func isValidIdentifier(s string) bool {
	if s == "" {
		return false
	}
	matched, _ := regexp.MatchString(`^[a-zA-Z][a-zA-Z0-9_-]*$`, s)
	return matched
}

func isValidOperator(op Operator) bool {
	validOps := map[Operator]bool{
		OpEquals:      true,
		OpNotEquals:   true,
		OpGreaterThan: true,
		OpGreaterEq:   true,
		OpLessThan:    true,
		OpLessEq:      true,
		OpContains:    true,
		OpStartsWith:  true,
		OpEndsWith:    true,
		OpMatches:     true,
		OpIn:          true,
		OpNotIn:       true,
	}
	return validOps[op]
}

// ValidateWarnings returns non-fatal warnings about the policy.
func (v *Validator) ValidateWarnings(def *PolicyDefinition) []string {
	var warnings []string

	// Check for potentially conflicting rules
	toolRules := make(map[string][]string)
	for _, rule := range def.Rules {
		if rule.Type == RuleTypeCapability {
			if tool, ok := rule.Conditions["tool"].(string); ok {
				toolRules[tool] = append(toolRules[tool], rule.ID)
			}
		}
	}

	for tool, ruleIDs := range toolRules {
		if len(ruleIDs) > 1 {
			warnings = append(warnings,
				fmt.Sprintf("multiple capability rules for tool '%s': %s",
					tool, strings.Join(ruleIDs, ", ")))
		}
	}

	return warnings
}
