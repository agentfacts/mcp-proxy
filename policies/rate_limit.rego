# MCP Proxy - Rate Limiting Policy
# Enforces per-agent rate limits based on session request counts

package mcp.policy

import rego.v1

# Default to allowing if rate limit check passes
default rate_limit_ok := true

# Get rate limit for specific agent
get_rate_limit(agent_id) := limit if {
    limit := data.rate_limits[agent_id]
}

# Fall back to default rate limit
get_rate_limit(agent_id) := limit if {
    not data.rate_limits[agent_id]
    limit := data.rate_limits["default"]
}

# Final fallback if no default configured
get_rate_limit(agent_id) := 1000 if {
    not data.rate_limits[agent_id]
    not data.rate_limits["default"]
}

# Check if within rate limit
rate_limit_ok if {
    limit := get_rate_limit(input.agent.id)
    input.session.request_count < limit
}

# Deny if over rate limit
rate_limit_ok := false if {
    limit := get_rate_limit(input.agent.id)
    input.session.request_count >= limit
}

# Add violation message when rate limit exceeded
violations[msg] if {
    not rate_limit_ok
    limit := get_rate_limit(input.agent.id)
    msg := sprintf("Agent '%s' exceeded rate limit (%d/%d requests in session)",
        [input.agent.id, input.session.request_count, limit])
}
