package system

import (
	"testing"
)

func TestDetectFirewall(t *testing.T) {
	fw := DetectFirewall()
	t.Logf("Detected firewall type: %s", fw)
}

func TestGetFirewallStatus(t *testing.T) {
	status, err := GetFirewallStatus()
	if err != nil {
		t.Fatalf("GetFirewallStatus error: %v", err)
	}
	t.Logf("Firewall: type=%s, installed=%v, active=%v, rules=%d",
		status.Type, status.Installed, status.Active, status.RulesCount)
}
