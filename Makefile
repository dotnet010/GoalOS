.PHONY: build test lint race deadcode clean install-plugin release ci

build:
	go build ./...

test:
	go test -count=1 -timeout 120s ./...

race:
	go test -count=1 -timeout 120s -race ./...

lint:
	go vet ./...

deadcode:
	@which staticcheck > /dev/null 2>&1 && staticcheck ./... || echo "install staticcheck: go install honnef.co/go/tools/cmd/staticcheck@latest"

all: lint race deadcode test build

# ci 目标：运行全部 CI 检查脚本（v0.2.0 audit fix）。
# 首个失败即退出。成功后运行全量 race 测试。
# 会议 #190 R-1052: check-resolution-propagation.sh 为第 7 硬闸口——
# 工作区布局下全量（层1+层2+幽灵+连续性）；路径自动检测，不依赖 CWD。
ci: lint build
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

release: test
	CGO_ENABLED=0 go build -o goalos-daemon ./cmd/goalos/
	CGO_ENABLED=0 go build -o goalos ./cmd/goalos-cli/

clean:
	rm -f goalos-daemon goalos goalos-cli plugin-websearch
