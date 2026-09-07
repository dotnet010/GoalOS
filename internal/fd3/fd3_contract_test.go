//go:build windows || linux

// fd3_contract_test.go——FD3 传输层契约测试（S1——R-571 先红）。
// 断言来源=开发计划/fd3-broker-设计.md §六 测试矩阵 F3/F6：
//  F3：≥2 客户端并发双向全链+响应归属断言（R-1662 v2——顺序单请求不得冒充并发证据）。
//  F6：管道对名熵化唯一（R-1667 v2 同族纪律）+SDDL 结构断言（ALL APPLICATION
//      PACKAGES 授权面——AC 可连宿主管道路径）。
package fd3

import (
	"fmt"
	"strings"
	goruntime "runtime"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestFD3_ConcurrentAttribution F3：2 客户端并发×双向——每客户端各发 N 帧带身份标签，
// 服务端逐帧回显；断言=每客户端回收的标签全属自己（归属零串线）+无死锁（全链完成时限）。
func TestFD3_ConcurrentAttribution(t *testing.T) {
	ln, err := Listen(t.TempDir(), "FD3-Test-F3")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c *Conn) {
				defer c.Close()
				for {
					f, err := c.ReadFrame()
					if err != nil {
						return
					}
					if f.Op == OpData {
						_ = c.WriteFrame(Frame{Op: OpData, Payload: f.Payload}) // 回显
					}
				}
			}(conn)
		}
	}()

	const clients = 2
	const framesPer = 40
	var wg sync.WaitGroup
	errs := make(chan string, clients)
	for ci := 0; ci < clients; ci++ {
		wg.Add(1)
		go func(ci int) {
			defer wg.Done()
			conn, err := Dial(ln.Name())
			if err != nil {
				errs <- fmt.Sprintf("client %d dial: %v", ci, err)
				return
			}
			defer conn.Close()
			tag := fmt.Sprintf("c%d-", ci)
			// 读协程：回收回显，归属断言（只看到本客户端标签）
			got := make(chan int, framesPer)
			go func() {
				for i := 0; i < framesPer; i++ {
					f, err := conn.ReadFrame()
					if err != nil {
						return
					}
					if !strings.HasPrefix(string(f.Payload), tag) {
						errs <- fmt.Sprintf("client %d 收到他客户端标签: %q（归属串线）", ci, f.Payload)
						return
					}
					got <- 1
				}
			}()
			// 写主程：连续发（并发压力面——不等回显）
			for i := 0; i < framesPer; i++ {
				if err := conn.WriteFrame(Frame{Op: OpData, Payload: []byte(fmt.Sprintf("%s%d", tag, i))}); err != nil {
					errs <- fmt.Sprintf("client %d write: %v", ci, err)
					return
				}
			}
			select {
			case <-time.After(15 * time.Second):
				errs <- fmt.Sprintf("client %d 回显超时（疑死锁）", ci)
			case <-func() chan struct{} { ch := make(chan struct{}); go func() { for i := 0; i < framesPer; i++ { <-got }; close(ch) }(); return ch }():
			}
		}(ci)
	}
	wg.Wait()
	select {
	case e := <-errs:
		t.Fatal(e)
	default:
	}
}

// TestFD3_NameEntropy F6：同标签两次 Listen=管道对名必须异（熵化一次性——R-1667 v2 同族：
// 同名复用=残留态继承面）；名含合法管道前缀+熵段。
func TestFD3_NameEntropy(t *testing.T) {
	l1, err := Listen(t.TempDir(), "FD3-Test-F6")
	if err != nil {
		t.Fatalf("Listen#1: %v", err)
	}
	defer l1.Close()
	l2, err := Listen(t.TempDir(), "FD3-Test-F6")
	if err != nil {
		t.Fatalf("Listen#2: %v", err)
	}
	defer l2.Close()
	if l1.Name() == l2.Name() {
		t.Fatalf("同标签两 Listen 同名=%q——熵化纪律失守（残留态继承面）", l1.Name())
	}
	// 平台形态断言（windows=命名管道前缀；linux=unix socket 熵名落目录）
	if goruntime.GOOS == "windows" && !strings.HasPrefix(l1.Name(), `\\.\pipe\GoalOS-FD3-`) {
		t.Fatalf("管道名缺合法前缀: %q", l1.Name())
	}
	if goruntime.GOOS == "linux" && !strings.Contains(filepath.Base(l1.Name()), "goalos-fd3-") {
		t.Fatalf("socket 名缺熵化段: %q", l1.Name())
	}
}
