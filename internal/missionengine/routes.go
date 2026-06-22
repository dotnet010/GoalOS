// Package missionengine — StubAgent 路由规则（MVP 过渡方案）。
// W3 目标：GoalAgent + LLM 推理替代关键词匹配。
// 路由规则通过配置文件加载，不硬编码在 Agent 中。
package missionengine

import (
	"fmt"
	"log"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// RouteRule 定义一条关键词→Plugin 路由规则。
type RouteRule struct {
	Keywords    []string `yaml:"keywords"`     // 触发关键词
	ActionType  string   `yaml:"action_type"`  // 目标 Plugin
	Description string   `yaml:"description"`  // 规则说明
}

// RouteConfig 路由配置。
type RouteConfig struct {
	DefaultAction string      `yaml:"default_action"` // 无匹配时的默认 action_type
	Rules         []RouteRule `yaml:"rules"`
}

// DefaultRoutes 返回内置默认路由规则（MVP 硬编码后备）。
func DefaultRoutes() *RouteConfig {
	return &RouteConfig{
		DefaultAction: "fs.read",
		Rules: []RouteRule{
			{Keywords: []string{"搜索", "search", "查找", "检索"}, ActionType: "web.search", Description: "搜索类目标"},
			{Keywords: []string{"创建", "生成", "写", "开发", "HTML", "代码", "文件", "应用", "3D", "三维", "动画", "游戏"}, ActionType: "shell.execute", Description: "创建/生成类目标"},
		},
	}
}

// LoadRoutes 从配置文件加载路由规则。文件不存在时使用默认规则。
func LoadRoutes(path string) *RouteConfig {
	if path == "" {
		return DefaultRoutes()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("[routes] 配置文件 %s 不存在，使用默认路由规则", path)
		return DefaultRoutes()
	}
	var cfg RouteConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Printf("[routes] 配置文件解析失败: %v，使用默认路由规则", err)
		return DefaultRoutes()
	}
	if cfg.DefaultAction == "" {
		cfg.DefaultAction = "fs.read"
	}
	return &cfg
}

// Match 根据目标文本匹配路由规则。返回 action_type。
func (cfg *RouteConfig) Match(goal string) string {
	for _, rule := range cfg.Rules {
		for _, kw := range rule.Keywords {
			if len(goal) >= len(kw) {
				for i := 0; i <= len(goal)-len(kw); i++ {
					if goal[i:i+len(kw)] == kw {
						return rule.ActionType
					}
				}
			}
		}
	}
	return cfg.DefaultAction
}

// MatchWithTarget 匹配路由规则并生成 target。
func (cfg *RouteConfig) MatchWithTarget(goal string) (string, string) {
	actionType := cfg.Match(goal)
	target := goal
	if actionType == "web.search" {
		target = extractSearchQuery(goal)
	} else if actionType == "shell.execute" {
		target = fmt.Sprintf("cat > output/goalos_task.html << 'GOALEOF'\n<!DOCTYPE html><html><head><meta charset=\"UTF-8\"><title>GoalOS Task</title></head><body><h1>%s</h1><p>此文件由 GoalOS 系统自动生成。任务已路由到 shell.execute Plugin。</p></body></html>\nGOALEOF", goal)
	}
	return actionType, target
}

func extractSearchQuery(goal string) string {
	prefixes := []string{"搜索一下", "搜索", "查找", "检索", "帮我搜索", "帮我查"}
	lower := strings.ToLower(goal)
	for _, p := range prefixes {
		if len(goal) > len(p) && strings.HasPrefix(goal, p) {
			return goal[len(p):]
		}
		if strings.HasPrefix(lower, p) {
			return goal[len(p):]
		}
	}
	return goal
}
