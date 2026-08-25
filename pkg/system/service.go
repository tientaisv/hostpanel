package system

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

// AutoInstallSystemdService installs and activates systemd service automatically
func AutoInstallSystemdService() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("cần quyền root để cài đặt Systemd Service")
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("không thể xác định đường dẫn file thực thi: %v", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("không thể resolve symlink: %v", err)
	}

	workDir := filepath.Dir(execPath)
	serviceContent := fmt.Sprintf(`[Unit]
Description=DockPulse - Ultra Lightweight Docker & Podman Compose Manager
After=network.target docker.service podman.service podman.socket
Wants=docker.service podman.service podman.socket

[Service]
Type=simple
User=root
WorkingDirectory=%s
ExecStart=%s
Restart=always
RestartSec=5
Environment=PORT=3800

[Install]
WantedBy=multi-user.target
`, workDir, execPath)

	servicePath := "/etc/systemd/system/dockpulse.service"
	err = ioutil.WriteFile(servicePath, []byte(serviceContent), 0644)
	if err != nil {
		return fmt.Errorf("không thể ghi file %s: %v", servicePath, err)
	}

	// Reload daemon
	_ = exec.Command("systemctl", "daemon-reload").Run()
	// Enable service
	_ = exec.Command("systemctl", "enable", "dockpulse").Run()
	// Start service
	_ = exec.Command("systemctl", "restart", "dockpulse").Run()

	log.Printf("🚀 [Systemd] Đã tự động tạo và kích hoạt dịch vụ tại: %s\n", servicePath)
	return nil
}

// CheckAndAutoInstallIfMissing tự động tạo service nếu chưa có trên Linux
func CheckAndAutoInstallIfMissing() {
	if os.Geteuid() != 0 {
		return
	}

	servicePath := "/etc/systemd/system/dockpulse.service"
	if _, err := os.Stat(servicePath); os.IsNotExist(err) {
		log.Println("⚙️ Phát hiện chưa có Systemd Service -> Đang tự động tạo và kích hoạt...")
		if errInst := AutoInstallSystemdService(); errInst != nil {
			log.Printf("⚠️ Tự động tạo Systemd Service thất bại: %v\n", errInst)
		} else {
			log.Println("✅ Đã tự động thiết lập Systemd Service thành công!")
		}
	}
}
