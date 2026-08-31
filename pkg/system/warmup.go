package system

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"runtime"
	"sync"
	"time"
)

// WarmupState defines the operational phase of the warmup system
type WarmupState string

const (
	StateDisabled   WarmupState = "DISABLED"
	StateMonitoring WarmupState = "MONITORING"
	StateWarming    WarmupState = "WARMING"
	StateCooldown   WarmupState = "COOLDOWN"
	StateTesting    WarmupState = "TESTING"
)

// WarmupConfig holds configurable parameters for the keep-warm system
type WarmupConfig struct {
	Enabled           bool    `json:"enabled"`
	LowCPUThreshold   float64 `json:"low_cpu_threshold"`      // Trigger warmup if CPU load < this % (default: 30.0)
	TargetCPUPercent  float64 `json:"target_cpu_percent"`     // Regulate CPU to this % during warmup (default: 45.0)
	MaxCPUThreshold   float64 `json:"max_cpu_threshold"`      // Emergency throttle/pause if CPU > this % (default: 65.0)
	IdleCheckDuration int     `json:"idle_check_duration_sec"`// Time CPU must remain low before warmup (default: 1800s = 30m)
	WarmupDuration    int     `json:"warmup_duration_sec"`    // Duration of each warmup session (default: 1800s = 30m)
	CooldownDuration  int     `json:"cooldown_duration_sec"`  // Rest period after warmup (default: 1800s = 30m)
}

// WarmupStatus represents the real-time public status of the warmup manager
type WarmupStatus struct {
	Enabled             bool        `json:"enabled"`
	State               WarmupState `json:"state"`
	StateDescription    string      `json:"state_description"`
	CurrentCPU          float64     `json:"current_cpu"`
	AverageCPU30m       float64     `json:"average_cpu_30m"`
	LowCPUTimerSec      int         `json:"low_cpu_timer_sec"`
	PhaseElapsedSec     int         `json:"phase_elapsed_sec"`
	PhaseRemainingSec   int         `json:"phase_remaining_sec"`
	TargetCPUPercent    float64     `json:"target_cpu_percent"`
	LowCPUThreshold     float64     `json:"low_cpu_threshold"`
	MaxCPUThreshold     float64     `json:"max_cpu_threshold"`
	IsThrottled         bool        `json:"is_throttled"`
	LastWarmedAt        string      `json:"last_warmed_at,omitempty"`
	TotalWarmupSessions int         `json:"total_warmup_sessions"`
	ActiveWorkers       int         `json:"active_workers"`
	Message             string      `json:"message"`
}

type WarmupManager struct {
	mu               sync.Mutex
	config           WarmupConfig
	configFile       string
	state            WarmupState
	cancelWarmup     context.CancelFunc
	cancelTest       context.CancelFunc
	phaseStartTime   time.Time
	phaseDurationSec int
	lowCPUSince      time.Time
	lowCPUTimerSec   int
	lastWarmedAt     time.Time
	totalSessions    int
	isThrottled      bool
	currentCPU       float64
	cpuHistory       []float64
	activeWorkers    int
	workerWorkMs     int // Adaptive milliseconds of active work per 100ms cycle
}

var (
	GlobalWarmupManager *WarmupManager
	warmupOnce          sync.Once
)

const defaultConfigFile = "warmup_config.json"

// InitWarmupManager initializes the singleton WarmupManager
func InitWarmupManager() *WarmupManager {
	warmupOnce.Do(func() {
		cfg := WarmupConfig{
			Enabled:           false,
			LowCPUThreshold:   30.0,
			TargetCPUPercent:  45.0,
			MaxCPUThreshold:   65.0,
			IdleCheckDuration: 1800, // 30 mins
			WarmupDuration:    1800, // 30 mins
			CooldownDuration:  1800, // 30 mins
		}

		// Load config if exists
		if data, err := ioutil.ReadFile(defaultConfigFile); err == nil {
			var loaded WarmupConfig
			if err := json.Unmarshal(data, &loaded); err == nil {
				// Validate sane defaults
				if loaded.LowCPUThreshold <= 0 {
					loaded.LowCPUThreshold = 30.0
				}
				if loaded.TargetCPUPercent <= 0 {
					loaded.TargetCPUPercent = 45.0
				}
				if loaded.MaxCPUThreshold <= 0 {
					loaded.MaxCPUThreshold = 65.0
				}
				if loaded.IdleCheckDuration <= 0 {
					loaded.IdleCheckDuration = 1800
				}
				if loaded.WarmupDuration <= 0 {
					loaded.WarmupDuration = 1800
				}
				if loaded.CooldownDuration <= 0 {
					loaded.CooldownDuration = 1800
				}
				cfg = loaded
			}
		}

		GlobalWarmupManager = &WarmupManager{
			config:       cfg,
			configFile:   defaultConfigFile,
			state:        StateDisabled,
			workerWorkMs: 40, // Base 40ms compute / 60ms sleep per 100ms cycle (~40% load)
			cpuHistory:   make([]float64, 0, 180),
		}

		if cfg.Enabled {
			GlobalWarmupManager.state = StateMonitoring
			GlobalWarmupManager.phaseStartTime = time.Now()
			GlobalWarmupManager.phaseDurationSec = cfg.IdleCheckDuration
		}

		// Start background monitor loop
		go GlobalWarmupManager.runSupervisor()
	})

	return GlobalWarmupManager
}

// saveConfig writes configuration to disk
func (wm *WarmupManager) saveConfig() {
	if data, err := json.MarshalIndent(wm.config, "", "  "); err == nil {
		_ = ioutil.WriteFile(wm.configFile, data, 0644)
	}
}

// GetStatus returns the current snapshot of warmup manager
func (wm *WarmupManager) GetStatus() WarmupStatus {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	now := time.Now()
	elapsed := 0
	remaining := 0

	if !wm.phaseStartTime.IsZero() && wm.phaseDurationSec > 0 {
		elapsed = int(now.Sub(wm.phaseStartTime).Seconds())
		if elapsed > wm.phaseDurationSec {
			elapsed = wm.phaseDurationSec
		}
		remaining = wm.phaseDurationSec - elapsed
		if remaining < 0 {
			remaining = 0
		}
	}

	// Calculate average CPU from historical samples
	var avgCPU float64
	if len(wm.cpuHistory) > 0 {
		var sum float64
		for _, v := range wm.cpuHistory {
			sum += v
		}
		avgCPU = sum / float64(len(wm.cpuHistory))
	} else {
		avgCPU = wm.currentCPU
	}

	stateDesc := "Đã Tắt"
	message := "Chế độ làm nóng máy chủ đang tắt."

	switch wm.state {
	case StateMonitoring:
		stateDesc = "Đang Giám Sát Nhàn Rỗi (< 30%)"
		message = fmt.Sprintf("Đang theo dõi CPU hệ thống (Hiện tại: %.1f%%). Tự động làm nóng nếu < %.0f%% trong %d phút.",
			wm.currentCPU, wm.config.LowCPUThreshold, wm.config.IdleCheckDuration/60)
	case StateWarming:
		stateDesc = "🔥 Đang Làm Nóng An Toàn (~45% CPU)"
		if wm.isThrottled {
			message = fmt.Sprintf("⚠️ Tạm thời giảm tải (Auto-Throttle) do CPU hệ thống đạt %.1f%% (> %.0f%% an toàn).",
				wm.currentCPU, wm.config.MaxCPUThreshold)
		} else {
			message = fmt.Sprintf("Đang duy trì tải CPU an toàn %.1f%% (Mục tiêu: %.0f%%). Còn lại %s.",
				wm.currentCPU, wm.config.TargetCPUPercent, formatDuration(remaining))
		}
	case StateCooldown:
		stateDesc = "⏳ Đang Nghỉ Ngơi (Cooldown 30m)"
		message = fmt.Sprintf("Đã hoàn tất phiên làm nóng 30 phút. Đang nghỉ ngơi, còn lại %s.", formatDuration(remaining))
	case StateTesting:
		stateDesc = "⚡ Đang Chạy Thử Nghiệm (Test Mode)"
		message = fmt.Sprintf("Đang chạy thử nghiệm làm nóng CPU trong 1 phút (Còn lại: %ds).", remaining)
	}

	lastWarmedStr := ""
	if !wm.lastWarmedAt.IsZero() {
		lastWarmedStr = wm.lastWarmedAt.Format(time.RFC3339)
	}

	return WarmupStatus{
		Enabled:             wm.config.Enabled,
		State:               wm.state,
		StateDescription:    stateDesc,
		CurrentCPU:          wm.currentCPU,
		AverageCPU30m:       avgCPU,
		LowCPUTimerSec:      wm.lowCPUTimerSec,
		PhaseElapsedSec:     elapsed,
		PhaseRemainingSec:   remaining,
		TargetCPUPercent:    wm.config.TargetCPUPercent,
		LowCPUThreshold:     wm.config.LowCPUThreshold,
		MaxCPUThreshold:     wm.config.MaxCPUThreshold,
		IsThrottled:         wm.isThrottled,
		LastWarmedAt:        lastWarmedStr,
		TotalWarmupSessions: wm.totalSessions,
		ActiveWorkers:       wm.activeWorkers,
		Message:             message,
	}
}

// Toggle enables or disables the automatic keep-warm mode
func (wm *WarmupManager) Toggle(enable bool) (WarmupStatus, error) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	wm.config.Enabled = enable
	wm.saveConfig()

	if !enable {
		// Stop any ongoing warmup workers
		if wm.cancelWarmup != nil {
			wm.cancelWarmup()
			wm.cancelWarmup = nil
		}
		if wm.cancelTest != nil {
			wm.cancelTest()
			wm.cancelTest = nil
		}
		wm.state = StateDisabled
		wm.lowCPUTimerSec = 0
		wm.phaseDurationSec = 0
		wm.phaseStartTime = time.Time{}
		wm.isThrottled = false
		wm.activeWorkers = 0
	} else {
		// Switch to monitoring
		if wm.state != StateWarming && wm.state != StateTesting {
			wm.state = StateMonitoring
			wm.phaseStartTime = time.Now()
			wm.phaseDurationSec = wm.config.IdleCheckDuration
			wm.lowCPUTimerSec = 0
			wm.lowCPUSince = time.Time{}
		}
	}

	return wm.getStatusLocked(), nil
}

// TriggerTest starts a short test warmup session (e.g. 60 seconds)
func (wm *WarmupManager) TriggerTest(durationSec int) (WarmupStatus, error) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if durationSec <= 0 || durationSec > 300 {
		durationSec = 60
	}

	// Stop previous test or warmup if any
	if wm.cancelWarmup != nil {
		wm.cancelWarmup()
		wm.cancelWarmup = nil
	}
	if wm.cancelTest != nil {
		wm.cancelTest()
		wm.cancelTest = nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(durationSec)*time.Second)
	wm.cancelTest = cancel
	wm.state = StateTesting
	wm.phaseStartTime = time.Now()
	wm.phaseDurationSec = durationSec
	wm.isThrottled = false

	go wm.startWarmupWorkers(ctx, durationSec, true)

	return wm.getStatusLocked(), nil
}

// UpdateConfig updates the customizable thresholds
func (wm *WarmupManager) UpdateConfig(cfg WarmupConfig) (WarmupStatus, error) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if cfg.LowCPUThreshold > 0 && cfg.LowCPUThreshold <= 80 {
		wm.config.LowCPUThreshold = cfg.LowCPUThreshold
	}
	if cfg.TargetCPUPercent >= 35 && cfg.TargetCPUPercent <= 70 {
		wm.config.TargetCPUPercent = cfg.TargetCPUPercent
	}
	if cfg.MaxCPUThreshold > wm.config.TargetCPUPercent && cfg.MaxCPUThreshold <= 95 {
		wm.config.MaxCPUThreshold = cfg.MaxCPUThreshold
	}
	if cfg.IdleCheckDuration >= 60 && cfg.IdleCheckDuration <= 7200 {
		wm.config.IdleCheckDuration = cfg.IdleCheckDuration
	}
	if cfg.WarmupDuration >= 60 && cfg.WarmupDuration <= 7200 {
		wm.config.WarmupDuration = cfg.WarmupDuration
	}
	if cfg.CooldownDuration >= 60 && cfg.CooldownDuration <= 7200 {
		wm.config.CooldownDuration = cfg.CooldownDuration
	}

	wm.saveConfig()
	return wm.getStatusLocked(), nil
}

// getStatusLocked helper without re-locking
func (wm *WarmupManager) getStatusLocked() WarmupStatus {
	now := time.Now()
	elapsed := 0
	remaining := 0

	if !wm.phaseStartTime.IsZero() && wm.phaseDurationSec > 0 {
		elapsed = int(now.Sub(wm.phaseStartTime).Seconds())
		if elapsed > wm.phaseDurationSec {
			elapsed = wm.phaseDurationSec
		}
		remaining = wm.phaseDurationSec - elapsed
		if remaining < 0 {
			remaining = 0
		}
	}

	var avgCPU float64
	if len(wm.cpuHistory) > 0 {
		var sum float64
		for _, v := range wm.cpuHistory {
			sum += v
		}
		avgCPU = sum / float64(len(wm.cpuHistory))
	} else {
		avgCPU = wm.currentCPU
	}

	stateDesc := "Đã Tắt"
	message := "Chế độ làm nóng máy chủ đang tắt."
	switch wm.state {
	case StateMonitoring:
		stateDesc = "Đang Giám Sát Nhàn Rỗi (< 30%)"
		message = fmt.Sprintf("Đang theo dõi CPU hệ thống (Hiện tại: %.1f%%). Tự động làm nóng nếu < %.0f%% trong %d phút.",
			wm.currentCPU, wm.config.LowCPUThreshold, wm.config.IdleCheckDuration/60)
	case StateWarming:
		stateDesc = "🔥 Đang Làm Nóng An Toàn (~45% CPU)"
		message = fmt.Sprintf("Đang duy trì tải CPU an toàn %.1f%% (Mục tiêu: %.0f%%).", wm.currentCPU, wm.config.TargetCPUPercent)
	case StateCooldown:
		stateDesc = "⏳ Đang Nghỉ Ngơi (Cooldown 30m)"
		message = fmt.Sprintf("Đang nghỉ ngơi 30 phút, còn lại %s.", formatDuration(remaining))
	case StateTesting:
		stateDesc = "⚡ Đang Chạy Thử Nghiệm (Test Mode)"
		message = fmt.Sprintf("Đang chạy thử nghiệm làm nóng CPU (Còn lại: %ds).", remaining)
	}

	lastWarmedStr := ""
	if !wm.lastWarmedAt.IsZero() {
		lastWarmedStr = wm.lastWarmedAt.Format(time.RFC3339)
	}

	return WarmupStatus{
		Enabled:             wm.config.Enabled,
		State:               wm.state,
		StateDescription:    stateDesc,
		CurrentCPU:          wm.currentCPU,
		AverageCPU30m:       avgCPU,
		LowCPUTimerSec:      wm.lowCPUTimerSec,
		PhaseElapsedSec:     elapsed,
		PhaseRemainingSec:   remaining,
		TargetCPUPercent:    wm.config.TargetCPUPercent,
		LowCPUThreshold:     wm.config.LowCPUThreshold,
		MaxCPUThreshold:     wm.config.MaxCPUThreshold,
		IsThrottled:         wm.isThrottled,
		LastWarmedAt:        lastWarmedStr,
		TotalWarmupSessions: wm.totalSessions,
		ActiveWorkers:       wm.activeWorkers,
		Message:             message,
	}
}

// runSupervisor is the background daemon that controls monitoring, warming, cooldown, and adaptive tuning
func (wm *WarmupManager) runSupervisor() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var lastSampleTotal, lastSampleIdle uint64

	for range ticker.C {
		// Read real-time CPU %
		_, idle, total, _ := readCPUStats()
		var cpuPct float64
		if lastSampleTotal > 0 && total > lastSampleTotal {
			tDelta := float64(total - lastSampleTotal)
			iDelta := float64(idle - lastSampleIdle)
			if tDelta > 0 {
				cpuPct = (1.0 - (iDelta / tDelta)) * 100.0
				if cpuPct < 0 {
					cpuPct = 0
				}
				if cpuPct > 100 {
					cpuPct = 100
				}
			}
		}
		lastSampleTotal = total
		lastSampleIdle = idle

		wm.mu.Lock()
		wm.currentCPU = cpuPct
		// Keep up to 180 samples (6 minutes of 2s intervals)
		if len(wm.cpuHistory) >= 180 {
			wm.cpuHistory = wm.cpuHistory[1:]
		}
		wm.cpuHistory = append(wm.cpuHistory, cpuPct)

		if !wm.config.Enabled && wm.state != StateTesting {
			wm.state = StateDisabled
			wm.mu.Unlock()
			continue
		}

		now := time.Now()

		switch wm.state {
		case StateMonitoring:
			if cpuPct < wm.config.LowCPUThreshold {
				if wm.lowCPUSince.IsZero() {
					wm.lowCPUSince = now
				}
				wm.lowCPUTimerSec = int(now.Sub(wm.lowCPUSince).Seconds())

				// Check if reached idle duration (e.g. 30 mins)
				if wm.lowCPUTimerSec >= wm.config.IdleCheckDuration {
					// Trigger 30-min Warmup Session!
					log.Printf("🔥 [Warmup Manager] CPU load has been < %.1f%% for %d seconds. Starting %d seconds Warm-up Session!",
						wm.config.LowCPUThreshold, wm.lowCPUTimerSec, wm.config.WarmupDuration)

					ctx, cancel := context.WithTimeout(context.Background(), time.Duration(wm.config.WarmupDuration)*time.Second)
					wm.cancelWarmup = cancel
					wm.state = StateWarming
					wm.phaseStartTime = now
					wm.phaseDurationSec = wm.config.WarmupDuration
					wm.lowCPUTimerSec = 0
					wm.lowCPUSince = time.Time{}
					wm.totalSessions++
					wm.lastWarmedAt = now

					go wm.startWarmupWorkers(ctx, wm.config.WarmupDuration, false)
				}
			} else {
				// CPU is above threshold (>30%), reset idle counter because server is actively busy
				wm.lowCPUSince = time.Time{}
				wm.lowCPUTimerSec = 0
			}

		case StateWarming:
			// Adaptive Tuning & Overload Protection Loop:
			// Check if we exceed emergency threshold (>65%)
			if cpuPct > wm.config.MaxCPUThreshold {
				wm.isThrottled = true
				// Reduce work duration to lowest
				wm.workerWorkMs = 10
			} else {
				wm.isThrottled = false
				// Dynamic feedback adjustment towards target CPU (e.g. 45%)
				if cpuPct < wm.config.TargetCPUPercent-2.0 {
					if wm.workerWorkMs < 85 {
						wm.workerWorkMs += 3
					}
				} else if cpuPct > wm.config.TargetCPUPercent+3.0 {
					if wm.workerWorkMs > 15 {
						wm.workerWorkMs -= 3
					}
				}
			}

			// Check if session finished
			if int(now.Sub(wm.phaseStartTime).Seconds()) >= wm.config.WarmupDuration {
				if wm.cancelWarmup != nil {
					wm.cancelWarmup()
					wm.cancelWarmup = nil
				}
				// Transition to Cooldown (30 mins)
				log.Printf("⏳ [Warmup Manager] Warmup session completed. Entering %d seconds Cooldown phase.", wm.config.CooldownDuration)
				wm.state = StateCooldown
				wm.phaseStartTime = now
				wm.phaseDurationSec = wm.config.CooldownDuration
				wm.activeWorkers = 0
				wm.isThrottled = false
			}

		case StateCooldown:
			// Check if cooldown finished
			if int(now.Sub(wm.phaseStartTime).Seconds()) >= wm.config.CooldownDuration {
				log.Printf("🔄 [Warmup Manager] Cooldown finished. Returning to Monitoring phase.")
				wm.state = StateMonitoring
				wm.phaseStartTime = now
				wm.phaseDurationSec = wm.config.IdleCheckDuration
				wm.lowCPUTimerSec = 0
				wm.lowCPUSince = time.Time{}
			}

		case StateTesting:
			// Handled by test context
		}

		wm.mu.Unlock()
	}
}

// startWarmupWorkers launches multi-threaded lightweight safe CPU modulation goroutines
func (wm *WarmupManager) startWarmupWorkers(ctx context.Context, durationSec int, isTest bool) {
	numCores := runtime.NumCPU()
	if numCores < 1 {
		numCores = 1
	}

	wm.mu.Lock()
	wm.activeWorkers = numCores
	wm.mu.Unlock()

	defer func() {
		wm.mu.Lock()
		wm.activeWorkers = 0
		if isTest && wm.state == StateTesting {
			if wm.config.Enabled {
				wm.state = StateMonitoring
				wm.phaseStartTime = time.Now()
				wm.phaseDurationSec = wm.config.IdleCheckDuration
			} else {
				wm.state = StateDisabled
				wm.phaseDurationSec = 0
			}
		}
		wm.mu.Unlock()
	}()

	var wg sync.WaitGroup
	for i := 0; i < numCores; i++ {
		wg.Add(1)
		go func(coreIndex int) {
			defer wg.Done()
			wm.workerLoop(ctx, coreIndex)
		}(i)
	}

	wg.Wait()
}

// workerLoop executes safe mathematical time-sliced load per core
func (wm *WarmupManager) workerLoop(ctx context.Context, coreID int) {
	// We use a 100ms cycle budget:
	// Work for `workMs` milliseconds, Sleep for `100 - workMs` milliseconds
	cycleDuration := 100 * time.Millisecond
	dummyData := []byte(fmt.Sprintf("dockpulse-safe-keep-warm-core-%d-%d", coreID, time.Now().UnixNano()))

	for {
		select {
		case <-ctx.Done():
			return
		default:
			wm.mu.Lock()
			workMs := wm.workerWorkMs
			throttled := wm.isThrottled
			wm.mu.Unlock()

			if throttled {
				// In throttled mode, sleep almost entire cycle to yield CPU
				time.Sleep(95 * time.Millisecond)
				continue
			}

			if workMs < 5 {
				workMs = 5
			}
			if workMs > 90 {
				workMs = 90
			}

			workTime := time.Duration(workMs) * time.Millisecond
			sleepTime := cycleDuration - workTime

			start := time.Now()
			// Pure CPU bound in-memory hash computation without allocation spikes
			h := sha256.New()
			for time.Since(start) < workTime {
				h.Write(dummyData)
				_ = h.Sum(nil)
				h.Reset()
			}

			if sleepTime > 0 {
				time.Sleep(sleepTime)
			}
		}
	}
}

func formatDuration(sec int) string {
	if sec <= 0 {
		return "0s"
	}
	m := sec / 60
	s := sec % 60
	if m > 0 {
		return fmt.Sprintf("%dm %02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
