package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for the proxy.
type Metrics struct {
	// Request metrics
	RequestsTotal    *prometheus.CounterVec
	RequestDuration  *prometheus.HistogramVec
	RequestsInFlight prometheus.Gauge

	// Session metrics
	ActiveSessions  prometheus.Gauge
	SessionsTotal   *prometheus.CounterVec
	SessionDuration prometheus.Histogram

	// Policy metrics
	PolicyDecisions   *prometheus.CounterVec
	PolicyEvaluation  prometheus.Histogram
	PolicyCacheHits   prometheus.Counter
	PolicyCacheMisses prometheus.Counter

	// Upstream metrics
	UpstreamRequests  *prometheus.CounterVec
	UpstreamDuration  prometheus.Histogram
	UpstreamConnected prometheus.Gauge

	// Audit metrics
	AuditRecordsWritten prometheus.Counter
	AuditRecordsDropped prometheus.Counter
	AuditBufferSize     prometheus.Gauge
	AuditFlushes        prometheus.Counter
}

// NewMetrics creates and registers all Prometheus metrics.
func NewMetrics(namespace string) *Metrics {
	if namespace == "" {
		namespace = "mcp_proxy"
	}

	return &Metrics{
		// Request metrics
		RequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "requests_total",
				Help:      "Total number of MCP requests processed",
			},
			[]string{"method", "tool", "allowed"},
		),
		RequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "request_duration_seconds",
				Help:      "Request processing duration in seconds",
				Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"method"},
		),
		RequestsInFlight: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "requests_in_flight",
				Help:      "Number of requests currently being processed",
			},
		),

		// Session metrics
		ActiveSessions: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "sessions_active",
				Help:      "Number of active sessions",
			},
		),
		SessionsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "sessions_total",
				Help:      "Total number of sessions created",
			},
			[]string{"transport"},
		),
		SessionDuration: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "session_duration_seconds",
				Help:      "Session duration in seconds",
				Buckets:   []float64{1, 5, 15, 30, 60, 120, 300, 600, 1800, 3600},
			},
		),

		// Policy metrics
		PolicyDecisions: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "policy_decisions_total",
				Help:      "Total policy decisions by result",
			},
			[]string{"decision", "rule", "mode"},
		),
		PolicyEvaluation: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "policy_evaluation_seconds",
				Help:      "Policy evaluation duration in seconds",
				Buckets:   []float64{.0001, .0005, .001, .005, .01, .025, .05, .1},
			},
		),
		PolicyCacheHits: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "policy_cache_hits_total",
				Help:      "Number of policy cache hits",
			},
		),
		PolicyCacheMisses: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "policy_cache_misses_total",
				Help:      "Number of policy cache misses",
			},
		),

		// Upstream metrics
		UpstreamRequests: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "upstream_requests_total",
				Help:      "Total upstream requests by status",
			},
			[]string{"status"},
		),
		UpstreamDuration: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "upstream_request_duration_seconds",
				Help:      "Upstream request duration in seconds",
				Buckets:   []float64{.01, .05, .1, .25, .5, 1, 2.5, 5, 10, 30},
			},
		),
		UpstreamConnected: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "upstream_connected",
				Help:      "Whether upstream is connected (1) or not (0)",
			},
		),

		// Audit metrics
		AuditRecordsWritten: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "audit_records_written_total",
				Help:      "Total audit records written to storage",
			},
		),
		AuditRecordsDropped: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "audit_records_dropped_total",
				Help:      "Total audit records dropped due to buffer overflow or errors",
			},
		),
		AuditBufferSize: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "audit_buffer_size",
				Help:      "Current number of records in audit buffer",
			},
		),
		AuditFlushes: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "audit_flushes_total",
				Help:      "Total number of audit buffer flushes",
			},
		),
	}
}

// RecordRequest records metrics for a processed request.
func (m *Metrics) RecordRequest(method, tool string, allowed bool, durationSeconds float64) {
	allowedStr := "true"
	if !allowed {
		allowedStr = "false"
	}
	m.RequestsTotal.WithLabelValues(method, tool, allowedStr).Inc()
	m.RequestDuration.WithLabelValues(method).Observe(durationSeconds)
}

// RecordPolicyDecision records a policy evaluation result.
func (m *Metrics) RecordPolicyDecision(allowed bool, rule, mode string, durationSeconds float64) {
	decision := "allow"
	if !allowed {
		decision = "deny"
	}
	m.PolicyDecisions.WithLabelValues(decision, rule, mode).Inc()
	m.PolicyEvaluation.Observe(durationSeconds)
}

// RecordSession records session metrics.
func (m *Metrics) RecordSession(transport string, durationSeconds float64) {
	m.SessionsTotal.WithLabelValues(transport).Inc()
	if durationSeconds > 0 {
		m.SessionDuration.Observe(durationSeconds)
	}
}

// RecordUpstreamRequest records an upstream request result.
func (m *Metrics) RecordUpstreamRequest(status string, durationSeconds float64) {
	m.UpstreamRequests.WithLabelValues(status).Inc()
	m.UpstreamDuration.Observe(durationSeconds)
}

// UpdateAuditStats updates audit-related gauges.
func (m *Metrics) UpdateAuditStats(bufferSize int, written, dropped, flushes int64) {
	m.AuditBufferSize.Set(float64(bufferSize))
	// Note: Counters can only be incremented, so we use Add for deltas
	// In practice, we'd track these incrementally
}

// IncrementAuditWritten increments the audit records written counter.
func (m *Metrics) IncrementAuditWritten(count int) {
	m.AuditRecordsWritten.Add(float64(count))
}

// IncrementAuditDropped increments the audit records dropped counter.
func (m *Metrics) IncrementAuditDropped(count int) {
	m.AuditRecordsDropped.Add(float64(count))
}

// IncrementAuditFlushes increments the audit flushes counter.
func (m *Metrics) IncrementAuditFlushes() {
	m.AuditFlushes.Inc()
}
