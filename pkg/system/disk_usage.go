package system

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

type FolderUsageInfo struct {
	Name          string  `json:"name"`
	Path          string  `json:"path"`
	SizeBytes     int64   `json:"size_bytes"`
	FormattedSize string  `json:"formatted_size"`
	Percent       float64 `json:"percent"`
	FileCount     int64   `json:"file_count"`
	DirCount      int64   `json:"dir_count"`
}

type DiskUsageSummary struct {
	TargetPath     string            `json:"target_path"`
	TotalDiskGB    float64           `json:"total_disk_gb"`
	UsedDiskGB     float64           `json:"used_disk_gb"`
	FreeDiskGB     float64           `json:"free_disk_gb"`
	DiskPercent    float64           `json:"disk_percent"`
	TotalScannedGB float64           `json:"total_scanned_gb"`
	ScanTimeMs     int64             `json:"scan_time_ms"`
	TopFolders     []FolderUsageInfo `json:"top_folders"`
}

var ignoredPaths = map[string]bool{
	"/proc":                    true,
	"/sys":                     true,
	"/dev":                     true,
	"/run":                     true,
	"/sys/fs/cgroup":           true,
	"/proc/sys/fs/binfmt_misc": true,
}

func isIgnoredPath(path string) bool {
	cleanPath := filepath.Clean(path)
	if ignoredPaths[cleanPath] {
		return true
	}
	if strings.HasPrefix(cleanPath, "/proc/") ||
		strings.HasPrefix(cleanPath, "/sys/") ||
		strings.HasPrefix(cleanPath, "/dev/") ||
		strings.HasPrefix(cleanPath, "/run/") {
		return true
	}
	return false
}

func FormatBytes(b int64) string {
	if b <= 0 {
		return "0 B"
	}
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func GetFolderDiskUsage(targetPath string, topN int) (*DiskUsageSummary, error) {
	startTime := time.Now()

	if targetPath == "" {
		targetPath = "/"
	}
	targetPath = filepath.Clean(targetPath)

	if topN <= 0 {
		topN = 15
	}
	if topN > 50 {
		topN = 50
	}

	summary := &DiskUsageSummary{
		TargetPath: targetPath,
		TopFolders: []FolderUsageInfo{},
	}

	// 1. Get Overall Disk Stats via Statfs
	var stat syscall.Statfs_t
	if err := syscall.Statfs(targetPath, &stat); err == nil {
		bsize := uint64(stat.Bsize)
		if stat.Frsize > 0 {
			bsize = uint64(stat.Frsize)
		}
		totalBytes := stat.Blocks * bsize
		freeBytes := stat.Bfree * bsize
		availBytes := stat.Bavail * bsize
		usedBytes := totalBytes - freeBytes

		summary.TotalDiskGB = float64(totalBytes) / (1024 * 1024 * 1024)
		summary.FreeDiskGB = float64(availBytes) / (1024 * 1024 * 1024)
		summary.UsedDiskGB = float64(usedBytes) / (1024 * 1024 * 1024)
		if totalBytes > 0 {
			summary.DiskPercent = (float64(usedBytes) / float64(totalBytes)) * 100.0
		}
	}

	// 2. Read entries in targetPath
	entries, err := ioutil.ReadDir(targetPath)
	if err != nil {
		return nil, fmt.Errorf("không thể đọc thư mục %s: %v", targetPath, err)
	}

	type subfolderTask struct {
		name string
		path string
	}

	var tasks []subfolderTask
	var directFilesSize int64
	var directFilesCount int64

	for _, entry := range entries {
		fullPath := filepath.Join(targetPath, entry.Name())
		if isIgnoredPath(fullPath) {
			continue
		}

		if entry.Mode()&os.ModeSymlink != 0 {
			continue
		}

		if entry.IsDir() {
			tasks = append(tasks, subfolderTask{name: entry.Name(), path: fullPath})
		} else {
			directFilesSize += entry.Size()
			directFilesCount++
		}
	}

	// 3. Scan subfolders in parallel with worker pool
	var results []FolderUsageInfo
	var mu sync.Mutex
	var totalScannedBytes int64

	// Concurrency limiter channel (max 8 concurrent directory scans)
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup

	for _, task := range tasks {
		wg.Add(1)
		go func(t subfolderTask) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			size, fileCount, dirCount := walkDirFast(t.path)

			info := FolderUsageInfo{
				Name:          t.name,
				Path:          t.path,
				SizeBytes:     size,
				FormattedSize: FormatBytes(size),
				FileCount:     fileCount,
				DirCount:      dirCount,
			}

			mu.Lock()
			results = append(results, info)
			totalScannedBytes += size
			mu.Unlock()
		}(task)
	}

	wg.Wait()

	if directFilesCount > 0 {
		results = append(results, FolderUsageInfo{
			Name:          "[Tệp tin tại thư mục hiện tại]",
			Path:          targetPath,
			SizeBytes:     directFilesSize,
			FormattedSize: FormatBytes(directFilesSize),
			FileCount:     directFilesCount,
			DirCount:      0,
		})
		totalScannedBytes += directFilesSize
	}

	summary.TotalScannedGB = float64(totalScannedBytes) / (1024 * 1024 * 1024)

	// Calculate % for each folder relative to total scanned bytes
	for i := range results {
		if totalScannedBytes > 0 {
			results[i].Percent = (float64(results[i].SizeBytes) / float64(totalScannedBytes)) * 100.0
		}
	}

	// Sort descending by SizeBytes
	sort.Slice(results, func(i, j int) bool {
		return results[i].SizeBytes > results[j].SizeBytes
	})

	if len(results) > topN {
		summary.TopFolders = results[:topN]
	} else {
		summary.TopFolders = results
	}

	summary.ScanTimeMs = time.Since(startTime).Milliseconds()
	return summary, nil
}

func walkDirFast(dirPath string) (int64, int64, int64) {
	var totalSize int64
	var fileCount int64
	var dirCount int64

	var scan func(p string)
	scan = func(p string) {
		entries, err := ioutil.ReadDir(p)
		if err != nil {
			return
		}

		for _, entry := range entries {
			fullPath := filepath.Join(p, entry.Name())

			// Skip symlinks & pseudo filesystems
			if entry.Mode()&os.ModeSymlink != 0 || isIgnoredPath(fullPath) {
				continue
			}

			if entry.IsDir() {
				dirCount++
				scan(fullPath)
			} else {
				fileCount++
				totalSize += entry.Size()
			}
		}
	}

	scan(dirPath)
	return totalSize, fileCount, dirCount
}
