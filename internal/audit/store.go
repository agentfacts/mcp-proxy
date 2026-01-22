package audit

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog/log"
)

// Store provides SQLite-based audit log storage.
type Store struct {
	db     *sql.DB
	dbPath string
}

// StoreConfig holds configuration for the audit store.
type StoreConfig struct {
	DBPath string // Path to SQLite file, ":memory:" for in-memory
}

// NewStore creates a new SQLite audit store.
func NewStore(cfg StoreConfig) (*Store, error) {
	if cfg.DBPath == "" {
		cfg.DBPath = "audit.db"
	}

	db, err := sql.Open("sqlite3", cfg.DBPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(1) // SQLite only supports one writer
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

	store := &Store{
		db:     db,
		dbPath: cfg.DBPath,
	}

	// Initialize schema
	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return store, nil
}

// initSchema creates the audit table and indexes.
func (s *Store) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS audit_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		request_id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		latency_ms REAL,

		-- Agent info
		agent_id TEXT NOT NULL,
		agent_name TEXT,
		capabilities TEXT,

		-- Request info
		method TEXT NOT NULL,
		tool TEXT,
		resource_uri TEXT,
		arguments TEXT,

		-- Identity info
		identity_verified INTEGER DEFAULT 0,
		did TEXT,

		-- Policy decision
		allowed INTEGER NOT NULL,
		matched_rule TEXT,
		violations TEXT,
		policy_mode TEXT,

		-- Environment
		source_ip TEXT,
		environment TEXT
	);

	-- Indexes for common queries
	CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_log(timestamp);
	CREATE INDEX IF NOT EXISTS idx_audit_agent_id ON audit_log(agent_id);
	CREATE INDEX IF NOT EXISTS idx_audit_session_id ON audit_log(session_id);
	CREATE INDEX IF NOT EXISTS idx_audit_method ON audit_log(method);
	CREATE INDEX IF NOT EXISTS idx_audit_allowed ON audit_log(allowed);
	CREATE INDEX IF NOT EXISTS idx_audit_tool ON audit_log(tool);
	`

	_, err := s.db.Exec(schema)
	return err
}

// Insert adds a single audit record.
func (s *Store) Insert(ctx context.Context, record *Record) error {
	query := `
	INSERT INTO audit_log (
		request_id, session_id, timestamp, latency_ms,
		agent_id, agent_name, capabilities,
		method, tool, resource_uri, arguments,
		identity_verified, did,
		allowed, matched_rule, violations, policy_mode,
		source_ip, environment
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := s.db.ExecContext(ctx, query,
		record.RequestID, record.SessionID, record.Timestamp, record.Latency,
		record.AgentID, record.AgentName, record.Capabilities,
		record.Method, record.Tool, record.ResourceURI, record.Arguments,
		record.IdentityVerified, record.DID,
		record.Allowed, record.MatchedRule, record.Violations, record.PolicyMode,
		record.SourceIP, record.Environment,
	)

	return err
}

// InsertBatch inserts multiple records in a single transaction.
func (s *Store) InsertBatch(ctx context.Context, records []*Record) error {
	if len(records) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO audit_log (
			request_id, session_id, timestamp, latency_ms,
			agent_id, agent_name, capabilities,
			method, tool, resource_uri, arguments,
			identity_verified, did,
			allowed, matched_rule, violations, policy_mode,
			source_ip, environment
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, record := range records {
		_, err := stmt.ExecContext(ctx,
			record.RequestID, record.SessionID, record.Timestamp, record.Latency,
			record.AgentID, record.AgentName, record.Capabilities,
			record.Method, record.Tool, record.ResourceURI, record.Arguments,
			record.IdentityVerified, record.DID,
			record.Allowed, record.MatchedRule, record.Violations, record.PolicyMode,
			record.SourceIP, record.Environment,
		)
		if err != nil {
			return fmt.Errorf("failed to insert record: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// allowedOrderByColumns defines the whitelist of columns that can be used in ORDER BY.
// This prevents SQL injection through the OrderBy field.
var allowedOrderByColumns = map[string]bool{
	"id":         true,
	"timestamp":  true,
	"agent_id":   true,
	"session_id": true,
	"method":     true,
	"tool":       true,
	"allowed":    true,
	"latency_ms": true,
	"source_ip":  true,
}

// Query retrieves audit records based on options.
func (s *Store) Query(ctx context.Context, opts QueryOptions) ([]*Record, error) {
	var conditions []string
	var args []interface{}

	if opts.StartTime != nil {
		conditions = append(conditions, "timestamp >= ?")
		args = append(args, *opts.StartTime)
	}
	if opts.EndTime != nil {
		conditions = append(conditions, "timestamp <= ?")
		args = append(args, *opts.EndTime)
	}
	if opts.AgentID != "" {
		conditions = append(conditions, "agent_id = ?")
		args = append(args, opts.AgentID)
	}
	if opts.SessionID != "" {
		conditions = append(conditions, "session_id = ?")
		args = append(args, opts.SessionID)
	}
	if opts.Method != "" {
		conditions = append(conditions, "method = ?")
		args = append(args, opts.Method)
	}
	if opts.Tool != "" {
		conditions = append(conditions, "tool = ?")
		args = append(args, opts.Tool)
	}
	if opts.Allowed != nil {
		conditions = append(conditions, "allowed = ?")
		args = append(args, *opts.Allowed)
	}

	query := "SELECT id, request_id, session_id, timestamp, latency_ms, " +
		"agent_id, agent_name, capabilities, " +
		"method, tool, resource_uri, arguments, " +
		"identity_verified, did, " +
		"allowed, matched_rule, violations, policy_mode, " +
		"source_ip, environment " +
		"FROM audit_log"

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	// Order by - validate against whitelist to prevent SQL injection
	orderBy := "timestamp"
	if opts.OrderBy != "" {
		if !allowedOrderByColumns[opts.OrderBy] {
			return nil, fmt.Errorf("invalid order by column: %s", opts.OrderBy)
		}
		orderBy = opts.OrderBy
	}
	order := "ASC"
	if opts.OrderDesc {
		order = "DESC"
	}
	query += fmt.Sprintf(" ORDER BY %s %s", orderBy, order)

	// Pagination
	if opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}
	if opts.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", opts.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query: %w", err)
	}
	defer rows.Close()

	var records []*Record
	for rows.Next() {
		r := &Record{}
		err := rows.Scan(
			&r.ID, &r.RequestID, &r.SessionID, &r.Timestamp, &r.Latency,
			&r.AgentID, &r.AgentName, &r.Capabilities,
			&r.Method, &r.Tool, &r.ResourceURI, &r.Arguments,
			&r.IdentityVerified, &r.DID,
			&r.Allowed, &r.MatchedRule, &r.Violations, &r.PolicyMode,
			&r.SourceIP, &r.Environment,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		records = append(records, r)
	}

	return records, rows.Err()
}

// GetStats returns aggregate statistics.
func (s *Store) GetStats(ctx context.Context, since *time.Time) (*Stats, error) {
	query := `
	SELECT
		COUNT(*) as total,
		COALESCE(SUM(CASE WHEN allowed = 1 THEN 1 ELSE 0 END), 0) as allowed,
		COALESCE(SUM(CASE WHEN allowed = 0 THEN 1 ELSE 0 END), 0) as denied,
		COUNT(DISTINCT agent_id) as unique_agents,
		COUNT(DISTINCT session_id) as unique_sessions,
		AVG(latency_ms) as avg_latency
	FROM audit_log
	`

	var args []interface{}
	if since != nil {
		query += " WHERE timestamp >= ?"
		args = append(args, *since)
	}

	var stats Stats
	var avgLatency sql.NullFloat64

	err := s.db.QueryRowContext(ctx, query, args...).Scan(
		&stats.TotalRequests,
		&stats.AllowedRequests,
		&stats.DeniedRequests,
		&stats.UniqueAgents,
		&stats.UniqueSessions,
		&avgLatency,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}

	if avgLatency.Valid {
		stats.AvgLatencyMs = avgLatency.Float64
	}

	return &stats, nil
}

// Prune removes records older than the specified duration.
func (s *Store) Prune(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)

	result, err := s.db.ExecContext(ctx,
		"DELETE FROM audit_log WHERE timestamp < ?",
		cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to prune: %w", err)
	}

	return result.RowsAffected()
}

// Ping checks database connectivity.
func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// Close closes the database connection.
func (s *Store) Close() error {
	log.Info().Str("path", s.dbPath).Msg("Closing audit store")
	return s.db.Close()
}
