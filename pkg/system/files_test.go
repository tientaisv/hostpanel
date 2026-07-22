package system

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestFilesOperations(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "test_file_mgr_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	testFilePath := filepath.Join(tempDir, "sample.txt")
	content := "hello world\nline 2"

	// 1. Create File & Write
	if err := WriteFileContent(testFilePath, content); err != nil {
		t.Fatalf("WriteFileContent failed: %v", err)
	}

	// 2. Read File
	readBack, err := ReadFileContent(testFilePath)
	if err != nil {
		t.Fatalf("ReadFileContent failed: %v", err)
	}
	if readBack != content {
		t.Fatalf("content mismatch: got %q, expected %q", readBack, content)
	}

	// 3. List Files
	items, err := ListFiles(tempDir)
	if err != nil {
		t.Fatalf("ListFiles failed: %v", err)
	}
	if len(items) != 1 || items[0].Name != "sample.txt" {
		t.Fatalf("unexpected list items: %+v", items)
	}

	// 4. Delete File
	if err := DeleteItem(testFilePath); err != nil {
		t.Fatalf("DeleteItem failed: %v", err)
	}

	itemsAfter, _ := ListFiles(tempDir)
	if len(itemsAfter) != 0 {
		t.Fatalf("expected empty directory after delete, got %d items", len(itemsAfter))
	}
}
