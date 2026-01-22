# MCP Proxy - Main Policy Entry Point
# This file serves as the entry point for policy evaluation

package mcp.policy

import rego.v1

# Default deny - explicit allow required
default allow := false

# Main decision object returned to the proxy
decision := {
    "allow": allow,
    "violations": violations,
    "matched_rule": matched_rule,
}

# Allow if all checks pass
allow if {
    capability_check
    rate_limit_ok
    not blocked
}

# Determine which rule matched for logging
# Use else chain to ensure exactly one rule matches
matched_rule := "blocked" if {
    blocked
} else := "rate_limit_exceeded" if {
    not rate_limit_ok
} else := "missing_capability" if {
    not capability_check
} else := "allowed" if {
    allow
} else := "default_deny"
