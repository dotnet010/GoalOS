//go:build darwin
package pluginrunner
import "os/exec"
func sanitizeChildProcess(cmd *exec.Cmd) {
    // macOS: 暂时禁用 Setpgid，排查输出捕获问题
}
