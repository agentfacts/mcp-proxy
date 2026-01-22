# MCP Proxy - Blocklist Policy
# Blocks specific agents, tools, and DIDs

package mcp.policy

import rego.v1

# Default to not blocked
default blocked := false

# Block if tool is in blocklist
blocked if {
    input.request.tool in data.blocked_tools
}

# Block if agent ID is in blocklist
blocked if {
    input.agent.id in data.blocked_agents
}

# Block if DID is in blocklist (when identity is verified)
blocked if {
    input.identity.verified
    input.identity.did in data.blocked_dids
}

# Violation message for blocked tool
violations[msg] if {
    input.request.tool in data.blocked_tools
    msg := sprintf("Tool '%s' is blocked by policy", [input.request.tool])
}

# Violation message for blocked agent
violations[msg] if {
    input.agent.id in data.blocked_agents
    msg := sprintf("Agent '%s' is blocked by policy", [input.agent.id])
}

# Violation message for blocked DID
violations[msg] if {
    input.identity.verified
    input.identity.did in data.blocked_dids
    msg := sprintf("DID '%s' is blocked by policy", [input.identity.did])
}
