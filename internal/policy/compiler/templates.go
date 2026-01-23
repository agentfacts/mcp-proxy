package compiler

import (
	"bytes"
	"fmt"
	"text/template"
)

// Templates for Rego code generation.
var templates *template.Template

//nolint:gochecknoinits // template initialization is idiomatic with init
func init() {
	templates = template.New("rego").Funcs(template.FuncMap{
		"quote":      quoteString,
		"quoteSlice": quoteSlice,
		"join":       joinStrings,
	})

	template.Must(templates.New("header").Parse(headerTemplate))
	template.Must(templates.New("capability").Parse(capabilityTemplate))
	template.Must(templates.New("blocklist").Parse(blocklistTemplate))
	template.Must(templates.New("ratelimit").Parse(rateLimitTemplate))
	template.Must(templates.New("custom").Parse(customTemplate))
}

func quoteString(s string) string {
	return fmt.Sprintf("%q", s)
}

func quoteSlice(items []string) string {
	var buf bytes.Buffer
	buf.WriteString("[")
	for i, item := range items {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(fmt.Sprintf("%q", item))
	}
	buf.WriteString("]")
	return buf.String()
}

func joinStrings(sep string, items []string) string {
	var buf bytes.Buffer
	for i, item := range items {
		if i > 0 {
			buf.WriteString(sep)
		}
		buf.WriteString(item)
	}
	return buf.String()
}

const headerTemplate = `# Auto-generated from JSON policy: {{.PolicyName}}
# Description: {{.Description}}
# Generated at: {{.Timestamp}}
# DO NOT EDIT - changes will be overwritten

package mcp.policy

import rego.v1
`

const capabilityTemplate = `
# Rule: {{.RuleID}} (capability)
# Tool: {{.Tool}} requires capability: {{.Capability}}

{{.RuleID}}_check if {
    input.request.tool == {{quote .Tool}}
    required := {{quote .Capability}}
    some cap in input.agent.capabilities
    capability_matches(cap, required)
}

violations[msg] if {
    input.request.tool == {{quote .Tool}}
    not {{.RuleID}}_check
    msg := {{quote .Message}}
}
`

const blocklistTemplate = `
# Rule: {{.RuleID}} (blocklist)
# Blocks {{.MatchType}}: {{.Values}}

{{.RuleID}}_blocked if {
    {{if eq .MatchType "tool"}}input.request.tool{{else if eq .MatchType "agent"}}input.agent.id{{else}}input.identity.did{{end}} in {{quoteSlice .Values}}
}

blocked if {
    {{.RuleID}}_blocked
}

violations[msg] if {
    {{.RuleID}}_blocked
    msg := {{quote .Message}}
}
`

const rateLimitTemplate = `
# Rule: {{.RuleID}} (rate_limit)
# Limit: {{.Limit}} per {{.Window}}

{{.RuleID}}_exceeded if {
    {{if .AgentPattern}}regex.match({{quote .AgentPattern}}, input.agent.id){{else if .AgentID}}input.agent.id == {{quote .AgentID}}{{else}}true{{end}}
    input.session.request_count >= {{.Limit}}
}

rate_limit_ok if {
    not {{.RuleID}}_exceeded
}

violations[msg] if {
    {{.RuleID}}_exceeded
    msg := {{quote .Message}}
}
`

const customTemplate = `
# Rule: {{.RuleID}} (custom)
# {{.Description}}

{{.RuleID}}_match if {
{{.Conditions}}
}

{{if eq .Action "deny"}}
violations[msg] if {
    {{.RuleID}}_match
    msg := {{quote .Message}}
}
{{else}}
allow if {
    {{.RuleID}}_match
}
{{end}}
`

// TemplateData provides data for template rendering.
type TemplateData struct {
	PolicyName  string
	Description string
	Timestamp   string
}

// CapabilityData provides data for capability rule templates.
type CapabilityData struct {
	RuleID     string
	Tool       string
	Capability string
	Message    string
}

// BlocklistData provides data for blocklist rule templates.
type BlocklistData struct {
	RuleID    string
	MatchType string
	Values    []string
	Message   string
}

// RateLimitData provides data for rate limit rule templates.
type RateLimitData struct {
	RuleID       string
	AgentID      string
	AgentPattern string
	Limit        int
	Window       string
	Message      string
}

// CustomData provides data for custom rule templates.
type CustomData struct {
	RuleID      string
	Description string
	Conditions  string
	Action      Action
	Message     string
}

// RenderHeader renders the Rego file header.
func RenderHeader(data TemplateData) (string, error) {
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, "header", data); err != nil {
		return "", fmt.Errorf("render header: %w", err)
	}
	return buf.String(), nil
}

// RenderCapability renders a capability rule.
func RenderCapability(data CapabilityData) (string, error) {
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, "capability", data); err != nil {
		return "", fmt.Errorf("render capability: %w", err)
	}
	return buf.String(), nil
}

// RenderBlocklist renders a blocklist rule.
func RenderBlocklist(data BlocklistData) (string, error) {
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, "blocklist", data); err != nil {
		return "", fmt.Errorf("render blocklist: %w", err)
	}
	return buf.String(), nil
}

// RenderRateLimit renders a rate limit rule.
func RenderRateLimit(data RateLimitData) (string, error) {
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, "ratelimit", data); err != nil {
		return "", fmt.Errorf("render ratelimit: %w", err)
	}
	return buf.String(), nil
}

// RenderCustom renders a custom rule.
func RenderCustom(data CustomData) (string, error) {
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, "custom", data); err != nil {
		return "", fmt.Errorf("render custom: %w", err)
	}
	return buf.String(), nil
}
