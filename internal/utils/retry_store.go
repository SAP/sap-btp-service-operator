package utils

import (
	"math"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/types"
)

type RetryState struct {
	Attempts      int
	NextRetry     time.Time
	CorrelationID string
}

type RetryStore struct {
	mu        sync.Mutex
	state     map[types.NamespacedName]*RetryState
	baseDelay time.Duration
	maxDelay  time.Duration
}

func NewRetryStore(baseDelay, maxDelay time.Duration) *RetryStore {
	return &RetryStore{
		state:     make(map[types.NamespacedName]*RetryState),
		baseDelay: baseDelay,
		maxDelay:  maxDelay,
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
	ccopy := *s
	return &ccopy
}

func (r *RetryStore) RegisterFailure(key types.NamespacedName, correlationID string) *RetryState {
	r.mu.Lock()
	defer r.mu.Unlock()

	s, exists := r.state[key]
	if !exists {
		s = &RetryState{CorrelationID: correlationID}
		r.state[key] = s
	}

	s.Attempts++
	backoff := r.calculateBackoff(s.Attempts)
	s.NextRetry = time.Now().Add(backoff)
	return s
}

func (r *RetryStore) Reset(key types.NamespacedName) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.state, key)
}

func (r *RetryStore) calculateBackoff(attempt int) time.Duration {
	backoff := float64(r.baseDelay.Nanoseconds()) * math.Pow(2, float64(attempt))
	if backoff > math.MaxInt64 {
		return r.maxDelay
	}

	calculated := time.Duration(backoff)
	if calculated > r.maxDelay {
		return r.maxDelay
	}

	return calculated
}
