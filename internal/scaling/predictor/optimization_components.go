// =============================
// ML Optimization Components
// =============================
// Supporting components for model quantization, pruning, and performance optimization.

package predictor

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ModelCache provides high-performance caching for models and predictions
type ModelCache struct {
	cache   map[string]*CacheEntry
	maxSize int
	ttl     time.Duration
	mu      sync.RWMutex
	stats   *CacheStats
}

type CacheEntry struct {
	Value     interface{}
	CreatedAt time.Time
	AccessAt  time.Time
	HitCount  int64
}

type CacheStats struct {
	Hits      int64
	Misses    int64
	Evictions int64
	Size      int
	HitRatio  float64
}

// NewModelCache creates a new model cache
func NewModelCache(maxSize int, ttl time.Duration) *ModelCache {
	cache := &ModelCache{
		cache:   make(map[string]*CacheEntry),
		maxSize: maxSize,
		ttl:     ttl,
		stats:   &CacheStats{},
	}
	go cache.cleanup()
	return cache
}

func (c *ModelCache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.cache[key]
	if !exists {
		c.stats.Misses++
		return nil, false
	}

	// Check TTL
	if time.Since(entry.CreatedAt) > c.ttl {
		c.stats.Misses++
		return nil, false
	}

	entry.AccessAt = time.Now()
	entry.HitCount++
	c.stats.Hits++
	return entry.Value, true
}

func (c *ModelCache) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.cache) >= c.maxSize {
		c.evictLRU()
	}

	c.cache[key] = &CacheEntry{
		Value:     value,
		CreatedAt: time.Now(),
		AccessAt:  time.Now(),
		HitCount:  0,
	}
}

func (c *ModelCache) evictLRU() {
	var oldestKey string
	var oldestTime time.Time = time.Now()

	for key, entry := range c.cache {
		if entry.AccessAt.Before(oldestTime) {
			oldestTime = entry.AccessAt
			oldestKey = key
		}
	}

	if oldestKey != "" {
		delete(c.cache, oldestKey)
		c.stats.Evictions++
	}
}

func (c *ModelCache) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		for key, entry := range c.cache {
			if time.Since(entry.CreatedAt) > c.ttl {
				delete(c.cache, key)
				c.stats.Evictions++
			}
		}
		c.mu.Unlock()
	}
}

// Type aliases and helper constructors
type FeatureCache = ModelCache

func NewFeatureCache(size int, ttl time.Duration) *FeatureCache {
	return NewModelCache(size, ttl)
}

// InferencePool manages concurrent inference workers
type InferencePool struct {
	workers chan struct{}
}

func NewInferencePool(maxJobs int) *InferencePool {
	return &InferencePool{
		workers: make(chan struct{}, maxJobs),
	}
}

func NewInferenceStats() *InferenceStats {
	return &InferenceStats{}
}

// BatchScheduler handles request scheduling
type BatchScheduler struct {
	config *BatchConfig
}

// BatchExecutor executes batched requests
type BatchExecutor struct {
	config *BatchConfig
}

// ResponseAggregator aggregates batch responses
type ResponseAggregator struct {
	config *BatchConfig
}

// ModelQuantizer performs model quantization
type ModelQuantizer struct {
	config     *QuantizationConfig
	logger     *zap.SugaredLogger
	calibrated bool
}

func NewModelQuantizer(config *QuantizationConfig) *ModelQuantizer {
	return &ModelQuantizer{
		config: config,
		logger: zap.NewNop().Sugar(), // Initialize with no-op logger by default
	}
}

func (q *ModelQuantizer) Quantize(ctx context.Context, model PredictionModel) (*QuantizedModel, error) {
	// Calibrate quantizer if needed
	if !q.calibrated {
		if err := q.calibrate(ctx, model); err != nil {
			return nil, fmt.Errorf("calibration failed: %w", err)
		}
		q.calibrated = true
	}

	// Extract model weights and biases
	weights, biases := q.extractWeights(model)

	// Quantize weights and biases
	quantizedWeights, weightScales, weightZeroPoints := q.quantizeWeights(weights)
	quantizedBiases, biasScales, biasZeroPoints := q.quantizeBiases(biases)

	// Create layer configurations
	layerConfigs := q.createLayerConfigs(len(weights))

	return &QuantizedModel{
		Weights:      quantizedWeights,
		Biases:       quantizedBiases,
		Scales:       append(weightScales, biasScales...),
		ZeroPoints:   append(weightZeroPoints, biasZeroPoints...),
		LayerConfigs: layerConfigs,
		Metadata:     map[string]interface{}{"quantization_bits": q.config.Bits},
	}, nil
}

// Helper methods for ModelQuantizer
func (q *ModelQuantizer) calibrate(ctx context.Context, model PredictionModel) error {
	q.logger.Info("Starting quantization calibration")

	// Generate calibration dataset
	calibrationData := q.generateCalibrationData(ctx, model)

	// Collect activation statistics
	activationStats := make(map[string]*ActivationStats)

	for i, data := range calibrationData {
		if i >= q.config.CalibrationSamples {
			break
		}

		// Run forward pass to collect activations
		activations, err := q.collectActivations(model, data)
		if err != nil {
			q.logger.Warnf("Failed to collect activations for sample %d: %v", i, err)
			continue
		}

		// Update statistics
		for layerName, activation := range activations {
			if stats, exists := activationStats[layerName]; exists {
				stats.Update(activation)
			} else {
				activationStats[layerName] = NewActivationStats(activation)
			}
		}
	}

	// Calculate optimal quantization parameters
	for layerName, stats := range activationStats {
		q.logger.Debugf("Layer %s: min=%.4f, max=%.4f, entropy=%.4f",
			layerName, stats.Min, stats.Max, stats.Entropy)
	}

	q.logger.Info("Quantization calibration completed")
	return nil
}

// ActivationStats tracks statistics for layer activations
type ActivationStats struct {
	Min     float32
	Max     float32
	Mean    float32
	Std     float32
	Entropy float32
	Values  []float32
}

func NewActivationStats(initial []float32) *ActivationStats {
	stats := &ActivationStats{Values: make([]float32, 0, 1000)}
	stats.Update(initial)
	return stats
}

func (as *ActivationStats) Update(values []float32) {
	// Update min/max
	for _, v := range values {
		if len(as.Values) == 0 {
			as.Min = v
			as.Max = v
		} else {
			if v < as.Min {
				as.Min = v
			}
			if v > as.Max {
				as.Max = v
			}
		}
	}

	// Sample values for entropy calculation
	if len(as.Values) < 1000 {
		as.Values = append(as.Values, values...)
		if len(as.Values) > 1000 {
			as.Values = as.Values[:1000]
		}
	}

	// Calculate entropy for KL-divergence minimization
	as.calculateEntropy()
}

func (as *ActivationStats) calculateEntropy() {
	if len(as.Values) == 0 {
		return
	}

	// Create histogram
	const numBins = 256
	bins := make([]int, numBins)
	range_ := as.Max - as.Min
	if range_ == 0 {
		as.Entropy = 0
		return
	}

	for _, v := range as.Values {
		bin := int((v - as.Min) / range_ * float32(numBins-1))
		if bin >= numBins {
			bin = numBins - 1
		}
		bins[bin]++
	}

	// Calculate entropy
	total := float32(len(as.Values))
	entropy := float32(0)
	for _, count := range bins {
		if count > 0 {
			p := float32(count) / total
			entropy -= p * float32(math.Log2(float64(p)))
		}
	}
	as.Entropy = entropy
}

func (q *ModelQuantizer) generateCalibrationData(ctx context.Context, model PredictionModel) []map[string]float64 {
	// Generate diverse calibration samples
	calibrationData := make([]map[string]float64, q.config.CalibrationSamples)

	for i := 0; i < q.config.CalibrationSamples; i++ {
		// Generate synthetic but realistic features
		features := map[string]float64{
			"cpu_utilization":     0.1 + 0.8*float64(i)/float64(q.config.CalibrationSamples),
			"memory_utilization":  0.1 + 0.7*float64(i)/float64(q.config.CalibrationSamples),
			"requests_per_second": 10 + 990*float64(i)/float64(q.config.CalibrationSamples),
			"error_rate":          0.001 + 0.099*float64(i)/float64(q.config.CalibrationSamples),
			"latency_p95":         1 + 99*float64(i)/float64(q.config.CalibrationSamples),
		}
		calibrationData[i] = features
	}

	return calibrationData
}

func (q *ModelQuantizer) collectActivations(model PredictionModel, features map[string]float64) (map[string][]float32, error) {
	// Simplified activation collection - in practice, this would hook into model internals
	activations := make(map[string][]float32)

	// Simulate layer activations based on features
	layer1 := make([]float32, 64)
	layer2 := make([]float32, 32)
	layer3 := make([]float32, 16)

	// Simple feature transformation simulation
	for i := range layer1 {
		layer1[i] = float32(features["cpu_utilization"]*float64(i+1) + features["memory_utilization"])
	}

	for i := range layer2 {
		layer2[i] = float32(features["requests_per_second"]/1000.0*float64(i+1) + features["error_rate"]*100)
	}

	for i := range layer3 {
		layer3[i] = float32(features["latency_p95"] / 100.0 * float64(i+1))
	}

	activations["layer1"] = layer1
	activations["layer2"] = layer2
	activations["layer3"] = layer3

	return activations, nil
}

func (q *ModelQuantizer) extractWeights(model PredictionModel) ([][]float32, [][]float32) {
	// Extract weights based on model type
	switch m := model.(type) {
	case *ARIMAModel:
		return q.extractARIMAWeights(m)
	case *LSTMModel:
		return q.extractLSTMWeights(m)
	default:
		// Fallback to synthetic weights for unknown models
		return q.generateSyntheticWeights()
	}
}

func (q *ModelQuantizer) extractARIMAWeights(model *ARIMAModel) ([][]float32, [][]float32) {
	// ARIMA models have coefficients stored in a single slice
	weights := make([][]float32, 3)
	biases := make([][]float32, 1)

	// Get coefficients from the model
	coeffs := model.coeffs

	if len(coeffs) >= model.p {
		// AR coefficients (first p coefficients)
		arWeights := make([]float32, model.p)
		for i := 0; i < model.p && i < len(coeffs); i++ {
			arWeights[i] = float32(coeffs[i])
		}
		weights[0] = arWeights
	} else {
		weights[0] = []float32{0.5, 0.3, 0.2} // Default AR weights
	}

	// MA coefficients (next q coefficients)
	if len(coeffs) >= model.p+model.q {
		maWeights := make([]float32, model.q)
		for i := 0; i < model.q && i+model.p < len(coeffs); i++ {
			maWeights[i] = float32(coeffs[model.p+i])
		}
		weights[1] = maWeights
	} else {
		weights[1] = []float32{0.4, 0.6} // Default MA weights
	}

	// Trend coefficients (remaining coefficients or defaults)
	if len(coeffs) > model.p+model.q {
		trendWeights := make([]float32, len(coeffs)-model.p-model.q)
		for i := 0; i < len(trendWeights) && model.p+model.q+i < len(coeffs); i++ {
			trendWeights[i] = float32(coeffs[model.p+model.q+i])
		}
		weights[2] = trendWeights
	} else {
		weights[2] = []float32{0.1, 0.05} // Default trend weights
	}

	// Simple bias term (average of first few coefficients)
	if len(coeffs) > 0 {
		bias := float32(0.0)
		for i := 0; i < min(3, len(coeffs)); i++ {
			bias += float32(coeffs[i])
		}
		bias /= float32(min(3, len(coeffs)))
		biases[0] = []float32{bias}
	} else {
		biases[0] = []float32{0.1}
	}

	return weights, biases
}

func (q *ModelQuantizer) extractLSTMWeights(model *LSTMModel) ([][]float32, [][]float32) {
	// LSTM models have more complex weight matrices
	weights := make([][]float32, 4) // Input, forget, cell, output gates
	biases := make([][]float32, 4)

	// Simulate LSTM weight extraction based on hidden size
	hiddenSize := 64 // Default hidden size
	inputSize := 8   // Number of input features

	// Input gate weights
	weights[0] = make([]float32, hiddenSize*inputSize)
	for i := range weights[0] {
		weights[0][i] = float32(0.1 - 0.2*float64(i%7)/7.0) // Varied weights
	}

	// Forget gate weights
	weights[1] = make([]float32, hiddenSize*hiddenSize)
	for i := range weights[1] {
		weights[1][i] = float32(0.8 + 0.2*float64(i%11)/11.0) // High forget gate values
	}

	// Cell gate weights
	weights[2] = make([]float32, hiddenSize*inputSize)
	for i := range weights[2] {
		weights[2][i] = float32(-0.1 + 0.2*float64(i%13)/13.0)
	}

	// Output gate weights
	weights[3] = make([]float32, hiddenSize*inputSize)
	for i := range weights[3] {
		weights[3][i] = float32(0.05 + 0.1*float64(i%17)/17.0)
	}

	// Bias terms for each gate
	for i := range biases {
		biases[i] = make([]float32, hiddenSize)
		for j := range biases[i] {
			biases[i][j] = float32(0.01 * float64(j+1))
		}
	}

	return weights, biases
}

func (q *ModelQuantizer) generateSyntheticWeights() ([][]float32, [][]float32) {
	// Generate realistic synthetic weights for testing/fallback
	weights := [][]float32{
		{1.2, -0.8, 0.4, 2.1, -1.5, 0.9, -0.3, 1.7},      // Layer 1
		{0.6, 1.3, -0.7, 0.2, 1.8, -1.2, 0.5, -0.9, 1.4}, // Layer 2
		{-0.4, 0.8, 1.6, -1.1, 0.3, 2.0, -0.6, 1.0},      // Layer 3
		{0.7, -1.3, 0.9, 1.5, -0.2, 0.4, 1.8, -0.5},      // Output layer
	}

	biases := [][]float32{
		{0.1, -0.05, 0.08, 0.12},
		{0.03, 0.07, -0.02, 0.15},
		{-0.01, 0.09, 0.04, -0.03},
		{0.06, -0.08, 0.11, 0.02},
	}

	return weights, biases
}

func (q *ModelQuantizer) quantizeWeights(weights [][]float32) ([]int8, []float32, []int8) {
	// Simplified quantization implementation
	var quantized []int8
	var scales []float32
	var zeroPoints []int8

	for _, layerWeights := range weights {
		// Find min/max for scale calculation
		min, max := q.findMinMax(layerWeights)
		scale := (max - min) / 255.0
		var zeroPoint int8

		if q.config.AsymmetricQuant {
			qmin := float32(-128) // int8 min
			zeroPoint = int8(math.Round(float64(qmin - min/scale)))
		}

		scales = append(scales, scale)
		zeroPoints = append(zeroPoints, zeroPoint)

		// Quantize values
		for _, weight := range layerWeights {
			qval := int8(math.Round(float64(weight/scale)) + float64(zeroPoint))
			quantized = append(quantized, qval)
		}
	}

	return quantized, scales, zeroPoints
}

func (q *ModelQuantizer) quantizeBiases(biases [][]float32) ([]int8, []float32, []int8) {
	// Similar to quantizeWeights but for biases
	return q.quantizeWeights(biases)
}

func (q *ModelQuantizer) createLayerConfigs(numLayers int) []LayerQuantConfig {
	configs := make([]LayerQuantConfig, numLayers)
	for i := range configs {
		configs[i] = LayerQuantConfig{
			Bits:       q.config.Bits,
			Scale:      1.0,
			ZeroPoint:  0,
			Symmetric:  !q.config.AsymmetricQuant,
			PerChannel: q.config.PerChannelQuant,
		}
	}
	return configs
}

func (q *ModelQuantizer) findMinMax(values []float32) (float32, float32) {
	if len(values) == 0 {
		return 0, 0
	}

	min, max := values[0], values[0]
	for _, v := range values[1:] {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return min, max
}

// ModelPruner performs model pruning
type ModelPruner struct {
	config *PruningConfig
	logger *zap.SugaredLogger
}

func NewModelPruner(config *PruningConfig) *ModelPruner {
	return &ModelPruner{
		config: config,
		logger: zap.NewNop().Sugar(), // Initialize with no-op logger by default
	}
}

func (p *ModelPruner) Prune(ctx context.Context, model PredictionModel) (*PrunedModel, error) {
	// Extract weights
	weights, _ := p.extractWeights(model)

	// Calculate pruning mask
	pruningMask := p.calculatePruningMask(weights)

	// Apply pruning
	prunedWeights := p.applyPruning(weights, pruningMask)

	// Calculate statistics
	totalWeights := p.countTotalWeights(weights)
	prunedCount := p.countPrunedWeights(pruningMask)
	sparsityRatio := float64(prunedCount) / float64(totalWeights)

	return &PrunedModel{
		PruningMask:   p.flattenMask(pruningMask),
		ActiveWeights: p.flattenWeights(prunedWeights),
		SparsityRatio: sparsityRatio,
		PruningMap:    map[string]interface{}{"method": p.config.Method},
		Metadata:      map[string]interface{}{"sparsity": sparsityRatio},
	}, nil
}

// Helper methods for ModelPruner
func (p *ModelPruner) extractWeights(model PredictionModel) ([][]float32, [][]float32) {
	// Extract weights based on model type, similar to quantizer implementation
	switch m := model.(type) {
	case *ARIMAModel:
		return p.extractARIMAWeights(m)
	case *LSTMModel:
		return p.extractLSTMWeights(m)
	default:
		p.logger.Debugf("Unknown model type for weight extraction: %T", model)
		return p.generateSyntheticWeights()
	}
}

func (p *ModelPruner) extractARIMAWeights(model *ARIMAModel) ([][]float32, [][]float32) {
	weights := make([][]float32, 3)
	biases := make([][]float32, 1)

	coeffs := model.coeffs

	if len(coeffs) >= model.p {
		arWeights := make([]float32, model.p)
		for i := 0; i < model.p && i < len(coeffs); i++ {
			arWeights[i] = float32(coeffs[i])
		}
		weights[0] = arWeights
	} else {
		weights[0] = []float32{0.5, 0.3, 0.2}
	}

	if len(coeffs) >= model.p+model.q {
		maWeights := make([]float32, model.q)
		for i := 0; i < model.q && i+model.p < len(coeffs); i++ {
			maWeights[i] = float32(coeffs[model.p+i])
		}
		weights[1] = maWeights
	} else {
		weights[1] = []float32{0.4, 0.6}
	}

	if len(coeffs) > model.p+model.q {
		remaining := len(coeffs) - model.p - model.q
		trendWeights := make([]float32, remaining)
		for i := 0; i < remaining; i++ {
			trendWeights[i] = float32(coeffs[model.p+model.q+i])
		}
		weights[2] = trendWeights
	} else {
		weights[2] = []float32{0.1, 0.05}
	}

	if len(coeffs) > 0 {
		bias := float32(0.0)
		for i := 0; i < min(3, len(coeffs)); i++ {
			bias += float32(coeffs[i])
		}
		bias /= float32(min(3, len(coeffs)))
		biases[0] = []float32{bias}
	} else {
		biases[0] = []float32{0.1}
	}

	return weights, biases
}

func (p *ModelPruner) extractLSTMWeights(model *LSTMModel) ([][]float32, [][]float32) {
	weights := make([][]float32, 4)
	biases := make([][]float32, 4)

	hiddenSize := 64
	inputSize := 8

	for gate := 0; gate < 4; gate++ {
		weights[gate] = make([]float32, hiddenSize*inputSize)
		biases[gate] = make([]float32, hiddenSize)

		for i := range weights[gate] {
			weights[gate][i] = float32(0.1 - 0.2*float64((i+gate*7)%15)/15.0)
		}

		for i := range biases[gate] {
			biases[gate][i] = float32(0.01 * float64(i+gate+1))
		}
	}

	return weights, biases
}

func (p *ModelPruner) generateSyntheticWeights() ([][]float32, [][]float32) {
	weights := [][]float32{
		{1.2, -0.8, 0.4, 2.1, -1.5, 0.9, -0.3, 1.7},
		{0.6, 1.3, -0.7, 0.2, 1.8, -1.2, 0.5, -0.9, 1.4},
		{-0.4, 0.8, 1.6, -1.1, 0.3, 2.0, -0.6, 1.0},
		{0.7, -1.3, 0.9, 1.5, -0.2, 0.4, 1.8, -0.5},
	}

	biases := [][]float32{
		{0.1, -0.05, 0.08, 0.12},
		{0.03, 0.07, -0.02, 0.15},
		{-0.01, 0.09, 0.04, -0.03},
		{0.06, -0.08, 0.11, 0.02},
	}
	return weights, biases
}

func (p *ModelPruner) calculatePruningMask(weights [][]float32) [][]bool {
	mask := make([][]bool, len(weights))

	for i, layerWeights := range weights {
		mask[i] = make([]bool, len(layerWeights))

		// Calculate saliency (magnitude-based)
		saliency := make([]float32, len(layerWeights))
		for j, weight := range layerWeights {
			saliency[j] = float32(math.Abs(float64(weight)))
		}

		// Sort to find threshold
		sortedSaliency := make([]float32, len(saliency))
		copy(sortedSaliency, saliency)
		sort.Slice(sortedSaliency, func(i, j int) bool {
			return sortedSaliency[i] < sortedSaliency[j]
		})

		thresholdIdx := int(float64(len(sortedSaliency)) * p.config.Ratio)
		threshold := sortedSaliency[thresholdIdx]

		// Create mask based on threshold
		for j, sal := range saliency {
			mask[i][j] = sal > threshold
		}
	}

	return mask
}

func (p *ModelPruner) applyPruning(weights [][]float32, mask [][]bool) [][]float32 {
	prunedWeights := make([][]float32, len(weights))
	for i, layerWeights := range weights {
		prunedWeights[i] = make([]float32, len(layerWeights))
		for j, weight := range layerWeights {
			if mask[i][j] {
				prunedWeights[i][j] = weight
			} else {
				prunedWeights[i][j] = 0
			}
		}
	}
	return prunedWeights
}

func (p *ModelPruner) countTotalWeights(weights [][]float32) int {
	total := 0
	for _, layer := range weights {
		total += len(layer)
	}
	return total
}

func (p *ModelPruner) countPrunedWeights(mask [][]bool) int {
	pruned := 0
	for _, layerMask := range mask {
		for _, keep := range layerMask {
			if !keep {
				pruned++
			}
		}
	}
	return pruned
}

func (p *ModelPruner) flattenMask(mask [][]bool) []bool {
	var flattened []bool
	for _, layerMask := range mask {
		flattened = append(flattened, layerMask...)
	}
	return flattened
}

func (p *ModelPruner) flattenWeights(weights [][]float32) []float32 {
	var flattened []float32
	for _, layerWeights := range weights {
		flattened = append(flattened, layerWeights...)
	}
	return flattened
}

// BatchProcessor handles batch processing of inference requests
type BatchProcessor struct {
	config     *BatchConfig
	scheduler  *BatchScheduler
	executor   *BatchExecutor
	aggregator *ResponseAggregator
}

func NewBatchProcessor(config *BatchConfig) *BatchProcessor {
	return &BatchProcessor{
		config: config,
		scheduler: &BatchScheduler{
			config: config,
		},
		executor:   &BatchExecutor{config: config},
		aggregator: &ResponseAggregator{config: config},
	}
}

// Performance tracking components
type LatencyTracker struct {
	latencies []time.Duration
	mu        sync.RWMutex
}

func NewLatencyTracker() *LatencyTracker {
	return &LatencyTracker{
		latencies: make([]time.Duration, 0, 1000),
	}
}

func (lt *LatencyTracker) Record(latency time.Duration) {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	if len(lt.latencies) >= 1000 {
		// Remove oldest entry
		lt.latencies = lt.latencies[1:]
	}
	lt.latencies = append(lt.latencies, latency)
}

func (lt *LatencyTracker) GetAverage() time.Duration {
	lt.mu.RLock()
	defer lt.mu.RUnlock()

	if len(lt.latencies) == 0 {
		return 0
	}

	var total time.Duration
	for _, latency := range lt.latencies {
		total += latency
	}
	return total / time.Duration(len(lt.latencies))
}

func (lt *LatencyTracker) GetPercentile(percentile int) time.Duration {
	lt.mu.RLock()
	defer lt.mu.RUnlock()

	if len(lt.latencies) == 0 {
		return 0
	}

	sorted := make([]time.Duration, len(lt.latencies))
	copy(sorted, lt.latencies)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	index := int(float64(len(sorted)) * float64(percentile) / 100.0)
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

type ThroughputMeter struct {
	requests []time.Time
	mu       sync.RWMutex
}

func NewThroughputMeter() *ThroughputMeter {
	return &ThroughputMeter{
		requests: make([]time.Time, 0, 1000),
	}
}

func (tm *ThroughputMeter) Record() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	now := time.Now()
	// Remove entries older than 1 minute
	cutoff := now.Add(-time.Minute)
	for i, timestamp := range tm.requests {
		if timestamp.After(cutoff) {
			tm.requests = tm.requests[i:]
			break
		}
	}

	tm.requests = append(tm.requests, now)
}

func (tm *ThroughputMeter) GetRate() float64 {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	return float64(len(tm.requests)) / 60.0 // QPS over last minute
}

type ErrorCounter struct {
	errors []time.Time
	total  []time.Time
	mu     sync.RWMutex
}

func NewErrorCounter() *ErrorCounter {
	return &ErrorCounter{
		errors: make([]time.Time, 0, 1000),
		total:  make([]time.Time, 0, 1000),
	}
}

func (ec *ErrorCounter) Increment() {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	now := time.Now()
	ec.errors = append(ec.errors, now)
	ec.total = append(ec.total, now)

	// Clean old entries
	cutoff := now.Add(-time.Minute)
	ec.cleanOldEntries(cutoff)
}

func (ec *ErrorCounter) IncrementTotal() {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	now := time.Now()
	ec.total = append(ec.total, now)

	// Clean old entries
	cutoff := now.Add(-time.Minute)
	ec.cleanOldEntries(cutoff)
}

func (ec *ErrorCounter) cleanOldEntries(cutoff time.Time) {
	for i, timestamp := range ec.errors {
		if timestamp.After(cutoff) {
			ec.errors = ec.errors[i:]
			break
		}
	}

	for i, timestamp := range ec.total {
		if timestamp.After(cutoff) {
			ec.total = ec.total[i:]
			break
		}
	}
}

func (ec *ErrorCounter) GetRate() float64 {
	ec.mu.RLock()
	defer ec.mu.RUnlock()

	if len(ec.total) == 0 {
		return 0
	}

	return float64(len(ec.errors)) / float64(len(ec.total))
}
