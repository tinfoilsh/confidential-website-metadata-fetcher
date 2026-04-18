package cache

import (
	"container/list"
	"strings"
	"sync"
	"time"

	"net/url"
)

// trackingParamPrefixes and trackingParamExact define query parameters that
// are stripped during cache-key normalization so the same logical URL hits
// the same entry regardless of referrer tracking noise.
var trackingParamPrefixes = []string{"utm_"}

var trackingParamExact = map[string]struct{}{
	"gclid":   {},
	"fbclid":  {},
	"mc_cid":  {},
	"mc_eid":  {},
	"ref":     {},
	"ref_src": {},
	"ref_url": {},
}

// NormalizeURL returns a canonical string for cache lookups. It drops the
// fragment, lowercases the host, and removes common tracking parameters.
// Callers may pass either the raw URL or a pre-parsed *url.URL.
func NormalizeURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Fragment = ""
	q := parsed.Query()
	for key := range q {
		lower := strings.ToLower(key)
		if _, ok := trackingParamExact[lower]; ok {
			q.Del(key)
			continue
		}
		for _, prefix := range trackingParamPrefixes {
			if strings.HasPrefix(lower, prefix) {
				q.Del(key)
				break
			}
		}
	}
	parsed.RawQuery = q.Encode()
	return parsed.String()
}

type entry[V any] struct {
	key       string
	value     V
	expiresAt time.Time
	elem      *list.Element
}

// LRU is a small thread-safe LRU cache with per-entry TTLs. Generics keep the
// type parameter close to the call site while still being ergonomic.
type LRU[V any] struct {
	mu    sync.Mutex
	max   int
	ttl   time.Duration
	list  *list.List
	items map[string]*entry[V]
}

func New[V any](max int, ttl time.Duration) *LRU[V] {
	if max <= 0 {
		max = 1
	}
	return &LRU[V]{
		max:   max,
		ttl:   ttl,
		list:  list.New(),
		items: make(map[string]*entry[V], max),
	}
}

func (c *LRU[V]) Get(key string) (V, bool) {
	var zero V
	c.mu.Lock()
	defer c.mu.Unlock()
	ent, ok := c.items[key]
	if !ok {
		return zero, false
	}
	if time.Now().After(ent.expiresAt) {
		c.list.Remove(ent.elem)
		delete(c.items, key)
		return zero, false
	}
	c.list.MoveToFront(ent.elem)
	return ent.value, true
}

func (c *LRU[V]) Set(key string, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if ent, ok := c.items[key]; ok {
		ent.value = value
		ent.expiresAt = now.Add(c.ttl)
		c.list.MoveToFront(ent.elem)
		return
	}
	ent := &entry[V]{key: key, value: value, expiresAt: now.Add(c.ttl)}
	ent.elem = c.list.PushFront(ent)
	c.items[key] = ent
	for c.list.Len() > c.max {
		oldest := c.list.Back()
		if oldest == nil {
			break
		}
		c.list.Remove(oldest)
		delete(c.items, oldest.Value.(*entry[V]).key)
	}
}
