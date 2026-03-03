package server

import "sync"

// userSemaphore limits concurrent AI calls per user to avoid rate limits.
type userSemaphore struct {
	mu       sync.Mutex
	maxConc  int
	channels map[string]chan struct{}
}

func newUserSemaphore(maxConcurrent int) *userSemaphore {
	return &userSemaphore{
		maxConc:  maxConcurrent,
		channels: make(map[string]chan struct{}),
	}
}

func (s *userSemaphore) acquire(userID string) {
	s.mu.Lock()
	ch, ok := s.channels[userID]
	if !ok {
		ch = make(chan struct{}, s.maxConc)
		s.channels[userID] = ch
	}
	s.mu.Unlock()

	ch <- struct{}{}
}

func (s *userSemaphore) release(userID string) {
	s.mu.Lock()
	ch := s.channels[userID]
	s.mu.Unlock()

	<-ch
}
