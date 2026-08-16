package sandbox

import (
	"bytes"
	"encoding/binary"
)

// canonical_key.go — 复合数据线性化共享工具（R-1460——会议 #218 发现 19）。
//
// 契约：长度前缀编码——不依赖"某个字节永远不出现"的域假设，对任意字节内容无歧义。
// 四处场景收敛：HMAC 行拼接/CRC32 附加位置/prevHash 拼接/CacheKey——统一用本函数。

// CanonicalKey — 多个字段合并为无歧义的扁平字符串（长度前缀编码）。
// 格式：[4字节 uint32 长度][内容] × N 字段——长度前缀消除歧义（不依赖分隔符域假设）。
func CanonicalKey(fields ...string) string {
	var buf bytes.Buffer
	for _, f := range fields {
		binary.Write(&buf, binary.BigEndian, uint32(len(f)))
		buf.WriteString(f)
	}
	return buf.String()
}
