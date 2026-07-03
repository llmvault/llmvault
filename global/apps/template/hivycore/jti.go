// DO NOT EDIT — hivycore is the Hivy platform core. See doc.go.

package hivycore

import (
	"container/list"
	"sync"
	"time"
)

// jtiGuard enforces one-time use of launch-token JTIs with a bounded
// in-memory LRU. Entries share a uniform TTL, so insertion order equals
// expiry order and pruning walks from the front. Launch tokens live 60s;
// the retention window just has to exceed the longest possible token life.
type jtiGuard struct {
	mu      sync.Mutex
	ttl     time.Duration
	max     int
	order   *list.List // front = oldest
	entries map[string]*list.Element
	now     func() time.Time
}

type jtiEntry struct {
	id      string
	expires time.Time
}

func newJTIGuard(ttl time.Duration, max int) *jtiGuard {
	return &jtiGuard{
		ttl:     ttl,
		max:     max,
		order:   list.New(),
		entries: make(map[string]*list.Element),
		now:     time.Now,
	}
}

// firstUse records the JTI and reports whether this was its first
// appearance inside the retention window. A false return means replay.
func (g *jtiGuard) firstUse(id string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := g.now()
	for e := g.order.Front(); e != nil; {
		entry := e.Value.(*jtiEntry)
		if entry.expires.After(now) {
			break
		}
		next := e.Next()
		g.order.Remove(e)
		delete(g.entries, entry.id)
		e = next
	}

	if _, seen := g.entries[id]; seen {
		return false
	}
	if g.order.Len() >= g.max {
		oldest := g.order.Front()
		if oldest != nil {
			g.order.Remove(oldest)
			delete(g.entries, oldest.Value.(*jtiEntry).id)
		}
	}
	g.entries[id] = g.order.PushBack(&jtiEntry{id: id, expires: now.Add(g.ttl)})
	return true
}
