// forwarder.go——fd3d 沙箱内回环转发器（S3——R-1660 v2「沙箱内回环转发器承接」落地）。
//
// 形态：在受限档沙箱内监听 127.0.0.1:<port>（同 AC 回环=OS 原生放行——四场景
// 实证(1)），agent 代码直连 localhost 无感知；每连接→命名管道→daemon 侧 broker
// （OPEN 帧携带映射端点——契约校验+治理留痕归 broker 面）。
// 失败方向全部 fail-closed：管道断/broker 拒=立即关连接（agent 侧=连接重置，
// 绝不静默挂起——R-1660 v2 秒级超时坑构造性消除）。
//
// 平台性：本文件平台中立（net.Listen+Dial 抽象）；Windows=命名管道传输（生产面），
// 其他平台 Dial=ErrUnsupportedPlatform fail-closed（不静默降级）。
package fd3

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
)

// EndpointMapping 转发映射（ListenPort=沙箱内回环端口；Endpoint=broker 校验的真实目标）。
type EndpointMapping struct {
	ListenPort int    // 沙箱内 127.0.0.1 监听端口
	Endpoint   string // 契约声明的真实目标 "host:port"
}

// Forwarder fd3d 转发器（沙箱内长驻——生命周期=session；绞杀=Job 关闭面）。
type Forwarder struct {
	PipeBase string
	Mappings []EndpointMapping
	// ActualMappings=Run 后的实际监听（绑定冲突回落 :0 自由端口——实机实锤：
	// AC 回环与宿主同命名空间，宿主已占端口（Ollama 11434 族）在 AC 内不可重绑）。
	ActualMappings []EndpointMapping
	cancel         context.CancelFunc
	wg             sync.WaitGroup
}

// ParseMappings 映射串解析（"11434=127.0.0.1:11434,8000=192.168.3.52:8000"）。
// 省略左值=端口同目标（"127.0.0.1:11434"→127.0.0.1:11434 透明映射——R-1660 无感知纪律）。
func ParseMappings(s string) ([]EndpointMapping, error) {
	if s == "" {
		return nil, nil
	}
	var out []EndpointMapping
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		lhs, rhs, found := strings.Cut(part, "=")
		if !found {
			// 裸端点=同端口透明映射（注意：Cut 未命中时 rhs=""——用 lhs=整段，
			// 实机实锤：rhs 空串直切=端口解析假失败）
			var port int
			if _, err := fmt.Sscanf(lhs[strings.LastIndex(lhs, ":")+1:], "%d", &port); err != nil || port == 0 {
				return nil, fmt.Errorf("fd3d: 映射 %q 端口解析失败", part)
			}
			out = append(out, EndpointMapping{ListenPort: port, Endpoint: lhs})
			continue
		}
		var port int
		if _, err := fmt.Sscanf(lhs, "%d", &port); err != nil || port == 0 {
			return nil, fmt.Errorf("fd3d: 映射 %q 监听端口非法", part)
		}
		out = append(out, EndpointMapping{ListenPort: port, Endpoint: rhs})
	}
	return out, nil
}

// Run 启动全部映射的监听（后台——wg 计数；ctx 取消=全停）。
// 端口冲突纪律（实机实锤——AC 回环与宿主同命名空间）：目标端口被占=
// 回落 :0 自由端口（ActualMappings 记录实占——上层经映射文件知悉实际端口，
// 禁静默错配=agent 连错服务对象）。
func (f *Forwarder) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	f.cancel = cancel
	for _, m := range f.Mappings {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", m.ListenPort))
		if err != nil {
			// 冲突回落自由端口（EADDRINUSE 族——宿主同命名空间占用）
			ln, err = net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				cancel()
				return fmt.Errorf("fd3d: 监听回落也失败: %w", err)
			}
		}
		actual := ln.Addr().(*net.TCPAddr).Port
		f.ActualMappings = append(f.ActualMappings, EndpointMapping{ListenPort: actual, Endpoint: m.Endpoint})
		f.wg.Add(1)
		go func(ln net.Listener, m EndpointMapping) {
			defer f.wg.Done()
			defer ln.Close()
			go func() { <-ctx.Done(); ln.Close() }()
			for {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				go f.serveConn(c, m.Endpoint)
			}
		}(ln, m)
	}
	return nil
}

// Stop 停全部监听（连接在飞=各自收尾）。
func (f *Forwarder) Stop() {
	if f.cancel != nil {
		f.cancel()
	}
	f.wg.Wait()
}

// serveConn 单连接：管道开+OPEN 映射端点+broker 应答校验+双向字节泵。
func (f *Forwarder) serveConn(c net.Conn, endpoint string) {
	defer c.Close()
	pipe, err := Dial(f.PipeBase)
	if err != nil {
		return // broker/管道不可达=连接即关（fail-closed——agent 侧连接重置，不挂起）
	}
	defer pipe.Close()
	if err := pipe.WriteFrame(Frame{Op: OpOpen, Payload: []byte(endpoint)}); err != nil {
		return
	}
	resp, err := pipe.ReadFrame()
	if err != nil || resp.Op != OpOpenOK {
		return // broker 拒绝/断=关（治理否定=连接重置——审批语义不回传细节，防探测）
	}
	// 双向泵：沙箱字节流↔DATA 帧
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { // 沙箱→管道
		defer wg.Done()
		buf := make([]byte, 32*1024)
		for {
			n, err := c.Read(buf)
			if n > 0 {
				if werr := pipe.WriteFrame(Frame{Op: OpData, Payload: buf[:n]}); werr != nil {
					return
				}
			}
			if err != nil {
				if err == io.EOF {
					_ = pipe.WriteFrame(Frame{Op: OpClose})
				}
				return
			}
		}
	}()
	go func() { // 管道→沙箱
		defer wg.Done()
		for {
			fr, err := pipe.ReadFrame()
			if err != nil {
				return
			}
			switch fr.Op {
			case OpData:
				if _, werr := c.Write(fr.Payload); werr != nil {
					return
				}
			case OpClose:
				if tc, ok := c.(*net.TCPConn); ok {
					_ = tc.CloseWrite()
				}
				return
			}
		}
	}()
	wg.Wait()
}

// ForwarderMain fd3d 子进程入口（__goalos-fd3d <pipebase> [mappings]——
// re-exec 自身=零外部二进制纪律 R-1666 同族）。返回 exit code。
func ForwarderMain(args []string) int {
	if len(args) < 1 {
		fmt.Println("FD3D-FATAL: usage: __goalos-fd3d <pipebase> [mappings]")
		return 1
	}
	var mappings []EndpointMapping
	var err error
	if len(args) >= 2 {
		mappings, err = ParseMappings(args[1])
		if err != nil {
			fmt.Printf("FD3D-FATAL: 映射解析: %v\n", err)
			return 1
		}
	}
	f := &Forwarder{PipeBase: args[0], Mappings: mappings}
	if err := f.Run(context.Background()); err != nil {
		fmt.Printf("FD3D-FATAL: %v\n", err)
		return 1
	}
	// 实际映射上报（spawn 侧解析写映射文件——工作负载据此知真实端口；
	// 绑定冲突回落后「声明端口≠实际端口」必须显式可见，禁静默错配）
	for _, m := range f.ActualMappings {
		fmt.Printf("FD3D-MAP %d=%s\n", m.ListenPort, m.Endpoint)
	}
	fmt.Println("FD3D-READY") // 就绪信号
	f.wg.Wait()              // 长驻至监听全停（绞杀=Job KILL_ON_JOB_CLOSE——session 生命周期面）
	return 0
}
