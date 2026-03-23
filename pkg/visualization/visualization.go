package visualization

import (
	"math"
	"math/rand" // #nosec G404 - cryptographic randomness not needed for visualization algorithms
	"sort"

	"github.com/limyedb/limyedb/pkg/point"
)

// =============================================================================
// Dimensionality Reduction for Visualization
// =============================================================================

// Point2D represents a 2D point for visualization
type Point2D struct {
	ID    string  `json:"id"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Label string  `json:"label,omitempty"`
}

// Point3D represents a 3D point for visualization
type Point3D struct {
	ID    string  `json:"id"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Z     float64 `json:"z"`
	Label string  `json:"label,omitempty"`
}

// =============================================================================
// PCA (Principal Component Analysis)
// =============================================================================

// PCA performs Principal Component Analysis
type PCA struct {
	components int
	mean       []float64
	eigenvecs  [][]float64
}

// NewPCA creates a new PCA reducer
func NewPCA(components int) *PCA {
	return &PCA{components: components}
}

// Fit fits the PCA model to data
func (p *PCA) Fit(vectors []point.Vector) error {
	if len(vectors) == 0 {
		return nil
	}

	dim := len(vectors[0])
	n := len(vectors)

	// Calculate mean
	p.mean = make([]float64, dim)
	for _, v := range vectors {
		for i, val := range v {
			p.mean[i] += float64(val)
		}
	}
	for i := range p.mean {
		p.mean[i] /= float64(n)
	}

	// Center data
	centered := make([][]float64, n)
	for i, v := range vectors {
		centered[i] = make([]float64, dim)
		for j, val := range v {
			centered[i][j] = float64(val) - p.mean[j]
		}
	}

	// Compute covariance matrix (simplified power iteration for top components)
	p.eigenvecs = make([][]float64, p.components)
	for c := 0; c < p.components; c++ {
		p.eigenvecs[c] = powerIteration(centered, p.eigenvecs[:c], 100)
	}

	return nil
}

// Transform projects vectors to lower dimensions
func (p *PCA) Transform(vectors []point.Vector) [][]float64 {
	result := make([][]float64, len(vectors))

	for i, v := range vectors {
		result[i] = make([]float64, p.components)
		for c := 0; c < p.components; c++ {
			var sum float64
			for j, val := range v {
				sum += (float64(val) - p.mean[j]) * p.eigenvecs[c][j]
			}
			result[i][c] = sum
		}
	}

	return result
}

// FitTransform fits and transforms in one step
func (p *PCA) FitTransform(vectors []point.Vector) [][]float64 {
	_ = p.Fit(vectors) // Error is logged internally if any
	return p.Transform(vectors)
}

// powerIteration finds a principal component using power iteration
func powerIteration(centered [][]float64, existing [][]float64, iterations int) []float64 {
	if len(centered) == 0 || len(centered[0]) == 0 {
		return nil
	}

	dim := len(centered[0])
	vec := make([]float64, dim)

	// Random initialization
	for i := range vec {
		vec[i] = rand.Float64() - 0.5 // #nosec G404 - crypto random not needed for PCA
	}
	normalize(vec)

	for iter := 0; iter < iterations; iter++ {
		// Multiply by covariance matrix (X^T * X * v)
		newVec := make([]float64, dim)

		// First compute X * v
		proj := make([]float64, len(centered))
		for i, row := range centered {
			for j := range row {
				proj[i] += row[j] * vec[j]
			}
		}

		// Then compute X^T * (X * v)
		for i, row := range centered {
			for j := range row {
				newVec[j] += row[j] * proj[i]
			}
		}

		// Remove existing components (Gram-Schmidt)
		for _, ev := range existing {
			dot := dotProduct(newVec, ev)
			for i := range newVec {
				newVec[i] -= dot * ev[i]
			}
		}

		normalize(newVec)
		vec = newVec
	}

	return vec
}

// =============================================================================
// t-SNE (t-Distributed Stochastic Neighbor Embedding)
// =============================================================================

// TSNE performs t-SNE dimensionality reduction
type TSNE struct {
	Components   int
	Perplexity   float64
	LearningRate float64
	Iterations   int
}

// NewTSNE creates a new t-SNE reducer
func NewTSNE(components int) *TSNE {
	return &TSNE{
		Components:   components,
		Perplexity:   30.0,
		LearningRate: 200.0,
		Iterations:   1000,
	}
}

// FitTransform performs t-SNE on the input vectors
func (t *TSNE) FitTransform(vectors []point.Vector) [][]float64 {
	n := len(vectors)
	if n == 0 {
		return nil
	}

	// Compute pairwise distances
	distances := computeDistances(vectors)

	// Compute affinities (P matrix)
	P := computeAffinities(distances, t.Perplexity)

	// Symmetrize P
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			pij := (P[i][j] + P[j][i]) / (2.0 * float64(n))
			P[i][j] = pij
			P[j][i] = pij
		}
	}

	// Initialize embedding randomly
	Y := make([][]float64, n)
	for i := range Y {
		Y[i] = make([]float64, t.Components)
		for j := range Y[i] {
			Y[i][j] = rand.NormFloat64() * 0.0001 // #nosec G404 - crypto random not needed for t-SNE
		}
	}

	// Gradient descent
	gains := make([][]float64, n)
	velocities := make([][]float64, n)
	for i := range gains {
		gains[i] = make([]float64, t.Components)
		velocities[i] = make([]float64, t.Components)
		for j := range gains[i] {
			gains[i][j] = 1.0
		}
	}

	for iter := 0; iter < t.Iterations; iter++ {
		// Compute Q matrix (student-t distribution)
		Q := computeStudentT(Y)

		// Compute gradient
		gradient := make([][]float64, n)
		for i := range gradient {
			gradient[i] = make([]float64, t.Components)
		}

		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				if i == j {
					continue
				}
				mult := 4.0 * (P[i][j] - Q[i][j]) * (1.0 + squaredDist(Y[i], Y[j]))
				for d := 0; d < t.Components; d++ {
					gradient[i][d] += mult * (Y[i][d] - Y[j][d])
				}
			}
		}

		// Update with momentum
		momentum := 0.5
		if iter > 250 {
			momentum = 0.8
		}

		for i := 0; i < n; i++ {
			for d := 0; d < t.Components; d++ {
				// Update gains
				if (gradient[i][d] > 0) != (velocities[i][d] > 0) {
					gains[i][d] += 0.2
				} else {
					gains[i][d] *= 0.8
				}
				if gains[i][d] < 0.01 {
					gains[i][d] = 0.01
				}

				velocities[i][d] = momentum*velocities[i][d] - t.LearningRate*gains[i][d]*gradient[i][d]
				Y[i][d] += velocities[i][d]
			}
		}

		// Center Y
		for d := 0; d < t.Components; d++ {
			var mean float64
			for i := 0; i < n; i++ {
				mean += Y[i][d]
			}
			mean /= float64(n)
			for i := 0; i < n; i++ {
				Y[i][d] -= mean
			}
		}
	}

	return Y
}

// computeDistances computes pairwise squared Euclidean distances
func computeDistances(vectors []point.Vector) [][]float64 {
	n := len(vectors)
	distances := make([][]float64, n)
	for i := range distances {
		distances[i] = make([]float64, n)
	}

	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			var sum float64
			for k := range vectors[i] {
				diff := float64(vectors[i][k] - vectors[j][k])
				sum += diff * diff
			}
			distances[i][j] = sum
			distances[j][i] = sum
		}
	}

	return distances
}

// computeAffinities computes conditional probabilities with binary search for sigma
func computeAffinities(distances [][]float64, perplexity float64) [][]float64 {
	n := len(distances)
	P := make([][]float64, n)
	for i := range P {
		P[i] = make([]float64, n)
	}

	targetEntropy := math.Log(perplexity)

	for i := 0; i < n; i++ {
		// Binary search for sigma
		sigmaMin, sigmaMax := 1e-20, 1e10
		sigma := 1.0

		for iter := 0; iter < 50; iter++ {
			// Compute probabilities
			var sumP float64
			for j := 0; j < n; j++ {
				if i != j {
					P[i][j] = math.Exp(-distances[i][j] / (2 * sigma * sigma))
					sumP += P[i][j]
				}
			}

			// Normalize
			var entropy float64
			for j := 0; j < n; j++ {
				if i != j && sumP > 0 {
					P[i][j] /= sumP
					if P[i][j] > 1e-10 {
						entropy -= P[i][j] * math.Log(P[i][j])
					}
				}
			}

			// Adjust sigma
			diff := entropy - targetEntropy
			if math.Abs(diff) < 1e-5 {
				break
			}
			if diff > 0 {
				sigmaMax = sigma
				sigma = (sigma + sigmaMin) / 2
			} else {
				sigmaMin = sigma
				sigma = (sigma + sigmaMax) / 2
			}
		}
	}

	return P
}

// computeStudentT computes student-t based affinities for embedding
func computeStudentT(Y [][]float64) [][]float64 {
	n := len(Y)
	Q := make([][]float64, n)
	for i := range Q {
		Q[i] = make([]float64, n)
	}

	var sumQ float64
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			qij := 1.0 / (1.0 + squaredDist(Y[i], Y[j]))
			Q[i][j] = qij
			Q[j][i] = qij
			sumQ += 2 * qij
		}
	}

	// Normalize
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			Q[i][j] /= sumQ
			if Q[i][j] < 1e-12 {
				Q[i][j] = 1e-12
			}
		}
	}

	return Q
}

// =============================================================================
// Visualization Helpers
// =============================================================================

// Visualizer provides methods for vector visualization
type Visualizer struct {
	pca  *PCA
	tsne *TSNE
}

// NewVisualizer creates a new visualizer
func NewVisualizer() *Visualizer {
	return &Visualizer{
		pca:  NewPCA(2),
		tsne: NewTSNE(2),
	}
}

// ReducePCA reduces vectors to 2D using PCA
func (v *Visualizer) ReducePCA(vectors []point.Vector, ids []string) []Point2D {
	if len(vectors) == 0 {
		return nil
	}

	coords := v.pca.FitTransform(vectors)
	return v.toPoints2D(coords, ids)
}

// ReduceTSNE reduces vectors to 2D using t-SNE
func (v *Visualizer) ReduceTSNE(vectors []point.Vector, ids []string) []Point2D {
	if len(vectors) == 0 {
		return nil
	}

	// Limit iterations for speed
	v.tsne.Iterations = 500 // Faster for visualization

	coords := v.tsne.FitTransform(vectors)
	return v.toPoints2D(coords, ids)
}

// ReducePCA3D reduces vectors to 3D using PCA
func (v *Visualizer) ReducePCA3D(vectors []point.Vector, ids []string) []Point3D {
	if len(vectors) == 0 {
		return nil
	}

	pca3d := NewPCA(3)
	coords := pca3d.FitTransform(vectors)
	return v.toPoints3D(coords, ids)
}

// toPoints2D converts coordinates to Point2D slice
func (v *Visualizer) toPoints2D(coords [][]float64, ids []string) []Point2D {
	points := make([]Point2D, len(coords))
	for i, coord := range coords {
		id := ""
		if i < len(ids) {
			id = ids[i]
		}
		points[i] = Point2D{
			ID: id,
			X:  coord[0],
			Y:  coord[1],
		}
	}

	// Normalize to 0-1 range
	normalizePoints2D(points)
	return points
}

// toPoints3D converts coordinates to Point3D slice
func (v *Visualizer) toPoints3D(coords [][]float64, ids []string) []Point3D {
	points := make([]Point3D, len(coords))
	for i, coord := range coords {
		id := ""
		if i < len(ids) {
			id = ids[i]
		}
		points[i] = Point3D{
			ID: id,
			X:  coord[0],
			Y:  coord[1],
			Z:  coord[2],
		}
	}

	normalizePoints3D(points)
	return points
}

// normalizePoints2D normalizes 2D points to 0-1 range
func normalizePoints2D(points []Point2D) {
	if len(points) == 0 {
		return
	}

	minX, maxX := points[0].X, points[0].X
	minY, maxY := points[0].Y, points[0].Y

	for _, p := range points {
		if p.X < minX {
			minX = p.X
		}
		if p.X > maxX {
			maxX = p.X
		}
		if p.Y < minY {
			minY = p.Y
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}

	rangeX := maxX - minX
	rangeY := maxY - minY
	if rangeX < 1e-10 {
		rangeX = 1
	}
	if rangeY < 1e-10 {
		rangeY = 1
	}

	for i := range points {
		points[i].X = (points[i].X - minX) / rangeX
		points[i].Y = (points[i].Y - minY) / rangeY
	}
}

// normalizePoints3D normalizes 3D points to 0-1 range
func normalizePoints3D(points []Point3D) {
	if len(points) == 0 {
		return
	}

	minX, maxX := points[0].X, points[0].X
	minY, maxY := points[0].Y, points[0].Y
	minZ, maxZ := points[0].Z, points[0].Z

	for _, p := range points {
		if p.X < minX {
			minX = p.X
		}
		if p.X > maxX {
			maxX = p.X
		}
		if p.Y < minY {
			minY = p.Y
		}
		if p.Y > maxY {
			maxY = p.Y
		}
		if p.Z < minZ {
			minZ = p.Z
		}
		if p.Z > maxZ {
			maxZ = p.Z
		}
	}

	rangeX := maxX - minX
	rangeY := maxY - minY
	rangeZ := maxZ - minZ
	if rangeX < 1e-10 {
		rangeX = 1
	}
	if rangeY < 1e-10 {
		rangeY = 1
	}
	if rangeZ < 1e-10 {
		rangeZ = 1
	}

	for i := range points {
		points[i].X = (points[i].X - minX) / rangeX
		points[i].Y = (points[i].Y - minY) / rangeY
		points[i].Z = (points[i].Z - minZ) / rangeZ
	}
}

// =============================================================================
// K-Nearest Neighbors Visualization
// =============================================================================

// KNNVisualization shows k-nearest neighbors for a query
type KNNVisualization struct {
	Query     Point2D   `json:"query"`
	Neighbors []Point2D `json:"neighbors"`
	Edges     []Edge    `json:"edges"`
}

// Edge represents a connection between points
type Edge struct {
	From     string  `json:"from"`
	To       string  `json:"to"`
	Distance float64 `json:"distance"`
}

// VisualizeKNN creates a visualization of k-nearest neighbors
func (v *Visualizer) VisualizeKNN(query point.Vector, neighbors []point.Vector, neighborIDs []string, distances []float32) *KNNVisualization {
	// Combine query and neighbors for PCA
	allVectors := make([]point.Vector, len(neighbors)+1)
	allVectors[0] = query
	copy(allVectors[1:], neighbors)

	allIDs := make([]string, len(neighborIDs)+1)
	allIDs[0] = "query"
	copy(allIDs[1:], neighborIDs)

	// Reduce to 2D
	points := v.ReducePCA(allVectors, allIDs)

	// Build edges
	edges := make([]Edge, len(neighbors))
	for i, id := range neighborIDs {
		edges[i] = Edge{
			From:     "query",
			To:       id,
			Distance: float64(distances[i]),
		}
	}

	return &KNNVisualization{
		Query:     points[0],
		Neighbors: points[1:],
		Edges:     edges,
	}
}

// =============================================================================
// Cluster Visualization
// =============================================================================

// ClusterResult represents a clustering result for visualization
type ClusterResult struct {
	Points   []ClusterPoint `json:"points"`
	Centroids []Point2D     `json:"centroids"`
}

// ClusterPoint is a point with cluster assignment
type ClusterPoint struct {
	Point2D
	Cluster int `json:"cluster"`
}

// VisualizeClusters performs k-means and visualizes clusters
func (v *Visualizer) VisualizeClusters(vectors []point.Vector, ids []string, k int) *ClusterResult {
	if len(vectors) == 0 {
		return nil
	}

	// Reduce to 2D first
	points2D := v.ReducePCA(vectors, ids)

	// K-means on 2D points
	assignments, centroids := kmeans2D(points2D, k, 50)

	// Build result
	clusterPoints := make([]ClusterPoint, len(points2D))
	for i, p := range points2D {
		clusterPoints[i] = ClusterPoint{
			Point2D: p,
			Cluster: assignments[i],
		}
	}

	return &ClusterResult{
		Points:    clusterPoints,
		Centroids: centroids,
	}
}

// kmeans2D performs k-means clustering on 2D points
func kmeans2D(points []Point2D, k, iterations int) ([]int, []Point2D) {
	n := len(points)
	if n == 0 || k <= 0 {
		return nil, nil
	}

	// Initialize centroids randomly
	centroids := make([]Point2D, k)
	perm := rand.Perm(n)
	for i := 0; i < k && i < n; i++ {
		centroids[i] = points[perm[i]]
		centroids[i].ID = ""
	}

	assignments := make([]int, n)

	for iter := 0; iter < iterations; iter++ {
		// Assign points to nearest centroid
		for i, p := range points {
			minDist := math.MaxFloat64
			minIdx := 0
			for j, c := range centroids {
				dist := (p.X-c.X)*(p.X-c.X) + (p.Y-c.Y)*(p.Y-c.Y)
				if dist < minDist {
					minDist = dist
					minIdx = j
				}
			}
			assignments[i] = minIdx
		}

		// Update centroids
		counts := make([]int, k)
		newCentroids := make([]Point2D, k)

		for i, p := range points {
			c := assignments[i]
			newCentroids[c].X += p.X
			newCentroids[c].Y += p.Y
			counts[c]++
		}

		for i := 0; i < k; i++ {
			if counts[i] > 0 {
				newCentroids[i].X /= float64(counts[i])
				newCentroids[i].Y /= float64(counts[i])
			}
		}
		centroids = newCentroids
	}

	return assignments, centroids
}

// =============================================================================
// Helper functions
// =============================================================================

func normalize(v []float64) {
	var norm float64
	for _, val := range v {
		norm += val * val
	}
	norm = math.Sqrt(norm)
	if norm > 1e-10 {
		for i := range v {
			v[i] /= norm
		}
	}
}

func dotProduct(a, b []float64) float64 {
	var sum float64
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}

func squaredDist(a, b []float64) float64 {
	var sum float64
	for i := range a {
		diff := a[i] - b[i]
		sum += diff * diff
	}
	return sum
}

// SortPointsByDistance sorts points by distance from origin
func SortPointsByDistance(points []Point2D) {
	sort.Slice(points, func(i, j int) bool {
		distI := points[i].X*points[i].X + points[i].Y*points[i].Y
		distJ := points[j].X*points[j].X + points[j].Y*points[j].Y
		return distI < distJ
	})
}
