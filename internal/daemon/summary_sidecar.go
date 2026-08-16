// summary_sidecar.go — .summary 侧车实现（任务 7.27——D50 会议 #198，R-1133 落地载体）。
//
// 契约：schema=YAML——复用 CompletionContract 渲染字段（D41 统一命名）+artifact_metadata
// （R-1133 非文本产出物元数据）；写入方=产出方 Plugin（与产出物原子写入同目录，
// write-temp→fsync→rename 复用 08 §3.3 契约）；daemon 校验 schema。
package daemon

import "errors"

// SummarySidecar — .summary 侧车（YAML schema）。
type SummarySidecar struct {
	// 复用 CompletionContract 渲染字段（D41 统一命名）
	GoalID         string `yaml:"goal_id"`         // Goal ID
	ActionID       string `yaml:"action_id"`       // Action ID
	CompletionType string `yaml:"completion_type"` // 完成类型
	// artifact_metadata（R-1133 非文本产出物元数据）
	Metadata ArtifactMetadata `yaml:"artifact_metadata"` // 产出物元数据（R-1470——字段名同名修正）
}

// ArtifactMetadata — 非文本产出物元数据（R-1133）。
type ArtifactMetadata struct {
	MediaType string `yaml:"media_type"` // MIME 类型
	Size      int64  `yaml:"size"`       // 字节数
	Digest    string `yaml:"digest"`     // SHA-256 摘要（R-1431 Artifact struct 候选字段）
}

// ValidateSummary — daemon 侧 schema 校验（R-1185 .summary 强制 Guard）。
// 契约：schema 非法→拒绝（fail-closed）；三字段全校验（R-1470——函数名承诺范围=
// 实际检查范围：GoalID/ActionID/Digest/MediaType/Size 全部校验——值携带语义比
// 实际验证范围更宽=撒谎）。
func ValidateSummary(s *SummarySidecar) error {
	if s.GoalID == "" || s.ActionID == "" {
		return ErrInvalidSummary
	}
	if s.Metadata.Digest == "" || s.Metadata.MediaType == "" || s.Metadata.Size <= 0 {
		return ErrInvalidSummary
	}
	return nil
}

// ErrInvalidSummary — .summary 校验失败（fail-closed——R-1185 .summary 强制 Guard）。
var ErrInvalidSummary = errors.New("summary sidecar: invalid schema")
