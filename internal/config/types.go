package config

import "time"

// Config is the root configuration structure for the MCP MCP Proxy.
type Config struct {
	Version    string           `yaml:"version"`
	Server     ServerConfig     `yaml:"server"`
	Upstream   UpstreamConfig   `yaml:"upstream"`
	Agent      AgentConfig      `yaml:"agent"`
	AgentFacts AgentFactsConfig `yaml:"agentfacts"`
	Policy     PolicyConfig     `yaml:"policy"`
	Audit      AuditConfig      `yaml:"audit"`
	Metrics    MetricsConfig    `yaml:"metrics"`
	Health     HealthConfig     `yaml:"health"`
	Logging    LoggingConfig    `yaml:"logging"`
	TLS        TLSConfig        `yaml:"tls"`
}

// ServerConfig defines the proxy server settings.
type ServerConfig struct {
	Listen           ListenConfig   `yaml:"listen"`
	Transport        string         `yaml:"transport"` // sse, stdio, http
	ReadTimeout      time.Duration  `yaml:"read_timeout"`
	WriteTimeout     time.Duration  `yaml:"write_timeout"`
	IdleTimeout      time.Duration  `yaml:"idle_timeout"`
	GracefulShutdown time.Duration  `yaml:"graceful_shutdown"`
	MaxConnections   int            `yaml:"max_connections"`
	Security         SecurityConfig `yaml:"security"`
}

// SecurityConfig defines security-related settings.
type SecurityConfig struct {
	// CORS settings
	CORSAllowedOrigins []string `yaml:"cors_allowed_origins"` // Empty = block all, ["*"] = allow all
	// Security headers
	EnableSecurityHeaders bool `yaml:"enable_security_headers"`
}

// ListenConfig defines the server listen address.
type ListenConfig struct {
	Address string `yaml:"address"`
	Port    int    `yaml:"port"`
}

// UpstreamConfig defines the upstream MCP server connection settings.
type UpstreamConfig struct {
	URL            string               `yaml:"url"`
	Transport      string               `yaml:"transport"`
	Timeout        time.Duration        `yaml:"timeout"`
	ConnectionPool ConnectionPoolConfig `yaml:"connection_pool"`
	Retry          RetryConfig          `yaml:"retry"`
	CircuitBreaker CircuitBreakerConfig `yaml:"circuit_breaker"`
}

// ConnectionPoolConfig defines connection pool settings.
type ConnectionPoolConfig struct {
	MaxIdle     int           `yaml:"max_idle"`
	MaxOpen     int           `yaml:"max_open"`
	IdleTimeout time.Duration `yaml:"idle_timeout"`
}

// RetryConfig defines retry behavior for upstream connections.
type RetryConfig struct {
	Enabled      bool          `yaml:"enabled"`
	MaxAttempts  int           `yaml:"max_attempts"`
	InitialDelay time.Duration `yaml:"initial_delay"`
	MaxDelay     time.Duration `yaml:"max_delay"`
	Backoff      string        `yaml:"backoff"` // exponential, linear, constant
}

// CircuitBreakerConfig defines circuit breaker settings.
type CircuitBreakerConfig struct {
	Enabled   bool          `yaml:"enabled"`
	Threshold int           `yaml:"threshold"`
	Timeout   time.Duration `yaml:"timeout"`
}

// AgentConfig defines the default agent identity (used when AgentFacts not provided).
type AgentConfig struct {
	ID           string   `yaml:"id"`
	Name         string   `yaml:"name"`
	Capabilities []string `yaml:"capabilities"`
	Model        string   `yaml:"model"`
	Publisher    string   `yaml:"publisher"`
	Tags         []string `yaml:"tags"`
}

// AgentFactsConfig defines AgentFacts verification settings.
type AgentFactsConfig struct {
	Mode           string        `yaml:"mode"` // disabled, optional, required
	MaxAge         time.Duration `yaml:"max_age"`
	ClockSkew      time.Duration `yaml:"clock_skew"`
	AllowedDIDs    []string      `yaml:"allowed_dids"`
	VerifyLogProof bool          `yaml:"verify_log_proof"`
	Cache          CacheConfig   `yaml:"cache"`
}

// PolicyConfig defines the OPA policy engine settings.
type PolicyConfig struct {
	Enabled         bool             `yaml:"enabled"`
	Mode            string           `yaml:"mode"` // audit, enforce
	PolicyDir       string           `yaml:"policy_dir"`
	DataFile        string           `yaml:"data_file"`
	WatchForChanges bool             `yaml:"watch_for_changes"`
	Environment     string           `yaml:"environment"` // development, staging, production
	Cache           CacheConfig      `yaml:"cache"`
	Evaluation      EvaluationConfig `yaml:"evaluation"`
}

// EvaluationConfig defines policy evaluation settings.
type EvaluationConfig struct {
	Timeout             time.Duration `yaml:"timeout"`
	StrictBuiltinErrors bool          `yaml:"strict_builtin_errors"`
}

// CacheConfig defines caching settings.
type CacheConfig struct {
	Enabled    bool          `yaml:"enabled"`
	TTL        time.Duration `yaml:"ttl"`
	MaxEntries int           `yaml:"max_entries"`
}

// AuditConfig defines audit logging settings.
type AuditConfig struct {
	Enabled       bool          `yaml:"enabled"`
	DBPath        string        `yaml:"db_path"`        // SQLite database path
	BufferSize    int           `yaml:"buffer_size"`    // Max records to buffer
	FlushInterval time.Duration `yaml:"flush_interval"` // How often to flush
	RetentionDays int           `yaml:"retention_days"` // Days to keep records (0 = forever)
	Capture       CaptureConfig `yaml:"capture"`
}

// CaptureConfig defines what to capture in audit logs.
type CaptureConfig struct {
	RequestArguments bool `yaml:"request_arguments"`
	ResponseSummary  bool `yaml:"response_summary"`
}

// MetricsConfig defines Prometheus metrics settings.
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Address string `yaml:"address"`
	Port    int    `yaml:"port"`
	Path    string `yaml:"path"`
}

// HealthConfig defines health check endpoint settings.
type HealthConfig struct {
	Enabled       bool   `yaml:"enabled"`
	Address       string `yaml:"address"`
	Port          int    `yaml:"port"`
	LivenessPath  string `yaml:"liveness_path"`
	ReadinessPath string `yaml:"readiness_path"`
}

// LoggingConfig defines logging settings.
type LoggingConfig struct {
	Level  string     `yaml:"level"`  // debug, info, warn, error
	Format string     `yaml:"format"` // json, text
	Output string     `yaml:"output"` // stdout, stderr, file
	File   FileConfig `yaml:"file"`
}

// FileConfig defines log file settings.
type FileConfig struct {
	Path       string `yaml:"path"`
	MaxSize    int    `yaml:"max_size"`    // MB
	MaxBackups int    `yaml:"max_backups"`
	MaxAge     int    `yaml:"max_age"`     // days
}

// TLSConfig defines TLS settings.
type TLSConfig struct {
	Enabled    bool   `yaml:"enabled"`
	CertFile   string `yaml:"cert_file"`
	KeyFile    string `yaml:"key_file"`
	CAFile     string `yaml:"ca_file"`
	MinVersion string `yaml:"min_version"`
	ClientAuth string `yaml:"client_auth"` // none, request, require
}
