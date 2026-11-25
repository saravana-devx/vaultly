package api

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// dekCache keeps unwrapped project keys in memory for a short time so that each
// request does not need a round-trip to OpenBao. TTL 0 disables caching.
type dekCache struct {
	mu  sync.Mutex
	ttl time.Duration
	m   map[uuid.UUID]cachedDEK
}

type cachedDEK struct {
	key     []byte
	expires time.Time
}

func newDEKCache(ttl time.Duration) *dekCache {
	return &dekCache{ttl: ttl, m: map[uuid.UUID]cachedDEK{}}
}

func (c *dekCache) get(id uuid.UUID) ([]byte, bool) {
	if c.ttl <= 0 {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[id]
	if !ok || time.Now().After(e.expires) {
		delete(c.m, id)
		return nil, false
	}
	return e.key, true
}

func (c *dekCache) put(id uuid.UUID, key []byte) {
	if c.ttl <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.m) > 1024 {
		now := time.Now()
		for k, v := range c.m {
			if now.After(v.expires) {
				delete(c.m, k)
			}
		}
	}
	c.m[id] = cachedDEK{key: key, expires: time.Now().Add(c.ttl)}
}

func (c *dekCache) forget(id uuid.UUID) {
	c.mu.Lock()
	delete(c.m, id)
	c.mu.Unlock()
}
