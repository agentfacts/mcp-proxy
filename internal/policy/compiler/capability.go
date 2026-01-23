package compiler

import (
	"fmt"
	"strings"
)

// CompileCapabilityRules compiles capability rules to Rego.
func CompileCapabilityRules(rules []RuleDefinition, policyName string) (string, []string, error) {
	var warnings []string
	var builder strings.Builder

	for _, rule := range rules {
		if !rule.IsEnabled() {
			continue
		}

		tool, ok := rule.Conditions["tool"].(string)
		if !ok {
			return "", nil, fmt.Errorf("rule %s: 'tool' must be a string", rule.ID)
		}

		capability, ok := rule.Conditions["requires_capability"].(string)
		if !ok {
			return "", nil, fmt.Errorf("rule %s: 'requires_capability' must be a string", rule.ID)
		}

		message := rule.Message
		if message == "" {
			message = fmt.Sprintf("Agent '%%s' lacks capability '%s' for tool '%s'",
				capability, tool)
		}

		// Replace placeholders in message
		message = replacePlaceholders(message, map[string]string{
			"agent.id":  "' + input.agent.id + '",
			"tool":      tool,
			"required":  capability,
		})

		data := CapabilityData{
			RuleID:     sanitizeRuleID(rule.ID),
			Tool:       tool,
			Capability: capability,
			Message:    message,
		}

		rendered, err := RenderCapability(data)
		if err != nil {
			return "", nil, fmt.Errorf("rule %s: %w", rule.ID, err)
		}

		builder.WriteString(rendered)
		builder.WriteString("\n")
	}

	return builder.String(), warnings, nil
}

// sanitizeRuleID converts a rule ID to a valid Rego identifier.
func sanitizeRuleID(id string) string {
	// Replace hyphens with underscores
	result := strings.ReplaceAll(id, "-", "_")
	// Remove any other invalid characters
	var builder strings.Builder
	for _, r := range result {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' {
			builder.WriteRune(r)
		} else if builder.Len() > 0 && r >= '0' && r <= '9' {
			// Only allow digits after we've started building (not at the start)
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

// replacePlaceholders replaces %{key} placeholders with values.
func replacePlaceholders(template string, values map[string]string) string {
	result := template
	for key, value := range values {
		placeholder := fmt.Sprintf("%%{%s}", key)
		result = strings.ReplaceAll(result, placeholder, value)
	}
	return result
}
