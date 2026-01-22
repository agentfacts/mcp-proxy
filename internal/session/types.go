package session

import (
	"context"
	"sync"
	"time"
)

// Session represents an active client connection session.
type Session struct {
	// ID is the unique session identifier
	ID string `json:"id"`

	// CreatedAt is when the session was established
	CreatedAt time.Time `json:"created_at"`

	// LastActivityAt is the time of the last request
	LastActivityAt time.Time `json:"last_activity_at"`

	// RequestCount is the total number of requests in this session
	RequestCount int `json:"request_count"`

	// AgentID is the identifier of the connected agent (from config or AgentFacts)
	AgentID string `json:"agent_id"`

	// AgentName is the human-readable agent name
	AgentName string `json:"agent_name,omitempty"`

	// Capabilities are the agent's granted capabilities
	Capabilities []string `json:"capabilities,omitempty"`

	// IdentityVerified indicates if AgentFacts token was verified
	IdentityVerified bool `json:"identity_verified"`

	// DID is the agent's decentralized identifier (if verified)
	DID string `json:"did,omitempty"`

	// SourceIP is the client's IP address
	SourceIP string `json:"source_ip,omitempty"`

	// UserAgent is the client's user agent string
	UserAgent string `json:"user_agent,omitempty"`

	// MessageChan is used to send SSE messages back to the client
	MessageChan chan []byte `json:"-"`

	// Done is closed when the session is terminated
	Done chan struct{} `json:"-"`

	// mu protects concurrent access to session fields
	mu sync.RWMutex `json:"-"`
}

// NewSession creates a new session with the given ID.
func NewSession(id string) *Session {
	return &Session{
		ID:             id,
		CreatedAt:      time.Now(),
		LastActivityAt: time.Now(),
		RequestCount:   0,
		MessageChan:    make(chan []byte, 100), // Buffered channel for messages
		Done:           make(chan struct{}),
	}
}

// IncrementRequestCount atomically increments the request counter and returns the new value.
func (s *Session) IncrementRequestCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.RequestCount++
	s.LastActivityAt = time.Now()
	return s.RequestCount
}

// GetRequestCount returns the current request count.
func (s *Session) GetRequestCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.RequestCount
}

// SetAgent sets the agent identity information.
func (s *Session) SetAgent(agentID, agentName string, capabilities []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.AgentID = agentID
	s.AgentName = agentName
	s.Capabilities = capabilities
}

// SetIdentity sets the verified identity information.
func (s *Session) SetIdentity(verified bool, did string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.IdentityVerified = verified
	s.DID = did
}

// SetClientInfo sets the client connection information.
func (s *Session) SetClientInfo(sourceIP, userAgent string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SourceIP = sourceIP
	s.UserAgent = userAgent
}

// Close closes the session channels.
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.Done:
		// Already closed
	default:
		close(s.Done)
	}
}

// IsClosed returns true if the session has been closed.
func (s *Session) IsClosed() bool {
	select {
	case <-s.Done:
		return true
	default:
		return false
	}
}

// SendMessage sends a message to the client via the message channel.
// Returns false if the session is closed or the channel is full.
func (s *Session) SendMessage(msg []byte) bool {
	select {
	case <-s.Done:
		return false
	case s.MessageChan <- msg:
		return true
	default:
		// Channel full, message dropped
		return false
	}
}

// Context returns a context that is cancelled when the session is closed.
func (s *Session) Context() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-s.Done
		cancel()
	}()
	return ctx
}

// Age returns how long the session has been active.
func (s *Session) Age() time.Duration {
	return time.Since(s.CreatedAt)
}

// IdleTime returns how long since the last activity.
func (s *Session) IdleTime() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return time.Since(s.LastActivityAt)
}
