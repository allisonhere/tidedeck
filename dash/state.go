package dash

import "sync"

// State holds a panel's model behind a read-write lock. Refresh runs off the
// UI goroutine while View runs on it, so every panel that fetches needs this
// discipline; embedding State writes it once instead of fourteen times.
//
// It is also what keeps a model type out of the Panel interface: Load returns
// a copy and Store replaces one, and neither signature escapes the panel.
type State[T any] struct {
	mu    sync.RWMutex
	value T
}

// Load returns the current value.
func (s *State[T]) Load() T {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.value
}

// Store replaces the current value.
func (s *State[T]) Store(value T) {
	s.mu.Lock()
	s.value = value
	s.mu.Unlock()
}

// Update applies a function to the value under the write lock, for the cases
// where the new value depends on the old one - a rolling history, say.
func (s *State[T]) Update(fn func(T) T) {
	s.mu.Lock()
	s.value = fn(s.value)
	s.mu.Unlock()
}
