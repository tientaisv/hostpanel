package system

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type ScanTargetType string

const (
	ScanTargetQuick  ScanTargetType = "QUICK"
	ScanTargetCustom ScanTargetType = "CUSTOM"
	ScanTargetFull   ScanTargetType = "FULL"
)

type ThreatSeverity string

const (
	SeverityCritical ThreatSeverity = "CRITICAL"
	SeverityHigh     ThreatSeverity = "HIGH"
	SeverityMedium   ThreatSeverity = "MEDIUM"
	SeverityLow      ThreatSeverity = "LOW"
)

type MalwareThreat struct {
	ID             string         `json:"id"`
	FilePath       string         `json:"file_path"`
	FileName       string         `json:"file_name"`
	FileSize       int64          `json:"file_size"`
	Severity       ThreatSeverity `json:"severity"`
	Category       string         `json:"category"`
	Description    string         `json:"description"`
	MatchedPattern string         `json:"matched_pattern"`
	LineNumber     int            `json:"line_number"`
	Snippet        string         `json:"snippet"`
	DetectedAt     string         `json:"detected_at"`
	Status         string         `json:"status"` // "DETECTED", "QUARANTINED", "DELETED"
}

type ScanReport struct {
	ScanID             string         `json:"scan_id"`
	TargetType         ScanTargetType `json:"target_type"`
	ScannedPaths       []string       `json:"scanned_paths"`
	TotalFilesScanned  int            `json:"total_files_scanned"`
	TotalDirsScanned   int            `json:"total_dirs_scanned"`
	ThreatsFoundCount  int            `json:"threats_found_count"`
	Threats            []MalwareThreat `json:"threats"`
	ScanDurationSec    float64        `json:"scan_duration_sec"`
	StartedAt          string         `json:"started_at"`
	FinishedAt         string         `json:"finished_at,omitempty"`
	IsClamAVAvailable  bool           `json:"is_clamav_available"`
	IsClamAVRunning    bool           `json:"is_clamav_running"`
	IsScanning         bool           `json:"is_scanning"`
	CurrentScanningDir string         `json:"current_scanning_dir,omitempty"`
	Status             string         `json:"status"` // "IDLE", "RUNNING", "COMPLETED", "ABORTED", "FAILED"
	ErrorMessage       string         `json:"error_message,omitempty"`
}

type patternDef struct {
	category    string
	severity    ThreatSeverity
	description string
	regex       *regexp.Regexp
	fileExts    []string // empty means all text files
	pathFilter  string   // if specified, path must contain this substring
}

type ScannerManager struct {
	mu             sync.Mutex
	latestReport   ScanReport
	cancelFunc     context.CancelFunc
	patterns       []patternDef
	quarantineDir  string
	reportHistory  []ScanReport
	historyFile    string
}

var (
	GlobalScannerManager *ScannerManager
	scannerOnce          sync.Once
)

const defaultQuarantineDir = "/root/.dockpulse_quarantine"
const defaultScannerHistoryFile = "scanner_history.json"

// InitScannerManager creates the singleton ScannerManager
func InitScannerManager() *ScannerManager {
	scannerOnce.Do(func() {
		_ = os.MkdirAll(defaultQuarantineDir, 0700)

		GlobalScannerManager = &ScannerManager{
			quarantineDir: defaultQuarantineDir,
			historyFile:   defaultScannerHistoryFile,
			patterns:      buildDetectionPatterns(),
			latestReport: ScanReport{
				Status:            "IDLE",
				Threats:           make([]MalwareThreat, 0),
				ScannedPaths:      make([]string, 0),
				IsClamAVAvailable: checkClamAVAvailable(),
			},
		}

		// Load historical report if available
		if data, err := ioutil.ReadFile(defaultScannerHistoryFile); err == nil {
			var loaded ScanReport
			if err := json.Unmarshal(data, &loaded); err == nil {
				GlobalScannerManager.latestReport = loaded
				GlobalScannerManager.latestReport.IsScanning = false
				GlobalScannerManager.latestReport.IsClamAVAvailable = checkClamAVAvailable()
			}
		}
	})

	return GlobalScannerManager
}

func checkClamAVAvailable() bool {
	_, err := exec.LookPath("clamscan")
	return err == nil
}

func buildDetectionPatterns() []patternDef {
	defs := []struct {
		cat        string
		sev        ThreatSeverity
		desc       string
		expr       string
		exts       []string
		pathFilter string
	}{
		// --- WEBSHELL PATTERNS ---
		{
			cat:   "PHP Webshell",
			sev:   SeverityCritical,
			desc:  "Obfuscated execution: eval(base64_decode/gzinflate)",
			expr:  `(?i)eval\s*\(\s*(?:base64_decode|gzinflate|gzuncompress|str_rot13|hex2bin)\s*\(`,
			exts:  []string{".php", ".phtml", ".php5", ".php7", ".inc", ".module"},
		},
		{
			cat:   "PHP Webshell",
			sev:   SeverityCritical,
			desc:  "Direct remote command execution via passthru/shell_exec/system with input",
			expr:  `(?i)(?:passthru|shell_exec|system|exec|popen|proc_open)\s*\(\s*\$_(?:POST|GET|REQUEST|COOKIE|SERVER)\s*\[`,
			exts:  []string{".php", ".phtml", ".php5", ".php7", ".inc"},
		},
		{
			cat:   "PHP Webshell",
			sev:   SeverityCritical,
			desc:  "Assert dynamic execution via user input payload",
			expr:  `(?i)assert\s*\(\s*\$_(?:POST|GET|REQUEST|COOKIE)\s*\[`,
			exts:  []string{".php", ".phtml", ".php5", ".php7", ".inc"},
		},
		{
			cat:   "PHP Webshell",
			sev:   SeverityHigh,
			desc:  "Variable function invocation from request variables",
			expr:  `(?i)\$_(?:POST|GET|REQUEST|COOKIE)\[[^\]]+\]\s*\(\s*\$_(?:POST|GET|REQUEST|COOKIE)`,
			exts:  []string{".php", ".phtml", ".php5", ".php7"},
		},
		{
			cat:   "Known Webshell Signature",
			sev:   SeverityCritical,
			desc:  "Classic Webshell signature (c99, r57, b374k, WSO, Weevely, FilesMan)",
			expr:  `(?i)(?:c99shell|r57shell|b374k|weevely|FilesMan|WSOset|China\s+Chopper|alfa-team|cyber-warrior|indoxploit|madspot)`,
			exts:  nil,
		},

		// --- REVERSE SHELLS & BACKDOORS ---
		{
			cat:   "Reverse Shell",
			sev:   SeverityCritical,
			desc:  "Bash TCP Socket Reverse Shell (/bin/bash -i >& /dev/tcp)",
			expr:  `(?i)(?:/bin/bash|/bin/sh|bash|sh)\s+-i\s+>&?\s+/dev/tcp/\d+\.\d+\.\d+\.\d+/\d+`,
			exts:  nil,
		},
		{
			cat:   "Reverse Shell",
			sev:   SeverityCritical,
			desc:  "Netcat Reverse Shell with interactive binary execution (-e /bin/sh)",
			expr:  `(?i)(?:nc|netcat|ncat)\s+(?:-[a-zA-Z]*e\s+|.*-e\s+)(?:/bin/sh|/bin/bash|sh|bash)`,
			exts:  nil,
		},
		{
			cat:   "Reverse Shell",
			sev:   SeverityCritical,
			desc:  "Python PTY / Socket interactive reverse shell payload",
			expr:  `(?i)import\s+socket.*import\s+(?:os|subprocess|pty).*pty\.spawn\(["']/bin/(?:ba)?sh["']\)`,
			exts:  []string{".py", ".sh", ".pl", ".php"},
		},
		{
			cat:   "Reverse Shell",
			sev:   SeverityHigh,
			desc:  "FIFO Pipe interactive shell redirection",
			expr:  `(?i)mkfifo\s+/tmp/\w+;\s*cat\s+/tmp/\w+\s*\|\s*(?:/bin/)?(?:ba)?sh\s+-i`,
			exts:  nil,
		},

		// --- CRON & PERSISTENCE MALWARE ---
		{
			cat:        "Malicious Cronjob",
			sev:        SeverityCritical,
			desc:       "Cronjob downloading and piping directly into shell execution (curl/wget | sh)",
			expr:       `(?i)(?:curl\s+-[sSkKfF]*L*s*|wget\s+-[qQO-]*)\s+https?://[^\s]+\s*\|\s*(?:/bin/)?(?:ba)?sh`,
			exts:       nil,
			pathFilter: "cron",
		},
		{
			cat:        "Suspicious Cron Entry",
			sev:        SeverityHigh,
			desc:       "Cron entry executing binaries directly from temporary directories (/tmp, /dev/shm)",
			expr:       `(?i)(?:/tmp/|/dev/shm/|/var/tmp/)[a-zA-Z0-9_.-]+\s*(?:&|;|\||\n|$)`,
			exts:       nil,
			pathFilter: "cron",
		},

		// --- CRYPTO MINING SCRIPTS ---
		{
			cat:   "Crypto Miner Config",
			sev:   SeverityCritical,
			desc:  "Mining pool connection stratum protocol (Monero / XMRig / C3Pool)",
			expr:  `(?i)(?:stratum\+(?:tcp|ssl)://|pool\.supportxmr\.com|minexmr\.com|c3pool\.com|moneroocean\.stream|xmr\.nanopool\.org)`,
			exts:  nil,
		},
		{
			cat:   "Crypto Miner Config",
			sev:   SeverityHigh,
			desc:  "XMRig miner configuration payload (algo rx/0)",
			expr:  `(?i)"algo"\s*:\s*"rx/0"|"pools"\s*:\s*\[\s*\{\s*"url"\s*:`,
			exts:  []string{".json", ".conf", ".cfg", ".txt"},
		},

		// --- SUSPICIOUS DOWNLOADERS ---
		{
			cat:   "Suspicious Downloader",
			sev:   SeverityHigh,
			desc:  "Base64 decoded command execution piped to bash",
			expr:  `(?i)echo\s+["'][A-Za-z0-9+/=]{20,}["']\s*\|\s*base64\s+-(?:d|decode)\s*\|\s*(?:/bin/)?(?:ba)?sh`,
			exts:  nil,
		},
	}

	patterns := make([]patternDef, 0, len(defs))
	for _, d := range defs {
		re, err := regexp.Compile(d.expr)
		if err == nil {
			patterns = append(patterns, patternDef{
				category:    d.cat,
				severity:    d.sev,
				description: d.desc,
				regex:       re,
				fileExts:    d.exts,
				pathFilter:  d.pathFilter,
			})
		}
	}
	return patterns
}

// GetStatus returns the current status and latest scan report
func (sm *ScannerManager) GetStatus() ScanReport {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	r := sm.latestReport
	r.IsClamAVAvailable = checkClamAVAvailable()
	return r
}

// StartScan launches an asynchronous scan task
func (sm *ScannerManager) StartScan(targetType ScanTargetType, customPath string, useClamAV bool) (ScanReport, error) {
	sm.mu.Lock()
	if sm.latestReport.IsScanning {
		r := sm.latestReport
		sm.mu.Unlock()
		return r, fmt.Errorf("một phiên quét khác đang diễn ra")
	}

	ctx, cancel := context.WithCancel(context.Background())
	sm.cancelFunc = cancel

	var targetPaths []string
	switch targetType {
	case ScanTargetCustom:
		cleanPath := filepath.Clean(strings.TrimSpace(customPath))
		if cleanPath == "" || cleanPath == "." {
			cleanPath = "/var/www"
		}
		if _, err := os.Stat(cleanPath); err != nil {
			sm.mu.Unlock()
			return ScanReport{}, fmt.Errorf("thư mục không tồn tại: %s", cleanPath)
		}
		targetPaths = []string{cleanPath}

	case ScanTargetFull:
		targetPaths = []string{"/etc", "/var", "/tmp", "/dev/shm", "/root", "/home", "/usr/local", "/opt"}

	default: // ScanTargetQuick
		targetType = ScanTargetQuick
		targetPaths = []string{
			"/tmp",
			"/dev/shm",
			"/var/tmp",
			"/etc/cron.d",
			"/etc/cron.daily",
			"/etc/cron.hourly",
			"/etc/cron.weekly",
			"/etc/cron.monthly",
			"/etc/crontab",
			"/var/spool/cron",
			"/var/www",
			"/root",
		}
	}

	report := ScanReport{
		ScanID:             fmt.Sprintf("scan_%d", time.Now().Unix()),
		TargetType:         targetType,
		ScannedPaths:       targetPaths,
		TotalFilesScanned:  0,
		TotalDirsScanned:   0,
		ThreatsFoundCount:  0,
		Threats:            make([]MalwareThreat, 0),
		StartedAt:          time.Now().Format("15:04:05 02/01/2006"),
		IsScanning:         true,
		Status:             "RUNNING",
		IsClamAVAvailable:  checkClamAVAvailable(),
	}

	sm.latestReport = report
	sm.mu.Unlock()

	go sm.executeScan(ctx, targetPaths, useClamAV)

	return report, nil
}

// AbortScan stops any running scan
func (sm *ScannerManager) AbortScan() (ScanReport, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if !sm.latestReport.IsScanning {
		return sm.latestReport, fmt.Errorf("không có phiên quét nào đang chạy")
	}

	if sm.cancelFunc != nil {
		sm.cancelFunc()
		sm.cancelFunc = nil
	}

	sm.latestReport.IsScanning = false
	sm.latestReport.Status = "ABORTED"
	sm.latestReport.FinishedAt = time.Now().Format("15:04:05 02/01/2006")
	sm.latestReport.CurrentScanningDir = ""
	sm.saveReportLocked()

	return sm.latestReport, nil
}

// executeScan runs file analysis in background
func (sm *ScannerManager) executeScan(ctx context.Context, paths []string, useClamAV bool) {
	startTime := time.Now()

	totalFiles := 0
	totalDirs := 0
	threats := make([]MalwareThreat, 0)
	seenThreats := make(map[string]bool)

	// Skip patterns for noisy / special mount filesystems
	skipDirs := map[string]bool{
		"/proc":       true,
		"/sys":        true,
		"/dev":        true,
		"/run/user":   true,
		"/var/lib/docker/overlay2": true,
		"/var/lib/containers/storage/overlay": true,
		defaultQuarantineDir: true,
	}

	for _, basePath := range paths {
		select {
		case <-ctx.Done():
			sm.mu.Lock()
			sm.latestReport.IsScanning = false
			sm.latestReport.Status = "ABORTED"
			sm.latestReport.FinishedAt = time.Now().Format("15:04:05 02/01/2006")
			sm.latestReport.ScanDurationSec = time.Since(startTime).Seconds()
			sm.saveReportLocked()
			sm.mu.Unlock()
			return
		default:
		}

		info, err := os.Stat(basePath)
		if err != nil {
			continue
		}

		if !info.IsDir() {
			// Single file check
			totalFiles++
			foundThreats := sm.inspectFile(basePath, info)
			for _, t := range foundThreats {
				if !seenThreats[t.FilePath+t.Category] {
					seenThreats[t.FilePath+t.Category] = true
					threats = append(threats, t)
				}
			}
			continue
		}

		// Walk Directory
		_ = filepath.Walk(basePath, func(path string, fileInfo os.FileInfo, walkErr error) error {
			select {
			case <-ctx.Done():
				return fmt.Errorf("aborted")
			default:
			}

			if walkErr != nil {
				return nil
			}

			// Check skip directory
			if fileInfo.IsDir() {
				totalDirs++
				dirName := fileInfo.Name()
				cleanPath := filepath.Clean(path)

				// In Quick Scan mode on /root, only scan top-level root directory files and don't recurse into sub-projects
				if sm.latestReport.TargetType == ScanTargetQuick && basePath == "/root" && cleanPath != "/root" {
					return filepath.SkipDir
				}

				// Fast skip hidden config dirs, database volume mounts, build caches, and node_modules
				if strings.HasPrefix(dirName, ".") || dirName == "node_modules" || dirName == "vendor" || dirName == "snap" || dirName == "volumes" || dirName == "pgdata" || strings.HasPrefix(dirName, "go-build") || strings.HasPrefix(dirName, "codex-bwrap") || strings.HasPrefix(dirName, "tapp-") || strings.HasPrefix(dirName, "claude-") || strings.Contains(dirName, "cache") || strings.Contains(dirName, "gocache") {
					return filepath.SkipDir
				}

				for skip := range skipDirs {
					if cleanPath == skip || strings.HasPrefix(cleanPath, skip+"/") {
						return filepath.SkipDir
					}
				}

				sm.mu.Lock()
				sm.latestReport.CurrentScanningDir = cleanPath
				sm.latestReport.TotalFilesScanned = totalFiles
				sm.latestReport.TotalDirsScanned = totalDirs
				sm.latestReport.ThreatsFoundCount = len(threats)
				sm.mu.Unlock()
				return nil
			}

			totalFiles++

			// Inspect single file
			foundThreats := sm.inspectFile(path, fileInfo)
			for _, t := range foundThreats {
				if !seenThreats[t.FilePath+t.Category] {
					seenThreats[t.FilePath+t.Category] = true
					threats = append(threats, t)
				}
			}

			// Periodically update report counters
			if totalFiles%100 == 0 {
				sm.mu.Lock()
				sm.latestReport.TotalFilesScanned = totalFiles
				sm.latestReport.TotalDirsScanned = totalDirs
				sm.latestReport.ThreatsFoundCount = len(threats)
				sm.mu.Unlock()
			}

			return nil
		})
	}

	// Run ClamAV if requested and installed
	if useClamAV && checkClamAVAvailable() {
		for _, p := range paths {
			clamThreats := sm.runClamAVScan(ctx, p)
			for _, t := range clamThreats {
				if !seenThreats[t.FilePath+t.Category] {
					seenThreats[t.FilePath+t.Category] = true
					threats = append(threats, t)
				}
			}
		}
	}

	duration := time.Since(startTime).Seconds()

	sm.mu.Lock()
	sm.latestReport.IsScanning = false
	sm.latestReport.Status = "COMPLETED"
	sm.latestReport.FinishedAt = time.Now().Format("15:04:05 02/01/2006")
	sm.latestReport.ScanDurationSec = duration
	sm.latestReport.TotalFilesScanned = totalFiles
	sm.latestReport.TotalDirsScanned = totalDirs
	sm.latestReport.ThreatsFoundCount = len(threats)
	sm.latestReport.Threats = threats
	sm.latestReport.CurrentScanningDir = ""
	sm.saveReportLocked()
	sm.mu.Unlock()

	log.Printf("🛡️ [Security Scanner] Scan finished in %.2fs. Total files: %d, Threats found: %d",
		duration, totalFiles, len(threats))
}

// inspectFile analyzes a single file for malware patterns
func (sm *ScannerManager) inspectFile(path string, info os.FileInfo) []MalwareThreat {
	var threats []MalwareThreat

	// Rule 1: Suspicious ELF Executable in Temporary Dirs
	isTempDir := strings.HasPrefix(path, "/tmp/") || strings.HasPrefix(path, "/dev/shm/") || strings.HasPrefix(path, "/var/tmp/")
	if isTempDir && info.Mode()&0111 != 0 && info.Size() > 0 {
		// Read first 4 bytes for ELF header
		header := make([]byte, 4)
		if f, err := os.Open(path); err == nil {
			_, _ = f.Read(header)
			f.Close()
			if bytes.Equal(header, []byte{0x7f, 'E', 'L', 'F'}) {
				threats = append(threats, MalwareThreat{
					ID:             generateThreatID(path, "ELF Executable in Temp"),
					FilePath:       path,
					FileName:       info.Name(),
					FileSize:       info.Size(),
					Severity:       SeverityCritical,
					Category:       "Suspicious Executable",
					Description:    "Binary file thực thi (ELF) được phát hiện nằm trong thư mục tạm (/tmp hoặc /dev/shm)",
					MatchedPattern: "ELF Header [7F 45 4C 46]",
					LineNumber:     1,
					Snippet:        "ELF 64-bit/32-bit executable binary in temp directory",
					DetectedAt:     time.Now().Format(time.RFC3339),
					Status:         "DETECTED",
				})
			}
		}
	}

	ext := strings.ToLower(filepath.Ext(path))

	// Skip known non-executable static media / archive formats
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".ico", ".svg", ".webp", ".mp4", ".mp3", ".pdf",
		".zip", ".tar", ".gz", ".bz2", ".xz", ".7z", ".iso", ".so", ".a", ".o", ".dylib", ".dll",
		".wasm", ".db", ".sqlite", ".sqlite3", ".parquet", ".arrow", ".woff", ".woff2", ".ttf", ".eot":
		return threats
	}

	// Skip files larger than 5MB to avoid excessive memory / CPU usage
	if info.Size() > 5*1024*1024 || info.Size() == 0 {
		return threats
	}

	// Fast binary check: read first 512 bytes. If it contains null bytes \x00, it's binary data
	sample := make([]byte, 512)
	fSample, errSample := os.Open(path)
	if errSample != nil {
		return threats
	}
	nSample, _ := fSample.Read(sample)
	fSample.Close()

	if bytes.IndexByte(sample[:nSample], 0) != -1 {
		// Binary file - skip text heuristic scan
		return threats
	}

	// Read file content line by line (up to 5,000 lines)
	file, err := os.Open(path)
	if err != nil {
		return threats
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// Allocate 32KB buffer for long lines
	buf := make([]byte, 32*1024)
	scanner.Buffer(buf, 256*1024)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if lineNum > 20000 {
			break
		}
		line := scanner.Text()

		for _, pat := range sm.patterns {
			// Check path filter if specified (e.g. only scan cron files for cron rules)
			if pat.pathFilter != "" && !strings.Contains(path, pat.pathFilter) {
				continue
			}

			// Check file extension filter
			if len(pat.fileExts) > 0 {
				matchedExt := false
				for _, e := range pat.fileExts {
					if ext == e {
						matchedExt = true
						break
					}
				}
				if !matchedExt {
					continue
				}
			}

			if pat.regex.MatchString(line) {
				snippet := strings.TrimSpace(line)
				if len(snippet) > 160 {
					snippet = snippet[:157] + "..."
				}

				threats = append(threats, MalwareThreat{
					ID:             generateThreatID(path, pat.category),
					FilePath:       path,
					FileName:       info.Name(),
					FileSize:       info.Size(),
					Severity:       pat.severity,
					Category:       pat.category,
					Description:    pat.description,
					MatchedPattern: pat.regex.String(),
					LineNumber:     lineNum,
					Snippet:        snippet,
					DetectedAt:     time.Now().Format(time.RFC3339),
					Status:         "DETECTED",
				})
				// Found a threat for this pattern on this file, stop scanning other lines for this pattern
				break
			}
		}
	}

	return threats
}

// runClamAVScan queries clamscan on target path
func (sm *ScannerManager) runClamAVScan(ctx context.Context, targetPath string) []MalwareThreat {
	var threats []MalwareThreat
	cmd := exec.CommandContext(ctx, "clamscan", "-r", "-i", "--no-summary", targetPath)
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		return threats
	}

	// Output format: /path/to/file: Virus.Name FOUND
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasSuffix(line, "FOUND") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				fPath := strings.TrimSpace(parts[0])
				virusName := strings.TrimSpace(strings.TrimSuffix(parts[1], "FOUND"))

				fInfo, _ := os.Stat(fPath)
				fSize := int64(0)
				fName := filepath.Base(fPath)
				if fInfo != nil {
					fSize = fInfo.Size()
					fName = fInfo.Name()
				}

				threats = append(threats, MalwareThreat{
					ID:             generateThreatID(fPath, "ClamAV"),
					FilePath:       fPath,
					FileName:       fName,
					FileSize:       fSize,
					Severity:       SeverityCritical,
					Category:       "ClamAV Signature",
					Description:    fmt.Sprintf("Phát hiện bởi ClamAV Antivirus: %s", virusName),
					MatchedPattern: virusName,
					LineNumber:     1,
					Snippet:        fmt.Sprintf("ClamAV Malware Signature: %s", virusName),
					DetectedAt:     time.Now().Format(time.RFC3339),
					Status:         "DETECTED",
				})
			}
		}
	}

	return threats
}

// QuarantineThreat disables and safely moves a threat file to quarantine
func (sm *ScannerManager) QuarantineThreat(filePath string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	cleanPath := filepath.Clean(filePath)
	info, err := os.Stat(cleanPath)
	if err != nil {
		return fmt.Errorf("tệp không tồn tại: %v", err)
	}

	_ = os.MkdirAll(sm.quarantineDir, 0700)

	// Safe filename in quarantine
	safeName := fmt.Sprintf("%d_%s.quarantine", time.Now().Unix(), filepath.Base(cleanPath))
	destPath := filepath.Join(sm.quarantineDir, safeName)

	// 1. Strip all permissions first (chmod 0000)
	_ = os.Chmod(cleanPath, 0000)

	// 2. Move file
	if err := os.Rename(cleanPath, destPath); err != nil {
		// Fallback: Copy and delete
		input, errRead := ioutil.ReadFile(cleanPath)
		if errRead != nil {
			return fmt.Errorf("không thể đọc file để cách ly: %v", errRead)
		}
		if errWrite := ioutil.WriteFile(destPath, input, 0000); errWrite != nil {
			return fmt.Errorf("không thể ghi file vào thư mục cách ly: %v", errWrite)
		}
		_ = os.Remove(cleanPath)
	}

	// Strip permissions on destPath to be 100% safe
	_ = os.Chmod(destPath, 0000)

	// Update in latest report
	for i, t := range sm.latestReport.Threats {
		if t.FilePath == cleanPath {
			sm.latestReport.Threats[i].Status = "QUARANTINED"
		}
	}
	sm.saveReportLocked()

	log.Printf("🔒 [Security Scanner] Quarantined threat file: %s -> %s (Size: %d bytes, Mode: 0000)",
		cleanPath, destPath, info.Size())

	return nil
}

// DeleteThreat permanently deletes a detected malware file
func (sm *ScannerManager) DeleteThreat(filePath string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	cleanPath := filepath.Clean(filePath)
	if err := os.Remove(cleanPath); err != nil {
		return fmt.Errorf("không thể xóa tệp: %v", err)
	}

	for i, t := range sm.latestReport.Threats {
		if t.FilePath == cleanPath {
			sm.latestReport.Threats[i].Status = "DELETED"
		}
	}
	sm.saveReportLocked()

	log.Printf("🗑️ [Security Scanner] Deleted malware file: %s", cleanPath)
	return nil
}

// ReadFileSnippet reads safe head content of a suspicious file
func (sm *ScannerManager) ReadFileSnippet(filePath string, maxBytes int) (string, error) {
	cleanPath := filepath.Clean(filePath)
	f, err := os.Open(cleanPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if maxBytes <= 0 || maxBytes > 64*1024 {
		maxBytes = 8 * 1024
	}

	buf := make([]byte, maxBytes)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}

	// Clean unprintable binary characters
	sanitized := strings.ToValidUTF8(string(buf[:n]), "")
	return sanitized, nil
}

// InstallClamAV initiates apt installation of ClamAV daemon
func (sm *ScannerManager) InstallClamAV() error {
	cmd := exec.Command("bash", "-c", "DEBIAN_FRONTEND=noninteractive apt-get update && apt-get install -y clamav clamav-daemon")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cài đặt ClamAV thất bại: %v (%s)", err, string(out))
	}
	return nil
}

func (sm *ScannerManager) saveReportLocked() {
	if data, err := json.MarshalIndent(sm.latestReport, "", "  "); err == nil {
		_ = ioutil.WriteFile(sm.historyFile, data, 0644)
	}
}

func generateThreatID(path, cat string) string {
	h := sha256.Sum256([]byte(path + ":" + cat))
	return hex.EncodeToString(h[:8])
}
