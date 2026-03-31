package diskann

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"

	"github.com/limyedb/limyedb/pkg/point"
)

func TestRaceVamanaInsertSearch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	g := NewVamanaGraph(4, 8, 1.2, "euclidean")

	const goroutines = 10
	const ops = 50

	var wg sync.WaitGroup

	// Inserters
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < ops; j++ {
				p := &point.Point{
					ID:     fmt.Sprintf("p%d-%d", id, j),
					Vector: generateRandomVector(4),
				}
				_ = g.Insert(p)
			}
		}(i)
	}

	// Searchers
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < ops; j++ {
				_, _ = g.Search(generateRandomVector(4), 5)
			}
		}()
	}

	wg.Wait()
}

func TestRaceVamanaInsertDelete(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	g := NewVamanaGraph(4, 8, 1.2, "euclidean")

	const goroutines = 10
	const ops = 50

	var wg sync.WaitGroup

	// Inserters
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < ops; j++ {
				p := &point.Point{
					ID:     fmt.Sprintf("p%d-%d", id, j),
					Vector: generateRandomVector(4),
				}
				_ = g.Insert(p)
			}
		}(i)
	}

	// Deleters
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < ops; j++ {
				_ = g.Delete(fmt.Sprintf("p%d-%d", id, j))
			}
		}(i)
	}

	wg.Wait()
}

func TestRaceVamanaGetStats(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	g := NewVamanaGraph(4, 8, 1.2, "euclidean")

	const goroutines = 10
	const ops = 100

	var wg sync.WaitGroup

	// Writers
	wg.Add(goroutines / 2)
	for i := 0; i < goroutines/2; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < ops; j++ {
				p := &point.Point{
					ID:     fmt.Sprintf("p%d-%d", id, j),
					Vector: generateRandomVector(4),
				}
				_ = g.Insert(p)
			}
		}(i)
	}

	// Stats readers
	wg.Add(goroutines / 2)
	for i := 0; i < goroutines/2; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < ops; j++ {
				_ = g.GetStats()
			}
		}()
	}

	wg.Wait()
}

func TestRaceVamanaIterate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	g := NewVamanaGraph(4, 8, 1.2, "euclidean")

	// Pre-populate
	for i := 0; i < 50; i++ {
		p := &point.Point{
			ID:     fmt.Sprintf("p%d", i),
			Vector: generateRandomVector(4),
		}
		_ = g.Insert(p)
	}

	const goroutines = 10
	const ops = 20

	var wg sync.WaitGroup

	// Iterators
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < ops; j++ {
				_ = g.Iterate(func(id string) error {
					return nil
				})
			}
		}()
	}

	// Writers
	wg.Add(goroutines / 2)
	for i := 0; i < goroutines/2; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < ops; j++ {
				p := &point.Point{
					ID:     fmt.Sprintf("new%d-%d", id, j),
					Vector: generateRandomVector(4),
				}
				_ = g.Insert(p)
			}
		}(i)
	}

	wg.Wait()
}

func TestRaceVamanaSearchWithEf(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	g := NewVamanaGraph(4, 8, 1.2, "euclidean")

	// Pre-populate
	for i := 0; i < 100; i++ {
		p := &point.Point{
			ID:     fmt.Sprintf("p%d", i),
			Vector: generateRandomVector(4),
		}
		_ = g.Insert(p)
	}

	const goroutines = 20
	const ops = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < ops; j++ {
				ef := rand.Intn(50) + 10 // #nosec G404
				_, _ = g.SearchWithEf(generateRandomVector(4), 5, ef)
			}
		}()
	}

	wg.Wait()
}

func TestRaceVamanaRecommend(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	g := NewVamanaGraph(4, 8, 1.2, "euclidean")

	// Pre-populate
	for i := 0; i < 50; i++ {
		p := &point.Point{
			ID:     fmt.Sprintf("p%d", i),
			Vector: generateRandomVector(4),
		}
		_ = g.Insert(p)
	}

	const goroutines = 10
	const ops = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < ops; j++ {
				id := fmt.Sprintf("p%d", rand.Intn(50)) // #nosec G404
				_, _ = g.Recommend(id, 5)
			}
		}()
	}

	wg.Wait()
}

func TestRaceVamanaGetAllPoints(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	g := NewVamanaGraph(4, 8, 1.2, "euclidean")

	// Pre-populate
	for i := 0; i < 50; i++ {
		p := &point.Point{
			ID:     fmt.Sprintf("p%d", i),
			Vector: generateRandomVector(4),
		}
		_ = g.Insert(p)
	}

	const goroutines = 10
	const ops = 20

	var wg sync.WaitGroup

	// Readers
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < ops; j++ {
				_ = g.GetAllPoints()
			}
		}()
	}

	// Writers
	wg.Add(goroutines / 2)
	for i := 0; i < goroutines/2; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < ops; j++ {
				p := &point.Point{
					ID:     fmt.Sprintf("new%d-%d", id, j),
					Vector: generateRandomVector(4),
				}
				_ = g.Insert(p)
			}
		}(i)
	}

	wg.Wait()
}

func TestRaceVamanaSetSearchL(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	g := NewVamanaGraph(4, 8, 1.2, "euclidean")

	// Pre-populate
	for i := 0; i < 20; i++ {
		p := &point.Point{
			ID:     fmt.Sprintf("p%d", i),
			Vector: generateRandomVector(4),
		}
		_ = g.Insert(p)
	}

	const goroutines = 10
	const ops = 100

	var wg sync.WaitGroup

	// SetSearchL
	wg.Add(goroutines / 2)
	for i := 0; i < goroutines/2; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < ops; j++ {
				g.SetSearchL(50 + rand.Intn(50)) // #nosec G404
			}
		}()
	}

	// Searchers
	wg.Add(goroutines / 2)
	for i := 0; i < goroutines/2; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < ops; j++ {
				_, _ = g.Search(generateRandomVector(4), 5)
			}
		}()
	}

	wg.Wait()
}
