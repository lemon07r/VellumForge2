package metrics

import (
	"log/slog"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// API metrics
	apiRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "vellumforge_api_request_duration_seconds",
			Help:    "API request duration in seconds by model",
			Buckets: prometheus.ExponentialBuckets(0.1, 2, 10), // 0.1s to ~100s
		},
		[]string{"model", "status"},
	)

	rateLimiterWaitDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "vellumforge_rate_limiter_wait_duration_seconds",
			Help:    "Rate limiter wait duration in seconds by model",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms to ~32s
		},
		[]string{"model"},
	)

	// Worker metrics
	workerQueueDepth = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vellumforge_worker_queue_depth",
			Help: "Current depth of worker job queue",
		},
		[]string{"phase"}, // "prompts" or "pairs"
	)

	jobProcessingDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "vellumforge_job_processing_duration_seconds",
			Help:    "Job processing duration breakdown by stage",
			Buckets: prometheus.ExponentialBuckets(0.5, 2, 10), // 0.5s to ~500s
		},
		[]string{"stage"}, // "chosen", "rejected", "judge", "total"
	)

	// Generation metrics
	generationThroughput = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vellumforge_generation_total",
			Help: "Total number of generations completed",
		},
		[]string{"stage", "status"}, // stage: "subtopics"/"prompts"/"pairs", status: "success"/"error"
	)

	activeWorkers = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vellumforge_active_workers",
			Help: "Number of active workers by phase",
		},
		[]string{"phase"},
	)
)

// Collector provides convenience methods for recording metrics
type Collector struct {
	logger *slog.Logger
	mu     sync.Mutex
}

// NewCollector creates a new metrics collector
func NewCollector(logger *slog.Logger) *Collector {
	return &Collector{
		logger: logger,
	}
}

// RecordAPIRequest records an API request duration
func (c *Collector) RecordAPIRequest(model string, duration time.Duration, success bool) {
	status := "success"
	if !success {
		status = "error"
	}
	apiRequestDuration.WithLabelValues(model, status).Observe(duration.Seconds())
}

// RecordRateLimiterWait records rate limiter wait time
func (c *Collector) RecordRateLimiterWait(model string, duration time.Duration) {
	rateLimiterWaitDuration.WithLabelValues(model).Observe(duration.Seconds())
}

// SetWorkerQueueDepth sets the current queue depth
func (c *Collector) SetWorkerQueueDepth(phase string, depth int) {
	workerQueueDepth.WithLabelValues(phase).Set(float64(depth))
}

// RecordJobProcessing records job processing duration by stage
func (c *Collector) RecordJobProcessing(stage string, duration time.Duration) {
	jobProcessingDuration.WithLabelValues(stage).Observe(duration.Seconds())
}

// IncrementGeneration increments generation counter
func (c *Collector) IncrementGeneration(stage string, success bool) {
	status := "success"
	if !success {
		status = "error"
	}
	generationThroughput.WithLabelValues(stage, status).Inc()
}

// SetActiveWorkers sets the number of active workers
func (c *Collector) SetActiveWorkers(phase string, count int) {
	activeWorkers.WithLabelValues(phase).Set(float64(count))
}

// GetMetricsSummary returns a human-readable summary of current metrics
func (c *Collector) GetMetricsSummary() string {
	// This is a placeholder - in practice you'd gather from prometheus registry
	return "Metrics collection enabled. View at http://localhost:2112/metrics"
}
