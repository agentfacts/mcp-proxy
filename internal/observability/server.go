package observability

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
)

// ServerConfig holds configuration for the observability server.
type ServerConfig struct {
	// Metrics configuration
	MetricsEnabled bool
	MetricsAddress string
	MetricsPort    int
	MetricsPath    string

	// Health configuration
	HealthEnabled bool
	HealthAddress string
	HealthPort    int
	LivenessPath  string
	ReadinessPath string
}

// Server serves metrics and health check endpoints.
type Server struct {
	cfg     ServerConfig
	metrics *Metrics
	health  *Health

	metricsServer *http.Server
	healthServer  *http.Server
}

// NewServer creates a new observability server.
func NewServer(cfg ServerConfig, metrics *Metrics, health *Health) *Server {
	return &Server{
		cfg:     cfg,
		metrics: metrics,
		health:  health,
	}
}

// Start starts the observability servers.
func (s *Server) Start(ctx context.Context) error {
	// Start metrics server if enabled
	if s.cfg.MetricsEnabled {
		if err := s.startMetricsServer(); err != nil {
			return fmt.Errorf("failed to start metrics server: %w", err)
		}
	}

	// Start health server if enabled
	if s.cfg.HealthEnabled {
		if err := s.startHealthServer(); err != nil {
			return fmt.Errorf("failed to start health server: %w", err)
		}
	}

	return nil
}

// startMetricsServer starts the Prometheus metrics HTTP server.
func (s *Server) startMetricsServer() error {
	mux := http.NewServeMux()
	mux.Handle(s.cfg.MetricsPath, promhttp.Handler())

	addr := fmt.Sprintf("%s:%d", s.cfg.MetricsAddress, s.cfg.MetricsPort)
	s.metricsServer = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		log.Info().
			Str("address", addr).
			Str("path", s.cfg.MetricsPath).
			Msg("Metrics server listening")

		if err := s.metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("Metrics server error")
		}
	}()

	return nil
}

// startHealthServer starts the health check HTTP server.
func (s *Server) startHealthServer() error {
	mux := http.NewServeMux()
	mux.HandleFunc(s.cfg.LivenessPath, s.health.LivenessHandler())
	mux.HandleFunc(s.cfg.ReadinessPath, s.health.ReadinessHandler())
	mux.HandleFunc("/health/full", s.health.FullHealthHandler())

	addr := fmt.Sprintf("%s:%d", s.cfg.HealthAddress, s.cfg.HealthPort)
	s.healthServer = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		log.Info().
			Str("address", addr).
			Str("liveness", s.cfg.LivenessPath).
			Str("readiness", s.cfg.ReadinessPath).
			Msg("Health server listening")

		if err := s.healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("Health server error")
		}
	}()

	return nil
}

// Stop gracefully stops the observability servers.
func (s *Server) Stop(ctx context.Context) error {
	var errs []error

	if s.metricsServer != nil {
		log.Info().Msg("Stopping metrics server...")
		if err := s.metricsServer.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("metrics server shutdown: %w", err))
		}
	}

	if s.healthServer != nil {
		log.Info().Msg("Stopping health server...")
		if err := s.healthServer.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("health server shutdown: %w", err))
		}
	}

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}
