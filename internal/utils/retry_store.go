package utils

import (
	"k8s.io/apimachinery/pkg/types"
	"math"
	"sync"
	"time"
)

type RetryState struct {
	Attempts  int
	NextRetry time.Time
}

type RetryStore struct {
	mu    sync.Mutex
	state map[types.NamespacedName]*RetryState
}

func NewRetryStore() *RetryStore {
	return &RetryStore{
		state: make(map[types.NamespacedName]*RetryState),
	}
}

func (r *RetryStore) Get(key types.NamespacedName) *RetryState {
	r.mu.Lock()
	defer r.mu.Unlock()

	s, exists := r.state[key]
	if !exists {
		return nil
	}

	// return copy to avoid races
	copy := *s
	return &copy
}

func (r *RetryStore) RegisterFailure(key types.NamespacedName) *RetryState {
	r.mu.Lock()
	defer r.mu.Unlock()

	s, exists := r.state[key]
	if !exists {
		s = &RetryState{}
		r.state[key] = s
	}

	backoff := calculateBackoff(s.Attempts)
	s.NextRetry = time.Now().Add(backoff)
	s.Attempts++
	return s
}

func (r *RetryStore) Reset(key types.NamespacedName) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.state, key)
}

func calculateBackoff(attempt int) time.Duration {
	base := 10 * time.Second
	maxDelay := 3 * time.Hour

	backoff := float64(base.Nanoseconds()) * math.Pow(2, float64(attempt))
	if backoff > math.MaxInt64 {
		return maxDelay
	}

	calculated := time.Duration(backoff)
	if calculated > maxDelay {
		return maxDelay
	}

	return calculated
}
