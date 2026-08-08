package system

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestGetFolderDiskUsage(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "disk_usage_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	subDir1 := filepath.Join(tempDir, "folder_a")
	subDir2 := filepath.Join(tempDir, "folder_b")
	os.MkdirAll(subDir1, 0755)
	os.MkdirAll(subDir2, 0755)

	// Create dummy files
	file1 := filepath.Join(subDir1, "data1.txt")
	file2 := filepath.Join(subDir2, "data2.txt")
	ioutil.WriteFile(file1, []byte("Hello World 1234567890"), 0644)
	ioutil.WriteFile(file2, []byte("Testing Go Disk Usage Calculation Feature"), 0644)

	summary, err := GetFolderDiskUsage(tempDir, 10)
	if err != nil {
		t.Fatalf("GetFolderDiskUsage returned error: %v", err)
	}

	if summary.TargetPath != tempDir {
		t.Errorf("Expected TargetPath %s, got %s", tempDir, summary.TargetPath)
	}

	if len(summary.TopFolders) < 2 {
		t.Errorf("Expected at least 2 top folders, got %d", len(summary.TopFolders))
	}
}
