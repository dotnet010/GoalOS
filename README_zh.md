# GoalOS — 面向人类目标的个人操作系统

[![Build & Release](https://img.shields.io/github/actions/workflow/status/dotnet010/GoalOS/docker-publish.yml)](https://github.com/dotnet010/GoalOS/actions/workflows/docker-publish.yml)

> **用户只管提出目标，系统负责安全地实现它。**

GoalOS 不是聊天机器人，不是 Workflow 引擎，不是 Agent 框架。它是**面向人类目标的个人操作系统**——你只需要说出"我要什么"，系统理解、规划、执行、验证、交付。

---

## 愿景

你表述目标 → 系统理解目标 → 双方就"什么叫完成"达成共识 → 系统安全执行 → 验证完成 → 交付结果。

任何不直接服务这一链路的功能都应被摒弃。

## v0.1.0 核心能力

| 能力 | 说明 |
|------|------|
| **CompletionContract** | 系统与你就"什么叫完成"建立契约。不再盲目执行 |
| **Primitive 执行引擎** | Check→Exec→Wait→Decide 四原语管线。状态从事件推导 |
| **Flow 模板** | 同一类目标每次按相同标准流程执行——结果可预测 |
| **Multi-LLM 交叉验证** | 多个 AI 模型独立审查产出。验证金字塔：auto→cross-model→behavioral |
| **自修正** | 执行失败时自动分析根因、修正、重试（最多 3 次） |
| **Zero Trust 安全** | Capability Token + seccomp 沙箱 + IPC HMAC。每个 Action 必经 Governance |
| **Persona 解耦** | 系统的"声音"由你控制。Core 产中性事件，Persona 决定怎么说 |
| **运行可观测** | Timeline 审计、运行时不变式、Prometheus metrics |

## 快速开始

### 安装

从 [Releases](https://github.com/dotnet010/GoalOS/releases) 下载对应平台的二进制文件。

```bash
# macOS/Linux
tar xzf goalos-<os>-<arch>.tar.gz
./goalos-daemon &
```

### 第一个目标

```bash
# 通过 CLI
goalos "设计一个3D魔方。使用HTML/CSS创建可以旋转的3D魔方。"

# 或通过 HTTP API
curl -X POST http://localhost:18920/api/goals \
  -H "Content-Type: application/json" \
  -d '{"goal":"创建一个 CRM 系统"}'
```

### 配置

首次启动时 GoalOS 会自动生成 `~/.goalos/config/daemon.yaml`（带注释）。直接编辑：

```yaml
daemon:
  port: 18920
  autonomy_level: autonomous   # observe|suggest|approve|autonomous

llm:
  provider: openai
  model: glm-5.1
  api_key: "sk-..."            # API Key 直接填写，不需要环境变量
  base_url: https://your-llm-api.com/v1
  max_tokens: 4096

# 热加载配置（无需重启 daemon）
# curl -X POST http://localhost:18920/api/system/reload
```

### 多模型验证

配置多个 LLM Provider 进行并行代码审查：

```yaml
multi_llm:
  enabled: true
  providers:
    - name: glm
      model: glm-5.1
      api_key: "sk-..."
      base_url: https://ws-hwiv1ueutcxpjuzq.cn-beijing.maas.aliyuncs.com/compatible-mode/v1
      allowed_for: [L0,L1,L2,L3,L4,L5]
    - name: openrouter
      model: nvidia/nemotron-3-ultra-550b-a55b:free
      api_key: "your-openrouter-api-key"
      base_url: https://openrouter.ai/api/v1
      allowed_for: [L0,L1,L2]
```

系统并行调用所有 Provider → VerdictCombiner 合并裁决。分歧会展示给用户。

### 交互通道

| 通道 | 场景 |
|------|------|
| **HTTP API** | 系统集成、脚本自动化 |
| **CLI** (`goalos`) | 终端用户、CI/CD |
| **Web UI** | `http://localhost:18920` — 目标仪表盘、Timeline |
| **Telegram Bot** | 移动端轻量交互 |

## 架构

```
用户目标 → Agent(Align→Analyze→Plan) → MissionGraph
         → Governance(五引擎审批) → PipelineRunner(Check→Exec→Wait→Decide)
         → Plugin Runner(seccomp沙箱) → 产出物(~/Goals/)
         → Verifier(验证金字塔) → CompletionContract 验收 → 结果交付
```

### 核心原则

1. **Plugin over Build** — 可变能力皆 Plugin。核心不变
2. **Event over Call** — 模块通过事件通信。可审计可回放
3. **File over Database** — 数据是文件。用户拥有。零外部存储依赖
4. **Projection over State** — 状态从事件推导。缓存可重建
5. **Delegate over Build** — 已有平台的不重复做
6. **One over Many** — 数据证明需要多个前只做一个
7. **Interface over Implementation** — 定义"做什么"。实现可替换
8. **User-Owned over System-Managed** — 用户拥有数据
9. **Persona Decoupled** — Core 产中性事件。Persona 是可替换渲染层

## API

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/goals` | 创建目标 |
| GET | `/api/goals/:id` | 查询目标状态 |
| GET | `/api/goals` | 目标列表 |
| POST | `/api/goals/:id/pause` | 暂停 |
| POST | `/api/goals/:id/resume` | 恢复 |
| POST | `/api/goals/:id/stop` | 终止 |
| GET | `/api/health` | 健康检查 |
| POST | `/api/system/reload` | 热加载配置 |

## 从源码构建

```bash
git clone https://github.com/dotnet010/GoalOS.git
cd GoalOS
go build -o goalos-daemon ./cmd/goalos/
go build -o goalos ./cmd/goalos-cli/
```

## 文档

- [用户手册](用户手册.md)
- [开发计划 v0.1.0](开发计划v0.1.0.md)

## License

GPL-3.0
