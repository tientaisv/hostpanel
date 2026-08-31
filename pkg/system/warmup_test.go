package system

import (
	"testing"
	"time"
)

func TestWarmupManagerInitAndToggle(t *testing.T) {
	wm := InitWarmupManager()
	if wm == nil {
		t.Fatalf("Expected non-nil WarmupManager")
	}

	// Test initial status
	status := wm.GetStatus()
	if status.TargetCPUPercent <= 0 {
		t.Errorf("Expected positive TargetCPUPercent, got %v", status.TargetCPUPercent)
	}

	// Test Toggle ON
	newStatus, err := wm.Toggle(true)
	if err != nil {
		t.Fatalf("Toggle(true) failed: %v", err)
	}
	if !newStatus.Enabled {
		t.Errorf("Expected Enabled == true")
	}

	// Test Test Mode
	testStatus, err := wm.TriggerTest(2)
	if err != nil {
		t.Fatalf("TriggerTest(2) failed: %v", err)
	}
	if testStatus.State != StateTesting {
		t.Errorf("Expected State == StateTesting, got %v", testStatus.State)
	}

	time.Sleep(3 * time.Second)

	// Test Toggle OFF
	offStatus, err := wm.Toggle(false)
	if err != nil {
		t.Fatalf("Toggle(false) failed: %v", err)
	}
	if offStatus.Enabled {
		t.Errorf("Expected Enabled == false")
	}
}
