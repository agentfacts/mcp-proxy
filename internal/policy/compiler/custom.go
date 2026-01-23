package compiler

import (
	"fmt"
	"strings"
)

// CompileCustomRules compiles custom rules to Rego.
func CompileCustomRules(rules []RuleDefinition, policyName string) (string, []string, error) {
	var warnings []string
	var builder strings.Builder

	exprCompiler := NewExpressionCompiler()

	for _, rule := range rules {
		if !rule.IsEnabled() {
			continue
		}

		// Compile conditions to Rego
		conditions, err := exprCompiler.Compile(rule.Conditions)
		if err != nil {
			return "", nil, fmt.Errorf("rule %s: %w", rule.ID, err)
		}

		message := rule.Message
		if message == "" {
			message = fmt.Sprintf("Custom policy rule '%s' matched", rule.ID)
		}

		description := ""
		if len(rule.Conditions) > 0 {
			description = fmt.Sprintf("Conditions: %v", rule.Conditions)
		}

		data := CustomData{
			RuleID:      sanitizeRuleID(rule.ID),
			Description: description,
			Conditions:  conditions,
			Action:      rule.Action,
			Message:     message,
		}

		rendered, err := RenderCustom(data)
		if err != nil {
			return "", nil, fmt.Errorf("rule %s: %w", rule.ID, err)
		}

		builder.WriteString(rendered)
		builder.WriteString("\n")
	}

	return builder.String(), warnings, nil
}
