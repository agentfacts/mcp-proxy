package compiler

import (
	"fmt"
	"strings"
)

// CompileRateLimitRules compiles rate limit rules to Rego.
func CompileRateLimitRules(rules []RuleDefinition, policyName string, defaultLimit int) (string, []string, error) {
	var warnings []string
	var builder strings.Builder

	for _, rule := range rules {
		if !rule.IsEnabled() {
			continue
		}

		limit, err := toInt(rule.Conditions["limit"])
		if err != nil {
			return "", nil, fmt.Errorf("rule %s: %w", rule.ID, err)
		}

		agentID, _ := rule.Conditions["agent_id"].(string)
		agentPattern, _ := rule.Conditions["agent_pattern"].(string)

		window := "session"
		if w, ok := rule.Conditions["window"].(string); ok {
			window = w
		}

		message := rule.Message
		if message == "" {
			message = fmt.Sprintf("Rate limit exceeded (%d per %s)", limit, window)
		}

		data := RateLimitData{
			RuleID:       sanitizeRuleID(rule.ID),
			AgentID:      agentID,
			AgentPattern: agentPattern,
			Limit:        limit,
			Window:       window,
			Message:      message,
		}

		rendered, err := RenderRateLimit(data)
		if err != nil {
			return "", nil, fmt.Errorf("rule %s: %w", rule.ID, err)
		}

		builder.WriteString(rendered)
		builder.WriteString("\n")
	}

	return builder.String(), warnings, nil
}

// toInt converts an interface{} to int.
func toInt(v interface{}) (int, error) {
	switch val := v.(type) {
	case int:
		return val, nil
	case int64:
		return int(val), nil
	case float64:
		return int(val), nil
	default:
		return 0, fmt.Errorf("'limit' must be a number")
	}
}
