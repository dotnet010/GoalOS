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

	// Step 1: Ensure ~/.goalos/ exists, create full directory tree.
	// 根目录 0700。子目录 0755。文件 0600。
	rootDirs := []string{
		goalOSDir,
	}
	subDirs := []string{
		goalOSDir + "/events",
		goalOSDir + "/events/snapshots",
		goalOSDir + "/plugins/capability",
		goalOSDir + "/plugins/agent",
		goalOSDir + "/plugins/channel",
		goalOSDir + "/memory/decisions",
		goalOSDir + "/memory/lessons",
		goalOSDir + "/memory/patterns",
		goalOSDir + "/cache",
		goalOSDir + "/config",
		goalOSDir + "/logs",
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
	log.Println("[Daemon] Step 1: directory tree created (root 0700, subdirs 0755)")

	// Step 2: Load config (优先级: 环境变量 > daemon.yaml > 默认值)
	cfg, err := config.Load(home + "/" + defaultConfigPath)
	if err != nil {
		log.Printf("[Daemon] Step 2: config load warning: %v (using defaults)", err)
		cfg = config.Default()
	}
	log.Printf("[Daemon] Step 2: config loaded (port=%d, autonomy=%s, persona=%s, llm=%s/%s)",
		cfg.Daemon.Port, cfg.Daemon.AutonomyLevel, cfg.Persona, cfg.LLM.Provider, cfg.LLM.Model)

	// Step 3: Create Event Bus
	bus := eventbus.New()
	log.Println("[Daemon] Step 3: Event Bus created")

	// Step 4: Create Logger (JSON 格式, INFO 级别)
	logFile, err := os.OpenFile(goalOSDir+"/logs/daemon.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		log.Printf("[Daemon] Step 4: log file creation failed (using stderr): %v", err)
	} else {
		log.SetOutput(logFile)
		log.SetFlags(0) // JSON 格式不需要 flag 前缀
		log.Printf(`{"level":"INFO","ts":"%s","msg":"Step 4: logger initialized (JSON format)"}`, time.Now().Format(time.RFC3339))
	}

	// Step 5: Create State Store
	store := statestore.New(goalOSDir + "/events")
	log.Printf(`{"level":"INFO","ts":"%s","msg":"Step 5: State Store initialized"}`, time.Now().Format(time.RFC3339))

	// Step 6: Register Scheduler (state machine driver, 含 GoalAnchor)
	goalAnchor := scheduler.NewGoalAnchorTracker(20)
	sched := scheduler.New(bus, store, goalAnchor)
	sched.Start()
	log.Printf(`{"level":"INFO","ts":"%s","msg":"Step 6: Scheduler registered (GoalAnchor N=20)"}`, time.Now().Format(time.RFC3339))

	// Step 7: Register Governance (execution gate, 五引擎)
	secretKey, err := governance.LoadOrGenerateSecret(goalOSDir + "/secrets.enc")
	if err != nil {
		log.Printf(`{"level":"WARN","ts":"%s","msg":"Step 7: secret key load failed, tokens disabled: %v"}`, time.Now().Format(time.RFC3339), err)
	}
	gov := governance.New(bus, secretKey)
	gov.Start()
	log.Printf(`{"level":"INFO","ts":"%s","msg":"Step 7: Governance registered (5 engines)"}`, time.Now().Format(time.RFC3339))

	// Step 8: Register Context Engine
	ctxEng := contextengine.New(home+"/Goals", home+"/.goalos/memory")
	_ = ctxEng
	log.Printf(`{"level":"INFO","ts":"%s","msg":"Step 8: Context Engine registered"}`, time.Now().Format(time.RFC3339))

	// Step 9: Register Mission Engine。默认 StubAgent；配置 LLM Provider 后使用 GoalAgent。
	var agent missionengine.Agent = missionengine.NewStubAgent()
	if cfg.LLM.Provider != "" && cfg.LLM.APIKeyEnv != "" {
		agent = missionengine.NewStubAgent() // GoalAgent 待 LLMClient 实例化后启用
	}
	missionEng := missionengine.New(bus, agent)
	missionEng.Start()
	log.Printf(`{"level":"INFO","ts":"%s","msg":"Step 9: Mission Engine registered (StubAgent)"}`, time.Now().Format(time.RFC3339))

	// Step 10: Register Plugin Runner (扫描 plugins/ 目录，加载 Plugins)
	runner := pluginrunner.New(bus)
	runner.Start()

	// 将发现的 Plugin 能力注册到 Governance（Capability Engine 授权检查的前提）
	for _, p := range runner.DiscoveredPlugins() {
		gov.RegisterCapabilities(p.Manifest.Name, p.Manifest.DeclaredCapabilities)
	}
	// 注册内置能力（MVP 无真实 Plugin 二进制时保证核心链路可用）
	gov.RegisterCapabilities("builtin", []string{"fs.read", "fs.write", "shell.execute", "browser.open", "browser.click"})
	log.Printf(`{"level":"INFO","ts":"%s","msg":"Step 10: Plugin Runner registered (%d plugins + builtin caps)"}`, time.Now().Format(time.RFC3339), len(runner.DiscoveredPlugins()))

	// Step 11: Snapshot 冷启动恢复
	recovered, err := store.RecoverAll()
	if err != nil {
		log.Printf(`{"level":"WARN","ts":"%s","msg":"Step 11: recovery error: %v"}`, time.Now().Format(time.RFC3339), err)
	} else {
		log.Printf(`{"level":"INFO","ts":"%s","msg":"Step 11: recovered %d goals"}`, time.Now().Format(time.RFC3339), len(recovered))
	}

	// Step 12: PID file lock (flock 语义通过 O_EXCL 实现)
	pidFile := goalOSDir + "/goalos.pid"
	pidLock, err := acquirePIDLock(pidFile)
	if err != nil {
		log.Fatalf(`{"level":"FATAL","ts":"%s","msg":"Step 12: PID lock failed (daemon already running?)"}`, time.Now().Format(time.RFC3339))
	}
	defer os.Remove(pidFile)
	defer pidLock.Close()
	log.Printf(`{"level":"INFO","ts":"%s","msg":"Step 12: PID lock acquired"}`, time.Now().Format(time.RFC3339))

	// Step 13: Start Local UI Server (localhost:18920)
	api := daemon.NewHandler()
	api.SetPort(cfg.Daemon.Port)
	api.SetStartTime(startTime)
	daemon.SetEventBus(bus)
	daemon.SetStateStore(store)

	sse := daemon.NewSSEManager()
	bus.Subscribe("GoalCreated", func(evt events.Event) error { sse.Push("GoalCreated", evt.Payload); return nil })
	bus.Subscribe("GoalCompleted", func(evt events.Event) error { sse.Push("GoalCompleted", evt.Payload); return nil })
	bus.Subscribe("ActionPendingApproval", func(evt events.Event) error { sse.Push("ActionPendingApproval", evt.Payload); return nil })

	mux := http.NewServeMux()
	mux.HandleFunc("/", daemon.HandleDashboard)
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
		id := strings.TrimPrefix(r.URL.Path, "/api/goals/")
		id = strings.Split(id, "/")[0]
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
	mux.HandleFunc("/api/system/status", api.HandleSystemStatus)
	mux.HandleFunc("/api/system/stop", api.HandleDaemonStop)
	mux.HandleFunc("/api/system/restart", api.HandleDaemonRestart)

	server := &http.Server{
		Addr:    fmt.Sprintf("localhost:%d", cfg.Daemon.Port),
		Handler: mux,
	}

	go func() {
		log.Printf(`{"level":"INFO","ts":"%s","msg":"Step 13: HTTP server listening on localhost:%d"}`, time.Now().Format(time.RFC3339), cfg.Daemon.Port)
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf(`{"level":"FATAL","ts":"%s","msg":"HTTP server error: %v"}`, time.Now().Format(time.RFC3339), err)
		}
	}()

	// Step 14: Publish SystemStarted
	bus.Publish(events.Event{
		Type:    events.TypeSystemStarted,
		Source:  "daemon",
		Seq:     0,
		Payload: map[string]interface{}{
			"pid":  os.Getpid(),
			"port": cfg.Daemon.Port,
		},
	})
	log.Printf(`{"level":"INFO","ts":"%s","msg":"Step 14: SystemStarted event published"}`, time.Now().Format(time.RFC3339))

	// Wait for shutdown signal
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	<-ctx.Done()
	log.Printf(`{"level":"INFO","ts":"%s","msg":"Shutting down..."}`, time.Now().Format(time.RFC3339))

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Daemon.ShutdownTimeout)
	defer cancel()
	api.SetShutdownHook(func() { cancel() })
	server.Shutdown(shutdownCtx)

	log.Printf(`{"level":"INFO","ts":"%s","msg":"GoalOS stopped. Goodbye."}`, time.Now().Format(time.RFC3339))
}

// acquirePIDLock 通过 O_EXCL 获取 PID 文件锁（单实例保证）。
// 返回 os.File 用于后续 defer Close + Remove。
func acquirePIDLock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("acquire pid lock: %w", err)
	}
	fmt.Fprintf(f, "%d\n", os.Getpid())
	return f, nil
}
