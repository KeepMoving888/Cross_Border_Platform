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

	// RAG 检索专用指标
	// 反映向量检索质量、降级频率、延迟与缓存命中率,用于持续调优分块策略与 topK
	RAGSearchDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rag_search_duration_seconds",
			Help:    "RAG search duration in seconds (includes embedding + vector/tfidf query)",
			Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10},
		},
		[]string{"strategy"}, // strategy: vector | tfidf | cache_hit
	)

	RAGSearchScore = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rag_search_score",
			Help:    "RAG search top-1 similarity score (0-1, higher is better)",
			Buckets: []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 0.95},
		},
		[]string{"strategy"}, // strategy: vector | tfidf
	)

	RAGSearchTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rag_search_total",
			Help: "Total number of RAG searches, labeled by strategy and status",
		},
		[]string{"strategy", "status"}, // strategy: vector|tfidf|cache_hit, status: success|failed|empty
	)

	RAGFallbackTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rag_fallback_total",
			Help: "Number of RAG searches that fell back from vector to TF-IDF",
		},
		[]string{"reason"}, // reason: pg_unavailable | embed_failed | query_failed | no_results
	)

	RAGCacheHitsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rag_cache_hits_total",
			Help: "Number of RAG searches served from Redis cache",
		},
		[]string{"knowledge_base_id"},
	)

	RAGIndexDocsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rag_index_docs_total",
			Help: "Total number of documents indexed by RAG",
		},
		[]string{"status"}, // status: success | failed
	)

	RAGIndexChunks = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "rag_index_chunks_per_doc",
			Help:    "Number of chunks generated per indexed document",
			Buckets: []float64{1, 2, 5, 10, 20, 50, 100, 200},
		},
	)

	// RAG Reranker 指标
	// 反映重排序延迟与成功率,标签 strategy: api(调用外部API)/heuristic(本地启发式)/failed
	RAGRerankDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "rag_rerank_duration_seconds",
			Help:    "RAG rerank duration in seconds (cross-encoder or heuristic)",
			Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5},
		},
	)

	RAGRerankTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rag_rerank_total",
			Help: "Total number of RAG rerank operations, labeled by strategy",
		},
		[]string{"strategy"}, // strategy: success(API) | heuristic | failed
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
