// Package ivf implements the Inverted File Index for approximate nearest neighbor search.
// IVF partitions the vector space using k-means clustering, enabling efficient search
// by only examining vectors in nearby clusters.
package ivf

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"errors"
	"math"
	"math/rand"
	"sync"

	"github.com/limyedb/limyedb/pkg/distance"
	"github.com/limyedb/limyedb/pkg/point"
)

// KMeansConfig holds configuration for k-means clustering
type KMeansConfig struct {
	NumClusters   int     // Number of clusters (k)
	MaxIterations int     // Maximum iterations for convergence
	Tolerance     float32 // Convergence tolerance
	NumInitRuns   int     // Number of random initializations (k-means++)
	Seed          int64   // Random seed (0 = use crypto/rand)
}

// DefaultKMeansConfig returns default k-means configuration
func DefaultKMeansConfig() *KMeansConfig {
	return &KMeansConfig{
		NumClusters:   100,
		MaxIterations: 50,
		Tolerance:     1e-4,
		NumInitRuns:   3,
		Seed:          0,
	}
}

// KMeans implements k-means clustering algorithm
type KMeans struct {
	config    *KMeansConfig
	centroids []point.Vector
	distCalc  distance.Calculator
	dimension int
	trained   bool
	mu        sync.RWMutex
	rng       *rand.Rand
}

// NewKMeans creates a new k-means clusterer
func NewKMeans(cfg *KMeansConfig, distCalc distance.Calculator, dimension int) *KMeans {
	seed := cfg.Seed
	if seed == 0 {
		var seedBytes [8]byte
		if _, err := cryptorand.Read(seedBytes[:]); err == nil {
			seed = int64(binary.LittleEndian.Uint64(seedBytes[:]))
		} else {
			seed = rand.Int63() // #nosec G404 - fallback only
		}
	}

	return &KMeans{
		config:    cfg,
		centroids: make([]point.Vector, 0, cfg.NumClusters),
		distCalc:  distCalc,
		dimension: dimension,
		rng:       rand.New(rand.NewSource(seed)), // #nosec G404 - uses secure seed
	}
}

// Train performs k-means clustering on the given vectors
func (km *KMeans) Train(vectors []point.Vector) error {
	if len(vectors) == 0 {
		return errors.New("no vectors to train on")
	}
	if len(vectors) < km.config.NumClusters {
		return errors.New("not enough vectors for the specified number of clusters")
	}

	km.mu.Lock()
	defer km.mu.Unlock()

	// Run k-means multiple times and keep the best result
	var bestCentroids []point.Vector
	bestInertia := float32(math.MaxFloat32)

	for run := 0; run < km.config.NumInitRuns; run++ {
		centroids := km.initializeCentroidsKMeansPlusPlus(vectors)
		if len(centroids) == 0 {
			// Fallback: use random sampling if k-means++ fails
			centroids = make([]point.Vector, km.config.NumClusters)
			for i := range centroids {
				idx := km.rng.Intn(len(vectors))
				centroids[i] = copyVector(vectors[idx])
			}
		}
		centroids, inertia := km.runKMeans(vectors, centroids)

		if inertia < bestInertia {
			bestInertia = inertia
			bestCentroids = centroids
		}
	}

	// Ensure we have centroids even if all runs produced high inertia
	if len(bestCentroids) == 0 && km.config.NumClusters > 0 {
		// Direct fallback: randomly sample centroids
		bestCentroids = make([]point.Vector, km.config.NumClusters)
		for i := range bestCentroids {
			idx := km.rng.Intn(len(vectors))
			bestCentroids[i] = copyVector(vectors[idx])
		}
	}

	km.centroids = bestCentroids
	km.trained = true
	return nil
}

// initializeCentroidsKMeansPlusPlus uses k-means++ initialization
func (km *KMeans) initializeCentroidsKMeansPlusPlus(vectors []point.Vector) []point.Vector {
	n := len(vectors)
	k := km.config.NumClusters

	if n == 0 || k == 0 {
		return nil
	}

	centroids := make([]point.Vector, 0, k)

	// Choose first centroid randomly
	firstIdx := km.rng.Intn(n)
	centroids = append(centroids, copyVector(vectors[firstIdx]))

	// Distance from each point to nearest centroid
	distances := make([]float32, n)
	for i := range distances {
		distances[i] = float32(math.MaxFloat32)
	}

	// Choose remaining centroids
	for len(centroids) < k {
		// Update distances to nearest centroid
		var totalDist float32
		lastCentroid := centroids[len(centroids)-1]

		for i, vec := range vectors {
			dist := km.distCalc.Distance(vec, lastCentroid)
			if dist < distances[i] {
				distances[i] = dist
			}
			totalDist += distances[i] * distances[i]
		}

		// Handle edge case where all distances are 0
		if totalDist == 0 {
			// All points are identical to existing centroids, pick random
			centroids = append(centroids, copyVector(vectors[km.rng.Intn(n)]))
			continue
		}

		// Choose next centroid with probability proportional to D^2
		prevLen := len(centroids)
		threshold := km.rng.Float32() * totalDist
		var cumSum float32
		for i := range distances {
			cumSum += distances[i] * distances[i]
			if cumSum >= threshold {
				centroids = append(centroids, copyVector(vectors[i]))
				break
			}
		}

		// Fallback if we didn't select any (numerical issues)
		if len(centroids) == prevLen {
			centroids = append(centroids, copyVector(vectors[km.rng.Intn(n)]))
		}
	}

	return centroids
}

// runKMeans runs the k-means algorithm until convergence
func (km *KMeans) runKMeans(vectors []point.Vector, centroids []point.Vector) ([]point.Vector, float32) {
	n := len(vectors)
	k := len(centroids)

	assignments := make([]int, n)
	clusterSizes := make([]int, k)
	newCentroids := make([]point.Vector, k)

	for i := range newCentroids {
		newCentroids[i] = make(point.Vector, km.dimension)
	}

	var inertia float32

	for iter := 0; iter < km.config.MaxIterations; iter++ {
		// Assignment step: assign each vector to nearest centroid
		inertia = 0
		for i := range clusterSizes {
			clusterSizes[i] = 0
			for j := range newCentroids[i] {
				newCentroids[i][j] = 0
			}
		}

		for i, vec := range vectors {
			minDist := float32(math.MaxFloat32)
			minCluster := 0

			for j, centroid := range centroids {
				dist := km.distCalc.Distance(vec, centroid)
				if dist < minDist {
					minDist = dist
					minCluster = j
				}
			}

			assignments[i] = minCluster
			clusterSizes[minCluster]++
			inertia += minDist * minDist

			// Accumulate for centroid update
			for d := 0; d < km.dimension; d++ {
				newCentroids[minCluster][d] += vec[d]
			}
		}

		// Update step: compute new centroids
		maxShift := float32(0)
		for j := 0; j < k; j++ {
			if clusterSizes[j] > 0 {
				for d := 0; d < km.dimension; d++ {
					newCentroids[j][d] /= float32(clusterSizes[j])
				}

				// Compute shift
				shift := km.distCalc.Distance(centroids[j], newCentroids[j])
				if shift > maxShift {
					maxShift = shift
				}

				copy(centroids[j], newCentroids[j])
			}
		}

		// Check for convergence
		if maxShift < km.config.Tolerance {
			break
		}
	}

	return centroids, inertia
}

// Centroids returns the cluster centroids
func (km *KMeans) Centroids() []point.Vector {
	km.mu.RLock()
	defer km.mu.RUnlock()
	return km.centroids
}

// NumClusters returns the number of clusters
func (km *KMeans) NumClusters() int {
	return km.config.NumClusters
}

// FindNearestCentroid returns the index of the nearest centroid
func (km *KMeans) FindNearestCentroid(vec point.Vector) int {
	km.mu.RLock()
	defer km.mu.RUnlock()

	if len(km.centroids) == 0 {
		return -1
	}

	minDist := float32(math.MaxFloat32)
	minIdx := 0

	for i, centroid := range km.centroids {
		dist := km.distCalc.Distance(vec, centroid)
		if dist < minDist {
			minDist = dist
			minIdx = i
		}
	}

	return minIdx
}

// FindNearestCentroids returns the indices of the n nearest centroids
func (km *KMeans) FindNearestCentroids(vec point.Vector, n int) []CentroidDistance {
	km.mu.RLock()
	defer km.mu.RUnlock()

	if len(km.centroids) == 0 {
		return nil
	}

	if n > len(km.centroids) {
		n = len(km.centroids)
	}

	// Calculate distances to all centroids
	distances := make([]CentroidDistance, len(km.centroids))
	for i, centroid := range km.centroids {
		distances[i] = CentroidDistance{
			Index:    i,
			Distance: km.distCalc.Distance(vec, centroid),
		}
	}

	// Partial sort to find top n
	partialSort(distances, n)

	return distances[:n]
}

// CentroidDistance holds a centroid index and its distance
type CentroidDistance struct {
	Index    int
	Distance float32
}

// partialSort performs a partial sort to find the smallest n elements
func partialSort(items []CentroidDistance, n int) {
	for i := 0; i < n && i < len(items); i++ {
		minIdx := i
		for j := i + 1; j < len(items); j++ {
			if items[j].Distance < items[minIdx].Distance {
				minIdx = j
			}
		}
		items[i], items[minIdx] = items[minIdx], items[i]
	}
}

// IsTrained returns whether the k-means has been trained
func (km *KMeans) IsTrained() bool {
	km.mu.RLock()
	defer km.mu.RUnlock()
	return km.trained
}

// copyVector creates a copy of a vector
func copyVector(v point.Vector) point.Vector {
	c := make(point.Vector, len(v))
	copy(c, v)
	return c
}
