package system

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type CPUCoreInfo struct {
	CoreID  int     `json:"core_id"`
	Percent float64 `json:"percent"`
}

type HostStats struct {
	Hostname           string        `json:"hostname"`
	OSInfo             string        `json:"os_info"`
	KernelVersion      string        `json:"kernel_version"`
	CPUModel           string        `json:"cpu_model"`
	CoresCount         int           `json:"cores_count"`
	CPUCorePercents    []CPUCoreInfo `json:"cpu_core_percents"`
	CPUPercent         float64       `json:"cpu_percent"`
	MemTotalMB         uint64        `json:"mem_total_mb"`
	MemUsedMB          uint64        `json:"mem_used_mb"`
	MemPercent         float64       `json:"mem_percent"`
	SwapTotalMB        uint64        `json:"swap_total_mb"`
	SwapUsedMB         uint64        `json:"swap_used_mb"`
	SwapPercent        float64       `json:"swap_percent"`
	DiskTotalGB        uint64        `json:"disk_total_gb"`
	DiskUsedGB         uint64        `json:"disk_used_gb"`
	DiskPercent        float64       `json:"disk_percent"`
	NetRxKB            float64       `json:"net_rx_kb"`
	NetTxKB            float64       `json:"net_tx_kb"`
	DiskReadMB         float64       `json:"disk_read_mb"`
	DiskWriteMB        float64       `json:"disk_write_mb"`
	NetRxRateKB        float64       `json:"net_rx_rate_kb"`
	NetTxRateKB        float64       `json:"net_tx_rate_kb"`
	DiskReadRateMB     float64       `json:"disk_read_rate_mb"`
	DiskWriteRateMB    float64       `json:"disk_write_rate_mb"`
	LoadAvg1           float64       `json:"load_1"`
	LoadAvg5           float64       `json:"load_5"`
	LoadAvg15          float64       `json:"load_15"`
	UptimeSec          uint64        `json:"uptime_sec"`
	Containers         int           `json:"containers_count"`
	DockerRunningCount int           `json:"docker_running_count"`
	DockerCPUPercent   float64       `json:"docker_cpu_percent"`
	DockerMemUsedMB    float64       `json:"docker_mem_used_mb"`
	DockerMemPercent   float64       `json:"docker_mem_percent"`
	DockerNetRxMB      float64       `json:"docker_net_rx_mb"`
	DockerNetTxMB      float64       `json:"docker_net_tx_mb"`
	EngineName         string        `json:"engine_name"`
	EngineVersion      string        `json:"engine_version"`
	IsPodman           bool          `json:"is_podman"`
}

type ContainerStats struct {
	CPUPercent   float64 `json:"cpu_percent"`
	MemUsageMB   float64 `json:"mem_usage_mb"`
	MemLimitMB   float64 `json:"mem_limit_mb"`
	MemPercent   float64 `json:"mem_percent"`
	NetRxMB      float64 `json:"net_rx_mb"`
	NetTxMB      float64 `json:"net_tx_mb"`
	BlockReadMB  float64 `json:"block_read_mb"`
	BlockWriteMB float64 `json:"block_write_mb"`
}

type coreRaw struct {
	idle  uint64
	total uint64
}

var (
	lastIdleTime, lastTotalTime     uint64
	lastCoreStats                   map[int]coreRaw
	lastNetRxKB, lastNetTxKB        float64
	lastDiskReadMB, lastDiskWriteMB float64
	lastStatsTime                   time.Time

	cachedHostname string
	cachedOSInfo   string
	cachedKernel   string
	cachedCPUModel string
	cachedCores    int
)

func getStaticServerInfo() (string, string, string, string, int) {
	if cachedHostname == "" {
		if h, err := os.Hostname(); err == nil {
			cachedHostname = h
		} else {
			cachedHostname = "Unknown Host"
		}

		cachedOSInfo = "Linux"
		if data, err := ioutil.ReadFile("/etc/os-release"); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(line, "PRETTY_NAME=") {
					cachedOSInfo = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
					break
				}
			}
		}

		if data, err := ioutil.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
			cachedKernel = strings.TrimSpace(string(data))
		} else {
			cachedKernel = "Linux"
		}

		if data, err := ioutil.ReadFile("/proc/cpuinfo"); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(line, "model name") {
					parts := strings.Split(line, ":")
					if len(parts) >= 2 {
						cachedCPUModel = strings.TrimSpace(parts[1])
						break
					}
				}
			}
		}
		if cachedCPUModel == "" {
			cachedCPUModel = "Standard Processor"
		}

		cachedCores = runtime.NumCPU()
	}
	return cachedHostname, cachedOSInfo, cachedKernel, cachedCPUModel, cachedCores
}

func GetHostStats() (*HostStats, error) {
	stats := &HostStats{}

	// Server Static Info
	stats.Hostname, stats.OSInfo, stats.KernelVersion, stats.CPUModel, stats.CoresCount = getStaticServerInfo()

	// CPU % & Per-Core %
	cpuPct, idle, total, coreInfos := readCPUStats()
	if lastTotalTime > 0 && total > lastTotalTime {
		totalDelta := float64(total - lastTotalTime)
		idleDelta := float64(idle - lastIdleTime)
		if totalDelta > 0 {
			cpuPct = (1.0 - (idleDelta / totalDelta)) * 100.0
		}
	}
	lastIdleTime = idle
	lastTotalTime = total
	stats.CPUPercent = cpuPct
	stats.CPUCorePercents = coreInfos

	// Mem & Swap info
	memTotal, memFree, memBuffers, memCached, swapTotal, swapFree := readMemAndSwapStats()
	if memTotal > 0 {
		memUsed := memTotal - (memFree + memBuffers + memCached)
		stats.MemTotalMB = memTotal / 1024
		stats.MemUsedMB = memUsed / 1024
		stats.MemPercent = (float64(memUsed) / float64(memTotal)) * 100.0
	}
	if swapTotal > 0 {
		swapUsed := swapTotal - swapFree
		stats.SwapTotalMB = swapTotal / 1024
		stats.SwapUsedMB = swapUsed / 1024
		stats.SwapPercent = (float64(swapUsed) / float64(swapTotal)) * 100.0
	}

	// Disk info (root mount /)
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err == nil {
		totalBytes := stat.Blocks * uint64(stat.Bsize)
		freeBytes := stat.Bfree * uint64(stat.Bsize)
		usedBytes := totalBytes - freeBytes

		stats.DiskTotalGB = totalBytes / (1024 * 1024 * 1024)
		stats.DiskUsedGB = usedBytes / (1024 * 1024 * 1024)
		if totalBytes > 0 {
			stats.DiskPercent = (float64(usedBytes) / float64(totalBytes)) * 100.0
		}
	}

	now := time.Now()
	deltaSec := 2.0
	if !lastStatsTime.IsZero() {
		if d := now.Sub(lastStatsTime).Seconds(); d > 0 {
			deltaSec = d
		}
	}

	// Network I/O
	stats.NetRxKB, stats.NetTxKB = readNetDevStats()
	if lastNetRxKB > 0 && stats.NetRxKB >= lastNetRxKB {
		stats.NetRxRateKB = (stats.NetRxKB - lastNetRxKB) / deltaSec
	}
	if lastNetTxKB > 0 && stats.NetTxKB >= lastNetTxKB {
		stats.NetTxRateKB = (stats.NetTxKB - lastNetTxKB) / deltaSec
	}
	lastNetRxKB = stats.NetRxKB
	lastNetTxKB = stats.NetTxKB

	// Disk I/O
	stats.DiskReadMB, stats.DiskWriteMB = readDiskIOStats()
	if lastDiskReadMB > 0 && stats.DiskReadMB >= lastDiskReadMB {
		stats.DiskReadRateMB = (stats.DiskReadMB - lastDiskReadMB) / deltaSec
	}
	if lastDiskWriteMB > 0 && stats.DiskWriteMB >= lastDiskWriteMB {
		stats.DiskWriteRateMB = (stats.DiskWriteMB - lastDiskWriteMB) / deltaSec
	}
	lastDiskReadMB = stats.DiskReadMB
	lastDiskWriteMB = stats.DiskWriteMB
	lastStatsTime = now

	// Load Average
	if data, err := ioutil.ReadFile("/proc/loadavg"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) >= 3 {
			stats.LoadAvg1, _ = strconv.ParseFloat(fields[0], 64)
			stats.LoadAvg5, _ = strconv.ParseFloat(fields[1], 64)
			stats.LoadAvg15, _ = strconv.ParseFloat(fields[2], 64)
		}
	}

	// Uptime
	if data, err := ioutil.ReadFile("/proc/uptime"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) >= 1 {
			upFloat, _ := strconv.ParseFloat(fields[0], 64)
			stats.UptimeSec = uint64(upFloat)
		}
	}

	return stats, nil
}

func readCPUStats() (float64, uint64, uint64, []CPUCoreInfo) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, 0, nil
	}
	defer file.Close()

	var totalIdle, totalTime uint64
	currentCores := make(map[int]coreRaw)
	coreInfos := make([]CPUCoreInfo, 0)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		if fields[0] == "cpu" {
			for i, field := range fields[1:] {
				val, _ := strconv.ParseUint(field, 10, 64)
				totalTime += val
				if i == 3 { // 4th field is idle
					totalIdle = val
				}
			}
		} else if strings.HasPrefix(fields[0], "cpu") {
			coreIDStr := strings.TrimPrefix(fields[0], "cpu")
			coreID, errCore := strconv.Atoi(coreIDStr)
			if errCore == nil {
				var cTotal, cIdle uint64
				for i, field := range fields[1:] {
					val, _ := strconv.ParseUint(field, 10, 64)
					cTotal += val
					if i == 3 {
						cIdle = val
					}
				}
				currentCores[coreID] = coreRaw{idle: cIdle, total: cTotal}

				var pct float64
				if lastCoreStats != nil {
					if old, ok := lastCoreStats[coreID]; ok && cTotal > old.total {
						tDelta := float64(cTotal - old.total)
						iDelta := float64(cIdle - old.idle)
						if tDelta > 0 {
							pct = (1.0 - (iDelta / tDelta)) * 100.0
							if pct < 0 {
								pct = 0
							}
						}
					}
				}
				coreInfos = append(coreInfos, CPUCoreInfo{
					CoreID:  coreID,
					Percent: pct,
				})
			}
		}
	}
	lastCoreStats = currentCores
	return 0, totalIdle, totalTime, coreInfos
}

func ResetSwap() (string, error) {
	_, memFree, memBuffers, memCached, swapTotal, swapFree := readMemAndSwapStats()
	if swapTotal == 0 {
		return "", fmt.Errorf("Hệ thống không cài đặt bộ nhớ Swap (SwapTotal = 0)")
	}

	swapUsed := swapTotal - swapFree
	swapUsedMB := swapUsed / 1024
	if swapUsed == 0 {
		return "Swap hiện tại đang trống (0 MB). Không cần reset!", nil
	}

	// Calculate RAM available: free + buffers + cached
	ramAvail := memFree + memBuffers + memCached
	ramAvailMB := ramAvail / 1024

	if ramAvail <= swapUsed {
		return "", fmt.Errorf("Không đủ RAM trống để reset Swap an toàn! Swap đang sử dụng: %d MB, trong khi RAM khả dụng chỉ có: %d MB. Vui lòng giải phóng RAM trước.", swapUsedMB, ramAvailMB)
	}

	// 1. Flush/drop page cache to free up contiguous physical RAM before swapoff
	_ = exec.Command("sysctl", "-w", "vm.drop_caches=3").Run()

	// 2. Execute swapoff -a
	outOff, errOff := exec.Command("swapoff", "-a").CombinedOutput()
	if errOff != nil {
		// Fallback: try specifying active swap files directly from /proc/swaps
		outOff2, errOff2 := exec.Command("bash", "-c", "for s in $(tail -n +2 /proc/swaps | awk '{print $1}'); do swapoff \"$s\" || exit 1; done").CombinedOutput()
		if errOff2 != nil {
			return "", fmt.Errorf("Không thể giải phóng Swap: %v. Chi tiết: %s %s. Đảm bảo server có đủ RAM khả dụng.", errOff, strings.TrimSpace(string(outOff)), strings.TrimSpace(string(outOff2)))
		}
	}

	// 3. Execute swapon -a
	outOn, errOn := exec.Command("swapon", "-a").CombinedOutput()
	if errOn != nil {
		return "", fmt.Errorf("Đã giải phóng Swap nhưng gặp lỗi khi bật lại Swap (swapon -a): %v (%s)", errOn, strings.TrimSpace(string(outOn)))
	}

	// 4. Restart zram services if present to ensure zram swap partitions are re-enabled
	_ = exec.Command("systemctl", "restart", "zram-config").Run()
	_ = exec.Command("systemctl", "restart", "zramswap").Run()

	return fmt.Sprintf("Reset Swap thành công! Đã chuyển %d MB từ Swap trở lại RAM.", swapUsedMB), nil
}

func RunPwmConfig(channel int, speed int) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "pwmconfig", "-c", strconv.Itoa(channel), "-s", strconv.Itoa(speed))
	out, err := cmd.CombinedOutput()
	outStr := strings.TrimSpace(string(out))

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return outStr, fmt.Errorf("Lệnh thực thi bị quá thời gian (timeout 15s)")
		}
		if outStr != "" {
			return outStr, fmt.Errorf("%s", outStr)
		}
		return "", err
	}

	if outStr == "" {
		outStr = fmt.Sprintf("Lệnh 'pwmconfig -c %d -s %d' đã thực thi thành công.", channel, speed)
	}
	return outStr, nil
}

func readMemAndSwapStats() (uint64, uint64, uint64, uint64, uint64, uint64) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, 0, 0, 0, 0
	}
	defer file.Close()

	var total, free, buffers, cached, swapTotal, swapFree uint64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			val, _ := strconv.ParseUint(fields[1], 10, 64)
			switch fields[0] {
			case "MemTotal:":
				total = val
			case "MemFree:":
				free = val
			case "Buffers:":
				buffers = val
			case "Cached:":
				cached = val
			case "SwapTotal:":
				swapTotal = val
			case "SwapFree:":
				swapFree = val
			}
		}
	}
	return total, free, buffers, cached, swapTotal, swapFree
}

func readMemStats() (uint64, uint64, uint64, uint64) {
	t, f, b, c, _, _ := readMemAndSwapStats()
	return t, f, b, c
}

func readNetDevStats() (float64, float64) {
	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return 0, 0
	}
	defer file.Close()

	var rxTotal, txTotal uint64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			ifName := strings.TrimSpace(parts[0])
			if ifName == "lo" {
				continue
			}
			fields := strings.Fields(parts[1])
			if len(fields) >= 9 {
				rx, _ := strconv.ParseUint(fields[0], 10, 64)
				tx, _ := strconv.ParseUint(fields[8], 10, 64)
				rxTotal += rx
				txTotal += tx
			}
		}
	}
	return float64(rxTotal) / 1024.0, float64(txTotal) / 1024.0
}

func readDiskIOStats() (float64, float64) {
	file, err := os.Open("/proc/diskstats")
	if err != nil {
		return 0, 0
	}
	defer file.Close()

	var readSectors, writeSectors uint64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 10 {
			devName := fields[2]
			if strings.HasPrefix(devName, "sd") || strings.HasPrefix(devName, "vd") || strings.HasPrefix(devName, "nvme") {
				rSec, _ := strconv.ParseUint(fields[5], 10, 64)
				wSec, _ := strconv.ParseUint(fields[9], 10, 64)
				readSectors += rSec
				writeSectors += wSec
			}
		}
	}
	readMB := float64(readSectors*512) / (1024 * 1024)
	writeMB := float64(writeSectors*512) / (1024 * 1024)
	return readMB, writeMB
}

type RawDockerStats struct {
	ID       string `json:"Id"`
	Name     string `json:"Name"`
	CPUStats struct {
		CPUUsage struct {
			TotalUsage  uint64   `json:"total_usage"`
			PercpuUsage []uint64 `json:"percpu_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs     int    `json:"online_cpus"`
	} `json:"cpu_stats"`
	PreCPUStats struct {
		CPUUsage struct {
			TotalUsage  uint64   `json:"total_usage"`
			PercpuUsage []uint64 `json:"percpu_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs     int    `json:"online_cpus"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64 `json:"usage"`
		Limit uint64 `json:"limit"`
		Stats struct {
			InactiveFile uint64 `json:"inactive_file"`
			Cache        uint64 `json:"cache"`
		} `json:"stats"`
	} `json:"memory_stats"`
	Networks map[string]struct {
		RxBytes uint64 `json:"rx_bytes"`
		TxBytes uint64 `json:"tx_bytes"`
	} `json:"networks"`
	BlkioStats struct {
		IOServiceBytesRecursive []struct {
			Op    string `json:"op"`
			Value uint64 `json:"value"`
		} `json:"io_service_bytes_recursive"`
	} `json:"blkio_stats"`
}

type containerCPUSample struct {
	totalUsage  uint64
	systemUsage uint64
	recordedAt  time.Time
}

var (
	containerCPUMap = make(map[string]containerCPUSample)
	containerCPUMu  = &sync.Mutex{}
)

func ParseDockerStats(body []byte) (*ContainerStats, error) {
	var raw RawDockerStats
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}

	cs := &ContainerStats{}

	// Online CPUs count
	numCPUs := raw.CPUStats.OnlineCPUs
	if numCPUs <= 0 {
		numCPUs = len(raw.CPUStats.CPUUsage.PercpuUsage)
	}
	if numCPUs <= 0 {
		numCPUs = runtime.NumCPU()
	}
	if numCPUs <= 0 {
		numCPUs = 1
	}

	now := time.Now()
	containerID := raw.ID
	if containerID == "" {
		containerID = raw.Name
	}

	containerCPUMu.Lock()
	lastSample, hasLast := containerCPUMap[containerID]
	containerCPUMap[containerID] = containerCPUSample{
		totalUsage:  raw.CPUStats.CPUUsage.TotalUsage,
		systemUsage: raw.CPUStats.SystemCPUUsage,
		recordedAt:  now,
	}
	containerCPUMu.Unlock()

	// CPU Delta calculation
	var cpuDelta, systemDelta float64
	if raw.PreCPUStats.CPUUsage.TotalUsage > 0 && raw.PreCPUStats.SystemCPUUsage > 0 {
		cpuDelta = float64(raw.CPUStats.CPUUsage.TotalUsage - raw.PreCPUStats.CPUUsage.TotalUsage)
		systemDelta = float64(raw.CPUStats.SystemCPUUsage - raw.PreCPUStats.SystemCPUUsage)
	} else if hasLast {
		if raw.CPUStats.CPUUsage.TotalUsage >= lastSample.totalUsage {
			cpuDelta = float64(raw.CPUStats.CPUUsage.TotalUsage - lastSample.totalUsage)
		}
		if raw.CPUStats.SystemCPUUsage > lastSample.systemUsage {
			systemDelta = float64(raw.CPUStats.SystemCPUUsage - lastSample.systemUsage)
		} else {
			// Fallback to elapsed wall-clock nanoseconds
			elapsedNs := now.Sub(lastSample.recordedAt).Nanoseconds()
			if elapsedNs > 0 {
				systemDelta = float64(elapsedNs)
			}
		}
	}

	if systemDelta > 0 && cpuDelta > 0 {
		cs.CPUPercent = (cpuDelta / systemDelta) * float64(numCPUs) * 100.0
		if cs.CPUPercent < 0 {
			cs.CPUPercent = 0
		}
	}

	// Memory Stats
	memUsage := raw.MemoryStats.Usage
	if raw.MemoryStats.Stats.InactiveFile > 0 && memUsage > raw.MemoryStats.Stats.InactiveFile {
		memUsage -= raw.MemoryStats.Stats.InactiveFile
	}
	cs.MemUsageMB = float64(memUsage) / (1024 * 1024)
	cs.MemLimitMB = float64(raw.MemoryStats.Limit) / (1024 * 1024)
	if raw.MemoryStats.Limit > 0 {
		cs.MemPercent = (float64(memUsage) / float64(raw.MemoryStats.Limit)) * 100.0
		if cs.MemPercent > 100.0 {
			cs.MemPercent = 100.0
		}
	}

	// Network I/O
	var totalRx, totalTx uint64
	for _, n := range raw.Networks {
		totalRx += n.RxBytes
		totalTx += n.TxBytes
	}
	cs.NetRxMB = float64(totalRx) / (1024 * 1024)
	cs.NetTxMB = float64(totalTx) / (1024 * 1024)

	// Block I/O
	for _, ioItem := range raw.BlkioStats.IOServiceBytesRecursive {
		op := strings.ToLower(ioItem.Op)
		if strings.Contains(op, "read") {
			cs.BlockReadMB += float64(ioItem.Value) / (1024 * 1024)
		} else if strings.Contains(op, "write") {
			cs.BlockWriteMB += float64(ioItem.Value) / (1024 * 1024)
		}
	}

	return cs, nil
}

