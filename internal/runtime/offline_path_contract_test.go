//go:build darwin

// offline_path_contract_test.go——TC-RT-061（任务 8.2——离线路径验证；R-1503/R-1513）。
// AC-05 离线语义=执行阶段离线可用（已规划目标断网照常执行——协作/受限档全路径无远程依赖）。
// 真实证明形态：执行段在「网络全拒」的 OS 边界内跑通（非声称——seatbelt deny network*
// 边界内子进程成功=断网可执行的构造性证据）；签发/验签=纯密码学本地（keyring+HS256）。
// 禁止内存私钥降级断言=密钥载体=secrets.key 文件（LoadOrGenerateSecret 路径）。
// 标注=实现同步补强（非先红——诚实标注纪律）。12 清单 G 节登记。
package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/goalos/goalos/internal/governance"
)

// TestRuntime_Offline_LocalPaths（TC-RT-061——断网全路径）：
// (1)keyring 签发→验签全链路（本地密码学——无远程依赖）；
// (2)Token 签发→契约验证全链路（HS256 本地）；
// (3)Resolver 决策（纯本地判定）；
// (4)执行段=网络全拒 OS 边界内真实执行（seatbelt deny network*——断网可执行构造性证据）；
// (5)密钥载体=secrets.key 文件（0600——禁止内存私钥降级：LoadOrGenerateSecret 文件路径实证）。
func TestRuntime_Offline_LocalPaths(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()

	// (5)密钥载体=文件（禁止内存私钥降级——secrets.key 0600）
	keyPath := filepath.Join(home, "secrets.key")
	key, err := governance.LoadOrGenerateSecret(keyPath)
	if err != nil {
		t.Fatalf("密钥文件加载失败: %v", err)
	}
	if len(key) != 32 {
		t.Fatalf("密钥应=32B（文件载体），实际 %d", len(key))
	}
	fi, err := os.Stat(keyPath)
	if err != nil || fi.Mode().Perm() != 0600 {
		t.Fatalf("(5)secrets.key 载体应=文件 0600——内存降级禁止: %v mode=%v", err, fi.Mode().Perm())
	}

	// (1)keyring 签发→验签（本地密码学全链路）
	kr := governance.NewKeyring(24*time.Hour, time.Now)
	if err := kr.AddGeneration("gen-1", key); err != nil {
		t.Fatal(err)
	}
	kid, signKey, err := kr.SignKey()
	if err != nil || kid != "gen-1" {
		t.Fatalf("(1)keyring 签发材料获取失败: %v", err)
	}
	if _, err := kr.VerifyKey(kid); err != nil {
		t.Fatalf("(1)keyring 验签失败: %v", err)
	}

	// (2)Token 签发→契约验证（HS256 本地全链路）
	claims := governance.TokenClaims{
		GoalID: "g-off", ActionID: "a-off", Capabilities: []string{"shell.execute"},
		IssuedAt: time.Now().Unix(), ExpiresAt: time.Now().Unix() + 300,
		Subject: "offline", SessionID: "sess-off",
		ProfileDigest:           "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		RequiresRealEnforcement: true, MinIsolation: "I2",
		Nonce:       "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		IssuerKeyID: kid, PolicyRevision: "builtin-v1",
	}
	tok, err := governance.IssueToken(claims, signKey)
	if err != nil {
		t.Fatal(err)
	}
	verifier := NewContractVerifier(signKey, NewNonceRegistry())
	if _, err := verifier.Verify(tok); err != nil {
		t.Fatalf("(2)契约验证失败: %v", err)
	}

	// (3)Resolver 决策（本地）
	resolver := NewResolver(nil).WithPlatformMaxIsolation(DetectPlatformIsolation)
	sel, err := resolver.Resolve(ResolveInput{RequiresRealEnforcement: true, MinIsolation: I2})
	if err != nil || sel.Tier != TierRestricted {
		t.Fatalf("(3)Resolver 决策失败: %v tier=%v", err, sel.Tier)
	}

	// (4)执行段=网络全拒边界内真实执行（断网可执行构造性证据）
	p := NewDarwinSeatbeltProvider(home+"/ws", home+"/tmp")
	if err := p.Prepare(ctx, RuntimePlan{PlanID: "offline", Tier: TierRestricted}); err != nil {
		t.Fatal(err)
	}
	h, err := p.Acquire(ctx, LeaseRequest{GoalID: "g-off", ActionID: "a-off"})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.Precheck(ctx); err != nil {
		t.Fatal(err)
	}
	res, err := h.Execute(ctx, ExecuteRequest{
		ActionID: "a-off", ActionType: "process.exec",
		Params: map[string]string{"binary": "/usr/bin/true"},
	})
	if err != nil || res.Status != "success" {
		t.Fatalf("(4)断网边界内执行应成功（离线可用构造性证据），实际: status=%s err=%v", res.Status, err)
	}
	if err := h.Release(ctx); err != nil {
		t.Fatal(err)
	}
}
