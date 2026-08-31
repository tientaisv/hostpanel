package system

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os/exec"
	"strings"
)

type JailInfo struct {
	Name            string   `json:"name"`
	CurrentlyFailed int      `json:"currently_failed"`
	TotalFailed     int      `json:"total_failed"`
	CurrentlyBanned int      `json:"currently_banned"`
	TotalBanned     int      `json:"total_banned"`
	BannedIPs       []string `json:"banned_ips"`
}

type Fail2banStatus struct {
	Installed       bool       `json:"installed"`
	Active          bool       `json:"active"`
	Version         string     `json:"version,omitempty"`
	JailCount       int        `json:"jail_count"`
	TotalBannedIPs  int        `json:"total_banned_ips"`
	Jails           []JailInfo `json:"jails"`
	ErrorMessage    string     `json:"error_message,omitempty"`
}

// GetFail2banStatus checks if Fail2ban is installed, active, and queries all jail metrics
func GetFail2banStatus() (*Fail2banStatus, error) {
	status := &Fail2banStatus{
		Installed: false,
		Active:    false,
		Jails:     make([]JailInfo, 0),
	}

	// 1. Check if binary exists
	f2bPath, err := exec.LookPath("fail2ban-client")
	if err != nil {
		return status, nil
	}
	status.Installed = true

	// Check version
	verOut, err := exec.Command(f2bPath, "version").Output()
	if err == nil {
		status.Version = strings.TrimSpace(string(verOut))
	}

	// 2. Check if service is active
	out, err := exec.Command(f2bPath, "status").Output()
	if err != nil {
		status.ErrorMessage = "Fail2ban is installed but service is not running."
		return status, nil
	}
	status.Active = true

	// Parse jail list from `fail2ban-client status`
	// Output format:
	// |- Number of jail:      4
	// `- Jail list:   nginx-bad-request, nginx-botsearch, nginx-http-auth, sshd
	jailList := parseJailList(string(out))
	status.JailCount = len(jailList)

	totalBannedCount := 0

	// 3. Query details for each jail
	for _, jailName := range jailList {
		jailDetailOut, err := exec.Command(f2bPath, "status", jailName).Output()
		if err != nil {
			continue
		}

		info := parseJailDetail(jailName, string(jailDetailOut))
		totalBannedCount += info.CurrentlyBanned
		status.Jails = append(status.Jails, info)
	}

	status.TotalBannedIPs = totalBannedCount

	return status, nil
}

func parseJailList(out string) []string {
	var list []string
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		if strings.Contains(line, "Jail list:") {
			parts := strings.Split(line, ":")
			if len(parts) > 1 {
				rawJails := strings.TrimSpace(parts[1])
				if rawJails != "" {
					items := strings.Split(rawJails, ",")
					for _, item := range items {
						trimmed := strings.TrimSpace(item)
						if trimmed != "" {
							list = append(list, trimmed)
						}
					}
				}
			}
		}
	}
	return list
}

func parseJailDetail(jailName, out string) JailInfo {
	info := JailInfo{
		Name:      jailName,
		BannedIPs: make([]string, 0),
	}

	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.Contains(line, "Currently failed:") {
			parts := strings.Split(line, ":")
			if len(parts) > 1 {
				fmt.Sscanf(strings.TrimSpace(parts[1]), "%d", &info.CurrentlyFailed)
			}
		} else if strings.Contains(line, "Total failed:") {
			parts := strings.Split(line, ":")
			if len(parts) > 1 {
				fmt.Sscanf(strings.TrimSpace(parts[1]), "%d", &info.TotalFailed)
			}
		} else if strings.Contains(line, "Currently banned:") {
			parts := strings.Split(line, ":")
			if len(parts) > 1 {
				fmt.Sscanf(strings.TrimSpace(parts[1]), "%d", &info.CurrentlyBanned)
			}
		} else if strings.Contains(line, "Total banned:") {
			parts := strings.Split(line, ":")
			if len(parts) > 1 {
				fmt.Sscanf(strings.TrimSpace(parts[1]), "%d", &info.TotalBanned)
			}
		} else if strings.Contains(line, "Banned IP list:") {
			parts := strings.Split(line, ":")
			if len(parts) > 1 {
				ips := strings.Fields(strings.TrimSpace(parts[1]))
				for _, ip := range ips {
					if ip != "" {
						info.BannedIPs = append(info.BannedIPs, ip)
					}
				}
			}
		}
	}

	return info
}

// UnbanIP removes an IP from a specified jail or all jails if jail is empty
func UnbanIP(jail, ip string) error {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return fmt.Errorf("ip address cannot be empty")
	}

	f2bPath, err := exec.LookPath("fail2ban-client")
	if err != nil {
		return fmt.Errorf("fail2ban-client is not installed")
	}

	jail = strings.TrimSpace(jail)
	if jail == "" {
		jail = "sshd"
	}

	out, err := exec.Command(f2bPath, "set", jail, "unbanip", ip).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(out))
	}
	return nil
}

// BanIP manually adds an IP to a jail's ban list
func BanIP(jail, ip string) error {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return fmt.Errorf("ip address cannot be empty")
	}

	f2bPath, err := exec.LookPath("fail2ban-client")
	if err != nil {
		return fmt.Errorf("fail2ban-client is not installed")
	}

	jail = strings.TrimSpace(jail)
	if jail == "" {
		jail = "sshd"
	}

	out, err := exec.Command(f2bPath, "set", jail, "banip", ip).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(out))
	}
	return nil
}

// InstallAndConfigureFail2ban automates installation and sets up safe default jail.local
func InstallAndConfigureFail2ban(clientIP string) error {
	// 1. Detect Package Manager & Install
	var installCmd *exec.Cmd
	if _, err := exec.LookPath("apt-get"); err == nil {
		installCmd = exec.Command("sh", "-c", "DEBIAN_FRONTEND=noninteractive apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y fail2ban")
	} else if _, err := exec.LookPath("dnf"); err == nil {
		installCmd = exec.Command("dnf", "install", "-y", "epel-release", "fail2ban")
	} else if _, err := exec.LookPath("yum"); err == nil {
		installCmd = exec.Command("yum", "install", "-y", "epel-release", "fail2ban")
	} else if _, err := exec.LookPath("apk"); err == nil {
		installCmd = exec.Command("apk", "add", "--no-cache", "fail2ban")
	} else {
		return fmt.Errorf("no supported package manager found (apt, dnf, yum, apk)")
	}

	out, err := installCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to install fail2ban: %v (%s)", err, string(out))
	}

	// 2. Prepare whitelist
	whitelist := "127.0.0.1/8 ::1 10.0.0.0/8 172.16.0.0/12 192.168.0.0/16"
	if clientIP != "" && clientIP != "127.0.0.1" && clientIP != "::1" {
		// Verify if it is valid IP
		if parsed := net.ParseIP(clientIP); parsed != nil {
			whitelist = fmt.Sprintf("%s %s", whitelist, clientIP)
		}
	}

	// 3. Write default jail.local
	jailLocalContent := fmt.Sprintf(`[DEFAULT]
# Whitelist local and administrator IPs to prevent lockout
ignoreip = %s

# Ban time configuration
bantime = 1h
findtime = 10m
maxretry = 5

# Incremental ban time (exponential increase for repeat offenders)
bantime.increment = true
bantime.factor = 1

# Default backend
backend = systemd

# SSH Protection
[sshd]
enabled = true
port = 22
mode = aggressive
logpath = %s(sshd_log)s
backend = %s(sshd_backend)s
maxretry = 5
findtime = 10m
bantime = 1h

# Nginx Protection (if Nginx is installed)
[nginx-http-auth]
enabled = true
port = http,https
logpath = /var/log/nginx/error.log

[nginx-botsearch]
enabled = true
port = http,https
logpath = /var/log/nginx/access.log
maxretry = 2
findtime = 10m
bantime = 24h

[nginx-bad-request]
enabled = true
port = http,https
logpath = /var/log/nginx/access.log
maxretry = 5
findtime = 10m
bantime = 1h
`, whitelist, "%", "%")

	if err := ioutil.WriteFile("/etc/fail2ban/jail.local", []byte(jailLocalContent), 0644); err != nil {
		return fmt.Errorf("failed to write /etc/fail2ban/jail.local: %v", err)
	}

	// 4. Enable and start fail2ban service
	_ = exec.Command("systemctl", "daemon-reload").Run()
	_ = exec.Command("systemctl", "enable", "fail2ban").Run()
	startOut, err := exec.Command("systemctl", "restart", "fail2ban").CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to restart fail2ban service: %v (%s)", err, string(startOut))
	}

	return nil
}

// ExtractClientIP extracts real client IP from incoming HTTP request
func ExtractClientIP(r *http.Request) string {
	// Check headers
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			ip := strings.TrimSpace(ips[0])
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	}
	if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		if net.ParseIP(strings.TrimSpace(xrip)) != nil {
			return strings.TrimSpace(xrip)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && net.ParseIP(host) != nil {
		return host
	}
	return r.RemoteAddr
}
