// keyring.go——签名密钥族 keyring（06 §X.8 密钥两类：主密钥 vs 签名密钥族——
// 本类型=签名密钥族载体；R-1505 HS256+secrets.key keyring；R-1389 代际窗口）。
// TC-RT-030 轮换演练：HS256+kid 代际窗口——旧代际保留验至窗口过期，unknown-kid
// fail-closed。TC-RT-031b 内存清零：Zeroize 用毕清零（K-2 口径：不落盘+不跨进程+用毕清零）。
package governance

import (
	"errors"
	"sync"
	"time"
)

// ErrUnknownKeyID unknown-kid（fail-closed——keyring 唯一验签来源）。
var ErrUnknownKeyID = errors.New("keyring: unknown kid（fail-closed）")

// ErrKeyringZeroized keyring 已清零（用毕清零后任何操作=fail-closed）。
var ErrKeyringZeroized = errors.New("keyring: 已清零（用毕清零——TC-RT-031b）")

// keyEntry 代际条目。
type keyEntry struct {
	key       []byte
	createdAt time.Time
}

// Keyring 签名密钥族多代际环（代际窗口=旧代际保留验签的时长上限——R-1389）。
type Keyring struct {
	mu        sync.RWMutex
	entries   map[string]keyEntry
	activeKID string
	window    time.Duration
	now       func() time.Time
	zeroized  bool
}

// NewKeyring 构造（window=代际重叠窗口；now=时钟注入——测试可控）。
func NewKeyring(window time.Duration, now func() time.Time) *Keyring {
	return &Keyring{entries: make(map[string]keyEntry), window: window, now: now}
}

// AddGeneration 注册新代际（首次注册=活跃）。
// 清零后注册=fail-closed 返回 ErrKeyringZeroized（R-1640-4——静默 no-op 会让调用方
// 误以为注册成功，Kees：fail-closed 文化下错误的沉默不可接受）。
func (k *Keyring) AddGeneration(kid string, key []byte) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.zeroized {
		return ErrKeyringZeroized
	}
	k.entries[kid] = keyEntry{key: append([]byte{}, key...), createdAt: k.now()}
	if k.activeKID == "" {
		k.activeKID = kid
	}
	return nil
}

// Rotate 轮换（新代际=活跃；旧代际保留——窗口内可验）。
// 清零后轮换=fail-closed 返回 ErrKeyringZeroized（R-1640-4）。
func (k *Keyring) Rotate(newKID string, newKey []byte) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.zeroized {
		return ErrKeyringZeroized
	}
	k.entries[newKID] = keyEntry{key: append([]byte{}, newKey...), createdAt: k.now()}
	k.activeKID = newKID
	return nil
}

// SignKey 当前活跃签发材料（清零/无代际=fail-closed 错误）。
func (k *Keyring) SignKey() (string, []byte, error) {
	k.mu.RLock()
	defer k.mu.RUnlock()
	if k.zeroized {
		return "", nil, ErrKeyringZeroized
	}
	if k.activeKID == "" {
		return "", nil, errors.New("keyring: 无活跃代际")
	}
	e := k.entries[k.activeKID]
	return k.activeKID, append([]byte{}, e.key...), nil
}

// VerifyKey 验签材料查询：unknown-kid=fail-closed；旧代际窗口内可验；过窗口=拒绝。
func (k *Keyring) VerifyKey(kid string) ([]byte, error) {
	k.mu.RLock()
	defer k.mu.RUnlock()
	if k.zeroized {
		return nil, ErrKeyringZeroized
	}
	e, ok := k.entries[kid]
	if !ok {
		return nil, ErrUnknownKeyID
	}
	if kid != k.activeKID && k.now().After(e.createdAt.Add(k.window)) {
		return nil, errors.New("keyring: 旧代际已过代际窗口——拒绝（R-1389）")
	}
	return append([]byte{}, e.key...), nil
}

// Zeroize 用毕清零（密钥材料内存擦除——TC-RT-031b；清零后任何操作=fail-closed）。
func (k *Keyring) Zeroize() {
	k.mu.Lock()
	defer k.mu.Unlock()
	for kid, e := range k.entries {
		for i := range e.key {
			e.key[i] = 0
		}
		k.entries[kid] = e
	}
	k.entries = make(map[string]keyEntry)
	k.activeKID = ""
	k.zeroized = true
}
