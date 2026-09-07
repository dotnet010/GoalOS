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
//
// 输出协议与 bash 侧既有解析器逐字节兼容（COVERAGE:/MISSING:/PARSE_ERROR:）。
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: toolcheck <stoken|jsonfield|jsonvalidate|jsonnohmac> ...")
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
