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

	"github.com/goalos/goalos/internal/daemon"
	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/governance"
	"github.com/goalos/goalos/internal/missionengine"
	"github.com/goalos/goalos/internal/scheduler"
	"github.com/goalos/goalos/internal/statestore"
	"github.com/goalos/goalos/pkg/events"
)

const (
	defaultPort       = 18920
	defaultConfigPath = ".goalos/config/daemon.yaml"
	defaultEventsDir  = ".goalos/events"
	defaultLogsDir    = ".goalos/logs"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("[Daemon] GoalOS starting...")

	// Step 1: Ensure ~/.goalos/ exists, create full directory tree
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("[Daemon] cannot find home dir: %v", err)
	}
	goalOSDir := home + "/.goalos"

	dirs := []string{
		goalOSDir,
		goalOSDir + "/events",
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
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			log.Fatalf("[Daemon] create dir %s: %v", d, err)
		}
	}
	log.Println("[Daemon] Step 1: directory tree created")

	// Step 2: Load config (MVP: defaults)
	cfg := defaultConfig()
	log.Printf("[Daemon] Step 2: config loaded (port=%d)", cfg.Port)

	// Step 3: Create Event Bus
	bus := eventbus.New()
	log.Println("[Daemon] Step 3: Event Bus created")

	// Step 4: Create Logger
	logFile, err := os.OpenFile(goalOSDir+"/logs/daemon.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		log.Printf("[Daemon] Step 4: log file creation failed (using stderr): %v", err)
	} else {
		log.SetOutput(logFile)
		log.Println("[Daemon] Step 4: logger initialized")
	}

	// Step 5: Create State Store
	store := statestore.New(goalOSDir + "/events")
	log.Println("[Daemon] Step 5: State Store initialized")

	// Step 6: Register Scheduler (state machine driver)
	sched := scheduler.New(bus, store)
	sched.Start()
	log.Println("[Daemon] Step 6: Scheduler registered")

	// Step 7: Register Governance (execution gate)
	gov := governance.New(bus)
	gov.Start()
	log.Println("[Daemon] Step 7: Governance registered")

	// Step 8: Register Context Engine (W1 stub)
	log.Println("[Daemon] Step 8: Context Engine (W9-W10)")

	// Step 9: Register Mission Engine (with W1 StubAgent)
	stub := &missionengine.StubAgent{}
	missionEng := missionengine.New(bus, stub)
	missionEng.Start()
	log.Println("[Daemon] Step 9: Mission Engine registered (StubAgent)")

	// Step 10: Register Plugin Runner (W1 stub)
	log.Println("[Daemon] Step 10: Plugin Runner (W3-W4)")

	// Step 11: Snapshot 冷启动恢复
	results, err := store.RecoverAll(); if err != nil { log.Printf("[Daemon] Step 11: recovery error: %v", err) } else { log.Printf("[Daemon] Step 11: recovered %d goals", len(results)) }

	// Step 12: PID file lock
	pidFile := goalOSDir + "/goalos.pid"
	if err := acquirePIDLock(pidFile); err != nil {
		log.Fatalf("[Daemon] Step 12: PID lock failed: %v (daemon already running?)", err)
	}
	defer os.Remove(pidFile)
	log.Println("[Daemon] Step 12: PID lock acquired")

	// Step 13: Start Local UI Server
	mux := http.NewServeMux()
	api := daemon.NewHandler()
	daemon.SetEventBus(bus); sse := daemon.NewSSEManager(); bus.Subscribe("GoalCreated", func(evt events.Event) error { sse.Push("GoalCreated", evt.Payload); return nil }); bus.Subscribe("GoalCompleted", func(evt events.Event) error { sse.Push("GoalCompleted", evt.Payload); return nil }); bus.Subscribe("ActionPendingApproval", func(evt events.Event) error { sse.Push("ActionPendingApproval", evt.Payload); return nil }); mux.HandleFunc("/", daemon.HandleDashboard); mux.HandleFunc("/api/events", sse.HandleSSE)
	// shutdown hook 在 signal handler 中设置（cancel 在后面定义）
	mux.HandleFunc("/api/health", api.HandleHealth)
	mux.HandleFunc("/api/goals", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost { api.HandleCreateGoal(w, r) } else { api.HandleListGoals(w, r) }
	})
	mux.HandleFunc("/api/goals/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/goals/")
		id = strings.Split(id, "/")[0]
		r.SetPathValue("id", id)
		switch {
		case strings.HasSuffix(r.URL.Path, "/pause"): api.HandlePauseGoal(w, r)
		case strings.HasSuffix(r.URL.Path, "/resume"): api.HandleResumeGoal(w, r)
		case strings.HasSuffix(r.URL.Path, "/stop"): api.HandleStopGoal(w, r)
		case strings.HasSuffix(r.URL.Path, "/log"): api.HandleGoalLog(w, r)
		case strings.HasSuffix(r.URL.Path, "/events"): api.HandleGoalEvents(w, r)
		default: api.HandleGetGoal(w, r)
		}
	})
	mux.HandleFunc("/api/system/status", api.HandleSystemStatus)
	mux.HandleFunc("/api/system/stop", api.HandleDaemonStop)
	mux.HandleFunc("/api/system/restart", api.HandleDaemonRestart)

	server := &http.Server{
		Addr:    fmt.Sprintf("localhost:%d", cfg.Port),
		Handler: mux,
	}

	go func() {
		log.Printf("[Daemon] Step 13: HTTP server listening on localhost:%d", cfg.Port)
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("[Daemon] HTTP server error: %v", err)
		}
	}()

	// Step 14: Publish SystemStarted
	bus.Publish(events.Event{
		Type:   events.TypeSystemStarted,
		Source: "daemon",
		Seq:    0,
	})
	log.Println("[Daemon] Step 14: SystemStarted event published")

	// Wait for shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	<-ctx.Done()
	log.Println("[Daemon] Shutting down...")

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	api.SetShutdownHook(func() { cancel() })
	server.Shutdown(shutdownCtx)

	log.Println("[Daemon] GoalOS stopped. Goodbye.")
}

type config struct {
	Port            int
	ShutdownTimeout time.Duration
}

func defaultConfig() *config {
	return &config{
		Port:            defaultPort,
		ShutdownTimeout: 5 * time.Second,
	}
}

func acquirePIDLock(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("acquire pid lock: %w", err)
	}
	fmt.Fprintf(f, "%d\n", os.Getpid())
	return f.Close()
}

// healthHandler implements GET /api/health.
func healthHandler(bus *eventbus.EventBus, store *statestore.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","pid":%d}`, os.Getpid())
	}
}

// goalsHandler implements POST /api/goals (W1 skeleton).
func goalsHandler(bus *eventbus.EventBus, store *statestore.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":{"code":"INVALID_REQUEST","message":"method not allowed"}}`, http.StatusMethodNotAllowed)
			return
		}

		// W1: accept JSON body {"goal":"..."}, publish GoalCreated, return goal_id
		var body struct{ Goal string `json:"goal"` }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Goal == "" {
			http.Error(w, `{"error":{"code":"INVALID_REQUEST","message":"goal is required"}}`, http.StatusBadRequest)
			return
		}
		goalText := body.Goal

		goalID := generateGoalID()
		evt := events.Event{
			Type:   events.TypeGoalCreated,
			GoalID: goalID,
			Source: "daemon",
			Seq:    1,
			Payload: map[string]interface{}{
				"title":       goalText,
				"description": goalText,
			},
		}
		store.Append(goalID, evt)
		bus.Publish(evt)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, `{"goal_id":"%s","status":"created"}`, goalID)
	}
}

var goalCounter int

func generateGoalID() string {
	goalCounter++
	return fmt.Sprintf("goal_%03d", goalCounter)
}
