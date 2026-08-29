//go:build linux

// provider_agentbox_linux.go——Linux 探针形态（TC-RT-001b）。
package runtime

// probeWriteBin/Args fs 禁闭探针（写工作区外——NEWNS 内 mount 面由 agentbox 承载）。
func probeWriteBin() string     { return "/usr/bin/touch" }
func probeWriteArgs() []string  { return []string{"/usr/goalos-probe-denied"} }

// probeNetBin/Args 网络探针（NetworkBlocked 下出站必败）。
func probeNetBin() string { return "/usr/bin/nc" }
func probeNetArgs() []string {
	return []string{"-w", "1", "192.0.2.1", "80"}
}
