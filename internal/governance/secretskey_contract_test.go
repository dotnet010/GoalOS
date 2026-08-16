// 契约测试：主密钥唯一文件 ~/.goalos/secrets.key 权限 0600（R-1387）。
//
// 断言来源: R-1387（secrets.key 唯一主密钥文件（0600）；secrets.enc 导入后删除——
//   一次性演进关系；Keychain 回退 loud；损坏拒绝启动）+ R-1360（secrets.key
//   Linux/信创兜底——唯一主密钥载体）。
//
// 先红状态（2026-08-14）: 当前密钥接线（cmd/goalos/main.go Step 7）将密钥写入
//   "~/.goalos/secrets.enc"——"secrets.key" 未出现在任何配置/路径载体中 → 探针 B
//   红（本测试红的充分条件）；密钥文件创建权限 0600 已满足（LoadOrGenerateSecret
//   以 0600 写入）→ 探针 A 绿；secrets.enc 作为主密钥载体持续存在 → 探针 C 红。
//
// 转绿任务: 3.22（计划 C-2 表——secrets.key 主密钥文件迁移：唯一主密钥载体 0600；
//   secrets.enc 一次性导入后删除；config 增加 secrets 配置节承载密钥文件路径）。
//
// 契约 MUST（R-1387/R-1360）:
//   - MUST 1: 密钥文件创建权限必须为 0600（os.WriteFile 0600——可观测行为）。
//   - MUST 2: 主密钥文件载体（config secrets 节/公开常量）的有效路径必须包含
//     "secrets.key"（唯一主密钥文件——非 secrets.enc）。
//   - MUST 3: 主密钥载体不得引用 "secrets.enc" 作为主密钥文件名（secrets.enc 仅作
//     一次性导入源，导入后删除）。
//
// 安全纪律: 不得破坏真实 ~/.goalos——不调用任何强制写真实路径的 API（本测试对
//   LoadOrGenerateSecret 仅以 t.TempDir 路径调用）；真实路径契约用 reflect 探针
//   （编译安全）断言配置载体。

package governance_test

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/goalos/goalos/internal/config"
	"github.com/goalos/goalos/internal/governance"
)

func TestSecretsKey_Permissions_0600(t *testing.T) {
	gaps := 0

	// 探针 A（R-1387）: 密钥文件创建权限必须为 0600。
	keyPath := filepath.Join(t.TempDir(), "secrets.key")
	secret, err := governance.LoadOrGenerateSecret(keyPath)
	if err != nil {
		t.Fatalf("LoadOrGenerateSecret 失败: %v", err)
	}
	if len(secret) != 32 {
		t.Errorf("MUST 1（R-1387）: 密钥长度=%d，必须为 32 字节（HMAC-SHA256）", len(secret))
		gaps++
	}
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("密钥文件未创建: %v", err)
	}
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Errorf("MUST 1（R-1387）: 密钥文件权限=%o，必须为 0600（仅 daemon 属主可读写）", perm)
			gaps++
		}
	} else {
		t.Logf("Windows ACL 模型：跳过 POSIX 0600 位断言（权限经 DACL 承载——R-1387 平台×进程威胁模型）")
	}

	// 探针 B/C（R-1387/R-1360）: 主密钥文件载体必须为 "secrets.key"（非 "secrets.enc"）。
	// 反射 config.Default() 的 secrets 配置节——寻找含密钥文件名的字符串字段。
	carrier, found := findMasterKeyCarrier(t)
	if !found {
		t.Error("MUST 2（R-1387）: config 无 secrets 配置节/路径载体——主密钥唯一文件 ~/.goalos/secrets.key 未落地；当前接线仍为 secrets.enc（cmd/goalos/main.go）")
		gaps++
	} else if !strings.Contains(carrier, "secrets.key") {
		t.Errorf("MUST 2（R-1387）: 主密钥载体路径=%q，必须包含 %q（唯一主密钥文件）", carrier, "secrets.key")
		gaps++
	} else if strings.Contains(carrier, "secrets.enc") {
		t.Errorf("MUST 3（R-1387）: 主密钥载体路径=%q 引用 secrets.enc——secrets.enc 仅作一次性导入源，导入后必须删除，不得作为主密钥载体", carrier)
		gaps++
	}

	if gaps > 0 {
		t.Errorf("主密钥文件契约缺口 %d 项——R-1387（secrets.key 唯一主密钥文件 0600 + secrets.enc 一次性导入）未落地", gaps)
	}
}

// findMasterKeyCarrier 反射查找 config.Default() 中承载主密钥文件名的配置载体。
// 契约: secrets 配置节（字段名含 "secret"，大小写不敏感）内至少一个字符串字段
// 的有效值（默认值）引用密钥文件名（"secrets.key"/"secrets.enc"）。
// 返回该字段值及是否找到载体。编译安全——不依赖具体字段名。
func findMasterKeyCarrier(t *testing.T) (string, bool) {
	t.Helper()
	cfg := config.Default()
	if cfg == nil {
		return "", false
	}
	v := reflect.ValueOf(cfg).Elem()
	typ := v.Type()
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if !strings.Contains(strings.ToLower(f.Name), "secret") {
			continue
		}
		fv := v.Field(i)
		if fv.Kind() != reflect.Struct {
			continue
		}
		ft := fv.Type()
		for j := 0; j < ft.NumField(); j++ {
			sf := ft.Field(j)
			if sf.Type.Kind() != reflect.String {
				continue
			}
			val := fv.Field(j).String()
			if val != "" && (strings.Contains(val, "secrets.key") || strings.Contains(val, "secrets.enc")) {
				return val, true
			}
		}
	}
	return "", false
}
