.PHONY: build test lint race deadcode clean install-plugin release ci build-xinchuang build-plugins test-platform test-xinchuang

# EXE_EXT：Windows 下插件产物带 .exe 后缀（update_plugin_signatures.go 按平台
# 补 .exe 解析产物——裸名输出=工具找不到=签名跳闸空转，releasecheck 红）。
# 2026-08-31 实机实证：Windows 本地 make ci 假红根因。
#
# 可复现构建配方（R-1695——PM 裁定：非代码因素剥离，跨机确定性构建）：
#   产物构建点（ci 插件/release/install-plugin/build-xinchuang）统一钉
#   `CGO_ENABLED=0 go build -trimpath -buildvcs=false`。三因素各剥离一类非代码元数据：
#     ① -trimpath          剥离构建机绝对路径（源路径烙入二进制）
#     ② -buildvcs=false    剥离 VCS 元数据（vcs.revision/vcs.modified 随提交与脏树翻转
#                          ——R-1629 已实证的跑步机根因）
#     ③ CGO_ENABLED=0      剥离 C 工具链/libc 面（cgo 构建标签翻转→stdlib 选择集不同→
#                          同机不同默认值即不同产物；R-1695 实证：darwin/arm64 同源同旗标
#                          两档哈希 296bb4b5≠6d50aabc）
#   残余不可剥离因素=GOOS/GOARCH+Go 工具链版本（平台/版本内在差异）——故签名指纹
#   =平台作用域内可复现（同平台同工具链字节级一致），非跨平台单值承诺。
#   开发回环目标（build/test/race/daemon）不钉——不产指纹制品，保持 go 工具原生行为。
ifeq ($(OS),Windows_NT)
EXE_EXT := .exe
else
EXE_EXT :=
endif

build:
	go build ./...

test:
	go test -count=1 -timeout 120s ./...

race:
	go test -count=1 -timeout 120s -race ./...
	@echo "=== prototype 族（构建 tag 隔离——不进发布二进制；W7 T2 出数闸=R-1478③） ==="
	go test -count=1 -timeout 60s -tags prototype ./prototype/

lint:
	go vet ./...

# test-platform：平台专项特测车道（R-1695 ②——FD3 测试分层纳管）。
# 跑 `-tags platformtest` 的物理传输面测试：真实 socket bind、sun_path 预算、
# 目录权限、并发连接隔离（internal/fd3 的 F3/F6 + darwin 镜像决算族）。
# 刻意**不入 make ci / 常规车道**：本车道主体是「各开发机基底路径差异」——
# 轻量构建/常规 PR 不应被平台路径面误伤（PM 裁定原文）。触发面：
#   本地=本目标；CI=darwin-nightly（darwin）· windows-daily（windows）·
#   docker-publish test 作业（linux）。未跑=平台面零实证，不得冒充全绿。
test-platform:
	go test -count=1 -v -timeout 120s -tags platformtest ./internal/...

# test-xinchuang：信创构建标签车道（`-tags xinchuang` 的**测试面**——此前只有编译面）。
# 承载面澄清（勿混淆）：信创**生产**承载 = 运行期 bwrap 探测（internal/runtime，
# 与构建标签无关）；本车道验证的是 `-tags xinchuang` 在 internal/sandbox 编译面
# 选中的骨架族（backend_xinchuang.go + platform_backend_other_test.go）的测试行为。
#
# **linux-only**：darwin + xinchuang 是**编译错误**——platform_backend_other_test.go
# （`windows || xinchuang`）与 platform_backend_darwin_test.go（`darwin`）同时被选中
# → `platformBackend redeclared`。故本目标带 OS 硬门（非仅注释约定），且**刻意不入
# make ci**（本机 darwin 会直接红；理由同 test-platform：平台面差异不应误伤轻量车道）。
# windows 面不适用——windows-daily.yml 保持纯净、不接信创标签（PM 指令③）。
#
# **显式前置 build-plugins**（PM 指令②，严禁隐式前提）：车道跑 ./internal/... 全量，
# 含 releasecheck 的 plugin-signatures 闸——插件未构建则首红（红因与标签无关）。
# 触发面：本地=本目标；CI=docker-publish test 作业（runner=ubuntu，天然 linux）。
# 未跑=标签测试面零实证，不得冒充全绿。
#
# 实证锚（2026-09-10 goalos-test：Ubuntu 24.04 / kernel 6.8.0-139 / go1.25.14）：
#   TEST_EXIT=0；29 包全 ok；`ok internal/sandbox 0.002s`；--- FAIL 计数=0。
#   4 处 platformBackend 调用点全 PASS——TestSandbox_GVisorTier_DetectAndRoute /
#   TestSandbox_L5Disposable_HonestDeferral / TestSandbox_Spawn_AllFdsClosedExceptAllowlist /
#   TestSandbox_MinimalRuntimeExec（骨架期 Execute 必返非 nil error 的 fail-closed 契约）。
test-xinchuang: build-plugins
	@if [ "$$(go env GOOS)" != "linux" ]; then \
		echo "test-xinchuang: 仅限 linux（当前 GOOS=$$(go env GOOS)）——darwin+xinchuang 为编译错误（platformBackend 重声明），windows 面不适用"; \
		exit 1; \
	fi
	go test -p 1 -count=1 -v -timeout 300s -tags xinchuang ./internal/...

deadcode:
	@which staticcheck > /dev/null 2>&1 || (echo "install staticcheck: go install honnef.co/go/tools/cmd/staticcheck@v0.7.0  # 钉版本：v0.8.0 起要求 Go >= 1.26" && exit 1)
	staticcheck ./...

lint-full:
	@which golangci-lint > /dev/null 2>&1 || (echo "install golangci-lint: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2  # 钉版本：v2.13 起要求 Go >= 1.26；注意 v2 模块路径带 /v2" && exit 1)
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
# build-plugins：插件产物 + 签名刷新（发布规范 #9 本地签名一致性）。
# **显式前置目标**（2026-09-10 PM 指令②）：内部测试闸口 TestReleaseReadiness_All
# （internal/releasecheck 的 plugin-signatures 项）要求插件二进制已构建——缺则
# `[FAIL] plugin-signatures: binary not found`，且红因**与构建标签无关**。
# 事故实证（2026-09-10 goalos-test 实跑）：`go test -tags xinchuang ./internal/...`
# 在干净检出下 TEST_EXIT=1，首红即本项；补建插件后复跑 TEST_EXIT=0。
# 故凡跑全量 `./internal/...` 的车道**必须**显式依赖本目标——严禁隐式前提。
build-plugins:
	@echo "=== Building plugins (releasecheck 前置——发布规范 #9 本地签名一致性) ==="
	@CGO_ENABLED=0 go build -trimpath -buildvcs=false -o plugins/capability/shell-executor/plugin-shell$(EXE_EXT) ./cmd/plugin-shell
	@CGO_ENABLED=0 go build -trimpath -buildvcs=false -o plugins/capability/websearch/plugin-websearch$(EXE_EXT) ./cmd/plugin-websearch
	@go run scripts/update_plugin_signatures.go

ci: lint build
	@$(MAKE) build-plugins
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
	@bash scripts/check-sensitive-path-write.sh || exit 1
	@$(MAKE) build-xinchuang
	@echo "=== Running full race tests ==="
	@go test -count=1 -timeout 180s -race ./...
	@echo "=== CI ALL GREEN ==="

install-plugin:
	@mkdir -p ~/.goalos/plugins/capability/websearch
	CGO_ENABLED=0 go build -trimpath -buildvcs=false -o ~/.goalos/plugins/capability/websearch/plugin-websearch ./cmd/plugin-websearch/
	cp plugins/capability/websearch/plugin.json ~/.goalos/plugins/capability/websearch/
	@echo "Plugin installed to ~/.goalos/plugins/capability/websearch/"

daemon:
	go build -o goalos-daemon ./cmd/goalos/

# build-xinchuang（R-1157 D22/S-45）: 信创交叉编译验收——PlatformID 变体
# 'linux/amd64+xinchuang'; build tag 由本目标显式传 -tags xinchuang,
# '//go:build linux && xinchuang' 源文件仅在信创构建目标下编译。
build-xinchuang:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags xinchuang -trimpath -buildvcs=false -o goalos-daemon-xinchuang ./cmd/goalos/

release: test
	CGO_ENABLED=0 go build -trimpath -buildvcs=false -o goalos-daemon ./cmd/goalos/
	CGO_ENABLED=0 go build -trimpath -buildvcs=false -o goalos ./cmd/goalos-cli/

clean:
	rm -f goalos-daemon goalos goalos-cli plugin-websearch goalos-daemon-xinchuang
