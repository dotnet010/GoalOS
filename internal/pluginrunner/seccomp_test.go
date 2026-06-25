package pluginrunner

import "testing"

func TestDefaultSeccomp_IsStrict(t *testing.T) {
	p := DefaultSeccomp()
	if p.DefaultAction != "kill" {
		t.Errorf("DefaultAction: got %q, want %q", p.DefaultAction, "kill")
	}
	if len(p.AllowedSyscalls) < 20 {
		t.Errorf("AllowedSyscalls: got %d, want >= 20 (strict whitelist)", len(p.AllowedSyscalls))
	}
}

func TestExtendedSeccomp_HasMoreSyscalls(t *testing.T) {
	def := DefaultSeccomp()
	ext := ExtendedSeccomp()
	if len(ext.AllowedSyscalls) <= len(def.AllowedSyscalls) {
		t.Errorf("ExtendedSeccomp should have MORE syscalls than DefaultSeccomp. def=%d, ext=%d",
			len(def.AllowedSyscalls), len(ext.AllowedSyscalls))
	}
	// 验证网络 syscall 存在
	hasSocket := false
	for _, s := range ext.AllowedSyscalls {
		if s == "socket" {
			hasSocket = true
			break
		}
	}
	if !hasSocket {
		t.Error("ExtendedSeccomp should include 'socket' syscall")
	}
}

func TestSeccompForRiskLevel_L0_ReturnsDefault(t *testing.T) {
	for _, level := range []string{"L0", "L1", "L2"} {
		p := SeccompForRiskLevel(level)
		if len(p.AllowedSyscalls) < 20 {
			t.Errorf("SeccompForRiskLevel(%s): got %d syscalls, expected strict whitelist (>=20)", level, len(p.AllowedSyscalls))
		}
	}
}

func TestSeccompForRiskLevel_L3_ReturnsExtended(t *testing.T) {
	for _, level := range []string{"L3", "L4", "L5"} {
		p := SeccompForRiskLevel(level)
		if len(p.AllowedSyscalls) < 40 {
			t.Errorf("SeccompForRiskLevel(%s): got %d syscalls, expected extended whitelist (>=40)", level, len(p.AllowedSyscalls))
		}
	}
}

func TestSeccompForRiskLevel_Unknown_ReturnsStrict(t *testing.T) {
	// fail-secure: 未知风险等级默认走严格白名单
	p := SeccompForRiskLevel("unknown")
	if len(p.AllowedSyscalls) >= 40 {
		t.Errorf("SeccompForRiskLevel(unknown): should return strict whitelist as fail-secure. got %d syscalls", len(p.AllowedSyscalls))
	}
}
