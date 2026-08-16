# GoalOS — 面向人类目标的个人操作系统

[![Build & Release](https://img.shields.io/github/actions/workflow/status/dotnet010/GoalOS/docker-publish.yml)](https://github.com/dotnet010/GoalOS/actions/workflows/docker-publish.yml)
[![Release](https://img.shields.io/github/v/release/dotnet010/GoalOS)](https://github.com/dotnet010/GoalOS/releases)

> **用户只管提出目标，系统负责安全地实现它。**

GoalOS 不是聊天机器人，不是 Workflow 引擎，不是 Agent 框架。它是**面向人类目标的个人操作系统**——你只需要说出"我要什么"，系统理解、规划、执行、验证、交付。

[English](README.md)

---

## 愿景

你表述目标 → 系统理解目标 → 双方就"什么叫完成"达成共识（CompletionContract）→ 系统安全执行 → MultiLLM 验证 → 验证完成 → 交付结果。

任何不直接服务这一链路的功能都应被摒弃。

## v0.3.0 核心能力

| 能力 | 说明 |
|------|------|
| **CompletionContract** | 系统与你就"什么叫完成"建立契约。不再盲目执行 |
| **PipelineRunner** | 三原语管线：Check→Exec→Decide。Wait 为中间状态。状态从事件推导 |
| **Flow 模板** | 同一类目标每次按相同标准流程执行——结果可预测。flow 无匹配=确认流程唯一路径（禁止静默回退，R-1368） |
| **Multi-LLM 交叉验证** | 多 AI 模型（跨 Provider）独立审查产出。投票制裁决（R-844）。分歧时辩论轮次（R-860）。冷验证模式（R-858） |
| **ReviewReport + 用户决策** | 结构化审查报告——含每个 Provider 的独立意见。Dashboard 审查面板 + CLI `goalos review`。用户决定：带反馈重试 / 接受结果 / 修改需求 |
| **验证金字塔** | auto_tests → cross_model_review → behavioral_tests。确定性验证是最终裁决 |
| **新 Session 重做** | 执行失败时：新 Session 重试 1 次 → 人工介入。取消同 Session 循环重试（语义漂移防护） |
| **PlanHash 防篡改** | SHA256(MissionGraph) 在规划时计算，执行全链路验证（R-859） |
| **Provider 健康检查** | daemon 启动时检测所有 MultiLLM Provider。不可用的自动跳过（R-861） |
| **Zero Trust 安全** | Capability Token + seccomp 沙箱（Linux）+ FD3 IPC + HMAC。每个 Action 必经 Governance 五引擎审批。治理面 UDS-only——审批族永不暴露于 TCP（R-1378/R-1322） |
| **Persona 解耦** | 系统的"声音"由你控制。Core 产中性事件，Persona 决定怎么说 |
| **诚实反馈** | 系统不伪造成功。MultiLLM 审查是概率性判断——不是确定性验证。审查面板显式标注（R-865） |
| **WAL Truth 模型** | events.jsonl=唯一逻辑真理（append+fsync 提交点）。快照投影=可丢弃滞后副作用（R-1397）。CRC32+全局 seq+哈希链完整性（R-1393/R-1453） |
| **状态代数矩阵** | 四维状态矩阵（Goal×Action×Pipeline×Approval）单一权威。非法迁移拒绝+StateMachineViolation（R-1095/R-1407/R-1343） |
| **运行可观测** | Timeline 审计、运行时不变式、Prometheus metrics、SSE 进度推送 |

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

### 查看 MultiLLM 审查结果

```bash
# 查看目标的审查摘要
goalos review <goal_id>

# 查看完整审查报告（含每个 AI 模型的审查意见）
goalos review <goal_id> <action_id>

# 对 FAIL/WARN 结果做出决策
goalos review <goal_id> <action_id> --retry --feedback "修复认证问题"
goalos review <goal_id> <action_id> --accept --confirm
goalos review <goal_id> <action_id> --refine "改用 argon2 替代 bcrypt"
```

### 配置

首次启动时 GoalOS 会自动生成 `~/.goalos/config/daemon.yaml`（带注释）。直接编辑：

```yaml
daemon:
  port: 18920
  autonomy_level: approve   # observe|suggest|approve|autonomous

llm:
  provider: openai
  model: qwen3.6-flash
  api_key: "sk-..."         # API Key 直接填写，不需要环境变量
  base_url: https://your-llm-api.com/v1
  max_tokens: 65536

# 热加载配置（无需重启 daemon）
# curl -X POST http://localhost:18920/api/system/reload
```

### 多模型验证

配置多个 LLM Provider 进行并行代码审查。系统并行调用所有 Provider → VerdictCombiner 投票制裁决（R-844）。启动时自动检测 Provider 健康状态（R-861）。

```yaml
multi_llm:
  enabled: true
  debate_round: false       # R-860: 启用跨模型辩论（额外约 1.5× Token 成本）
  cold_review: false        # R-858: 验证者隔离 builder 上下文
  providers:
    - name: primary
      model: qwen3.6-flash
      api_key: "sk-..."
      base_url: https://your-llm-api.com/v1
      allowed_for: [L0,L1,L2,L3,L4,L5]
    - name: reviewer
      model: google/gemma-4-26b-a4b-it:free
      api_key: "your-openrouter-api-key"
      base_url: https://openrouter.ai/api/v1
      allowed_for: [L0,L1,L2]
```

### 交互通道

| 通道 | 场景 |
|------|------|
| **HTTP API** | 系统集成、脚本自动化 |
| **CLI** (`goalos`) | 终端用户、CI/CD |
| **Web UI** | `http://localhost:18920` — 目标仪表盘、Timeline、审查面板 |
| **Telegram Bot** | 移动端轻量交互 |

## 架构

```
用户目标 → Agent(Align→Analyze→Plan) → MissionGraph + PlanHash
         → Governance(五引擎审批) → PipelineRunner(Check→Exec→Decide)
         → Plugin Runner(seccomp沙箱, FD3 IPC) → 产出物(~/Goals/)
         → MultiLLM 验证(投票 + 辩论 + 冷验证)
         → ReviewReport → 用户决策(重试/接受/修改需求)
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
10. **Honest Feedback（诚实反馈）** — 系统真实反映状态。不伪造成功。不静默降低用户目标。用户拥有最终决策权

## API

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/goals` | 创建目标 |
| GET | `/api/goals/:id` | 查询目标状态（含 MultiLLM 裁决） |
| GET | `/api/goals` | 目标列表 |
| GET | `/api/goals/:id/reviews` | MultiLLM 审查摘要列表 |
| GET | `/api/goals/:id/reviews/:action_id` | 完整审查报告（含各 Provider 意见） |
| POST | `/api/goals/:id/reviews/:action_id/decide` | 提交用户决策（accept/retry/refine） |
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
go build -o goalos-cli ./cmd/goalos-cli/
```

## 文档

- [用户手册](用户手册.md)
- [架构会议纪要](../开发文档/会议纪要.md)

## License

GPL-3.0
