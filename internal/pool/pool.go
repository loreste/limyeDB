package pool

import (
	"sync"
)

// vectorWrapper wraps a slice to avoid allocation when putting into sync.Pool
type vectorWrapper struct {
	data []float32
}

// VectorPool is a pool for reusable vector slices
type VectorPool struct {
	pools map[int]*sync.Pool
	mu    sync.RWMutex
}

// NewVectorPool creates a new vector pool
func NewVectorPool() *VectorPool {
	return &VectorPool{
		pools: make(map[int]*sync.Pool),
	}
}

// Get retrieves a vector of the given dimension from the pool
func (vp *VectorPool) Get(dimension int) []float32 {
	vp.mu.RLock()
	pool, ok := vp.pools[dimension]
	vp.mu.RUnlock()

	if !ok {
		vp.mu.Lock()
		pool, ok = vp.pools[dimension]
		if !ok {
			pool = &sync.Pool{
				New: func() interface{} {
					return &vectorWrapper{data: make([]float32, dimension)}
				},
			}
			vp.pools[dimension] = pool
		}
		vp.mu.Unlock()
	}

	wrapper, _ := pool.Get().(*vectorWrapper)
	vec := wrapper.data
	// Zero out the vector
	for i := range vec {
		vec[i] = 0
	}
	return vec
}

// Put returns a vector to the pool
func (vp *VectorPool) Put(vec []float32) {
	dimension := len(vec)
	if dimension == 0 {
		return
	}

	vp.mu.RLock()
	pool, ok := vp.pools[dimension]
	vp.mu.RUnlock()

	if ok {
		pool.Put(&vectorWrapper{data: vec})
	}
}

// byteWrapper wraps a byte slice to avoid allocation when putting into sync.Pool
type byteWrapper struct {
	data []byte
}

// BytePool is a pool for byte slices
type BytePool struct {
	pools []*sync.Pool
	sizes []int
}

// NewBytePool creates a new byte pool with predefined sizes
func NewBytePool() *BytePool {
	sizes := []int{64, 256, 1024, 4096, 16384, 65536}
	pools := make([]*sync.Pool, len(sizes))

	for i, size := range sizes {
		s := size // Capture for closure
		pools[i] = &sync.Pool{
			New: func() interface{} {
				return &byteWrapper{data: make([]byte, s)}
			},
		}
	}

	return &BytePool{
		pools: pools,
		sizes: sizes,
	}
}

// Get retrieves a byte slice of at least the given size
func (bp *BytePool) Get(size int) []byte {
	for i, s := range bp.sizes {
		if s >= size {
			wrapper, _ := bp.pools[i].Get().(*byteWrapper)
			return wrapper.data[:size]
		}
	}
	// Size is larger than any pool, allocate directly
	return make([]byte, size)
}

// Put returns a byte slice to the pool
func (bp *BytePool) Put(buf []byte) {
	capacity := cap(buf)
	for i, s := range bp.sizes {
		if s == capacity {
			bp.pools[i].Put(&byteWrapper{data: buf[:capacity]})
			return
		}
	}
	// Not from pool, let GC handle it
}

// candidateWrapper wraps a candidate slice to avoid allocation when putting into sync.Pool
type candidateWrapper struct {
	data []Candidate
}

// CandidatePool is a pool for search candidates
type CandidatePool struct {
	pool *sync.Pool
}

// Candidate represents a search candidate
type Candidate struct {
	ID       uint32
	Distance float32
}

// NewCandidatePool creates a new candidate pool
func NewCandidatePool(capacity int) *CandidatePool {
	return &CandidatePool{
		pool: &sync.Pool{
			New: func() interface{} {
				return &candidateWrapper{data: make([]Candidate, 0, capacity)}
			},
		},
	}
}

// Get retrieves a candidate slice
func (cp *CandidatePool) Get() []Candidate {
	wrapper, _ := cp.pool.Get().(*candidateWrapper)
	return wrapper.data[:0]
}

// Put returns a candidate slice to the pool
func (cp *CandidatePool) Put(candidates []Candidate) {
	cp.pool.Put(&candidateWrapper{data: candidates[:0]})
}

// WorkerPool manages a pool of worker goroutines
type WorkerPool struct {
	tasks   chan func()
	workers int
	wg      sync.WaitGroup
	quit    chan struct{}
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(workers, queueSize int) *WorkerPool {
	wp := &WorkerPool{
		tasks:   make(chan func(), queueSize),
		workers: workers,
		quit:    make(chan struct{}),
	}

	wp.start()
	return wp
}

func (wp *WorkerPool) start() {
	for i := 0; i < wp.workers; i++ {
		wp.wg.Add(1)
		go wp.worker()
	}
}

func (wp *WorkerPool) worker() {
	defer wp.wg.Done()

	for {
		select {
		case task := <-wp.tasks:
			task()
		case <-wp.quit:
			return
		}
	}
}

// Submit submits a task to the worker pool
func (wp *WorkerPool) Submit(task func()) {
	select {
	case wp.tasks <- task:
	case <-wp.quit:
	}
}

// SubmitWait submits a task and waits for completion.
// Returns false if the pool is shutting down and the task was not executed.
func (wp *WorkerPool) SubmitWait(task func()) bool {
	done := make(chan struct{})
	wp.Submit(func() {
		task()
		close(done)
	})
	select {
	case <-done:
		return true
	case <-wp.quit:
		return false
	}
}

// Stop stops all workers
func (wp *WorkerPool) Stop() {
	close(wp.quit)
	wp.wg.Wait()
}

// bufferWrapper wraps a generic slice to avoid allocation when putting into sync.Pool
type bufferWrapper[T any] struct {
	data []T
}

// BufferPool is a generic buffer pool
type BufferPool[T any] struct {
	pool *sync.Pool
}

// NewBufferPool creates a new typed buffer pool
func NewBufferPool[T any](capacity int) *BufferPool[T] {
	return &BufferPool[T]{
		pool: &sync.Pool{
			New: func() interface{} {
				return &bufferWrapper[T]{data: make([]T, 0, capacity)}
			},
		},
	}
}

// Get retrieves a buffer
func (bp *BufferPool[T]) Get() []T {
	wrapper, _ := bp.pool.Get().(*bufferWrapper[T])
	return wrapper.data[:0]
}

// Put returns a buffer to the pool
func (bp *BufferPool[T]) Put(buf []T) {
	bp.pool.Put(&bufferWrapper[T]{data: buf[:0]})
}

// ObjectPool is a generic object pool
type ObjectPool[T any] struct {
	pool *sync.Pool
}

// NewObjectPool creates a new object pool
func NewObjectPool[T any](newFunc func() T) *ObjectPool[T] {
	return &ObjectPool[T]{
		pool: &sync.Pool{
			New: func() interface{} {
				return newFunc()
			},
		},
	}
}

// Get retrieves an object from the pool
func (op *ObjectPool[T]) Get() T {
	val, _ := op.pool.Get().(T)
	return val
}

// Put returns an object to the pool
func (op *ObjectPool[T]) Put(obj T) {
	op.pool.Put(obj)
}

// VisitedList is a dense integer tracker for tracking visited state.
// It avoids zeroing vectors on Reset by incrementing a generational 'match' index.
type VisitedList struct {
	matches []uint32
	match   uint32
}

// NewVisitedList creates a new array-backed visited tracker
func NewVisitedList(size int) *VisitedList {
	return &VisitedList{
		matches: make([]uint32, size),
		match:   0,
	}
}

// Add sets the bit for an ID, returns true if already visited
func (vl *VisitedList) Add(id uint32) bool {
	if int(id) >= len(vl.matches) {
		newCap := int(id)*2 + 100
		newMatches := make([]uint32, newCap)
		copy(newMatches, vl.matches)
		vl.matches = newMatches
	}
	if vl.matches[id] == vl.match {
		return true // Already visited in this search pass
	}
	vl.matches[id] = vl.match
	return false
}

// Contains checks if the ID was visited on the current generation
func (vl *VisitedList) Contains(id uint32) bool {
	if int(id) >= len(vl.matches) {
		return false
	}
	return vl.matches[id] == vl.match
}

// Reset clears the tracker extremely fast by merely advancing generation ID
func (vl *VisitedList) Reset() {
	vl.match++
	if vl.match == 0 { // uint32 wrapped around
		for i := range vl.matches {
			vl.matches[i] = 0
		}
		vl.match = 1
	}
}

// VisitedListPool re-uses VisitedLists seamlessly across multiple coroutines
type VisitedListPool struct {
	pool *sync.Pool
}

// NewVisitedListPool provides high throughput visited objects and skips map allocation spikes
func NewVisitedListPool(initialSize int) *VisitedListPool {
	return &VisitedListPool{
		pool: &sync.Pool{
			New: func() interface{} {
				return NewVisitedList(initialSize)
			},
		},
	}
}

func (vp *VisitedListPool) Get() *VisitedList {
	vl, _ := vp.pool.Get().(*VisitedList)
	vl.Reset()
	return vl
}

func (vp *VisitedListPool) Put(vl *VisitedList) {
	vp.pool.Put(vl)
}
