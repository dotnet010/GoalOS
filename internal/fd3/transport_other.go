//go:build !windows

// transport_other.go——FD3 非 Windows 传输层占位（fail-closed——诚实不实现）：
// Linux 模式 B 的宿主中继面=另行设计（abstract socket 族——见 fd3-broker-设计.md
// 注记）；本包在 linux/darwin 仅帧协议（frame.go 平台中立）可用。
package fd3

import "errors"

// ErrUnsupportedPlatform FD3 传输层本平台未实现（fail-closed——不静默降级）。
var ErrUnsupportedPlatform = errors.New("fd3: 本平台传输层未实现（windows=命名管道；linux=待设计）")

// Listener 占位（仅满足编译面）。
type Listener struct{}

// Name 占位。
func (l *Listener) Name() string { return "" }

// Close 占位。
func (l *Listener) Close() error { return nil }

// Listen 占位（恒 fail-closed）。
func Listen(label string) (*Listener, error) { return nil, ErrUnsupportedPlatform }

// Accept 占位。
func (l *Listener) Accept() (*Conn, error) { return nil, ErrUnsupportedPlatform }

// Dial 占位。
func Dial(base string) (*Conn, error) { return nil, ErrUnsupportedPlatform }

// Conn 占位（方法集与 windows 版对称——broker.go 等平台中立消费面可编译）。
type Conn struct{}

// ReadFrame 占位（恒 fail-closed）。
func (c *Conn) ReadFrame() (Frame, error) { return Frame{}, ErrUnsupportedPlatform }

// WriteFrame 占位（恒 fail-closed）。
func (c *Conn) WriteFrame(f Frame) error { return ErrUnsupportedPlatform }

// Close 占位。
func (c *Conn) Close() error { return nil }
