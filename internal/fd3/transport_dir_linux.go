//go:build linux

// transport_dir_linux.go——FD3 linux socket 目录决算=直通（darwin 对照面=同族
// transport_dir_darwin.go——定义面=使用面按构建上下文对齐，U1000 纪律）。
// linux 不迁址的两条硬依据：①sun_path=108B（宽 5B）且 tmpDir 惯例为短路径（/tmp 族）；
// ②tmpDir=Landlock 已授写面——unix socket connect 需该路径写权限，静默迁址会使沙箱内
// 工作负载的 connect 越出授权目录（ Landlock 拒=故障点从 Start 期 bind 挪到运行期 connect，
// 更晚更难归因）。故 linux 保持「超限即 bind 报错」fail-loud 语义（诚实暴露优于静默迁址）。
package fd3

// listenSockDir linux 直通（语义见文件头——本平台永不迁址）。
func listenSockDir(dir, _ string) (string, func(), error) {
	return dir, nil, nil
}
