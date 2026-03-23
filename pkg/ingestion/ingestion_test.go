package ingestion

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/limyedb/limyedb/pkg/point"
)

func TestEngineBasic(t *testing.T) {
	var processed atomic.Int64

	config := &Config{
		MaxMemoryBytes:        10 * 1024 * 1024, // 10MB
		MemoryCheckInterval:   50 * time.Millisecond,
		BatchSize:             10,
		MaxPendingBatches:     10,
		FlushInterval:         100 * time.Millisecond,
		BackpressureThreshold: 0.8,
		BackpressureDelay:     10 * time.Millisecond,
		NumWorkers:            2,
	}

	engine := NewEngine(config, func(batch []*point.Point) error {
		processed.Add(int64(len(batch)))
		return nil
	})

	ctx := context.Background()
	if err := engine.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Ingest 100 points
	for i := 0; i < 100; i++ {
		p := point.NewPointWithID(
			string(rune('A'+i%26)),
			point.Vector{float32(i), float32(i + 1)},
			nil,
		)
		if err := engine.Ingest(ctx, p); err != nil {
			t.Fatalf("Ingest failed: %v", err)
		}
	}

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	if err := engine.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if processed.Load() != 100 {
		t.Errorf("Expected 100 processed points, got %d", processed.Load())
	}
}

func TestEngineBackpressure(t *testing.T) {
	var processed atomic.Int64

	config := &Config{
		MaxMemoryBytes:        1024, // Very small to trigger backpressure
		MemoryCheckInterval:   10 * time.Millisecond,
		BatchSize:             5,
		MaxPendingBatches:     2,
		FlushInterval:         50 * time.Millisecond,
		BackpressureThreshold: 0.5,
		BackpressureDelay:     5 * time.Millisecond,
		NumWorkers:            1,
	}

	engine := NewEngine(config, func(batch []*point.Point) error {
		time.Sleep(10 * time.Millisecond) // Slow processing
		processed.Add(int64(len(batch)))
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := engine.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Ingest points - should experience backpressure
	for i := 0; i < 50; i++ {
		p := point.NewPointWithID(
			string(rune('A'+i%26)),
			point.Vector{float32(i)},
			nil,
		)
		engine.Ingest(ctx, p)
	}

	time.Sleep(500 * time.Millisecond)
	engine.Stop()

	stats := engine.GetStats()
	if stats.BackpressureHits == 0 {
		t.Log("Warning: Expected some backpressure hits")
	}

	t.Logf("Stats: processed=%d, backpressure=%d", processed.Load(), stats.BackpressureHits)
}

func TestEngineStats(t *testing.T) {
	config := DefaultConfig()
	config.BatchSize = 10
	config.NumWorkers = 1

	engine := NewEngine(config, func(batch []*point.Point) error {
		return nil
	})

	ctx := context.Background()
	engine.Start(ctx)

	// Ingest 25 points (should create 2 full batches + partial)
	for i := 0; i < 25; i++ {
		p := point.NewPointWithID(
			string(rune('A'+i%26)),
			point.Vector{float32(i)},
			nil,
		)
		engine.Ingest(ctx, p)
	}

	time.Sleep(200 * time.Millisecond)
	engine.Stop()

	stats := engine.GetStats()
	if stats.PointsIngested != 25 {
		t.Errorf("Expected 25 points ingested, got %d", stats.PointsIngested)
	}
}

func TestRateLimiter(t *testing.T) {
	limiter := NewRateLimiter(100) // 100 points per second

	// Should allow first 100
	for i := 0; i < 100; i++ {
		if !limiter.Allow(1) {
			t.Errorf("Should allow point %d", i)
		}
	}

	// Should block 101st
	if limiter.Allow(1) {
		t.Error("Should block after rate limit")
	}

	// Wait for reset
	time.Sleep(1100 * time.Millisecond)

	// Should allow again
	if !limiter.Allow(1) {
		t.Error("Should allow after rate limit reset")
	}
}

func TestRateLimiterWait(t *testing.T) {
	limiter := NewRateLimiter(10) // 10 points per second

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	start := time.Now()

	// Exhaust rate limit
	for i := 0; i < 10; i++ {
		limiter.Allow(1)
	}

	// Wait for next slot
	err := limiter.Wait(ctx, 1)
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}

	elapsed := time.Since(start)
	if elapsed < 900*time.Millisecond {
		t.Errorf("Expected to wait ~1 second, waited %v", elapsed)
	}
}

func BenchmarkEngineIngestion(b *testing.B) {
	config := DefaultConfig()
	config.BatchSize = 1000
	config.NumWorkers = 4

	engine := NewEngine(config, func(batch []*point.Point) error {
		return nil
	})

	ctx := context.Background()
	engine.Start(ctx)

	points := make([]*point.Point, b.N)
	for i := range points {
		points[i] = point.NewPointWithID(
			string(rune('A'+i%26)),
			make(point.Vector, 128),
			nil,
		)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.Ingest(ctx, points[i])
	}

	engine.Stop()
}
