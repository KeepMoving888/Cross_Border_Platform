package tracing

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/cb-platform/internal/pkg/logger"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

// 全局 TracerProvider 关闭函数(供 main.go defer 调用)
var (
	shutdownOnce sync.Once
	shutdownFunc func(context.Context) error
)

// Init 初始化 OpenTelemetry TracerProvider
//
// 参数:
//   - endpoint: OTLP Collector 地址(如 "jaeger:4318" 或 "localhost:4318")
//   - serviceName: 服务名(如 "cb-gateway" / "cb-ai-svc" / "cb-rag-svc")
//   - samplingRatio: 采样率(0.0~1.0,生产环境建议 0.1,开发环境 1.0)
//
// endpoint 为空时不初始化 OTel,回退到现有 X-Trace-Id 机制
func Init(endpoint, serviceName string, samplingRatio float64) (func(context.Context) error, error) {
	if endpoint == "" {
		logger.Get().Info("otel: endpoint not configured, tracing disabled (using X-Trace-Id fallback)")
		return nil, nil
	}

	// 创建 OTLP HTTP Exporter(发送到 Jaeger/Tempo 的 OTLP 端点)
	exporter, err := otlptracehttp.New(context.Background(),
		otlptracehttp.WithEndpoint(endpoint),
		otlptracehttp.WithInsecure(), // 内部网络不启用 TLS
	)
	if err != nil {
		return nil, fmt.Errorf("create otlp exporter: %w", err)
	}

	// 创建 Resource(标识服务来源)
	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion("1.0.0"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	// 创建 TracerProvider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter,
			sdktrace.WithBatchTimeout(5*time.Second), // 批量发送间隔
		),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(samplingRatio)),
	)

	// 设置全局 TracerProvider 和 TextMap 传播器
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, // W3C Trace Context
		propagation.Baggage{},     // W3C Baggage
	))

	logger.Get().Infof("otel: tracer initialized, service=%s, endpoint=%s, sampling=%.2f",
		serviceName, endpoint, samplingRatio)

	shutdownFunc = tp.Shutdown
	return tp.Shutdown, nil
}

// Shutdown 优雅关闭 TracerProvider(刷新未发送的 span)
func Shutdown(ctx context.Context) {
	shutdownOnce.Do(func() {
		if shutdownFunc != nil {
			if err := shutdownFunc(ctx); err != nil {
				logger.Get().Warnf("otel: shutdown failed: %v", err)
			} else {
				logger.Get().Info("otel: tracer shutdown completed")
			}
		}
	})
}

// Tracer 返回全局 Tracer(用于创建自定义 span)
func Tracer(name string) trace.Tracer {
	return otel.Tracer(name)
}
