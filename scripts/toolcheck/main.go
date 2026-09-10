// toolcheck——CI 检查脚本的原生工具面（会议 #280——R-1666 同族纪律延伸至检查链）：
// python3 依赖全退役（WindowsApps Store stub=exit 49 静默死——本地测不出、推到
// CI 才见红的断层实机事故=v0.3.3 yaml 转义首红）。Go 实现=本地与 CI 同工具链
// 同行为（Go 项目=Go 工具链必在场），无运行时外部依赖。
//
// 子命令：
//   stoken <resolutions.yaml>   S-token 完备性（desc 字段锚定扫描——F-16：
//                               整文件 grep 假绿禁止，必须真解析取 desc 拼接扫）
//   jsonfield <file> <field>    JSON 顶层字段提取（check-plugin-protocol 用）
//   jsonvalidate                stdin JSON 合法性（合法 exit 0）
//   jsonnohmac                  stdin JSON 合法且顶层无 "hmac" 键（旧协议断言）
//   senswrite [roots...]        测试夹具敏感路径写入静态拦截（会议 #282——原
//                               check-sensitive-path-write.sh 三段判定的 Go 移植；
//                               默认根=internal cmd pkg test）
//
// 输出协议与 bash 侧既有解析器逐字节兼容（COVERAGE:/MISSING:/PARSE_ERROR:）。
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: toolcheck <stoken|jsonfield|jsonvalidate|jsonnohmac|senswrite> ...")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "stoken":
		stokenMain()
	case "jsonfield":
		jsonfieldMain()
	case "jsonvalidate":
		jsonValidateMain(false)
	case "jsonnohmac":
		jsonValidateMain(true)
	case "senswrite":
		senswriteMain()
	default:
		fmt.Fprintln(os.Stderr, "unknown subcommand:", os.Args[1])
		os.Exit(2)
	}
}

// stokenMain S 决议 token 完备性（S'-31+F-16——python3+PyYAML 原实现的 Go 逐语义移植）。
func stokenMain() {
	if len(os.Args) < 3 {
		fmt.Println("PARSE_ERROR:usage: toolcheck stoken <yaml>")
		return
	}
	data, err := os.ReadFile(os.Args[2])
	if err != nil {
		fmt.Println("PARSE_ERROR:" + err.Error())
		return
	}
	var doc map[string]interface{}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		fmt.Println("PARSE_ERROR:" + err.Error())
		return
	}
	res, ok := doc["resolutions"].(map[string]interface{})
	if !ok {
		fmt.Println("PARSE_ERROR:resolutions top-level key missing or wrong type")
		return
	}
	var descs []string
	for _, v := range res {
		if m, ok := v.(map[string]interface{}); ok {
			if d, ok := m["desc"].(string); ok {
				descs = append(descs, d)
			}
		}
	}
	joined := strings.Join(descs, "\n")

	var tokens []string
	for n := 1; n <= 48; n++ {
		tokens = append(tokens, fmt.Sprintf("S-%02d", n))
	}
	for n := 1; n <= 39; n++ {
		tokens = append(tokens, fmt.Sprintf("S'-%02d", n))
	}
	var missing []string
	for _, tok := range tokens {
		// whole-token 匹配（S-01 不得命中 S-010/S'-01 内部——python 版 lookbehind
		// (?<![\w'-]) 的 Go 移植：RE2 无 lookbehind=边界手工判定）
		pat := regexp.MustCompile(regexp.QuoteMeta(tok))
		for _, loc := range pat.FindAllStringIndex(joined, -1) {
			before := byte(0)
			if loc[0] > 0 {
				before = joined[loc[0]-1]
			}
			after := byte(0)
			if loc[1] < len(joined) {
				after = joined[loc[1]]
			}
			beforeBad := isWordByte(before) || before == '\'' || before == '-'
			afterBad := isWordByte(after)
			if !beforeBad && !afterBad {
				goto found
			}
		}
		missing = append(missing, tok)
	found:
	}
	fmt.Printf("COVERAGE:total=%d:missing=%d\n", len(tokens), len(missing))
	for _, m := range missing {
		fmt.Println("MISSING:" + m)
	}
}

func isWordByte(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// jsonfieldMain JSON 顶层字段提取（check-plugin-protocol.sh 的 python3 -c json 替代）。
func jsonfieldMain() {
	if len(os.Args) < 4 {
		os.Exit(1)
	}
	data, err := os.ReadFile(os.Args[2])
	if err != nil {
		os.Exit(1)
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(data, &doc); err != nil {
		os.Exit(1)
	}
	v, ok := doc[os.Args[3]]
	if !ok {
		os.Exit(1)
	}
	fmt.Println(v)
}

// senswriteMain 测试夹具敏感路径写入静态拦截（会议 #282——check-sensitive-path-write.sh
// 三段判定的 Go 逐语义移植；事故史=WinAC Boundary(2) os.WriteFile(真实 ~/.ssh/config)
// 毁损用户配置——测试可跑在任何人的机器上，夹具永不写真实用户既有文件）。
// 判定（bash 原版语义逐字保持）：
//  (1)文件含敏感目录构造字面量（".ssh"/".aws"/".gnupg"）
//  (2)文件含敏感终段字面量行（"config"/"id_*"/"credentials"/".gitconfig"/".netrc"/
//    "known_hosts"）且行无豁免标记（goalos- 夹具名 / safefixture: 注释）
//  (3)文件含写入动词（os.WriteFile/os.Create/os.MkdirAll/os.Remove/os.RemoveAll）
// 同文件(1)(2)(3)全命中=红（exit 1）；纯分类/纯读取（无写入动词）=合法通过。
func senswriteMain() {
	roots := os.Args[2:]
	if len(roots) == 0 {
		roots = []string{"internal", "cmd", "pkg", "test"}
	}
	fmt.Println("=== check-sensitive-path-write: 测试夹具敏感路径写入扫描（三段判定） ===")

	failed := 0
	for _, root := range roots {
		dirEntries, err := readDirRecursive(root)
		if err != nil {
			if os.IsNotExist(err) {
				continue // 根缺席（如 cmd/goalos-cli 无 *_test.go 扫描面）=非失败
			}
			fmt.Fprintln(os.Stderr, "senswrite: 扫描错误:", err)
			os.Exit(2)
		}
		for _, f := range dirEntries {
			if !strings.HasSuffix(f, "_test.go") {
				continue
			}
			if checkSensitiveTestFile(f) {
				failed = 1
			}
		}
	}
	if failed == 0 {
		fmt.Println("✅ 通过——测试夹具零触碰真实敏感条目")
	}
	os.Exit(failed)
}

var (
	// (1)敏感目录构造字面量（grep '"\.(ssh|aws|gnupg)"' 语义——双引号包裹）。
	reSensDir = regexp.MustCompile(`"\.(ssh|aws|gnupg)"`)
	// (2)敏感终段字面量（grep '"(config|id_[a-zA-Z0-9_]*|credentials|\.gitconfig|\.netrc|known_hosts)"'）。
	reSensTerm = regexp.MustCompile(`"(config|id_[a-zA-Z0-9_]*|credentials|\.gitconfig|\.netrc|known_hosts)"`)
	// (3)写入动词（grep 'os\.(WriteFile|Create|MkdirAll|Remove|RemoveAll)\('）。
	reWriteOp = regexp.MustCompile(`os\.(WriteFile|Create|MkdirAll|Remove|RemoveAll)\(`)
)

// checkSensitiveTestFile 单文件三段判定（与 bash 版一致：(1)=文件级，(2)=行级+豁免，
// (3)=文件级）。命中=打印违规行并返回 true。
func checkSensitiveTestFile(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	content := string(data)
	if !reSensDir.MatchString(content) {
		return false // (1)不满足=无需走(2)(3)
	}
	if !reWriteOp.MatchString(content) {
		return false // (3)不满足=纯分类/纯读取=合法（反证迭代：写入动词与构造同在场才红）
	}
	// (2)逐行：敏感终段行且无豁免标记
	hits := []string{}
	for i, line := range strings.Split(content, "\n") {
		if !reSensTerm.MatchString(line) {
			continue
		}
		if strings.Contains(line, "goalos-") || strings.Contains(line, "safefixture:") {
			continue // 豁免（goalos- 专用夹具名 / safefixture: 注释）
		}
		hits = append(hits, fmt.Sprintf("%d: %s", i+1, strings.TrimSpace(line)))
	}
	if len(hits) == 0 {
		return false
	}
	fmt.Printf("❌ FAIL: %s\n", path)
	for _, h := range hits[:min(len(hits), 5)] {
		fmt.Println("  " + h)
	}
	fmt.Println("  → 同文件含写入动词+敏感终段构造——夹具须用 goalos- 专用名或 safefixture: 注释豁免")
	return true
}

// readDirRecursive 递归列出目录下文件（filepath.WalkDir 包装）。
func readDirRecursive(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	return files, err
}

// jsonValidateMain stdin JSON 校验（+可选无 hmac 键断言——旧协议残留检查）。
func jsonValidateMain(noHmac bool) {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(1)
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(data, &doc); err != nil {
		os.Exit(1)
	}
	if noHmac {
		if _, has := doc["hmac"]; has {
			os.Exit(1)
		}
	}
	os.Exit(0)
}
