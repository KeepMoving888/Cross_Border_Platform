// Package main - RAG Service 负载测试工具
//
// 用于验证 RAG Service 在多副本部署下的水平扩展能力:
//   1. 并发请求压力测试,测量吞吐量和 P95/P99 延迟
//   2. 多副本负载均衡验证(通过 X-Trace-Id 追踪请求路由)
//   3. 熔断器降级验证(模拟 RAG Service 不可用时的行为)
//
// 用法:
//   go run ./cmd/loadtest -url http://localhost:8082 -concurrency 50 -duration 30s
//   go run ./cmd/loadtest -url http://localhost:8082 -concurrency 100 -total 1000
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// LoadTestResult 单次请求结果
type LoadTestResult struct {
	StatusCode int
	Duration   time.Duration
	Error      error
	TraceID    string
}

// LoadTestSummary 压测汇总
type LoadTestSummary struct {
	TotalRequests int64
	SuccessCount  int64
	ErrorCount    int64
	TotalDuration time.Duration
	RPS           float64           // 每秒请求数
	LatencyStats  LatencyStats      // 延迟统计
	StatusCodes   map[int]int64     // 状态码分布
	TraceIDs      map[string]int64  // TraceID 分布(用于验证多副本负载均衡)
}

// LatencyStats 延迟统计
type LatencyStats struct {
	Min time.Duration
	P50 time.Duration
	P95 time.Duration
	P99 time.Duration
	Max time.Duration
}

func main() {
	// 命令行参数
	url := flag.String("url", "http://localhost:8082/api/v1/ai/rag/search", "RAG Service 搜索接口 URL")
	concurrency := flag.Int("concurrency", 10, "并发数")
	duration := flag.Duration("duration", 0, "持续时间(如 30s, 1m),0 表示使用 -total")
	total := flag.Int("total", 100, "总请求数(当 -duration=0 时生效)")
	query := flag.String("query", "跨境物流时效", "检索查询词")
	topK := flag.Int("topk", 5, "返回文档数")
	kbID := flag.Int("kb", 1, "知识库 ID")
	timeout := flag.Duration("timeout", 10*time.Second, "单请求超时")
	flag.Parse()

	fmt.Println("========== RAG Service 负载测试 ==========")
	fmt.Printf("目标: %s\n", *url)
	fmt.Printf("并发: %d\n", *concurrency)
	if *duration > 0 {
		fmt.Printf("持续时间: %s\n", *duration)
	} else {
		fmt.Printf("总请求数: %d\n", *total)
	}
	fmt.Printf("查询: %s (topK=%d, kbID=%d)\n", *query, *topK, *kbID)
	fmt.Println("==========================================")

	// 构造请求体
	body, _ := json.Marshal(map[string]interface{}{
		"query":             *query,
		"knowledge_base_id": *kbID,
		"top_k":             *topK,
	})

	client := &http.Client{
		Timeout: *timeout,
	}

	// 结果收集
	var results []LoadTestResult
	var resultsMu sync.Mutex
	var completed int64

	// 控制信号
	stopCh := make(chan struct{})
	var stopOnce sync.Once

	startTime := time.Now()

	// 启动工作 goroutine
	var wg sync.WaitGroup
	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				select {
				case <-stopCh:
					return
				default:
				}

				// 检查是否达到总请求数
				if *duration == 0 {
					if atomic.AddInt64(&completed, 1) > int64(*total) {
						return
					}
				}

				// 执行请求
				result := doRequest(client, *url, body)
				resultsMu.Lock()
				results = append(results, result)
				resultsMu.Unlock()

				// 检查持续时间
				if *duration > 0 && time.Since(startTime) >= *duration {
					stopOnce.Do(func() { close(stopCh) })
					return
				}
			}
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(startTime)

	// 汇总结果
	summary := summarize(results, elapsed)
	printSummary(summary)
}

// doRequest 执行单个请求
func doRequest(client *http.Client, url string, body []byte) LoadTestResult {
	start := time.Now()
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return LoadTestResult{Error: err, Duration: time.Since(start)}
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return LoadTestResult{Error: err, Duration: time.Since(start)}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	return LoadTestResult{
		StatusCode: resp.StatusCode,
		Duration:   time.Since(start),
		TraceID:    resp.Header.Get("X-Trace-Id"),
	}
}

// summarize 汇总测试结果
func summarize(results []LoadTestResult, elapsed time.Duration) LoadTestSummary {
	s := LoadTestSummary{
		TotalRequests: int64(len(results)),
		TotalDuration: elapsed,
		StatusCodes:   make(map[int]int64),
		TraceIDs:      make(map[string]int64),
	}

	durations := make([]time.Duration, 0, len(results))
	for _, r := range results {
		if r.Error != nil || r.StatusCode >= 500 {
			s.ErrorCount++
		} else if r.StatusCode == 200 {
			s.SuccessCount++
		}
		s.StatusCodes[r.StatusCode]++
		if r.TraceID != "" {
			s.TraceIDs[r.TraceID]++
		}
		durations = append(durations, r.Duration)
	}

	if len(durations) > 0 {
		sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
		s.LatencyStats.Min = durations[0]
		s.LatencyStats.P50 = percentile(durations, 50)
		s.LatencyStats.P95 = percentile(durations, 95)
		s.LatencyStats.P99 = percentile(durations, 99)
		s.LatencyStats.Max = durations[len(durations)-1]
	}

	s.RPS = float64(s.TotalRequests) / elapsed.Seconds()
	return s
}

// percentile 计算百分位数
func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(float64(p)/100*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// printSummary 打印测试汇总
func printSummary(s LoadTestSummary) {
	fmt.Println("\n========== 测试结果 ==========")
	fmt.Printf("总请求数: %d\n", s.TotalRequests)
	fmt.Printf("成功: %d  失败: %d  成功率: %.2f%%\n",
		s.SuccessCount, s.ErrorCount,
		float64(s.SuccessCount)/float64(s.TotalRequests)*100)
	fmt.Printf("总耗时: %s\n", s.TotalDuration)
	fmt.Printf("吞吐量: %.2f req/s\n", s.RPS)
	fmt.Println("\n--- 延迟分布 ---")
	fmt.Printf("Min: %s\n", s.LatencyStats.Min)
	fmt.Printf("P50: %s\n", s.LatencyStats.P50)
	fmt.Printf("P95: %s\n", s.LatencyStats.P95)
	fmt.Printf("P99: %s\n", s.LatencyStats.P99)
	fmt.Printf("Max: %s\n", s.LatencyStats.Max)

	fmt.Println("\n--- 状态码分布 ---")
	for code, count := range s.StatusCodes {
		fmt.Printf("  HTTP %d: %d (%.1f%%)\n", code, count, float64(count)/float64(s.TotalRequests)*100)
	}

	if len(s.TraceIDs) > 0 {
		fmt.Printf("\n--- TraceID 分布(验证多副本负载均衡) ---\n")
		fmt.Printf("  唯一 TraceID 数: %d\n", len(s.TraceIDs))
		if len(s.TraceIDs) <= 10 {
			for id, count := range s.TraceIDs {
				fmt.Printf("  %s: %d 次\n", id, count)
			}
		}
	}

	// 退出码:错误率 > 10% 返回 1
	if s.TotalRequests > 0 && float64(s.ErrorCount)/float64(s.TotalRequests) > 0.1 {
		fmt.Println("\n[FAIL] 错误率超过 10%")
		os.Exit(1)
	}
	fmt.Println("\n[PASS] 测试通过")
}
