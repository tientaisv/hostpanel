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
	CurrentVersion    = "v1.5.0"
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

	// 1. If inside a git repository, check git remote directly (works for both public and private repos)
	localSHA := getLocalGitSHA()
	if localSHA != "" {
		remoteOut, err := exec.Command("git", "ls-remote", "origin", "-h", "refs/heads/main").Output()
		if err == nil && len(remoteOut) > 0 {
			fields := strings.Fields(string(remoteOut))
			if len(fields) > 0 {
				remoteSHA := fields[0]
				shortRemote := remoteSHA
				if len(shortRemote) > 7 {
					shortRemote = shortRemote[:7]
				}
				info.LatestCommitSHA = remoteSHA
				info.LatestVersion = fmt.Sprintf("Commit %s", shortRemote)
				info.ReleaseNotes = "Bản cập nhật mới nhất từ nhánh main trên GitHub."

				if localSHA != remoteSHA && !strings.HasPrefix(localSHA, remoteSHA) && !strings.HasPrefix(remoteSHA, localSHA) {
					info.HasUpdate = true
				}
			}
		}
	}

	// 2. Try checking GitHub Release API for formal releases
	client := &http.Client{Timeout: 5 * time.Second}
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
		}
	}
	if respRel != nil {
		respRel.Body.Close()
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

// ApplyUpdate executes the self-update process asynchronously (for Web UI)
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

		if err := executeSelfUpdate(false); err != nil {
			UpdateMu.Lock()
			LastUpdateLog = fmt.Sprintf("❌ Lỗi nâng cấp: %v", err)
			UpdateMu.Unlock()
			return
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

// ApplyUpdateSync executes the self-update synchronously (for CLI dockpulse --update)
func ApplyUpdateSync(printProgress bool) error {
	UpdateMu.Lock()
	if IsUpdating {
		UpdateMu.Unlock()
		return fmt.Errorf("hệ thống đang trong quá trình cập nhật, vui lòng đợi")
	}
	IsUpdating = true
	UpdateMu.Unlock()

	defer func() {
		UpdateMu.Lock()
		IsUpdating = false
		UpdateMu.Unlock()
	}()

	if err := executeSelfUpdate(printProgress); err != nil {
		return err
	}

	if printProgress {
		fmt.Println("🔄 Đang khởi động lại dịch vụ dockpulse...")
	}
	_ = restartService()
	return nil
}

func executeSelfUpdate(printProgress bool) error {
	appDir, err := os.Getwd()
	if err != nil {
		ex, _ := os.Executable()
		appDir = filepath.Dir(ex)
	}

	log.Printf("📦 Executing DockPulse Auto-Update in directory: %s\n", appDir)

	// Case A: Git Repository Update
	if _, err := os.Stat(filepath.Join(appDir, ".git")); err == nil {
		if printProgress {
			fmt.Println("📥 Đang kéo mã nguồn mới nhất từ GitHub qua Git...")
		}
		UpdateMu.Lock()
		LastUpdateLog = "📥 Đang tải mã nguồn mới nhất từ GitHub qua Git..."
		UpdateMu.Unlock()

		pullOut, err := exec.Command("git", "pull", "origin", "main").CombinedOutput()
		if err != nil {
			return fmt.Errorf("lỗi git pull: %v (%s)", err, string(pullOut))
		}

		// Ensure Go compiler is ready
		if printProgress {
			fmt.Println("🔍 Kiểm tra trình biên dịch Go...")
		}
		_ = ensureGoCompiler()

		// Rebuild Go binary
		if _, err := exec.LookPath("go"); err == nil {
			if printProgress {
				fmt.Println("⚙️ Đang biên dịch phiên bản Go binary mới...")
			}
			UpdateMu.Lock()
			LastUpdateLog = "⚙️ Đang biên dịch phiên bản Go binary mới..."
			UpdateMu.Unlock()

			buildOut, err := exec.Command("go", "build", "-o", filepath.Join(appDir, "dockpulse"), filepath.Join(appDir, "main.go")).CombinedOutput()
			if err != nil {
				return fmt.Errorf("lỗi biên dịch Go: %v (%s)", err, string(buildOut))
			}
		}
	} else {
		// Case B: Standalone / Tarball Update
		if printProgress {
			fmt.Println("📥 Đang tải gói cập nhật (Tarball) từ GitHub...")
		}
		UpdateMu.Lock()
		LastUpdateLog = "📥 Đang tải gói cập nhật (Tarball) từ GitHub..."
		UpdateMu.Unlock()

		tarUrl := fmt.Sprintf("https://github.com/%s/archive/refs/heads/main.tar.gz", GitHubRepo)
		if err := downloadAndExtractRepo(tarUrl, appDir); err != nil {
			return fmt.Errorf("lỗi tải & giải nén bản cập nhật: %v", err)
		}

		if printProgress {
			fmt.Println("🔍 Kiểm tra trình biên dịch Go...")
		}
		_ = ensureGoCompiler()
		if _, err := exec.LookPath("go"); err == nil {
			if printProgress {
				fmt.Println("⚙️ Đang biên dịch binary mới...")
			}
			UpdateMu.Lock()
			LastUpdateLog = "⚙️ Đang biên dịch binary mới..."
			UpdateMu.Unlock()
			buildOut, err := exec.Command("go", "build", "-o", filepath.Join(appDir, "dockpulse"), filepath.Join(appDir, "main.go")).CombinedOutput()
			if err != nil {
				return fmt.Errorf("lỗi build binary: %v (%s)", err, string(buildOut))
			}
		}
	}

	return nil
}

func ensureGoCompiler() error {
	if _, err := exec.LookPath("go"); err == nil {
		return nil
	}
	log.Println("⚙️ Go compiler not found. Auto-installing lightweight Go runtime...")
	if _, err := exec.LookPath("apt-get"); err == nil {
		cmd := exec.Command("sh", "-c", "DEBIAN_FRONTEND=noninteractive apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq golang-go")
		return cmd.Run()
	} else if _, err := exec.LookPath("dnf"); err == nil {
		return exec.Command("dnf", "install", "-y", "-q", "golang").Run()
	} else if _, err := exec.LookPath("yum"); err == nil {
		return exec.Command("yum", "install", "-y", "-q", "golang").Run()
	}
	return fmt.Errorf("no package manager found to install Go")
}

func restartService() error {
	// Try systemctl restart dockpulse
	cmd := exec.Command("systemctl", "restart", "dockpulse")
	if err := cmd.Start(); err == nil {
		return nil
	}
	return fmt.Errorf("systemctl restart failed")
}

func getGitHubToken() string {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	return strings.TrimSpace(token)
}

func downloadAndExtractRepo(tarUrl, destDir string) error {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", tarUrl, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "DockPulse-AutoUpdater")
	if token := getGitHubToken(); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}

	resp, err := client.Do(req)
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
