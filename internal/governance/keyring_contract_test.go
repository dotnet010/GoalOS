// keyring_contract_test.go——任务 3.4 契约测试（R-571 先红；TC-RT-030/031——
// 12 清单 F 节；规格=06 §X.8 密钥纪律+R-1505 收窄+R-1389 代际窗口）。
package governance

import (
	"errors"
	"testing"
	"time"
)

// TestRuntime_KeyRotation_OverlapWindow（TC-RT-030——12 清单 F 节）：
// 密钥轮换演练——HS256+kid 代际窗口：(1)轮换后新 kid 签发/验签 (2)旧代际窗口内保留验签
// (3)unknown-kid fail-closed (4)窗口过期后旧代际拒绝。
func TestRuntime_KeyRotation_OverlapWindow(t *testing.T) {
	now := time.Now()
	clock := func() time.Time { return now }
	kr := NewKeyring(24*time.Hour, clock) // 代际窗口=24h

	// 首代注册
	if err := kr.AddGeneration("kid-1", []byte("key-material-generation-1-32bytes!")); err != nil {
		t.Fatalf("首代注册失败: %v", err)
	}
	if kid, _, err := kr.SignKey(); err != nil || kid != "kid-1" {
		t.Fatalf("活跃 kid 应=kid-1，实际 %q", kid)
	}

	// 轮换：kid-2 活跃，kid-1 保留窗口内验签
	if err := kr.Rotate("kid-2", []byte("key-material-generation-2-32bytes!")); err != nil {
		t.Fatalf("轮换失败: %v", err)
	}
	if kid, _, err := kr.SignKey(); err != nil || kid != "kid-2" {
		t.Fatalf("轮换后活跃 kid 应=kid-2，实际 %q", kid)
	}
	// 旧代际窗口内仍可验（重叠窗口）
	if _, err := kr.VerifyKey("kid-1"); err != nil {
		t.Fatalf("重叠窗口内旧代际应可验: %v", err)
	}
	// unknown-kid fail-closed
	if _, err := kr.VerifyKey("kid-unknown"); err == nil {
		t.Fatal("unknown-kid 未拒绝——fail-closed 失效")
	}
	// 窗口过期后旧代际拒绝
	now = now.Add(25 * time.Hour)
	if _, err := kr.VerifyKey("kid-1"); err == nil {
		t.Fatal("窗口过期后旧代际未拒绝")
	}
	// 活跃代际不受窗口影响
	if _, err := kr.VerifyKey("kid-2"); err != nil {
		t.Fatalf("活跃代际不应受窗口影响: %v", err)
	}
}

// TestRuntime_Keystore_NoPlaintextDisk（TC-RT-031——12 清单 F 节三子句口径 R-1557）：
// a) 载体 0600=既有 TestSecretsKey_Permissions_0600 指针（本测试不重复建设）
// b) 内存清零：keyring 密钥材料用毕清零（Zeroize 后读取=全零）
// c) 不跨进程传递：见 pluginrunner 包 env 白名单测试（TestChildEnv_Whitelist——构造侧隔离）
func TestRuntime_Keystore_NoPlaintextDisk(t *testing.T) {
	kr := NewKeyring(time.Hour, time.Now)
	if err := kr.AddGeneration("kid-1", []byte("sensitive-key-material-32-bytes!!!")); err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	// 取引用后清零
	kid, key, err := kr.SignKey()
	if err != nil || kid != "kid-1" || len(key) == 0 {
		t.Fatal("SignKey 应返回活跃代际")
	}
	kr.Zeroize()
	// 清零后：VerifyKey 不得返回明文（材料已擦除=fail-closed）
	if _, err := kr.VerifyKey("kid-1"); err == nil {
		t.Fatal("Zeroize 后旧代际仍可验——内存清零未生效")
	}
	if _, _, err := kr.SignKey(); err == nil {
		t.Fatal("Zeroize 后仍可签发——内存清零未生效")
	}
	// R-1640-4：清零后写操作 fail-closed（静默 no-op=调用方误以为注册成功——Kees 裁决）
	if err := kr.AddGeneration("kid-9", []byte("late-key-material-32-bytes!!!!!!")); !errors.Is(err, ErrKeyringZeroized) {
		t.Fatalf("Zeroize 后 AddGeneration 应=ErrKeyringZeroized，实际: %v", err)
	}
	if err := kr.Rotate("kid-9", []byte("late-key-material-32-bytes!!!!!!")); !errors.Is(err, ErrKeyringZeroized) {
		t.Fatalf("Zeroize 后 Rotate 应=ErrKeyringZeroized，实际: %v", err)
	}
}
