package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/agentfacts/mcp-proxy/internal/audit"
	"github.com/agentfacts/mcp-proxy/internal/config"
	"github.com/agentfacts/mcp-proxy/internal/observability"
	"github.com/agentfacts/mcp-proxy/internal/policy"
	"github.com/agentfacts/mcp-proxy/internal/router"
	"github.com/agentfacts/mcp-proxy/internal/session"
	"github.com/agentfacts/mcp-proxy/internal/transport"
	"github.com/agentfacts/mcp-proxy/internal/transport/sse"
	"github.com/agentfacts/mcp-proxy/internal/transport/stdio"
	"github.com/agentfacts/mcp-proxy/internal/upstream"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	version   = "0.1.0"
	buildTime = "unknown"
	gitCommit = "unknown"
)

// Application holds all the components of the proxy.
type Application struct {
	cfg            *config.Config
	sessionManager *session.Manager
	router         *router.Router
	transport      transport.Transport
	upstreamClient *upstream.Client
	policyEngine   *policy.Engine
	auditStore     *audit.Store
	auditWriter    *audit.Writer

	// Observability
	metrics   *observability.Metrics
	health    *observability.Health
	obsServer *observability.Server
}

func main() {
	// Parse command line flags
	configPath := flag.String("config", "config/proxy.yaml", "Path to configuration file")
	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *showVersion {
		fmt.Printf("MCP Proxy\n")
		fmt.Printf("  Version:    %s\n", version)
		fmt.Printf("  Build Time: %s\n", buildTime)
		fmt.Printf("  Git Commit: %s\n", gitCommit)
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	initLogger(cfg.Logging)

	log.Info().
		Str("version", version).
		Str("config", *configPath).
		Msg("Starting MCP Proxy")

	// Create root context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize application
	app, err := newApplication(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize application")
	}

	// Start application
	if err := app.Start(ctx); err != nil {
		log.Fatal().Err(err).Msg("Failed to start application")
	}

	log.Info().
		Str("address", cfg.Server.Listen.Address).
		Int("port", cfg.Server.Listen.Port).
		Str("transport", cfg.Server.Transport).
		Str("upstream", cfg.Upstream.URL).
		Str("policy_mode", cfg.Policy.Mode).
		Msg("Proxy server ready")

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for shutdown signal
	sig := <-sigChan
	log.Info().Str("signal", sig.String()).Msg("Received shutdown signal")

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, cfg.Server.GracefulShutdown)
	defer shutdownCancel()

	// Perform graceful shutdown
	if err := app.Stop(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("Error during shutdown")
		os.Exit(1)
	}

	log.Info().Msg("Shutdown complete")
}

func newApplication(cfg *config.Config) (*Application, error) {
	app := &Application{
		cfg: cfg,
	}

	// Initialize session manager
	app.sessionManager = session.NewManager(session.ManagerConfig{
		SessionTTL:      2 * time.Hour,
		CleanupInterval: 1 * time.Minute,
		MaxSessions:     cfg.Server.MaxConnections,
	})

	// Initialize upstream client (if URL configured)
	if cfg.Upstream.URL != "" {
		app.upstreamClient = upstream.NewClient(cfg.Upstream)
	}

	// Initialize message router
	app.router = router.NewRouter()

	// Set up upstream sender for router
	app.router.SetUpstreamSender(func(ctx context.Context, message []byte) ([]byte, error) {
		if app.upstreamClient != nil && app.upstreamClient.IsConnected() {
			return app.upstreamClient.Send(ctx, message)
		}
		// No upstream - echo back for testing
		return message, nil
	})

	// Initialize audit store and writer (if enabled)
	if cfg.Audit.Enabled {
		var err error
		app.auditStore, err = audit.NewStore(audit.StoreConfig{
			DBPath: cfg.Audit.DBPath,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create audit store: %w", err)
		}

		app.auditWriter = audit.NewWriter(app.auditStore, audit.WriterConfig{
			BufferSize:    cfg.Audit.BufferSize,
			FlushInterval: cfg.Audit.FlushInterval,
		})
	}

	// Set up audit logger
	app.router.SetAuditLogger(func(ctx context.Context, sess *session.Session, reqCtx *router.RequestContext, decision *router.PolicyDecision, response []byte, latency time.Duration) {
		allowed := decision == nil || decision.Allow
		durationSeconds := latency.Seconds()

		// Record metrics
		tool := reqCtx.Tool
		if tool == "" {
			tool = "unknown"
		}
		app.metrics.RecordRequest(reqCtx.Method, tool, allowed, durationSeconds)

		if decision != nil {
			app.metrics.RecordPolicyDecision(allowed, decision.MatchedRule, decision.PolicyMode, durationSeconds)
		}

		// Always log to stdout
		log.Info().
			Str("request_id", reqCtx.RequestID).
			Str("session_id", sess.ID).
			Str("agent_id", sess.AgentID).
			Str("method", reqCtx.Method).
			Str("tool", reqCtx.Tool).
			Bool("allowed", allowed).
			Dur("latency", latency).
			Msg("Request processed")

		// Write to audit store if enabled
		if app.auditWriter != nil {
			// Build capabilities string
			capsJSON, _ := json.Marshal(sess.Capabilities)

			// Build arguments string if capture enabled
			var argsJSON string
			if cfg.Audit.Capture.RequestArguments && reqCtx.Arguments != nil {
				argsBytes, _ := json.Marshal(reqCtx.Arguments)
				argsJSON = string(argsBytes)
			}

			// Build violations string
			var violations string
			var matchedRule, policyMode string
			if decision != nil {
				if len(decision.Violations) > 0 {
					violations = strings.Join(decision.Violations, "; ")
				}
				matchedRule = decision.MatchedRule
				policyMode = decision.PolicyMode
			}

			record := audit.NewRecordBuilder().
				WithRequest(reqCtx.RequestID, sess.ID).
				WithTiming(float64(latency.Microseconds())/1000.0).
				WithAgent(sess.AgentID, sess.AgentName, string(capsJSON)).
				WithMethod(reqCtx.Method, reqCtx.Tool, reqCtx.ResourceURI, argsJSON).
				WithIdentity(sess.IdentityVerified, sess.DID).
				WithDecision(allowed, matchedRule, violations, policyMode).
				WithEnvironment(sess.SourceIP, cfg.Policy.Environment).
				Build()

			app.auditWriter.Write(record)
		}
	})

	// Initialize policy engine
	app.policyEngine = policy.NewEngine(policy.EngineConfig{
		Mode:    cfg.Policy.Mode,
		Enabled: cfg.Policy.Enabled,
		CacheConfig: policy.CacheConfig{
			Enabled:    true,
			TTL:        5 * time.Minute,
			MaxEntries: 10000,
		},
	})

	// Set up policy evaluator
	app.router.SetPolicyEvaluator(func(ctx context.Context, sess *session.Session, reqCtx *router.RequestContext) (*router.PolicyDecision, error) {
		// Build policy input
		input := policy.NewInputBuilder().
			WithAgent(sess.AgentID, sess.AgentID, sess.Capabilities).
			WithRequest(reqCtx.Method, reqCtx.Tool, reqCtx.Arguments).
			WithSession(sess.ID, sess.RequestCount, sess.CreatedAt).
			WithEnvironment(sess.SourceIP, cfg.Policy.Environment, cfg.Server.Listen.Address).
			Build()

		// Set agent details if available
		if cfg.Agent.ID != "" {
			input.Agent.Model = cfg.Agent.Model
			input.Agent.Publisher = cfg.Agent.Publisher
		}

		// Evaluate policy
		result, err := app.policyEngine.Evaluate(ctx, input)
		if err != nil {
			return nil, err
		}

		// Convert to router's PolicyDecision type
		return &router.PolicyDecision{
			Allow:       result.Decision.Allow,
			Violations:  result.Decision.Violations,
			MatchedRule: result.Decision.MatchedRule,
			PolicyMode:  result.PolicyMode,
		}, nil
	})

	// Initialize transport based on config
	switch cfg.Server.Transport {
	case "sse":
		app.transport = sse.NewServer(cfg.Server, cfg.Agent, app.sessionManager)
	case "stdio":
		stdioServer := stdio.NewServer(cfg.Agent, app.sessionManager)
		app.transport = stdioServer
	default:
		return nil, fmt.Errorf("unknown transport: %s", cfg.Server.Transport)
	}

	// Set up message handler to use router
	app.transport.SetMessageHandler(app.handleMessage)

	// Initialize observability
	app.metrics = observability.NewMetrics("mcp_proxy")
	app.health = observability.NewHealth(version)

	// Register health checkers
	if app.policyEngine != nil {
		app.health.RegisterChecker("policy_engine", observability.PolicyEngineChecker(func() bool {
			return app.policyEngine.IsReady()
		}))
	}
	if app.upstreamClient != nil {
		app.health.RegisterChecker("upstream", observability.UpstreamChecker(func() bool {
			return app.upstreamClient.IsConnected()
		}))
	}
	if app.auditStore != nil {
		app.health.RegisterChecker("audit_store", observability.DatabaseChecker(func(ctx context.Context) error {
			return app.auditStore.Ping(ctx)
		}))
	}

	// Create observability server
	app.obsServer = observability.NewServer(observability.ServerConfig{
		MetricsEnabled: cfg.Metrics.Enabled,
		MetricsAddress: cfg.Metrics.Address,
		MetricsPort:    cfg.Metrics.Port,
		MetricsPath:    cfg.Metrics.Path,
		HealthEnabled:  cfg.Health.Enabled,
		HealthAddress:  cfg.Health.Address,
		HealthPort:     cfg.Health.Port,
		LivenessPath:   cfg.Health.LivenessPath,
		ReadinessPath:  cfg.Health.ReadinessPath,
	}, app.metrics, app.health)

	return app, nil
}

// Start starts all application components.
func (app *Application) Start(ctx context.Context) error {
	// Load policies
	if app.cfg.Policy.Enabled {
		loader := policy.NewLoader(app.cfg.Policy.PolicyDir, app.cfg.Policy.DataFile)
		if err := loader.LoadAndInitialize(ctx, app.policyEngine); err != nil {
			return fmt.Errorf("failed to load policies: %w", err)
		}
		log.Info().
			Str("policy_dir", app.cfg.Policy.PolicyDir).
			Str("data_file", app.cfg.Policy.DataFile).
			Str("mode", app.cfg.Policy.Mode).
			Msg("Policy engine initialized")
	}

	// Start audit writer
	if app.auditWriter != nil {
		app.auditWriter.Start()
		log.Info().
			Str("db_path", app.cfg.Audit.DBPath).
			Msg("Audit logging enabled")
	}

	// Start session manager
	app.sessionManager.Start(ctx)

	// Connect to upstream (if configured)
	if app.upstreamClient != nil {
		if err := app.upstreamClient.Connect(ctx); err != nil {
			log.Warn().Err(err).Msg("Failed to connect to upstream - will operate in standalone mode")
			// Don't fail startup - proxy can work without upstream for testing
		}
	}

	// Start transport server
	if err := app.transport.Start(ctx); err != nil {
		return fmt.Errorf("failed to start %s server: %w", app.transport.Name(), err)
	}

	// Start observability server
	if err := app.obsServer.Start(ctx); err != nil {
		return fmt.Errorf("failed to start observability server: %w", err)
	}

	// Mark as ready for health checks
	app.health.SetReady(true)

	return nil
}

// Stop gracefully stops all application components.
func (app *Application) Stop(ctx context.Context) error {
	log.Info().Msg("Starting graceful shutdown...")

	// Mark as not ready immediately
	app.health.SetReady(false)

	// Stop observability server
	if err := app.obsServer.Stop(ctx); err != nil {
		log.Error().Err(err).Msg("Error stopping observability server")
	}

	// Stop transport server first (stop accepting new connections)
	if err := app.transport.Stop(ctx); err != nil {
		log.Error().Err(err).Msg("Error stopping transport server")
	}

	// Disconnect from upstream
	if app.upstreamClient != nil {
		app.upstreamClient.Disconnect()
	}

	// Stop session manager (closes all sessions)
	app.sessionManager.Stop()

	// Stop audit writer (flushes remaining records)
	if app.auditWriter != nil {
		app.auditWriter.Stop()
	}

	// Close audit store
	if app.auditStore != nil {
		if err := app.auditStore.Close(); err != nil {
			log.Error().Err(err).Msg("Error closing audit store")
		}
	}

	return nil
}

// handleMessage processes an incoming MCP message through the router.
func (app *Application) handleMessage(ctx context.Context, sess *session.Session, message []byte) ([]byte, error) {
	// Route the message through the router
	return app.router.Route(ctx, sess, message)
}

func initLogger(cfg config.LoggingConfig) {
	// Set log level
	level, err := zerolog.ParseLevel(cfg.Level)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	// Determine output destination
	var output io.Writer = os.Stdout
	switch cfg.Output {
	case "stderr":
		output = os.Stderr
	case "stdout", "":
		output = os.Stdout
		// File output could be added here if needed
	}

	// Configure output format
	if cfg.Format == "text" {
		log.Logger = log.Output(zerolog.ConsoleWriter{
			Out:        output,
			TimeFormat: time.RFC3339,
		})
	} else {
		// JSON format (default)
		zerolog.TimeFieldFormat = time.RFC3339Nano
		log.Logger = log.Output(output)
	}

	log.Debug().Str("level", cfg.Level).Str("format", cfg.Format).Str("output", cfg.Output).Msg("Logger initialized")
}
