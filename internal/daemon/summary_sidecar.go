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
	ArtifactMetadata ArtifactMetadata `yaml:"artifact_metadata"` // 产出物元数据
}

// ArtifactMetadata — 非文本产出物元数据（R-1133）。
type ArtifactMetadata struct {
	MediaType string `yaml:"media_type"` // MIME 类型
	Size      int64  `yaml:"size"`       // 字节数
	Digest    string `yaml:"digest"`     // SHA-256 摘要（R-1431 Artifact struct 候选字段）
}

// ValidateSummary — daemon 侧 schema 校验（R-1185 .summary 强制 Guard）。
// 契约：schema 非法→拒绝（fail-closed）；artifact_metadata.digest 非空（R-1431）。
func ValidateSummary(s *SummarySidecar) error {
	if s.GoalID == "" || s.ActionID == "" {
		return ErrInvalidSummary
	}
	if s.ArtifactMetadata.Digest == "" {
		return ErrInvalidSummary
	}
	return nil
}

// ErrInvalidSummary — .summary 校验失败（fail-closed——R-1185 .summary 强制 Guard）。
var ErrInvalidSummary = errors.New("summary sidecar: invalid schema")
