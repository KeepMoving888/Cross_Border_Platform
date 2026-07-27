// 负载测试工具 - 用于评估 CB-Platform API 性能
//
// 使用方式:
//
//	go run ./scripts/loadtest -target=http://localhost:8080 -duration=60s -rate=100
//
// 指标:
//   - QPS(每秒请求数)
//   - P50/P95/P99 响应时间
//   - 错误率
//   - 状态码分布
package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	"net/http"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type Result struct {
	StatusCode int
	Duration   time.Duration
	Err        error
}

type Stats struct {
	Total      int64
	Success    int64
	Failed     int64
	StatusDist map[int]int64
	Durations  []time.Duration
}

func main() {
	target := flag.String("target", "http://localhost:8080", "目标地址")
	duration := flag.Duration("duration", 30*time.Second, "测试时长")
	rate := flag.Int("rate", 50, "每秒请求数(QPS)")
	concurrency := flag.Int("concurrency", 10, "并发数")
	path := flag.String("path", "/health", "测试路径")
	flag.Parse()

	fmt.Printf("=== CB-Platform 负载测试 ===\n")
	fmt.Printf("目标: %s%s\n", *target, *path)
	fmt.Printf("时长: %s\n", *duration)
	fmt.Printf("目标 QPS: %d\n", *rate)
	fmt.Printf("并发数: %d\n", *concurrency)
	fmt.Printf("开始时间: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))

	url := *target + *path
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        *concurrency * 2,
			MaxIdleConnsPerHost: *concurrency * 2,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	stats := &Stats{
		StatusDist: make(map[int]int64),
		Durations:  make([]time.Duration, 0, *rate*int(*duration/time.Second)),
	}
	var mu sync.Mutex

	// 用令牌桶控制速率
	interval := time.Second / time.Duration(*rate)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// 启动 worker
	var wg sync.WaitGroup
	workerCh := make(chan struct{}, *concurrency)
	for i := 0; i < *concurrency; i++ {
		workerCh <- struct{}{}
	}

	startTime := time.Now()
	requestsSent := int64(0)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				select {
				case <-workerCh:
					wg.Add(1)
					go func() {
						defer wg.Done()
						defer func() { workerCh <- struct{}{} }()
						result := doRequest(ctx, client, url)
						atomic.AddInt64(&stats.Total, 1)
						mu.Lock()
						stats.StatusDist[result.StatusCode]++
						stats.Durations = append(stats.Durations, result.Duration)
						if result.Err != nil || result.StatusCode >= 500 {
							atomic.AddInt64(&stats.Failed, 1)
						} else {
							atomic.AddInt64(&stats.Success, 1)
						}
						mu.Unlock()
						atomic.AddInt64(&requestsSent, 1)
					}()
				default:
					// worker 池满,丢弃
				}
			}
		}
	}()

	<-ctx.Done()
	wg.Wait()
	actualDuration := time.Since(startTime)

	// 计算统计
	fmt.Printf("\n=== 测试结果 ===\n")
	fmt.Printf("实际时长: %s\n", actualDuration.Round(time.Millisecond))
	fmt.Printf("总请求数: %d\n", stats.Total)
	fmt.Printf("成功: %d\n", stats.Success)
	fmt.Printf("失败: %d\n", stats.Failed)

	if stats.Total > 0 {
		fmt.Printf("成功率: %.2f%%\n", float64(stats.Success)/float64(stats.Total)*100)
		fmt.Printf("实际 QPS: %.2f\n", float64(stats.Total)/actualDuration.Seconds())
	}

	fmt.Printf("\n=== 状态码分布 ===\n")
	codes := make([]int, 0, len(stats.StatusDist))
	for c := range stats.StatusDist {
		codes = append(codes, c)
	}
	sort.Ints(codes)
	for _, c := range codes {
		fmt.Printf("  %d: %d (%.1f%%)\n", c, stats.StatusDist[c], float64(stats.StatusDist[c])/float64(stats.Total)*100)
	}

	if len(stats.Durations) > 0 {
		sort.Slice(stats.Durations, func(i, j int) bool {
			return stats.Durations[i] < stats.Durations[j]
		})
		fmt.Printf("\n=== 响应时间分布 ===\n")
		fmt.Printf("  最小: %s\n", stats.Durations[0].Round(time.Microsecond))
		fmt.Printf("  P50:  %s\n", stats.Durations[len(stats.Durations)*50/100].Round(time.Microsecond))
		fmt.Printf("  P90:  %s\n", stats.Durations[len(stats.Durations)*90/100].Round(time.Microsecond))
		fmt.Printf("  P95:  %s\n", stats.Durations[int(math.Min(float64(len(stats.Durations)-1), float64(len(stats.Durations))*0.95))].Round(time.Microsecond))
		fmt.Printf("  P99:  %s\n", stats.Durations[int(math.Min(float64(len(stats.Durations)-1), float64(len(stats.Durations))*0.99))].Round(time.Microsecond))
		fmt.Printf("  最大: %s\n", stats.Durations[len(stats.Durations)-1].Round(time.Microsecond))

		var sum time.Duration
		for _, d := range stats.Durations {
			sum += d
		}
		fmt.Printf("  平均: %s\n", (sum / time.Duration(len(stats.Durations))).Round(time.Microsecond))
	}

	fmt.Printf("\n结束时间: %s\n", time.Now().Format("2006-01-02 15:04:05"))

	if stats.Failed > 0 {
		os.Exit(1)
	}
}

func doRequest(ctx context.Context, client *http.Client, url string) Result {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return Result{Duration: time.Since(start), Err: err}
	}
	resp, err := client.Do(req)
	if err != nil {
		return Result{Duration: time.Since(start), Err: err}
	}
	defer resp.Body.Close()
	return Result{
		StatusCode: resp.StatusCode,
		Duration:   time.Since(start),
	}
}
