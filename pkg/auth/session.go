package auth

import (
	"bufio"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Port      string `json:"port"`
	AdminUser string `json:"admin_user"`
	AdminPass string `json:"admin_pass"`
}

type loginAttempt struct {
	count     int
	lockUntil time.Time
	lastTry   time.Time
}

type SessionStore struct {
	mu           sync.Mutex
	sessions     map[string]int64 // token -> expiry timestamp
	loginTracker map[string]*loginAttempt
	Config       Config
}

var GlobalAuth *SessionStore

func InitAuth(configPath string) {
	if configPath == "" {
		configPath = "config.json"
	}

	cfg := Config{
		Port:      "3800",
		AdminUser: "admin",
		AdminPass: "dockpulse2026",
	}

	// Read from .env file if available
	envFiles := []string{".env"}
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
				case "ADMIN_USER":
					if v != "" {
						cfg.AdminUser = v
					}
				case "ADMIN_PASS":
					if v != "" {
						cfg.AdminPass = v
					}
				case "PORT":
					if v != "" {
						cfg.Port = v
					}
				}
			}
			file.Close()
		}
	}

	// Read from config file
	data, err := ioutil.ReadFile(configPath)
	if err == nil {
		var fileCfg Config
		if errJson := json.Unmarshal(data, &fileCfg); errJson == nil {
			if fileCfg.AdminUser != "" {
				cfg.AdminUser = fileCfg.AdminUser
			}
			if fileCfg.AdminPass != "" {
				cfg.AdminPass = fileCfg.AdminPass
			}
			if fileCfg.Port != "" {
				cfg.Port = fileCfg.Port
			}
		}
	}

	// Environment variables override all
	if u := os.Getenv("ADMIN_USER"); u != "" {
		cfg.AdminUser = u
	}
	if p := os.Getenv("ADMIN_PASS"); p != "" {
		cfg.AdminPass = p
	}
	if port := os.Getenv("PORT"); port != "" {
		cfg.Port = port
	}

	GlobalAuth = &SessionStore{
		sessions:     make(map[string]int64),
		loginTracker: make(map[string]*loginAttempt),
		Config:       cfg,
	}
}

func (s *SessionStore) IsDefaultCredentials() bool {
	return s.Config.AdminUser == "admin" && s.Config.AdminPass == "dockpulse2026"
}

func (s *SessionStore) CheckRateLimit(ip string) (bool, int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	att, exists := s.loginTracker[ip]
	if !exists {
		return true, 0
	}

	now := time.Now()
	if now.Before(att.lockUntil) {
		remainingSec := int(att.lockUntil.Sub(now).Seconds()) + 1
		return false, remainingSec
	}

	// If last try was more than 10 minutes ago, reset
	if now.Sub(att.lastTry) > 10*time.Minute {
		delete(s.loginTracker, ip)
		return true, 0
	}

	return true, 0
}

func (s *SessionStore) RecordFailedAttempt(ip string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	att, exists := s.loginTracker[ip]
	if !exists {
		att = &loginAttempt{count: 0}
		s.loginTracker[ip] = att
	}

	att.lastTry = now
	att.count++

	if att.count >= 5 {
		// Lock for 10 minutes
		att.lockUntil = now.Add(10 * time.Minute)
		return 600
	}

	return 0
}

func (s *SessionStore) ResetAttempts(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.loginTracker, ip)
}

func (s *SessionStore) Authenticate(user, pass string) bool {
	userMatch := subtle.ConstantTimeCompare([]byte(user), []byte(s.Config.AdminUser)) == 1
	passMatch := subtle.ConstantTimeCompare([]byte(pass), []byte(s.Config.AdminPass)) == 1
	return userMatch && passMatch
}

func (s *SessionStore) CreateSession(w http.ResponseWriter, r *http.Request) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	expiry := time.Now().Add(24 * time.Hour).Unix()

	s.mu.Lock()
	s.sessions[token] = expiry
	s.mu.Unlock()

	isSecure := false
	if r != nil {
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			isSecure = true
		}
	}

	cookie := &http.Cookie{
		Name:     "dockpulse_session",
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(24 * time.Hour),
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, cookie)
	return token, nil
}

func (s *SessionStore) ClearSession(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("dockpulse_session")
	if err == nil && cookie.Value != "" {
		s.mu.Lock()
		delete(s.sessions, cookie.Value)
		s.mu.Unlock()
	}

	isSecure := false
	if r != nil {
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			isSecure = true
		}
	}

	clearCookie := &http.Cookie{
		Name:     "dockpulse_session",
		Value:    "",
		Path:     "/",
		Expires:  time.Now().Add(-1 * time.Hour),
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, clearCookie)
}

func (s *SessionStore) ValidateRequest(r *http.Request) bool {
	cookie, err := r.Cookie("dockpulse_session")
	if err != nil || cookie.Value == "" {
		// Try query parameter for WebSocket connections
		token := r.URL.Query().Get("token")
		if token == "" {
			return false
		}
		return s.ValidateToken(token)
	}
	return s.ValidateToken(cookie.Value)
}

func (s *SessionStore) ValidateToken(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	expiry, exists := s.sessions[token]
	if !exists {
		return false
	}

	if time.Now().Unix() > expiry {
		delete(s.sessions, token)
		return false
	}

	return true
}

func ExtractClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			ip := strings.TrimSpace(ips[0])
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	}
	if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		ip := strings.TrimSpace(xrip)
		if net.ParseIP(ip) != nil {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && net.ParseIP(host) != nil {
		return host
	}
	return r.RemoteAddr
}
