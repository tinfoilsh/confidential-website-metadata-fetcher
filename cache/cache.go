package cache

import (
	"container/list"
	"sync"
	"time"
)

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
