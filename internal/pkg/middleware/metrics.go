package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total number of HTTP requests",
	}, []string{"method", "path", "status"})

	httpRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request duration in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"})

	httpRequestsInFlight = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "http_requests_in_flight",
		Help: "Number of HTTP requests currently being served",
	})

	httpResponseSize = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_response_size_bytes",
		Help:    "HTTP response size in bytes",
		Buckets: []float64{100, 1000, 5000, 10000, 50000, 100000, 500000},
	}, []string{"method", "path"})

	// AI 工作流专用指标
	AIWorkflowRunsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ai_workflow_runs_total",
			Help: "Total number of AI workflow runs",
		},
		[]string{"workflow_code", "status"},
	)

	AIWorkflowDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ai_workflow_duration_seconds",
			Help:    "AI workflow execution duration in seconds",
			Buckets: []float64{0.5, 1, 2, 5, 10, 30, 60, 120},
		},
		[]string{"workflow_code"},
	)

	AIWorkflowTokens = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ai_workflow_tokens_total",
			Help:    "AI workflow total tokens consumed",
			Buckets: []float64{100, 500, 1000, 2000, 5000, 10000},
		},
		[]string{"workflow_code"},
	)

	AIWorkflowCost = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ai_workflow_cost_usd",
			Help:    "AI workflow cost in USD",
			Buckets: []float64{0.001, 0.01, 0.05, 0.1, 0.5, 1, 5},
		},
		[]string{"workflow_code"},
	)
)

// Metrics Prometheus 指标采集中间件
func Metrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path == "/metrics" {
			c.Next()
			return
		}

		start := time.Now()
		httpRequestsInFlight.Inc()

		c.Next()

		httpRequestsInFlight.Dec()
		duration := time.Since(start).Seconds()

		status := strconv.Itoa(c.Writer.Status())
		path := c.FullPath()
		if path == "" {
			path = "unknown"
		}

		httpRequestsTotal.WithLabelValues(c.Request.Method, path, status).Inc()
		httpRequestDuration.WithLabelValues(c.Request.Method, path).Observe(duration)
		httpResponseSize.WithLabelValues(c.Request.Method, path).Observe(float64(c.Writer.Size()))
	}
}
