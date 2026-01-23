package compiler

import (
	"fmt"
	"strings"
)

// CompileBlocklistRules compiles blocklist rules to Rego.
func CompileBlocklistRules(rules []RuleDefinition, policyName string) (string, []string, error) {
	var warnings []string
	var builder strings.Builder

	for _, rule := range rules {
		if !rule.IsEnabled() {
			continue
		}

		matchType, ok := rule.Conditions["match_type"].(string)
		if !ok {
			return "", nil, fmt.Errorf("rule %s: 'match_type' must be a string", rule.ID)
		}

		valuesRaw, ok := rule.Conditions["values"]
		if !ok {
			return "", nil, fmt.Errorf("rule %s: 'values' is required", rule.ID)
		}

		values, err := toStringSlice(valuesRaw)
		if err != nil {
			return "", nil, fmt.Errorf("rule %s: %w", rule.ID, err)
		}

		message := rule.Message
		if message == "" {
			message = fmt.Sprintf("%s is blocked by policy", matchType)
		}

		data := BlocklistData{
			RuleID:    sanitizeRuleID(rule.ID),
			MatchType: matchType,
			Values:    values,
			Message:   message,
		}

		rendered, err := RenderBlocklist(data)
		if err != nil {
			return "", nil, fmt.Errorf("rule %s: %w", rule.ID, err)
		}

		builder.WriteString(rendered)
		builder.WriteString("\n")
	}

	return builder.String(), warnings, nil
}

// toStringSlice converts an interface{} to []string.
func toStringSlice(v interface{}) ([]string, error) {
	switch val := v.(type) {
	case []string:
		return val, nil
	case []interface{}:
		result := make([]string, len(val))
		for i, item := range val {
			str, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("values[%d] must be a string", i)
			}
			result[i] = str
		}
		return result, nil
	default:
		return nil, fmt.Errorf("values must be an array of strings")
	}
}
