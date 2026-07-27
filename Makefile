.PHONY: all build run test lint clean docker-up docker-down migrate seed fmt tidy coverage bench release version help

APP_NAME=cb-platform
BUILD_DIR=bin
MAIN_PKG=cmd/server
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_COMMIT?=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE?=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS=-s -w -X main.Version=$(VERSION) -X main.BuildCommit=$(BUILD_COMMIT) -X main.BuildDate=$(BUILD_DATE)

all: build

## build: 编译二进制(当前平台)
build:
	@echo "==> Building $(APP_NAME) $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME) ./$(MAIN_PKG)

## build-all: 跨平台编译
build-all:
	@echo "==> Building all platforms..."
	@mkdir -p $(BUILD_DIR)
	@for os in linux darwin; do \
		for arch in amd64 arm64; do \
			echo "  -> $$os/$$arch"; \
			GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" \
				-o $(BUILD_DIR)/$(APP_NAME)-$$os-$$arch ./$(MAIN_PKG); \
		done; \
	done

## run: 本地运行
run:
	@echo "==> Running $(APP_NAME)..."
	go run ./$(MAIN_PKG)

## version: 显示版本信息
version:
	@echo "Version:    $(VERSION)"
	@echo "Commit:     $(BUILD_COMMIT)"
	@echo "Build Date: $(BUILD_DATE)"

## test: 运行测试
test:
	@echo "==> Running tests..."
	go test -v -race -cover -timeout=10m ./... -coverprofile=coverage.txt
	@go tool cover -func=coverage.txt | tail -1

## test-short: 只运行短测试
test-short:
	@echo "==> Running short tests..."
	go test -short -race -cover ./...

## bench: 基准测试
bench:
	@echo "==> Running benchmarks..."
	go test -bench=. -benchmem -run=^$$ ./...

## lint: 代码检查
lint:
	@echo "==> Linting..."
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run --timeout=5m; \
	else \
		echo "golangci-lint not installed, using go vet"; \
		go vet ./...; \
	fi
	@echo "==> Checking formatting..."
	@if [ -n "$$(gofmt -l .)" ]; then \
		echo "以下文件需要格式化:"; \
		gofmt -l .; \
		exit 1; \
	fi

## fmt: 格式化代码
fmt:
	@echo "==> Formatting..."
	gofmt -s -w .
	@if command -v goimports > /dev/null; then \
		goimports -w .; \
	fi

## tidy: 整理依赖
tidy:
	@echo "==> Tidying modules..."
	go mod tidy

## coverage: 生成覆盖率报告(HTML)
coverage: test
	@echo "==> Generating HTML coverage report..."
	go tool cover -html=coverage.txt -o coverage.html
	@echo "Coverage report: coverage.html"

## clean: 清理构建产物
clean:
	@echo "==> Cleaning..."
	rm -rf $(BUILD_DIR) coverage.txt coverage.html

## docker-up: 启动基础设施(MySQL/Redis/PG/Prometheus/Grafana)
docker-up:
	@echo "==> Starting infrastructure..."
	docker compose -f deployments/docker-compose.yml up -d

## docker-down: 停止基础设施
docker-down:
	@echo "==> Stopping infrastructure..."
	docker compose -f deployments/docker-compose.yml down

## docker-build: 构建 Docker 镜像
docker-build:
	@echo "==> Building Docker image..."
	docker build -t $(APP_NAME):$(VERSION) -f deployments/Dockerfile .

## migrate: 数据库迁移
migrate:
	@echo "==> Running migrations..."
	go run ./$(MAIN_PKG) -migrate

## seed: 种子数据
seed:
	@echo "==> Seeding data..."
	go run ./$(MAIN_PKG) -seed

## release: 创建发布(需要 TAG 参数,如 make release TAG=v1.0.0)
release:
	@if [ -z "$(TAG)" ]; then echo "Usage: make release TAG=v1.0.0"; exit 1; fi
	@echo "==> Creating release $(TAG)..."
	@git tag -a $(TAG) -m "Release $(TAG)"
	@git push origin $(TAG)

## help: 显示帮助
help:
	@echo "CB-Platform 跨境电商智能运营中台"
	@echo ""
	@echo "可用命令:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //; s/:/ /' | awk 'NF {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'
