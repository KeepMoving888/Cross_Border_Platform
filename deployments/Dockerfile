# 多阶段构建 - 支持多平台(linux/amd64, linux/arm64)
# 通过 BUILDKIT_ARG 可覆盖基础镜像以使用镜像加速器
ARG BUILDER_IMAGE=golang:1.22-alpine
ARG RUNTIME_IMAGE=alpine:3.19

FROM --platform=$BUILDPLATFORM ${BUILDER_IMAGE} AS builder

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=dev
ARG BUILD_COMMIT=unknown
ARG BUILD_DATE=unknown

ENV GOPROXY=https://goproxy.cn,direct
ENV CGO_ENABLED=0
ENV GOOS=${TARGETOS}
ENV GOARCH=${TARGETARCH}

WORKDIR /build

# 利用缓存:先拷贝依赖文件(go.sum 可能不存在,由 go mod download 自动生成)
COPY go.mod ./
COPY go.sum* ./
RUN go mod download

# 拷贝源码
COPY . .

# 整理依赖(生成 go.sum 缓存,避免依赖变更)
RUN go mod tidy

# 编译,注入版本信息
RUN go build -trimpath \
    -ldflags "-s -w \
      -X main.Version=${VERSION} \
      -X main.BuildCommit=${BUILD_COMMIT} \
      -X main.BuildDate=${BUILD_DATE}" \
    -o /build/cb-platform ./cmd/server

# ============== 运行阶段 ==============
FROM --platform=$TARGETPLATFORM ${RUNTIME_IMAGE}

LABEL org.opencontainers.image.title="cb-platform" \
      org.opencontainers.image.description="跨境电商智能运营中台" \
      org.opencontainers.image.source="https://github.com/cb-platform/cb-platform" \
      org.opencontainers.image.licenses="MIT"

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories && \
    apk --no-cache add ca-certificates tzdata curl && \
    cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && \
    echo "Asia/Shanghai" > /etc/timezone && \
    addgroup -S app && adduser -S app -G app

WORKDIR /app

COPY --from=builder /build/cb-platform /app/cb-platform
COPY --from=builder /build/.env.example /app/.env.example
COPY --from=builder /build/migrations /app/migrations

RUN chown -R app:app /app
USER app

# 微服务部署:单一镜像通过 APP_ROLE 环境变量适配不同角色(gateway/ai/rag)
# APP_PORT 控制监听端口(gateway=8080, ai=8081, rag=8082),由 docker-compose 注入
EXPOSE 8080 8081 8082

# HEALTHCHECK 使用 shell 形式以支持 APP_PORT 变量展开
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -f "http://localhost:${APP_PORT:-8080}/health" || exit 1

ENTRYPOINT ["/app/cb-platform"]
