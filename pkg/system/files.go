package system

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type FileItem struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time"`
	Mode    string `json:"mode"`
}

func ListFiles(dirPath string) ([]FileItem, error) {
	if dirPath == "" {
		dirPath = "/"
	}
	dirPath = filepath.Clean(dirPath)

	entries, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	var items []FileItem
	for _, info := range entries {
		fullPath := filepath.Join(dirPath, info.Name())
		items = append(items, FileItem{
			Name:    info.Name(),
			Path:    fullPath,
			IsDir:   info.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime().Format(time.RFC3339),
			Mode:    info.Mode().String(),
		})
	}

	// Sort: directories first, then alphabetical
	sort.Slice(items, func(i, j int) bool {
		if items[i].IsDir != items[j].IsDir {
			return items[i].IsDir
		}
		return items[i].Name < items[j].Name
	})

	return items, nil
}

func ReadFileContent(filePath string) (string, error) {
	filePath = filepath.Clean(filePath)

	info, err := os.Stat(filePath)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("path is a directory, not a file")
	}

	// Max 2MB limit for web viewer/editor
	if info.Size() > 2*1024*1024 {
		return "", fmt.Errorf("file size is too large to edit on web (%d bytes, max 2MB)", info.Size())
	}

	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	// Simple check for binary file
	if bytes.IndexByte(content, 0) != -1 {
		return "", fmt.Errorf("file appears to be a binary file")
	}

	return string(content), nil
}

func WriteFileContent(filePath string, content string) error {
	filePath = filepath.Clean(filePath)

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return ioutil.WriteFile(filePath, []byte(content), 0644)
}

func CreateItem(itemPath string, isDir bool) error {
	itemPath = filepath.Clean(itemPath)

	if isDir {
		return os.MkdirAll(itemPath, 0755)
	}

	dir := filepath.Dir(itemPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	f, err := os.OpenFile(itemPath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	_ = f.Close()
	return nil
}

func DeleteItem(itemPath string) error {
	itemPath = filepath.Clean(itemPath)

	if itemPath == "/" || itemPath == "." {
		return fmt.Errorf("cannot delete root path")
	}

	return os.RemoveAll(itemPath)
}
