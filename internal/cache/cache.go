// Package cache — простой потокобезопасный in-memory кэш с TTL.
package cache

import (
	"sync"
	"time"
)

// entry — значение + момент протухания. Параметризовано типом V (generics).
type entry[V any] struct {
	value     V
	expiresAt time.Time
}

// Cache — обобщённый кэш «строковый ключ → значение V» с единым TTL.
// Потокобезопасен: HTTP-хендлеры исполняются в разных горутинах, поэтому
// доступ к map защищён RWMutex (много читателей — мало писателей).
type Cache[V any] struct {
	mu  sync.RWMutex
	ttl time.Duration
	m   map[string]entry[V]
}

// New создаёт кэш с заданным временем жизни записей.
func New[V any](ttl time.Duration) *Cache[V] {
	return &Cache[V]{
		ttl: ttl,
		m:   make(map[string]entry[V]),
	}
}

// Get возвращает значение и true, если оно есть и ещё не протухло.
// На чтении берём RLock — несколько горутин могут читать параллельно.
func (c *Cache[V]) Get(key string) (V, bool) {
	c.mu.RLock()
	e, ok := c.m[key]
	c.mu.RUnlock()

	var zero V
	if !ok || time.Now().After(e.expiresAt) {
		return zero, false
	}
	return e.value, true
}

// Set кладёт значение с TTL от текущего момента. Запись — под полным Lock.
func (c *Cache[V]) Set(key string, value V) {
	c.mu.Lock()
	c.m[key] = entry[V]{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
	c.mu.Unlock()
}
