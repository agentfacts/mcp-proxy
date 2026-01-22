package session

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// Manager handles session lifecycle and storage.
type Manager struct {
	sessions sync.Map // map[string]*Session

	// Configuration
	sessionTTL    time.Duration
	cleanupTicker *time.Ticker
	maxSessions   int

	// Metrics
	mu           sync.RWMutex
	activeCount  int
	totalCreated int64

	// Lifecycle
	done chan struct{}
}

// ManagerConfig holds session manager configuration.
type ManagerConfig struct {
	SessionTTL      time.Duration
	CleanupInterval time.Duration
	MaxSessions     int
}

// DefaultManagerConfig returns sensible defaults.
func DefaultManagerConfig() ManagerConfig {
	return ManagerConfig{
		SessionTTL:      2 * time.Hour,
		CleanupInterval: 1 * time.Minute,
		MaxSessions:     10000,
	}
}

// NewManager creates a new session manager.
func NewManager(cfg ManagerConfig) *Manager {
	if cfg.SessionTTL == 0 {
		cfg.SessionTTL = 2 * time.Hour
	}
	if cfg.CleanupInterval == 0 {
		cfg.CleanupInterval = 1 * time.Minute
	}
	if cfg.MaxSessions == 0 {
		cfg.MaxSessions = 10000
	}

	return &Manager{
		sessionTTL:  cfg.SessionTTL,
		maxSessions: cfg.MaxSessions,
		done:        make(chan struct{}),
	}
}

// Start begins the background cleanup goroutine.
func (m *Manager) Start(ctx context.Context) {
	m.cleanupTicker = time.NewTicker(1 * time.Minute)

	go func() {
		for {
			select {
			case <-ctx.Done():
				m.cleanupTicker.Stop()
				return
			case <-m.done:
				m.cleanupTicker.Stop()
				return
			case <-m.cleanupTicker.C:
				m.cleanup()
			}
		}
	}()

	log.Info().
		Dur("session_ttl", m.sessionTTL).
		Int("max_sessions", m.maxSessions).
		Msg("Session manager started")
}

// Stop shuts down the session manager.
func (m *Manager) Stop() {
	close(m.done)

	// Close all active sessions
	m.sessions.Range(func(key, value any) bool {
		if sess, ok := value.(*Session); ok {
			sess.Close()
		}
		m.sessions.Delete(key)
		return true
	})

	log.Info().Msg("Session manager stopped")
}

// Create creates a new session and returns it.
func (m *Manager) Create(ctx context.Context) (*Session, error) {
	// Use a single lock to prevent race condition between check and increment
	m.mu.Lock()

	// Check max sessions limit
	if m.activeCount >= m.maxSessions {
		m.mu.Unlock()
		log.Warn().Int("max", m.maxSessions).Msg("Max sessions limit reached")
		return nil, ErrMaxSessionsReached
	}

	// Generate unique session ID using full UUID for maximum entropy (122 bits)
	// Truncating UUIDs significantly reduces entropy and security
	sessionID := "sess_" + uuid.New().String()

	// Create session
	sess := NewSession(sessionID)

	// Store session and update metrics atomically
	m.sessions.Store(sessionID, sess)
	m.activeCount++
	m.totalCreated++
	m.mu.Unlock()

	log.Debug().
		Str("session_id", sessionID).
		Msg("Session created")

	return sess, nil
}

// Get retrieves a session by ID.
func (m *Manager) Get(sessionID string) (*Session, bool) {
	value, ok := m.sessions.Load(sessionID)
	if !ok {
		return nil, false
	}

	sess, ok := value.(*Session)
	if !ok {
		return nil, false
	}

	// Check if session is closed
	if sess.IsClosed() {
		m.Delete(sessionID)
		return nil, false
	}

	return sess, true
}

// Delete removes a session.
func (m *Manager) Delete(sessionID string) {
	value, loaded := m.sessions.LoadAndDelete(sessionID)
	if loaded {
		if sess, ok := value.(*Session); ok {
			sess.Close()
		}

		m.mu.Lock()
		m.activeCount--
		m.mu.Unlock()

		log.Debug().
			Str("session_id", sessionID).
			Msg("Session deleted")
	}
}

// IncrementRequestCount increments the request count for a session.
func (m *Manager) IncrementRequestCount(sessionID string) int {
	sess, ok := m.Get(sessionID)
	if !ok {
		return 0
	}
	return sess.IncrementRequestCount()
}

// ActiveCount returns the number of active sessions.
func (m *Manager) ActiveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activeCount
}

// TotalCreated returns the total number of sessions created.
func (m *Manager) TotalCreated() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.totalCreated
}

// cleanup removes expired and idle sessions.
func (m *Manager) cleanup() {
	var expired, idle int

	m.sessions.Range(func(key, value any) bool {
		sessionID, _ := key.(string)
		sess, ok := value.(*Session)
		if !ok {
			m.sessions.Delete(key)
			return true
		}

		// Remove closed sessions
		if sess.IsClosed() {
			m.sessions.Delete(key)
			m.mu.Lock()
			m.activeCount--
			m.mu.Unlock()
			return true
		}

		// Remove sessions that exceed TTL
		if sess.Age() > m.sessionTTL {
			sess.Close()
			m.sessions.Delete(key)
			m.mu.Lock()
			m.activeCount--
			m.mu.Unlock()
			expired++
			log.Debug().
				Str("session_id", sessionID).
				Dur("age", sess.Age()).
				Msg("Session expired")
			return true
		}

		// Remove sessions idle for more than half the TTL
		if sess.IdleTime() > m.sessionTTL/2 {
			sess.Close()
			m.sessions.Delete(key)
			m.mu.Lock()
			m.activeCount--
			m.mu.Unlock()
			idle++
			log.Debug().
				Str("session_id", sessionID).
				Dur("idle_time", sess.IdleTime()).
				Msg("Session idle timeout")
			return true
		}

		return true
	})

	if expired > 0 || idle > 0 {
		log.Info().
			Int("expired", expired).
			Int("idle", idle).
			Int("active", m.ActiveCount()).
			Msg("Session cleanup completed")
	}
}

// List returns all active sessions (for debugging/admin).
func (m *Manager) List() []*Session {
	var sessions []*Session
	m.sessions.Range(func(key, value any) bool {
		if sess, ok := value.(*Session); ok {
			if !sess.IsClosed() {
				sessions = append(sessions, sess)
			}
		}
		return true
	})
	return sessions
}

// Errors
var (
	ErrMaxSessionsReached = &SessionError{Message: "maximum sessions limit reached"}
	ErrSessionNotFound    = &SessionError{Message: "session not found"}
)

// SessionError represents a session-related error.
type SessionError struct {
	Message string
}

func (e *SessionError) Error() string {
	return e.Message
}
