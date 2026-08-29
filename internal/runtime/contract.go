// Package runtime — v0.3.1 Runtime Boundary 核心层（W1：契约验证层）。
// 规格权威：05 §X.6.3（ExecutionContract 字段表/凭据时序四句/契约边界）+
// 05 §X.6.4（Resolver 决策表）+06 §1.3（签发决策表）+07 §4.14（Runtime 族事件）。
// 契约边界（R-1501）：ExecutionContract 全文不下发到 Plugin/沙箱内——
// 契约验证只在 daemon 侧，会话建立前一次完成（验签/时效/吊销/ProfileDigest/Nonce）。
package runtime

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/goalos/goalos/internal/governance"
)

// ─── 拒绝原因（07 §4.14 ContractRejected.reject_reason 枚举）───

const (
	RejectSignatureInvalid      = "signature_invalid"
	RejectExpired               = "expired"
	RejectRevoked               = "revoked"
	RejectProfileDigestMismatch = "profile_digest_mismatch"
	RejectNonceReplayed         = "nonce_replayed"
	// RejectInvalidFields 字段畸形（v2 必填零值/格式非法——R-1628 会议 #251：
	// 签名有效但字段非法≠签名无效，语义贴切=排查指南）。
	RejectInvalidFields = "invalid_fields"
)

// ContractError 契约验证错误（携带 reject_reason——07 §4.14 枚举封闭）。
type ContractError struct {
	Reason string
	Detail string
}

func (e *ContractError) Error() string {
	return fmt.Sprintf("contract rejected: %s（%s）", e.Reason, e.Detail)
}

// IsRejectReason 判定错误是否携带指定 reject_reason（TC-RT 族断言入口）。
// errors.As 穿透包装层（执行门 fmt.Errorf %w 包装语义不破坏判定——R-1640② 落地实证）。
func IsRejectReason(err error, reason string) bool {
	var ce *ContractError
	if errors.As(err, &ce) {
		return ce.Reason == reason
	}
	return false
}

// VerifiedContract 已验证契约（导出类型+非导出字段+同包构造——R-1457 类型 3，
// 其余包不得构造"已验证"对象；R-1501 契约不过边界）。
type VerifiedContract struct {
	claims governance.TokenClaims
}

// Claims 返回契约声明只读副本（验证后消费方读字段用——非导出字段不外漏可变引用）。
func (v *VerifiedContract) Claims() governance.TokenClaims { return v.claims }

// ContractID 返回契约 ID（=goal_id/action_id 复合——R-1520 映射表：ID 含 goal 归属）。
func (v *VerifiedContract) ContractID() string {
	return v.claims.GoalID + "/" + v.claims.ActionID
}

// NonceRegistry Nonce 消费登记（R-1510 时序句①：Nonce 消费点=ExecutionSession 建立，
// 单次消费防跨会话重放；05 §X.6.3 并发所有权——本表唯一写者=验证层）。
type NonceRegistry struct {
	mu      sync.Mutex
	consumed map[string]bool
}

// NewNonceRegistry 构造空登记表。
func NewNonceRegistry() *NonceRegistry {
	return &NonceRegistry{consumed: make(map[string]bool)}
}

// tryConsume 原子消费——已消费返回 false。
func (r *NonceRegistry) tryConsume(nonce string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.consumed[nonce] {
		return false
	}
	r.consumed[nonce] = true
	return true
}

// ContractVerifier 契约验证层（daemon 侧唯一验证点——05 §X.6.3 时序四句）。
// 验证四步+重放防线：验签→时效→吊销→ProfileDigest→（消费时）Nonce。
// 零值非法（R-1106 同纪律）：v2 五字段空值/格式非法=拒绝（reject_reason=invalid_fields——R-1628①）。
type ContractVerifier struct {
	secret   []byte
	nonces   *NonceRegistry
	revoked  func(contractID string) bool // 吊销查询（治理层吊销表注入）
	onReject func(reason string)          // 事件发射点（ContractRejected——任务 1.5 接线；nil=不发射）
}

// NewContractVerifier 构造验证器。revoked/onReject 可空（吊销查询缺失=不查吊销——
// W1 范围诚实标注：吊销表生产接线随治理层；测试可注入）。
func NewContractVerifier(secret []byte, nonces *NonceRegistry, opts ...func(*ContractVerifier)) *ContractVerifier {
	v := &ContractVerifier{secret: secret, nonces: nonces}
	for _, opt := range opts {
		opt(v)
	}
	return v
}

// WithRevocationChecker 注入吊销查询。
func WithRevocationChecker(f func(string) bool) func(*ContractVerifier) {
	return func(v *ContractVerifier) { v.revoked = f }
}

// WithRevocationStore 吊销接线=治理层 TokenStore 既有机制（W2 闭合——
// 吊销=验证四步之③，非可选：tokenID 形态=goalID+"-"+actionID，R-1392 族）。
func WithRevocationStore(ts *governance.TokenStore) func(*ContractVerifier) {
	return func(v *ContractVerifier) {
		v.revoked = func(contractID string) bool {
			return ts.IsRevoked(strings.Replace(contractID, "/", "-", 1))
		}
	}
}

// WithRejectHook 注入拒绝事件发射点（ContractRejected 留痕——TC-RT-021/080）。
func WithRejectHook(f func(reason string)) func(*ContractVerifier) {
	return func(v *ContractVerifier) { v.onReject = f }
}

// v2 五字段零值非法检查（R-1106 同纪律——05 §X.6.3 字段表；R-1628：reject_reason=invalid_fields）。
func checkRequiredFields(c *governance.TokenClaims) error {
	if c.ProfileDigest == "" || c.SessionID == "" || c.MinIsolation == "" ||
		c.Nonce == "" || c.IssuerKeyID == "" {
		return &ContractError{Reason: RejectInvalidFields, Detail: "v2 必填字段零值非法（R-1106）"}
	}
	if _, err := hex.DecodeString(c.ProfileDigest); err != nil || len(c.ProfileDigest) != 64 {
		return &ContractError{Reason: RejectInvalidFields, Detail: "profile_digest 非 hex(32B)"}
	}
	if _, err := hex.DecodeString(c.Nonce); err != nil || len(c.Nonce) != 64 {
		return &ContractError{Reason: RejectInvalidFields, Detail: "nonce 非 hex(32B)"}
	}
	return nil
}

// verify 验签+时效+吊销+字段合法性（不消费 Nonce——凭据时序句③：会话内 Acquire 不重验）。
// 时效与验签分因（TC-RT-021）：过期=governance.ErrTokenExpired 哨兵映射 reject_reason=expired。
func (v *ContractVerifier) verify(tokenStr string) (*governance.TokenClaims, error) {
	claims, err := governance.VerifyToken(tokenStr, v.secret)
	if err != nil {
		if errors.Is(err, governance.ErrTokenExpired) {
			return nil, &ContractError{Reason: RejectExpired, Detail: err.Error()}
		}
		return nil, &ContractError{Reason: RejectSignatureInvalid, Detail: err.Error()}
	}
	if err := checkRequiredFields(claims); err != nil {
		return nil, err
	}
	if v.revoked != nil && v.revoked(claims.GoalID+"/"+claims.ActionID) {
		return nil, &ContractError{Reason: RejectRevoked, Detail: "契约已被吊销"}
	}
	return claims, nil
}

// Verify 只验不消费（会话内后续 Acquire——凭据时序句③）。
func (v *ContractVerifier) Verify(tokenStr string) (*VerifiedContract, error) {
	claims, err := v.verify(tokenStr)
	if err != nil {
		v.emitReject(err)
		return nil, err
	}
	return &VerifiedContract{claims: *claims}, nil
}

// VerifyWithProfile 验签+时效+吊销+ProfileDigest 比对（不消费 Nonce——
// TC-RT-080：租约获取前重算摘要，不一致默认拒绝+触发重新评估）。
func (v *ContractVerifier) VerifyWithProfile(tokenStr, currentProfileDigestHex string) (*VerifiedContract, error) {
	claims, err := v.verify(tokenStr)
	if err != nil {
		v.emitReject(err)
		return nil, err
	}
	if claims.ProfileDigest != currentProfileDigestHex {
		err := &ContractError{Reason: RejectProfileDigestMismatch,
			Detail: "ProfileDigest 不一致——默认拒绝+触发重新评估（05 §X.6.3）"}
		v.emitReject(err)
		return nil, err
	}
	return &VerifiedContract{claims: *claims}, nil
}

// VerifyAndConsume 验证+消费 Nonce（会话建立点——凭据时序句①）。
// 同一 Nonce 第二次消费=重放拒绝（TC-RT-020）。
func (v *ContractVerifier) VerifyAndConsume(tokenStr string) (*VerifiedContract, error) {
	claims, err := v.verify(tokenStr)
	if err != nil {
		v.emitReject(err)
		return nil, err
	}
	if !v.nonces.tryConsume(claims.Nonce) {
		err := &ContractError{Reason: RejectNonceReplayed,
			Detail: "Nonce 已消费——同一 Nonce 无法激活第二会话（TC-RT-020）"}
		v.emitReject(err)
		return nil, err
	}
	return &VerifiedContract{claims: *claims}, nil
}

// emitReject 发射 ContractRejected 留痕（07 §4.14——Publisher=契约验证层）。
func (v *ContractVerifier) emitReject(err error) {
	if v.onReject == nil {
		return
	}
	if ce, ok := err.(*ContractError); ok {
		v.onReject(ce.Reason)
	}
}
