// GoalOS Daemon — 核心入口。14 步启动序列。参见 05 架构文档 §2.2。
// 职责：启动 Event Bus → State Store → Scheduler → Governance → Mission Engine → HTTP Server。
// 优雅关闭：SIGINT/SIGTERM → 写 state.json → 退出。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/goalos/goalos/internal/channel"
	"github.com/goalos/goalos/internal/config"
	"github.com/goalos/goalos/internal/contextengine"
	"github.com/goalos/goalos/internal/daemon"
	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/governance"
	"github.com/goalos/goalos/internal/healthcheck"
	"github.com/goalos/goalos/internal/metrics"
	"github.com/goalos/goalos/internal/missionengine"
	"github.com/goalos/goalos/internal/persona"
	"github.com/goalos/goalos/internal/pluginrunner"
	"github.com/goalos/goalos/internal/scheduler"
	"github.com/goalos/goalos/internal/skills"
	"github.com/goalos/goalos/internal/statestore"
	"github.com/goalos/goalos/pkg/events"
)

const (
	defaultConfigPath = ".goalos/config/daemon.yaml"
)

func main() {
	startTime := time.Now()
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("[Daemon] GoalOS starting...")

	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("[Daemon] cannot find home dir: %v", err)
	}
	goalOSDir := home + "/.goalos"

	rootDirs := []string{goalOSDir}
	subDirs := []string{
		goalOSDir + "/events", goalOSDir + "/events/snapshots",
		goalOSDir + "/plugins/capability", goalOSDir + "/plugins/agent", goalOSDir + "/plugins/channel",
		goalOSDir + "/memory/decisions", goalOSDir + "/memory/lessons", goalOSDir + "/memory/patterns",
		goalOSDir + "/cache", goalOSDir + "/config", goalOSDir + "/logs",
	}
	for _, d := range rootDirs {
		if err := os.MkdirAll(d, 0700); err != nil {
			log.Fatalf("[Daemon] create dir %s: %v", d, err)
		}
	}
	for _, d := range subDirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			log.Fatalf("[Daemon] create dir %s: %v", d, err)
		}
	}
	log.Println("[Daemon] Step 1: directory tree created")

	cfg, err := config.Load(home + "/" + defaultConfigPath)
	if err != nil {
		cfg = config.Default()
	}
	log.Printf("[Daemon] Step 2: config loaded (port=%d)", cfg.Daemon.Port)

	// Step 2.5: 启动自检（v0.1.1）
	pluginsDir := home + "/.goalos/plugins/capability"
	results := healthcheck.RunAll(cfg, pluginsDir)
	log.Printf("[Daemon]\n%s", healthcheck.Report(results))
	if healthcheck.HasErrors(results) {
		log.Fatalf("[Daemon] 启动自检未通过。请修复以上问题后重新启动。")
	}
	log.Println("[Daemon] Step 2.5: health check passed")
	bus := eventbus.New()
	mreg := metrics.New() // v0.1.0 H8: Prometheus 指标注册表
	log.Println("[Daemon] Step 3: Event Bus + Metrics created")

	logFile, err := os.OpenFile(goalOSDir+"/logs/daemon.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err == nil {
		log.SetOutput(logFile)
		log.SetFlags(log.LstdFlags)
	}
	log.Printf(`{"level":"INFO","ts":"%s","msg":"Step 4: logger initialized"}`, time.Now().Format(time.RFC3339))

	store := statestore.New(goalOSDir + "/events")
	log.Printf(`{"level":"INFO","ts":"%s","msg":"Step 5: State Store initialized"}`, time.Now().Format(time.RFC3339))

	goalAnchor := scheduler.NewGoalAnchorTracker(20)
	sched := scheduler.New(bus, store, goalAnchor)
	sched.SetAutonomyLevel(cfg.Daemon.AutonomyLevel) // v0.1.1: autonomous→自动确认
	sched.Start()
	log.Printf(`{"level":"INFO","ts":"%s","msg":"Step 6: Scheduler registered"}`, time.Now().Format(time.RFC3339))

	// B14: BudgetTracker 提前声明，供 GoalCreated handler 闭包使用
	var bt *scheduler.BudgetTracker

	// TC-GL-006: GoalRunner per-Goal 执行控制。v0.1.1 fix: per-Goal PipelineRunner 避免跨 Goal 状态污染
	bus.Subscribe(events.TypeGoalCreated, func(evt events.Event) error {
		pr := scheduler.NewPipelineRunner(bus, store)
		// R-1384/R-1343: Wait 超时单一计时权威——注入 policy.approval_timeout（与 Governance 同源）
		pr.SetApprovalTimeout(time.Duration(cfg.Policy.ApprovalTimeout) * time.Second)
		if cfg.MultiLLM.Enabled && len(cfg.MultiLLM.Providers) > 0 {
			log.Printf("[Daemon] MultiLLM: enabled with %d providers", len(cfg.MultiLLM.Providers))
			// R-861: Provider 健康检查——启动时过滤不可用 Provider
			healthyCount := 0
			var providers []scheduler.ProviderClient
			for _, p := range cfg.MultiLLM.Providers {
				maxTokens := p.MaxTokens
				if maxTokens == 0 {
					maxTokens = 16384
				}
				pc := scheduler.ProviderConfig{Name: p.Name, Model: p.Model, Endpoint: p.BaseURL}
				status := scheduler.CheckProviderHealth(pc, p.APIKey)
				if status.Healthy {
					healthyCount++
					providers = append(providers, scheduler.ProviderClient{Name: p.Name, Model: p.Model,
						Client: missionengine.NewCloudLLMClient(p.BaseURL, p.APIKey, p.Model, maxTokens)})
					log.Printf("[Daemon] MultiLLM provider ✅ %s/%s: %s", p.Name, p.Model, status.Message)
				} else {
					log.Printf("[Daemon] MultiLLM provider ❌ %s/%s: %s — skipped for this session", p.Name, p.Model, status.Message)
				}
			}
			if healthyCount > 0 {
				log.Printf("[Daemon] MultiLLM: %d/%d providers healthy", healthyCount, len(cfg.MultiLLM.Providers))
				pr.SetMultiLLM(scheduler.NewMultiLLMVerifier(providers))
			} else {
				log.Printf("[Daemon] MultiLLM: all providers unhealthy, verification disabled")
			}
		} else {
			log.Printf("[Daemon] MultiLLM: disabled (enabled=%v, providers=%d)", cfg.MultiLLM.Enabled, len(cfg.MultiLLM.Providers))
		}
		gr := scheduler.NewGoalRunner(scheduler.Goal{ID: evt.GoalID, Title: fmt.Sprint(evt.Payload["title"])}, bus, store, pr, goalAnchor)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[GoalRunner] goal=%s PANIC: %v", evt.GoalID, r)
					bus.Publish(events.Event{Type: events.TypeGoalFailed, GoalID: evt.GoalID, Source: "goalrunner",
						Payload: map[string]interface{}{"reason": fmt.Sprintf("panic: %v", r), "error": "internal error"}})
				}
			}()
			log.Printf("[GoalRunner] goal=%s started", evt.GoalID)
			gr.Execute()
		}()
		return nil
	})

	// R-383 P0接线修复 #2-4: Flow/Snapshot/BudgetTracker
	flowReg := scheduler.NewFlowRegistry()
	flowComposer := scheduler.NewFlowComposer(flowReg) // A18: 创建 FlowComposer（稍后接入）
	store.SetSnapshotCallback(func(goalID string) {
		state, err := store.LoadState(goalID)
		if err != nil {
			log.Printf("[Daemon] snapshot callback: LoadState failed for %s: %v", goalID, err)
			return
		}
		if state != nil {
			if err := store.SaveSnapshot(goalID, state); err != nil {
				log.Printf("[Daemon] snapshot callback: SaveSnapshot failed for %s: %v", goalID, err)
			}
		}
	})
	bt = scheduler.NewBudgetTracker()
	bt.SetEventBus(bus)

	secretKey, err := governance.LoadOrGenerateSecret(goalOSDir + "/secrets.enc")
	if err != nil {
		log.Printf(`{"level":"WARN","msg":"Step 7: secret key: %v"}`, err)
	}
	gov := governance.New(bus, secretKey)
	gov.SetApprovalTimeout(time.Duration(cfg.Policy.ApprovalTimeout) * time.Second)
	gov.SetTokenTTL(time.Duration(cfg.Policy.TokenTTL) * time.Second) // R-1059: 令牌执行窗口
	gov.SetAutonomyLevel(cfg.Daemon.AutonomyLevel)
	gov.Start()
	log.Printf(`{"level":"INFO","ts":"%s","msg":"Step 7: Governance registered"}`, time.Now().Format(time.RFC3339))

	ctxEng := contextengine.New(home+"/Goals", home+"/.goalos/memory")
	ctxEng.Start(bus)            // R-362
	_ = persona.Get(cfg.Persona) // R-383 P0接线修复 #5: Persona渲染层——订阅 GoalCompleted/GoalFailed
	log.Printf(`{"level":"INFO","ts":"%s","msg":"Step 8: Context Engine started"}`, time.Now().Format(time.RFC3339))

	// Step 8.5: Skill 基础设施（R-351 v0.1.0 最小实现）
	skillLoader := skills.NewSkillLoader(home + "/.goalos/skills")
	skillRegistry := skills.NewSkillRegistry()
	if loaded, err := skillLoader.Load(); err == nil {
		skillRegistry.Register(loaded)
	}
	log.Printf(`{"level":"INFO","ts":"%s","msg":"Step 8.5: Skill infrastructure (%d loaded)"}`, time.Now().Format(time.RFC3339), skillRegistry.Count())
	_ = skillRegistry // v1.2: 接入 Gate 评估

	// Step 9: Agent 选择。优先级：daemon.yaml > 环境变量 > StubAgent。
	// 设计依据：R241（Ollama URL 可配置）、R251（Anthropic Provider）。
	var agent missionengine.Agent = missionengine.NewStubAgent()
	agentName := "StubAgent(fallback)"

	switch {
	case cfg.LLM.Provider == "ollama" || os.Getenv("OLLAMA_MODEL") != "":
		model := cfg.LLM.Model
		if m := os.Getenv("OLLAMA_MODEL"); m != "" {
			model = m
		}
		baseURL := cfg.LLM.BaseURL
		if baseURL == "" {
			baseURL = "http://localhost:11434"
		}
		ollamaClient := missionengine.NewOllamaClient(model, baseURL, cfg.LLM.MaxTokens)
		agent = missionengine.NewGoalAgentWithBus(ollamaClient, bus)
		agentName = "GoalAgent+Ollama(" + model + ")"

	case cfg.LLM.BaseURL != "" || os.Getenv("GOALOS_LLM_BASE_URL") != "":
		baseURL := cfg.LLM.BaseURL
		if u := os.Getenv("GOALOS_LLM_BASE_URL"); u != "" {
			baseURL = u
		}
		// E6: 优先级——config指定env > GOALOS_LLM_API_KEY > config文件值
		apiKey := os.Getenv(cfg.LLM.APIKeyEnv)
		if apiKey == "" {
			apiKey = os.Getenv("GOALOS_LLM_API_KEY")
		}
		if apiKey == "" {
			apiKey = cfg.LLM.APIKey
		}
		model := cfg.LLM.Model
		if m := os.Getenv("GOALOS_LLM_MODEL"); m != "" {
			model = m
		}
		// [FIXED] 增加 maxTokens 参数（从配置读取，默认 8192）
		maxTokens := cfg.LLM.MaxTokens
		if maxTokens == 0 {
			maxTokens = 16384
		}
		cloudClient := missionengine.NewCloudLLMClient(baseURL, apiKey, model, maxTokens)
		agent = missionengine.NewGoalAgentWithBus(cloudClient, bus)
		agentName = "GoalAgent+Cloud(" + model + ")"
	}
	// Plan 阶段超时通过配置文件 plan_timeout 设置
	if ga, ok := agent.(*missionengine.GoalAgent); ok {
		ga.SetPlanTimeout(cfg.LLM.PlanTimeout)
	}
	missionEng := missionengine.New(bus, agent)
	missionEng.SetFlowComposer(flowComposer) // A18: FlowComposer 接入
	// v0.2.2 W6 B10: autonomous 模式自动确认 MissionGraph
	missionEng.SetAutoConfirm(cfg.Daemon.AutonomyLevel == "autonomous")
	// v0.2.2 W8 B13: 设置回退 Provider（Multi-LLM 配置的第二 Provider）
	if len(cfg.MultiLLM.Providers) > 1 {
		p := cfg.MultiLLM.Providers[1]
		mt := p.MaxTokens
		if mt == 0 {
			mt = 16384
		}
		fallbackClient := missionengine.NewCloudLLMClient(p.BaseURL, p.APIKey, p.Model, mt)
		missionEng.SetFallbackAgent(missionengine.NewGoalAgentWithBus(fallbackClient, bus))
		log.Printf("[Daemon] B13: fallback provider set: %s/%s", p.Name, p.Model)
	}
	missionEng.Start()
	log.Printf(`{"level":"INFO","ts":"%s","msg":"Step 9: Mission Engine registered (%s)"}`, time.Now().Format(time.RFC3339), agentName)

	runner := pluginrunner.New(bus, secretKey, nil) // R-660: tokenVerifier=nil→fallback 无撤销检查。Week 3 注入 Engine
	runner.Start()
	for _, p := range runner.DiscoveredPlugins() {
		gov.RegisterCapabilities(p.Manifest.Name, p.Manifest.DeclaredCapabilities)
	}
	gov.RegisterCapabilities("builtin", []string{"fs.read", "fs.write", "shell.execute", "browser.open", "browser.click", "web.search"})
	log.Printf(`{"level":"INFO","ts":"%s","msg":"Step 10: Plugin Runner registered"}`, time.Now().Format(time.RFC3339))

	// v0.2.2 W6 B12: Telegram Bot Channel 启动接线
	if token := os.Getenv("TELEGRAM_BOT_TOKEN"); token != "" {
		tb := channel.NewTelegramBot(token)
		go func() {
			defer tb.Stop() // W6-B3: daemon 关闭时清理
			log.Printf("[Daemon] Step 10.5: Telegram Bot starting...")
			if err := tb.Start(func(msg channel.Message) error {
				// W6-B1: 保留 SenderID 以支持回复
				// W6-B2: 返回真实 error 而非永远 nil
				bus.Publish(events.Event{
					Type:   events.TypeMessageReceived,
					Source: "telegram-bot",
					GoalID: msg.SenderID, // 用 SenderID 关联用户
					Payload: map[string]interface{}{
						"channel": msg.Channel,
						"sender":  msg.SenderID,
						"content": msg.Content,
					},
				})
				return nil
			}); err != nil {
				log.Printf("[Daemon] Telegram Bot error: %v", err)
			}
		}()
		log.Printf(`{"level":"INFO","ts":"%s","msg":"Step 10.5: Telegram Bot registered"}`, time.Now().Format(time.RFC3339))
	}

	if _, err := store.RecoverAll(); err != nil {
		log.Printf(`{"level":"WARN","msg":"Step 11: recovery: %v"}`, err)
	} else {
		log.Printf(`{"level":"INFO","ts":"%s","msg":"Step 11: recovery ok"}`, time.Now().Format(time.RFC3339))
	}

	pidFile := goalOSDir + "/goalos.pid"
	// A19: 清理前次崩溃残留的 PID 文件
	if data, err := os.ReadFile(pidFile); err == nil {
		var oldPID int
		fmt.Sscanf(string(data), "%d", &oldPID)
		if oldPID > 0 && !isProcessAlive(oldPID) {
			os.Remove(pidFile) // 前次崩溃残留——清理
		}
	}
	pidLock, err := acquirePIDLock(pidFile)
	if err != nil {
		log.Fatalf("Step 12: PID lock failed: %v", err)
	}
	defer os.Remove(pidFile)
	defer pidLock.Close()

	api := daemon.NewHandler()
	api.Metrics = mreg // v0.1.0 H8: 指标注册表
	api.SetPort(cfg.Daemon.Port)
	api.SetStartTime(startTime)
	daemon.SetEventBus(bus)
	daemon.SetStateStore(store)
	sse := daemon.NewSSEManager()
	// R-835: PlanProgressUpdate → SSE 实时推送 LLM 进展
	bus.Subscribe("PlanProgressUpdate", func(evt events.Event) error { sse.Push("PlanProgressUpdate", evt.Payload); return nil })
	// R-845: MultiLLMVerificationCompleted → SSE + API（含完整审查报告）
	bus.Subscribe("MultiLLMVerificationCompleted", func(evt events.Event) error {
		sse.Push("MultiLLMVerificationCompleted", evt.Payload)
		if v, _ := evt.Payload["verdict"].(string); v != "" {
			report := buildMultiLLMReport(evt.Payload)
			api.UpdateMultiLLMVerdict(evt.GoalID, v, report)
		}
		return nil
	})
	bus.Subscribe("GoalCreated", func(evt events.Event) error { sse.Push("GoalCreated", evt.Payload); return nil })
	bus.Subscribe("GoalCompleted", func(evt events.Event) error {
		failed, _ := evt.Payload["failed"].(float64)
		if failed > 0 {
			api.UpdateGoalStatus(evt.GoalID, "部分完成")
		} else {
			api.UpdateGoalStatus(evt.GoalID, "已完成")
		}
		sse.Push("GoalCompleted", evt.Payload)
		return nil
	})
	bus.Subscribe("GoalFailed", func(evt events.Event) error {
		api.UpdateGoalStatus(evt.GoalID, "已失败")
		reason, _ := evt.Payload["error"].(string)
		hint := "目标执行失败"
		var suggestions []daemon.Suggestion
		if reason != "" {
			hint = "失败原因: " + reason
		}
		suggestions = append(suggestions, daemon.Suggestion{Action: "retry", Label: "重试当前目标"})
		suggestions = append(suggestions, daemon.Suggestion{Action: "new_goal", Label: "重新描述目标"})
		api.SetGoalErrorHint(evt.GoalID, hint, suggestions)
		sse.Push("GoalFailed", evt.Payload)
		return nil
	})
	// v0.1.1 进度状态：用户可见的中间状态
	bus.Subscribe("PlanRequested", func(evt events.Event) error {
		api.UpdateGoalStatus(evt.GoalID, "正在分析目标")
		return nil
	})
	bus.Subscribe("MissionGenerated", func(evt events.Event) error {
		api.UpdateGoalStatus(evt.GoalID, "正在执行")
		if nc, ok := evt.Payload["node_count"].(float64); ok {
			api.SetGoalActionsTotal(evt.GoalID, int(nc)) // R-378: 设置总 Action 数
		}
		return nil
	})
	// R-833 B18: ActionScheduled → SSE + action status
	bus.Subscribe("ActionScheduled", func(evt events.Event) error {
		api.UpdateGoalProgress(evt.GoalID)
		if actionID, ok := evt.Payload["action_id"].(string); ok && actionID != "" {
			actionType, _ := evt.Payload["action_type"].(string)
			api.UpdateActionStatus(evt.GoalID, actionID, actionType, "scheduled")
		}
		sse.Push("ActionScheduled", evt.Payload)
		return nil
	})
	// R-833 B18: ActionFailed → action status
	bus.Subscribe("ActionFailed", func(evt events.Event) error {
		api.UpdateGoalProgress(evt.GoalID)
		if actionID, ok := evt.Payload["action_id"].(string); ok && actionID != "" {
			api.UpdateActionStatus(evt.GoalID, actionID, "", "failed")
		}
		sse.Push("ActionFailed", evt.Payload)
		return nil
	})
	bus.Subscribe("ActionRetrying", func(evt events.Event) error {
		api.UpdateGoalStatus(evt.GoalID, "正在重试")
		return nil
	})
	bus.Subscribe("HumanInterventionRequested", func(evt events.Event) error {
		api.UpdateGoalStatus(evt.GoalID, "需要你的帮助")
		return nil
	})
	bus.Subscribe("ActionPendingApproval", func(evt events.Event) error {
		sse.Push("ActionPendingApproval", evt.Payload)
		api.TrackPendingApproval(daemon.PendingApproval{
			ActionID: fmt.Sprint(evt.Payload["action_id"]), GoalID: evt.GoalID,
			ActionType: fmt.Sprint(evt.Payload["action_description"]),
			RiskLevel:  fmt.Sprint(evt.Payload["risk_level"]),
		})
		return nil
	})
	// R-833 B18: ActionCompleted → SSE + action status
	bus.Subscribe(events.TypeActionCompleted, func(evt events.Event) error {
		if actionID, ok := evt.Payload["action_id"].(string); ok && actionID != "" {
			api.UpdateActionStatus(evt.GoalID, actionID, "", "completed")
		}
		// R-828 final: flat payload——status 不再嵌套在 result 内
		api.TrackResult(evt.GoalID, evt.Payload)
		if status, _ := evt.Payload["status"].(string); status == "success" {
			api.UpdateGoalStatus(evt.GoalID, "已完成")
		}
		sse.Push("ActionCompleted", evt.Payload)
		return nil
	})
	bus.Subscribe(events.TypeActionApproved, func(evt events.Event) error {
		api.RemovePendingApproval(fmt.Sprint(evt.Payload["action_id"]))
		return nil
	})
	bus.Subscribe(events.TypeActionRejected, func(evt events.Event) error {
		api.RemovePendingApproval(fmt.Sprint(evt.Payload["action_id"]))
		return nil
	})

	mux := buildHTTPMux(api, sse, cfg, bus, missionEng, home)
	server := &http.Server{Addr: fmt.Sprintf("localhost:%d", cfg.Daemon.Port), Handler: mux}
	go func() {
		log.Printf(`{"level":"INFO","ts":"%s","msg":"Step 13: HTTP on localhost:%d"}`, time.Now().Format(time.RFC3339), cfg.Daemon.Port)
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("HTTP: %v", err)
		}
	}()

	bus.Publish(events.Event{Type: events.TypeSystemStarted, Source: "daemon", Seq: 0,
		Payload: map[string]interface{}{"pid": os.Getpid(), "port": cfg.Daemon.Port}})
	log.Printf(`{"level":"INFO","ts":"%s","msg":"Step 14: SystemStarted"}`, time.Now().Format(time.RFC3339))
	// SIGHUP 热加载配置（v0.1.0 UX1）
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGHUP)
		for range sigCh {
			configPath := home + "/.goalos/config/daemon.yaml"
			if err := cfg.Reload(configPath); err != nil {
				log.Printf("[Daemon] SIGHUP reload failed: %v", err)
			} else {
				// R-1058: 热重载参数经事件总线分发（与 HTTP reload 同路径）。
				bus.Publish(events.Event{
					Type:   events.TypeConfigReloaded,
					Source: "daemon",
					Payload: map[string]interface{}{
						"approval_timeout_seconds": float64(cfg.Policy.ApprovalTimeout),
						"token_ttl_seconds":        float64(cfg.Policy.TokenTTL),
						"autonomy_level":           cfg.Daemon.AutonomyLevel,
					},
				})
				log.Printf("[Daemon] SIGHUP reloaded: model=%s", cfg.LLM.Model)
			}
		}
	}()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	log.Printf(`{"level":"INFO","ts":"%s","msg":"Shutting down..."}`, time.Now().Format(time.RFC3339))
	gov.Stop() // H18: 停止 auditFlushLoop + revokedTokensCleanup
	bus.Shutdown()
	log.Printf(`{"level":"INFO","ts":"%s","msg":"EventBus stopped."}`, time.Now().Format(time.RFC3339))
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Daemon.ShutdownTimeout)
	defer cancel()
	api.SetShutdownHook(func() { cancel() })
	server.Shutdown(shutdownCtx)
	log.Printf(`{"level":"INFO","ts":"%s","msg":"GoalOS stopped."}`, time.Now().Format(time.RFC3339))
}

// buildHTTPMux 组装 HTTP 路由表。
// R-1372（C-UI-01）: Dashboard 已拆除 R-1372——"/" 路由不再注册页面处理器，
// CLI 是唯一软件入口（R-1123），未匹配路径由 ServeMux 返回 404。
// 抽取为独立函数：契约测试直测真实路由表（R-1372）。
func buildHTTPMux(api *daemon.Handler, sse *daemon.SSEManager, cfg *config.Config,
	bus *eventbus.EventBus, missionEng *missionengine.Engine, home string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/events", sse.HandleSSE)
	mux.HandleFunc("/api/health", api.HandleHealth)
	mux.HandleFunc("/api/goals", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			api.HandleCreateGoal(w, r)
		} else {
			api.HandleListGoals(w, r)
		}
	})
	mux.HandleFunc("/api/goals/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/goals/")
		parts := strings.Split(path, "/")

		// R-846~R-850: MultiLLM ReviewReport API 子路径
		// GET  /api/goals/:goal_id/reviews              → 审查摘要列表
		// GET  /api/goals/:goal_id/reviews/:action_id    → 完整审查报告
		// POST /api/goals/:goal_id/reviews/:action_id/decide → 用户决策
		if len(parts) >= 2 && parts[1] == "reviews" {
			goalID := parts[0]
			switch {
			case len(parts) == 4 && parts[3] == "decide" && r.Method == http.MethodPost:
				api.HandleDecideReview(w, r, goalID, parts[2]) // parts[2]=actionID
			case len(parts) == 3 && parts[2] != "":
				api.HandleGetReviewDetail(w, r, goalID, parts[2]) // parts[2]=actionID
			default:
				api.HandleGetReviews(w, r, goalID)
			}
			return
		}

		id := parts[0]
		r.SetPathValue("id", id)
		switch {
		case strings.HasSuffix(r.URL.Path, "/pause"):
			api.HandlePauseGoal(w, r)
		case strings.HasSuffix(r.URL.Path, "/resume"):
			api.HandleResumeGoal(w, r)
		case strings.HasSuffix(r.URL.Path, "/stop"):
			api.HandleStopGoal(w, r)
		case strings.HasSuffix(r.URL.Path, "/log"):
			api.HandleGoalLog(w, r)
		case strings.HasSuffix(r.URL.Path, "/events"):
			api.HandleGoalEvents(w, r)
		default:
			api.HandleGetGoal(w, r)
		}
	})
	mux.HandleFunc("/api/approvals", api.HandleListApprovals)
	mux.HandleFunc("/api/approvals/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/approvals/")
		id = strings.Split(id, "/")[0]
		r.SetPathValue("id", id)
		if strings.HasSuffix(r.URL.Path, "/approve") {
			api.HandleApprove(w, r)
		} else if strings.HasSuffix(r.URL.Path, "/reject") {
			api.HandleReject(w, r)
		} else {
			http.Error(w, `{"error":{"code":"INVALID_REQUEST"}}`, http.StatusNotFound)
		}
	})
	mux.HandleFunc("/api/system/status", api.HandleSystemStatus)
	mux.HandleFunc("/api/system/stop", api.HandleDaemonStop)
	mux.HandleFunc("/api/system/restart", api.HandleDaemonRestart)
	// R-840: MultiLLM 运行时配置——用户可随时开关
	mux.HandleFunc("/api/system/multi-llm", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var body struct {
				Enabled bool `json:"enabled"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			cfg.MultiLLM.Enabled = body.Enabled
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"multi_llm":{"enabled":%v,"providers":%d}}`, cfg.MultiLLM.Enabled, len(cfg.MultiLLM.Providers))
			log.Printf("[Daemon] MultiLLM toggled: enabled=%v", cfg.MultiLLM.Enabled)
		} else {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"multi_llm":{"enabled":%v,"providers":%d}}`, cfg.MultiLLM.Enabled, len(cfg.MultiLLM.Providers))
		}
	})

	mux.HandleFunc("/metrics", api.HandleMetrics) // v0.1.0 H8: Prometheus 格式指标端点
	mux.HandleFunc("/api/system/reload", func(w http.ResponseWriter, r *http.Request) {
		configPath := home + "/.goalos/config/daemon.yaml"
		if err := cfg.Reload(configPath); err != nil {
			http.Error(w, `{"error":{"code":"INTERNAL_ERROR","message":"`+err.Error()+`"}}`, http.StatusInternalServerError)
			return
		}
		// R-1058: 热重载参数经事件总线分发（nginx 代际模型）——组件自行订阅，
		// 不再由 main 逐一直调（D-1: 手工接线每加一个参数就多一处遗漏点）。
		bus.Publish(events.Event{
			Type:   events.TypeConfigReloaded,
			Source: "daemon",
			Payload: map[string]interface{}{
				"approval_timeout_seconds": float64(cfg.Policy.ApprovalTimeout),
				"token_ttl_seconds":        float64(cfg.Policy.TokenTTL),
				"autonomy_level":           cfg.Daemon.AutonomyLevel,
			},
		})
		// E6: 优先级——config指定env > GOALOS_LLM_API_KEY > config文件值
		apiKey := os.Getenv(cfg.LLM.APIKeyEnv)
		if apiKey == "" {
			apiKey = os.Getenv("GOALOS_LLM_API_KEY")
		}
		if apiKey == "" {
			apiKey = cfg.LLM.APIKey
		}
		// [FIXED] 增加 maxTokens 参数（从配置读取，默认 8192）
		maxTokens := cfg.LLM.MaxTokens
		if maxTokens == 0 {
			maxTokens = 16384
		}
		// B19: 跨 Provider 热加载——与 startup 逻辑一致
		var newAgent missionengine.Agent
		model := cfg.LLM.Model
		if m := os.Getenv("GOALOS_LLM_MODEL"); m != "" {
			model = m
		}
		switch {
		case cfg.LLM.Provider == "ollama" || os.Getenv("OLLAMA_MODEL") != "":
			if m := os.Getenv("OLLAMA_MODEL"); m != "" {
				model = m
			}
			baseURL := cfg.LLM.BaseURL
			if baseURL == "" {
				baseURL = "http://localhost:11434"
			}
			ollamaClient := missionengine.NewOllamaClient(model, baseURL, maxTokens)
			newAgent = missionengine.NewGoalAgentWithBus(ollamaClient, bus)
		default:
			cloudClient := missionengine.NewCloudLLMClient(cfg.LLM.BaseURL, apiKey, model, maxTokens)
			newAgent = missionengine.NewGoalAgentWithBus(cloudClient, bus)
		}
		if ga, ok := newAgent.(*missionengine.GoalAgent); ok {
			ga.SetPlanTimeout(cfg.LLM.PlanTimeout)
		}
		missionEng.SetAgent(newAgent)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"reloaded","model":"` + cfg.LLM.Model + `","provider":"` + cfg.LLM.Provider + `"}`))
		log.Printf("[Daemon] hot-reloaded: provider=%s model=%s, agent swapped", cfg.LLM.Provider, cfg.LLM.Model)
	})
	return mux
}

func acquirePIDLock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("pid lock: %w", err)
	}
	fmt.Fprintf(f, "%d\n", os.Getpid())
	return f, nil
}

// isProcessAlive 检查进程是否存活（A19）。
func isProcessAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Unix: Signal(0) 不发送信号，只检查权限和存活
	err = p.Signal(os.Signal(nil))
	return err == nil
}

// buildMultiLLMReport 从 MultiLLM 事件构造人类可读审查报告（R-845）。
func buildMultiLLMReport(payload map[string]interface{}) string {
	var b strings.Builder
	if votes, ok := payload["votes"].([]interface{}); ok {
		for _, v := range votes {
			if vt, ok := v.(map[string]interface{}); ok {
				fmt.Fprintf(&b, "%s/%s → %s",
					vt["provider"], vt["model"], vt["vote"])
				if r, ok := vt["reasoning"].(string); ok && r != "" {
					fmt.Fprintf(&b, ": %s", r)
				}
				b.WriteString("\n")
			}
		}
	}
	return b.String()
}
