package ingestion

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/limyedb/limyedb/pkg/point"
)

// =============================================================================
// Memory-Bounded Ingestion Engine
// =============================================================================

// Config holds ingestion configuration
type Config struct {
	// Memory limits
	MaxMemoryBytes      int64         `json:"max_memory_bytes"`      // Maximum memory for ingestion buffers
	MemoryCheckInterval time.Duration `json:"memory_check_interval"` // How often to check memory

	// Batch settings
	BatchSize         int           `json:"batch_size"`          // Points per batch
	MaxPendingBatches int           `json:"max_pending_batches"` // Max batches in queue
	FlushInterval     time.Duration `json:"flush_interval"`      // Auto-flush interval

	// Backpressure settings
	BackpressureThreshold float64       `json:"backpressure_threshold"` // 0.0-1.0, when to start backpressure
	BackpressureDelay     time.Duration `json:"backpressure_delay"`     // Delay when backpressure active

	// Workers
	NumWorkers int `json:"num_workers"` // Number of ingestion workers
}

// DefaultConfig returns default ingestion configuration
func DefaultConfig() *Config {
	return &Config{
		MaxMemoryBytes:        512 * 1024 * 1024, // 512MB
		MemoryCheckInterval:   100 * time.Millisecond,
		BatchSize:             1000,
		MaxPendingBatches:     100,
		FlushInterval:         1 * time.Second,
		BackpressureThreshold: 0.8,
		BackpressureDelay:     50 * time.Millisecond,
		NumWorkers:            runtime.NumCPU(),
	}
}

// Engine handles memory-bounded point ingestion
type Engine struct {
	config *Config

	// Memory tracking
	currentMemory atomic.Int64
	peakMemory    atomic.Int64

	// Batching
	currentBatch []*point.Point
	batchMu      sync.Mutex

	// Queue
	batchQueue chan []*point.Point

	// Callbacks
	onBatch func([]*point.Point) error

	// State
	running atomic.Bool
	stopCh  chan struct{}
	wg      sync.WaitGroup

	// Stats
	stats Stats
}

// Stats holds ingestion statistics
type Stats struct {
	PointsIngested   atomic.Int64
	BatchesProcessed atomic.Int64
	BackpressureHits atomic.Int64
	Errors           atomic.Int64
	LastFlushTime    atomic.Value // time.Time
}

// NewEngine creates a new ingestion engine
func NewEngine(config *Config, onBatch func([]*point.Point) error) *Engine {
	if config == nil {
		config = DefaultConfig()
	}

	e := &Engine{
		config:       config,
		currentBatch: make([]*point.Point, 0, config.BatchSize),
		batchQueue:   make(chan []*point.Point, config.MaxPendingBatches),
		onBatch:      onBatch,
		stopCh:       make(chan struct{}),
	}

	e.stats.LastFlushTime.Store(time.Now())

	return e
}

// Start starts the ingestion engine
func (e *Engine) Start(ctx context.Context) error {
	if e.running.Swap(true) {
		return errors.New("engine already running")
	}

	// Start workers
	for i := 0; i < e.config.NumWorkers; i++ {
		e.wg.Add(1)
		go e.worker(ctx, i)
	}

	// Start flush timer
	e.wg.Add(1)
	go e.flushTimer(ctx)

	// Start memory monitor
	e.wg.Add(1)
	go e.memoryMonitor(ctx)

	return nil
}

// Stop stops the ingestion engine
func (e *Engine) Stop() error {
	if !e.running.Swap(false) {
		return nil
	}

	close(e.stopCh)

	// Flush remaining batch
	e.flush()

	// Close queue after flush
	close(e.batchQueue)

	// Wait for workers
	e.wg.Wait()

	// Force GC to release memory
	runtime.GC()

	return nil
}

// Ingest adds a point to the ingestion queue with backpressure
func (e *Engine) Ingest(ctx context.Context, p *point.Point) error {
	if !e.running.Load() {
		return errors.New("engine not running")
	}

	// Check backpressure
	if err := e.checkBackpressure(ctx); err != nil {
		return err
	}

	// Estimate point memory
	pointMem := e.estimatePointMemory(p)

	// Check memory limit
	for e.currentMemory.Load()+pointMem > e.config.MaxMemoryBytes {
		e.stats.BackpressureHits.Add(1)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-e.stopCh:
			return errors.New("engine stopped")
		case <-time.After(e.config.BackpressureDelay):
			// Retry after delay
		}
	}

	// Add to current batch
	e.batchMu.Lock()
	e.currentBatch = append(e.currentBatch, p)
	e.currentMemory.Add(pointMem)

	shouldFlush := len(e.currentBatch) >= e.config.BatchSize
	e.batchMu.Unlock()

	if shouldFlush {
		e.flush()
	}

	return nil
}

// IngestBatch adds multiple points with backpressure
func (e *Engine) IngestBatch(ctx context.Context, points []*point.Point) error {
	for _, p := range points {
		if err := e.Ingest(ctx, p); err != nil {
			return err
		}
	}
	return nil
}

// flush sends the current batch to the queue
func (e *Engine) flush() {
	e.batchMu.Lock()
	if len(e.currentBatch) == 0 {
		e.batchMu.Unlock()
		return
	}

	batch := e.currentBatch
	e.currentBatch = make([]*point.Point, 0, e.config.BatchSize)
	e.batchMu.Unlock()

	// Non-blocking send to queue
	select {
	case e.batchQueue <- batch:
		e.stats.LastFlushTime.Store(time.Now())
	default:
		// Queue full, process inline (shouldn't happen with backpressure)
		e.processBatch(batch)
	}
}

// worker processes batches from the queue
func (e *Engine) worker(ctx context.Context, id int) {
	defer e.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stopCh:
			// Drain remaining batches
			for batch := range e.batchQueue {
				e.processBatch(batch)
			}
			return
		case batch, ok := <-e.batchQueue:
			if !ok {
				return
			}
			e.processBatch(batch)
		}
	}
}

// processBatch processes a single batch
func (e *Engine) processBatch(batch []*point.Point) {
	if len(batch) == 0 {
		return
	}

	// Calculate batch memory
	var batchMem int64
	for _, p := range batch {
		batchMem += e.estimatePointMemory(p)
	}

	// Process batch
	if err := e.onBatch(batch); err != nil {
		e.stats.Errors.Add(1)
	} else {
		e.stats.PointsIngested.Add(int64(len(batch)))
		e.stats.BatchesProcessed.Add(1)
	}

	// Release memory tracking
	e.currentMemory.Add(-batchMem)

	// Update peak memory
	current := e.currentMemory.Load()
	for {
		peak := e.peakMemory.Load()
		if current <= peak {
			break
		}
		if e.peakMemory.CompareAndSwap(peak, current) {
			break
		}
	}

	// Clear batch references to help GC
	for i := range batch {
		batch[i] = nil
	}
}

// flushTimer periodically flushes the batch
func (e *Engine) flushTimer(ctx context.Context) {
	defer e.wg.Done()

	ticker := time.NewTicker(e.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stopCh:
			return
		case <-ticker.C:
			e.flush()
		}
	}
}

// memoryMonitor monitors memory usage
func (e *Engine) memoryMonitor(ctx context.Context) {
	defer e.wg.Done()

	ticker := time.NewTicker(e.config.MemoryCheckInterval)
	defer ticker.Stop()

	var lastGC time.Time

	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stopCh:
			return
		case <-ticker.C:
			// Check if we should trigger GC
			memUsage := e.currentMemory.Load()
			threshold := int64(float64(e.config.MaxMemoryBytes) * 0.9)

			if memUsage > threshold && time.Since(lastGC) > time.Second {
				runtime.GC()
				lastGC = time.Now()
			}
		}
	}
}

// checkBackpressure applies backpressure if needed
func (e *Engine) checkBackpressure(ctx context.Context) error {
	queueLen := len(e.batchQueue)
	queueCap := cap(e.batchQueue)

	threshold := int(float64(queueCap) * e.config.BackpressureThreshold)

	if queueLen >= threshold {
		e.stats.BackpressureHits.Add(1)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-e.stopCh:
			return errors.New("engine stopped")
		case <-time.After(e.config.BackpressureDelay):
			// Continue after delay
		}
	}

	return nil
}

// estimatePointMemory estimates memory usage for a point
func (e *Engine) estimatePointMemory(p *point.Point) int64 {
	// Base point struct + ID string
	mem := int64(64 + len(p.ID))

	// Vector (4 bytes per float32)
	mem += int64(len(p.Vector) * 4)

	// Payload estimation (rough)
	if p.Payload != nil {
		mem += int64(len(p.Payload) * 64) // Rough estimate per field
	}

	return mem
}

// GetStats returns ingestion statistics
func (e *Engine) GetStats() IngestionStats {
	return IngestionStats{
		PointsIngested:   e.stats.PointsIngested.Load(),
		BatchesProcessed: e.stats.BatchesProcessed.Load(),
		BackpressureHits: e.stats.BackpressureHits.Load(),
		Errors:           e.stats.Errors.Load(),
		CurrentMemory:    e.currentMemory.Load(),
		PeakMemory:       e.peakMemory.Load(),
		QueueSize:        len(e.batchQueue),
		QueueCapacity:    cap(e.batchQueue),
	}
}

// IngestionStats holds public statistics
type IngestionStats struct {
	PointsIngested   int64 `json:"points_ingested"`
	BatchesProcessed int64 `json:"batches_processed"`
	BackpressureHits int64 `json:"backpressure_hits"`
	Errors           int64 `json:"errors"`
	CurrentMemory    int64 `json:"current_memory_bytes"`
	PeakMemory       int64 `json:"peak_memory_bytes"`
	QueueSize        int   `json:"queue_size"`
	QueueCapacity    int   `json:"queue_capacity"`
}

// =============================================================================
// Rate Limiter for Ingestion
// =============================================================================

// RateLimiter limits ingestion rate
type RateLimiter struct {
	pointsPerSecond  int64
	lastCheck        time.Time
	pointsThisPeriod int64
	mu               sync.Mutex
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(pointsPerSecond int64) *RateLimiter {
	return &RateLimiter{
		pointsPerSecond: pointsPerSecond,
		lastCheck:       time.Now(),
	}
}

// Allow checks if ingestion is allowed
func (r *RateLimiter) Allow(points int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(r.lastCheck)

	// Reset counter every second
	if elapsed >= time.Second {
		r.pointsThisPeriod = 0
		r.lastCheck = now
	}

	if r.pointsThisPeriod+points <= r.pointsPerSecond {
		r.pointsThisPeriod += points
		return true
	}

	return false
}

// Wait waits until ingestion is allowed
func (r *RateLimiter) Wait(ctx context.Context, points int64) error {
	for {
		if r.Allow(points) {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
			// Retry
		}
	}
}
