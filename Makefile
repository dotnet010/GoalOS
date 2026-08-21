.PHONY: build test lint race deadcode clean install-plugin release ci build-xinchuang

build:
	go build ./...

test:
	go test -count=1 -timeout 120s ./...

race:
	go test -count=1 -timeout 120s -race ./...

lint:
	go vet ./...

deadcode:
	@which staticcheck > /dev/null 2>&1 || (echo "install staticcheck: go install honnef.co/go/tools/cmd/staticcheck@v0.7.0  # 钉版本：v0.8.0 起要求 Go >= 1.26" && exit 1)
	staticcheck ./...

lint-full:
	@which golangci-lint > /dev/null 2>&1 || (echo "install golangci-lint: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" && exit 1)
	golangci-lint run ./...

all: lint race deadcode test build

# ci 目标：运行全部 CI 检查脚本（v0.2.0 audit fix）。
# 首个失败即退出。成功后运行全量 race 测试。
# 会议 #190 R-1052: check-resolution-propagation.sh 为第 7 硬闸口——
# 工作区布局下全量（层1+层2+幽灵+连续性）；路径自动检测，不依赖 CWD。
# 会议 #200 S'-24 R-1267: check-error-codes.sh 接线（error-codes-source.yaml
# 唯一维护侧——yaml↔09 §2.5 双向、yaml↔07 内联枚举比对）;
# check-doc-completeness.sh 裁决接线（三布局自适应, repo-only 显式降级）。
# 会议 #200 S'-25 R-1268: check-risk-table.sh 接线（risk-formula.yaml 单一
# 数据源重算比对 05 映射表）。
# 会议 #198 D22 R-1157: build-xinchuang 接入 make ci 交叉编译检查
# （linux/amd64+xinchuang 信创变体, -tags xinchuang 显式传参）。
ci: lint build
	@echo "=== Building plugins (releasecheck 前置——发布规范 #9 本地签名一致性) ==="
	@go build -o plugins/capability/shell-executor/plugin-shell ./cmd/plugin-shell
	@go build -o plugins/capability/websearch/plugin-websearch ./cmd/plugin-websearch
	@go run scripts/update_plugin_signatures.go
	@echo "=== Running CI check scripts ==="
	@bash scripts/check-anti-cheat.sh . || exit 1
	@bash scripts/check-naked-map.sh . || exit 1
	@bash scripts/check-error-swallow.sh . || exit 1
	@bash scripts/check-contract-test-assertion.sh . || exit 1
	@bash scripts/check-plugin-protocol.sh . || exit 1
	@bash scripts/check-dead-code.sh . || exit 1
	@bash scripts/check-resolution-propagation.sh || exit 1
	@bash scripts/check-doc-version.sh || exit 1
	@bash scripts/check-deprecated.sh || exit 1
	@bash scripts/check-doc-completeness.sh || exit 1
	@bash scripts/check-error-codes.sh || exit 1
	@bash scripts/check-risk-table.sh || exit 1
	@$(MAKE) build-xinchuang
	@echo "=== Running full race tests ==="
	@go test -count=1 -timeout 180s -race ./...
	@echo "=== CI ALL GREEN ==="

install-plugin:
	@mkdir -p ~/.goalos/plugins/capability/websearch
	CGO_ENABLED=0 go build -o ~/.goalos/plugins/capability/websearch/plugin-websearch ./cmd/plugin-websearch/
	cp plugins/capability/websearch/plugin.json ~/.goalos/plugins/capability/websearch/
	@echo "Plugin installed to ~/.goalos/plugins/capability/websearch/"

daemon:
	go build -o goalos-daemon ./cmd/goalos/

# build-xinchuang（R-1157 D22/S-45）: 信创交叉编译验收——PlatformID 变体
# 'linux/amd64+xinchuang'; build tag 由本目标显式传 -tags xinchuang,
# '//go:build linux && xinchuang' 源文件仅在信创构建目标下编译。
build-xinchuang:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags xinchuang -o goalos-daemon-xinchuang ./cmd/goalos/

release: test
	CGO_ENABLED=0 go build -o goalos-daemon ./cmd/goalos/
	CGO_ENABLED=0 go build -o goalos ./cmd/goalos-cli/

clean:
	rm -f goalos-daemon goalos goalos-cli plugin-websearch goalos-daemon-xinchuang
