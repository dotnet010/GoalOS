package llm

import (
	"github.com/sashabaranov/go-openai/jsonschema"
)

// PlanGoalParams 定义 plan_goal 函数的 JSON Schema 参数。
// 通过 jsonschema.GenerateSchemaForType 自动生成 Function Calling 的 parameters 定义。
// 替代原先手写的 map[string]interface{} 方式。
//
// 设计依据：R244（jsonschema 结构化输出）。
type PlanGoalParams struct {
	Nodes []PlanGoalNode `json:"nodes" jsonschema_description:"任务节点列表，每个节点表示一个可执行步骤"`
	Edges []PlanGoalEdge `json:"edges" jsonschema_description:"节点之间的依赖关系边"`
}

// PlanGoalNode 是 MissionGraph 中单个节点的 JSON Schema 定义。
type PlanGoalNode struct {
	ID          string `json:"id" jsonschema_description:"节点唯一标识符（数字字符串，如 '1', '2'）"`
	Type        string `json:"type" jsonschema:"enum=mission,enum=action,enum=approval,enum=condition,enum=sub_goal,enum=clarification" jsonschema_description:"节点类型：mission-任务，action-动作，approval-审批，condition-条件，sub_goal-子目标，clarification-澄清"`
	Description string `json:"description" jsonschema_description:"人类可读的任务描述"`
	ActionType  string `json:"action_type" jsonschema:"enum=shell.execute,enum=web.search,enum=fs.read,enum=fs.write,enum=browser.open,enum=browser.click" jsonschema_description:"执行动作类型"`
	Target      string `json:"target" jsonschema_description:"操作目标：shell 命令、搜索查询、文件路径或 URL"`
}

// PlanGoalEdge 是 MissionGraph 中边的 JSON Schema 定义。
type PlanGoalEdge struct {
	From string `json:"from" jsonschema_description:"源节点 ID"`
	To   string `json:"to" jsonschema_description:"目标节点 ID"`
	Type string `json:"type" jsonschema:"enum=sequential,enum=parallel,enum=conditional,enum=on_completion,enum=on_failure" jsonschema_description:"边类型：sequential-顺序，parallel-并行，conditional-条件，on_completion-完成时，on_failure-失败时"`
}

// GeneratePlanSchema 生成 plan_goal 函数的 JSON Schema。
// 使用 go-openai 的 jsonschema 包从 Go struct 自动生成，确保类型安全。
func GeneratePlanSchema() *jsonschema.Definition {
	schema, err := jsonschema.GenerateSchemaForType(PlanGoalParams{})
	if err != nil {
		// 降级：返回一个最基本的 schema
		return &jsonschema.Definition{
			Type: jsonschema.Object,
			Properties: map[string]jsonschema.Definition{
				"nodes": {Type: jsonschema.Array},
				"edges": {Type: jsonschema.Array},
			},
			Required: []string{"nodes"},
		}
	}
	return schema
}

// ParsePlanResponse 从 LLM 响应中解析 MissionGraph。
// 使用 jsonschema 的 Unmarshal 方法，相比手写 JSON 解析更可靠。
// 返回解析后的 PlanGoalParams，由调用者转换为 MissionGraph。
func ParsePlanResponse(content string) (*PlanGoalParams, error) {
	schema := GeneratePlanSchema()
	var result PlanGoalParams
	if err := schema.Unmarshal(content, &result); err != nil {
		return nil, err
	}
	if len(result.Nodes) == 0 {
		return nil, &PlanParseError{Reason: "MissionGraph 无节点"}
	}
	return &result, nil
}

// PlanParseError 表示 MissionGraph JSON 解析失败。
type PlanParseError struct {
	Reason string
}

func (e *PlanParseError) Error() string {
	return "plan parse: " + e.Reason
}
