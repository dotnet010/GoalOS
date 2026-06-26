# GoalOS — A Personal Goal Execution Operating System

[![Build & Release](https://img.shields.io/github/actions/workflow/status/dotnet010/GoalOS/docker-publish.yml)](https://github.com/dotnet010/GoalOS/actions/workflows/docker-publish.yml)
[![Release](https://img.shields.io/github/v/release/dotnet010/GoalOS)](https://github.com/dotnet010/GoalOS/releases)

> **You state the goal. The system delivers it — safely.**

GoalOS is not a chatbot, not a workflow engine, not an agent framework. It is a **personal operating system for goal execution**. You say what you want — the system understands, plans, executes, verifies, and delivers.

[中文文档](README_zh.md)

---

## Vision

You state your goal → The system understands → You agree on "what done means" → The system executes safely → Verified → Delivered.

Any feature that does not directly serve this chain should be removed.

## v0.1.0 Core Capabilities

| Capability | Description |
|------------|-------------|
| **CompletionContract** | System establishes a contract with you on "what done means" before execution begins |
| **Primitive Execution Engine** | Check→Exec→Wait→Decide pipeline. State derived from events (Projection over State) |
| **Flow Templates** | Same goal type follows the same standard process — predictable results |
| **Multi-LLM Verification** | Multiple AI models independently review output. Verification pyramid: auto-test → cross-model → behavioral |
| **Self-Correction** | On failure, analyzes root cause, fixes, retries (up to 3×) before escalating to you |
| **Zero Trust Security** | Capability Token + seccomp sandbox + IPC HMAC. Every Action passes through Governance |
| **Persona Decoupled** | The system's "voice" is under your control. Core produces neutral events; Persona controls how it speaks |
| **Runtime Observability** | Audit timeline, runtime invariants, Prometheus metrics |

## Quick Start

### Install

Download the binary for your platform from [Releases](https://github.com/dotnet010/GoalOS/releases).

```bash
# macOS/Linux
tar xzf goalos-<os>-<arch>.tar.gz
./goalos-daemon &
```

### Your First Goal

```bash
# Via CLI
goalos "Design a 3D rotating Rubik's Cube in HTML/CSS"

# Via HTTP API
curl -X POST http://localhost:18920/api/goals \
  -H "Content-Type: application/json" \
  -d '{"goal":"Build a CRM system"}'
```

### Configuration

On first startup, GoalOS auto-generates `~/.goalos/config/daemon.yaml` with comments. Edit it:

```yaml
daemon:
  port: 18920
  autonomy_level: autonomous   # observe|suggest|approve|autonomous

llm:
  provider: openai
  model: glm-5.1
  api_key: "sk-..."            # API key directly in config — no env vars needed
  base_url: https://your-llm-api.com/v1
  max_tokens: 4096

# Hot-reload — no daemon restart needed
# curl -X POST http://localhost:18920/api/system/reload
```

### Multi-Model Verification

Configure multiple LLM providers for parallel code review:

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
      api_key: "sk-or-..."
      base_url: https://openrouter.ai/api/v1
      allowed_for: [L0,L1,L2]
```

System calls all providers in parallel → VerdictCombiner merges results. Divergence is shown to user.

### Interaction Channels

| Channel | Use Case |
|---------|----------|
| **HTTP API** | System integration, scripting, automation |
| **CLI** (`goalos`) | Terminal users, CI/CD |
| **Web UI** | `http://localhost:18920` — goal dashboard, timeline |
| **Telegram Bot** | Mobile lightweight interaction |

## Architecture

```
User Goal → Agent(Align→Analyze→Plan) → MissionGraph
          → Governance(5-engine approval) → PipelineRunner(Check→Exec→Wait→Decide)
          → Plugin Runner(seccomp sandbox) → Artifacts(~/Goals/)
          → Verifier(verification pyramid) → CompletionContract → Delivered
```

### Core Principles

1. **Plugin over Build** — All variable capabilities are Plugins. The core never changes
2. **Event over Call** — Modules communicate via events. Auditable and replayable
3. **File over Database** — Data is files. User-owned. Zero external storage dependencies
4. **Projection over State** — State is derived from events. Caches are rebuildable
5. **Delegate over Build** — Don't rebuild what platforms already do (messaging, keychain, notifications)
6. **One over Many** — Use one until data proves you need more
7. **Interface over Implementation** — Define "what", not "how". Implementations are replaceable
8. **User-Owned over System-Managed** — Data belongs to the user. Summaries in file frontmatter
9. **Persona Decoupled** — Core produces neutral events. Persona is a replaceable rendering layer

## API Reference

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/goals` | Create a goal |
| GET | `/api/goals/:id` | Get goal status |
| GET | `/api/goals` | List all goals |
| POST | `/api/goals/:id/pause` | Pause a goal |
| POST | `/api/goals/:id/resume` | Resume a goal |
| POST | `/api/goals/:id/stop` | Stop a goal |
| GET | `/api/health` | Health check |
| POST | `/api/system/reload` | Hot-reload config |

## Build from Source

```bash
git clone https://github.com/dotnet010/GoalOS.git
cd GoalOS
go build -o goalos-daemon ./cmd/goalos/
go build -o goalos ./cmd/goalos-cli/
```

## Documentation

- [User Manual](用户手册.md) (Chinese)
- [Development Plan v0.1.0](开发计划v0.1.0.md)

## License

GPL-3.0
