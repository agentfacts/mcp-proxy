package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Load reads and parses the configuration from a YAML file,
// then applies environment variable overrides.
func Load(path string) (*Config, error) {
	// Read configuration file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	// Parse YAML
	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	// Apply defaults
	applyDefaults(cfg)

	// Apply environment variable overrides
	applyEnvOverrides(cfg)

	// Validate configuration
	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return cfg, nil
}

// applyDefaults sets default values for configuration fields.
func applyDefaults(cfg *Config) {
	if cfg.Version == "" {
		cfg.Version = "1.0"
	}
	applyServerDefaults(&cfg.Server)
	applyUpstreamDefaults(&cfg.Upstream)
	applyAgentFactsDefaults(&cfg.AgentFacts)
	applyPolicyDefaults(&cfg.Policy)
	applyAuditDefaults(&cfg.Audit)
	applyMetricsDefaults(&cfg.Metrics)
	applyHealthDefaults(&cfg.Health)
	applyLoggingDefaults(&cfg.Logging)
	applyTLSDefaults(&cfg.TLS)
}

func applyServerDefaults(s *ServerConfig) {
	if s.Listen.Address == "" {
		s.Listen.Address = "0.0.0.0"
	}
	if s.Listen.Port == 0 {
		s.Listen.Port = 3000
	}
	if s.Transport == "" {
		s.Transport = "sse"
	}
	if s.ReadTimeout == 0 {
		s.ReadTimeout = 30 * time.Second
	}
	if s.WriteTimeout == 0 {
		s.WriteTimeout = 30 * time.Second
	}
	if s.IdleTimeout == 0 {
		s.IdleTimeout = 120 * time.Second
	}
	if s.GracefulShutdown == 0 {
		s.GracefulShutdown = 30 * time.Second
	}
	if s.MaxConnections == 0 {
		s.MaxConnections = 1000
	}
	s.Security.EnableSecurityHeaders = true
}

func applyUpstreamDefaults(u *UpstreamConfig) {
	if u.Transport == "" {
		u.Transport = "sse"
	}
	if u.Timeout == 0 {
		u.Timeout = 30 * time.Second
	}
	if u.ConnectionPool.MaxIdle == 0 {
		u.ConnectionPool.MaxIdle = 10
	}
	if u.ConnectionPool.MaxOpen == 0 {
		u.ConnectionPool.MaxOpen = 100
	}
	if u.ConnectionPool.IdleTimeout == 0 {
		u.ConnectionPool.IdleTimeout = 90 * time.Second
	}
	if u.Retry.MaxAttempts == 0 {
		u.Retry.MaxAttempts = 3
	}
	if u.Retry.InitialDelay == 0 {
		u.Retry.InitialDelay = 100 * time.Millisecond
	}
	if u.Retry.MaxDelay == 0 {
		u.Retry.MaxDelay = 5 * time.Second
	}
	if u.Retry.Backoff == "" {
		u.Retry.Backoff = "exponential"
	}
	if u.CircuitBreaker.Threshold == 0 {
		u.CircuitBreaker.Threshold = 5
	}
	if u.CircuitBreaker.Timeout == 0 {
		u.CircuitBreaker.Timeout = 30 * time.Second
	}
}

func applyAgentFactsDefaults(af *AgentFactsConfig) {
	if af.Mode == "" {
		af.Mode = "optional"
	}
	if af.MaxAge == 0 {
		af.MaxAge = 24 * time.Hour
	}
	if af.ClockSkew == 0 {
		af.ClockSkew = 5 * time.Minute
	}
	if af.Cache.TTL == 0 {
		af.Cache.TTL = 5 * time.Minute
	}
	if af.Cache.MaxEntries == 0 {
		af.Cache.MaxEntries = 1000
	}
}

func applyPolicyDefaults(p *PolicyConfig) {
	if p.Mode == "" {
		p.Mode = "enforce"
	}
	if p.PolicyDir == "" {
		p.PolicyDir = "policies"
	}
	if p.DataFile == "" {
		p.DataFile = "config/policy_data.json"
	}
	if p.Cache.TTL == 0 {
		p.Cache.TTL = 5 * time.Minute
	}
	if p.Cache.MaxEntries == 0 {
		p.Cache.MaxEntries = 10000
	}
	if p.Evaluation.Timeout == 0 {
		p.Evaluation.Timeout = 100 * time.Millisecond
	}
}

func applyAuditDefaults(a *AuditConfig) {
	if a.DBPath == "" {
		a.DBPath = "audit.db"
	}
	if a.BufferSize == 0 {
		a.BufferSize = 100
	}
	if a.FlushInterval == 0 {
		a.FlushInterval = time.Second
	}
	if a.RetentionDays == 0 {
		a.RetentionDays = 30
	}
}

func applyMetricsDefaults(m *MetricsConfig) {
	if m.Address == "" {
		m.Address = "0.0.0.0"
	}
	if m.Port == 0 {
		m.Port = 9090
	}
	if m.Path == "" {
		m.Path = "/metrics"
	}
}

func applyHealthDefaults(h *HealthConfig) {
	if h.Address == "" {
		h.Address = "0.0.0.0"
	}
	if h.Port == 0 {
		h.Port = 8080
	}
	if h.LivenessPath == "" {
		h.LivenessPath = "/health"
	}
	if h.ReadinessPath == "" {
		h.ReadinessPath = "/ready"
	}
}

func applyLoggingDefaults(l *LoggingConfig) {
	if l.Level == "" {
		l.Level = "info"
	}
	if l.Format == "" {
		l.Format = "json"
	}
	if l.Output == "" {
		l.Output = "stdout"
	}
}

func applyTLSDefaults(t *TLSConfig) {
	if t.MinVersion == "" {
		t.MinVersion = "1.2"
	}
	if t.ClientAuth == "" {
		t.ClientAuth = "none"
	}
}

// applyEnvOverrides applies environment variable overrides to the configuration.
// Environment variables use the format MCP_<SECTION>_<KEY> (uppercase, underscores).
func applyEnvOverrides(cfg *Config) {
	envMappings := map[string]func(string){
		"MCP_SERVER_PORT":      func(v string) { cfg.Server.Listen.Port = parseInt(v, cfg.Server.Listen.Port) },
		"MCP_SERVER_ADDRESS":   func(v string) { cfg.Server.Listen.Address = v },
		"MCP_SERVER_TRANSPORT": func(v string) { cfg.Server.Transport = v },
		"MCP_UPSTREAM_URL":     func(v string) { cfg.Upstream.URL = v },
		"MCP_AGENT_ID":         func(v string) { cfg.Agent.ID = v },
		"MCP_AGENT_NAME":       func(v string) { cfg.Agent.Name = v },
		"MCP_AGENTFACTS_MODE":  func(v string) { cfg.AgentFacts.Mode = v },
		"MCP_POLICY_MODE":      func(v string) { cfg.Policy.Mode = v },
		"MCP_POLICY_RULES_DIR": func(v string) { cfg.Policy.PolicyDir = v },
		"MCP_POLICY_DATA_FILE": func(v string) { cfg.Policy.DataFile = v },
		"MCP_AUDIT_ENABLED":    func(v string) { cfg.Audit.Enabled = parseBool(v) },
		"MCP_AUDIT_DB_PATH":    func(v string) { cfg.Audit.DBPath = v },
		"MCP_METRICS_ENABLED":  func(v string) { cfg.Metrics.Enabled = parseBool(v) },
		"MCP_METRICS_PORT":     func(v string) { cfg.Metrics.Port = parseInt(v, cfg.Metrics.Port) },
		"MCP_HEALTH_ENABLED":   func(v string) { cfg.Health.Enabled = parseBool(v) },
		"MCP_HEALTH_PORT":      func(v string) { cfg.Health.Port = parseInt(v, cfg.Health.Port) },
		"MCP_LOGGING_LEVEL":    func(v string) { cfg.Logging.Level = v },
		"MCP_LOGGING_FORMAT":   func(v string) { cfg.Logging.Format = v },
		"MCP_TLS_ENABLED":      func(v string) { cfg.TLS.Enabled = parseBool(v) },
		"MCP_TLS_CERT_FILE":    func(v string) { cfg.TLS.CertFile = v },
		"MCP_TLS_KEY_FILE":     func(v string) { cfg.TLS.KeyFile = v },
	}

	for env, setter := range envMappings {
		if value := os.Getenv(env); value != "" {
			setter(value)
		}
	}

	// Handle capabilities as comma-separated list
	if caps := os.Getenv("MCP_AGENT_CAPABILITIES"); caps != "" {
		cfg.Agent.Capabilities = strings.Split(caps, ",")
	}

	// Handle allowed DIDs as comma-separated list
	if dids := os.Getenv("MCP_AGENTFACTS_ALLOWED_DIDS"); dids != "" {
		cfg.AgentFacts.AllowedDIDs = strings.Split(dids, ",")
	}
}

// validate checks the configuration for errors.
func validate(cfg *Config) error {
	// Server validation
	if cfg.Server.Listen.Port < 1 || cfg.Server.Listen.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", cfg.Server.Listen.Port)
	}

	validTransports := map[string]bool{"sse": true, "stdio": true, "http": true}
	if !validTransports[cfg.Server.Transport] {
		return fmt.Errorf("invalid server transport: %s (must be sse, stdio, or http)", cfg.Server.Transport)
	}

	// AgentFacts mode validation
	validModes := map[string]bool{"disabled": true, "optional": true, "required": true}
	if !validModes[cfg.AgentFacts.Mode] {
		return fmt.Errorf("invalid agentfacts mode: %s (must be disabled, optional, or required)", cfg.AgentFacts.Mode)
	}

	// Policy mode validation
	validPolicyModes := map[string]bool{"audit": true, "enforce": true}
	if !validPolicyModes[cfg.Policy.Mode] {
		return fmt.Errorf("invalid policy mode: %s (must be audit or enforce)", cfg.Policy.Mode)
	}

	// Logging level validation
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[cfg.Logging.Level] {
		return fmt.Errorf("invalid logging level: %s (must be debug, info, warn, or error)", cfg.Logging.Level)
	}

	return nil
}

// parseInt parses a string to int, returning defaultVal on error.
func parseInt(s string, defaultVal int) int {
	if v, err := strconv.Atoi(s); err == nil {
		return v
	}
	return defaultVal
}

// parseBool parses a string to bool.
func parseBool(s string) bool {
	s = strings.ToLower(s)
	return s == "true" || s == "1" || s == "yes"
}

// String returns a string representation of the config for logging.
// Sensitive values are masked.
func (c *Config) String() string {
	return fmt.Sprintf("Config{version=%s, server=%s:%d, transport=%s, policy_mode=%s}",
		c.Version, c.Server.Listen.Address, c.Server.Listen.Port, c.Server.Transport, c.Policy.Mode)
}

// MaskSensitive returns a copy of the config with sensitive values masked.
func (c *Config) MaskSensitive() *Config {
	masked := *c
	if masked.TLS.KeyFile != "" {
		masked.TLS.KeyFile = "****"
	}
	return &masked
}

// GetEnvMapping returns a map of configuration paths to environment variable names.
func GetEnvMapping() map[string]string {
	return map[string]string{
		"server.port":             "MCP_SERVER_PORT",
		"server.address":          "MCP_SERVER_ADDRESS",
		"server.transport":        "MCP_SERVER_TRANSPORT",
		"upstream.url":            "MCP_UPSTREAM_URL",
		"agent.id":                "MCP_AGENT_ID",
		"agent.name":              "MCP_AGENT_NAME",
		"agent.capabilities":      "MCP_AGENT_CAPABILITIES",
		"agentfacts.mode":         "MCP_AGENTFACTS_MODE",
		"agentfacts.allowed_dids": "MCP_AGENTFACTS_ALLOWED_DIDS",
		"policy.mode":             "MCP_POLICY_MODE",
		"policy.rules_dir":        "MCP_POLICY_RULES_DIR",
		"policy.data_file":        "MCP_POLICY_DATA_FILE",
		"audit.enabled":           "MCP_AUDIT_ENABLED",
		"audit.db_path":           "MCP_AUDIT_DB_PATH",
		"metrics.enabled":         "MCP_METRICS_ENABLED",
		"metrics.port":            "MCP_METRICS_PORT",
		"health.enabled":          "MCP_HEALTH_ENABLED",
		"health.port":             "MCP_HEALTH_PORT",
		"logging.level":           "MCP_LOGGING_LEVEL",
		"logging.format":          "MCP_LOGGING_FORMAT",
		"tls.enabled":             "MCP_TLS_ENABLED",
		"tls.cert_file":           "MCP_TLS_CERT_FILE",
		"tls.key_file":            "MCP_TLS_KEY_FILE",
	}
}

// Unused but required for the reflect import
var _ = reflect.TypeOf(Config{})
