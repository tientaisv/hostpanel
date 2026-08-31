package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"dockpulse/pkg/ai"
	"dockpulse/pkg/auth"
	"dockpulse/pkg/docker"
	"dockpulse/pkg/metrics"
	"dockpulse/pkg/system"
	"dockpulse/pkg/updater"
	"dockpulse/pkg/ws"
)

var client *docker.Client

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--install-service", "install", "-i":
			if err := system.AutoInstallSystemdService(); err != nil {
				log.Fatalf("❌ Cài đặt Systemd Service thất bại: %v", err)
			}
			fmt.Println("🎉 Đã cài đặt và kích hoạt Systemd Service thành công! Dịch vụ đang chạy ngầm.")
			return
		case "--update", "-u", "update":
			fmt.Println("🔍 Đang kiểm tra bản cập nhật mới nhất từ GitHub...")
			info, err := updater.CheckUpdate(true)
			if err != nil {
				log.Fatalf("❌ Lỗi kiểm tra cập nhật: %v", err)
			}
			fmt.Printf("📦 Phiên bản hiện tại: %s | Phiên bản mới nhất: %s\n", info.CurrentVersion, info.LatestVersion)
			if !info.HasUpdate {
				fmt.Println("✅ Bạn đang chạy phiên bản mới nhất!")
				return
			}
			fmt.Println("🚀 Bắt đầu nâng cấp phiên bản mới...")
			if err := updater.ApplyUpdateSync(true); err != nil {
				log.Fatalf("❌ Lỗi nâng cấp: %v", err)
			}
			fmt.Println("🎉 Nâng cấp hoàn tất! Dịch vụ đang khởi động lại.")
			return
		case "--version", "-v", "version":
			fmt.Printf("DockPulse %s\n", updater.CurrentVersion)
			return
		}
	}

	// Tự động kiểm tra và cài đặt Systemd Service nếu chưa có
	system.CheckAndAutoInstallIfMissing()

	// Khởi động trình tự động kiểm tra bản cập nhật ngầm
	updater.StartBackgroundChecker()

	client = docker.NewClient("")
	engine := client.GetEngineInfo()
	log.Printf("🚀 Container Engine detected: %s (Version: %s, API: %s, Socket: %s, IsPodman: %t)\n",
		engine.Name, engine.Version, engine.APIVersion, engine.SocketPath, engine.IsPodman)

	metrics.InitLogger(client)
	ai.InitRotater(".env")
	auth.InitAuth("config.json")

	port := os.Getenv("PORT")
	if port == "" {
		port = auth.GlobalAuth.Config.Port
	}

	mux := http.NewServeMux()

	// Public Auth Endpoints
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("/api/login", handleLogin)
	mux.HandleFunc("/api/logout", handleLogout)
	mux.HandleFunc("/api/auth/check", handleAuthCheck)

	// Protected REST Endpoints
	mux.HandleFunc("/api/host", authMiddleware(handleHostStats))
	mux.HandleFunc("/api/engine/info", authMiddleware(handleEngineInfo))
	mux.HandleFunc("/api/containers", authMiddleware(handleContainers))
	mux.HandleFunc("/api/containers/action", authMiddleware(handleContainerAction))
	mux.HandleFunc("/api/containers/remove", authMiddleware(handleContainerRemove))
	mux.HandleFunc("/api/containers/logs", authMiddleware(handleContainerLogsAPI))
	mux.HandleFunc("/api/containers/stats", authMiddleware(handleContainerStatsAPI))
	mux.HandleFunc("/api/containers/stats/all", authMiddleware(handleAllContainerStatsAPI))
	mux.HandleFunc("/api/docker/stats/total", authMiddleware(handleTotalDockerStatsAPI))
	mux.HandleFunc("/api/metrics/history", authMiddleware(handleMetricsHistoryAPI))

	mux.HandleFunc("/api/compose", authMiddleware(handleComposeStacks))
	mux.HandleFunc("/api/compose/action", authMiddleware(handleComposeAction))

	mux.HandleFunc("/api/images", authMiddleware(handleImages))
	mux.HandleFunc("/api/images/remove", authMiddleware(handleImageRemove))
	mux.HandleFunc("/api/images/prune", authMiddleware(handleImagePrune))

	mux.HandleFunc("/api/volumes", authMiddleware(handleVolumes))
	mux.HandleFunc("/api/volumes/remove", authMiddleware(handleVolumeRemove))
	mux.HandleFunc("/api/volumes/prune", authMiddleware(handleVolumePrune))

	mux.HandleFunc("/api/system/processes", authMiddleware(handleProcesses))
	mux.HandleFunc("/api/system/processes/kill", authMiddleware(handleProcessKill))
	mux.HandleFunc("/api/system/security", authMiddleware(handleSecurityAudit))
	mux.HandleFunc("/api/system/security/block-ip", authMiddleware(handleBlockIP))
	mux.HandleFunc("/api/system/fail2ban/status", authMiddleware(handleFail2banStatus))
	mux.HandleFunc("/api/system/fail2ban/install", authMiddleware(handleFail2banInstall))
	mux.HandleFunc("/api/system/fail2ban/unban", authMiddleware(handleFail2banUnban))
	mux.HandleFunc("/api/system/fail2ban/ban", authMiddleware(handleFail2banBan))
	mux.HandleFunc("/api/system/firewall/status", authMiddleware(handleFirewallStatus))
	mux.HandleFunc("/api/system/firewall/toggle", authMiddleware(handleFirewallToggle))
	mux.HandleFunc("/api/system/firewall/rule/add", authMiddleware(handleFirewallRuleAdd))
	mux.HandleFunc("/api/system/firewall/rule/delete", authMiddleware(handleFirewallRuleDelete))
	mux.HandleFunc("/api/system/update/check", authMiddleware(handleUpdateCheck))
	mux.HandleFunc("/api/system/update/apply", authMiddleware(handleUpdateApply))
	mux.HandleFunc("/api/system/update/config", authMiddleware(handleUpdateConfig))
	mux.HandleFunc("/api/system/swap/reset", authMiddleware(handleResetSwap))
	mux.HandleFunc("/api/system/pwmconfig", authMiddleware(handlePwmConfig))

	mux.HandleFunc("/api/ai/diagnose", authMiddleware(handleAIDiagnose))
	mux.HandleFunc("/api/ai/audit", authMiddleware(handleAIAudit))
	mux.HandleFunc("/api/ai/chat", authMiddleware(handleAIChat))
	mux.HandleFunc("/api/ai/exec_cmd", authMiddleware(handleAIExecCmd))

	mux.HandleFunc("/api/networks", authMiddleware(handleNetworks))
	mux.HandleFunc("/api/networks/create", authMiddleware(handleNetworkCreate))
	mux.HandleFunc("/api/networks/remove", authMiddleware(handleNetworkRemove))

	mux.HandleFunc("/api/system/ports", authMiddleware(handlePorts))
	mux.HandleFunc("/api/system/full-info", authMiddleware(handleFullServerInfo))

	mux.HandleFunc("/api/files/list", authMiddleware(handleFileList))
	mux.HandleFunc("/api/files/read", authMiddleware(handleFileRead))
	mux.HandleFunc("/api/files/save", authMiddleware(handleFileSave))
	mux.HandleFunc("/api/files/create", authMiddleware(handleFileCreate))
	mux.HandleFunc("/api/files/delete", authMiddleware(handleFileDelete))
	mux.HandleFunc("/api/files/download", authMiddleware(handleFileDownload))
	mux.HandleFunc("/api/files/upload", authMiddleware(handleFileUpload))
	mux.HandleFunc("/api/files/disk-usage", authMiddleware(handleFileDiskUsage))

	// Protected WebSocket Endpoints
	mux.HandleFunc("/ws/logs", authWSMiddleware(handleWSLogs))
	mux.HandleFunc("/ws/terminal", authWSMiddleware(handleWSTerminal))
	mux.HandleFunc("/ws/host_terminal", authWSMiddleware(handleWSHostTerminal))
	mux.HandleFunc("/ws/stats", authWSMiddleware(handleWSStats))

	// Static Web Files
	webDir := "./web"
	if _, err := os.Stat(webDir); os.IsNotExist(err) {
		ex, errEx := os.Executable()
		if errEx == nil {
			webDir = filepath.Join(filepath.Dir(ex), "web")
		}
	}
	fs := http.FileServer(http.Dir(webDir))
	mux.Handle("/", staticAuthMiddleware(fs, webDir))

	addr := ":" + port
	fmt.Printf("🔒 DockPulse starting with Authentication (User: %s) on http://0.0.0.0%s (Loaded Gemini Keys: %d, Groq Keys: %d) ...\n",
		auth.GlobalAuth.Config.AdminUser, addr, ai.GlobalRotater.GeminiKeysCount(), ai.GlobalRotater.GroqKeysCount())

	if err := http.ListenAndServe(addr, securityHeadersMiddleware(mux)); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !auth.GlobalAuth.ValidateRequest(r) {
			jsonResponse(w, 401, map[string]string{"error": "Unauthorized access. Please login."})
			return
		}
		next(w, r)
	}
}

func authWSMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !auth.GlobalAuth.ValidateRequest(r) {
			http.Error(w, "Unauthorized WebSocket connection", 401)
			return
		}
		next(w, r)
	}
}

func staticAuthMiddleware(next http.Handler, webDir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		// Whitelist public resources required by login page
		if path == "/login.html" || path == "/css/style.css" || strings.HasPrefix(path, "/js/login.js") || path == "/favicon.ico" {
			next.ServeHTTP(w, r)
			return
		}
		// If not authenticated, redirect HTML pages to login and block static scripts/styles
		if !auth.GlobalAuth.ValidateRequest(r) {
			if path == "/" || path == "/index.html" || strings.HasSuffix(path, ".html") {
				http.Redirect(w, r, "/login.html", http.StatusFound)
				return
			}
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, 405, map[string]string{"error": "Method not allowed"})
		return
	}

	clientIP := auth.ExtractClientIP(r)
	allowed, retryAfterSec := auth.GlobalAuth.CheckRateLimit(clientIP)
	if !allowed {
		jsonResponse(w, 429, map[string]interface{}{
			"error":       fmt.Sprintf("Bạn đã đăng nhập sai quá nhiều lần. Vui lòng thử lại sau %d giây.", retryAfterSec),
			"retry_after": retryAfterSec,
		})
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, 400, map[string]string{"error": err.Error()})
		return
	}

	if auth.GlobalAuth.Authenticate(req.Username, req.Password) {
		auth.GlobalAuth.ResetAttempts(clientIP)
		token, err := auth.GlobalAuth.CreateSession(w, r)
		if err != nil {
			jsonResponse(w, 500, map[string]string{"error": err.Error()})
			return
		}
		jsonResponse(w, 200, map[string]interface{}{
			"status":              "authenticated",
			"token":               token,
			"user":                req.Username,
			"is_default_password": auth.GlobalAuth.IsDefaultCredentials(),
		})
		return
	}

	lockSec := auth.GlobalAuth.RecordFailedAttempt(clientIP)
	if lockSec > 0 {
		jsonResponse(w, 429, map[string]interface{}{
			"error": "Tên đăng nhập hoặc mật khẩu không chính xác. IP của bạn đã bị tạm khóa 10 phút do thử sai 5 lần liên tiếp.",
		})
		return
	}

	jsonResponse(w, 401, map[string]string{"error": "Tên đăng nhập hoặc mật khẩu không chính xác."})
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	auth.GlobalAuth.ClearSession(w, r)
	jsonResponse(w, 200, map[string]string{"status": "logged out"})
}

func handleAuthCheck(w http.ResponseWriter, r *http.Request) {
	if auth.GlobalAuth.ValidateRequest(r) {
		engine := client.GetEngineInfo()
		jsonResponse(w, 200, map[string]interface{}{
			"status":              "authenticated",
			"user":                auth.GlobalAuth.Config.AdminUser,
			"engine_name":         engine.Name,
			"engine_version":      engine.Version,
			"is_podman":           engine.IsPodman,
			"is_default_password": auth.GlobalAuth.IsDefaultCredentials(),
		})
		return
	}
	jsonResponse(w, 401, map[string]string{"status": "unauthenticated"})
}

func handleEngineInfo(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, 200, client.GetEngineInfo())
}

func handleHostStats(w http.ResponseWriter, r *http.Request) {
	stats, err := system.GetHostStats()
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}

	engine := client.GetEngineInfo()
	stats.EngineName = engine.Name
	stats.EngineVersion = engine.Version
	stats.IsPodman = engine.IsPodman

	if summary, errSum := client.GetTotalDockerStats(stats.MemTotalMB); errSum == nil && summary != nil {
		stats.Containers = summary.TotalContainers
		stats.DockerRunningCount = summary.RunningContainers
		stats.DockerCPUPercent = summary.CPUPercent
		stats.DockerMemUsedMB = summary.MemUsedMB
		stats.DockerMemPercent = summary.MemPercent
		stats.DockerNetRxMB = summary.NetRxMB
		stats.DockerNetTxMB = summary.NetTxMB
	}
	jsonResponse(w, 200, stats)
}

func handleFullServerInfo(w http.ResponseWriter, r *http.Request) {
	info, err := system.GetFullServerInfo()
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, info)
}

func handleContainers(w http.ResponseWriter, r *http.Request) {
	ctrs, err := client.ListContainers()
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, ctrs)
}

func handleContainerAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, 405, map[string]string{"error": "Method not allowed"})
		return
	}
	var req struct {
		ID     string `json:"id"`
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if err := client.ContainerAction(req.ID, req.Action); err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, map[string]string{"status": "ok"})
}

func handleContainerRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, 405, map[string]string{"error": "Method not allowed"})
		return
	}
	var req struct {
		ID    string `json:"id"`
		Force bool   `json:"force"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if err := client.RemoveContainer(req.ID, req.Force); err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, map[string]string{"status": "ok"})
}

func handleContainerLogsAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method == "DELETE" {
		id := r.URL.Query().Get("id")
		if id == "" {
			jsonResponse(w, 400, map[string]string{"error": "missing id"})
			return
		}
		if err := client.TruncateContainerLogs(id); err != nil {
			jsonResponse(w, 500, map[string]string{"error": err.Error()})
			return
		}
		jsonResponse(w, 200, map[string]string{"status": "logs cleared"})
		return
	}
	jsonResponse(w, 405, map[string]string{"error": "Method not allowed"})
}

func handleContainerStatsAPI(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		jsonResponse(w, 400, map[string]string{"error": "missing id"})
		return
	}
	if id == "all" {
		handleAllContainerStatsAPI(w, r)
		return
	}
	stats, err := client.GetContainerStats(id)
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, stats)
}

func handleAllContainerStatsAPI(w http.ResponseWriter, r *http.Request) {
	stats, err := client.GetAllContainersStats()
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, stats)
}

func handleTotalDockerStatsAPI(w http.ResponseWriter, r *http.Request) {
	hostStats, _ := system.GetHostStats()
	memTotal := uint64(0)
	if hostStats != nil {
		memTotal = hostStats.MemTotalMB
	}
	summary, err := client.GetTotalDockerStats(memTotal)
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, summary)
}

func handleMetricsHistoryAPI(w http.ResponseWriter, r *http.Request) {
	timeRange := r.URL.Query().Get("range")
	if timeRange == "" {
		timeRange = "24h"
	}
	if metrics.GlobalLogger == nil {
		jsonResponse(w, 500, map[string]string{"error": "metrics logger not initialized"})
		return
	}
	records, err := metrics.GlobalLogger.FetchHistory(timeRange)
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, records)
}

func handleComposeStacks(w http.ResponseWriter, r *http.Request) {
	includeStats := r.URL.Query().Get("stats") == "true"
	stacks, err := client.ListComposeStacksWithStats(includeStats)
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, stacks)
}

func handleComposeAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, 405, map[string]string{"error": "Method not allowed"})
		return
	}
	var req struct {
		Project string `json:"project"`
		Action  string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if err := client.StackAction(req.Project, req.Action); err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, map[string]string{"status": "ok"})
}

func handleImages(w http.ResponseWriter, r *http.Request) {
	imgs, err := client.ListImages()
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, imgs)
}

func handleImageRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, 405, map[string]string{"error": "Method not allowed"})
		return
	}
	var req struct {
		ID    string `json:"id"`
		Force bool   `json:"force"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if err := client.RemoveImage(req.ID, req.Force); err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, map[string]string{"status": "ok"})
}

func handleImagePrune(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, 405, map[string]string{"error": "Method not allowed"})
		return
	}
	res, err := client.PruneImages()
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(res)
}

func handleVolumes(w http.ResponseWriter, r *http.Request) {
	vols, err := client.ListVolumes()
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, vols)
}

func handleVolumeRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, 405, map[string]string{"error": "Method not allowed"})
		return
	}
	var req struct {
		Name  string `json:"name"`
		Force bool   `json:"force"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if err := client.RemoveVolume(req.Name, req.Force); err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, map[string]string{"status": "ok"})
}

func handleVolumePrune(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, 405, map[string]string{"error": "Method not allowed"})
		return
	}
	res, err := client.PruneVolumes()
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(res)
}

func handleProcesses(w http.ResponseWriter, r *http.Request) {
	sortBy := r.URL.Query().Get("sort")
	if sortBy == "" {
		sortBy = "cpu"
	}
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	procs, err := system.GetRunningProcesses(system.ProcessListOptions{
		SortBy: sortBy,
		Limit:  limit,
	})
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, procs)
}

func handleProcessKill(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, 405, map[string]string{"error": "Method not allowed"})
		return
	}
	var req struct {
		PID int `json:"pid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if err := system.KillProcess(req.PID, 9); err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, map[string]string{"status": "killed"})
}

func handleResetSwap(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, 405, map[string]string{"error": "Method not allowed"})
		return
	}
	msg, err := system.ResetSwap()
	if err != nil {
		jsonResponse(w, 400, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, map[string]string{"status": "ok", "message": msg})
}

func handlePwmConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, 405, map[string]string{"error": "Method not allowed"})
		return
	}
	var req struct {
		Channel *int `json:"channel"`
		Speed   *int `json:"speed"`
	}
	channel := 0
	speed := 255
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Channel != nil {
			channel = *req.Channel
		}
		if req.Speed != nil {
			speed = *req.Speed
		}
	}
	msg, err := system.RunPwmConfig(channel, speed)
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error(), "output": msg})
		return
	}
	jsonResponse(w, 200, map[string]string{"status": "ok", "message": msg})
}

func handleSecurityAudit(w http.ResponseWriter, r *http.Request) {
	report, err := system.PerformSecurityAudit()
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if auth.GlobalAuth != nil && auth.GlobalAuth.IsDefaultCredentials() {
		report.ThreatScore += 30
		report.Threats = append([]system.SecurityThreatItem{
			{
				Level:       "CRITICAL",
				Category:    "Authentication Risk",
				Title:       "Đang sử dụng tài khoản & mật khẩu mặc định (admin / dockpulse2026)",
				Description: "Hệ thống đang chạy với tài khoản quản trị mặc định. Bất kỳ ai trên mạng đều có thể đăng nhập và kiểm soát toàn bộ máy chủ.",
				ActionHint:  "Hãy đổi mật khẩu bằng cách cấu hình biến ADMIN_PASS trong file .env hoặc config.json",
			},
		}, report.Threats...)
		if report.ThreatScore >= 60 {
			report.ThreatLevel = "CRITICAL"
		} else if report.ThreatScore >= 20 {
			report.ThreatLevel = "WARNING"
		}
	}
	jsonResponse(w, 200, report)
}

func handleBlockIP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, 405, map[string]string{"error": "Method not allowed"})
		return
	}
	var req struct {
		IP string `json:"ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.IP == "" {
		jsonResponse(w, 400, map[string]string{"error": "invalid ip address"})
		return
	}
	if err := system.BlockIPWithFirewall(req.IP); err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, map[string]string{"status": "blocked", "ip": req.IP})
}

func handleFail2banStatus(w http.ResponseWriter, r *http.Request) {
	status, err := system.GetFail2banStatus()
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, status)
}

func handleFail2banInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, 405, map[string]string{"error": "Method not allowed"})
		return
	}
	clientIP := system.ExtractClientIP(r)
	if err := system.InstallAndConfigureFail2ban(clientIP); err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, map[string]string{"status": "ok", "message": "Fail2ban đã được cài đặt và kích hoạt bảo vệ thành công!"})
}

func handleFail2banUnban(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, 405, map[string]string{"error": "Method not allowed"})
		return
	}
	var req struct {
		Jail string `json:"jail"`
		IP   string `json:"ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.IP == "" {
		jsonResponse(w, 400, map[string]string{"error": "invalid request body or missing IP"})
		return
	}
	if err := system.UnbanIP(req.Jail, req.IP); err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, map[string]string{"status": "ok", "message": fmt.Sprintf("Đã mở chặn IP %s thành công!", req.IP)})
}

func handleFail2banBan(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, 405, map[string]string{"error": "Method not allowed"})
		return
	}
	var req struct {
		Jail string `json:"jail"`
		IP   string `json:"ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.IP == "" {
		jsonResponse(w, 400, map[string]string{"error": "invalid request body or missing IP"})
		return
	}
	if err := system.BanIP(req.Jail, req.IP); err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, map[string]string{"status": "ok", "message": fmt.Sprintf("Đã thêm quy tắc chặn IP %s thành công!", req.IP)})
}

func handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	force := r.URL.Query().Get("force") == "true"
	info, err := updater.CheckUpdate(force)
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, info)
}

func handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, 405, map[string]string{"error": "Method not allowed"})
		return
	}
	if err := updater.ApplyUpdate(); err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, map[string]string{
		"status":  "updating",
		"message": "Quá trình nâng cấp đã bắt đầu. Dịch vụ sẽ tự động khởi động lại sau khi hoàn tất.",
	})
}

func handleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		var req struct {
			AutoUpdateEnabled bool `json:"auto_update_enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonResponse(w, 400, map[string]string{"error": err.Error()})
			return
		}
		updater.AutoUpdateEnabled = req.AutoUpdateEnabled
		jsonResponse(w, 200, map[string]interface{}{"status": "ok", "auto_update_enabled": updater.AutoUpdateEnabled})
		return
	}
	jsonResponse(w, 200, map[string]interface{}{"auto_update_enabled": updater.AutoUpdateEnabled})
}

func handleFirewallStatus(w http.ResponseWriter, r *http.Request) {
	status, err := system.GetFirewallStatus()
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, status)
}

func handleFirewallToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, 405, map[string]string{"error": "Method not allowed"})
		return
	}
	var req struct {
		Enable bool `json:"enable"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if err := system.ToggleFirewall(req.Enable); err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	msg := "Đã tạm tắt tường lửa."
	if req.Enable {
		msg = "Đã kích hoạt tường lửa an toàn thành công (Đã giữ mở cổng SSH 22 & DockPulse 3800)!"
	}
	jsonResponse(w, 200, map[string]string{"status": "ok", "message": msg})
}

func handleFirewallRuleAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, 405, map[string]string{"error": "Method not allowed"})
		return
	}
	var rule system.FirewallRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		jsonResponse(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if err := system.AddFirewallRule(rule); err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, map[string]string{"status": "ok", "message": "Đã thêm quy tắc tường lửa thành công!"})
}

func handleFirewallRuleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, 405, map[string]string{"error": "Method not allowed"})
		return
	}
	var req struct {
		ID       string `json:"id"`
		Port     string `json:"port"`
		Protocol string `json:"protocol"`
		Action   string `json:"action"`
		FromIP   string `json:"from_ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if err := system.DeleteFirewallRule(req.ID, req.Port, req.Protocol, req.Action, req.FromIP); err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, map[string]string{"status": "ok", "message": "Đã xóa quy tắc tường lửa thành công!"})
}

func handleAIDiagnose(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, 405, map[string]string{"error": "Method not allowed"})
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, 400, map[string]string{"error": err.Error()})
		return
	}

	ctrs, _ := client.ListContainers()
	ctrInfo := fmt.Sprintf("ID: %s", req.ID)
	for _, c := range ctrs {
		if c.ID == req.ID || c.ShortID == req.ID {
			b, _ := json.Marshal(c)
			ctrInfo = string(b)
			break
		}
	}

	stats, _ := client.GetContainerStats(req.ID)
	statsJson, _ := json.Marshal(stats)
	ctrInfo += fmt.Sprintf("\nStats: %s", string(statsJson))

	diagResult, err := ai.DiagnoseContainer(ctrInfo, "Recent log active stream output")
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}

	jsonResponse(w, 200, map[string]string{"diagnosis": diagResult})
}

func handleAIAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, 405, map[string]string{"error": "Method not allowed"})
		return
	}

	hostStats, _ := system.GetHostStats()
	hostJson, _ := json.Marshal(hostStats)

	procs, _ := system.GetRunningProcesses(system.ProcessListOptions{SortBy: "cpu", Limit: 5})
	procsJson, _ := json.Marshal(procs)

	ctrs, _ := client.ListContainers()
	ctrsJson, _ := json.Marshal(ctrs)

	info := fmt.Sprintf("HOST METRICS:\n%s\n\nTOP HEAVY PROCESSES:\n%s\n\nCONTAINERS (%d):\n%s",
		string(hostJson), string(procsJson), len(ctrs), string(ctrsJson))

	auditResult, err := ai.AuditSystemHealth(info)
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}

	jsonResponse(w, 200, map[string]string{"audit": auditResult})
}

func handleAIChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, 405, map[string]string{"error": "Method not allowed"})
		return
	}
	var req struct {
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Prompt == "" {
		jsonResponse(w, 400, map[string]string{"error": "invalid prompt"})
		return
	}

	hostStats, _ := system.GetHostStats()
	procs, _ := system.GetRunningProcesses(system.ProcessListOptions{SortBy: "cpu", Limit: 5})
	ctrs, _ := client.ListContainers()

	contextStr := fmt.Sprintf("--- LIVE HOST CONTEXT ---\nCPU: %.1f%% | RAM: %d/%d MB (%.1f%%) | Swap: %d/%d MB (%.1f%%) | Disk: %d/%d GB (%.1f%%)\nLoad: %.2f %.2f %.2f | Containers Total: %d\nTop Heavy Processes: %v\n",
		hostStats.CPUPercent, hostStats.MemUsedMB, hostStats.MemTotalMB, hostStats.MemPercent,
		hostStats.SwapUsedMB, hostStats.SwapTotalMB, hostStats.SwapPercent,
		hostStats.DiskUsedGB, hostStats.DiskTotalGB, hostStats.DiskPercent,
		hostStats.LoadAvg1, hostStats.LoadAvg5, hostStats.LoadAvg15,
		len(ctrs), procs)

	fullPrompt := fmt.Sprintf("Bạn là Trợ lý AI Senior đang trực tiếp quản lý và khắc phục sự cố trên máy chủ Linux này.\n\n%s\n\n--- CÂU HỎI / YÊU CẦU TỪ NGƯỜI DÙNG ---\n%s\n\n--- YÊU CẦU BÁO CÁO (Định dạng Markdown tiếng Việt) ---\nDựa vào DỮ LIỆU THỰC TẾ LIVE CỦA MÁY CHỦ ở trên:\n1. Phân tích nguyên nhân & liên hệ trực tiếp với dữ liệu thực tế đang xảy ra trên Host.\n2. Đưa ra hướng giải quyết cụ thể.", contextStr, req.Prompt)

	resp, err := ai.QueryAI(fullPrompt)
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}

	jsonResponse(w, 200, map[string]string{"response": resp})
}

func handleAIExecCmd(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, 405, map[string]string{"error": "Method not allowed"})
		return
	}
	var req struct {
		Command string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Command == "" {
		jsonResponse(w, 400, map[string]string{"error": "invalid command"})
		return
	}

	result, err := ai.ExecuteBashCommand(req.Command)
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}

	jsonResponse(w, 200, result)
}

func handleWSLogs(w http.ResponseWriter, r *http.Request) {
	wsConn, err := ws.Upgrade(w, r)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	defer wsConn.Close()

	id := r.URL.Query().Get("id")
	tail := r.URL.Query().Get("tail")
	if id == "" {
		_ = wsConn.WriteText("Error: missing container id")
		return
	}

	_ = client.StreamLogsToWS(id, tail, wsConn)
}

func handleWSTerminal(w http.ResponseWriter, r *http.Request) {
	wsConn, err := ws.Upgrade(w, r)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	defer wsConn.Close()

	id := r.URL.Query().Get("id")
	if id == "" {
		_ = wsConn.WriteText("Error: missing container id")
		return
	}

	_ = client.HandleWebTerminal(id, wsConn)
}

func handleWSHostTerminal(w http.ResponseWriter, r *http.Request) {
	wsConn, err := ws.Upgrade(w, r)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	defer wsConn.Close()

	_ = system.HandleHostWebTerminal(wsConn)
}

func handleWSStats(w http.ResponseWriter, r *http.Request) {
	wsConn, err := ws.Upgrade(w, r)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	defer wsConn.Close()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		stats, errHost := system.GetHostStats()
		if errHost == nil {
			if summary, errSum := client.GetTotalDockerStats(stats.MemTotalMB); errSum == nil && summary != nil {
				stats.Containers = summary.TotalContainers
				stats.DockerRunningCount = summary.RunningContainers
				stats.DockerCPUPercent = summary.CPUPercent
				stats.DockerMemUsedMB = summary.MemUsedMB
				stats.DockerMemPercent = summary.MemPercent
				stats.DockerNetRxMB = summary.NetRxMB
				stats.DockerNetTxMB = summary.NetTxMB
			}
			b, _ := json.Marshal(stats)
			if errWs := wsConn.WriteText(string(b)); errWs != nil {
				break
			}
		}
	}
}

func handleNetworks(w http.ResponseWriter, r *http.Request) {
	nets, err := client.ListNetworks()
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, nets)
}

func handleNetworkCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, 405, map[string]string{"error": "Method not allowed"})
		return
	}
	var req struct {
		Name   string `json:"name"`
		Driver string `json:"driver"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if err := client.CreateNetwork(req.Name, req.Driver); err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, map[string]string{"status": "ok"})
}

func handleNetworkRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, 405, map[string]string{"error": "Method not allowed"})
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if err := client.RemoveNetwork(req.ID); err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, map[string]string{"status": "ok"})
}

func handlePorts(w http.ResponseWriter, r *http.Request) {
	ports, err := system.GetListeningPorts()
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, ports)
}

func handleFileList(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	items, err := system.ListFiles(path)
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, items)
}

func handleFileRead(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	content, err := system.ReadFileContent(path)
	if err != nil {
		jsonResponse(w, 400, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, map[string]string{"content": content, "path": path})
}

func handleFileSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, 405, map[string]string{"error": "Method not allowed"})
		return
	}
	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if err := system.WriteFileContent(req.Path, req.Content); err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, map[string]string{"status": "saved"})
}

func handleFileCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, 405, map[string]string{"error": "Method not allowed"})
		return
	}
	var req struct {
		Path  string `json:"path"`
		IsDir bool   `json:"is_dir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if err := system.CreateItem(req.Path, req.IsDir); err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, map[string]string{"status": "created"})
}

func handleFileDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, 405, map[string]string{"error": "Method not allowed"})
		return
	}
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if err := system.DeleteItem(req.Path); err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, map[string]string{"status": "deleted"})
}

func handleFileDownload(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if path == "" {
		http.Error(w, "missing path", 400)
		return
	}
	cleanPath := filepath.Clean(path)
	http.ServeFile(w, r, cleanPath)
}

func handleFileUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, 405, map[string]string{"error": "Method not allowed"})
		return
	}

	err := r.ParseMultipartForm(50 << 20)
	if err != nil {
		jsonResponse(w, 400, map[string]string{"error": err.Error()})
		return
	}

	dir := strings.TrimSpace(r.FormValue("dir"))
	if dir == "" {
		dir = "/"
	}
	cleanDir := filepath.Clean(dir)

	file, header, err := r.FormFile("file")
	if err != nil {
		jsonResponse(w, 400, map[string]string{"error": err.Error()})
		return
	}
	defer file.Close()

	cleanFilename := filepath.Base(filepath.Clean(header.Filename))
	if cleanFilename == "" || cleanFilename == "." || cleanFilename == "/" || cleanFilename == ".." {
		jsonResponse(w, 400, map[string]string{"error": "Tên tệp tin không hợp lệ"})
		return
	}

	destPath := filepath.Join(cleanDir, cleanFilename)
	out, err := os.Create(destPath)
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer out.Close()

	_, err = io.Copy(out, file)
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}

	jsonResponse(w, 200, map[string]string{"status": "uploaded", "path": destPath})
}

func handleFileDiskUsage(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/"
	}
	topStr := r.URL.Query().Get("top")
	topN := 15
	if topStr != "" {
		if val, err := strconv.Atoi(topStr); err == nil && val > 0 {
			topN = val
		}
	}

	summary, err := system.GetFolderDiskUsage(path, topN)
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, 200, summary)
}
