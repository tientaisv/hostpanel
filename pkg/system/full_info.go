package system

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type CPUCacheInfo struct {
	Level string `json:"level"`
	Type  string `json:"type"`
	Size  string `json:"size"`
}

type CPUDetails struct {
	ModelName      string         `json:"model_name"`
	VendorID       string         `json:"vendor_id"`
	PhysicalCores  int            `json:"physical_cores"`
	LogicalThreads int            `json:"logical_threads"`
	CurFreqMHz     float64        `json:"cur_freq_mhz"`
	MinFreqMHz     float64        `json:"min_freq_mhz"`
	MaxFreqMHz     float64        `json:"max_freq_mhz"`
	Governor       string         `json:"governor"`
	Architecture   string         `json:"architecture"`
	BogoMIPS       string         `json:"bogomips"`
	Caches         []CPUCacheInfo `json:"caches"`
	KeyFlags       []string       `json:"key_flags"`
	TemperatureC   float64        `json:"temperature_c"`
}

type MemoryDetails struct {
	TotalMB        uint64  `json:"total_mb"`
	UsedMB         uint64  `json:"used_mb"`
	AvailableMB    uint64  `json:"available_mb"`
	FreeMB         uint64  `json:"free_mb"`
	BuffersMB      uint64  `json:"buffers_mb"`
	CachedMB       uint64  `json:"cached_mb"`
	Percent        float64 `json:"percent"`
	SwapTotalMB    uint64  `json:"swap_total_mb"`
	SwapUsedMB     uint64  `json:"swap_used_mb"`
	SwapFreeMB     uint64  `json:"swap_free_mb"`
	SwapPercent    float64 `json:"swap_percent"`
	Swappiness     int     `json:"swappiness"`
	HugePagesTotal int     `json:"hugepages_total"`
	HugePagesFree  int     `json:"hugepages_free"`
	HugePageSizeKB int     `json:"hugepage_size_kb"`
}

type MountPartition struct {
	Filesystem  string  `json:"filesystem"`
	FSType      string  `json:"fstype"`
	MountPoint  string  `json:"mount_point"`
	TotalGB     float64 `json:"total_gb"`
	UsedGB      float64 `json:"used_gb"`
	AvailableGB float64 `json:"available_gb"`
	Percent     float64 `json:"percent"`
}

type BlockDevice struct {
	Name       string `json:"name"`
	Size       string `json:"size"`
	Type       string `json:"type"` // disk, part, lvm
	MountPoint string `json:"mount_point"`
	Model      string `json:"model"`
	IsSSD      bool   `json:"is_ssd"`
}

type NetworkInterfaceDetail struct {
	Name         string   `json:"name"`
	MAC          string   `json:"mac"`
	IPv4         []string `json:"ipv4"`
	IPv6         []string `json:"ipv6"`
	MTU          int      `json:"mtu"`
	State        string   `json:"state"` // UP / DOWN
	Speed        string   `json:"speed"` // 1000 Mbps
	Duplex       string   `json:"duplex"`
	RxBytes      uint64   `json:"rx_bytes"`
	TxBytes      uint64   `json:"tx_bytes"`
	RxPackets    uint64   `json:"rx_packets"`
	TxPackets    uint64   `json:"tx_packets"`
	RxErrors     uint64   `json:"rx_errors"`
	TxErrors     uint64   `json:"tx_errors"`
	IsVirtual    bool     `json:"is_virtual"`
}

type FullServerInfo struct {
	// System & Host
	Hostname         string                   `json:"hostname"`
	FQDN             string                   `json:"fqdn"`
	OSPrettyName     string                   `json:"os_pretty_name"`
	OSID             string                   `json:"os_id"`
	OSVersion        string                   `json:"os_version"`
	KernelRelease    string                   `json:"kernel_release"`
	KernelArch       string                   `json:"kernel_arch"`
	Virtualization   string                   `json:"virtualization"`
	ProductModel     string                   `json:"product_model"`
	ProductVendor    string                   `json:"product_vendor"`
	BIOSVersion      string                   `json:"bios_version"`
	BIOSDate         string                   `json:"bios_date"`
	UptimeFormatted  string                   `json:"uptime_formatted"`
	UptimeSec        uint64                   `json:"uptime_sec"`
	BootTime         string                   `json:"boot_time"`
	CurrentTime      string                   `json:"current_time"`
	Timezone         string                   `json:"timezone"`
	LoadAvg1         float64                  `json:"load_1"`
	LoadAvg5         float64                  `json:"load_5"`
	LoadAvg15        float64                  `json:"load_15"`
	LoggedUsersCount int                      `json:"logged_users_count"`
	TotalProcesses   int                      `json:"total_processes"`
	FileDescriptors  string                   `json:"file_descriptors"`

	// Sub-components
	CPU              CPUDetails               `json:"cpu"`
	Memory           MemoryDetails            `json:"memory"`
	Mounts           []MountPartition         `json:"mounts"`
	BlockDevices     []BlockDevice            `json:"block_devices"`
	Interfaces       []NetworkInterfaceDetail `json:"interfaces"`
	PublicIP         string                   `json:"public_ip"`
	DefaultGateway   string                   `json:"default_gateway"`
	DNSServers       []string                 `json:"dns_servers"`

	// Runtimes & Engines
	DockerVersion    string                   `json:"docker_version"`
	PodmanVersion    string                   `json:"podman_version"`
	SystemdVersion   string                   `json:"systemd_version"`
	GoVersion        string                   `json:"go_version"`
}

var (
	cachedPublicIP     string
	lastPublicIPCheck  time.Time
)

func GetFullServerInfo() (*FullServerInfo, error) {
	info := &FullServerInfo{
		CurrentTime:  time.Now().Format("15:04:05 02/01/2006"),
		Timezone:     time.Now().Location().String(),
		GoVersion:    runtime.Version(),
		Mounts:       make([]MountPartition, 0),
		BlockDevices: make([]BlockDevice, 0),
		Interfaces:   make([]NetworkInterfaceDetail, 0),
		DNSServers:   make([]string, 0),
	}

	// 1. Basic Host Info
	if h, err := os.Hostname(); err == nil {
		info.Hostname = h
		info.FQDN = h
	}

	// Read OS Release
	parseOSRelease(info)

	// Read Kernel
	if k, err := ioutil.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		info.KernelRelease = strings.TrimSpace(string(k))
	}
	info.KernelArch = runtime.GOARCH

	// DMI System hardware info (Motherboard/Server model)
	info.ProductModel = readSysFileTrim("/sys/class/dmi/id/product_name")
	info.ProductVendor = readSysFileTrim("/sys/class/dmi/id/sys_vendor")
	info.BIOSVersion = readSysFileTrim("/sys/class/dmi/id/bios_version")
	info.BIOSDate = readSysFileTrim("/sys/class/dmi/id/bios_date")
	if info.ProductModel == "" {
		info.ProductModel = readSysFileTrim("/sys/firmware/devicetree/base/model")
	}
	if info.ProductModel == "" {
		info.ProductModel = "Generic Linux Machine"
	}
	if info.ProductVendor == "" {
		info.ProductVendor = "Unknown / Virtual"
	}

	// Virtualization Check
	info.Virtualization = detectVirtualization()

	// Uptime & Boot Time
	parseUptime(info)

	// Load averages
	if data, err := ioutil.ReadFile("/proc/loadavg"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) >= 3 {
			info.LoadAvg1, _ = strconv.ParseFloat(fields[0], 64)
			info.LoadAvg5, _ = strconv.ParseFloat(fields[1], 64)
			info.LoadAvg15, _ = strconv.ParseFloat(fields[2], 64)
		}
	}

	// File Descriptors
	if data, err := ioutil.ReadFile("/proc/sys/fs/file-nr"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) >= 3 {
			info.FileDescriptors = fmt.Sprintf("%s / %s (Max)", fields[0], fields[2])
		}
	}

	// Total Running Processes
	if procs, err := ioutil.ReadDir("/proc"); err == nil {
		cnt := 0
		for _, p := range procs {
			if p.IsDir() {
				if _, errAtoi := strconv.Atoi(p.Name()); errAtoi == nil {
					cnt++
				}
			}
		}
		info.TotalProcesses = cnt
	}

	// Logged in users
	if whoOut, err := exec.Command("who").Output(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(whoOut)), "\n")
		if len(lines) == 1 && lines[0] == "" {
			info.LoggedUsersCount = 0
		} else {
			info.LoggedUsersCount = len(lines)
		}
	}

	// 2. CPU Hardware Details
	info.CPU = parseCPUDetails()

	// 3. Memory & Swap Details
	info.Memory = parseMemoryDetails()

	// 4. Mounts & Disks
	info.Mounts = parseMounts()
	info.BlockDevices = parseBlockDevices()

	// 5. Network Interfaces & IPs
	info.Interfaces = parseNetworkInterfaces()
	info.DNSServers = parseDNSServers()
	info.DefaultGateway = parseDefaultGateway()
	info.PublicIP = fetchPublicIP()

	// 6. Installed Engines & Services
	if v, err := exec.Command("docker", "--version").Output(); err == nil {
		info.DockerVersion = strings.TrimSpace(string(v))
	}
	if v, err := exec.Command("podman", "--version").Output(); err == nil {
		info.PodmanVersion = strings.TrimSpace(string(v))
	}
	if v, err := exec.Command("systemctl", "--version").Output(); err == nil {
		lines := strings.Split(string(v), "\n")
		if len(lines) > 0 {
			info.SystemdVersion = strings.TrimSpace(lines[0])
		}
	}

	return info, nil
}

func readSysFileTrim(path string) string {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return ""
	}
	res := strings.TrimSpace(string(b))
	res = strings.ReplaceAll(res, "\x00", "")
	return res
}

func parseOSRelease(info *FullServerInfo) {
	data, err := ioutil.ReadFile("/etc/os-release")
	if err != nil {
		info.OSPrettyName = "Linux"
		return
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			info.OSPrettyName = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
		} else if strings.HasPrefix(line, "ID=") {
			info.OSID = strings.Trim(strings.TrimPrefix(line, "ID="), "\"")
		} else if strings.HasPrefix(line, "VERSION_ID=") {
			info.OSVersion = strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), "\"")
		}
	}
}

func detectVirtualization() string {
	// 1. Try systemd-detect-virt
	if out, err := exec.Command("systemd-detect-virt").Output(); err == nil {
		virt := strings.TrimSpace(string(out))
		if virt != "" && virt != "none" {
			return strings.ToUpper(virt) + " Virtual Machine"
		}
		if virt == "none" {
			return "Bare-Metal (Máy chủ vật lý)"
		}
	}

	// 2. Fallback check /proc/cpuinfo / DMI
	vendor := readSysFileTrim("/sys/class/dmi/id/sys_vendor")
	product := readSysFileTrim("/sys/class/dmi/id/product_name")
	combined := strings.ToLower(vendor + " " + product)

	if strings.Contains(combined, "kvm") || strings.Contains(combined, "qemu") {
		return "KVM / QEMU Virtual Machine"
	}
	if strings.Contains(combined, "vmware") {
		return "VMware Virtual Machine"
	}
	if strings.Contains(combined, "virtualbox") {
		return "VirtualBox VM"
	}
	if strings.Contains(combined, "amazon") || strings.Contains(combined, "ec2") {
		return "AWS EC2 Cloud Instance"
	}
	if strings.Contains(combined, "google") {
		return "Google Cloud Compute Engine"
	}
	if strings.Contains(combined, "digitalocean") {
		return "DigitalOcean Droplet"
	}

	return "Standard Host / Server"
}

func parseUptime(info *FullServerInfo) {
	data, err := ioutil.ReadFile("/proc/uptime")
	if err != nil {
		return
	}
	fields := strings.Fields(string(data))
	if len(fields) >= 1 {
		upSecFloat, _ := strconv.ParseFloat(fields[0], 64)
		upSec := uint64(upSecFloat)
		info.UptimeSec = upSec

		days := upSec / 86400
		hours := (upSec % 86400) / 3600
		mins := (upSec % 3600) / 60
		secs := upSec % 60

		parts := []string{}
		if days > 0 {
			parts = append(parts, fmt.Sprintf("%d ngày", days))
		}
		if hours > 0 {
			parts = append(parts, fmt.Sprintf("%d giờ", hours))
		}
		if mins > 0 {
			parts = append(parts, fmt.Sprintf("%d phút", mins))
		}
		parts = append(parts, fmt.Sprintf("%d giây", secs))
		info.UptimeFormatted = strings.Join(parts, " ")

		bootTime := time.Now().Add(-time.Duration(upSec) * time.Second)
		info.BootTime = bootTime.Format("15:04:05 02/01/2006")
	}
}

func parseCPUDetails() CPUDetails {
	details := CPUDetails{
		LogicalThreads: runtime.NumCPU(),
		Architecture:   runtime.GOARCH,
		Caches:         make([]CPUCacheInfo, 0),
		KeyFlags:       make([]string, 0),
	}

	// Read /proc/cpuinfo
	file, err := os.Open("/proc/cpuinfo")
	if err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		physicalMap := make(map[string]bool)

		for scanner.Scan() {
			line := scanner.Text()
			parts := strings.SplitN(line, ":", 2)
			if len(parts) < 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])

			switch key {
			case "model name":
				if details.ModelName == "" {
					details.ModelName = val
				}
			case "vendor_id":
				if details.VendorID == "" {
					details.VendorID = val
				}
			case "cpu MHz":
				if details.CurFreqMHz == 0 {
					details.CurFreqMHz, _ = strconv.ParseFloat(val, 64)
				}
			case "bogomips":
				if details.BogoMIPS == "" {
					details.BogoMIPS = val
				}
			case "core id":
				physicalMap[val] = true
			case "flags", "Features":
				if len(details.KeyFlags) == 0 {
					interesting := []string{"aes", "avx", "avx2", "avx512f", "sse4_2", "vmx", "svm", "fma", "rdrand", "asimd", "crc32"}
					flagList := strings.Fields(val)
					flagMap := make(map[string]bool)
					for _, f := range flagList {
						flagMap[strings.ToLower(f)] = true
					}
					for _, target := range interesting {
						if flagMap[target] {
							details.KeyFlags = append(details.KeyFlags, strings.ToUpper(target))
						}
					}
				}
			}
		}
		if len(physicalMap) > 0 {
			details.PhysicalCores = len(physicalMap)
		} else {
			details.PhysicalCores = details.LogicalThreads
		}
	}

	if details.ModelName == "" {
		details.ModelName = "Standard Processor"
	}

	// Read scaling frequencies
	if maxF := readSysFileTrim("/sys/devices/system/cpu/cpu0/cpufreq/scaling_max_freq"); maxF != "" {
		if val, err := strconv.ParseFloat(maxF, 64); err == nil {
			details.MaxFreqMHz = val / 1000.0
		}
	}
	if minF := readSysFileTrim("/sys/devices/system/cpu/cpu0/cpufreq/scaling_min_freq"); minF != "" {
		if val, err := strconv.ParseFloat(minF, 64); err == nil {
			details.MinFreqMHz = val / 1000.0
		}
	}
	if gov := readSysFileTrim("/sys/devices/system/cpu/cpu0/cpufreq/scaling_governor"); gov != "" {
		details.Governor = gov
	}

	// Read CPU Caches
	cacheDirs, _ := filepath.Glob("/sys/devices/system/cpu/cpu0/cache/index*")
	for _, cDir := range cacheDirs {
		level := readSysFileTrim(filepath.Join(cDir, "level"))
		cType := readSysFileTrim(filepath.Join(cDir, "type"))
		size := readSysFileTrim(filepath.Join(cDir, "size"))
		if level != "" && size != "" {
			details.Caches = append(details.Caches, CPUCacheInfo{
				Level: "L" + level,
				Type:  cType,
				Size:  size,
			})
		}
	}

	// Read CPU Temperature
	details.TemperatureC = readCPUTemperature()

	return details
}

func readCPUTemperature() float64 {
	// Try /sys/class/thermal/thermal_zone*/temp
	zones, _ := filepath.Glob("/sys/class/thermal/thermal_zone*/temp")
	for _, z := range zones {
		if data, err := ioutil.ReadFile(z); err == nil {
			if tVal, errAtoi := strconv.Atoi(strings.TrimSpace(string(data))); errAtoi == nil && tVal > 0 {
				temp := float64(tVal)
				if temp > 1000 {
					temp = temp / 1000.0
				}
				if temp > 0 && temp < 150 {
					return temp
				}
			}
		}
	}
	return 0
}

func parseMemoryDetails() MemoryDetails {
	mem := MemoryDetails{}
	file, err := os.Open("/proc/meminfo")
	if err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		var memTotal, memFree, memAvail, buffers, cached, swapTotal, swapFree uint64

		for scanner.Scan() {
			line := scanner.Text()
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				val, _ := strconv.ParseUint(fields[1], 10, 64)
				switch fields[0] {
				case "MemTotal:":
					memTotal = val
				case "MemFree:":
					memFree = val
				case "MemAvailable:":
					memAvail = val
				case "Buffers:":
					buffers = val
				case "Cached:":
					cached = val
				case "SwapTotal:":
					swapTotal = val
				case "SwapFree:":
					swapFree = val
				case "HugePages_Total:":
					mem.HugePagesTotal = int(val)
				case "HugePages_Free:":
					mem.HugePagesFree = int(val)
				case "Hugepagesize:":
					mem.HugePageSizeKB = int(val)
				}
			}
		}

		mem.TotalMB = memTotal / 1024
		mem.FreeMB = memFree / 1024
		mem.AvailableMB = memAvail / 1024
		mem.BuffersMB = buffers / 1024
		mem.CachedMB = cached / 1024

		if memTotal > memAvail {
			mem.UsedMB = (memTotal - memAvail) / 1024
		}
		if mem.TotalMB > 0 {
			mem.Percent = (float64(mem.UsedMB) / float64(mem.TotalMB)) * 100.0
		}

		mem.SwapTotalMB = swapTotal / 1024
		mem.SwapFreeMB = swapFree / 1024
		if swapTotal > swapFree {
			mem.SwapUsedMB = (swapTotal - swapFree) / 1024
		}
		if mem.SwapTotalMB > 0 {
			mem.SwapPercent = (float64(mem.SwapUsedMB) / float64(mem.SwapTotalMB)) * 100.0
		}
	}

	// Read swappiness
	if data, err := ioutil.ReadFile("/proc/sys/vm/swappiness"); err == nil {
		mem.Swappiness, _ = strconv.Atoi(strings.TrimSpace(string(data)))
	}

	return mem
}

func parseMounts() []MountPartition {
	mounts := make([]MountPartition, 0)
	file, err := os.Open("/proc/mounts")
	if err != nil {
		return mounts
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	seenMounts := make(map[string]bool)

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) >= 3 {
			dev := fields[0]
			mountPoint := fields[1]
			fstype := fields[2]

			// Filter out virtual and runtime container fs
			if !strings.HasPrefix(dev, "/dev/") {
				continue
			}
			if strings.HasPrefix(dev, "/dev/loop") {
				continue // Skip snap/loop devices
			}
			if strings.HasPrefix(mountPoint, "/var/lib/docker") || strings.HasPrefix(mountPoint, "/var/lib/containers") || strings.HasPrefix(mountPoint, "/run/docker") || strings.HasPrefix(mountPoint, "/run/user") || strings.HasPrefix(mountPoint, "/run/netns") {
				continue
			}
			if seenMounts[mountPoint] {
				continue
			}
			seenMounts[mountPoint] = true

			var stat syscall.Statfs_t
			if errStat := syscall.Statfs(mountPoint, &stat); errStat == nil && stat.Blocks > 0 {
				totalBytes := stat.Blocks * uint64(stat.Bsize)
				freeBytes := stat.Bavail * uint64(stat.Bsize)
				usedBytes := totalBytes - freeBytes

				totalGB := float64(totalBytes) / (1024 * 1024 * 1024)
				usedGB := float64(usedBytes) / (1024 * 1024 * 1024)
				availGB := float64(freeBytes) / (1024 * 1024 * 1024)
				pct := (usedGB / totalGB) * 100.0

				mounts = append(mounts, MountPartition{
					Filesystem:  dev,
					FSType:      fstype,
					MountPoint:  mountPoint,
					TotalGB:     totalGB,
					UsedGB:      usedGB,
					AvailableGB: availGB,
					Percent:     pct,
				})
			}
		}
	}

	return mounts
}

func parseBlockDevices() []BlockDevice {
	devices := make([]BlockDevice, 0)
	out, err := exec.Command("lsblk", "-J", "-o", "NAME,SIZE,TYPE,MOUNTPOINT,MODEL,ROTA").Output()
	if err == nil {
		var res struct {
			Blockdevices []struct {
				Name       string `json:"name"`
				Size       string `json:"size"`
				Type       string `json:"type"`
				MountPoint string `json:"mountpoint"`
				Model      string `json:"model"`
				Rota       *bool  `json:"rota"`
				Children   []struct {
					Name       string `json:"name"`
					Size       string `json:"size"`
					Type       string `json:"type"`
					MountPoint string `json:"mountpoint"`
				} `json:"children"`
			} `json:"blockdevices"`
		}
		if errJson := json.Unmarshal(out, &res); errJson == nil {
			for _, d := range res.Blockdevices {
				isSSD := false
				if d.Rota != nil && !*d.Rota {
					isSSD = true
				}
				devices = append(devices, BlockDevice{
					Name:       d.Name,
					Size:       d.Size,
					Type:       d.Type,
					MountPoint: d.MountPoint,
					Model:      strings.TrimSpace(d.Model),
					IsSSD:      isSSD,
				})
				for _, c := range d.Children {
					devices = append(devices, BlockDevice{
						Name:       "  └─ " + c.Name,
						Size:       c.Size,
						Type:       c.Type,
						MountPoint: c.MountPoint,
						Model:      "",
						IsSSD:      isSSD,
					})
				}
			}
		}
	}
	return devices
}

func parseNetworkInterfaces() []NetworkInterfaceDetail {
	res := make([]NetworkInterfaceDetail, 0)
	ifaces, err := net.Interfaces()
	if err != nil {
		return res
	}

	for _, iface := range ifaces {
		// Skip loopback if desired or include it
		detail := NetworkInterfaceDetail{
			Name:      iface.Name,
			MAC:       iface.HardwareAddr.String(),
			MTU:       iface.MTU,
			IPv4:      make([]string, 0),
			IPv6:      make([]string, 0),
			IsVirtual: strings.HasPrefix(iface.Name, "veth") || strings.HasPrefix(iface.Name, "br-") || strings.HasPrefix(iface.Name, "docker") || strings.HasPrefix(iface.Name, "podman"),
		}

		if (iface.Flags & net.FlagUp) != 0 {
			detail.State = "UP"
		} else {
			detail.State = "DOWN"
		}

		addrs, errAddr := iface.Addrs()
		if errAddr == nil {
			for _, a := range addrs {
				ipNet, ok := a.(*net.IPNet)
				if ok && !ipNet.IP.IsLoopback() {
					if ipNet.IP.To4() != nil {
						detail.IPv4 = append(detail.IPv4, ipNet.String())
					} else {
						detail.IPv6 = append(detail.IPv6, ipNet.String())
					}
				}
			}
		}

		// Read statistics & speed from /sys/class/net/<iface>/
		sysNet := filepath.Join("/sys/class/net", iface.Name)
		if speed := readSysFileTrim(filepath.Join(sysNet, "speed")); speed != "" && speed != "-1" {
			detail.Speed = speed + " Mbps"
		}
		if duplex := readSysFileTrim(filepath.Join(sysNet, "duplex")); duplex != "" {
			detail.Duplex = strings.ToUpper(duplex)
		}

		// Stats
		detail.RxBytes = readUintFile(filepath.Join(sysNet, "statistics/rx_bytes"))
		detail.TxBytes = readUintFile(filepath.Join(sysNet, "statistics/tx_bytes"))
		detail.RxPackets = readUintFile(filepath.Join(sysNet, "statistics/rx_packets"))
		detail.TxPackets = readUintFile(filepath.Join(sysNet, "statistics/tx_packets"))
		detail.RxErrors = readUintFile(filepath.Join(sysNet, "statistics/rx_errors"))
		detail.TxErrors = readUintFile(filepath.Join(sysNet, "statistics/tx_errors"))

		res = append(res, detail)
	}

	return res
}

func readUintFile(path string) uint64 {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return 0
	}
	val, _ := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	return val
}

func parseDNSServers() []string {
	servers := make([]string, 0)
	file, err := os.Open("/etc/resolv.conf")
	if err != nil {
		return servers
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "nameserver") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				servers = append(servers, fields[1])
			}
		}
	}
	return servers
}

func parseDefaultGateway() string {
	file, err := os.Open("/proc/net/route")
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 3 && fields[1] == "00000000" { // Destination 0.0.0.0
			gwHex := fields[2]
			if len(gwHex) == 8 {
				d, _ := strconv.ParseInt(gwHex[0:2], 16, 64)
				c, _ := strconv.ParseInt(gwHex[2:4], 16, 64)
				b, _ := strconv.ParseInt(gwHex[4:6], 16, 64)
				a, _ := strconv.ParseInt(gwHex[6:8], 16, 64)
				return fmt.Sprintf("%d.%d.%d.%d (%s)", a, b, c, d, fields[0])
			}
		}
	}
	return ""
}

func init() {
	go updatePublicIPAsync()
}

func fetchPublicIP() string {
	if cachedPublicIP != "" {
		if time.Since(lastPublicIPCheck) > 10*time.Minute {
			go updatePublicIPAsync()
		}
		return cachedPublicIP
	}

	go updatePublicIPAsync()
	return "Đang xác định..."
}

func updatePublicIPAsync() {
	client := &http.Client{Timeout: 2 * time.Second}
	endpoints := []string{"https://api.ipify.org", "https://ifconfig.me/ip", "https://icanhazip.com"}
	for _, ep := range endpoints {
		resp, err := client.Get(ep)
		if err == nil && resp.StatusCode == 200 {
			body, _ := ioutil.ReadAll(resp.Body)
			resp.Body.Close()
			ip := strings.TrimSpace(string(body))
			if net.ParseIP(ip) != nil {
				cachedPublicIP = ip
				lastPublicIPCheck = time.Now()
				return
			}
		}
	}
}
