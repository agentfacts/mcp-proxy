package observability

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// HealthStatus represents the overall health status.
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
)

// ComponentHealth represents the health of a single component.
type ComponentHealth struct {
	Status  HealthStatus `json:"status"`
	Message string       `json:"message,omitempty"`
	Latency string       `json:"latency,omitempty"`
}

// HealthResponse is returned by health check endpoints.
type HealthResponse struct {
	Status     HealthStatus               `json:"status"`
	Timestamp  time.Time                  `json:"timestamp"`
	Version    string                     `json:"version,omitempty"`
	Components map[string]ComponentHealth `json:"components,omitempty"`
}

// HealthChecker defines a function that checks component health.
type HealthChecker func(ctx context.Context) ComponentHealth

// Health manages health check state and handlers.
type Health struct {
	version  string
	checkers map[string]HealthChecker
	mu       sync.RWMutex

	// Ready state can be toggled during startup/shutdown
	ready   bool
	readyMu sync.RWMutex
}

// NewHealth creates a new health checker.
func NewHealth(version string) *Health {
	return &Health{
		version:  version,
		checkers: make(map[string]HealthChecker),
		ready:    false,
	}
}

// RegisterChecker adds a health check for a named component.
func (h *Health) RegisterChecker(name string, checker HealthChecker) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checkers[name] = checker
}

// SetReady sets the readiness state.
func (h *Health) SetReady(ready bool) {
	h.readyMu.Lock()
	defer h.readyMu.Unlock()
	h.ready = ready
}

// IsReady returns the current readiness state.
func (h *Health) IsReady() bool {
	h.readyMu.RLock()
	defer h.readyMu.RUnlock()
	return h.ready
}

// LivenessHandler returns an HTTP handler for liveness checks.
// Liveness indicates the process is running and not deadlocked.
func (h *Health) LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response := HealthResponse{
			Status:    HealthStatusHealthy,
			Timestamp: time.Now().UTC(),
			Version:   h.version,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}
}

// ReadinessHandler returns an HTTP handler for readiness checks.
// Readiness indicates the service can accept traffic.
func (h *Health) ReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.IsReady() {
			response := HealthResponse{
				Status:    HealthStatusUnhealthy,
				Timestamp: time.Now().UTC(),
				Version:   h.version,
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(response)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		response := h.checkAll(ctx)

		w.Header().Set("Content-Type", "application/json")
		if response.Status == HealthStatusUnhealthy {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusOK)
		}
		json.NewEncoder(w).Encode(response)
	}
}

// FullHealthHandler returns a detailed health check of all components.
func (h *Health) FullHealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		response := h.checkAll(ctx)

		w.Header().Set("Content-Type", "application/json")
		if response.Status == HealthStatusUnhealthy {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusOK)
		}
		json.NewEncoder(w).Encode(response)
	}
}

// checkAll runs all registered health checkers.
func (h *Health) checkAll(ctx context.Context) HealthResponse {
	h.mu.RLock()
	checkers := make(map[string]HealthChecker, len(h.checkers))
	for name, checker := range h.checkers {
		checkers[name] = checker
	}
	h.mu.RUnlock()

	response := HealthResponse{
		Status:     HealthStatusHealthy,
		Timestamp:  time.Now().UTC(),
		Version:    h.version,
		Components: make(map[string]ComponentHealth),
	}

	// Run checks concurrently
	var wg sync.WaitGroup
	var mu sync.Mutex
	hasUnhealthy := false
	hasDegraded := false

	for name, checker := range checkers {
		wg.Add(1)
		go func(name string, checker HealthChecker) {
			defer wg.Done()

			start := time.Now()
			result := checker(ctx)
			result.Latency = time.Since(start).String()

			mu.Lock()
			response.Components[name] = result
			if result.Status == HealthStatusUnhealthy {
				hasUnhealthy = true
			} else if result.Status == HealthStatusDegraded {
				hasDegraded = true
			}
			mu.Unlock()
		}(name, checker)
	}

	wg.Wait()

	// Determine overall status
	if hasUnhealthy {
		response.Status = HealthStatusUnhealthy
	} else if hasDegraded {
		response.Status = HealthStatusDegraded
	}

	return response
}

// Common health checkers

// DatabaseChecker creates a health checker for database connectivity.
func DatabaseChecker(pingFunc func(ctx context.Context) error) HealthChecker {
	return func(ctx context.Context) ComponentHealth {
		if err := pingFunc(ctx); err != nil {
			return ComponentHealth{
				Status:  HealthStatusUnhealthy,
				Message: "database unreachable: " + err.Error(),
			}
		}
		return ComponentHealth{
			Status:  HealthStatusHealthy,
			Message: "connected",
		}
	}
}

// UpstreamChecker creates a health checker for upstream connectivity.
func UpstreamChecker(isConnected func() bool) HealthChecker {
	return func(ctx context.Context) ComponentHealth {
		if !isConnected() {
			return ComponentHealth{
				Status:  HealthStatusDegraded,
				Message: "upstream disconnected - operating in standalone mode",
			}
		}
		return ComponentHealth{
			Status:  HealthStatusHealthy,
			Message: "connected",
		}
	}
}

// PolicyEngineChecker creates a health checker for the policy engine.
func PolicyEngineChecker(isReady func() bool) HealthChecker {
	return func(ctx context.Context) ComponentHealth {
		if !isReady() {
			return ComponentHealth{
				Status:  HealthStatusUnhealthy,
				Message: "policy engine not initialized",
			}
		}
		return ComponentHealth{
			Status:  HealthStatusHealthy,
			Message: "ready",
		}
	}
}
