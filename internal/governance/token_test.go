package governance

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIssueAndVerifyToken(t *testing.T) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}

	claims := TokenClaims{
		GoalID:       "goal_001",
		ActionID:     "goal_001_act_001",
		Capabilities: []string{"fs.read", "fs.write"},
		IssuedAt:     time.Now().Unix(),
		ExpiresAt:    time.Now().Add(5 * time.Minute).Unix(),
	}

	token, err := IssueToken(claims, secret)
	if err != nil {
		t.Fatalf("IssueToken 失败: %v", err)
	}
	if token == "" {
		t.Fatal("token 为空")
	}

	// 验证 token 格式（3 段 base64url）
	parts := splitToken(token)
	if len(parts) != 3 {
		t.Fatalf("token 应有 3 段，实际 %d 段: %s", len(parts), token)
	}

	verified, err := VerifyToken(token, secret)
	if err != nil {
		t.Fatalf("VerifyToken 失败: %v", err)
	}
	if verified.GoalID != claims.GoalID {
		t.Errorf("GoalID: want %s, got %s", claims.GoalID, verified.GoalID)
	}
	if verified.ActionID != claims.ActionID {
		t.Errorf("ActionID: want %s, got %s", claims.ActionID, verified.ActionID)
	}
	if len(verified.Capabilities) != len(claims.Capabilities) {
		t.Errorf("Capabilities: want %d, got %d", len(claims.Capabilities), len(verified.Capabilities))
	}
}

func TestVerifyToken_WrongSecret(t *testing.T) {
	secret1 := make([]byte, 32)
	secret2 := make([]byte, 32)
	rand.Read(secret1)
	rand.Read(secret2)

	claims := TokenClaims{
		GoalID:    "goal_001",
		ActionID:  "goal_001_act_001",
		IssuedAt:  time.Now().Unix(),
		ExpiresAt: time.Now().Add(5 * time.Minute).Unix(),
	}

	token, err := IssueToken(claims, secret1)
	if err != nil {
		t.Fatal(err)
	}

	_, err = VerifyToken(token, secret2)
	if err == nil {
		t.Fatal("使用错误密钥验证应失败")
	}
}

func TestVerifyToken_Expired(t *testing.T) {
	secret := make([]byte, 32)
	rand.Read(secret)

	claims := TokenClaims{
		GoalID:    "goal_001",
		ActionID:  "goal_001_act_001",
		IssuedAt:  time.Now().Add(-10 * time.Minute).Unix(),
		ExpiresAt: time.Now().Add(-5 * time.Minute).Unix(),
	}

	token, err := IssueToken(claims, secret)
	if err != nil {
		t.Fatal(err)
	}

	_, err = VerifyToken(token, secret)
	if err == nil {
		t.Fatal("过期 Token 验证应失败")
	}
}

func TestVerifyToken_TamperedPayload(t *testing.T) {
	secret := make([]byte, 32)
	rand.Read(secret)

	claims := TokenClaims{
		GoalID:    "goal_001",
		ActionID:  "goal_001_act_001",
		IssuedAt:  time.Now().Unix(),
		ExpiresAt: time.Now().Add(5 * time.Minute).Unix(),
	}

	token, err := IssueToken(claims, secret)
	if err != nil {
		t.Fatal(err)
	}

	// 篡改 token（在 payload 段后添加字符）
	tampered := token + "tampered"
	_, err = VerifyToken(tampered, secret)
	if err == nil {
		t.Fatal("篡改 Token 验证应失败")
	}
}

func TestLoadOrGenerateSecret(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "secret_key")

	// 第一次：生成新密钥
	secret1, err := LoadOrGenerateSecret(keyPath)
	if err != nil {
		t.Fatalf("LoadOrGenerateSecret 失败: %v", err)
	}
	if len(secret1) != 32 {
		t.Fatalf("密钥长度应为 32，实际 %d", len(secret1))
	}

	// 确认文件已写入
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Fatal("密钥文件未写入")
	}

	// 第二次：加载已有密钥
	secret2, err := LoadOrGenerateSecret(keyPath)
	if err != nil {
		t.Fatalf("第二次 LoadOrGenerateSecret 失败: %v", err)
	}

	// 两次应返回相同密钥
	for i := range secret1 {
		if secret1[i] != secret2[i] {
			t.Fatal("两次调用应返回相同的密钥")
		}
	}
}
