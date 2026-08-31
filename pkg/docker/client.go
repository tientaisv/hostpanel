package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type EngineInfo struct {
	Name          string `json:"name"`
	Version       string `json:"version"`
	APIVersion    string `json:"api_version"`
	MinAPIVersion string `json:"min_api_version"`
	IsPodman      bool   `json:"is_podman"`
	SocketPath    string `json:"socket_path"`
}

type SingleEngineClient struct {
	httpClient *http.Client
	socketPath string
	engineInfo EngineInfo
	mu         sync.RWMutex
}

type Client struct {
	primary    *SingleEngineClient
	secondaries []*SingleEngineClient
	allClients []*SingleEngineClient
	mu         sync.RWMutex
}

func newSingleClient(socketPath string) (*SingleEngineClient, error) {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return net.DialTimeout("unix", socketPath, 3*time.Second)
		},
	}

	c := &SingleEngineClient{
		socketPath: socketPath,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   5 * time.Second,
		},
		engineInfo: EngineInfo{
			Name:       "Container Engine",
			SocketPath: socketPath,
		},
	}

	// Probe
	body, code, err := c.Get("/version")
	if err != nil || (code != 200 && code != 204) {
		return nil, fmt.Errorf("socket %s not responding: %v", socketPath, err)
	}

	var raw struct {
		Version       string `json:"Version"`
		APIVersion    string `json:"ApiVersion"`
		MinAPIVersion string `json:"MinAPIVersion"`
		Components    []struct {
			Name    string `json:"Name"`
			Version string `json:"Version"`
		} `json:"Components"`
	}

	c.engineInfo.Version = "unknown"
	if jsonErr := json.Unmarshal(body, &raw); jsonErr == nil {
		c.engineInfo.Version = raw.Version
		c.engineInfo.APIVersion = raw.APIVersion
		c.engineInfo.MinAPIVersion = raw.MinAPIVersion
	}

	isPodman := false
	engineName := "Docker"
	for _, comp := range raw.Components {
		if strings.Contains(strings.ToLower(comp.Name), "podman") || strings.Contains(strings.ToLower(comp.Name), "libpod") {
			isPodman = true
			engineName = "Podman"
			break
		}
	}
	if strings.Contains(strings.ToLower(string(body)), "podman") || strings.Contains(strings.ToLower(string(body)), "libpod") || strings.Contains(socketPath, "podman") {
		isPodman = true
		engineName = "Podman"
	}

	c.engineInfo.IsPodman = isPodman
	c.engineInfo.Name = engineName

	return c, nil
}

func NewClient(socketPath string) *Client {
	var activeClients []*SingleEngineClient

	if socketPath != "" {
		if sc, err := newSingleClient(socketPath); err == nil {
			activeClients = append(activeClients, sc)
		}
	} else {
		// Probe all candidate sockets
		candidates := []string{
			"/run/podman/podman.sock",
			"/var/run/podman/podman.sock",
		}
		if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" {
			candidates = append(candidates, runtimeDir+"/podman/podman.sock")
		}
		uid := os.Getuid()
		candidates = append(candidates, fmt.Sprintf("/run/user/%d/podman/podman.sock", uid))
		candidates = append(candidates, "/run/docker.sock", "/var/run/docker.sock")

		seenSockets := make(map[string]bool)

		for _, path := range candidates {
			realPath, errEval := filepath.EvalSymlinks(path)
			if errEval != nil {
				realPath = path
			}
			if seenSockets[realPath] {
				continue
			}

			if fi, err := os.Stat(realPath); err == nil {
				if fi.Mode()&os.ModeSocket != 0 || fi.Mode().IsRegular() || !fi.IsDir() {
					if sc, errNew := newSingleClient(realPath); errNew == nil {
						seenSockets[realPath] = true
						seenSockets[path] = true
						activeClients = append(activeClients, sc)
					}
				}
			}
		}
	}

	if len(activeClients) == 0 {
		// Fallback dummy
		fallback, _ := newSingleClient("/run/podman/podman.sock")
		if fallback == nil {
			fallback = &SingleEngineClient{
				socketPath: "/run/podman/podman.sock",
				httpClient: &http.Client{},
				engineInfo: EngineInfo{Name: "Podman", SocketPath: "/run/podman/podman.sock", IsPodman: true},
			}
		}
		activeClients = append(activeClients, fallback)
	}

	// Sort primary (Podman first, or first available)
	primary := activeClients[0]
	var secondaries []*SingleEngineClient
	for _, sc := range activeClients {
		if sc.engineInfo.IsPodman {
			primary = sc
			break
		}
	}
	for _, sc := range activeClients {
		if sc != primary {
			secondaries = append(secondaries, sc)
		}
	}

	return &Client{
		primary:     primary,
		secondaries: secondaries,
		allClients:  activeClients,
	}
}

func (c *Client) GetAllClients() []*SingleEngineClient {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.allClients
}

func (c *Client) GetPrimaryClient() *SingleEngineClient {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.primary
}

func (c *Client) GetEngineInfo() EngineInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()

	info := c.primary.engineInfo
	if len(c.allClients) > 1 {
		var names []string
		for _, sc := range c.allClients {
			names = append(names, fmt.Sprintf("%s v%s", sc.engineInfo.Name, sc.engineInfo.Version))
		}
		info.Name = strings.Join(names, " + ")
	}
	return info
}

func (c *Client) IsPodman() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, sc := range c.allClients {
		if sc.engineInfo.IsPodman {
			return true
		}
	}
	return false
}

func (c *Client) GetSocketPath() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.primary.socketPath
}

func (c *Client) Get(path string) ([]byte, int, error) {
	return c.primary.Get(path)
}

func (c *Client) Post(path string, body []byte) ([]byte, int, error) {
	return c.primary.Post(path, body)
}

func (c *Client) Delete(path string) ([]byte, int, error) {
	return c.primary.Delete(path)
}

func (c *Client) RawConn() (net.Conn, error) {
	return c.primary.RawConn()
}

func normalizePath(path string) string {
	if strings.HasPrefix(path, "/v1.") || strings.HasPrefix(path, "/v2.") || strings.HasPrefix(path, "/v3.") || strings.HasPrefix(path, "/v4.") || strings.HasPrefix(path, "/_ping") {
		return path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return "/v1.40" + path
}

func (sc *SingleEngineClient) Get(path string) ([]byte, int, error) {
	req, err := http.NewRequest("GET", "http://localhost"+normalizePath(path), nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := sc.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	return body, resp.StatusCode, err
}

func (sc *SingleEngineClient) Post(path string, body []byte) ([]byte, int, error) {
	var reqBody io.Reader
	if len(body) > 0 {
		reqBody = bytes.NewReader(body)
	}
	req, err := http.NewRequest("POST", "http://localhost"+normalizePath(path), reqBody)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := sc.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	resBody, err := ioutil.ReadAll(resp.Body)
	return resBody, resp.StatusCode, err
}

func (sc *SingleEngineClient) Delete(path string) ([]byte, int, error) {
	req, err := http.NewRequest("DELETE", "http://localhost"+normalizePath(path), nil)
	if err != nil {
		return nil, 0, err
	}

	resp, err := sc.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	resBody, err := ioutil.ReadAll(resp.Body)
	return resBody, resp.StatusCode, err
}

func (sc *SingleEngineClient) RawConn() (net.Conn, error) {
	return net.Dial("unix", sc.socketPath)
}

func (sc *SingleEngineClient) Engine() string {
	if sc.engineInfo.IsPodman {
		return "podman"
	}
	return "docker"
}
