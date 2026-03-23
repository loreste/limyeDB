package point

import (
	"sync"
)

// Batch represents a batch of points for bulk operations
type Batch struct {
	Points []*Point
	mu     sync.RWMutex
}

// NewBatch creates a new batch with the given capacity
func NewBatch(capacity int) *Batch {
	return &Batch{
		Points: make([]*Point, 0, capacity),
	}
}

// Add adds a point to the batch
func (b *Batch) Add(p *Point) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Points = append(b.Points, p)
}

// AddAll adds multiple points to the batch
func (b *Batch) AddAll(points []*Point) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Points = append(b.Points, points...)
}

// Size returns the number of points in the batch
func (b *Batch) Size() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.Points)
}

// Clear removes all points from the batch
func (b *Batch) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Points = b.Points[:0]
}

// Iterator returns a channel for iterating over points
func (b *Batch) Iterator() <-chan *Point {
	ch := make(chan *Point)
	go func() {
		b.mu.RLock()
		defer b.mu.RUnlock()
		defer close(ch)
		for _, p := range b.Points {
			ch <- p
		}
	}()
	return ch
}

// Validate validates all points in the batch
func (b *Batch) Validate(dimension int) []error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var errors []error
	for i, p := range b.Points {
		if err := p.Validate(); err != nil {
			errors = append(errors, &BatchError{Index: i, Err: err})
			continue
		}
		if p.Dimension() != dimension {
			errors = append(errors, &BatchError{
				Index: i,
				Err:   &DimensionMismatchError{Expected: dimension, Got: p.Dimension()},
			})
		}
	}
	return errors
}

// Partition divides the batch into n roughly equal parts
func (b *Batch) Partition(n int) [][]*Point {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if n <= 0 {
		n = 1
	}
	if n > len(b.Points) {
		n = len(b.Points)
	}

	partitions := make([][]*Point, n)
	size := len(b.Points) / n
	remainder := len(b.Points) % n

	start := 0
	for i := 0; i < n; i++ {
		end := start + size
		if i < remainder {
			end++
		}
		partitions[i] = b.Points[start:end]
		start = end
	}

	return partitions
}

// BatchResult holds the result of a batch operation
type BatchResult struct {
	Succeeded int
	Failed    int
	Errors    []BatchError
}

// BatchError represents an error for a specific point in the batch
type BatchError struct {
	Index int
	ID    string
	Err   error
}

func (e *BatchError) Error() string {
	return e.Err.Error()
}

// DimensionMismatchError indicates a vector dimension mismatch
type DimensionMismatchError struct {
	Expected int
	Got      int
}

func (e *DimensionMismatchError) Error() string {
	return "dimension mismatch"
}

// BatchProcessor handles parallel batch processing
type BatchProcessor struct {
	workers    int
	batchSize  int
	processFunc func([]*Point) error
}

// NewBatchProcessor creates a new batch processor
func NewBatchProcessor(workers, batchSize int, fn func([]*Point) error) *BatchProcessor {
	return &BatchProcessor{
		workers:    workers,
		batchSize:  batchSize,
		processFunc: fn,
	}
}

// Process processes all points in the batch using parallel workers
func (bp *BatchProcessor) Process(batch *Batch) *BatchResult {
	result := &BatchResult{}

	partitions := batch.Partition(bp.workers)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, partition := range partitions {
		wg.Add(1)
		go func(idx int, points []*Point) {
			defer wg.Done()

			if err := bp.processFunc(points); err != nil {
				mu.Lock()
				result.Failed += len(points)
				result.Errors = append(result.Errors, BatchError{
					Index: idx,
					Err:   err,
				})
				mu.Unlock()
				return
			}

			mu.Lock()
			result.Succeeded += len(points)
			mu.Unlock()
		}(i, partition)
	}

	wg.Wait()
	return result
}

// PointBuffer is a lock-free ring buffer for points
type PointBuffer struct {
	points   []*Point
	head     int
	tail     int
	capacity int
	mu       sync.Mutex
}

// NewPointBuffer creates a new point buffer
func NewPointBuffer(capacity int) *PointBuffer {
	return &PointBuffer{
		points:   make([]*Point, capacity),
		capacity: capacity,
	}
}

// Push adds a point to the buffer
func (pb *PointBuffer) Push(p *Point) bool {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	next := (pb.tail + 1) % pb.capacity
	if next == pb.head {
		return false // Buffer full
	}

	pb.points[pb.tail] = p
	pb.tail = next
	return true
}

// Pop removes and returns a point from the buffer
func (pb *PointBuffer) Pop() (*Point, bool) {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	if pb.head == pb.tail {
		return nil, false // Buffer empty
	}

	p := pb.points[pb.head]
	pb.points[pb.head] = nil // Allow GC
	pb.head = (pb.head + 1) % pb.capacity
	return p, true
}

// Len returns the number of points in the buffer
func (pb *PointBuffer) Len() int {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	if pb.tail >= pb.head {
		return pb.tail - pb.head
	}
	return pb.capacity - pb.head + pb.tail
}
