package sync

import (
	"sync"
	"sync/atomic"
)

// RWLock is an optimized read-write lock
type RWLock struct {
	w       sync.Mutex
	readers atomic.Int32
	writers atomic.Int32
}

// NewRWLock creates a new RWLock
func NewRWLock() *RWLock {
	return &RWLock{}
}

// RLock acquires a read lock
func (l *RWLock) RLock() {
	for {
		// Wait for writers to finish
		for l.writers.Load() > 0 {
			// Spin
		}

		l.readers.Add(1)

		// Double-check no writer acquired the lock
		if l.writers.Load() == 0 {
			return
		}

		// A writer got the lock, release and retry
		l.readers.Add(-1)
	}
}

// RUnlock releases a read lock
func (l *RWLock) RUnlock() {
	l.readers.Add(-1)
}

// Lock acquires a write lock
func (l *RWLock) Lock() {
	l.w.Lock()
	l.writers.Add(1)

	// Wait for readers to finish
	for l.readers.Load() > 0 {
		// Spin
	}
}

// Unlock releases a write lock
func (l *RWLock) Unlock() {
	l.writers.Add(-1)
	l.w.Unlock()
}

// TryRLock attempts to acquire a read lock without blocking
func (l *RWLock) TryRLock() bool {
	if l.writers.Load() > 0 {
		return false
	}

	l.readers.Add(1)

	if l.writers.Load() > 0 {
		l.readers.Add(-1)
		return false
	}

	return true
}

// TryLock attempts to acquire a write lock without blocking
func (l *RWLock) TryLock() bool {
	if !l.w.TryLock() {
		return false
	}

	l.writers.Add(1)

	if l.readers.Load() > 0 {
		l.writers.Add(-1)
		l.w.Unlock()
		return false
	}

	return true
}

// ShardedLock provides sharded locking for high concurrency
type ShardedLock struct {
	shards    []*sync.RWMutex
	numShards int
}

// NewShardedLock creates a new sharded lock
func NewShardedLock(numShards int) *ShardedLock {
	if numShards <= 0 {
		numShards = 32
	}

	shards := make([]*sync.RWMutex, numShards)
	for i := range shards {
		shards[i] = &sync.RWMutex{}
	}

	return &ShardedLock{
		shards:    shards,
		numShards: numShards,
	}
}

// GetShard returns the shard for a given key
func (sl *ShardedLock) GetShard(key uint64) *sync.RWMutex {
	// #nosec G115 - numShards is always positive
	return sl.shards[key%uint64(sl.numShards)]
}

// Lock acquires a write lock on the shard for the key
func (sl *ShardedLock) Lock(key uint64) {
	sl.GetShard(key).Lock()
}

// Unlock releases the write lock on the shard for the key
func (sl *ShardedLock) Unlock(key uint64) {
	sl.GetShard(key).Unlock()
}

// RLock acquires a read lock on the shard for the key
func (sl *ShardedLock) RLock(key uint64) {
	sl.GetShard(key).RLock()
}

// RUnlock releases the read lock on the shard for the key
func (sl *ShardedLock) RUnlock(key uint64) {
	sl.GetShard(key).RUnlock()
}

// LockAll acquires write locks on all shards
func (sl *ShardedLock) LockAll() {
	for _, shard := range sl.shards {
		shard.Lock()
	}
}

// UnlockAll releases write locks on all shards
func (sl *ShardedLock) UnlockAll() {
	for _, shard := range sl.shards {
		shard.Unlock()
	}
}

// Semaphore implements a counting semaphore
type Semaphore struct {
	ch chan struct{}
}

// NewSemaphore creates a semaphore with the given capacity
func NewSemaphore(n int) *Semaphore {
	return &Semaphore{
		ch: make(chan struct{}, n),
	}
}

// Acquire acquires n permits
func (s *Semaphore) Acquire(n int) {
	for i := 0; i < n; i++ {
		s.ch <- struct{}{}
	}
}

// TryAcquire attempts to acquire a permit without blocking
func (s *Semaphore) TryAcquire() bool {
	select {
	case s.ch <- struct{}{}:
		return true
	default:
		return false
	}
}

// Release releases n permits
func (s *Semaphore) Release(n int) {
	for i := 0; i < n; i++ {
		<-s.ch
	}
}

// Available returns the number of available permits
func (s *Semaphore) Available() int {
	return cap(s.ch) - len(s.ch)
}
