//go:build darwin

// transport_dir_darwin.go——FD3 darwin socket 目录决算（2026-09-10 实机首跑 RED 根治——
// red-evidence/2026-09-10-fd3-darwin-sunpath-red.txt）。
// 平台约束实锤：darwin sockaddr_un.sun_path=104B（含 NUL，可用 103；linux=108）——
// macOS 临时目录基底（/var/folders/<2>/<30>/T/...，t.TempDir() 实测 ~95B）拼 socket 名
// （goalos-fd3-<label≤24>-<16hex>.sock，≤57B）必超预算 → bind(2) EINVAL。生产面同构：
// 深路径 tmpDir 同样可超（非仅测试夹具问题）。
// 决算：dir+名超预算 → 落短镜像目录（/tmp 字面量下 0700 随机目录——os.TempDir() 在
// darwin=$TMPDIR=/var/folders/... 恰是长路径病根，故必须字面量 /tmp；属主独占先决于
// socket 0600；Close 经 cleanup 连带清理）。Name() 恒为真实路径——下游真值链
// （goalos-fd3-map.txt sock= / -D FD3_SOCK_PATH 注入[EvalSymlinks 规范化]）零改动随动。
// linux 面对照=transport_dir_linux.go（直通——Landlock 语义下迁址不安全，见该文件头）。
package fd3

import (
	"os"
	"path/filepath"
)

// unixSockPathBudget darwin sun_path 可用预算（104B 含 NUL——超限 bind EINVAL 实机实锤）。
const unixSockPathBudget = 103

// listenSockDir darwin 决算：dir+sockName 超预算 → 短镜像目录（cleanup 非 nil=Close
// 连带清理面）；预算内=原目录直通（cleanup=nil——零行为差）。
func listenSockDir(dir, sockName string) (string, func(), error) {
	if len(filepath.Join(dir, sockName)) <= unixSockPathBudget {
		return dir, nil, nil
	}
	mirror, err := os.MkdirTemp("/tmp", "goalos-fd3-") // 0700 属主独占
	if err != nil {
		return "", nil, err
	}
	return mirror, func() { _ = os.Remove(mirror) }, nil
}
