package keyedlock

import "sync"

// Locker serializes work for a single key while allowing different keys to run
// concurrently.
type Locker struct {
	mu    sync.Mutex
	locks map[string]*entry
}

type entry struct {
	mu   sync.Mutex
	refs int
}

func (l *Locker) Lock(key string) func() {
	l.mu.Lock()
	if l.locks == nil {
		l.locks = map[string]*entry{}
	}
	e := l.locks[key]
	if e == nil {
		e = &entry{}
		l.locks[key] = e
	}
	e.refs++
	l.mu.Unlock()

	e.mu.Lock()
	return func() {
		e.mu.Unlock()

		l.mu.Lock()
		e.refs--
		if e.refs == 0 {
			delete(l.locks, key)
		}
		l.mu.Unlock()
	}
}
