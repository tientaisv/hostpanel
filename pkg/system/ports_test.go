package system

import (
	"testing"
)

func TestGetListeningPorts(t *testing.T) {
	ports, err := GetListeningPorts()
	if err != nil {
		t.Fatalf("GetListeningPorts error: %v", err)
	}
	t.Logf("Found %d listening ports on host system", len(ports))
	for _, p := range ports {
		t.Logf("  [%s] %s:%d (PID %d %s)", p.Protocol, p.LocalIP, p.LocalPort, p.PID, p.ProcessName)
	}
}
