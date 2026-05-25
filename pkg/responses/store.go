package responses

import (
	"sync"
	"time"
)

// DefaultTTL is the default time-to-live for stored responses.
const DefaultTTL = 1 * time.Hour

// Store provides in-memory storage for responses with TTL-based expiration.
type Store struct {
	mu        sync.RWMutex
	responses map[string]*storedResponse
	ttl       time.Duration
	stopCh    chan struct{}
}

type storedResponse struct {
	response  *Response
	expiresAt time.Time
}

// NewStore creates a new response store with the given TTL.
func NewStore(ttl time.Duration) *Store {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	s := &Store{
		responses: make(map[string]*storedResponse),
		ttl:       ttl,
		stopCh:    make(chan struct{}),
	}
	// Start background cleanup goroutine.
	go s.cleanupLoop()
	return s
}

// Close stops the background cleanup goroutine. It must be called when the
// Store is no longer needed to avoid a goroutine leak.
func (s *Store) Close() {
	close(s.stopCh)
}

// Save stores a response.
func (s *Store) Save(resp *Response) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.responses[resp.ID] = &storedResponse{
		response:  resp,
		expiresAt: time.Now().Add(s.ttl),
	}
}

// Get retrieves a response by ID.
func (s *Store) Get(id string) (*Response, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	stored, ok := s.responses[id]
	if !ok {
		return nil, false
	}
	if time.Now().After(stored.expiresAt) {
		return nil, false
	}
	return stored.response, true
}

// Delete removes a response by ID.
func (s *Store) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.responses[id]; ok {
		delete(s.responses, id)
		return true
	}
	return false
}

// Update updates a response in place.
func (s *Store) Update(id string, updateFn func(*Response)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.responses[id]
	if !ok || time.Now().After(stored.expiresAt) {
		return false
	}
	updateFn(stored.response)
	// Refresh TTL on update
	stored.expiresAt = time.Now().Add(s.ttl)
	return true
}

// cleanupLoop periodically removes expired responses until Close is called.
func (s *Store) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.cleanup()
		case <-s.stopCh:
			return
		}
	}
}

// cleanup removes all expired responses.
func (s *Store) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for id, stored := range s.responses {
		if now.After(stored.expiresAt) {
			delete(s.responses, id)
		}
	}
}

// Count returns the number of stored responses.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.responses)
}

// GetResponseIDs returns all response IDs in the store (for testing purposes).
func (s *Store) GetResponseIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.responses))
	for id := range s.responses {
		ids = append(ids, id)
	}
	return ids
}
