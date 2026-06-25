// GoalOS Daemon — 核心入口。14 步启动序列。参见 05 架构文档 §2.2。
// 职责：启动 Event Bus → State Store → Scheduler → Governance → Mission Engine → HTTP Server。
// 优雅关闭：SIGINT/SIGTERM → 写 state.json → 退出。
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/goalos/goalos/internal/config"
	"github.com/goalos/goalos/internal/contextengine"
	"github.com/goalos/goalos/internal/daemon"
	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/governance"
	"github.com/goalos/goalos/internal/missionengine"
	"github.com/goalos/goalos/internal/pluginrunner"
	"github.com/goalos/goalos/internal/scheduler"
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
		if err := os.MkdirAll(d, 0700); err != nil { log.Fatalf("[Daemon] create dir %s: %v", d, err) }
	}
	for _, d := range subDirs {
		if err := os.MkdirAll(d, 0755); err != nil { log.Fatalf("[Daemon] create dir %s: %v", d, err) }
	}
	log.Println("[Daemon] Step 1: directory tree created")

	cfg, err := config.Load(home + "/" + defaultConfigPath)
	if err != nil { cfg = config.Default() }
	log.Printf("[Daemon] Step 2: config loaded (port=%d)", cfg.Daemon.Port)

	bus := eventbus.New()
	log.Println("[Daemon] Step 3: Event Bus created")

	logFile, err := os.OpenFile(goalOSDir+"/logs/daemon.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err == nil { log.SetOutput(logFile); log.SetFlags(0) }
	log.Printf(`{"level":"INFO","ts":"%s","msg":"Step 4: logger initialized"}`, time.Now().Format(time.RFC3339))

	store := statestore.New(goalOSDir + "/events")
	log.Printf(`{"level":"INFO","ts":"%s","msg":"Step 5: State Store initialized"}`, time.Now().Format(time.RFC3339))

	goalAnchor := scheduler.NewGoalAnchorTracker(20)
	sched := scheduler.New(bus, store, goalAnchor)
	sched.Start()
	log.Printf(`{"level":"INFO","ts":"%s","msg":"Step 6: Scheduler registered"}`, time.Now().Format(time.RFC3339))

	secretKey, err := governance.LoadOrGenerateSecret(goalOSDir + "/secrets.enc")
	if err != nil { log.Printf(`{"level":"WARN","msg":"Step 7: secret key: %v"}`, err) }
	gov := governance.New(bus, secretKey)
	gov.SetAutonomyLevel(cfg.Daemon.AutonomyLevel)
	gov.Start()
	log.Printf(`{"level":"INFO","ts":"%s","msg":"Step 7: Governance registered"}`, time.Now().Format(time.RFC3339))

	ctxEng := contextengine.New(home+"/Goals", home+"/.goalos/memory")
	_ = ctxEng
	log.Printf(`{"level":"INFO","ts":"%s","msg":"Step 8: Context Engine registered"}`, time.Now().Format(time.RFC3339))

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
		ollamaClient := missionengine.NewOllamaClient(model, baseURL)
		agent = missionengine.NewGoalAgentWithBus(ollamaClient, bus)
		agentName = "GoalAgent+Ollama(" + model + ")"

	case cfg.LLM.BaseURL != "" || os.Getenv("GOALOS_LLM_BASE_URL") != "":
		baseURL := cfg.LLM.BaseURL
		if u := os.Getenv("GOALOS_LLM_BASE_URL"); u != "" {
			baseURL = u
		}
		apiKey := os.Getenv(cfg.LLM.APIKeyEnv)
		if k := os.Getenv("GOALOS_LLM_API_KEY"); k != "" {
			apiKey = k
		}
		model := cfg.LLM.Model
		if m := os.Getenv("GOALOS_LLM_MODEL"); m != "" {
			model = m
		}
		cloudClient := missionengine.NewCloudLLMClient(baseURL, apiKey, model)
		agent = missionengine.NewGoalAgentWithBus(cloudClient, bus)
		agentName = "GoalAgent+Cloud(" + model + ")"
	}
	missionEng := missionengine.New(bus, agent)
	missionEng.Start()
	log.Printf(`{"level":"INFO","ts":"%s","msg":"Step 9: Mission Engine registered (%s)"}`, time.Now().Format(time.RFC3339), agentName)

	runner := pluginrunner.New(bus, secretKey)
	runner.Start()
	for _, p := range runner.DiscoveredPlugins() {
		gov.RegisterCapabilities(p.Manifest.Name, p.Manifest.DeclaredCapabilities)
	}
	gov.RegisterCapabilities("builtin", []string{"fs.read", "fs.write", "shell.execute", "browser.open", "browser.click", "web.search"})
	log.Printf(`{"level":"INFO","ts":"%s","msg":"Step 10: Plugin Runner registered"}`, time.Now().Format(time.RFC3339))

	if _, err := store.RecoverAll(); err != nil {
		log.Printf(`{"level":"WARN","msg":"Step 11: recovery: %v"}`, err)
	} else {
		log.Printf(`{"level":"INFO","ts":"%s","msg":"Step 11: recovery ok"}`, time.Now().Format(time.RFC3339))
	}

	pidFile := goalOSDir + "/goalos.pid"
	pidLock, err := acquirePIDLock(pidFile)
	if err != nil { log.Fatalf("Step 12: PID lock failed") }
	defer os.Remove(pidFile)
	defer pidLock.Close()

	api := daemon.NewHandler()
	api.SetPort(cfg.Daemon.Port); api.SetStartTime(startTime)
	daemon.SetEventBus(bus); daemon.SetStateStore(store)
	sse := daemon.NewSSEManager()
	bus.Subscribe("GoalCreated", func(evt events.Event) error { sse.Push("GoalCreated", evt.Payload); return nil })
	bus.Subscribe("GoalCompleted", func(evt events.Event) error {
		status := "已完成"
		if reason, _ := evt.Payload["reason"].(string); reason != "" { status = "需要处理" }
		api.UpdateGoalStatus(evt.GoalID, status)
		sse.Push("GoalCompleted", evt.Payload); return nil
	})
	bus.Subscribe("ActionPendingApproval", func(evt events.Event) error {
		sse.Push("ActionPendingApproval", evt.Payload)
		api.TrackPendingApproval(daemon.PendingApproval{
			ActionID: fmt.Sprint(evt.Payload["action_id"]), GoalID: evt.GoalID,
			ActionType: fmt.Sprint(evt.Payload["action_description"]),
			RiskLevel: fmt.Sprint(evt.Payload["risk_level"]),
		})
		return nil
	})
	bus.Subscribe(events.TypeActionCompleted, func(evt events.Event) error {
		if result, ok := evt.Payload["result"]; ok {
			api.TrackResult(evt.GoalID, result)
			if m, ok := result.(map[string]interface{}); ok && m["status"] == "success" {
				api.UpdateGoalStatus(evt.GoalID, "已完成")
			}
		}
		return nil
	})
	bus.Subscribe(events.TypeActionApproved, func(evt events.Event) error {
		api.RemovePendingApproval(fmt.Sprint(evt.Payload["action_id"])); return nil
	})
	bus.Subscribe(events.TypeActionRejected, func(evt events.Event) error {
		api.RemovePendingApproval(fmt.Sprint(evt.Payload["action_id"])); return nil
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/", daemon.HandleDashboard)
	mux.HandleFunc("/api/events", sse.HandleSSE)
	mux.HandleFunc("/api/health", api.HandleHealth)
	mux.HandleFunc("/api/goals", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost { api.HandleCreateGoal(w, r) } else { api.HandleListGoals(w, r) }
	})
	mux.HandleFunc("/api/goals/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/goals/"); id = strings.Split(id, "/")[0]; r.SetPathValue("id", id)
		switch {
		case strings.HasSuffix(r.URL.Path, "/pause"): api.HandlePauseGoal(w, r)
		case strings.HasSuffix(r.URL.Path, "/resume"): api.HandleResumeGoal(w, r)
		case strings.HasSuffix(r.URL.Path, "/stop"): api.HandleStopGoal(w, r)
		case strings.HasSuffix(r.URL.Path, "/log"): api.HandleGoalLog(w, r)
		case strings.HasSuffix(r.URL.Path, "/events"): api.HandleGoalEvents(w, r)
		default: api.HandleGetGoal(w, r)
		}
	})
	mux.HandleFunc("/api/approvals", api.HandleListApprovals)
	mux.HandleFunc("/api/approvals/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/approvals/"); id = strings.Split(id, "/")[0]; r.SetPathValue("id", id)
		if strings.HasSuffix(r.URL.Path, "/approve") { api.HandleApprove(w, r) } else if strings.HasSuffix(r.URL.Path, "/reject") { api.HandleReject(w, r) } else { http.Error(w, `{"error":{"code":"INVALID_REQUEST"}}`, http.StatusNotFound) }
	})
	mux.HandleFunc("/api/system/status", api.HandleSystemStatus)
	mux.HandleFunc("/api/system/stop", api.HandleDaemonStop)
	mux.HandleFunc("/api/system/restart", api.HandleDaemonRestart)
		mux.HandleFunc("/api/system/reload", func(w http.ResponseWriter, r *http.Request) {
			configPath := home + "/.goalos/config/daemon.yaml"
			if err := cfg.Reload(configPath); err != nil {
				http.Error(w, `{"error":{"code":"INTERNAL_ERROR","message":"`+err.Error()+`"}}`, http.StatusInternalServerError)
				return
			}
			apiKey := os.Getenv(cfg.LLM.APIKeyEnv)
			if k := os.Getenv("GOALOS_LLM_API_KEY"); k != "" { apiKey = k }
			cloudClient := missionengine.NewCloudLLMClient(cfg.LLM.BaseURL, apiKey, cfg.LLM.Model)
			newAgent := missionengine.NewGoalAgentWithBus(cloudClient, bus)
			missionEng.SetAgent(newAgent)
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status":"reloaded","model":"` + cfg.LLM.Model + `"}`))
			log.Printf("[Daemon] hot-reloaded: model=%s, agent swapped", cfg.LLM.Model)
		})
	server := &http.Server{Addr: fmt.Sprintf("localhost:%d", cfg.Daemon.Port), Handler: mux}
	go func() {
		log.Printf(`{"level":"INFO","ts":"%s","msg":"Step 13: HTTP on localhost:%d"}`, time.Now().Format(time.RFC3339), cfg.Daemon.Port)
		if err := server.ListenAndServe(); err != http.ErrServerClosed { log.Fatalf("HTTP: %v", err) }
	}()

	bus.Publish(events.Event{Type: events.TypeSystemStarted, Source: "daemon", Seq: 0,
		Payload: map[string]interface{}{"pid": os.Getpid(), "port": cfg.Daemon.Port}})
	log.Printf(`{"level":"INFO","ts":"%s","msg":"Step 14: SystemStarted"}`, time.Now().Format(time.RFC3339))
		// SIGHUP 热加载配置（v1.1.0 UX1）
		go func() { sigCh := make(chan os.Signal, 1); signal.Notify(sigCh, syscall.SIGHUP); for range sigCh { configPath := home + "/.goalos/config/daemon.yaml"; if err := cfg.Reload(configPath); err != nil { log.Printf("[Daemon] SIGHUP reload failed: %v", err) } else { log.Printf("[Daemon] SIGHUP reloaded: model=%s", cfg.LLM.Model) } } }()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	log.Printf(`{"level":"INFO","ts":"%s","msg":"Shutting down..."}`, time.Now().Format(time.RFC3339))
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Daemon.ShutdownTimeout)
	defer cancel()
	api.SetShutdownHook(func() { cancel() })
	server.Shutdown(shutdownCtx)
	log.Printf(`{"level":"INFO","ts":"%s","msg":"GoalOS stopped."}`, time.Now().Format(time.RFC3339))
}

func acquirePIDLock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil { return nil, fmt.Errorf("pid lock: %w", err) }
	fmt.Fprintf(f, "%d\n", os.Getpid())
	return f, nil
}
