package system

import (
	"bufio"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

type FirewallType string

const (
	FirewallUFW       FirewallType = "UFW"
	FirewallFirewalld FirewallType = "FIREWALLD"
	FirewallIptables  FirewallType = "IPTABLES"
)

type FirewallRule struct {
	ID          string `json:"id"`
	Port        string `json:"port"`
	Protocol    string `json:"protocol"` // "tcp", "udp", "any"
	Action      string `json:"action"`   // "ALLOW", "DENY", "REJECT"
	FromIP      string `json:"from_ip"`  // "Anywhere" or specific IP/subnet
	Description string `json:"description,omitempty"`
}

type FirewallStatus struct {
	Type          FirewallType   `json:"type"`
	Installed     bool           `json:"installed"`
	Active        bool           `json:"active"`
	DefaultInput  string         `json:"default_input"`  // "deny", "allow"
	DefaultOutput string         `json:"default_output"` // "allow"
	RulesCount    int            `json:"rules_count"`
	Rules         []FirewallRule `json:"rules"`
	ErrorMessage  string         `json:"error_message,omitempty"`
}

// DetectFirewall identifies the available firewall system on the host
func DetectFirewall() FirewallType {
	if _, err := exec.LookPath("ufw"); err == nil {
		return FirewallUFW
	}
	if _, err := exec.LookPath("firewall-cmd"); err == nil {
		return FirewallFirewalld
	}
	return FirewallIptables
}

// GetFirewallStatus queries the current firewall state and list of rules
func GetFirewallStatus() (*FirewallStatus, error) {
	fwType := DetectFirewall()
	status := &FirewallStatus{
		Type:          fwType,
		Installed:     true,
		Active:        false,
		DefaultInput:  "deny",
		DefaultOutput: "allow",
		Rules:         make([]FirewallRule, 0),
	}

	switch fwType {
	case FirewallUFW:
		return getUFWStatus(status)
	case FirewallFirewalld:
		return getFirewalldStatus(status)
	default:
		return getIptablesStatus(status)
	}
}

func getUFWStatus(status *FirewallStatus) (*FirewallStatus, error) {
	out, err := exec.Command("ufw", "status", "numbered").CombinedOutput()
	if err != nil {
		status.ErrorMessage = fmt.Sprintf("Lỗi đọc trạng thái UFW: %v", err)
		return status, nil
	}

	outStr := string(out)
	if strings.Contains(outStr, "Status: active") {
		status.Active = true
	} else {
		status.Active = false
		return status, nil
	}

	// Parse numbered rules
	// Format:
	// [ 1] 22/tcp                     ALLOW IN    Anywhere
	// [ 2] 80/tcp                     ALLOW IN    192.168.1.5
	re := regexp.MustCompile(`\[\s*(\d+)\]\s+([^\s]+)\s+([A-Z]+(?:\s+[A-Z]+)?)\s+(.+)`)
	scanner := bufio.NewScanner(strings.NewReader(outStr))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		matches := re.FindStringSubmatch(line)
		if len(matches) >= 5 {
			ruleID := matches[1]
			target := strings.TrimSpace(matches[2])
			actionRaw := strings.TrimSpace(matches[3])
			from := strings.TrimSpace(matches[4])

			// Skip IPv6 duplicate visual rules if same port
			isV6 := strings.Contains(from, "(v6)") || strings.Contains(target, "(v6)")
			fromClean := strings.ReplaceAll(from, " (v6)", "")
			targetClean := strings.ReplaceAll(target, " (v6)", "")

			port := targetClean
			proto := "any"
			if strings.Contains(targetClean, "/") {
				pParts := strings.Split(targetClean, "/")
				port = pParts[0]
				proto = pParts[1]
			}

			action := "ALLOW"
			if strings.Contains(actionRaw, "DENY") {
				action = "DENY"
			} else if strings.Contains(actionRaw, "REJECT") {
				action = "REJECT"
			}

			rule := FirewallRule{
				ID:       ruleID,
				Port:     port,
				Protocol: proto,
				Action:   action,
				FromIP:   fromClean,
			}
			if isV6 {
				rule.Description = "IPv6 Rule"
			}

			status.Rules = append(status.Rules, rule)
		}
	}

	status.RulesCount = len(status.Rules)
	return status, nil
}

func getFirewalldStatus(status *FirewallStatus) (*FirewallStatus, error) {
	stateOut, err := exec.Command("firewall-cmd", "--state").CombinedOutput()
	if err == nil && strings.TrimSpace(string(stateOut)) == "running" {
		status.Active = true
	} else {
		status.Active = false
		return status, nil
	}

	out, err := exec.Command("firewall-cmd", "--list-all").CombinedOutput()
	if err != nil {
		status.ErrorMessage = fmt.Sprintf("Lỗi đọc danh sách ports firewalld: %v", err)
		return status, nil
	}

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	ruleIndex := 1
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "ports:") {
			portsRaw := strings.TrimPrefix(line, "ports:")
			ports := strings.Fields(portsRaw)
			for _, p := range ports {
				pParts := strings.Split(p, "/")
				port := pParts[0]
				proto := "tcp"
				if len(pParts) > 1 {
					proto = pParts[1]
				}
				status.Rules = append(status.Rules, FirewallRule{
					ID:       strconv.Itoa(ruleIndex),
					Port:     port,
					Protocol: proto,
					Action:   "ALLOW",
					FromIP:   "Anywhere",
				})
				ruleIndex++
			}
		}
	}

	status.RulesCount = len(status.Rules)
	return status, nil
}

func getIptablesStatus(status *FirewallStatus) (*FirewallStatus, error) {
	out, err := exec.Command("iptables", "-L", "INPUT", "-n", "--line-numbers").CombinedOutput()
	if err != nil {
		status.ErrorMessage = fmt.Sprintf("Lỗi đọc iptables: %v", err)
		return status, nil
	}

	status.Active = true
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	isHeader := true

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "num") {
			isHeader = false
			continue
		}
		if isHeader || line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) >= 5 {
			num := fields[0]
			target := fields[1] // ACCEPT, DROP, REJECT
			proto := fields[2]
			source := fields[4]

			action := "ALLOW"
			if target == "DROP" || target == "REJECT" {
				action = "DENY"
			}

			port := "All"
			for _, f := range fields[5:] {
				if strings.HasPrefix(f, "dpt:") {
					port = strings.TrimPrefix(f, "dpt:")
				}
			}

			from := source
			if source == "0.0.0.0/0" {
				from = "Anywhere"
			}

			status.Rules = append(status.Rules, FirewallRule{
				ID:       num,
				Port:     port,
				Protocol: proto,
				Action:   action,
				FromIP:   from,
			})
		}
	}

	status.RulesCount = len(status.Rules)
	return status, nil
}

// ToggleFirewall enables or disables the host firewall
func ToggleFirewall(enable bool) error {
	fwType := DetectFirewall()

	switch fwType {
	case FirewallUFW:
		if enable {
			// Safety Guard: Ensure SSH port 22 and DockPulse port 3800 are explicitly allowed first
			_ = exec.Command("ufw", "allow", "22/tcp").Run()
			_ = exec.Command("ufw", "allow", "3800/tcp").Run()
			out, err := exec.Command("ufw", "--force", "enable").CombinedOutput()
			if err != nil {
				return fmt.Errorf("lỗi kích hoạt UFW: %v (%s)", err, string(out))
			}
		} else {
			out, err := exec.Command("ufw", "disable").CombinedOutput()
			if err != nil {
				return fmt.Errorf("lỗi tắt UFW: %v (%s)", err, string(out))
			}
		}
	case FirewallFirewalld:
		if enable {
			_ = exec.Command("systemctl", "enable", "--now", "firewalld").Run()
			_ = exec.Command("firewall-cmd", "--permanent", "--add-port=22/tcp").Run()
			_ = exec.Command("firewall-cmd", "--permanent", "--add-port=3800/tcp").Run()
			_ = exec.Command("firewall-cmd", "--reload").Run()
		} else {
			_ = exec.Command("systemctl", "stop", "firewalld").Run()
			_ = exec.Command("systemctl", "disable", "firewalld").Run()
		}
	default:
		return fmt.Errorf("iptables không hỗ trợ bật/tắt toàn cục một chạm, vui lòng quản lý theo từng quy tắc")
	}

	return nil
}

// AddFirewallRule adds a new rule (port/IP allow or deny)
func AddFirewallRule(rule FirewallRule) error {
	fwType := DetectFirewall()
	port := strings.TrimSpace(rule.Port)
	proto := strings.ToLower(strings.TrimSpace(rule.Protocol))
	if proto == "" {
		proto = "tcp"
	}
	action := strings.ToUpper(strings.TrimSpace(rule.Action))
	if action == "" {
		action = "ALLOW"
	}
	fromIP := strings.TrimSpace(rule.FromIP)

	switch fwType {
	case FirewallUFW:
		var cmd *exec.Cmd
		if fromIP != "" && fromIP != "Anywhere" && fromIP != "0.0.0.0/0" {
			if port != "" && port != "All" {
				if proto == "any" {
					cmd = exec.Command("ufw", strings.ToLower(action), "from", fromIP, "to", "any", "port", port)
				} else {
					cmd = exec.Command("ufw", strings.ToLower(action), "from", fromIP, "to", "any", "port", port, "proto", proto)
				}
			} else {
				cmd = exec.Command("ufw", strings.ToLower(action), "from", fromIP)
			}
		} else {
			if port == "" || port == "All" {
				return fmt.Errorf("vui lòng nhập cổng hoặc địa chỉ IP nguồn")
			}
			if proto == "any" {
				cmd = exec.Command("ufw", strings.ToLower(action), port)
			} else {
				cmd = exec.Command("ufw", strings.ToLower(action), fmt.Sprintf("%s/%s", port, proto))
			}
		}

		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("lỗi thêm quy tắc UFW: %v (%s)", err, string(out))
		}

	case FirewallFirewalld:
		if port != "" && port != "All" {
			portProto := fmt.Sprintf("%s/%s", port, proto)
			if proto == "any" {
				portProto = fmt.Sprintf("%s/tcp", port)
			}
			out, err := exec.Command("firewall-cmd", "--permanent", "--add-port="+portProto).CombinedOutput()
			if err != nil {
				return fmt.Errorf("lỗi thêm port firewalld: %v (%s)", err, string(out))
			}
			_ = exec.Command("firewall-cmd", "--reload").Run()
		} else if fromIP != "" {
			out, err := exec.Command("firewall-cmd", "--permanent", "--add-source="+fromIP).CombinedOutput()
			if err != nil {
				return fmt.Errorf("lỗi thêm source firewalld: %v (%s)", err, string(out))
			}
			_ = exec.Command("firewall-cmd", "--reload").Run()
		}

	default:
		// Fallback Iptables
		var iptAction = "-j ACCEPT"
		if action == "DENY" {
			iptAction = "-j DROP"
		}
		args := []string{"-A", "INPUT"}
		if proto != "any" && proto != "" {
			args = append(args, "-p", proto)
		}
		if port != "" && port != "All" {
			args = append(args, "--dport", port)
		}
		if fromIP != "" && fromIP != "Anywhere" {
			args = append(args, "-s", fromIP)
		}
		args = append(args, strings.Fields(iptAction)...)

		cmd := exec.Command("iptables", args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("lỗi thêm iptables: %v (%s)", err, string(out))
		}
	}

	return nil
}

// DeleteFirewallRule deletes a rule by number or signature
func DeleteFirewallRule(ruleID string, port string, proto string, action string, fromIP string) error {
	fwType := DetectFirewall()

	switch fwType {
	case FirewallUFW:
		if ruleID == "" {
			return fmt.Errorf("missing rule id")
		}
		out, err := exec.Command("ufw", "--force", "delete", ruleID).CombinedOutput()
		if err != nil {
			return fmt.Errorf("lỗi xóa quy tắc UFW #%s: %v (%s)", ruleID, err, string(out))
		}

	case FirewallFirewalld:
		if port != "" {
			if proto == "" || proto == "any" {
				proto = "tcp"
			}
			_ = exec.Command("firewall-cmd", "--permanent", "--remove-port="+port+"/"+proto).Run()
			_ = exec.Command("firewall-cmd", "--reload").Run()
		} else if fromIP != "" {
			_ = exec.Command("firewall-cmd", "--permanent", "--remove-source="+fromIP).Run()
			_ = exec.Command("firewall-cmd", "--reload").Run()
		}

	default:
		if ruleID != "" {
			out, err := exec.Command("iptables", "-D", "INPUT", ruleID).CombinedOutput()
			if err != nil {
				return fmt.Errorf("lỗi xóa iptables #%s: %v (%s)", ruleID, err, string(out))
			}
		}
	}

	return nil
}
