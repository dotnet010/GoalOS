//go:build darwin

// seatbelt_darwin.go——macOS Seatbelt 受限档 profile 单一来源（R-1641③ 收敛落地——会议 #256）。
// 消费方双方：internal/runtime darwinSeatbeltProvider（W5 任务 5.3）+internal/pluginrunner
// executor_darwin.go（插件路径）。禁止第三处副本——漂移即事故（E2 教训：副本从不执行）。
package sandbox

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed profile_darwin_restricted.sb
var restrictedDarwinSB string

// RestrictedSeatbeltProfile 返回受限档 Seatbelt profile（Option B 语义——R-1641：
// 写禁闭+敏感目录禁读+网络禁闭+子进程禁+读全域开放；参数=WORKSPACE_DIR/TMP_DIR/HOME_DIR/
// TARGET_BINARY 四处 -D 注入；SBPL 按真实路径匹配——调用方必须先 EvalSymlinks 规范化）。
func RestrictedSeatbeltProfile() string { return restrictedDarwinSB }

// 网络授权变体锚点（单源文件内的网络节——漂移即 fail-closed）。
const networkSectionAnchor = "(deny network*)"

// RestrictedSeatbeltProfileForNetwork 网络授权变体（R-1643 裁决④——D-2 蓝图 macOS 机制适配）：
// networkAuthorized=true（契约含网络能力且审批已过——data_sharing 上游已审）时，
// 网络节从全拒替换为「默认拒出站+端口级放行 tcp 443/80」。
// 实证纪律：SBPL 无 CIDR/裸 IP 粒度（会议 #258 会前实证）——OS 层=端口面；
// 网域粒度=用户态分类器收口（cloudllm/LLM 出站）+插件 manifest network_allowlist 安装期审核面。
// fail-closed：单源锚点缺失（文件漂移）=返回错误，绝不静默产出。
func RestrictedSeatbeltProfileForNetwork(networkAuthorized bool) (string, error) {
	if !networkAuthorized {
		return restrictedDarwinSB, nil
	}
	if !strings.Contains(restrictedDarwinSB, networkSectionAnchor) {
		return "", fmt.Errorf("sandbox: 受限档 profile 网络节锚点缺失（单源漂移——fail-closed 不产出）")
	}
	variant := strings.Replace(restrictedDarwinSB, networkSectionAnchor, `;; 网络授权变体（R-1643）：默认拒出站+端口级放行（SBPL 无 CIDR 粒度——实证）
;; 网域粒度不归本层（用户态分类器收口+manifest 审核面）
(deny network-outbound)
(allow network-outbound (to tcp "*:443"))
(allow network-outbound (to tcp "*:80"))`, 1)
	return variant, nil
}

// RestrictedSeatbeltProfileFD3 FD3 中继变体（R-1650 v2 darwin 面——会议 #277 续）：
// 契约声明 network_endpoints（宿主服务中继诉求）时——网络节从全拒收窄为
// 「仅 broker unix socket 出站点对点放行」（unix socket connect 在 macOS 沙箱=
// network-outbound 操作族管辖——路径字面量收窄=mDNSResponder 族 SBPL 先例形态；
// 实参经 -D FD3_SOCK_PATH 注入）。
// 诚实标注：本变体=评审级实现——SBPL 弃用私有 API 无文档可考，unix socket
// 字面量收窄的实机验证=待 mac 实机窗口（登记诚实缺口，不虚报）；deny-then-allow
// 收窄形态=R-1643 变体实证同构。
// fail-closed：锚点缺失（单源漂移）=返回错误，绝不静默产出。
func RestrictedSeatbeltProfileFD3() (string, error) {
	if !strings.Contains(restrictedDarwinSB, networkSectionAnchor) {
		return "", fmt.Errorf("sandbox: 受限档 profile 网络节锚点缺失（单源漂移——fail-closed 不产出）")
	}
	variant := strings.Replace(restrictedDarwinSB, networkSectionAnchor, `;; FD3 变体（R-1650 v2 darwin 面）：全拒收窄=仅 broker unix socket 出站放行
	(deny network-inbound)
	(deny network-bind)
	(deny network-outbound)
	(allow network-outbound (literal (param "FD3_SOCK_PATH")))`, 1)
	return variant, nil
}
