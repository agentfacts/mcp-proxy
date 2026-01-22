# MCP Proxy - Capability Policy Tests
# Run with: opa test policies/ -v

package mcp.policy

import rego.v1

# Test: Allow when agent has exact capability match
test_allow_exact_capability_match if {
    allow with input as {
        "agent": {"id": "test-agent", "capabilities": ["read:customers"]},
        "request": {"tool": "customer_lookup"},
        "session": {"request_count": 0},
        "identity": {"verified": false}
    } with data.tool_capabilities as {"customer_lookup": "read:customers"}
      with data.rate_limits as {"default": 1000}
      with data.blocked_tools as []
      with data.blocked_agents as []
      with data.blocked_dids as []
}

# Test: Allow when agent has wildcard capability
test_allow_wildcard_capability_match if {
    allow with input as {
        "agent": {"id": "test-agent", "capabilities": ["read:*"]},
        "request": {"tool": "customer_lookup"},
        "session": {"request_count": 0},
        "identity": {"verified": false}
    } with data.tool_capabilities as {"customer_lookup": "read:customers"}
      with data.rate_limits as {"default": 1000}
      with data.blocked_tools as []
      with data.blocked_agents as []
      with data.blocked_dids as []
}

# Test: Allow with super admin wildcard
test_allow_super_admin_wildcard if {
    allow with input as {
        "agent": {"id": "admin-agent", "capabilities": ["*"]},
        "request": {"tool": "admin_panel"},
        "session": {"request_count": 0},
        "identity": {"verified": false}
    } with data.tool_capabilities as {"admin_panel": "admin:*"}
      with data.rate_limits as {"default": 1000}
      with data.blocked_tools as []
      with data.blocked_agents as []
      with data.blocked_dids as []
}

# Test: Deny when agent lacks required capability
test_deny_missing_capability if {
    not allow with input as {
        "agent": {"id": "test-agent", "capabilities": ["read:tickets"]},
        "request": {"tool": "customer_lookup"},
        "session": {"request_count": 0},
        "identity": {"verified": false}
    } with data.tool_capabilities as {"customer_lookup": "read:customers"}
      with data.rate_limits as {"default": 1000}
      with data.blocked_tools as []
      with data.blocked_agents as []
      with data.blocked_dids as []
}

# Test: Deny when agent is blocked
test_deny_blocked_agent if {
    not allow with input as {
        "agent": {"id": "blocked-agent", "capabilities": ["read:customers"]},
        "request": {"tool": "customer_lookup"},
        "session": {"request_count": 0},
        "identity": {"verified": false}
    } with data.tool_capabilities as {"customer_lookup": "read:customers"}
      with data.rate_limits as {"default": 1000}
      with data.blocked_tools as []
      with data.blocked_agents as ["blocked-agent"]
      with data.blocked_dids as []
}

# Test: Deny when tool is blocked
test_deny_blocked_tool if {
    not allow with input as {
        "agent": {"id": "test-agent", "capabilities": ["admin:*"]},
        "request": {"tool": "database_drop"},
        "session": {"request_count": 0},
        "identity": {"verified": false}
    } with data.tool_capabilities as {"database_drop": "admin:database"}
      with data.rate_limits as {"default": 1000}
      with data.blocked_tools as ["database_drop"]
      with data.blocked_agents as []
      with data.blocked_dids as []
}

# Test: Deny when rate limit exceeded
test_deny_rate_limit_exceeded if {
    not allow with input as {
        "agent": {"id": "test-agent", "capabilities": ["read:customers"]},
        "request": {"tool": "customer_lookup"},
        "session": {"request_count": 1000},
        "identity": {"verified": false}
    } with data.tool_capabilities as {"customer_lookup": "read:customers"}
      with data.rate_limits as {"default": 1000}
      with data.blocked_tools as []
      with data.blocked_agents as []
      with data.blocked_dids as []
}

# Test: Use agent-specific rate limit
test_agent_specific_rate_limit if {
    not allow with input as {
        "agent": {"id": "limited-agent", "capabilities": ["read:customers"]},
        "request": {"tool": "customer_lookup"},
        "session": {"request_count": 100},
        "identity": {"verified": false}
    } with data.tool_capabilities as {"customer_lookup": "read:customers"}
      with data.rate_limits as {"limited-agent": 100, "default": 1000}
      with data.blocked_tools as []
      with data.blocked_agents as []
      with data.blocked_dids as []
}

# Test: Allow tool not in capability mapping
test_allow_unmapped_tool if {
    allow with input as {
        "agent": {"id": "test-agent", "capabilities": []},
        "request": {"tool": "unknown_tool"},
        "session": {"request_count": 0},
        "identity": {"verified": false}
    } with data.tool_capabilities as {}
      with data.rate_limits as {"default": 1000}
      with data.blocked_tools as []
      with data.blocked_agents as []
      with data.blocked_dids as []
}

# Test: Violations populated on capability deny
test_violations_on_capability_deny if {
    v := violations with input as {
        "agent": {"id": "test-agent", "capabilities": []},
        "request": {"tool": "customer_lookup"},
        "session": {"request_count": 0},
        "identity": {"verified": false}
    } with data.tool_capabilities as {"customer_lookup": "read:customers"}
      with data.rate_limits as {"default": 1000}
      with data.blocked_tools as []
      with data.blocked_agents as []
      with data.blocked_dids as []

    count(v) > 0
    some msg in v
    contains(msg, "lacks capability")
}

# Test: Violations populated on rate limit exceed
test_violations_on_rate_limit if {
    v := violations with input as {
        "agent": {"id": "test-agent", "capabilities": ["read:customers"]},
        "request": {"tool": "customer_lookup"},
        "session": {"request_count": 1000},
        "identity": {"verified": false}
    } with data.tool_capabilities as {"customer_lookup": "read:customers"}
      with data.rate_limits as {"default": 1000}
      with data.blocked_tools as []
      with data.blocked_agents as []
      with data.blocked_dids as []

    count(v) > 0
    some msg in v
    contains(msg, "exceeded rate limit")
}

# Test: Block DID when identity verified
test_deny_blocked_did if {
    not allow with input as {
        "agent": {"id": "test-agent", "capabilities": ["read:customers"]},
        "request": {"tool": "customer_lookup"},
        "session": {"request_count": 0},
        "identity": {"verified": true, "did": "did:key:z6MkBadActor"}
    } with data.tool_capabilities as {"customer_lookup": "read:customers"}
      with data.rate_limits as {"default": 1000}
      with data.blocked_tools as []
      with data.blocked_agents as []
      with data.blocked_dids as ["did:key:z6MkBadActor"]
}
