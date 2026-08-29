//go:build darwin

// seatbelt_darwin.go——macOS Seatbelt 受限档 profile 单一来源（R-1641③ 收敛落地——会议 #256）。
// 消费方双方：internal/runtime darwinSeatbeltProvider（W5 任务 5.3）+internal/pluginrunner
// executor_darwin.go（插件路径）。禁止第三处副本——漂移即事故（E2 教训：副本从不执行）。
package sandbox

import _ "embed"

//go:embed profile_darwin_restricted.sb
var restrictedDarwinSB string

// RestrictedSeatbeltProfile 返回受限档 Seatbelt profile（Option B 语义——R-1641：
// 写禁闭+敏感目录禁读+网络禁闭+子进程禁+读全域开放；参数=WORKSPACE_DIR/TMP_DIR/HOME_DIR/
// TARGET_BINARY 四处 -D 注入；SBPL 按真实路径匹配——调用方必须先 EvalSymlinks 规范化）。
func RestrictedSeatbeltProfile() string { return restrictedDarwinSB }
