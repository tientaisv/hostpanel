package system

import (
	"testing"
	"time"
)

func TestGetRunningProcesses(t *testing.T) {
	procs1, err := GetRunningProcesses(ProcessListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("Failed to get processes: %v", err)
	}
	if len(procs1) == 0 {
		t.Fatalf("Expected processes, got 0")
	}

	for _, p := range procs1 {
		if p.CPUPercent > 100.0 || p.CPUPercent < 0.0 {
			t.Errorf("Process PID %d (%s) CPUPercent out of bounds: %f", p.PID, p.Name, p.CPUPercent)
		}
	}

	time.Sleep(200 * time.Millisecond)

	procs2, err := GetRunningProcesses(ProcessListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("Failed to get processes second sample: %v", err)
	}

	for _, p := range procs2 {
		if p.CPUPercent > 100.0 || p.CPUPercent < 0.0 {
			t.Errorf("Process PID %d (%s) second sample CPUPercent out of bounds: %f", p.PID, p.Name, p.CPUPercent)
		}
	}
}
