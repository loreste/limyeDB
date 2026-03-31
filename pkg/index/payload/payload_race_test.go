package payload

import (
	"fmt"
	"sync"
	"testing"
)

func TestRaceIndexRemove(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	idx, err := NewIndex("")
	if err != nil {
		t.Fatalf("NewIndex() failed: %v", err)
	}
	defer idx.Close()

	const goroutines = 10
	const ops = 100

	var wg sync.WaitGroup

	// Indexers
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				pointID := uint32(id*ops + i)
				payload := map[string]interface{}{
					"key":   fmt.Sprintf("value-%d-%d", id, i),
					"count": i,
				}
				idx.IndexPoint(pointID, payload)
			}
		}(g)
	}

	// Removers
	wg.Add(goroutines / 2)
	for g := 0; g < goroutines/2; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				pointID := uint32(id*ops + i)
				idx.RemovePoint(pointID, nil)
			}
		}(g)
	}

	wg.Wait()
}

func TestRaceFilter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	idx, err := NewIndex("")
	if err != nil {
		t.Fatalf("NewIndex() failed: %v", err)
	}
	defer idx.Close()

	// Pre-populate
	for i := 0; i < 100; i++ {
		idx.IndexPoint(uint32(i), map[string]interface{}{
			"category": i % 5,
			"value":    i,
		})
	}

	const goroutines = 20
	const ops = 50

	var wg sync.WaitGroup

	// Searchers
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				filter := Field("category", Eq(i%5))
				_ = idx.Filter(filter)
			}
		}()
	}

	// Writers
	wg.Add(goroutines / 2)
	for g := 0; g < goroutines/2; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				pointID := uint32(100 + id*ops + i)
				idx.IndexPoint(pointID, map[string]interface{}{
					"category": i % 5,
					"value":    i,
				})
			}
		}(g)
	}

	wg.Wait()
}

func TestRaceFilterComplex(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	idx, err := NewIndex("")
	if err != nil {
		t.Fatalf("NewIndex() failed: %v", err)
	}
	defer idx.Close()

	// Pre-populate
	for i := 0; i < 50; i++ {
		idx.IndexPoint(uint32(i), map[string]interface{}{
			"status": []string{"active", "pending", "inactive"}[i%3],
			"score":  i * 10,
		})
	}

	const goroutines = 10
	const ops = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				// Test various filter types
				switch i % 5 {
				case 0:
					_ = idx.Filter(And(
						Field("status", Eq("active")),
						Field("score", Gte(100)),
					))
				case 1:
					_ = idx.Filter(Or(
						Field("status", Eq("active")),
						Field("status", Eq("pending")),
					))
				case 2:
					_ = idx.Filter(Not(Field("status", Eq("inactive"))))
				case 3:
					_ = idx.Filter(Field("score", Range(100, 300)))
				case 4:
					_ = idx.Filter(Field("status", In("active", "pending")))
				}
			}
		}()
	}

	wg.Wait()
}

func TestRaceCreateDeleteIndex(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	idx, err := NewIndex("")
	if err != nil {
		t.Fatalf("NewIndex() failed: %v", err)
	}
	defer idx.Close()

	const goroutines = 10
	const ops = 50

	fields := []string{"field1", "field2", "field3", "field4", "field5"}

	var wg sync.WaitGroup

	// Creators
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				idx.CreateIndex(fields[i%len(fields)], IndexTypeHash)
			}
		}()
	}

	// Deleters
	wg.Add(goroutines / 2)
	for g := 0; g < goroutines/2; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				idx.DeleteIndex(fields[i%len(fields)])
			}
		}()
	}

	// Readers
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				_ = idx.IndexedFields()
				_ = idx.GetIndexStats(fields[i%len(fields)])
			}
		}()
	}

	wg.Wait()
}

func TestRaceEvaluator(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	e := NewEvaluator()

	payloads := []map[string]interface{}{
		{"name": "John", "age": 25},
		{"name": "Jane", "age": 30},
		{"name": "Bob", "age": 35},
	}

	filters := []*Filter{
		Field("name", Eq("John")),
		Field("age", Gte(30)),
		And(Field("name", Eq("Jane")), Field("age", Lte(30))),
		Or(Field("name", Eq("Bob")), Field("age", Lt(30))),
	}

	const goroutines = 20
	const ops = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				payload := payloads[i%len(payloads)]
				filter := filters[i%len(filters)]
				_ = e.Evaluate(filter, payload)
			}
		}()
	}

	wg.Wait()
}

func TestRaceFilterBuilder(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	const goroutines = 10
	const ops = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				fb := NewFilterBuilder()
				fb.Where("field1", Eq("value"))
				fb.Where("field2", Gte(i))
				fb.OrWhere("field3", Contains("text"))
				_, _ = fb.Build()
			}
		}()
	}

	wg.Wait()
}
