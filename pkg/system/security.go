package system

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type SecurityThreatItem struct {
	Level       string `json:"level"`       // "CRITICAL", "WARNING", "INFO"
	Category    string `json:"category"`    // "SSH Auth", "Docker Security", "Process Threat", "Port Risk"
	Title       string `json:"title"`
	Description string `json:"description"`
	IpAddress   string `json:"ip_address,omitempty"`
	ActionHint  string `json:"action_hint,omitempty"`
}

type FailedLoginIP struct {
	IP    string `json:"ip"`
	Count int    `json:"count"`
}

type SecurityReport struct {
	ThreatLevel          string               `json:"threat_level"` // "SECURE", "WARNING", "CRITICAL"
	ThreatScore          int                  `json:"threat_score"` // 0-100
	FailedLoginsCount    int                  `json:"failed_logins_count"`
	FailedIPs            []FailedLoginIP      `json:"failed_ips"`
	PrivilegedCtrsCount  int                  `json:"privileged_ctrs_count"`
	SuspiciousProcsCount int                  `json:"suspicious_procs_count"`
	ExposedPortsCount    int                  `json:"exposed_ports_count"`
	Threats              []SecurityThreatItem `json:"threats"`
	ScanTime             string               `json:"scan_time"`
}

func PerformSecurityAudit() (*SecurityReport, error) {
	report := &SecurityReport{
		ThreatLevel: "SECURE",
		ThreatScore: 0,
		Threats:     make([]SecurityThreatItem, 0),
		FailedIPs:   make([]FailedLoginIP, 0),
		ScanTime:    time.Now().Format("15:04:05 02/01/2006"),
	}

	// 1. Audit SSH & System Auth Logs for Brute Force Attempts
	auditAuthLogs(report)

	// 2. Audit Docker Security Risks (Privileged containers & Socket mounts)
	auditDockerSecurity(report)

	// 3. Audit Suspicious High Resource / Miner Processes
	auditProcessThreats(report)

	// 4. Audit Open Exposed Listening Ports
	auditPortSecurity(report)

	// Determine overall Threat Level
	if report.ThreatScore >= 60 {
		report.ThreatLevel = "CRITICAL"
	} else if report.ThreatScore >= 20 {
		report.ThreatLevel = "WARNING"
	} else {
		report.ThreatLevel = "SECURE"
	}

	return report, nil
}

func auditAuthLogs(report *SecurityReport) {
	logPaths := []string{
		"/var/log/auth.log",
		"/var/log/secure",
	}

	ipMap := make(map[string]int)
	totalFailed := 0

	ipRegex := regexp.MustCompile(`from\s+([0-9]+\.[0-9]+\.[0-9]+\.[0-9]+)`)

	for _, path := range logPaths {
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "Failed password") || strings.Contains(line, "authentication failure") || strings.Contains(line, "Invalid user") {
				totalFailed++
				matches := ipRegex.FindStringSubmatch(line)
				if len(matches) > 1 {
					ipMap[matches[1]]++
				}
			}
		}
		file.Close()
	}

	// Fallback to journalctl if log files unreadable
	if totalFailed == 0 {
		out, err := exec.Command("journalctl", "-u", "sshd", "-n", "200", "--no-pager").Output()
		if err == nil {
			scanner := bufio.NewScanner(strings.NewReader(string(out)))
			for scanner.Scan() {
				line := scanner.Text()
				if strings.Contains(line, "Failed password") || strings.Contains(line, "Invalid user") {
					totalFailed++
					matches := ipRegex.FindStringSubmatch(line)
					if len(matches) > 1 {
						ipMap[matches[1]]++
					}
				}
			}
		}
	}

	report.FailedLoginsCount = totalFailed

	for ip, count := range ipMap {
		report.FailedIPs = append(report.FailedIPs, FailedLoginIP{IP: ip, Count: count})
		if count >= 10 {
			report.ThreatScore += 15
			report.Threats = append(report.Threats, SecurityThreatItem{
				Level:       "WARNING",
				Category:    "SSH Auth",
				Title:       fmt.Sprintf("Phát hiện IP dò mật khẩu SSH: %s", ip),
				Description: fmt.Sprintf("Địa chỉ IP %s đã thử đăng nhập thất bại %d lần.", ip, count),
				IpAddress:   ip,
				ActionHint:  "Khuyến nghị cài đặt Fail2ban hoặc chặn IP bằng Firewall",
			})
		}
	}

	if totalFailed > 50 {
		report.ThreatScore += 25
		report.Threats = append(report.Threats, SecurityThreatItem{
			Level:       "CRITICAL",
			Category:    "SSH Auth",
			Title:       "Cảnh báo tấn công Brute-Force SSH diện rộng",
			Description: fmt.Sprintf("Ghi nhận tổng cộng %d lần thử đăng nhập thất bại trên hệ thống.", totalFailed),
			ActionHint:  "Đổi cổng SSH mặc định 22 và tắt Password Authentication",
		})
	}
}

func auditDockerSecurity(report *SecurityReport) {
	// Inspect running container configs via podman or docker CLI
	var out []byte
	var err error
	cliName := "podman"
	out, err = exec.Command("podman", "ps", "-q").Output()
	if err != nil || len(out) == 0 {
		cliName = "docker"
		out, err = exec.Command("docker", "ps", "-q").Output()
	}
	if err != nil || len(out) == 0 {
		return
	}

	categoryName := "Container Security"
	if cliName == "podman" {
		categoryName = "Podman Security"
	} else {
		categoryName = "Docker Security"
	}

	containerIDs := strings.Fields(string(out))
	for _, id := range containerIDs {
		inspectOut, errInsp := exec.Command(cliName, "inspect", id).Output()
		if errInsp != nil {
			continue
		}
		inspectStr := string(inspectOut)

		// Check privileged flag
		if strings.Contains(inspectStr, `"Privileged": true`) {
			report.PrivilegedCtrsCount++
			report.ThreatScore += 15
			report.Threats = append(report.Threats, SecurityThreatItem{
				Level:       "WARNING",
				Category:    categoryName,
				Title:       fmt.Sprintf("Container %s đang chạy với quyền --privileged", id[:12]),
				Description: "Container chạy chế độ Privileged có thể truy cập trực tiếp Kernel Host Server.",
				ActionHint:  "Tắt cờ --privileged và chỉ bổ sung cap-add cụ thể nếu cần",
			})
		}

		// Check dangerous host socket / root mounts
		if strings.Contains(inspectStr, "/var/run/docker.sock") || strings.Contains(inspectStr, "/run/podman/podman.sock") {
			report.ThreatScore += 10
			report.Threats = append(report.Threats, SecurityThreatItem{
				Level:       "WARNING",
				Category:    categoryName,
				Title:       fmt.Sprintf("Container %s mount trực tiếp Socket máy chủ", id[:12]),
				Description: "Container mount container socket có quyền kiểm soát toàn bộ Container Engine trên Server.",
				ActionHint:  "Sử dụng API Proxy giới hạn quyền nếu không bắt buộc",
			})
		}
	}
}

func auditProcessThreats(report *SecurityReport) {
	// Scan running processes for known cryptominers or suspicious keywords
	suspiciousNames := []string{"xmrig", "minerd", "cpuminer", "kdevtmpf", "kintegrated"}

	procs, err := GetRunningProcesses(ProcessListOptions{SortBy: "cpu", Limit: 50})
	if err != nil {
		return
	}

	for _, p := range procs {
		pNameLower := strings.ToLower(p.Name + " " + p.Cmdline)
		for _, sus := range suspiciousNames {
			if strings.Contains(pNameLower, sus) {
				report.SuspiciousProcsCount++
				report.ThreatScore += 40
				report.Threats = append(report.Threats, SecurityThreatItem{
					Level:       "CRITICAL",
					Category:    "Process Threat",
					Title:       fmt.Sprintf("Phát hiện tiến trình nghi vấn Crypto Miner: %s (PID: %d)", p.Name, p.PID),
					Description: fmt.Sprintf("Tiến trình đang chiếm %.1f%% CPU và RAM %.1f MB.", p.CPUPercent, p.MemRSSMB),
					ActionHint:  fmt.Sprintf("Tiến hành Kill PID %d và kiểm tra file nguồn", p.PID),
				})
			}
		}
	}
}

func auditPortSecurity(report *SecurityReport) {
	ports, err := GetListeningPorts()
	if err != nil {
		return
	}

	sensitivePorts := map[int]string{
		6379:  "Redis Database Server",
		5432:  "PostgreSQL Database Server",
		3306:  "MySQL / MariaDB Server",
		27017: "MongoDB Server",
		9200:  "Elasticsearch Engine",
	}

	for _, p := range ports {
		if desc, isSensitive := sensitivePorts[p.LocalPort]; isSensitive {
			if p.LocalIP == "0.0.0.0" || p.LocalIP == "::" {
				report.ExposedPortsCount++
				report.ThreatScore += 10
				report.Threats = append(report.Threats, SecurityThreatItem{
					Level:       "WARNING",
					Category:    "Port Risk",
					Title:       fmt.Sprintf("Cổng Database %d (%s) đang mở Public 0.0.0.0", p.LocalPort, desc),
					Description: fmt.Sprintf("Cổng %d có thể bị truy cập công khai ngoài Internet nếu không có UFW Firewall.", p.LocalPort),
					ActionHint:  "Chỉ bind IP 127.0.0.1 hoặc chặn cổng bằng UFW Firewall",
				})
			}
		}
	}
}

func BlockIPWithFirewall(ip string) error {
	if ip == "" {
		return fmt.Errorf("ip address cannot be empty")
	}
	// Try iptables block
	cmd := exec.Command("iptables", "-A", "INPUT", "-s", ip, "-j", "DROP")
	return cmd.Run()
}
