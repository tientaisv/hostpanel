package metrics

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"dockpulse/pkg/docker"
	"dockpulse/pkg/system"
)

type MetricRecord struct {
	RecordedAt        string  `json:"recorded_at"`
	HostCPUPercent    float64 `json:"host_cpu_percent"`
	HostRAMUsedMB     int     `json:"host_ram_used_mb"`
	HostRAMTotalMB    int     `json:"host_ram_total_mb"`
	HostNetRxRateKB   float64 `json:"host_net_rx_rate_kb"`
	HostNetTxRateKB   float64 `json:"host_net_tx_rate_kb"`
	DockerCPUPercent  float64 `json:"docker_cpu_percent"`
	DockerRAMUsedMB   int     `json:"docker_ram_used_mb"`
	DockerRunningCtrs int     `json:"docker_running_ctrs"`
	DockerNetRxMB     float64 `json:"docker_net_rx_mb"`
	DockerNetTxMB     float64 `json:"docker_net_tx_mb"`
}

type MetricsLogger struct {
	mu           sync.Mutex
	supabaseURL  string
	supabaseKey  string
	pushInterval time.Duration
	dockerClient *docker.Client
	buffer       []MetricRecord
	recentBuffer []MetricRecord // In-memory fallback (max 1000 items)
	httpClient   *http.Client
}

var GlobalLogger *MetricsLogger

func InitLogger(dockerClient *docker.Client) {
	l := &MetricsLogger{
		dockerClient: dockerClient,
		buffer:       make([]MetricRecord, 0),
		recentBuffer: make([]MetricRecord, 0),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		pushInterval: 5 * time.Minute,
	}

	// Read config from .env files or environment
	l.loadEnvConfig()

	GlobalLogger = l

	if l.supabaseURL != "" && l.supabaseKey != "" {
		log.Printf("📊 Supabase Metrics Logging Enabled -> Destination: %s (Push Interval: %v)\n", l.supabaseURL, l.pushInterval)
	} else {
		log.Println("⚠️ Supabase Credentials not set in .env. Historical metrics will be stored in-memory fallback.")
	}

	// Start background worker loop
	go l.startWorker()
}

func (l *MetricsLogger) loadEnvConfig() {
	envFiles := []string{".env", "/root/hostcontrol/.env"}
	for _, envFile := range envFiles {
		file, err := os.Open(envFile)
		if err == nil {
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
					continue
				}
				parts := strings.SplitN(line, "=", 2)
				k := strings.TrimSpace(parts[0])
				v := strings.TrimSpace(parts[1])
				switch k {
				case "SUPABASE_URL":
					if v != "" {
						l.supabaseURL = strings.TrimRight(v, "/")
					}
				case "SUPABASE_KEY":
					if v != "" {
						l.supabaseKey = v
					}
				case "METRICS_PUSH_INTERVAL_SEC":
					if sec, errSec := strconv.Atoi(v); errSec == nil && sec > 0 {
						l.pushInterval = time.Duration(sec) * time.Second
					}
				}
			}
			file.Close()
		}
	}

	// Override with process env if set
	if url := os.Getenv("SUPABASE_URL"); url != "" {
		l.supabaseURL = strings.TrimRight(url, "/")
	}
	if key := os.Getenv("SUPABASE_KEY"); key != "" {
		l.supabaseKey = key
	}
}

func (l *MetricsLogger) startWorker() {
	collectTicker := time.NewTicker(1 * time.Minute)
	pushTicker := time.NewTicker(l.pushInterval)

	defer collectTicker.Stop()
	defer pushTicker.Stop()

	// Initial snapshot & immediate test flush after 3s startup delay
	time.Sleep(3 * time.Second)
	l.collectSnapshot()
	l.flushToSupabase()

	for {
		select {
		case <-collectTicker.C:
			l.collectSnapshot()
		case <-pushTicker.C:
			l.flushToSupabase()
		}
	}
}

func (l *MetricsLogger) collectSnapshot() {
	hostStats, errHost := system.GetHostStats()
	if errHost != nil {
		return
	}

	record := MetricRecord{
		RecordedAt:      time.Now().UTC().Format(time.RFC3339),
		HostCPUPercent:  hostStats.CPUPercent,
		HostRAMUsedMB:   int(hostStats.MemUsedMB),
		HostRAMTotalMB:  int(hostStats.MemTotalMB),
		HostNetRxRateKB: hostStats.NetRxRateKB,
		HostNetTxRateKB: hostStats.NetTxRateKB,
	}

	if l.dockerClient != nil {
		summary, errSum := l.dockerClient.GetTotalDockerStats(hostStats.MemTotalMB)
		if errSum == nil && summary != nil {
			record.DockerCPUPercent = summary.CPUPercent
			record.DockerRAMUsedMB = int(summary.MemUsedMB)
			record.DockerRunningCtrs = summary.RunningContainers
			record.DockerNetRxMB = summary.NetRxMB
			record.DockerNetTxMB = summary.NetTxMB
		}
	}

	l.mu.Lock()
	l.buffer = append(l.buffer, record)

	// Keep recent in-memory buffer up to 1440 items (24 hours at 1m rate)
	l.recentBuffer = append(l.recentBuffer, record)
	if len(l.recentBuffer) > 1440 {
		l.recentBuffer = l.recentBuffer[len(l.recentBuffer)-1440:]
	}
	l.mu.Unlock()
}

func (l *MetricsLogger) flushToSupabase() {
	l.mu.Lock()
	if len(l.buffer) == 0 {
		l.mu.Unlock()
		return
	}

	toPush := make([]MetricRecord, len(l.buffer))
	copy(toPush, l.buffer)
	l.buffer = make([]MetricRecord, 0)
	l.mu.Unlock()

	if l.supabaseURL == "" || l.supabaseKey == "" {
		return
	}

	payload, err := json.Marshal(toPush)
	if err != nil {
		return
	}

	endpoint := l.supabaseURL + "/rest/v1/resource_metrics"
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(payload))
	if err != nil {
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", l.supabaseKey)
	req.Header.Set("Authorization", "Bearer "+l.supabaseKey)
	req.Header.Set("Prefer", "return=minimal")

	resp, err := l.httpClient.Do(req)
	if err != nil {
		log.Printf("⚠️ Failed to push metrics to Supabase: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 || resp.StatusCode == 201 {
		log.Printf("✅ Successfully pushed %d metric records to Supabase!\n", len(toPush))
	} else {
		body, _ := ioutil.ReadAll(resp.Body)
		log.Printf("⚠️ Supabase metrics push HTTP %d: %s\n", resp.StatusCode, string(body))
	}
}

func (l *MetricsLogger) FetchHistory(timeRange string) ([]MetricRecord, error) {
	now := time.Now().UTC()
	var since time.Time

	switch timeRange {
	case "30d", "month":
		since = now.AddDate(0, 0, -30)
	case "7d", "week":
		since = now.AddDate(0, 0, -7)
	default: // 24h / day
		since = now.Add(-24 * time.Hour)
	}

	// Try fetching from Supabase if configured
	if l.supabaseURL != "" && l.supabaseKey != "" {
		sinceISO := since.Format(time.RFC3339)
		endpoint := fmt.Sprintf("%s/rest/v1/resource_metrics?recorded_at=gt.%s&order=recorded_at.asc&limit=2000", l.supabaseURL, sinceISO)

		req, err := http.NewRequest("GET", endpoint, nil)
		if err == nil {
			req.Header.Set("apikey", l.supabaseKey)
			req.Header.Set("Authorization", "Bearer "+l.supabaseKey)

			resp, errDo := l.httpClient.Do(req)
			if errDo == nil && resp.StatusCode == 200 {
				defer resp.Body.Close()
				body, errRead := ioutil.ReadAll(resp.Body)
				if errRead == nil {
					var records []MetricRecord
					if errJson := json.Unmarshal(body, &records); errJson == nil && len(records) > 0 {
						return records, nil
					}
				}
			}
		}
	}

	// Fallback to recent in-memory buffer
	l.mu.Lock()
	defer l.mu.Unlock()

	result := make([]MetricRecord, 0)
	for _, rec := range l.recentBuffer {
		if t, err := time.Parse(time.RFC3339, rec.RecordedAt); err == nil && t.After(since) {
			result = append(result, rec)
		}
	}

	return result, nil
}
