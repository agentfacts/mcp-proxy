package compiler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/open-policy-agent/opa/rego"
)

// Compiler compiles JSON policy definitions to Rego.
type Compiler struct {
	validator *Validator
}

// NewCompiler creates a new policy compiler.
func NewCompiler() *Compiler {
	return &Compiler{
		validator: NewValidator(),
	}
}

// Compile converts a JSON policy definition to Rego modules.
func (c *Compiler) Compile(def *PolicyDefinition) (*CompileResult, error) {
	// Validate the policy definition
	if err := c.validator.Validate(def); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	result := &CompileResult{
		Modules: make(map[string]string),
		Source:  def,
	}

	// Collect warnings
	result.Warnings = c.validator.ValidateWarnings(def)

	// Group rules by type
	grouped := c.groupRulesByType(def.Rules)

	// Build combined module
	var moduleBuilder strings.Builder

	// Add header
	header, err := RenderHeader(TemplateData{
		PolicyName:  def.Name,
		Description: def.Description,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return nil, fmt.Errorf("render header: %w", err)
	}
	moduleBuilder.WriteString(header)

	// Add helper functions
	moduleBuilder.WriteString(helperFunctions)

	// Compile each rule type
	if rules, ok := grouped[RuleTypeCapability]; ok {
		content, warnings, err := CompileCapabilityRules(rules, def.Name)
		if err != nil {
			return nil, fmt.Errorf("compile capability rules: %w", err)
		}
		moduleBuilder.WriteString(content)
		result.Warnings = append(result.Warnings, warnings...)
	}

	if rules, ok := grouped[RuleTypeBlocklist]; ok {
		content, warnings, err := CompileBlocklistRules(rules, def.Name)
		if err != nil {
			return nil, fmt.Errorf("compile blocklist rules: %w", err)
		}
		moduleBuilder.WriteString(content)
		result.Warnings = append(result.Warnings, warnings...)
	}

	if rules, ok := grouped[RuleTypeRateLimit]; ok {
		defaultLimit := def.Defaults.RateLimit
		if defaultLimit == 0 {
			defaultLimit = 1000
		}
		content, warnings, err := CompileRateLimitRules(rules, def.Name, defaultLimit)
		if err != nil {
			return nil, fmt.Errorf("compile rate limit rules: %w", err)
		}
		moduleBuilder.WriteString(content)
		result.Warnings = append(result.Warnings, warnings...)
	}

	if rules, ok := grouped[RuleTypeCustom]; ok {
		content, warnings, err := CompileCustomRules(rules, def.Name)
		if err != nil {
			return nil, fmt.Errorf("compile custom rules: %w", err)
		}
		moduleBuilder.WriteString(content)
		result.Warnings = append(result.Warnings, warnings...)
	}

	moduleName := fmt.Sprintf("json_%s.rego", sanitizeRuleID(def.Name))
	result.Modules[moduleName] = moduleBuilder.String()

	// Validate generated Rego compiles
	if err := c.validateGeneratedRego(result.Modules); err != nil {
		return nil, fmt.Errorf("generated Rego validation failed: %w", err)
	}

	return result, nil
}

func (c *Compiler) groupRulesByType(rules []RuleDefinition) map[RuleType][]RuleDefinition {
	grouped := make(map[RuleType][]RuleDefinition)
	for _, rule := range rules {
		if rule.IsEnabled() {
			grouped[rule.Type] = append(grouped[rule.Type], rule)
		}
	}
	return grouped
}

// validateGeneratedRego ensures the generated Rego is syntactically valid.
func (c *Compiler) validateGeneratedRego(modules map[string]string) error {
	opts := []func(*rego.Rego){
		rego.Query("data.mcp.policy"),
	}

	for name, content := range modules {
		opts = append(opts, rego.Module(name, content))
	}

	r := rego.New(opts...)
	_, err := r.PrepareForEval(context.Background())
	if err != nil {
		// Include generated Rego in error for debugging
		for name, content := range modules {
			return fmt.Errorf("module %s: %w\n\nGenerated Rego:\n%s", name, err, content)
		}
	}
	return err
}

// Helper functions included in all generated modules.
const helperFunctions = `
# Helper: Check if granted capability matches required capability
capability_matches(granted, required) if {
    granted == required
}

capability_matches(granted, required) if {
    granted == "*"
}

capability_matches(granted, required) if {
    endswith(granted, ":*")
    prefix := trim_suffix(granted, "*")
    startswith(required, prefix)
}

# Default rules (can be overridden by policy rules)
default capability_check := true
default blocked := false
default rate_limit_ok := true
`
