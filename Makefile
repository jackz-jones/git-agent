BUILD_TIME := $(shell date "+%F %T")
COMMIT_SHA1 := $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")

# 版本手动指定
VERSION=v1.0.0
LDFLAGS := -ldflags "-X 'github.com/jackz-jones/git-agent/internal.BuildTime=$(BUILD_TIME)' -X 'github.com/jackz-jones/git-agent/internal.CommitID=$(COMMIT_SHA1)' -X 'github.com/jackz-jones/git-agent/internal.Version=$(VERSION)'"
SOURCE := .
BUILD_NAME := git-agent

.PHONY: build build-web build-go-only run clean version lint test help serve dev dev-web dev-server dev-all

# 编译构建：先构建前端，再编译 Go 二进制（嵌入 web/dist）
build: build-web
	go build $(LDFLAGS) -o $(BUILD_NAME) $(SOURCE)

# 只构建前端到 internal/web/dist（供 go embed 使用）
# 若本机没有 Node，可直接 `make build-go-only` 跳过，使用 git 历史中已 checked-in 的产物（若有）
build-web:
	cd web && npm install && npm run build

# 跳过前端构建，直接编译 Go 二进制；适合 CI 中分离前后端构建的场景
build-go-only:
	go build $(LDFLAGS) -o $(BUILD_NAME) $(SOURCE)

# 启动 Web 服务（先构建再运行 serve）
serve: build
	./$(BUILD_NAME) serve

# 编译并运行（CLI 模式）
run: build
	./$(BUILD_NAME)

# 直接运行（开发模式，不注入版本信息）
dev:
	go run $(SOURCE)

# 前端热加载开发服务器（端口 5173，自动代理 /api 到后端 8088）
dev-web:
	cd web && npm run dev

# 后端开发服务器（端口 8088，不打开浏览器，配合 dev-web 使用）
# 优先使用 air 实现热加载；未安装 air 时回退到普通 go run
dev-server:
	@if command -v air >/dev/null 2>&1; then \
		echo "🔥 使用 air 启动后端（Go 代码修改自动重启）..."; \
		air; \
	else \
		echo "⚠  未检测到 air，使用普通 go run（修改后需手动重启）"; \
		echo "   安装 air: go install github.com/air-verse/air@latest"; \
		go run $(SOURCE) serve --open=false; \
	fi

# 同时启动前后端开发服务器（前端热加载 + 后端热加载）
# 访问 http://localhost:5173 即可实时预览前后端改动
dev-all:
	@echo ""
	@echo "  🔥 前端热加载地址: http://localhost:5173"
	@echo "  🔌 后端 API 地址:  http://localhost:8088"
	@echo ""
	@if command -v air >/dev/null 2>&1; then \
		echo "启动后端 API 服务 (air 热加载) ..."; \
		air & \
	else \
		echo "启动后端 API 服务 (无热加载，安装 air 可获得热加载: go install github.com/air-verse/air@latest) ..."; \
		go run $(SOURCE) serve --open=false & \
	fi
	@sleep 1
	@echo "启动前端热加载服务 (5173) ..."
	@cd web && npm run dev

# 清理编译产物
clean:
	rm -f $(BUILD_NAME)
	rm -f cover.out

# 显示版本信息（只需 Go 编译，不依赖前端）
version: build-go-only
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
	@echo "  make build        编译项目（先构建前端再 Go 编译，注入版本信息）"
	@echo "  make build-web    只构建前端到 internal/web/dist（供 go embed）"
	@echo "  make build-go-only 跳过前端，只编译 Go 二进制"
	@echo "  make serve        构建并启动 Web 服务（默认 127.0.0.1:8088）"
	@echo "  make run          编译并运行（CLI 模式）"
	@echo "  make dev          开发模式直接运行（不注入版本信息）"
	@echo "  make dev-web      启动前端热加载服务（端口 5173）"
	@echo "  make dev-server   启动后端 API 服务（端口 8088，支持 air 热加载）"
	@echo "  make dev-all      同时启动前后端（推荐开发方式，前后端改动实时生效）"
	@echo "  make clean        清理编译产物"
	@echo "  make version      查看版本信息"
	@echo "  make test         运行测试"
	@echo "  make test-cover   运行测试并生成覆盖率报告"
	@echo "  make lint         代码检查"
	@echo "  make tidy         整理依赖"
	@echo "  make install      安装到 GOPATH/bin"
	@echo ""
	@echo "开发推荐："
	@echo "  1. 终端 A: make dev-server   (启动后端 API，支持热加载)"
	@echo "  2. 终端 B: make dev-web      (启动前端热加载)"
	@echo "  3. 浏览器访问 http://localhost:5173"
	@echo "  或直接: make dev-all (一键启动前后端，均支持热加载)"
	@echo ""
	@echo "后端热加载需要安装 air: go install github.com/air-verse/air@latest"
	@echo ""
	@echo "修改版本号：编辑 Makefile 顶部的 VERSION 变量"
