package system

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

var governorMu sync.Mutex

type CpuGovernorInfo struct {
	Supported          bool     `json:"supported"`
	AvailableGovernors []string `json:"available_governors"`
	CurrentGovernor    string   `json:"current_governor"`
	Driver             string   `json:"driver"`
	CurFreqMHz         int      `json:"cur_freq_mhz"`
	MinFreqMHz         int      `json:"min_freq_mhz"`
	MaxFreqMHz         int      `json:"max_freq_mhz"`
	TurboSupported     bool     `json:"turbo_supported"`
	TurboEnabled       bool     `json:"turbo_enabled"`
	CoresCount         int      `json:"cores_count"`
}

func readSysfsString(path string) string {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func readSysfsInt(path string) int {
	s := readSysfsString(path)
	if s == "" {
		return 0
	}
	val, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return val
}

// GetCpuGovernorInfo returns information about CPU frequency scaling and governors
func GetCpuGovernorInfo() (*CpuGovernorInfo, error) {
	governorMu.Lock()
	defer governorMu.Unlock()

	info := &CpuGovernorInfo{
		AvailableGovernors: make([]string, 0),
	}

	baseCpuPath := "/sys/devices/system/cpu/cpu0/cpufreq"
	availStr := readSysfsString(filepath.Join(baseCpuPath, "scaling_available_governors"))
	if availStr == "" {
		// Try finding another core with scaling_available_governors
		matches, _ := filepath.Glob("/sys/devices/system/cpu/cpu*/cpufreq/scaling_available_governors")
		if len(matches) > 0 {
			availStr = readSysfsString(matches[0])
			baseCpuPath = filepath.Dir(matches[0])
		}
	}

	if availStr == "" {
		info.Supported = false
		return info, nil
	}

	info.Supported = true
	fields := strings.Fields(availStr)
	info.AvailableGovernors = fields

	info.CurrentGovernor = readSysfsString(filepath.Join(baseCpuPath, "scaling_governor"))
	info.Driver = readSysfsString(filepath.Join(baseCpuPath, "scaling_driver"))

	// Frequencies in sysfs are in kHz, convert to MHz
	if cur := readSysfsInt(filepath.Join(baseCpuPath, "scaling_cur_freq")); cur > 0 {
		info.CurFreqMHz = cur / 1000
	}
	if min := readSysfsInt(filepath.Join(baseCpuPath, "scaling_min_freq")); min > 0 {
		info.MinFreqMHz = min / 1000
	}
	if max := readSysfsInt(filepath.Join(baseCpuPath, "scaling_max_freq")); max > 0 {
		info.MaxFreqMHz = max / 1000
	}

	// Count CPU cores with governor control
	coreMatches, _ := filepath.Glob("/sys/devices/system/cpu/cpu[0-9]*/cpufreq/scaling_governor")
	info.CoresCount = len(coreMatches)
	if info.CoresCount == 0 {
		info.CoresCount = 1
	}

	// Detect Turbo Boost status
	// 1. Intel P-State: /sys/devices/system/cpu/intel_pstate/no_turbo (0 = enabled, 1 = disabled)
	intelNoTurbo := "/sys/devices/system/cpu/intel_pstate/no_turbo"
	if content := readSysfsString(intelNoTurbo); content != "" {
		info.TurboSupported = true
		info.TurboEnabled = (content == "0")
	} else {
		// 2. Generic cpufreq boost: /sys/devices/system/cpu/cpufreq/boost (1 = enabled, 0 = disabled)
		amdBoost := "/sys/devices/system/cpu/cpufreq/boost"
		if content := readSysfsString(amdBoost); content != "" {
			info.TurboSupported = true
			info.TurboEnabled = (content == "1")
		}
	}

	return info, nil
}

// SetCpuGovernor sets the CPU scaling governor across all available CPU cores
func SetCpuGovernor(governor string) error {
	governor = strings.TrimSpace(strings.ToLower(governor))
	if governor == "" {
		return fmt.Errorf("governor không được để trống")
	}

	governorMu.Lock()
	defer governorMu.Unlock()

	// Check against available governors if detectable
	baseCpuPath := "/sys/devices/system/cpu/cpu0/cpufreq"
	availStr := readSysfsString(filepath.Join(baseCpuPath, "scaling_available_governors"))
	if availStr != "" {
		availList := strings.Fields(availStr)
		found := false
		for _, g := range availList {
			if strings.EqualFold(g, governor) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("governor '%s' không được hỗ trợ trên CPU này (Hỗ trợ: %s)", governor, strings.Join(availList, ", "))
		}
	}

	coreMatches, err := filepath.Glob("/sys/devices/system/cpu/cpu[0-9]*/cpufreq/scaling_governor")
	if err != nil || len(coreMatches) == 0 {
		return fmt.Errorf("không tìm thấy nhân CPU hỗ trợ cpufreq scaling_governor")
	}

	var errors []string
	appliedCount := 0
	for _, corePath := range coreMatches {
		if writeErr := ioutil.WriteFile(corePath, []byte(governor+"\n"), 0644); writeErr != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", filepath.Base(filepath.Dir(filepath.Dir(corePath))), writeErr))
		} else {
			appliedCount++
		}
	}

	if len(errors) > 0 && appliedCount == 0 {
		return fmt.Errorf("thất bại khi áp dụng governor '%s': %s", governor, strings.Join(errors, "; "))
	}

	return nil
}

// SetCpuTurbo enables or disables CPU Turbo / Boost
func SetCpuTurbo(enable bool) error {
	governorMu.Lock()
	defer governorMu.Unlock()

	intelNoTurbo := "/sys/devices/system/cpu/intel_pstate/no_turbo"
	if content := readSysfsString(intelNoTurbo); content != "" {
		val := "0\n"
		if !enable {
			val = "1\n"
		}
		if err := ioutil.WriteFile(intelNoTurbo, []byte(val), 0644); err != nil {
			return fmt.Errorf("không thể cập nhật intel_pstate no_turbo: %v", err)
		}
		return nil
	}

	amdBoost := "/sys/devices/system/cpu/cpufreq/boost"
	if content := readSysfsString(amdBoost); content != "" {
		val := "1\n"
		if !enable {
			val = "0\n"
		}
		if err := ioutil.WriteFile(amdBoost, []byte(val), 0644); err != nil {
			return fmt.Errorf("không thể cập nhật cpufreq boost: %v", err)
		}
		return nil
	}

	return fmt.Errorf("hệ thống không hỗ trợ bật/tắt Turbo/Boost qua sysfs")
}
