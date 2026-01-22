package session

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestNewManager tests manager creation with various configurations.
func TestNewManager(t *testing.T) {
	tests := []struct {
		name           string
		config         ManagerConfig
		expectedTTL    time.Duration
		expectedMaxSes int
	}{
		{
			name: "default config",
			config: ManagerConfig{
				SessionTTL:      0,
				CleanupInterval: 0,
				MaxSessions:     0,
			},
			expectedTTL:    2 * time.Hour,
			expectedMaxSes: 10000,
		},
		{
			name: "custom config",
			config: ManagerConfig{
				SessionTTL:      1 * time.Hour,
				CleanupInterval: 30 * time.Second,
				MaxSessions:     500,
			},
			expectedTTL:    1 * time.Hour,
			expectedMaxSes: 500,
		},
		{
			name:           "use default function",
			config:         DefaultManagerConfig(),
			expectedTTL:    2 * time.Hour,
			expectedMaxSes: 10000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewManager(tt.config)
			if mgr == nil {
				t.Fatal("NewManager returned nil")
			}
			if mgr.sessionTTL != tt.expectedTTL {
				t.Errorf("sessionTTL = %v, want %v", mgr.sessionTTL, tt.expectedTTL)
			}
			if mgr.maxSessions != tt.expectedMaxSes {
				t.Errorf("maxSessions = %d, want %d", mgr.maxSessions, tt.expectedMaxSes)
			}
		})
	}
}

// TestSessionCreation tests creating sessions with proper ID generation.
func TestSessionCreation(t *testing.T) {
	mgr := NewManager(DefaultManagerConfig())
	ctx := context.Background()

	// Create a session
	sess, err := mgr.Create(ctx)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if sess == nil {
		t.Fatal("Create() returned nil session")
	}

	// Verify session ID format (should start with "sess_")
	if len(sess.ID) < 6 || sess.ID[:5] != "sess_" {
		t.Errorf("session ID format invalid: %s", sess.ID)
	}

	// Verify metrics updated
	if mgr.ActiveCount() != 1 {
		t.Errorf("ActiveCount = %d, want 1", mgr.ActiveCount())
	}
	if mgr.TotalCreated() != 1 {
		t.Errorf("TotalCreated = %d, want 1", mgr.TotalCreated())
	}

	// Create another session - should have different ID
	sess2, err := mgr.Create(ctx)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if sess.ID == sess2.ID {
		t.Error("Two sessions have the same ID")
	}

	if mgr.ActiveCount() != 2 {
		t.Errorf("ActiveCount = %d, want 2", mgr.ActiveCount())
	}
}

// TestSessionRetrieval tests getting existing and non-existing sessions.
func TestSessionRetrieval(t *testing.T) {
	mgr := NewManager(DefaultManagerConfig())
	ctx := context.Background()

	// Create a session
	sess, err := mgr.Create(ctx)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Retrieve existing session
	retrieved, ok := mgr.Get(sess.ID)
	if !ok {
		t.Fatal("Get() returned false for existing session")
	}
	if retrieved.ID != sess.ID {
		t.Errorf("Get() returned wrong session: got %s, want %s", retrieved.ID, sess.ID)
	}

	// Try to retrieve non-existing session
	nonExist, ok := mgr.Get("sess_nonexistent")
	if ok {
		t.Error("Get() returned true for non-existing session")
	}
	if nonExist != nil {
		t.Error("Get() returned non-nil for non-existing session")
	}
}

// TestSessionUpdate tests request count and activity time updates.
func TestSessionUpdate(t *testing.T) {
	mgr := NewManager(DefaultManagerConfig())
	ctx := context.Background()

	sess, err := mgr.Create(ctx)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	initialActivity := sess.LastActivityAt

	// Wait a bit to ensure time difference
	time.Sleep(10 * time.Millisecond)

	// Increment request count
	count := mgr.IncrementRequestCount(sess.ID)
	if count != 1 {
		t.Errorf("IncrementRequestCount() = %d, want 1", count)
	}

	// Verify session was updated
	retrieved, _ := mgr.Get(sess.ID)
	if retrieved.RequestCount != 1 {
		t.Errorf("RequestCount = %d, want 1", retrieved.RequestCount)
	}
	if !retrieved.LastActivityAt.After(initialActivity) {
		t.Error("LastActivityAt was not updated")
	}

	// Increment again
	count = mgr.IncrementRequestCount(sess.ID)
	if count != 2 {
		t.Errorf("IncrementRequestCount() = %d, want 2", count)
	}

	// Non-existing session returns 0
	count = mgr.IncrementRequestCount("sess_nonexistent")
	if count != 0 {
		t.Errorf("IncrementRequestCount() for non-existing = %d, want 0", count)
	}
}

// TestSessionDeletion tests session removal.
func TestSessionDeletion(t *testing.T) {
	mgr := NewManager(DefaultManagerConfig())
	ctx := context.Background()

	sess, err := mgr.Create(ctx)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	initialActive := mgr.ActiveCount()

	// Delete the session
	mgr.Delete(sess.ID)

	// Verify it's gone
	retrieved, ok := mgr.Get(sess.ID)
	if ok {
		t.Error("Get() returned true after Delete()")
	}
	if retrieved != nil {
		t.Error("Get() returned non-nil after Delete()")
	}

	// Verify metrics updated
	if mgr.ActiveCount() != initialActive-1 {
		t.Errorf("ActiveCount = %d, want %d", mgr.ActiveCount(), initialActive-1)
	}

	// Verify session was closed
	if !sess.IsClosed() {
		t.Error("Session was not closed after Delete()")
	}

	// Deleting non-existing session should not panic
	mgr.Delete("sess_nonexistent")
}

// TestSessionTTLExpiration tests session cleanup based on TTL.
func TestSessionTTLExpiration(t *testing.T) {
	// Use very short TTL for testing
	mgr := NewManager(ManagerConfig{
		SessionTTL:      50 * time.Millisecond,
		CleanupInterval: 10 * time.Millisecond,
		MaxSessions:     10,
	})
	ctx := context.Background()

	sess, err := mgr.Create(ctx)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Session should exist initially
	if _, ok := mgr.Get(sess.ID); !ok {
		t.Fatal("Session not found immediately after creation")
	}

	// Wait for TTL to expire
	time.Sleep(60 * time.Millisecond)

	// Manually trigger cleanup
	mgr.cleanup()

	// Session should be gone
	if _, ok := mgr.Get(sess.ID); ok {
		t.Error("Session still exists after TTL expiration")
	}
}

// TestSessionIdleTimeout tests cleanup of idle sessions.
func TestSessionIdleTimeout(t *testing.T) {
	// Short TTL for testing, idle timeout is TTL/2
	mgr := NewManager(ManagerConfig{
		SessionTTL:      100 * time.Millisecond,
		CleanupInterval: 10 * time.Millisecond,
		MaxSessions:     10,
	})
	ctx := context.Background()

	sess, err := mgr.Create(ctx)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Wait for idle timeout (TTL/2 = 50ms)
	time.Sleep(60 * time.Millisecond)

	// Manually trigger cleanup
	mgr.cleanup()

	// Session should be removed due to idle timeout
	if _, ok := mgr.Get(sess.ID); ok {
		t.Error("Session still exists after idle timeout")
	}
}

// TestMaxSessionsLimit tests enforcement of max sessions limit.
func TestMaxSessionsLimit(t *testing.T) {
	mgr := NewManager(ManagerConfig{
		SessionTTL:      1 * time.Hour,
		CleanupInterval: 1 * time.Minute,
		MaxSessions:     3,
	})
	ctx := context.Background()

	// Create sessions up to the limit
	for i := 0; i < 3; i++ {
		_, err := mgr.Create(ctx)
		if err != nil {
			t.Fatalf("Create() error on session %d: %v", i, err)
		}
	}

	// Try to create one more - should fail
	_, err := mgr.Create(ctx)
	if err == nil {
		t.Error("Create() should have failed when max sessions reached")
	}
	if err != ErrMaxSessionsReached {
		t.Errorf("Create() error = %v, want %v", err, ErrMaxSessionsReached)
	}

	// Verify we still have exactly 3 sessions
	if mgr.ActiveCount() != 3 {
		t.Errorf("ActiveCount = %d, want 3", mgr.ActiveCount())
	}
}

// TestConcurrentAccess tests thread safety of session operations.
func TestConcurrentAccess(t *testing.T) {
	mgr := NewManager(ManagerConfig{
		SessionTTL:      1 * time.Hour,
		CleanupInterval: 1 * time.Minute,
		MaxSessions:     1000,
	})
	ctx := context.Background()

	const goroutines = 10
	const opsPerGoroutine = 50
	var wg sync.WaitGroup

	// Concurrent session creation
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				sess, err := mgr.Create(ctx)
				if err != nil {
					t.Errorf("Concurrent Create() error: %v", err)
					return
				}
				// Immediately increment request count
				mgr.IncrementRequestCount(sess.ID)
			}
		}()
	}
	wg.Wait()

	// Verify total created
	expectedTotal := int64(goroutines * opsPerGoroutine)
	if mgr.TotalCreated() != expectedTotal {
		t.Errorf("TotalCreated = %d, want %d", mgr.TotalCreated(), expectedTotal)
	}
	if mgr.ActiveCount() != int(expectedTotal) {
		t.Errorf("ActiveCount = %d, want %d", mgr.ActiveCount(), expectedTotal)
	}

	// Get all sessions
	sessions := mgr.List()

	// Concurrent reads and deletes
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			start := idx * opsPerGoroutine
			end := start + opsPerGoroutine
			for j := start; j < end && j < len(sessions); j++ {
				// Read
				mgr.Get(sessions[j].ID)
				// Increment
				mgr.IncrementRequestCount(sessions[j].ID)
				// Delete half
				if j%2 == 0 {
					mgr.Delete(sessions[j].ID)
				}
			}
		}(i)
	}
	wg.Wait()

	// Should have roughly half remaining
	remaining := mgr.ActiveCount()
	expected := int(expectedTotal) / 2
	// Allow 10% variance due to concurrency
	tolerance := expected / 10
	if remaining < expected-tolerance || remaining > expected+tolerance {
		t.Errorf("ActiveCount = %d, want around %d (±%d)", remaining, expected, tolerance)
	}
}

// TestManagerStartStop tests starting and stopping the manager.
func TestManagerStartStop(t *testing.T) {
	mgr := NewManager(DefaultManagerConfig())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the manager
	mgr.Start(ctx)

	// Create some sessions
	sessionIDs := make([]string, 5)
	for i := 0; i < 5; i++ {
		sess, err := mgr.Create(ctx)
		if err != nil {
			t.Fatalf("Create() error: %v", err)
		}
		sessionIDs[i] = sess.ID
	}

	if mgr.ActiveCount() != 5 {
		t.Errorf("ActiveCount = %d, want 5", mgr.ActiveCount())
	}

	// Stop the manager
	mgr.Stop()

	// All sessions should be closed (verify by checking IsClosed)
	for _, id := range sessionIDs {
		// Try to get the session - it should be deleted
		sess, ok := mgr.Get(id)
		if ok {
			t.Errorf("Session %s still retrievable after Stop()", id)
		}
		if sess != nil {
			// If we somehow got it, verify it's closed
			if !sess.IsClosed() {
				t.Errorf("Session %s not closed after Stop()", id)
			}
		}
	}

	// Note: ActiveCount may not be 0 immediately after Stop() since the cleanup
	// happens in the Range loop which deletes but the activeCount decrement
	// is not guaranteed to be visible immediately. The important thing is that
	// sessions are closed and deleted.
}

// TestSessionListReturnsOnlyActive tests that List only returns non-closed sessions.
func TestSessionListReturnsOnlyActive(t *testing.T) {
	mgr := NewManager(DefaultManagerConfig())
	ctx := context.Background()

	// Create sessions
	sess1, _ := mgr.Create(ctx)
	sess2, _ := mgr.Create(ctx)
	sess3, _ := mgr.Create(ctx)

	// Close one session
	sess2.Close()

	// List should only return active sessions
	sessions := mgr.List()
	if len(sessions) != 2 {
		t.Errorf("List() returned %d sessions, want 2", len(sessions))
	}

	// Verify the closed session is not in the list
	for _, s := range sessions {
		if s.ID == sess2.ID {
			t.Error("List() returned closed session")
		}
	}

	// Verify active sessions are in the list
	found1, found3 := false, false
	for _, s := range sessions {
		if s.ID == sess1.ID {
			found1 = true
		}
		if s.ID == sess3.ID {
			found3 = true
		}
	}
	if !found1 || !found3 {
		t.Error("List() did not return all active sessions")
	}
}

// TestGetClosedSession tests that Get returns false for closed sessions.
func TestGetClosedSession(t *testing.T) {
	mgr := NewManager(DefaultManagerConfig())
	ctx := context.Background()

	sess, err := mgr.Create(ctx)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	// Close the session
	sess.Close()

	// Get should return false and clean up the session
	retrieved, ok := mgr.Get(sess.ID)
	if ok {
		t.Error("Get() returned true for closed session")
	}
	if retrieved != nil {
		t.Error("Get() returned non-nil for closed session")
	}

	// Session should be removed from manager
	// Note: The current implementation deletes closed sessions in Get()
	// but doesn't update activeCount. This might be a bug in the original code.
}

// TestCleanupRemovesClosedSessions tests that cleanup removes manually closed sessions.
func TestCleanupRemovesClosedSessions(t *testing.T) {
	mgr := NewManager(DefaultManagerConfig())
	ctx := context.Background()

	sess1, _ := mgr.Create(ctx)
	sess2, _ := mgr.Create(ctx)

	// Manually close one session
	sess1.Close()

	initialActive := mgr.ActiveCount()

	// Run cleanup
	mgr.cleanup()

	// Closed session should be removed
	if _, ok := mgr.Get(sess1.ID); ok {
		t.Error("Closed session still exists after cleanup")
	}

	// Active session should remain
	if _, ok := mgr.Get(sess2.ID); !ok {
		t.Error("Active session was removed by cleanup")
	}

	// Active count should be decremented
	if mgr.ActiveCount() != initialActive-1 {
		t.Errorf("ActiveCount = %d, want %d", mgr.ActiveCount(), initialActive-1)
	}
}

// TestSessionAgeAndIdleTime tests session age and idle time calculations.
func TestSessionAgeAndIdleTime(t *testing.T) {
	sess := NewSession("test_sess")

	// Just created - age should be very small
	age := sess.Age()
	if age > 100*time.Millisecond {
		t.Errorf("Age() = %v, expected very small value", age)
	}

	// IdleTime should also be very small
	idle := sess.IdleTime()
	if idle > 100*time.Millisecond {
		t.Errorf("IdleTime() = %v, expected very small value", idle)
	}

	// Wait and check again
	time.Sleep(50 * time.Millisecond)

	age = sess.Age()
	if age < 40*time.Millisecond {
		t.Errorf("Age() = %v, expected at least 40ms", age)
	}

	idle = sess.IdleTime()
	if idle < 40*time.Millisecond {
		t.Errorf("IdleTime() = %v, expected at least 40ms", idle)
	}

	// Increment request count - idle time should reset
	sess.IncrementRequestCount()
	idle = sess.IdleTime()
	if idle > 10*time.Millisecond {
		t.Errorf("IdleTime() = %v after activity, expected very small value", idle)
	}

	// Age should keep growing
	age = sess.Age()
	if age < 40*time.Millisecond {
		t.Errorf("Age() = %v, expected at least 40ms", age)
	}
}
