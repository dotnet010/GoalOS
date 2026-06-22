// Package missionengine implements the GoalOS Mission Engine.
// 订阅 PlanRequested → 调用 Agent.plan() → 校验 MissionGraph → 发布 MissionGenerated/MissionGraphRejected。
//
// 设计依据：05 架构文档 §5、R153、R227。
package missionengine

import (
	"log"
	"strings"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/pkg/events"
)

// Agent is the planning interface.
// MVP 提供两个实现: StubAgent (硬编码 3 节点图，无 LLM 依赖)、GoalAgent (LLM 驱动)。
type Agent interface {
	Plan(goal string, ctx Context) (*MissionGraph, error)
}

// Context is the planning context. Simplified for W1.
type Context struct {
	GoalID      string
	GoalText    string
	AnchorCheck bool
}

// MissionGraph is the output of Agent.plan().
type MissionGraph struct {
	GoalID string
	Nodes  []GraphNode
	Edges  []GraphEdge
}

// GraphNode is a node in the MissionGraph.
type GraphNode struct {
	ID          string `json:"id"`
	Type        string `json:"type"`        // "mission" | "action" | "approval" | "condition" | "sub_goal" | "clarification"
	Description string `json:"description"` // 人类可读描述
	ActionType  string `json:"action_type"` // 对应的 Capability action_type（如 "web.search", "fs.read"）
	Target      string `json:"target"`      // 操作目标（搜索查询、文件路径等）
}

// GraphEdge connects two nodes.
type GraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"` // "sequential" | "parallel" | "conditional"
}

// Engine is the Mission Engine.
type Engine struct {
	bus   *eventbus.EventBus
	agent Agent
	seq   int
}

// New creates a Mission Engine.
func New(bus *eventbus.EventBus, agent Agent) *Engine {
	return &Engine{bus: bus, agent: agent}
}

// Start subscribes to PlanRequested and begins processing.
func (e *Engine) Start() {
	e.bus.Subscribe(events.TypePlanRequested, e.handlePlanRequested)
	log.Println("[MissionEngine] started, subscribed to PlanRequested")
}

func (e *Engine) handlePlanRequested(evt events.Event) error {
	goalText, _ := evt.Payload["goal_text"].(string)
	anchorCheck, _ := evt.Payload["goal_anchor_check"].(bool)

	ctx := Context{
		GoalID:      evt.GoalID,
		GoalText:    goalText,
		AnchorCheck: anchorCheck,
	}

	graph, err := e.agent.Plan(goalText, ctx)
	if err != nil {
		log.Printf("[MissionEngine] Agent.Plan failed: %v", err)
		e.publish(events.Event{
			Type:   events.TypeMissionGraphRejected,
			GoalID: evt.GoalID,
			Source: "mission-engine",
			Payload: map[string]interface{}{
				"error": err.Error(),
			},
		})
		return nil
	}

	// Validate MissionGraph
	if err := e.validate(graph); err != nil {
		log.Printf("[MissionEngine] validation failed: %v", err)
		e.publish(events.Event{
			Type:   events.TypeMissionGraphRejected,
			GoalID: evt.GoalID,
			Source: "mission-engine",
			Payload: map[string]interface{}{
				"reject_reasons": []string{err.Error()},
				"attempt":        1,
			},
		})
		return nil
	}

	// 构造节点 payload 列表（供 Scheduler 读取 action_type/target）
	nodesPayload := make([]interface{}, len(graph.Nodes))
	for i, n := range graph.Nodes {
		nodesPayload[i] = map[string]interface{}{
			"id":          n.ID,
			"type":        n.Type,
			"description": n.Description,
			"action_type": n.ActionType,
			"target":      n.Target,
		}
	}
	e.publish(events.Event{
		Type:   events.TypeMissionGenerated,
		GoalID: evt.GoalID,
		Source: "mission-engine",
		Payload: map[string]interface{}{
			"node_count": float64(len(graph.Nodes)),
			"strategy":   "GoalAgent",
			"nodes":      nodesPayload,
		},
	})

	// Also publish next event to drive state machine
	e.publish(events.Event{
		Type:   events.TypeUserConfirmed,
		GoalID: evt.GoalID,
		Source: "mission-engine",
	})

	return nil
}

func (e *Engine) validate(g *MissionGraph) error {
	if g == nil {
		return errEmptyGraph
	}
	if len(g.Nodes) == 0 {
		return errEmptyGraph
	}

	// 构建节点索引
	nodeIDs := make(map[string]bool)
	for _, n := range g.Nodes {
		if n.ID == "" {
			return &ValidationError{"节点 ID 不能为空"}
		}
		if n.Description == "" {
			return &ValidationError{"节点描述不能为空: " + n.ID}
		}
		nodeIDs[n.ID] = true
	}

	// 验证边的 from/to 引用存在性 + 边类型合法性
	validEdgeTypes := map[string]bool{"sequential": true, "parallel": true, "conditional": true, "on_completion": true, "on_failure": true}
	for _, edge := range g.Edges {
		if !nodeIDs[edge.From] {
			return &ValidationError{"边引用了不存在的源节点: " + edge.From}
		}
		if !nodeIDs[edge.To] {
			return &ValidationError{"边引用了不存在的目标节点: " + edge.To}
		}
		if edge.From == edge.To {
			return &ValidationError{"自循环边: " + edge.From}
		}
		if !validEdgeTypes[edge.Type] {
			return &ValidationError{"无效的边类型: " + edge.Type}
		}
	}

	// 拓扑排序检测循环依赖
	if hasCycle(g.Nodes, g.Edges) {
		return &ValidationError{"MissionGraph 包含循环依赖"}
	}

	return nil
}

// hasCycle 使用拓扑排序（Kahn's algorithm）检测图是否有环。
func hasCycle(nodes []GraphNode, edges []GraphEdge) bool {
	indegree := make(map[string]int)
	graph := make(map[string][]string)
	for _, n := range nodes {
		indegree[n.ID] = 0
	}
	for _, e := range edges {
		graph[e.From] = append(graph[e.From], e.To)
		indegree[e.To]++
	}

	queue := []string{}
	for id, deg := range indegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	visited := 0
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		visited++
		for _, neighbor := range graph[node] {
			indegree[neighbor]--
			if indegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	return visited != len(nodes) // 有剩余节点 → 存在环
}

func (e *Engine) publish(evt events.Event) {
	e.seq++
	evt.Seq = e.seq
	e.bus.Publish(evt)
}

// Sentinel errors for validation.
var (
	errEmptyGraph = &ValidationError{"MissionGraph is empty"}
)

// ValidationError is a MissionGraph validation error.
type ValidationError struct {
	Reason string
}

func (e *ValidationError) Error() string { return "validation: " + e.Reason }

// StubAgent 硬编码 3 节点图，用于无 LLM 环境下的核心链路测试。
// 配置 LLM Provider 后自动切换到 GoalAgent。
type StubAgent struct{}

// NewStubAgent 创建 StubAgent（默认 Agent，零外部依赖）。
func NewStubAgent() *StubAgent { return &StubAgent{} }

func (s *StubAgent) Plan(goal string, ctx Context) (*MissionGraph, error) {
	actionType, target := s.inferAction(goal)
	return &MissionGraph{
		GoalID: ctx.GoalID,
		Nodes: []GraphNode{
			{ID: "1", Type: "mission", Description: goal, ActionType: actionType, Target: target},
		},
		Edges: []GraphEdge{},
	}, nil
}

func (s *StubAgent) inferAction(goal string) (string, string) {
	// 搜索类 → web.search
	if containsAny(goal, "搜索", "search", "查找", "检索") {
		return "web.search", extractQuery(goal)
	}
	// 代码/文件生成类 → shell.execute
	if containsAny(goal, "创建", "生成", "写", "开发", "HTML", "html", "代码", "文件", "应用", "魔方", "3D", "三维", "动画", "游戏") {
		cmd := buildCreateCommand(goal)
		return "shell.execute", cmd
	}
	return "fs.read", goal
}

// buildCreateCommand 根据目标描述生成 shell 命令。
func buildCreateCommand(goal string) string {
	// 3D魔方 → 生成完整HTML文件
	if containsAny(goal, "魔方", "3D", "三维") {
		return `cat > $HOME/Goals/` + goalToFilename(goal) + ` << 'HTMLEOF'
<!DOCTYPE html>
<html><head><meta charset="UTF-8"><title>3D魔方</title>
<style>*{margin:0;padding:0;box-sizing:border-box}body{background:#1a1a2e;display:flex;flex-direction:column;align-items:center;justify-content:center;min-height:100vh;font-family:Arial}
canvas{display:block;margin:20px}
.btns{display:flex;gap:20px;margin:20px}
button{padding:12px 36px;font-size:18px;border:none;border-radius:8px;cursor:pointer;color:#fff;transition:all .3s}
.scramble{background:#e94560}.scramble:hover{background:#ff6b81}
.solve{background:#0f3460}.solve:hover{background:#1a5276}
</style></head><body>
<canvas id="cube"></canvas>
<div class="btns"><button class="scramble" onclick="scramble()">打乱</button><button class="solve" onclick="solve()">解决</button></div>
<script src="https://cdnjs.cloudflare.com/ajax/libs/three.js/r128/three.min.js"></script>
<script>
const scene=new THREE.Scene();scene.background=new THREE.Color(0x1a1a2e);
const camera=new THREE.PerspectiveCamera(45,1,0.1,100);camera.position.set(5,4,6);camera.lookAt(0,0,0);
const renderer=new THREE.WebGLRenderer({canvas:document.getElementById('cube'),antialias:true});renderer.setSize(400,400);
const light=new THREE.DirectionalLight(0xffffff,1);light.position.set(5,5,5);scene.add(light);scene.add(new THREE.AmbientLight(0x404040));
const geometry=new THREE.BoxGeometry(1,1,1);
const colors=[0xff0000,0x00ff00,0x0000ff,0xffff00,0xff8800,0xffffff];
const materials=colors.map(c=>new THREE.MeshPhongMaterial({color:c}));
const cube=new THREE.Mesh(geometry,materials);scene.add(cube);
const edges=new THREE.EdgesGeometry(geometry);scene.add(new THREE.LineSegments(edges,new THREE.LineBasicMaterial({color:0x000000})));
const originalRot={x:0,y:0,z:0};let animId,targetRot={x:0,y:0,z:0},currentRot={x:0,y:0,z:0};
function animate(){currentRot.x+=(targetRot.x-currentRot.x)*0.1;currentRot.y+=(targetRot.y-currentRot.y)*0.1;currentRot.z+=(targetRot.z-currentRot.z)*0.1;cube.rotation.set(currentRot.x,currentRot.y,currentRot.z);renderer.render(scene,camera);animId=requestAnimationFrame(animate)}
function scramble(){targetRot={x:Math.random()*Math.PI*4,y:Math.random()*Math.PI*4,z:Math.random()*Math.PI*4}}
function solve(){targetRot={x:0,y:0,z:0}}
animate();
</script></body></html>
HTMLEOF`
	}
	// 通用代码生成
	return `cat > $HOME/Goals/` + goalToFilename(goal) + ` << 'EOF'
# 根据目标生成的代码文件
# 目标: ` + goal + `
EOF`
}

func goalToFilename(goal string) string {
	name := goal
	for _, c := range []string{" ", "，", "。", "、", "：", "；"} {
		name = strings.ReplaceAll(name, c, "_")
	}
	if len(name) > 30 {
		name = name[:30]
	}
	return name + ".html"
}

func containsAny(s string, keywords ...string) bool {
	for _, kw := range keywords {
		if len(s) >= len(kw) {
			for i := 0; i <= len(s)-len(kw); i++ {
				if s[i:i+len(kw)] == kw {
					return true
				}
			}
		}
	}
	return false
}

func extractQuery(goal string) string {
	// 去掉"搜索"/"查找"等前缀词，保留核心查询
	prefixes := []string{"搜索一下", "搜索", "查找", "检索", "帮我搜索", "帮我查"}
	for _, p := range prefixes {
		if len(goal) > len(p) && goal[:len(p)] == p {
			return goal[len(p):]
		}
	}
	return goal
}
