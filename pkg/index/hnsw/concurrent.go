package hnsw

import (
	"math"
	"sync"
	"sync/atomic"
)

// safeIntToUint32 converts an int to uint32 with bounds checking.
// Returns 0 if the value is negative or exceeds uint32 max.
func safeIntToUint32(v int) uint32 {
	if v < 0 || v > math.MaxUint32 {
		return 0
	}
	return uint32(v) //nolint:gosec
}

// safeIntToInt32 converts an int to int32 with bounds checking.
// Returns 0 if the value exceeds int32 range.
func safeIntToInt32(v int) int32 {
	if v < math.MinInt32 || v > math.MaxInt32 {
		return 0
	}
	return int32(v) //nolint:gosec
}

// NodeLock provides fine-grained locking for individual nodes
// This enables concurrent insertions to different parts of the graph
type NodeLock struct {
	locks []sync.RWMutex
	size  int
}

// NewNodeLock creates a new node lock pool with the specified number of shards
func NewNodeLock(numShards int) *NodeLock {
	if numShards < 1 {
		numShards = 256 // Default to 256 shards
	}
	return &NodeLock{
		locks: make([]sync.RWMutex, numShards),
		size:  numShards,
	}
}

// Lock acquires a write lock for the given node ID
func (nl *NodeLock) Lock(nodeID uint32) {
	nl.locks[nodeID%safeIntToUint32(nl.size)].Lock()
}

// Unlock releases the write lock for the given node ID
func (nl *NodeLock) Unlock(nodeID uint32) {
	nl.locks[nodeID%safeIntToUint32(nl.size)].Unlock()
}

// RLock acquires a read lock for the given node ID
func (nl *NodeLock) RLock(nodeID uint32) {
	nl.locks[nodeID%safeIntToUint32(nl.size)].RLock()
}

// RUnlock releases the read lock for the given node ID
func (nl *NodeLock) RUnlock(nodeID uint32) {
	nl.locks[nodeID%safeIntToUint32(nl.size)].RUnlock()
}

// LockMultiple acquires write locks for multiple nodes in order to prevent deadlock
func (nl *NodeLock) LockMultiple(nodeIDs []uint32) {
	// Sort to prevent deadlock (always acquire locks in consistent order)
	sorted := make([]uint32, len(nodeIDs))
	copy(sorted, nodeIDs)
	sortUint32(sorted)

	// Remove duplicates and lock
	var last uint32 = 0xFFFFFFFF
	for _, id := range sorted {
		shard := id % safeIntToUint32(nl.size)
		if shard != last {
			nl.locks[shard].Lock()
			last = shard
		}
	}
}

// UnlockMultiple releases write locks for multiple nodes
func (nl *NodeLock) UnlockMultiple(nodeIDs []uint32) {
	sorted := make([]uint32, len(nodeIDs))
	copy(sorted, nodeIDs)
	sortUint32(sorted)

	var last uint32 = 0xFFFFFFFF
	for _, id := range sorted {
		shard := id % safeIntToUint32(nl.size)
		if shard != last {
			nl.locks[shard].Unlock()
			last = shard
		}
	}
}

// sortUint32 sorts a slice of uint32 in ascending order
func sortUint32(arr []uint32) {
	// Simple insertion sort for small arrays
	for i := 1; i < len(arr); i++ {
		key := arr[i]
		j := i - 1
		for j >= 0 && arr[j] > key {
			arr[j+1] = arr[j]
			j--
		}
		arr[j+1] = key
	}
}

// InsertionTracker tracks concurrent insertions for progress reporting
type InsertionTracker struct {
	total     int64
	completed atomic.Int64
	failed    atomic.Int64
}

// NewInsertionTracker creates a new insertion tracker
func NewInsertionTracker(total int64) *InsertionTracker {
	return &InsertionTracker{
		total: total,
	}
}

// MarkCompleted increments the completed counter
func (t *InsertionTracker) MarkCompleted() {
	t.completed.Add(1)
}

// MarkFailed increments the failed counter
func (t *InsertionTracker) MarkFailed() {
	t.failed.Add(1)
}

// Progress returns the current progress (completed, failed, total)
func (t *InsertionTracker) Progress() (completed, failed, total int64) {
	return t.completed.Load(), t.failed.Load(), t.total
}

// ConnectionBuffer provides a lock-free buffer for pending connection updates
// This allows batch application of connection changes
type ConnectionBuffer struct {
	updates []connectionUpdate
	mu      sync.Mutex
}

type connectionUpdate struct {
	fromID uint32
	toID   uint32
	layer  int
}

// NewConnectionBuffer creates a new connection buffer
func NewConnectionBuffer(capacity int) *ConnectionBuffer {
	return &ConnectionBuffer{
		updates: make([]connectionUpdate, 0, capacity),
	}
}

// Add adds a connection update to the buffer
func (cb *ConnectionBuffer) Add(fromID, toID uint32, layer int) {
	cb.mu.Lock()
	cb.updates = append(cb.updates, connectionUpdate{
		fromID: fromID,
		toID:   toID,
		layer:  layer,
	})
	cb.mu.Unlock()
}

// Flush returns all pending updates and clears the buffer
func (cb *ConnectionBuffer) Flush() []connectionUpdate {
	cb.mu.Lock()
	updates := cb.updates
	cb.updates = make([]connectionUpdate, 0, cap(updates))
	cb.mu.Unlock()
	return updates
}

// Len returns the number of pending updates
func (cb *ConnectionBuffer) Len() int {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return len(cb.updates)
}

// AtomicEntryPoint provides atomic operations for the graph entry point
type AtomicEntryPoint struct {
	id    atomic.Uint32
	level atomic.Int32
}

// NewAtomicEntryPoint creates a new atomic entry point
func NewAtomicEntryPoint() *AtomicEntryPoint {
	ep := &AtomicEntryPoint{}
	ep.level.Store(-1) // -1 indicates empty graph
	return ep
}

// Get returns the current entry point ID and level
func (ep *AtomicEntryPoint) Get() (uint32, int) {
	return ep.id.Load(), int(ep.level.Load())
}

// Set atomically sets the entry point if the new level is higher
func (ep *AtomicEntryPoint) Set(id uint32, level int) bool {
	for {
		currentLevel := ep.level.Load()
		newLevel := safeIntToInt32(level)
		if newLevel <= currentLevel {
			return false
		}
		if ep.level.CompareAndSwap(currentLevel, newLevel) {
			ep.id.Store(id)
			return true
		}
	}
}

// SetIfEmpty sets the entry point only if the graph is empty
func (ep *AtomicEntryPoint) SetIfEmpty(id uint32, level int) bool {
	if ep.level.CompareAndSwap(-1, safeIntToInt32(level)) {
		ep.id.Store(id)
		return true
	}
	return false
}

// WorkerPool manages a pool of worker goroutines for concurrent operations
type WorkerPool struct {
	workers   int
	taskQueue chan func()
	done      chan struct{}
	wg        sync.WaitGroup
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(workers int) *WorkerPool {
	if workers < 1 {
		workers = 4
	}
	wp := &WorkerPool{
		workers:   workers,
		taskQueue: make(chan func(), workers*10),
		done:      make(chan struct{}),
	}
	wp.start()
	return wp
}

// start initializes the worker goroutines
func (wp *WorkerPool) start() {
	for i := 0; i < wp.workers; i++ {
		wp.wg.Add(1)
		go func() {
			defer wp.wg.Done()
			for {
				select {
				case task := <-wp.taskQueue:
					if task != nil {
						task()
					}
				case <-wp.done:
					return
				}
			}
		}()
	}
}

// Submit adds a task to the queue
func (wp *WorkerPool) Submit(task func()) {
	select {
	case wp.taskQueue <- task:
	case <-wp.done:
	}
}

// Stop stops all workers and waits for them to finish
func (wp *WorkerPool) Stop() {
	close(wp.done)
	wp.wg.Wait()
}

// Wait waits for all submitted tasks to complete
func (wp *WorkerPool) Wait() {
	// Submit sentinel tasks
	waiting := make(chan struct{}, wp.workers)
	for i := 0; i < wp.workers; i++ {
		wp.Submit(func() {
			waiting <- struct{}{}
		})
	}
	for i := 0; i < wp.workers; i++ {
		<-waiting
	}
}
