-- SOTH MCP Proxy - PostgreSQL Schema
-- Version: 1.0
-- Description: Audit log schema with partitioning and indexes

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Create audit_log table
CREATE TABLE IF NOT EXISTS audit_log (
    -- Primary key
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Timestamps
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Request identification
    request_id VARCHAR(64) NOT NULL,
    session_id VARCHAR(64) NOT NULL,

    -- Agent identity
    agent_id VARCHAR(256) NOT NULL,
    agent_name VARCHAR(256),
    agent_did VARCHAR(512),
    agent_model VARCHAR(128),
    agent_publisher VARCHAR(512),
    identity_verified BOOLEAN DEFAULT FALSE,

    -- Request details
    mcp_method VARCHAR(64) NOT NULL,
    tool_name VARCHAR(256),
    tool_arguments JSONB,

    -- Policy decision
    decision VARCHAR(16) NOT NULL CHECK (decision IN ('allow', 'deny', 'error')),
    policy_mode VARCHAR(16) NOT NULL CHECK (policy_mode IN ('audit', 'enforce')),
    violations TEXT[],
    matched_rule VARCHAR(128),

    -- Response (if captured)
    response_summary JSONB,
    response_error TEXT,

    -- Performance metrics
    total_latency_ms DECIMAL(10,3),
    policy_eval_ms DECIMAL(10,3),
    upstream_latency_ms DECIMAL(10,3),

    -- Context
    source_ip INET,
    user_agent TEXT,
    environment VARCHAR(64),
    proxy_version VARCHAR(32),
    proxy_region VARCHAR(64),

    -- Integrity (computed on insert)
    entry_hash CHAR(64)
) PARTITION BY RANGE (timestamp);

-- Create partitions for current and next month
-- These should be created by a maintenance job in production
CREATE TABLE IF NOT EXISTS audit_log_2026_01 PARTITION OF audit_log
    FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');

CREATE TABLE IF NOT EXISTS audit_log_2026_02 PARTITION OF audit_log
    FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');

CREATE TABLE IF NOT EXISTS audit_log_2026_03 PARTITION OF audit_log
    FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');

-- Create default partition for any data outside defined ranges
CREATE TABLE IF NOT EXISTS audit_log_default PARTITION OF audit_log DEFAULT;

-- Indexes for common query patterns
CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_log (timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_audit_agent_id ON audit_log (agent_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_audit_decision ON audit_log (decision, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_audit_tool ON audit_log (tool_name, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_audit_session ON audit_log (session_id);
CREATE INDEX IF NOT EXISTS idx_audit_request ON audit_log (request_id);
CREATE INDEX IF NOT EXISTS idx_audit_violations ON audit_log USING GIN (violations);
CREATE INDEX IF NOT EXISTS idx_audit_agent_did ON audit_log (agent_did) WHERE agent_did IS NOT NULL;

-- Function to compute entry hash
CREATE OR REPLACE FUNCTION compute_audit_entry_hash()
RETURNS TRIGGER AS $$
BEGIN
    NEW.entry_hash := encode(
        digest(
            concat_ws('|',
                NEW.id::text,
                NEW.timestamp::text,
                NEW.request_id,
                NEW.session_id,
                NEW.agent_id,
                NEW.mcp_method,
                COALESCE(NEW.tool_name, ''),
                NEW.decision,
                NEW.policy_mode
            ),
            'sha256'
        ),
        'hex'
    );
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger to compute hash on insert
DROP TRIGGER IF EXISTS trg_audit_entry_hash ON audit_log;
CREATE TRIGGER trg_audit_entry_hash
    BEFORE INSERT ON audit_log
    FOR EACH ROW
    EXECUTE FUNCTION compute_audit_entry_hash();

-- View for recent activity summary
CREATE OR REPLACE VIEW audit_summary AS
SELECT
    date_trunc('hour', timestamp) AS hour,
    agent_id,
    COUNT(*) AS total_requests,
    COUNT(*) FILTER (WHERE decision = 'allow') AS allowed,
    COUNT(*) FILTER (WHERE decision = 'deny') AS denied,
    COUNT(*) FILTER (WHERE decision = 'error') AS errors,
    AVG(total_latency_ms)::DECIMAL(10,3) AS avg_latency_ms,
    MAX(total_latency_ms)::DECIMAL(10,3) AS max_latency_ms
FROM audit_log
WHERE timestamp > NOW() - INTERVAL '24 hours'
GROUP BY 1, 2
ORDER BY 1 DESC, 2;

-- View for policy violations
CREATE OR REPLACE VIEW policy_violations AS
SELECT
    timestamp,
    request_id,
    agent_id,
    agent_did,
    tool_name,
    violations,
    policy_mode
FROM audit_log
WHERE decision = 'deny'
    AND timestamp > NOW() - INTERVAL '7 days'
ORDER BY timestamp DESC;

-- Function to create future partitions (call monthly via cron)
CREATE OR REPLACE FUNCTION create_monthly_partition(year INT, month INT)
RETURNS VOID AS $$
DECLARE
    partition_name TEXT;
    start_date DATE;
    end_date DATE;
BEGIN
    partition_name := format('audit_log_%s_%s', year, lpad(month::text, 2, '0'));
    start_date := make_date(year, month, 1);
    end_date := start_date + INTERVAL '1 month';

    EXECUTE format(
        'CREATE TABLE IF NOT EXISTS %I PARTITION OF audit_log FOR VALUES FROM (%L) TO (%L)',
        partition_name,
        start_date,
        end_date
    );
END;
$$ LANGUAGE plpgsql;

-- Grant permissions (adjust as needed for your environment)
-- GRANT SELECT, INSERT ON audit_log TO soth_proxy;
-- GRANT SELECT ON audit_summary TO soth_dashboard;
-- GRANT SELECT ON policy_violations TO soth_dashboard;

-- Add comment for documentation
COMMENT ON TABLE audit_log IS 'SOTH MCP Proxy audit log - captures all agent-tool interactions with policy decisions';
