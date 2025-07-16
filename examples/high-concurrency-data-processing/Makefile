# RxGo 高并发数据处理示例 Makefile

.PHONY: all run build clean test benchmark profile help

# 默认目标
all: run

# 运行所有示例
run:
	@echo "🚀 运行RxGo高并发数据处理示例..."
	go run .

# 构建可执行文件
build:
	@echo "🔨 构建示例程序..."
	go build -o rxgo-examples .

# 运行单个示例
run-pipeline:
	@echo "📊 运行数据处理管道示例..."
	go run main.go data_pipeline.go

run-batch:
	@echo "📦 运行批处理示例..."
	go run main.go batch_processor.go

run-crawler:
	@echo "🕷️ 运行网络爬虫示例..."
	go run main.go network_crawler.go

run-analytics:
	@echo "📈 运行日志分析示例..."
	go run main.go remaining_examples.go

run-stream:
	@echo "🌊 运行流处理示例..."
	go run main.go remaining_examples.go

run-mapreduce:
	@echo "🗺️ 运行MapReduce示例..."
	go run main.go remaining_examples.go

# 性能测试
benchmark:
	@echo "⚡ 运行性能基准测试..."
	go test -bench=. -benchmem ./...

# 性能分析
profile:
	@echo "🔍 运行性能分析..."
	go run . -cpuprofile=cpu.prof -memprofile=mem.prof
	@echo "使用以下命令查看分析结果:"
	@echo "go tool pprof cpu.prof"
	@echo "go tool pprof mem.prof"

# 测试
test:
	@echo "🧪 运行测试..."
	go test ./...

# 代码格式化
fmt:
	@echo "🎨 格式化代码..."
	go fmt ./...

# 代码检查
lint:
	@echo "🔎 运行代码检查..."
	golangci-lint run

# 整理依赖
mod-tidy:
	@echo "📦 整理Go模块依赖..."
	go mod tidy

# 清理
clean:
	@echo "🧹 清理构建文件..."
	rm -f rxgo-examples
	rm -f *.prof
	rm -f *.pprof

# 查看帮助
help:
	@echo "RxGo 高并发数据处理示例 - 可用命令:"
	@echo ""
	@echo "  make run          - 运行所有示例"
	@echo "  make build        - 构建可执行文件"
	@echo "  make clean        - 清理构建文件"
	@echo ""
	@echo "单个示例:"
	@echo "  make run-pipeline    - 数据处理管道"
	@echo "  make run-batch       - 批处理"
	@echo "  make run-crawler     - 网络爬虫"
	@echo "  make run-analytics   - 日志分析"
	@echo "  make run-stream      - 流处理"
	@echo "  make run-mapreduce   - MapReduce"
	@echo ""
	@echo "开发工具:"
	@echo "  make test         - 运行测试"
	@echo "  make benchmark    - 性能基准测试"
	@echo "  make profile      - 性能分析"
	@echo "  make fmt          - 格式化代码"
	@echo "  make lint         - 代码检查"
	@echo "  make mod-tidy     - 整理依赖"
	@echo ""

# 显示系统信息
info:
	@echo "系统信息:"
	@echo "Go版本: $(shell go version)"
	@echo "CPU核心数: $(shell go run -c 'package main; import ("fmt"; "runtime"); func main() { fmt.Println(runtime.NumCPU()) }')"
	@echo "操作系统: $(shell go env GOOS)"
	@echo "架构: $(shell go env GOARCH)" 