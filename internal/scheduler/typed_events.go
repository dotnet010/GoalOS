// Package scheduler — R-828: typed payload 权威来源已迁移到 pkg/events。
// 本文件保留 GoalCreatedPayload 本地定义 + type alias 指向 pkg/events。
package scheduler

import (
	"fmt"

	"github.com/goalos/goalos/internal/kernel"
	"github.com/goalos/goalos/pkg/events"
)

// 编译期验证：所有 typed payload 实现 EventPayload 接口 (R-828: alias→pkg/events)。
var (
	_ kernel.EventPayload = GoalCreatedPayload{}
	_ kernel.EventPayload = events.ActionScheduledPayload{}
	_ kernel.EventPayload = events.ActionCompletedPayload{}
	_ kernel.EventPayload = events.GoalCompletedPayloadV2{}
	_ kernel.EventPayload = events.GoalFailedPayload{}
)

// GoalCreatedPayload 是 GoalCreated 事件的 typed payload。
type GoalCreatedPayload struct {
	GoalID      string   `json:"goal_id"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

func (p GoalCreatedPayload) EventType() string { return "GoalCreated" }
func (p GoalCreatedPayload) Validate() error {
	if p.GoalID == "" {
		return fmt.Errorf("GoalCreatedPayload: GoalID is required")
	}
	if p.Title == "" {
		return fmt.Errorf("GoalCreatedPayload: Title is required (M4)")
	}
	if len(p.Title) > 10000 {
		return fmt.Errorf("GoalCreatedPayload: Title exceeds 10000 characters")
	}
	return nil
}

// ActionScheduledPayload → pkg/events (R-828 Step 2)
type ActionScheduledPayload = events.ActionScheduledPayload

// ActionCompletedPayload → pkg/events (R-828 Step 2)
type ActionCompletedPayload = events.ActionCompletedPayload

// GoalCompletedPayloadTyped → pkg/events.GoalCompletedPayloadV2 (R-828 Step 2)
type GoalCompletedPayloadTyped = events.GoalCompletedPayloadV2

// GoalFailedPayload → pkg/events (R-828 Step 2)
type GoalFailedPayload = events.GoalFailedPayload

// typedPayloadToMap → events.PayloadToMap (R-828 R4: 统一到 pkg/events)
func typedPayloadToMap(v interface{}) map[string]interface{} {
	return events.PayloadToMap(v)
}
