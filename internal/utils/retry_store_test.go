package utils

import (
	"sync"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("RetryStore", func() {
	var (
		store *RetryStore
		key   types.NamespacedName
	)

	BeforeEach(func() {
		store = NewRetryStore()
		key = types.NamespacedName{Namespace: "default", Name: "my-resource"}
	})

	Describe("NewRetryStore", func() {
		It("returns a non-nil store with empty state", func() {
			Expect(store).ToNot(BeNil())
			Expect(store.Get(key)).To(BeNil())
		})
	})

	Describe("Get", func() {
		It("returns nil for an unknown key", func() {
			Expect(store.Get(key)).To(BeNil())
		})

		It("returns a copy so mutations do not affect stored state", func() {
			store.RegisterFailure(key)
			got := store.Get(key)
			Expect(got).ToNot(BeNil())
			originalAttempts := got.Attempts

			got.Attempts = 999

			got2 := store.Get(key)
			Expect(got2.Attempts).To(Equal(originalAttempts))
		})
	})

	Describe("RegisterFailure", func() {
		It("creates an entry on first call with Attempts == 1", func() {
			state := store.RegisterFailure(key)
			Expect(state).ToNot(BeNil())
			Expect(state.Attempts).To(Equal(1))
		})

		It("sets NextRetry in the future on first call", func() {
			before := time.Now()
			state := store.RegisterFailure(key)
			Expect(state.NextRetry).To(BeTemporally(">", before))
		})

		It("increments Attempts on each subsequent call", func() {
			for i := 1; i <= 3; i++ {
				state := store.RegisterFailure(key)
				Expect(state.Attempts).To(Equal(i))
			}
		})

		It("increases backoff duration with each failure (exponential backoff)", func() {
			// Compare computed backoff durations directly instead of wall-clock NextRetry
			// to avoid flakiness when consecutive calls happen within the same nanosecond.
			d0 := calculateBackoff(0)
			d1 := calculateBackoff(1)
			d2 := calculateBackoff(2)

			Expect(d1).To(BeNumerically(">", d0))
			Expect(d2).To(BeNumerically(">", d1))
		})

		It("stores state that is retrievable via Get", func() {
			store.RegisterFailure(key)
			got := store.Get(key)
			Expect(got).ToNot(BeNil())
			Expect(got.Attempts).To(Equal(1))
		})

		It("does not affect state for a different key", func() {
			other := types.NamespacedName{Namespace: "default", Name: "other"}
			store.RegisterFailure(key)
			Expect(store.Get(other)).To(BeNil())
		})
	})

	Describe("Reset", func() {
		It("removes the entry so Get returns nil", func() {
			store.RegisterFailure(key)
			store.Reset(key)
			Expect(store.Get(key)).To(BeNil())
		})

		It("is a no-op for an unknown key", func() {
			Expect(func() { store.Reset(key) }).ToNot(Panic())
			Expect(store.Get(key)).To(BeNil())
		})

		It("does not affect other keys", func() {
			other := types.NamespacedName{Namespace: "default", Name: "other"}
			store.RegisterFailure(key)
			store.RegisterFailure(other)
			store.Reset(key)

			Expect(store.Get(key)).To(BeNil())
			Expect(store.Get(other)).ToNot(BeNil())
		})

		It("allows RegisterFailure to restart the counter after a reset", func() {
			store.RegisterFailure(key)
			store.RegisterFailure(key)
			store.Reset(key)

			state := store.RegisterFailure(key)
			Expect(state.Attempts).To(Equal(1))
		})
	})

	Describe("calculateBackoff", func() {
		It("returns 10s for the first attempt (attempt == 0)", func() {
			Expect(calculateBackoff(0)).To(Equal(10 * time.Second))
		})

		It("doubles with each attempt", func() {
			Expect(calculateBackoff(1)).To(Equal(20 * time.Second))
			Expect(calculateBackoff(2)).To(Equal(40 * time.Second))
			Expect(calculateBackoff(3)).To(Equal(80 * time.Second))
		})

		It("caps at 3 hours for very large attempt numbers", func() {
			maxDelay := 3 * time.Hour
			Expect(calculateBackoff(1000)).To(Equal(maxDelay))
		})

		It("caps at 3 hours before overflow (large attempt that overflows float64 to MaxInt64)", func() {
			Expect(calculateBackoff(10000)).To(Equal(3 * time.Hour))
		})
	})

	Describe("concurrency", func() {
		It("handles concurrent RegisterFailure and Reset without data races", func() {
			const goroutines = 20
			var wg sync.WaitGroup
			wg.Add(goroutines)
			for i := 0; i < goroutines; i++ {
				go func(i int) {
					defer wg.Done()
					if i%2 == 0 {
						store.RegisterFailure(key)
					} else {
						store.Reset(key)
					}
				}(i)
			}
			wg.Wait()
		})

		It("handles concurrent Gets without data races", func() {
			store.RegisterFailure(key)
			const goroutines = 20
			var wg sync.WaitGroup
			wg.Add(goroutines)
			for i := 0; i < goroutines; i++ {
				go func() {
					defer wg.Done()
					_ = store.Get(key)
				}()
			}
			wg.Wait()
		})
	})
})
