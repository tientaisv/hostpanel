package updater

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	CurrentVersion    = "v1.3.1"
	GitHubRepo        = "tientaisv/hostpanel"
	UpdateMu          sync.Mutex
	IsUpdating        bool
	LastUpdateLog     string
	AutoUpdateEnabled = true
	cachedUpdateInfo  *UpdateInfo
	lastCheckTime     time.Time
)

type UpdateInfo struct {
	CurrentVersion    string `json:"current_version"`
	LatestVersion     string `json:"latest_version"`
	LatestCommitSHA   string `json:"latest_commit_sha,omitempty"`
	HasUpdate         bool   `json:"has_update"`
	ReleaseNotes      string `json:"release_notes"`
	PublishedAt       string `json:"published_at"`
	AutoUpdateEnabled bool   `json:"auto_update_enabled"`
	IsUpdating        bool   `json:"is_updating"`
	LastUpdateLog     string `json:"last_update_log,omitempty"`
}

type GitHubCommit struct {
	SHA    string `json:"sha"`
	Commit struct {
		Message string `json:"message"`
		Author  struct {
			Date string `json:"date"`
		} `json:"author"`
	} `json:"commit"`
}

type GitHubRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	PublishedAt string `json:"published_at"`
}

// CheckUpdate checks GitHub for latest release or latest commit
func CheckUpdate(force bool) (*UpdateInfo, error) {
	UpdateMu.Lock()
	defer UpdateMu.Unlock()

	// Cache check result for 2 minutes unless forced
	if !force && cachedUpdateInfo != nil && time.Since(lastCheckTime) < 2*time.Minute {
		cachedUpdateInfo.IsUpdating = IsUpdating
		cachedUpdateInfo.LastUpdateLog = LastUpdateLog
		cachedUpdateInfo.AutoUpdateEnabled = AutoUpdateEnabled
		return cachedUpdateInfo, nil
	}

	info := &UpdateInfo{
		CurrentVersion:    CurrentVersion,
		LatestVersion:     CurrentVersion,
		HasUpdate:         false,
		AutoUpdateEnabled: AutoUpdateEnabled,
		IsUpdating:        IsUpdating,
		LastUpdateLog:     LastUpdateLog,
	}

	client := &http.Client{Timeout: 8 * time.Second}

	// 1. Try checking latest GitHub Release
	reqRel, _ := http.NewRequest("GET", fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", GitHubRepo), nil)
	reqRel.Header.Set("User-Agent", "DockPulse-AutoUpdater")
	respRel, err := client.Do(reqRel)
	if err == nil && respRel.StatusCode == 200 {
		defer respRel.Body.Close()
		var rel GitHubRelease
		if err := json.NewDecoder(respRel.Body).Decode(&rel); err == nil && rel.TagName != "" {
			info.LatestVersion = rel.TagName
			info.ReleaseNotes = rel.Body
			if rel.Name != "" {
				info.ReleaseNotes = fmt.Sprintf("### %s\n\n%s", rel.Name, rel.Body)
			}
			info.PublishedAt = rel.PublishedAt
			if rel.TagName != CurrentVersion {
				info.HasUpdate = true
			}
			cachedUpdateInfo = info
			lastCheckTime = time.Now()
			return info, nil
		}
	}
	if respRel != nil {
		respRel.Body.Close()
	}

	// 2. Fallback: Check latest commit on main branch
	reqCommit, _ := http.NewRequest("GET", fmt.Sprintf("https://api.github.com/repos/%s/commits/main", GitHubRepo), nil)
	reqCommit.Header.Set("User-Agent", "DockPulse-AutoUpdater")
	respCommit, err := client.Do(reqCommit)
	if err == nil && respCommit.StatusCode == 200 {
		defer respCommit.Body.Close()
		var commit GitHubCommit
		if err := json.NewDecoder(respCommit.Body).Decode(&commit); err == nil && commit.SHA != "" {
			shortSHA := commit.SHA
			if len(shortSHA) > 7 {
				shortSHA = shortSHA[:7]
			}
			info.LatestVersion = fmt.Sprintf("Commit %s", shortSHA)
			info.LatestCommitSHA = commit.SHA
			info.ReleaseNotes = commit.Commit.Message
			info.PublishedAt = commit.Commit.Author.Date

			// Check local git SHA if available
			localSHA := getLocalGitSHA()
			if localSHA != "" {
				if !strings.HasPrefix(commit.SHA, localSHA) && !strings.HasPrefix(localSHA, commit.SHA) {
					info.HasUpdate = true
				}
			} else {
				// If not git repo, compare with CurrentVersion or assume latest
				info.HasUpdate = false
			}

			cachedUpdateInfo = info
			lastCheckTime = time.Now()
			return info, nil
		}
	}
	if respCommit != nil {
		respCommit.Body.Close()
	}

	cachedUpdateInfo = info
	lastCheckTime = time.Now()
	return info, nil
}

func getLocalGitSHA() string {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	return ""
}

// ApplyUpdate executes the self-update process asynchronously
func ApplyUpdate() error {
	UpdateMu.Lock()
	if IsUpdating {
		UpdateMu.Unlock()
		return fmt.Errorf("hệ thống đang trong quá trình cập nhật, vui lòng đợi")
	}
	IsUpdating = true
	LastUpdateLog = "🚀 Bắt đầu quá trình nâng cấp hệ thống..."
	UpdateMu.Unlock()

	go func() {
		defer func() {
			UpdateMu.Lock()
			IsUpdating = false
			UpdateMu.Unlock()
		}()

		appDir, err := os.Getwd()
		if err != nil {
			ex, _ := os.Executable()
			appDir = filepath.Dir(ex)
		}

		log.Printf("📦 Executing DockPulse Auto-Update in directory: %s\n", appDir)

		// Case A: Git Repository Update
		if _, err := os.Stat(filepath.Join(appDir, ".git")); err == nil {
			UpdateMu.Lock()
			LastUpdateLog = "📥 Đang tải mã nguồn mới nhất từ GitHub qua Git..."
			UpdateMu.Unlock()

			pullOut, err := exec.Command("git", "pull", "origin", "main").CombinedOutput()
			if err != nil {
				UpdateMu.Lock()
				LastUpdateLog = fmt.Sprintf("❌ Lỗi git pull: %v (%s)", err, string(pullOut))
				UpdateMu.Unlock()
				return
			}

			// Rebuild Go binary if Go compiler exists
			if _, err := exec.LookPath("go"); err == nil {
				UpdateMu.Lock()
				LastUpdateLog = "⚙️ Đang biên dịch phiên bản Go binary mới..."
				UpdateMu.Unlock()

				buildOut, err := exec.Command("go", "build", "-o", "dockpulse", "main.go").CombinedOutput()
				if err != nil {
					UpdateMu.Lock()
					LastUpdateLog = fmt.Sprintf("❌ Lỗi biên dịch Go: %v (%s)", err, string(buildOut))
					UpdateMu.Unlock()
					return
				}
			}
		} else {
			// Case B: Standalone / Tarball Update
			UpdateMu.Lock()
			LastUpdateLog = "📥 Đang tải gói cập nhật (Tarball) từ GitHub..."
			UpdateMu.Unlock()

			tarUrl := fmt.Sprintf("https://github.com/%s/archive/refs/heads/main.tar.gz", GitHubRepo)
			if err := downloadAndExtractRepo(tarUrl, appDir); err != nil {
				UpdateMu.Lock()
				LastUpdateLog = fmt.Sprintf("❌ Lỗi tải & giải nén bản cập nhật: %v", err)
				UpdateMu.Unlock()
				return
			}

			if _, err := exec.LookPath("go"); err == nil {
				UpdateMu.Lock()
				LastUpdateLog = "⚙️ Đang biên dịch binary mới..."
				UpdateMu.Unlock()
				buildOut, err := exec.Command("go", "build", "-o", filepath.Join(appDir, "dockpulse"), filepath.Join(appDir, "main.go")).CombinedOutput()
				if err != nil {
					UpdateMu.Lock()
					LastUpdateLog = fmt.Sprintf("❌ Lỗi build: %v (%s)", err, string(buildOut))
					UpdateMu.Unlock()
					return
				}
			}
		}

		UpdateMu.Lock()
		LastUpdateLog = "✅ Nâng cấp thành công! Đang khởi động lại dịch vụ..."
		UpdateMu.Unlock()

		// Wait 1.5 seconds to let UI receive completion status, then restart service
		time.Sleep(1500 * time.Millisecond)

		if err := restartService(); err != nil {
			log.Printf("⚠️ Restart service error: %v (falling back to process exit)\n", err)
			os.Exit(0)
		}
	}()

	return nil
}

func restartService() error {
	// Try systemctl restart dockpulse
	cmd := exec.Command("systemctl", "restart", "dockpulse")
	if err := cmd.Start(); err == nil {
		return nil
	}
	return fmt.Errorf("systemctl restart failed")
}

func downloadAndExtractRepo(tarUrl, destDir string) error {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(tarUrl)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP error %d downloading update", resp.StatusCode)
	}

	gr, err := gzip.NewReader(resp.Body)
	if err != nil {
		return err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Skip root directory prefix (e.g. hostpanel-main/)
		parts := strings.Split(header.Name, "/")
		if len(parts) <= 1 {
			continue
		}
		relPath := strings.Join(parts[1:], "/")
		if relPath == "" {
			continue
		}

		targetPath := filepath.Join(destDir, relPath)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return err
			}
			outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				continue
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				continue
			}
			outFile.Close()
		}
	}

	return nil
}

// StartBackgroundChecker periodically polls GitHub and executes auto-update if enabled
func StartBackgroundChecker() {
	go func() {
		// First check 15 seconds after startup
		time.Sleep(15 * time.Second)
		if info, err := CheckUpdate(true); err == nil {
			if info.HasUpdate {
				log.Printf("🔔 [AutoUpdater] Có bản cập nhật mới: %s (Phiên bản hiện tại: %s)\n", info.LatestVersion, info.CurrentVersion)
			}
		}

		// Periodic check every 6 hours
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()

		for range ticker.C {
			info, err := CheckUpdate(true)
			if err != nil {
				continue
			}

			if info.HasUpdate && AutoUpdateEnabled {
				log.Printf("🌙 [AutoUpdater] Tự động nâng cấp phiên bản mới (%s)...", info.LatestVersion)
				_ = ApplyUpdate()
			}
		}
	}()
}
