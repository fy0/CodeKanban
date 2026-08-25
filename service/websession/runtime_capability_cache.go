package websession

import (
	"sync"
	"time"
)

const (
	runtimeCapabilityFailureBackoff    = time.Minute
	runtimeCapabilityMaxFailureBackoff = 30 * time.Minute
)

type runtimeCapabilityCachePolicy struct {
	successTTL     time.Duration
	failureBackoff time.Duration
	maxBackoff     time.Duration
}

type runtimeCapabilityFlight struct {
	done chan struct{}
}

type runtimeCapabilityCache[T any] struct {
	mu           sync.Mutex
	value        T
	expiresAt    time.Time
	loaded       bool
	failureCount int
	inFlight     *runtimeCapabilityFlight
}

func (c *runtimeCapabilityCache[T]) get(
	force bool,
	policy runtimeCapabilityCachePolicy,
	clone func(T) T,
	probe func() (T, error),
) T {
	now := time.Now()
	c.mu.Lock()
	if c.loaded && !force {
		cached := clone(c.value)
		if now.Before(c.expiresAt) {
			c.mu.Unlock()
			return cached
		}
		if c.inFlight == nil {
			flight := &runtimeCapabilityFlight{done: make(chan struct{})}
			c.inFlight = flight
			c.mu.Unlock()
			go c.refresh(flight, policy, clone, probe)
			return cached
		}
		c.mu.Unlock()
		return cached
	}

	if c.inFlight != nil {
		flight := c.inFlight
		c.mu.Unlock()
		<-flight.done
		return c.snapshot(clone)
	}

	flight := &runtimeCapabilityFlight{done: make(chan struct{})}
	c.inFlight = flight
	c.mu.Unlock()
	c.refresh(flight, policy, clone, probe)
	return c.snapshot(clone)
}

func (c *runtimeCapabilityCache[T]) snapshot(clone func(T) T) T {
	c.mu.Lock()
	defer c.mu.Unlock()
	return clone(c.value)
}

func (c *runtimeCapabilityCache[T]) refresh(
	flight *runtimeCapabilityFlight,
	policy runtimeCapabilityCachePolicy,
	clone func(T) T,
	probe func() (T, error),
) {
	value, err := probe()
	now := time.Now()

	c.mu.Lock()
	if err == nil {
		c.value = clone(value)
		c.loaded = true
		c.failureCount = 0
		c.expiresAt = now.Add(policy.successTTL)
	} else {
		if !c.loaded {
			c.value = clone(value)
			c.loaded = true
		}
		c.failureCount++
		c.expiresAt = now.Add(runtimeCapabilityBackoff(policy, c.failureCount))
	}
	if c.inFlight == flight {
		c.inFlight = nil
	}
	close(flight.done)
	c.mu.Unlock()
}

func runtimeCapabilityBackoff(policy runtimeCapabilityCachePolicy, failureCount int) time.Duration {
	backoff := policy.failureBackoff
	if backoff <= 0 {
		backoff = runtimeCapabilityFailureBackoff
	}
	maxBackoff := policy.maxBackoff
	if maxBackoff <= 0 {
		maxBackoff = runtimeCapabilityMaxFailureBackoff
	}
	for attempt := 1; attempt < failureCount && backoff < maxBackoff; attempt++ {
		if backoff > maxBackoff/2 {
			return maxBackoff
		}
		backoff *= 2
	}
	if backoff > maxBackoff {
		return maxBackoff
	}
	return backoff
}
