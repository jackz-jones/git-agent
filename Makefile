BUILD_TIME := $(shell date "+%F %T")
COMMIT_SHA1 := $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")

# 版本手动指定
VERSION=v0.1.0
LDFLAGS := -ldflags "-X 'github.com/jackz-jones/git-agent/internal.BuildTime=$(BUILD_TIME)' -X 'github.com/jackz-jones/git-agent/internal.CommitID=$(COMMIT_SHA1)' -X 'github.com/jackz-jones/git-agent/internal.Version=$(VERSION)'"
SOURCE := ./main.go
BUILD_NAME := git-agent

.PHONY: build run clean version lint test help

# 编译构建
build:
	go build $(LDFLAGS) -o $(BUILD_NAME) $(SOURCE)

# 编译并运行
run: build
	./$(BUILD_NAME)

# 直接运行（开发模式，不注入版本信息）
dev:
	go run $(SOURCE)

# 清理编译产物
clean:
	rm -f $(BUILD_NAME)
	rm -f cover.out

# 显示版本信息（需要先 build）
version: build
	@./$(BUILD_NAME) --version

# 运行测试
test:
	go test -v ./...

# 运行测试并生成覆盖率报告
test-cover:
	go test -coverprofile cover.out ./...
	@echo ""
	@echo "UT覆盖率：" `go tool cover -func=cover.out | tail -1 | grep -P '\d+\.\d+(?=\%)' -o`
	@echo ""

# 代码检查
lint:
	golangci-lint run ./...

# 整理依赖
tidy:
	go mod tidy

# 安装到 GOPATH/bin
install: build
	cp $(BUILD_NAME) $(GOPATH)/bin/ 2>/dev/null || cp $(BUILD_NAME) $(HOME)/go/bin/ 2>/dev/null || echo "请手动将 $(BUILD_NAME) 复制到 PATH 中"

# 帮助信息
help:
	@echo "Git Agent Makefile 使用说明"
	@echo ""
	@echo "  make build        编译项目（注入版本信息）"
	@echo "  make run          编译并运行"
	@echo "  make dev          开发模式直接运行（不注入版本信息）"
	@echo "  make clean        清理编译产物"
	@echo "  make version      查看版本信息"
	@echo "  make test         运行测试"
	@echo "  make test-cover   运行测试并生成覆盖率报告"
	@echo "  make lint         代码检查"
	@echo "  make tidy         整理依赖"
	@echo "  make install      安装到 GOPATH/bin"
	@echo ""
	@echo "修改版本号：编辑 Makefile 顶部的 VERSION 变量"
