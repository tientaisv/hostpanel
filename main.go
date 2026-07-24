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
	"dockpulse/pkg/ws"
)

var client *docker.Client

func main() {
	client = docker.NewClient("/var/run/docker.sock")
	metrics.InitLogger(client)
	ai.InitRotater(".env", "/root/hostcontrol/.env", "/home/data/appck/.env", "/home/data/taissh/.env")
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
	mux.HandleFunc("/api/system/swap/reset", authMiddleware(handleResetSwap))

	mux.HandleFunc("/api/ai/diagnose", authMiddleware(handleAIDiagnose))
	mux.HandleFunc("/api/ai/audit", authMiddleware(handleAIAudit))
	mux.HandleFunc("/api/ai/chat", authMiddleware(handleAIChat))
	mux.HandleFunc("/api/ai/exec_cmd", authMiddleware(handleAIExecCmd))

	mux.HandleFunc("/api/networks", authMiddleware(handleNetworks))
	mux.HandleFunc("/api/networks/create", authMiddleware(handleNetworkCreate))
	mux.HandleFunc("/api/networks/remove", authMiddleware(handleNetworkRemove))

	mux.HandleFunc("/api/system/ports", authMiddleware(handlePorts))

	mux.HandleFunc("/api/files/list", authMiddleware(handleFileList))
	mux.HandleFunc("/api/files/read", authMiddleware(handleFileRead))
	mux.HandleFunc("/api/files/save", authMiddleware(handleFileSave))
	mux.HandleFunc("/api/files/create", authMiddleware(handleFileCreate))
	mux.HandleFunc("/api/files/delete", authMiddleware(handleFileDelete))
	mux.HandleFunc("/api/files/download", authMiddleware(handleFileDownload))
	mux.HandleFunc("/api/files/upload", authMiddleware(handleFileUpload))

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

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Server error: %v", err)
	}
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
		if path == "/login.html" || path == "/css/style.css" || strings.HasPrefix(path, "/js/login.js") {
			next.ServeHTTP(w, r)
			return
		}
		if !auth.GlobalAuth.ValidateRequest(r) {
			if path == "/" || path == "/index.html" {
				http.Redirect(w, r, "/login.html", http.StatusFound)
				return
			}
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
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, 400, map[string]string{"error": err.Error()})
		return
	}

	if auth.GlobalAuth.Authenticate(req.Username, req.Password) {
		token, err := auth.GlobalAuth.CreateSession(w)
		if err != nil {
			jsonResponse(w, 500, map[string]string{"error": err.Error()})
			return
		}
		jsonResponse(w, 200, map[string]string{
			"status": "authenticated",
			"token":  token,
			"user":   req.Username,
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
		jsonResponse(w, 200, map[string]string{"status": "authenticated", "user": auth.GlobalAuth.Config.AdminUser})
		return
	}
	jsonResponse(w, 401, map[string]string{"status": "unauthenticated"})
}

func handleHostStats(w http.ResponseWriter, r *http.Request) {
	stats, err := system.GetHostStats()
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
	}
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

func handleSecurityAudit(w http.ResponseWriter, r *http.Request) {
	report, err := system.PerformSecurityAudit()
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": err.Error()})
		return
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
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "missing path", 400)
		return
	}
	http.ServeFile(w, r, path)
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

	dir := r.FormValue("dir")
	if dir == "" {
		dir = "/"
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		jsonResponse(w, 400, map[string]string{"error": err.Error()})
		return
	}
	defer file.Close()

	destPath := filepath.Join(dir, header.Filename)
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
