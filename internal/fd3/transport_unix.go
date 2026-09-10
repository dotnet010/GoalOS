//go:build linux || darwin

// transport_unix.go——FD3 Linux/darwin 传输层：unix socket（单连接全双工——无 Windows
// 命名管道同步句柄坑，无需双单向对）。
//
// 与 Windows 面的形态差异（设计=开发计划/fd3-broker-设计.md 注记成文）：
// 模式 B（免 userns——R-1664）seccomp 白名单放行 AF_UNIX——沙箱内进程**直连**
// 宿主 unix socket，无 AC 回环隔离问题（那是 WinAC 特有），故 Linux 面**无 fd3d
// 转发器**——broker 监听 unix socket（落 tmpDir=Landlock 已授写权面——connect
// unix socket 需该路径写权限），沙箱内工作负载直接 connect。
// 透明性差异诚实标注：Windows=回环端口同端口映射（agent 无感知）；Linux=unix
// socket 路径（工作负载经 goalos-fd3-map 知悉 socket 路径——非回环透明）。
package fd3

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// dialTimeoutUnix unix socket 连接超时（本地回环族——毫秒级即成，超时=路径坏）。
const dialTimeoutUnix = 3 * time.Second

// Listener FD3 unix socket 监听器（路径熵化一次性——R-1667 v2 同族纪律）。
type Listener struct {
	base       string // unix socket 路径（无 -req/-resp 后缀——单连接全双工）
	ln         net.Listener
	once       sync.Once
	cleanupDir func() // 非 nil=镜像目录连带清理面（darwin 短镜像决算——transport_dir_darwin.go）
}

// Name socket 路径（Dial 侧寻址用）。
func (l *Listener) Name() string { return l.base }

// Listen 在指定目录建 unix socket 监听（dir=tmpDir——Landlock 已授写权面）。
// 名=goalos-fd3-<label 净化>-<64bit 熵>.sock（一次性——残留 socket 文件不复用）。
// 实际落点经 listenSockDir 平台决算（darwin sun_path=104B 预算超限→短镜像目录——
// 2026-09-10 实机 RED 根治；Name() 恒为真实路径，下游真值链零改动随动）。
func Listen(dir, label string) (*Listener, error) {
	var entropy [8]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return nil, err
	}
	name := fmt.Sprintf("goalos-fd3-%s-%s.sock", sanitizeLabel(label), hex.EncodeToString(entropy[:]))
	sockDir, cleanup, err := listenSockDir(dir, name)
	if err != nil {
		return nil, fmt.Errorf("fd3: socket 目录决算失败: %w", err)
	}
	path := filepath.Join(sockDir, name)
	// 清理可能存在的陈旧同名文件（熵名下不应存在——防御性）
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		return nil, fmt.Errorf("fd3: unix 监听 %s: %w", path, err)
	}
	// 权限收窄（0600——仅属主可连；沙箱内进程=同用户=可连）
	if err := os.Chmod(path, 0600); err != nil {
		ln.Close()
		if cleanup != nil {
			cleanup()
		}
		return nil, fmt.Errorf("fd3: socket 权限收窄失败: %w", err)
	}
	return &Listener{base: path, ln: ln, cleanupDir: cleanup}, nil
}

// Accept 接受一连接。
func (l *Listener) Accept() (*Conn, error) {
	c, err := l.ln.Accept()
	if err != nil {
		return nil, err
	}
	return &Conn{rw: c}, nil
}

// Close 监听关闭+socket 文件清理（幂等）；darwin 镜像目录决算时连带清理镜像（零残留）。
func (l *Listener) Close() error {
	var err error
	l.once.Do(func() {
		if l.ln != nil {
			err = l.ln.Close()
		}
		_ = os.Remove(l.base)
		if l.cleanupDir != nil {
			l.cleanupDir()
		}
	})
	return err
}

// Dial 客户端连接 unix socket。
func Dial(path string) (*Conn, error) {
	c, err := net.DialTimeout("unix", path, dialTimeoutUnix)
	if err != nil {
		return nil, fmt.Errorf("fd3: unix 连接 %s: %w", path, err)
	}
	return &Conn{rw: c}, nil
}

// Conn unix 连接（全双工单句柄——读/写同一连接）。
type Conn struct {
	rw net.Conn
}

// ReadFrame 读帧。
func (c *Conn) ReadFrame() (Frame, error) { return ReadFrame(c.rw) }

// WriteFrame 写帧。
func (c *Conn) WriteFrame(f Frame) error { return WriteFrame(c.rw, f) }

// Close 关连接。
func (c *Conn) Close() error { return c.rw.Close() }
