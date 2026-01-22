# MCP Proxy - Capability Matching Policy
# Validates that agents have required capabilities for requested tools

package mcp.policy

import rego.v1

# Check if agent has required capability for the requested tool
default capability_check := false

# Get required capability for the requested tool
required_capability(tool) := cap if {
    cap := data.tool_capabilities[tool]
}

# No capability required if tool not in mapping (allow by default)
capability_check if {
    not required_capability(input.request.tool)
}

# Check if agent has the exact required capability
capability_check if {
    required := required_capability(input.request.tool)
    some cap in input.agent.capabilities
    capability_matches(cap, required)
}

# Direct match - granted capability equals required
capability_matches(granted, required) if {
    granted == required
}

# Wildcard match - "read:*" matches "read:customers"
capability_matches(granted, required) if {
    endswith(granted, ":*")
    prefix := trim_suffix(granted, "*")
    startswith(required, prefix)
}

# Super admin wildcard - "*" matches everything
capability_matches(granted, _) if {
    granted == "*"
}

# Collect capability violations
violations[msg] if {
    required := required_capability(input.request.tool)
    not capability_check
    msg := sprintf("Agent '%s' lacks capability '%s' required for tool '%s'",
        [input.agent.id, required, input.request.tool])
}
