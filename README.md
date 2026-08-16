# GoalOS — A Personal Goal Execution Operating System

[![Build & Release](https://img.shields.io/github/actions/workflow/status/dotnet010/GoalOS/docker-publish.yml)](https://github.com/dotnet010/GoalOS/actions/workflows/docker-publish.yml)
[![Release](https://img.shields.io/github/v/release/dotnet010/GoalOS)](https://github.com/dotnet010/GoalOS/releases)

> **You state the goal. The system delivers it — safely.**

GoalOS is not a chatbot, not a workflow engine, not an agent framework. It is a **personal operating system for goal execution**. You say what you want — the system understands, plans, executes, verifies, and delivers.

[中文文档](README_zh.md)

---

## Vision

You state your goal → The system understands → You agree on "what done means" (CompletionContract) → The system executes safely → Multi-LLM verification → Verified → Delivered.

Any feature that does not directly serve this chain should be removed.

## v0.3.0 Core Capabilities

| Capability | Description |
|------------|-------------|
| **CompletionContract** | System establishes a contract with you on "what done means" before execution begins |
| **PipelineRunner** | 3-primitive pipeline: Check→Exec→Decide. Wait is intermediate state. State projected from events |
| **Flow Templates** | Same goal type follows the same standard process — predictable results. Unknown flow → confirmation flow only (no silent fallback, R-1368) |
| **Multi-LLM Verification** | Multiple AI models (cross-Provider) independently review output. Voting-based verdict (R-844). Debate round on divergence (R-860). Cold review mode (R-858) |
| **ReviewReport + User Decision** | Structured review reports with per-Provider reasoning. Dashboard review panel + CLI `goalos review`. User decides: retry / accept / refine |
| **Verification Pyramid** | auto_tests → cross_model_review → behavioral_tests. Deterministic verification is the final authority |
| **New Session Redo** | On failure: fresh session retry (1×) → human handoff. No same-session retry loop (Semantic Drift protection) |
| **PlanHash Tamper Detection** | SHA256(MissionGraph) computed at plan time, verified throughout execution (R-859) |
| **Provider Health Check** | All MultiLLM providers tested at daemon startup. Unhealthy providers auto-skipped (R-861) |
| **Zero Trust Security** | Capability Token + seccomp sandbox (Linux) + FD3 IPC + HMAC. Every Action passes through Governance. Governance face UDS-only — approvals family never exposed over TCP (R-1378/R-1322) |
| **Persona Decoupled** | The system's "voice" is under your control. Core produces neutral events; Persona controls how it speaks |
| **Honest Feedback** | System never fakes success. MultiLLM review is probabilistic — not deterministic verification. Review report shows this disclosure (R-865) |
| **WAL Truth Model** | events.jsonl = single logical truth (append + fsync commit point). Snapshot projections are discardable lagging side-effects (R-1397). CRC32 + global seq + hash chain integrity (R-1393/R-1453) |
| **State Algebra Matrix** | Four-dimensional state matrix (Goal×Action×Pipeline×Approval) as single authority. Illegal transitions rejected + StateMachineViolation (R-1095/R-1407/R-1343) |
| **Runtime Observability** | Audit timeline, runtime invariants, Prometheus metrics, SSE progress streaming |

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

### Review MultiLLM Verification

```bash
# View review summary for a goal
goalos review <goal_id>

# View full review report with per-Provider reasoning
goalos review <goal_id> <action_id>

# Make a decision on FAIL/WARN verdict
goalos review <goal_id> <action_id> --retry --feedback "fix the auth issue"
goalos review <goal_id> <action_id> --accept --confirm
goalos review <goal_id> <action_id> --refine "use argon2 instead of bcrypt"
```

### Configuration

On first startup, GoalOS auto-generates `~/.goalos/config/daemon.yaml` with comments. Edit it:

```yaml
daemon:
  port: 18920
  autonomy_level: approve   # observe|suggest|approve|autonomous

llm:
  provider: openai
  model: qwen3.6-flash
  api_key: "sk-..."         # API key directly in config
  base_url: https://your-llm-api.com/v1
  max_tokens: 65536

# Hot-reload — no daemon restart needed
# curl -X POST http://localhost:18920/api/system/reload
```

### Multi-Model Verification

Configure multiple LLM providers for parallel code review. System calls all providers in parallel → VerdictCombiner merges results with voting (R-844). Provider health checked at startup (R-861).

```yaml
multi_llm:
  enabled: true
  debate_round: false        # R-860: enable cross-model debate (adds ~1.5x token cost)
  cold_review: false         # R-858: isolate verifier from builder context
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

### Interaction Channels

| Channel | Use Case |
|---------|----------|
| **HTTP API** | System integration, scripting, automation |
| **CLI** (`goalos`) | Terminal users, CI/CD |
| **Web UI** | `http://localhost:18920` — goal dashboard, timeline, review panel |
| **Telegram Bot** | Mobile lightweight interaction |

## Architecture

```
User Goal → Agent(Align→Analyze→Plan) → MissionGraph + PlanHash
          → Governance(5-engine approval) → PipelineRunner(Check→Exec→Decide)
          → Plugin Runner(seccomp sandbox, FD3 IPC) → Artifacts(~/Goals/)
          → MultiLLM Verification(voting + debate + cold review)
          → ReviewReport → User Decision(retry/accept/refine)
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
10. **Honest Feedback** — System truthfully reflects state. Never fakes success. Never silently downgrades user goals. User has final decision authority

## API Reference

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/goals` | Create a goal |
| GET | `/api/goals/:id` | Get goal status (includes MultiLLM verdict) |
| GET | `/api/goals` | List all goals |
| GET | `/api/goals/:id/reviews` | List MultiLLM review summaries |
| GET | `/api/goals/:id/reviews/:action_id` | Get full review report (per-Provider reasoning) |
| POST | `/api/goals/:id/reviews/:action_id/decide` | Submit user decision (accept/retry/refine) |
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
go build -o goalos-cli ./cmd/goalos-cli/
```

## Documentation

- [User Manual (Chinese)](用户手册.md)
- [Architecture Meeting Minutes (Chinese)](../开发文档/会议纪要.md)

## License

GPL-3.0
