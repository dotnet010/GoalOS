// degraded_banner.go — 用户可见降级警告（任务 5.6——诚实标注格式：CLI 启动横幅+
// SSE 事件流；R-1195/D60 修正——identity labels 自 DegradedItems() 数据源，供未来 UI
// 订阅；横幅 SSE 连接计入 4 上限不设豁免 F-19；CLI 本机 SSE 走 unix socket）。
//
// 契约：特性缺失时明确告知当前安全级别——诚实标注（R-1075 Go/No-Go 安全签字；
// R-1110 v0.3.0=产品版定位：诚实反馈是产品原则）。
package daemon

import "fmt"

// DegradedBanner — 用户可见降级警告（CLI 启动横幅+SSE 事件流）。
type DegradedBanner struct {
	Items []string // 降级组件清单（identity labels 自 DegradedItems() 数据源）
}

// Render — 横幅渲染（CLI 启动横幅）。
// 契约：降级组件逐条列出+当前安全级别明确告知（诚实标注格式）。
func (d *DegradedBanner) Render() string {
	if len(d.Items) == 0 {
		return ""
	}
	msg := "[降级警告] 以下组件运行于降级模式：\n"
	for _, item := range d.Items {
		msg += fmt.Sprintf("  - %s\n", item)
	}
	msg += "当前安全级别=降级模式（详见 goalos sandbox doctor）"
	return msg
}

// SSEPayload — SSE 事件流载荷（降级警告事件）。
// 契约：横幅 SSE 连接计入 4 上限不设豁免（F-19）；CLI 本机 SSE 走 unix socket。
func (d *DegradedBanner) SSEPayload() map[string]interface{} {
	return map[string]interface{}{
		"type":    "degraded_banner",
		"items":   d.Items,
		"level":   "degraded",
		"message": "当前安全级别=降级模式",
	}
}
