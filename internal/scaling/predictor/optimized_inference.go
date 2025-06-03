// =============================
// Optimized ML Inference Engine
// =============================
// This module implements high-performance inference optimizations including
// quantization, pruning, batching, and caching for real-time ML predictions.

package predictor

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"
)

// OptimizedInferenceEngine provides high-performance ML inference with optimization techniques
type OptimizedInferenceEngine struct {
	config           *InferenceConfig
	logger           *zap.SugaredLogger
	modelCache       *ModelCache
	quantizer        *ModelQuantizer
	pruner           *ModelPruner
	batchProcessor   *BatchProcessor
	featureCache     *FeatureCache
	performanceStats *InferenceStats

	// Optimized models
	optimizedModels map[string]*OptimizedModel
	mu              sync.RWMutex

	// Inference pipeline
	inferencePool    *InferencePool
	requestQueue     chan *InferenceRequest
	responseChannels map[string]chan *InferenceResponse
	ctx              context.Context
	cancel           context.CancelFunc

	// Performance monitoring
	latencyTracker  *LatencyTracker
	throughputMeter *ThroughputMeter
	errorCounter    *ErrorCounter
}

// InferenceConfig contains configuration for optimized inference
type InferenceConfig struct {
	MaxBatchSize        int           `json:"max_batch_size"`
	BatchTimeout        time.Duration `json:"batch_timeout"`
	CacheSize           int           `json:"cache_size"`
	CacheTTL            time.Duration `json:"cache_ttl"`
	QuantizationEnabled bool          `json:"quantization_enabled"`
	PruningEnabled      bool          `json:"pruning_enabled"`
	MaxConcurrentJobs   int           `json:"max_concurrent_jobs"`
	MemoryPoolSize      int           `json:"memory_pool_size"`
	NumWorkers          int           `json:"num_workers"`
	QueueSize           int           `json:"queue_size"`
	MaxRetries          int           `json:"max_retries"`

	// Quantization settings
	QuantizationBits   int    `json:"quantization_bits"`
	QuantizationMethod string `json:"quantization_method"`
	CalibrationSamples int    `json:"calibration_samples"`

	// Pruning settings
	PruningRatio       float64 `json:"pruning_ratio"`
	PruningMethod      string  `json:"pruning_method"`
	PruningGranularity string  `json:"pruning_granularity"`

	// Performance targets
	MaxLatencyMs      int     `json:"max_latency_ms"`
	MinThroughputQPS  float64 `json:"min_throughput_qps"`
	AccuracyThreshold float64 `json:"accuracy_threshold"`
}

// OptimizedModel represents a model optimized for inference
type OptimizedModel struct {
	ID               string
	OriginalModel    PredictionModel
	QuantizedModel   *QuantizedModel
	PrunedModel      *PrunedModel
	OptimizationMeta *OptimizationMetadata

	// Performance metrics
	OriginalLatency  time.Duration
	OptimizedLatency time.Duration
	OriginalSize     int64
	OptimizedSize    int64
	AccuracyDrop     float64

	// Runtime state
	IsLoaded       bool
	LoadTime       time.Time
	InferenceCount int64
	LastUsed       time.Time
}

// InferenceRequest represents a request for ML inference
type InferenceRequest struct {
	ID         string
	ModelID    string
	Features   map[string]float64
	Context    context.Context
	StartTime  time.Time
	Priority   int
	Timeout    time.Duration
	ResponseCh chan *InferenceResponse
}

// InferenceResponse represents the response from ML inference
type InferenceResponse struct {
	ID         string
	Prediction *PredictionResult
	Confidence float64
	Latency    time.Duration
	ModelUsed  string
	Error      error
	Metadata   map[string]interface{}
}

// QuantizedModel represents a quantized ML model
type QuantizedModel struct {
	Weights      []int8
	Biases       []int8
	Scales       []float32
	ZeroPoints   []int8
	LayerConfigs []LayerQuantConfig
	Metadata     map[string]interface{}
}

// PrunedModel represents a pruned ML model
type PrunedModel struct {
	PruningMask   []bool
	ActiveWeights []float32
	SparsityRatio float64
	PruningMap    map[string]interface{}
	Metadata      map[string]interface{}
}

// OptimizationMetadata contains metadata about model optimization
type OptimizationMetadata struct {
	AvgLatency time.Duration
	ModelSize  int64
	Accuracy   float64
}

// InferenceStats contains performance statistics for inference
type InferenceStats struct {
	TotalRequests      int64         `json:"total_requests"`
	SuccessfulRequests int64         `json:"successful_requests"`
	FailedRequests     int64         `json:"failed_requests"`
	CacheHits          int64         `json:"cache_hits"`
	CacheMisses        int64         `json:"cache_misses"`
	AvgLatency         time.Duration `json:"avg_latency"`
	P95Latency         time.Duration `json:"p95_latency"`
	P99Latency         time.Duration `json:"p99_latency"`
	ThroughputQPS      float64       `json:"throughput_qps"`
	ErrorRate          float64       `json:"error_rate"`
	OptimizedModels    int           `json:"optimized_models"`
}

// LayerQuantConfig contains quantization configuration for a layer
type LayerQuantConfig struct {
	Bits       int     `json:"bits"`
	Scale      float32 `json:"scale"`
	ZeroPoint  int8    `json:"zero_point"`
	Symmetric  bool    `json:"symmetric"`
	PerChannel bool    `json:"per_channel"`
}

// NewOptimizedInferenceEngine creates a new optimized inference engine
func NewOptimizedInferenceEngine(config *InferenceConfig, logger *zap.SugaredLogger) *OptimizedInferenceEngine {
	ctx, cancel := context.WithCancel(context.Background())

	engine := &OptimizedInferenceEngine{
		config:           config,
		logger:           logger,
		optimizedModels:  make(map[string]*OptimizedModel),
		responseChannels: make(map[string]chan *InferenceResponse),
		requestQueue:     make(chan *InferenceRequest, config.QueueSize),
		ctx:              ctx,
		cancel:           cancel,
	}
	// Initialize components
	engine.modelCache = NewModelCache(config.CacheSize, config.CacheTTL)
	engine.quantizer = NewModelQuantizer(&QuantizationConfig{
		Bits:               config.QuantizationBits,
		Method:             config.QuantizationMethod,
		AsymmetricQuant:    true,
		PerChannelQuant:    true,
		CalibrationMethod:  "entropy",
		CalibrationSamples: config.CalibrationSamples,
	})

	engine.pruner = NewModelPruner(&PruningConfig{
		Ratio:       config.PruningRatio,
		Method:      config.PruningMethod,
		Granularity: config.PruningGranularity,
	})

	engine.batchProcessor = NewBatchProcessor(&BatchConfig{
		MaxBatchSize: config.MaxBatchSize,
		BatchTimeout: config.BatchTimeout,
	})

	engine.featureCache = NewFeatureCache(config.CacheSize, config.CacheTTL)
	engine.performanceStats = NewInferenceStats()
	engine.latencyTracker = NewLatencyTracker()
	engine.throughputMeter = NewThroughputMeter()
	engine.errorCounter = NewErrorCounter()

	// Initialize inference pool
	engine.inferencePool = NewInferencePool(config.MaxConcurrentJobs)

	// Start processing goroutines
	go engine.processRequests()
	go engine.monitorPerformance()

	return engine
}

// QuantizationConfig contains configuration for model quantization
type QuantizationConfig struct {
	Bits               int    `json:"bits"`
	Method             string `json:"method"`
	AsymmetricQuant    bool   `json:"asymmetric_quant"`
	PerChannelQuant    bool   `json:"per_channel_quant"`
	CalibrationMethod  string `json:"calibration_method"`
	CalibrationSamples int    `json:"calibration_samples"`
}

// PruningConfig contains configuration for model pruning
type PruningConfig struct {
	Ratio       float64 `json:"ratio"`
	Method      string  `json:"method"`
	Granularity string  `json:"granularity"`
}

// BatchConfig contains configuration for batch processing
type BatchConfig struct {
	MaxBatchSize int           `json:"max_batch_size"`
	BatchTimeout time.Duration `json:"batch_timeout"`
}

// OptimizeModel applies all optimization techniques to a model
func (e *OptimizedInferenceEngine) OptimizeModel(ctx context.Context, model PredictionModel) (*OptimizedModel, error) {
	e.logger.Infof("Starting optimization for model: %s", model.GetModelType())

	startTime := time.Now()
	optimized := &OptimizedModel{
		ID:            fmt.Sprintf("%s_optimized_%d", model.GetModelType(), time.Now().Unix()),
		OriginalModel: model,
		LoadTime:      startTime,
	}

	// Measure original performance
	originalMetrics, err := e.benchmarkModel(ctx, model)
	if err != nil {
		return nil, fmt.Errorf("failed to benchmark original model: %w", err)
	}
	optimized.OriginalLatency = originalMetrics.AvgLatency
	optimized.OriginalSize = originalMetrics.ModelSize

	// Apply quantization if enabled
	if e.config.QuantizationEnabled {
		e.logger.Info("Applying quantization...")
		quantizedModel, err := e.quantizer.Quantize(ctx, model)
		if err != nil {
			e.logger.Warnf("Quantization failed: %v", err)
		} else {
			optimized.QuantizedModel = quantizedModel
			e.logger.Info("Quantization completed successfully")
		}
	}

	// Apply pruning if enabled
	if e.config.PruningEnabled {
		e.logger.Info("Applying pruning...")
		prunedModel, err := e.pruner.Prune(ctx, model)
		if err != nil {
			e.logger.Warnf("Pruning failed: %v", err)
		} else {
			optimized.PrunedModel = prunedModel
			e.logger.Info("Pruning completed successfully")
		}
	}

	// Measure optimized performance
	optimizedMetrics, err := e.benchmarkOptimizedModel(ctx, optimized)
	if err != nil {
		return nil, fmt.Errorf("failed to benchmark optimized model: %w", err)
	}

	optimized.OptimizedLatency = optimizedMetrics.AvgLatency
	optimized.OptimizedSize = optimizedMetrics.ModelSize
	optimized.AccuracyDrop = originalMetrics.Accuracy - optimizedMetrics.Accuracy

	// Validate optimization results
	if err := e.validateOptimization(optimized); err != nil {
		return nil, fmt.Errorf("optimization validation failed: %w", err)
	}

	optimized.IsLoaded = true

	// Store in registry
	e.mu.Lock()
	e.optimizedModels[optimized.ID] = optimized
	e.mu.Unlock()

	e.logger.Infof("Model optimization completed: latency improvement %.2f%%, size reduction %.2f%%, accuracy drop %.4f",
		float64(originalMetrics.AvgLatency-optimizedMetrics.AvgLatency)/float64(originalMetrics.AvgLatency)*100,
		float64(originalMetrics.ModelSize-optimizedMetrics.ModelSize)/float64(originalMetrics.ModelSize)*100,
		optimized.AccuracyDrop)

	return optimized, nil
}

// PredictOptimized performs optimized inference
func (e *OptimizedInferenceEngine) PredictOptimized(ctx context.Context, modelID string, features map[string]float64) (*InferenceResponse, error) {
	startTime := time.Now()

	// Check feature cache first
	cacheKey := e.generateCacheKey(modelID, features)
	if cached, found := e.featureCache.Get(cacheKey); found {
		e.performanceStats.CacheHits++
		return &InferenceResponse{
			ID:         e.generateRequestID(),
			Prediction: cached.(*PredictionResult),
			Confidence: 1.0,
			Latency:    time.Since(startTime),
			ModelUsed:  modelID,
			Metadata:   map[string]interface{}{"cache_hit": true},
		}, nil
	}

	e.performanceStats.CacheMisses++

	// Create inference request
	request := &InferenceRequest{
		ID:         e.generateRequestID(),
		ModelID:    modelID,
		Features:   features,
		Context:    ctx,
		StartTime:  startTime,
		Priority:   1,
		Timeout:    time.Second * 5,
		ResponseCh: make(chan *InferenceResponse, 1),
	}

	// Submit request
	select {
	case e.requestQueue <- request:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(request.Timeout):
		return nil, fmt.Errorf("request timeout")
	}

	// Wait for response
	select {
	case response := <-request.ResponseCh:
		// Cache successful predictions
		if response.Error == nil {
			e.featureCache.Set(cacheKey, response.Prediction)
		}
		return response, response.Error
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(request.Timeout):
		return nil, fmt.Errorf("inference timeout")
	}
}

// processRequests handles incoming inference requests
func (e *OptimizedInferenceEngine) processRequests() {
	for {
		select {
		case request := <-e.requestQueue:
			go e.handleRequest(request)
		case <-e.ctx.Done():
			return
		}
	}
}

// handleRequest processes a single inference request
func (e *OptimizedInferenceEngine) handleRequest(request *InferenceRequest) {
	defer func() {
		if r := recover(); r != nil {
			e.logger.Errorf("Panic in inference request: %v", r)
			request.ResponseCh <- &InferenceResponse{
				ID:    request.ID,
				Error: fmt.Errorf("internal error during inference"),
			}
		}
	}()

	startTime := time.Now()

	// Get optimized model
	e.mu.RLock()
	optimizedModel, exists := e.optimizedModels[request.ModelID]
	e.mu.RUnlock()

	if !exists {
		request.ResponseCh <- &InferenceResponse{
			ID:    request.ID,
			Error: fmt.Errorf("model not found: %s", request.ModelID),
		}
		return
	}

	// Update usage tracking
	optimizedModel.InferenceCount++
	optimizedModel.LastUsed = time.Now()

	// Perform inference with optimized model
	var prediction *PredictionResult
	var err error

	if optimizedModel.QuantizedModel != nil {
		prediction, err = e.runQuantizedInference(optimizedModel.QuantizedModel, request.Features)
	} else if optimizedModel.PrunedModel != nil {
		prediction, err = e.runPrunedInference(optimizedModel.PrunedModel, request.Features)
	} else {
		prediction, err = optimizedModel.OriginalModel.Predict(request.Context, time.Hour)
	}

	latency := time.Since(startTime)

	// Update performance metrics
	e.latencyTracker.Record(latency)
	e.throughputMeter.Record()
	if err != nil {
		e.errorCounter.Increment()
	}

	response := &InferenceResponse{
		ID:         request.ID,
		Prediction: prediction,
		Confidence: e.calculateConfidence(prediction, optimizedModel),
		Latency:    latency,
		ModelUsed:  request.ModelID,
		Error:      err,
		Metadata: map[string]interface{}{
			"optimization_used": e.getOptimizationInfo(optimizedModel),
			"model_size":        optimizedModel.OptimizedSize,
			"inference_count":   optimizedModel.InferenceCount,
		},
	}

	request.ResponseCh <- response
}

// runQuantizedInference performs inference with quantized model
func (e *OptimizedInferenceEngine) runQuantizedInference(model *QuantizedModel, features map[string]float64) (*PredictionResult, error) {
	// Convert features to quantized format
	quantizedFeatures := e.quantizeFeatures(features, model.Scales, model.ZeroPoints)

	// Perform quantized inference (simplified implementation)
	result := e.computeQuantizedPrediction(quantizedFeatures, model)

	// Dequantize result to LoadMetrics
	dequantizedResult := e.dequantizeResult(result, model.Scales, model.ZeroPoints)

	return &PredictionResult{
		Timestamp:     time.Now(),
		PredictedLoad: dequantizedResult,
		Confidence:    0.93, // Slightly reduced confidence for quantized models
		ModelMetadata: &ModelMetadata{ModelType: "quantized"},
	}, nil
}

// Performance monitoring and optimization validation methods
func (e *OptimizedInferenceEngine) validateOptimization(model *OptimizedModel) error {
	// Check latency improvement
	if model.OptimizedLatency > model.OriginalLatency {
		return fmt.Errorf("optimization increased latency: %v -> %v",
			model.OriginalLatency, model.OptimizedLatency)
	}

	// Check accuracy drop threshold
	if model.AccuracyDrop > e.config.AccuracyThreshold {
		return fmt.Errorf("accuracy drop %.4f exceeds threshold %.4f",
			model.AccuracyDrop, e.config.AccuracyThreshold)
	}

	// Check if latency meets target
	if model.OptimizedLatency > time.Duration(e.config.MaxLatencyMs)*time.Millisecond {
		return fmt.Errorf("optimized latency %v exceeds target %dms",
			model.OptimizedLatency, e.config.MaxLatencyMs)
	}

	return nil
}

// GetPerformanceStats returns current performance statistics
func (e *OptimizedInferenceEngine) GetPerformanceStats() *InferenceStats {
	return &InferenceStats{
		TotalRequests:      e.performanceStats.TotalRequests,
		SuccessfulRequests: e.performanceStats.SuccessfulRequests,
		FailedRequests:     e.performanceStats.FailedRequests,
		CacheHits:          e.performanceStats.CacheHits,
		CacheMisses:        e.performanceStats.CacheMisses,
		AvgLatency:         e.latencyTracker.GetAverage(),
		P95Latency:         e.latencyTracker.GetPercentile(95),
		P99Latency:         e.latencyTracker.GetPercentile(99),
		ThroughputQPS:      e.throughputMeter.GetRate(),
		ErrorRate:          e.errorCounter.GetRate(),
		OptimizedModels:    len(e.optimizedModels),
	}
}

// Helper method implementations
func (e *OptimizedInferenceEngine) benchmarkModel(ctx context.Context, model PredictionModel) (*OptimizationMetadata, error) {
	// Benchmark original model performance
	const numIterations = 10
	var totalLatency time.Duration
	var predictions []*PredictionResult

	for i := 0; i < numIterations; i++ {
		start := time.Now()
		prediction, err := model.Predict(ctx, time.Hour)
		if err != nil {
			e.logger.Warnf("Benchmark prediction failed: %v", err)
			continue
		}

		latency := time.Since(start)
		totalLatency += latency
		predictions = append(predictions, prediction)
	}

	avgLatency := totalLatency / time.Duration(len(predictions))

	// Estimate model size (simplified)
	modelSize := e.estimateModelSize(model)

	// Calculate accuracy metric (simplified)
	accuracy := e.calculateAccuracy(predictions)

	return &OptimizationMetadata{
		AvgLatency: avgLatency,
		ModelSize:  modelSize,
		Accuracy:   accuracy,
	}, nil
}

func (e *OptimizedInferenceEngine) benchmarkOptimizedModel(ctx context.Context, model *OptimizedModel) (*OptimizationMetadata, error) {
	// Benchmark optimized model performance
	const numIterations = 10
	var totalLatency time.Duration
	var predictions []*PredictionResult

	// Create test features for optimized inference
	testFeatures := map[string]float64{
		"cpu_utilization":     0.5,
		"memory_utilization":  0.6,
		"requests_per_second": 100,
		"error_rate":          0.01,
		"latency_p95":         50,
	}

	for i := 0; i < numIterations; i++ {
		start := time.Now()

		var prediction *PredictionResult
		var err error

		// Use optimized inference path
		if model.QuantizedModel != nil {
			prediction, err = e.runQuantizedInference(model.QuantizedModel, testFeatures)
		} else if model.PrunedModel != nil {
			prediction, err = e.runPrunedInference(model.PrunedModel, testFeatures)
		} else {
			prediction, err = model.OriginalModel.Predict(ctx, time.Hour)
		}

		if err != nil {
			e.logger.Warnf("Optimized benchmark prediction failed: %v", err)
			continue
		}

		latency := time.Since(start)
		totalLatency += latency
		predictions = append(predictions, prediction)
	}

	if len(predictions) == 0 {
		return nil, fmt.Errorf("no successful predictions during benchmarking")
	}

	avgLatency := totalLatency / time.Duration(len(predictions))

	// Estimate optimized model size
	modelSize := e.estimateOptimizedModelSize(model)

	// Calculate accuracy (may be slightly reduced for optimized models)
	accuracy := e.calculateAccuracy(predictions)
	if model.QuantizedModel != nil {
		accuracy *= 0.98 // Slight accuracy reduction for quantization
	}
	if model.PrunedModel != nil {
		accuracy *= 0.99 // Minor accuracy reduction for pruning
	}

	return &OptimizationMetadata{
		AvgLatency: avgLatency,
		ModelSize:  modelSize,
		Accuracy:   accuracy,
	}, nil
}

func (e *OptimizedInferenceEngine) generateCacheKey(modelID string, features map[string]float64) string {
	// Create deterministic cache key from model ID and features
	h := sha256.New()
	h.Write([]byte(modelID))

	// Sort feature keys for consistent ordering
	keys := make([]string, 0, len(features))
	for k := range features {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Hash each feature key-value pair
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte(fmt.Sprintf("%.6f", features[k])))
	}

	return fmt.Sprintf("%x", h.Sum(nil))[:16] // Use first 16 chars
}

func (e *OptimizedInferenceEngine) generateRequestID() string {
	// Generate unique request ID using timestamp and random component
	timestamp := time.Now().UnixNano()
	random := rand.Int63n(1000000)
	return fmt.Sprintf("req_%d_%d", timestamp, random)
}

func (e *OptimizedInferenceEngine) monitorPerformance() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			stats := e.GetPerformanceStats()
			e.logger.Infof("Performance stats: QPS=%.2f, ErrorRate=%.4f, P95Latency=%v, OptimizedModels=%d",
				stats.ThroughputQPS, stats.ErrorRate, stats.P95Latency, stats.OptimizedModels)

			// Log cache performance
			cacheHitRate := 0.0
			if stats.CacheHits+stats.CacheMisses > 0 {
				cacheHitRate = float64(stats.CacheHits) / float64(stats.CacheHits+stats.CacheMisses) * 100
			}
			e.logger.Debugf("Cache performance: HitRate=%.2f%%, Hits=%d, Misses=%d",
				cacheHitRate, stats.CacheHits, stats.CacheMisses)

			// Check performance against targets
			if stats.ThroughputQPS < e.config.MinThroughputQPS {
				e.logger.Warnf("Throughput below target: %.2f < %.2f QPS",
					stats.ThroughputQPS, e.config.MinThroughputQPS)
			}

			if stats.P95Latency > time.Duration(e.config.MaxLatencyMs)*time.Millisecond {
				e.logger.Warnf("P95 latency above target: %v > %dms",
					stats.P95Latency, e.config.MaxLatencyMs)
			}

		case <-e.ctx.Done():
			e.logger.Info("Performance monitoring stopped")
			return
		}
	}
}

func (e *OptimizedInferenceEngine) runPrunedInference(model *PrunedModel, features map[string]float64) (*PredictionResult, error) {
	// Implement pruned inference logic using sparse matrix operations

	// Extract input features in normalized order
	inputValues := []float32{
		float32(features["cpu_utilization"]),
		float32(features["memory_utilization"]),
		float32(features["requests_per_second"] / 1000.0), // Normalize
		float32(features["error_rate"] * 100),             // Scale to percentage
		float32(features["latency_p95"] / 100.0),          // Normalize
	}

	// Perform sparse computation using pruning mask
	result := e.computePrunedPrediction(inputValues, model)

	// Convert to LoadMetrics
	loadMetrics := &LoadMetrics{
		CPUUtilization:    float64(result[0]),
		MemoryUtilization: float64(result[1]),
		RequestsPerSecond: float64(result[2]) * 1000, // Denormalize
		LatencyP95Ms:      float64(result[3]) * 100,  // Denormalize
		ErrorRate:         float64(result[4]) / 100,  // Convert back to ratio
	}

	return &PredictionResult{
		PredictedLoad: loadMetrics,
		Confidence:    0.94, // Slightly reduced confidence for pruned models
		Timestamp:     time.Now(),
		ModelMetadata: &ModelMetadata{ModelType: "pruned"},
	}, nil
}

func (e *OptimizedInferenceEngine) computePrunedPrediction(input []float32, model *PrunedModel) []float32 {
	// Simulate sparse matrix multiplication with pruning mask

	// Use a simplified neural network simulation with 3 layers
	layer1Size := 16
	layer2Size := 8
	outputSize := 5

	// Layer 1: Input to hidden
	hidden1 := make([]float32, layer1Size)
	weightIdx := 0

	for i := 0; i < layer1Size; i++ {
		for j := 0; j < len(input); j++ {
			if weightIdx < len(model.PruningMask) && model.PruningMask[weightIdx] {
				// Use active weight if not pruned
				if weightIdx < len(model.ActiveWeights) {
					hidden1[i] += input[j] * model.ActiveWeights[weightIdx]
				}
			}
			// If pruned, weight is 0 (skip multiplication)
			weightIdx++
		}
		// Apply ReLU activation
		if hidden1[i] < 0 {
			hidden1[i] = 0
		}
	}

	// Layer 2: Hidden to hidden
	hidden2 := make([]float32, layer2Size)
	for i := 0; i < layer2Size; i++ {
		for j := 0; j < layer1Size; j++ {
			if weightIdx < len(model.PruningMask) && model.PruningMask[weightIdx] {
				if weightIdx < len(model.ActiveWeights) {
					hidden2[i] += hidden1[j] * model.ActiveWeights[weightIdx]
				}
			}
			weightIdx++
		}
		if hidden2[i] < 0 {
			hidden2[i] = 0
		}
	}

	// Output layer
	output := make([]float32, outputSize)
	for i := 0; i < outputSize; i++ {
		for j := 0; j < layer2Size; j++ {
			if weightIdx < len(model.PruningMask) && model.PruningMask[weightIdx] {
				if weightIdx < len(model.ActiveWeights) {
					output[i] += hidden2[j] * model.ActiveWeights[weightIdx]
				}
			}
			weightIdx++
		}
		// Apply sigmoid activation for output (normalize to 0-1)
		output[i] = 1.0 / (1.0 + float32(math.Exp(-float64(output[i]))))
	}

	return output
}

func (e *OptimizedInferenceEngine) calculateConfidence(prediction *PredictionResult, model *OptimizedModel) float64 {
	// Calculate confidence based on model type and prediction characteristics
	baseConfidence := 0.95

	if prediction == nil || prediction.PredictedLoad == nil {
		return 0.5 // Low confidence for null predictions
	}

	// Reduce confidence based on optimization type
	if model.QuantizedModel != nil {
		baseConfidence *= 0.98 // Slight reduction for quantization
	}
	if model.PrunedModel != nil {
		// Reduction based on sparsity ratio
		sparsityPenalty := model.PrunedModel.SparsityRatio * 0.1
		baseConfidence *= (1.0 - sparsityPenalty)
	}

	// Adjust confidence based on prediction values
	load := prediction.PredictedLoad

	// Check for reasonable ranges
	if load.CPUUtilization < 0 || load.CPUUtilization > 1 {
		baseConfidence *= 0.8 // Penalize out-of-range predictions
	}
	if load.MemoryUtilization < 0 || load.MemoryUtilization > 1 {
		baseConfidence *= 0.8
	}
	if load.ErrorRate < 0 || load.ErrorRate > 1 {
		baseConfidence *= 0.8
	}

	// Factor in model accuracy drop
	if model.AccuracyDrop > 0 {
		baseConfidence *= (1.0 - model.AccuracyDrop)
	}

	// Factor in inference count (higher usage = higher confidence in stability)
	usageFactor := math.Min(float64(model.InferenceCount)/1000.0, 0.05)
	baseConfidence += usageFactor

	// Clamp to reasonable range
	if baseConfidence < 0.5 {
		baseConfidence = 0.5
	}
	if baseConfidence > 0.99 {
		baseConfidence = 0.99
	}

	return baseConfidence
}

func (e *OptimizedInferenceEngine) getOptimizationInfo(model *OptimizedModel) map[string]interface{} {
	info := make(map[string]interface{})

	info["model_id"] = model.ID
	info["original_model_type"] = model.OriginalModel.GetModelType()
	info["is_loaded"] = model.IsLoaded
	info["inference_count"] = model.InferenceCount
	info["last_used"] = model.LastUsed

	// Optimization details
	optimizations := make([]string, 0)
	if model.QuantizedModel != nil {
		optimizations = append(optimizations, "quantization")
		info["quantization_metadata"] = model.QuantizedModel.Metadata
	}
	if model.PrunedModel != nil {
		optimizations = append(optimizations, "pruning")
		info["sparsity_ratio"] = model.PrunedModel.SparsityRatio
		info["pruning_metadata"] = model.PrunedModel.Metadata
	}
	info["optimizations"] = optimizations

	// Performance metrics
	if model.OriginalLatency > 0 && model.OptimizedLatency > 0 {
		improvement := float64(model.OriginalLatency-model.OptimizedLatency) / float64(model.OriginalLatency) * 100
		info["latency_improvement_percent"] = improvement
	}

	if model.OriginalSize > 0 && model.OptimizedSize > 0 {
		reduction := float64(model.OriginalSize-model.OptimizedSize) / float64(model.OriginalSize) * 100
		info["size_reduction_percent"] = reduction
	}

	info["accuracy_drop"] = model.AccuracyDrop

	return info
}

func (e *OptimizedInferenceEngine) quantizeFeatures(features map[string]float64, scales []float32, zeroPoints []int8) []int8 {
	// Convert input features to quantized format
	quantizedValues := make([]int8, 0, len(features))

	// Order features consistently
	featureOrder := []string{"cpu_utilization", "memory_utilization", "requests_per_second", "error_rate", "latency_p95"}

	for i, feature := range featureOrder {
		if value, exists := features[feature]; exists {
			// Normalize feature value
			normalizedValue := value
			if feature == "requests_per_second" {
				normalizedValue = value / 1000.0 // Normalize large values
			} else if feature == "error_rate" {
				normalizedValue = value * 100 // Scale small values
			} else if feature == "latency_p95" {
				normalizedValue = value / 100.0
			} // Apply quantization using scale and zero point
			scale := scales[i%len(scales)]
			zeroPoint := zeroPoints[i%len(zeroPoints)]

			valueToQuantize := math.Round(normalizedValue/float64(scale)) + float64(zeroPoint)

			// Clamp to int8 range before casting
			const minInt8 = -128
			const maxInt8 = 127
			if valueToQuantize < minInt8 {
				valueToQuantize = minInt8
			} else if valueToQuantize > maxInt8 {
				valueToQuantize = maxInt8
			}

			quantizedValue := int8(valueToQuantize)
			quantizedValues = append(quantizedValues, quantizedValue)
		}
	}

	return quantizedValues
}

func (e *OptimizedInferenceEngine) computeQuantizedPrediction(quantizedFeatures []int8, model *QuantizedModel) float64 {
	// Perform quantized inference computation

	// Simulate quantized neural network layers
	layer1Size := 16
	layer2Size := 8

	// Layer 1: Quantized input to hidden layer
	hidden1 := make([]int32, layer1Size) // Use int32 to prevent overflow
	weightIdx := 0

	for i := 0; i < layer1Size && i < len(quantizedFeatures); i++ {
		for j := 0; j < len(quantizedFeatures); j++ {
			if weightIdx < len(model.Weights) {
				// Quantized multiplication: input * weight
				product := int32(quantizedFeatures[j]) * int32(model.Weights[weightIdx])
				hidden1[i] += product
			}
			weightIdx++
		}

		// Add bias if available
		if i < len(model.Biases) {
			hidden1[i] += int32(model.Biases[i])
		}
	}

	// Layer 2: Hidden to output (simplified to single output)
	output := int32(0)
	for i := 0; i < layer2Size && i < len(hidden1); i++ {
		if weightIdx < len(model.Weights) {
			// Use quantized values with saturation to prevent overflow
			if hidden1[i] > 127 {
				hidden1[i] = 127
			} else if hidden1[i] < -128 {
				hidden1[i] = -128
			}

			product := hidden1[i] * int32(model.Weights[weightIdx])
			output += product
		}
		weightIdx++
	}

	// Dequantize to get final result
	if len(model.Scales) > 0 {
		scale := model.Scales[0]
		zeroPoint := int32(0)
		if len(model.ZeroPoints) > 0 {
			zeroPoint = int32(model.ZeroPoints[0])
		}

		dequantized := (float64(output) - float64(zeroPoint)) * float64(scale)

		// Apply sigmoid activation and normalize to 0-1 range
		result := 1.0 / (1.0 + math.Exp(-dequantized/1000.0)) // Scale down for stability
		return result
	}

	// Fallback if no scales available
	return float64(output) / 10000.0 // Simple normalization
}

func (e *OptimizedInferenceEngine) dequantizeResult(result float64, scales []float32, zeroPoints []int8) *LoadMetrics {
	// TODO: Implement dequantization - return LoadMetrics instead of float64
	return &LoadMetrics{
		CPUUtilization:    result,
		MemoryUtilization: result,
		RequestsPerSecond: result * 100,
		LatencyP95Ms:      10,
		ErrorRate:         0.01,
	}
}

// Helper methods for model benchmarking and optimization
func (e *OptimizedInferenceEngine) estimateModelSize(model PredictionModel) int64 {
	// Estimate model size based on type
	switch m := model.(type) {
	case *ARIMAModel:
		// ARIMA models are relatively small
		return int64(len(m.coeffs)*8 + 1024) // coefficients + metadata
	case *LSTMModel:
		// LSTM models are larger due to weight matrices
		hiddenSize := 64
		inputSize := 8
		return int64(hiddenSize*inputSize*4*4*4 + 1024*4) // 4 gates, 4 bytes per float32, metadata
	default:
		return 1024 * 1024 // Default 1MB estimate
	}
}

func (e *OptimizedInferenceEngine) estimateOptimizedModelSize(model *OptimizedModel) int64 {
	// Start with original model size
	originalSize := e.estimateModelSize(model.OriginalModel)

	// Apply size reductions based on optimizations
	if model.QuantizedModel != nil {
		// Quantization typically reduces size by ~4x (float32 to int8)
		originalSize = originalSize / 4
	}

	if model.PrunedModel != nil {
		// Pruning reduces size based on sparsity ratio
		sparsityReduction := 1.0 - model.PrunedModel.SparsityRatio
		originalSize = int64(float64(originalSize) * sparsityReduction)
	}

	// Add overhead for optimization metadata
	overhead := int64(1024) // 1KB overhead
	return originalSize + overhead
}

func (e *OptimizedInferenceEngine) calculateAccuracy(predictions []*PredictionResult) float64 {
	// Simplified accuracy calculation based on consistency
	if len(predictions) < 2 {
		return 0.95 // Default accuracy
	}

	// Calculate coefficient of variation for CPU utilization predictions
	var values []float64
	for _, pred := range predictions {
		if pred.PredictedLoad != nil {
			values = append(values, pred.PredictedLoad.CPUUtilization)
		}
	}

	if len(values) == 0 {
		return 0.95
	}

	// Calculate mean
	mean := 0.0
	for _, v := range values {
		mean += v
	}
	mean /= float64(len(values))

	// Calculate standard deviation
	variance := 0.0
	for _, v := range values {
		variance += (v - mean) * (v - mean)
	}
	variance /= float64(len(values))
	stddev := variance

	// Higher consistency = higher accuracy
	cv := stddev / mean
	accuracy := 1.0 - cv

	// Clamp between 0.8 and 0.99
	if accuracy < 0.8 {
		accuracy = 0.8
	}
	if accuracy > 0.99 {
		accuracy = 0.99
	}

	return accuracy
}

// Shutdown gracefully shuts down the inference engine
func (e *OptimizedInferenceEngine) Shutdown(ctx context.Context) error {
	e.logger.Info("Shutting down optimized inference engine...")

	// Signal shutdown to all goroutines
	e.cancel()

	// Wait for graceful shutdown with timeout
	shutdownTimer := time.NewTimer(10 * time.Second)
	defer shutdownTimer.Stop()

	select {
	case <-ctx.Done():
		e.logger.Warn("Shutdown cancelled by context")
		return ctx.Err()
	case <-shutdownTimer.C:
		e.logger.Info("Shutdown completed")
		return nil
	}
}

// GetModelInfo returns information about a specific optimized model
func (e *OptimizedInferenceEngine) GetModelInfo(modelID string) (*OptimizedModel, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	model, exists := e.optimizedModels[modelID]
	return model, exists
}

// ListOptimizedModels returns a list of all optimized model IDs
func (e *OptimizedInferenceEngine) ListOptimizedModels() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	modelIDs := make([]string, 0, len(e.optimizedModels))
	for id := range e.optimizedModels {
		modelIDs = append(modelIDs, id)
	}
	return modelIDs
}

// UnloadModel removes an optimized model from memory
func (e *OptimizedInferenceEngine) UnloadModel(modelID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if model, exists := e.optimizedModels[modelID]; exists {
		model.IsLoaded = false
		delete(e.optimizedModels, modelID)
		e.logger.Infof("Unloaded optimized model: %s", modelID)
		return nil
	}

	return fmt.Errorf("model not found: %s", modelID)
}

// UpdateConfiguration updates the engine configuration
func (e *OptimizedInferenceEngine) UpdateConfiguration(newConfig *InferenceConfig) error {
	if newConfig == nil {
		return fmt.Errorf("configuration cannot be nil")
	}

	e.logger.Info("Updating inference engine configuration")

	// Validate new configuration
	if newConfig.MaxBatchSize <= 0 {
		return fmt.Errorf("MaxBatchSize must be positive")
	}
	if newConfig.MaxConcurrentJobs <= 0 {
		return fmt.Errorf("MaxConcurrentJobs must be positive")
	}
	if newConfig.AccuracyThreshold < 0 || newConfig.AccuracyThreshold > 1 {
		return fmt.Errorf("AccuracyThreshold must be between 0 and 1")
	}

	// Update configuration (in practice, would need to restart some components)
	e.config = newConfig

	e.logger.Info("Configuration updated successfully")
	return nil
}

// min helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
