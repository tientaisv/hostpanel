package system

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type ProcessInfo struct {
	PID        int     `json:"pid"`
	User       string  `json:"user"`
	Name       string  `json:"name"`
	Cmdline    string  `json:"cmdline"`
	State      string  `json:"state"`
	MemRSSMB   float64 `json:"mem_rss_mb"`
	MemPercent float64 `json:"mem_percent"`
	CPUPercent float64 `json:"cpu_percent"`
}

type ProcessListOptions struct {
	SortBy string `json:"sort_by"` // "cpu" or "memory"
	Limit  int    `json:"limit"`
}

type procStatSample struct {
	totalTicks float64
	sysTotal   float64
	updated    time.Time
}

var (
	procCache     = make(map[int]procStatSample)
	procCacheLock sync.Mutex
)

func getSystemTotalTicks() float64 {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 5 && fields[0] == "cpu" {
			var total float64
			for _, f := range fields[1:] {
				val, _ := strconv.ParseFloat(f, 64)
				total += val
			}
			return total
		}
	}
	return 0
}

func getSystemUptime() float64 {
	data, err := ioutil.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) >= 1 {
		up, _ := strconv.ParseFloat(fields[0], 64)
		return up
	}
	return 0
}

func GetRunningProcesses(opts ProcessListOptions) ([]ProcessInfo, error) {
	if opts.Limit <= 0 {
		opts.Limit = 50
	}
	if opts.SortBy == "" {
		opts.SortBy = "cpu"
	}

	memTotalMB := float64(1)
	if memTotal, _, _, _ := readMemStats(); memTotal > 0 {
		memTotalMB = float64(memTotal) / 1024.0
	}

	sysTotalTicks := getSystemTotalTicks()

	files, err := ioutil.ReadDir("/proc")
	if err != nil {
		return nil, err
	}

	var procs []ProcessInfo
	uidCache := make(map[string]string)

	for _, f := range files {
		if !f.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(f.Name())
		if err != nil {
			// Not a PID directory
			continue
		}

		proc, err := readProcessDetails(pid, memTotalMB, uidCache, sysTotalTicks)
		if err == nil && proc != nil {
			procs = append(procs, *proc)
		}
	}

	// Clean up dead PIDs from cache
	activePIDs := make(map[int]bool, len(procs))
	for _, p := range procs {
		activePIDs[p.PID] = true
	}
	procCacheLock.Lock()
	for pid := range procCache {
		if !activePIDs[pid] {
			delete(procCache, pid)
		}
	}
	procCacheLock.Unlock()

	// Sort processes
	if opts.SortBy == "memory" {
		sort.Slice(procs, func(i, j int) bool {
			return procs[i].MemRSSMB > procs[j].MemRSSMB
		})
	} else {
		// Sort by CPU (or fallback to RAM if CPU is zero)
		sort.Slice(procs, func(i, j int) bool {
			if procs[i].CPUPercent == procs[j].CPUPercent {
				return procs[i].MemRSSMB > procs[j].MemRSSMB
			}
			return procs[i].CPUPercent > procs[j].CPUPercent
		})
	}

	if len(procs) > opts.Limit {
		procs = procs[:opts.Limit]
	}

	return procs, nil
}

func readProcessDetails(pid int, memTotalMB float64, uidCache map[string]string, sysTotal float64) (*ProcessInfo, error) {
	statusPath := fmt.Sprintf("/proc/%d/status", pid)
	statusFile, err := os.Open(statusPath)
	if err != nil {
		return nil, err
	}
	defer statusFile.Close()

	var name, state, uidStr string
	var rssKB float64

	scanner := bufio.NewScanner(statusFile)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			switch fields[0] {
			case "Name:":
				name = fields[1]
			case "State:":
				state = fields[1]
			case "Uid:":
				uidStr = fields[1]
			case "VmRSS:":
				rssKB, _ = strconv.ParseFloat(fields[1], 64)
			}
		}
	}

	// Username lookup
	userName := uidCache[uidStr]
	if userName == "" && uidStr != "" {
		if u, errU := user.LookupId(uidStr); errU == nil {
			userName = u.Username
		} else {
			userName = uidStr
		}
		uidCache[uidStr] = userName
	}

	// Cmdline
	cmdline := name
	cmdBytes, errCmd := ioutil.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if errCmd == nil && len(cmdBytes) > 0 {
		cmdStr := strings.ReplaceAll(string(cmdBytes), "\x00", " ")
		cmdStr = strings.TrimSpace(cmdStr)
		if cmdStr != "" {
			cmdline = cmdStr
		}
	}

	// Stat for CPU calculation
	var cpuPct float64
	statBytes, errStat := ioutil.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if errStat == nil {
		statStr := string(statBytes)
		idx := strings.LastIndex(statStr, ")")
		if idx != -1 && len(statStr) > idx+2 {
			fields := strings.Fields(statStr[idx+2:])
			if len(fields) >= 13 {
				utime, _ := strconv.ParseFloat(fields[11], 64)
				stime, _ := strconv.ParseFloat(fields[12], 64)
				totalTicks := utime + stime

				now := time.Now()
				procCacheLock.Lock()
				prev, exists := procCache[pid]
				procCache[pid] = procStatSample{
					totalTicks: totalTicks,
					sysTotal:   sysTotal,
					updated:    now,
				}
				procCacheLock.Unlock()

				if exists && sysTotal > prev.sysTotal && totalTicks >= prev.totalTicks {
					deltaProc := totalTicks - prev.totalTicks
					deltaSys := sysTotal - prev.sysTotal
					if deltaSys > 0 {
						cpuPct = (deltaProc / deltaSys) * 100.0
					}
				} else if len(fields) >= 20 {
					// Fallback for initial sample: calculate lifetime average CPU %
					startTimeTicks, _ := strconv.ParseFloat(fields[19], 64)
					if sysUptime := getSystemUptime(); sysUptime > 0 {
						clkTck := 100.0
						procUptimeSec := sysUptime - (startTimeTicks / clkTck)
						procCpuSec := totalTicks / clkTck
						if procUptimeSec > 0 {
							numCPU := float64(runtime.NumCPU())
							if numCPU > 0 {
								cpuPct = (procCpuSec / procUptimeSec / numCPU) * 100.0
							} else {
								cpuPct = (procCpuSec / procUptimeSec) * 100.0
							}
						}
					}
				}

				if cpuPct > 100.0 {
					cpuPct = 100.0
				} else if cpuPct < 0 {
					cpuPct = 0
				}
			}
		}
	}

	rssMB := rssKB / 1024.0
	memPct := (rssMB / memTotalMB) * 100.0

	return &ProcessInfo{
		PID:        pid,
		User:       userName,
		Name:       name,
		Cmdline:    cmdline,
		State:      state,
		MemRSSMB:   rssMB,
		MemPercent: memPct,
		CPUPercent: cpuPct,
	}, nil
}

func KillProcess(pid int, signal int) error {
	if signal <= 0 {
		signal = 9 // SIGKILL default
	}
	return syscall.Kill(pid, syscall.Signal(signal))
}
