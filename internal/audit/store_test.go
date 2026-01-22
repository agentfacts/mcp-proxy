package audit

import (
	"context"
	"testing"
	"time"
)

// TestNewStore tests creating a new audit store.
func TestNewStore(t *testing.T) {
	tests := []struct {
		name   string
		config StoreConfig
	}{
		{
			name: "in-memory database",
			config: StoreConfig{
				DBPath: ":memory:",
			},
		},
		{
			name:   "default config",
			config: StoreConfig{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := NewStore(tt.config)
			if err != nil {
				t.Fatalf("NewStore() error = %v", err)
			}
			defer store.Close()

			if store.db == nil {
				t.Error("Store database is nil")
			}

			// Test ping
			ctx := context.Background()
			if err := store.Ping(ctx); err != nil {
				t.Errorf("Ping() error = %v", err)
			}
		})
	}
}

// TestInsertRecord tests inserting a single audit record.
func TestInsertRecord(t *testing.T) {
	store, err := NewStore(StoreConfig{DBPath: ":memory:"})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	record := NewRecordBuilder().
		WithRequest("req_123", "sess_456").
		WithTiming(42.5).
		WithAgent("agent1", "Test Agent", `["read","write"]`).
		WithMethod("tools/call", "read_file", "", `{"path":"/test"}`).
		WithIdentity(true, "did:example:123").
		WithDecision(true, "allow_rule", "", "enforce").
		WithEnvironment("192.168.1.1", "production").
		Build()

	err = store.Insert(ctx, record)
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	// Query to verify insertion
	opts := QueryOptions{
		Limit: 10,
	}
	records, err := store.Query(ctx, opts)
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}

	if len(records) != 1 {
		t.Fatalf("Query() returned %d records, want 1", len(records))
	}

	r := records[0]
	if r.RequestID != "req_123" {
		t.Errorf("RequestID = %s, want 'req_123'", r.RequestID)
	}
	if r.SessionID != "sess_456" {
		t.Errorf("SessionID = %s, want 'sess_456'", r.SessionID)
	}
	if r.AgentID != "agent1" {
		t.Errorf("AgentID = %s, want 'agent1'", r.AgentID)
	}
	if r.Method != "tools/call" {
		t.Errorf("Method = %s, want 'tools/call'", r.Method)
	}
	if r.Tool != "read_file" {
		t.Errorf("Tool = %s, want 'read_file'", r.Tool)
	}
	if !r.Allowed {
		t.Error("Allowed should be true")
	}
	if !r.IdentityVerified {
		t.Error("IdentityVerified should be true")
	}
	if r.Latency != 42.5 {
		t.Errorf("Latency = %f, want 42.5", r.Latency)
	}
}

// TestInsertBatch tests inserting multiple records in a transaction.
func TestInsertBatch(t *testing.T) {
	store, err := NewStore(StoreConfig{DBPath: ":memory:"})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Create multiple records
	records := []*Record{}
	for i := 0; i < 10; i++ {
		record := NewRecordBuilder().
			WithRequest("req_"+string(rune('0'+i)), "sess_test").
			WithAgent("agent1", "Test Agent", `["read"]`).
			WithMethod("tools/call", "test_tool", "", "{}").
			WithDecision(true, "allow_rule", "", "enforce").
			Build()
		records = append(records, record)
	}

	err = store.InsertBatch(ctx, records)
	if err != nil {
		t.Fatalf("InsertBatch() error = %v", err)
	}

	// Verify all records were inserted
	opts := QueryOptions{
		Limit: 20,
	}
	retrieved, err := store.Query(ctx, opts)
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}

	if len(retrieved) != 10 {
		t.Errorf("Query() returned %d records, want 10", len(retrieved))
	}
}

// TestInsertBatchEmpty tests inserting empty batch (should be no-op).
func TestInsertBatchEmpty(t *testing.T) {
	store, err := NewStore(StoreConfig{DBPath: ":memory:"})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	err = store.InsertBatch(ctx, []*Record{})
	if err != nil {
		t.Errorf("InsertBatch() with empty slice should not error, got %v", err)
	}
}

// TestQueryWithFilters tests querying with various filters.
func TestQueryWithFilters(t *testing.T) {
	store, err := NewStore(StoreConfig{DBPath: ":memory:"})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Insert test data
	now := time.Now()
	records := []*Record{
		{
			RequestID: "req_1",
			SessionID: "sess_a",
			Timestamp: now.Add(-2 * time.Hour),
			AgentID:   "agent1",
			Method:    "tools/call",
			Tool:      "read_file",
			Allowed:   true,
		},
		{
			RequestID: "req_2",
			SessionID: "sess_a",
			Timestamp: now.Add(-1 * time.Hour),
			AgentID:   "agent1",
			Method:    "tools/call",
			Tool:      "write_file",
			Allowed:   false,
		},
		{
			RequestID: "req_3",
			SessionID: "sess_b",
			Timestamp: now.Add(-30 * time.Minute),
			AgentID:   "agent2",
			Method:    "resources/read",
			Tool:      "",
			Allowed:   true,
		},
	}

	for _, r := range records {
		if err := store.Insert(ctx, r); err != nil {
			t.Fatalf("Insert() error = %v", err)
		}
	}

	tests := []struct {
		name        string
		opts        QueryOptions
		expectCount int
	}{
		{
			name: "filter by agent",
			opts: QueryOptions{
				AgentID: "agent1",
			},
			expectCount: 2,
		},
		{
			name: "filter by session",
			opts: QueryOptions{
				SessionID: "sess_a",
			},
			expectCount: 2,
		},
		{
			name: "filter by method",
			opts: QueryOptions{
				Method: "tools/call",
			},
			expectCount: 2,
		},
		{
			name: "filter by tool",
			opts: QueryOptions{
				Tool: "read_file",
			},
			expectCount: 1,
		},
		{
			name: "filter by allowed",
			opts: QueryOptions{
				Allowed: boolPtr(true),
			},
			expectCount: 2,
		},
		{
			name: "filter by denied",
			opts: QueryOptions{
				Allowed: boolPtr(false),
			},
			expectCount: 1,
		},
		{
			name: "filter by time range",
			opts: QueryOptions{
				StartTime: timePtr(now.Add(-90 * time.Minute)),
				EndTime:   timePtr(now),
			},
			expectCount: 2,
		},
		{
			name: "limit results",
			opts: QueryOptions{
				Limit: 2,
			},
			expectCount: 2,
		},
		{
			name: "offset results",
			opts: QueryOptions{
				Limit:  10,
				Offset: 1,
			},
			expectCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := store.Query(ctx, tt.opts)
			if err != nil {
				t.Fatalf("Query() error = %v", err)
			}
			if len(results) != tt.expectCount {
				t.Errorf("Query() returned %d records, want %d", len(results), tt.expectCount)
			}
		})
	}
}

// TestQueryOrdering tests query result ordering.
func TestQueryOrdering(t *testing.T) {
	store, err := NewStore(StoreConfig{DBPath: ":memory:"})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Insert records with different timestamps
	now := time.Now()
	records := []*Record{
		{
			RequestID: "req_1",
			SessionID: "sess_test",
			Timestamp: now.Add(-3 * time.Hour),
			AgentID:   "agent1",
			Method:    "tools/call",
			Allowed:   true,
		},
		{
			RequestID: "req_2",
			SessionID: "sess_test",
			Timestamp: now.Add(-1 * time.Hour),
			AgentID:   "agent1",
			Method:    "tools/call",
			Allowed:   true,
		},
		{
			RequestID: "req_3",
			SessionID: "sess_test",
			Timestamp: now.Add(-2 * time.Hour),
			AgentID:   "agent1",
			Method:    "tools/call",
			Allowed:   true,
		},
	}

	for _, r := range records {
		if err := store.Insert(ctx, r); err != nil {
			t.Fatalf("Insert() error = %v", err)
		}
	}

	// Query with ascending order (default)
	optsAsc := QueryOptions{
		OrderBy:   "timestamp",
		OrderDesc: false,
	}
	resultsAsc, err := store.Query(ctx, optsAsc)
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(resultsAsc) != 3 {
		t.Fatalf("Query() returned %d records, want 3", len(resultsAsc))
	}
	if resultsAsc[0].RequestID != "req_1" {
		t.Errorf("First record = %s, want 'req_1'", resultsAsc[0].RequestID)
	}

	// Query with descending order
	optsDesc := QueryOptions{
		OrderBy:   "timestamp",
		OrderDesc: true,
	}
	resultsDesc, err := store.Query(ctx, optsDesc)
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(resultsDesc) != 3 {
		t.Fatalf("Query() returned %d records, want 3", len(resultsDesc))
	}
	if resultsDesc[0].RequestID != "req_2" {
		t.Errorf("First record = %s, want 'req_2'", resultsDesc[0].RequestID)
	}
}

// TestGetStats tests statistics aggregation.
func TestGetStats(t *testing.T) {
	store, err := NewStore(StoreConfig{DBPath: ":memory:"})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Insert test data
	now := time.Now()
	records := []*Record{
		{
			RequestID: "req_1",
			SessionID: "sess_a",
			Timestamp: now.Add(-1 * time.Hour),
			AgentID:   "agent1",
			Method:    "tools/call",
			Allowed:   true,
			Latency:   10.0,
		},
		{
			RequestID: "req_2",
			SessionID: "sess_a",
			Timestamp: now.Add(-1 * time.Hour),
			AgentID:   "agent1",
			Method:    "tools/call",
			Allowed:   false,
			Latency:   20.0,
		},
		{
			RequestID: "req_3",
			SessionID: "sess_b",
			Timestamp: now.Add(-30 * time.Minute),
			AgentID:   "agent2",
			Method:    "tools/call",
			Allowed:   true,
			Latency:   30.0,
		},
		{
			RequestID: "req_4",
			SessionID: "sess_a",
			Timestamp: now.Add(-3 * time.Hour),
			AgentID:   "agent1",
			Method:    "tools/call",
			Allowed:   true,
			Latency:   40.0,
		},
	}

	for _, r := range records {
		if err := store.Insert(ctx, r); err != nil {
			t.Fatalf("Insert() error = %v", err)
		}
	}

	// Get stats for all records
	stats, err := store.GetStats(ctx, nil)
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}

	if stats.TotalRequests != 4 {
		t.Errorf("TotalRequests = %d, want 4", stats.TotalRequests)
	}
	if stats.AllowedRequests != 3 {
		t.Errorf("AllowedRequests = %d, want 3", stats.AllowedRequests)
	}
	if stats.DeniedRequests != 1 {
		t.Errorf("DeniedRequests = %d, want 1", stats.DeniedRequests)
	}
	if stats.UniqueAgents != 2 {
		t.Errorf("UniqueAgents = %d, want 2", stats.UniqueAgents)
	}
	if stats.UniqueSessions != 2 {
		t.Errorf("UniqueSessions = %d, want 2", stats.UniqueSessions)
	}

	// Average latency should be (10+20+30+40)/4 = 25.0
	expectedAvg := 25.0
	tolerance := 0.1
	if stats.AvgLatencyMs < expectedAvg-tolerance || stats.AvgLatencyMs > expectedAvg+tolerance {
		t.Errorf("AvgLatencyMs = %f, want %f", stats.AvgLatencyMs, expectedAvg)
	}

	// Get stats since 2 hours ago (should exclude the 3-hour old record)
	since := now.Add(-2 * time.Hour)
	statsRecent, err := store.GetStats(ctx, &since)
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}

	if statsRecent.TotalRequests != 3 {
		t.Errorf("TotalRequests (recent) = %d, want 3", statsRecent.TotalRequests)
	}
	if statsRecent.AllowedRequests != 2 {
		t.Errorf("AllowedRequests (recent) = %d, want 2", statsRecent.AllowedRequests)
	}
}

// TestPrune tests pruning old records.
func TestPrune(t *testing.T) {
	store, err := NewStore(StoreConfig{DBPath: ":memory:"})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Insert records with different timestamps
	now := time.Now()
	records := []*Record{
		{
			RequestID: "req_old",
			SessionID: "sess_test",
			Timestamp: now.Add(-48 * time.Hour),
			AgentID:   "agent1",
			Method:    "tools/call",
			Allowed:   true,
		},
		{
			RequestID: "req_recent",
			SessionID: "sess_test",
			Timestamp: now.Add(-1 * time.Hour),
			AgentID:   "agent1",
			Method:    "tools/call",
			Allowed:   true,
		},
	}

	for _, r := range records {
		if err := store.Insert(ctx, r); err != nil {
			t.Fatalf("Insert() error = %v", err)
		}
	}

	// Prune records older than 24 hours
	deleted, err := store.Prune(ctx, 24*time.Hour)
	if err != nil {
		t.Fatalf("Prune() error = %v", err)
	}

	if deleted != 1 {
		t.Errorf("Prune() deleted %d records, want 1", deleted)
	}

	// Verify only recent record remains
	opts := QueryOptions{}
	remaining, err := store.Query(ctx, opts)
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}

	if len(remaining) != 1 {
		t.Errorf("Query() returned %d records after prune, want 1", len(remaining))
	}
	if len(remaining) > 0 && remaining[0].RequestID != "req_recent" {
		t.Errorf("Remaining record = %s, want 'req_recent'", remaining[0].RequestID)
	}
}

// TestRecordBuilder tests the audit record builder.
func TestRecordBuilder(t *testing.T) {
	record := NewRecordBuilder().
		WithRequest("req_123", "sess_456").
		WithTiming(42.5).
		WithAgent("agent1", "Test Agent", `["read","write"]`).
		WithMethod("tools/call", "read_file", "file:///test", `{"path":"/test"}`).
		WithIdentity(true, "did:example:123").
		WithDecision(true, "allow_rule", `["violation1"]`, "enforce").
		WithEnvironment("192.168.1.1", "production").
		Build()

	if record.RequestID != "req_123" {
		t.Errorf("RequestID = %s, want 'req_123'", record.RequestID)
	}
	if record.SessionID != "sess_456" {
		t.Errorf("SessionID = %s, want 'sess_456'", record.SessionID)
	}
	if record.Latency != 42.5 {
		t.Errorf("Latency = %f, want 42.5", record.Latency)
	}
	if record.AgentID != "agent1" {
		t.Errorf("AgentID = %s, want 'agent1'", record.AgentID)
	}
	if record.AgentName != "Test Agent" {
		t.Errorf("AgentName = %s, want 'Test Agent'", record.AgentName)
	}
	if record.Method != "tools/call" {
		t.Errorf("Method = %s, want 'tools/call'", record.Method)
	}
	if record.Tool != "read_file" {
		t.Errorf("Tool = %s, want 'read_file'", record.Tool)
	}
	if record.ResourceURI != "file:///test" {
		t.Errorf("ResourceURI = %s, want 'file:///test'", record.ResourceURI)
	}
	if !record.IdentityVerified {
		t.Error("IdentityVerified should be true")
	}
	if record.DID != "did:example:123" {
		t.Errorf("DID = %s, want 'did:example:123'", record.DID)
	}
	if !record.Allowed {
		t.Error("Allowed should be true")
	}
	if record.MatchedRule != "allow_rule" {
		t.Errorf("MatchedRule = %s, want 'allow_rule'", record.MatchedRule)
	}
	if record.PolicyMode != "enforce" {
		t.Errorf("PolicyMode = %s, want 'enforce'", record.PolicyMode)
	}
	if record.SourceIP != "192.168.1.1" {
		t.Errorf("SourceIP = %s, want '192.168.1.1'", record.SourceIP)
	}
	if record.Environment != "production" {
		t.Errorf("Environment = %s, want 'production'", record.Environment)
	}
}

// TestMultipleFilters tests querying with multiple filters combined.
func TestMultipleFilters(t *testing.T) {
	store, err := NewStore(StoreConfig{DBPath: ":memory:"})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Insert test data
	now := time.Now()
	records := []*Record{
		{
			RequestID: "req_1",
			SessionID: "sess_a",
			Timestamp: now.Add(-1 * time.Hour),
			AgentID:   "agent1",
			Method:    "tools/call",
			Tool:      "read_file",
			Allowed:   true,
		},
		{
			RequestID: "req_2",
			SessionID: "sess_a",
			Timestamp: now.Add(-1 * time.Hour),
			AgentID:   "agent1",
			Method:    "tools/call",
			Tool:      "write_file",
			Allowed:   false,
		},
		{
			RequestID: "req_3",
			SessionID: "sess_b",
			Timestamp: now.Add(-30 * time.Minute),
			AgentID:   "agent2",
			Method:    "tools/call",
			Tool:      "read_file",
			Allowed:   true,
		},
	}

	for _, r := range records {
		if err := store.Insert(ctx, r); err != nil {
			t.Fatalf("Insert() error = %v", err)
		}
	}

	// Query with multiple filters: agent1 + read_file
	opts := QueryOptions{
		AgentID: "agent1",
		Tool:    "read_file",
	}

	results, err := store.Query(ctx, opts)
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}

	// Should only match req_1
	if len(results) != 1 {
		t.Errorf("Query() returned %d records, want 1", len(results))
	}
	if len(results) > 0 && results[0].RequestID != "req_1" {
		t.Errorf("RequestID = %s, want 'req_1'", results[0].RequestID)
	}
}

// TestStatsEmpty tests getting stats from an empty database.
func TestStatsEmpty(t *testing.T) {
	store, err := NewStore(StoreConfig{DBPath: ":memory:"})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	stats, err := store.GetStats(ctx, nil)
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}

	if stats.TotalRequests != 0 {
		t.Errorf("TotalRequests = %d, want 0", stats.TotalRequests)
	}
	if stats.AllowedRequests != 0 {
		t.Errorf("AllowedRequests = %d, want 0", stats.AllowedRequests)
	}
	if stats.DeniedRequests != 0 {
		t.Errorf("DeniedRequests = %d, want 0", stats.DeniedRequests)
	}
	if stats.AvgLatencyMs != 0 {
		t.Errorf("AvgLatencyMs = %f, want 0", stats.AvgLatencyMs)
	}
}

// TestPruneEmpty tests pruning an empty database.
func TestPruneEmpty(t *testing.T) {
	store, err := NewStore(StoreConfig{DBPath: ":memory:"})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	deleted, err := store.Prune(ctx, 24*time.Hour)
	if err != nil {
		t.Fatalf("Prune() error = %v", err)
	}

	if deleted != 0 {
		t.Errorf("Prune() deleted %d records, want 0", deleted)
	}
}

// Helper functions

func boolPtr(b bool) *bool {
	return &b
}

func timePtr(t time.Time) *time.Time {
	return &t
}
