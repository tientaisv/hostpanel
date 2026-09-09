package system

import (
	"testing"
)

func TestCpuGovernorInfo(t *testing.T) {
	info, err := GetCpuGovernorInfo()
	if err != nil {
		t.Fatalf("GetCpuGovernorInfo failed: %v", err)
	}

	if !info.Supported {
		t.Skip("CPU frequency scaling not supported in this test environment")
	}

	t.Logf("Governor Info: Supported=%t, Available=%v, Current=%s, Driver=%s, Freq=%dMHz (Min=%d, Max=%d), TurboSupported=%t, TurboEnabled=%t, Cores=%d",
		info.Supported, info.AvailableGovernors, info.CurrentGovernor, info.Driver,
		info.CurFreqMHz, info.MinFreqMHz, info.MaxFreqMHz,
		info.TurboSupported, info.TurboEnabled, info.CoresCount)

	if len(info.AvailableGovernors) == 0 {
		t.Errorf("Expected available governors, got empty")
	}

	if info.CurrentGovernor == "" {
		t.Errorf("Expected current governor, got empty")
	}
}
