package docker

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"dockpulse/pkg/system"
)

type PortMapping struct {
	IP          string `json:"ip"`
	PrivatePort int    `json:"private_port"`
	PublicPort  int    `json:"public_port"`
	Type        string `json:"type"`
}

type ContainerSummary struct {
	ID      string            `json:"id"`
	ShortID string            `json:"short_id"`
	Names   []string          `json:"names"`
	Name    string            `json:"name"`
	Image   string            `json:"image"`
	ImageID string            `json:"image_id"`
	Command string            `json:"command"`
	Created int64             `json:"created"`
	State   string            `json:"state"`
	Status  string            `json:"status"`
	Ports   []PortMapping     `json:"ports"`
	IPs     map[string]string `json:"ips"` // network_name -> IP
	Labels  map[string]string `json:"labels"`
	Project string            `json:"project"` // Compose project name
	Engine  string            `json:"engine"`  // "podman" or "docker"
}

type DockerPortItem struct {
	IP          string `json:"IP"`
	PrivatePort int    `json:"PrivatePort"`
	PublicPort  int    `json:"PublicPort"`
	Type        string `json:"Type"`
}

type DockerNetworkItem struct {
	IPAddress string `json:"IPAddress"`
}

type DockerNetworkSettings struct {
	Networks map[string]DockerNetworkItem `json:"Networks"`
}

type RawContainer struct {
	ID              string                `json:"Id"`
	Names           []string              `json:"Names"`
	Image           string                `json:"Image"`
	ImageID         string                `json:"ImageID"`
	Command         string                `json:"Command"`
	Created         int64                 `json:"Created"`
	State           string                `json:"State"`
	Status          string                `json:"Status"`
	Ports           []DockerPortItem      `json:"Ports"`
	Labels          map[string]string     `json:"Labels"`
	NetworkSettings DockerNetworkSettings `json:"NetworkSettings"`
}

func (c *Client) ListContainers() ([]ContainerSummary, error) {
	var rawCandidates []ContainerSummary
	seenIDs := make(map[string]bool)

	for _, sc := range c.allClients {
		body, code, err := sc.Get("/containers/json?all=1")
		if err != nil || code != 200 {
			continue
		}

		var rawList []RawContainer
		if err := json.Unmarshal(body, &rawList); err != nil {
			continue
		}

		engineName := sc.Engine()

		for _, r := range rawList {
			if seenIDs[r.ID] {
				continue
			}
			seenIDs[r.ID] = true

			name := ""
			if len(r.Names) > 0 {
				name = r.Names[0]
				if len(name) > 0 && name[0] == '/' {
					name = name[1:]
				}
			}

			shortID := r.ID
			if len(shortID) > 12 {
				shortID = shortID[:12]
			}

			ports := make([]PortMapping, 0)
			for _, p := range r.Ports {
				ports = append(ports, PortMapping{
					IP:          p.IP,
					PrivatePort: p.PrivatePort,
					PublicPort:  p.PublicPort,
					Type:        p.Type,
				})
			}

			ips := make(map[string]string)
			for netName, netObj := range r.NetworkSettings.Networks {
				if netObj.IPAddress != "" {
					ips[netName] = netObj.IPAddress
				}
			}

			proj := ""
			if r.Labels != nil {
				if p, ok := r.Labels["com.docker.compose.project"]; ok && p != "" {
					proj = p
				} else if p, ok := r.Labels["io.podman.compose.project"]; ok && p != "" {
					proj = p
				} else if p, ok := r.Labels["io.kubernetes.pod.name"]; ok && p != "" {
					proj = p
				} else if p, ok := r.Labels["pod"]; ok && p != "" {
					proj = p
				}
			}

			rawCandidates = append(rawCandidates, ContainerSummary{
				ID:      r.ID,
				ShortID: shortID,
				Names:   r.Names,
				Name:    name,
				Image:   r.Image,
				ImageID: r.ImageID,
				Command: r.Command,
				Created: r.Created,
				State:   r.State,
				Status:  r.Status,
				Ports:   ports,
				IPs:     ips,
				Labels:  r.Labels,
				Project: proj,
				Engine:  engineName,
			})
		}
	}

	// Smart cross-engine deduplication by container name
	// When a container is restarted or migrated between engines (e.g., Docker -> Podman):
	// 1. If one is 'running' and another is 'exited'/'stopped', keep the running container and discard the stale ghost.
	// 2. If multiple are 'running', keep all running containers.
	// 3. If all are stopped, keep the newest one (highest Created timestamp).
	byName := make(map[string][]ContainerSummary)
	var unnamedList []ContainerSummary

	for _, ctr := range rawCandidates {
		cleanName := strings.TrimPrefix(ctr.Name, "/")
		if cleanName == "" {
			unnamedList = append(unnamedList, ctr)
		} else {
			byName[cleanName] = append(byName[cleanName], ctr)
		}
	}

	var result []ContainerSummary
	result = append(result, unnamedList...)

	for _, group := range byName {
		if len(group) == 1 {
			result = append(result, group[0])
			continue
		}

		var running []ContainerSummary
		for _, ctr := range group {
			if ctr.State == "running" {
				running = append(running, ctr)
			}
		}

		if len(running) == 1 {
			// One running, others stopped/exited: select the active running one
			result = append(result, running[0])
		} else if len(running) > 1 {
			// Multiple running: keep all running
			result = append(result, running...)
		} else {
			// All stopped: pick the newest one by Created timestamp
			best := group[0]
			for _, ctr := range group[1:] {
				if ctr.Created > best.Created {
					best = ctr
				}
			}
			result = append(result, best)
		}
	}

	// Sort result: Running first, then by name
	sort.Slice(result, func(i, j int) bool {
		iRunning := result[i].State == "running"
		jRunning := result[j].State == "running"
		if iRunning != jRunning {
			return iRunning
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})

	return result, nil
}

func (c *Client) ContainerAction(id, action string) error {
	path := fmt.Sprintf("/containers/%s/%s", id, action)
	sc := c.GetClientForContainer(id)
	body, code, err := sc.Post(path, nil)
	if err == nil && (code == 204 || code == 200 || code == 304) {
		return nil
	}

	// Fallback to all other clients if primary failed
	var lastErr error
	if err != nil {
		lastErr = err
	} else {
		lastErr = fmt.Errorf("action %s failed status %d: %s", action, code, string(body))
	}

	for _, client := range c.allClients {
		if client == sc {
			continue
		}
		_, cd, e := client.Post(path, nil)
		if e == nil && (cd == 204 || cd == 200 || cd == 304) {
			return nil
		}
	}
	return lastErr
}

func (c *Client) RemoveContainer(id string, force bool) error {
	path := fmt.Sprintf("/containers/%s?force=%t&v=true", id, force)
	sc := c.GetClientForContainer(id)
	body, code, err := sc.Delete(path)
	if err == nil && (code == 204 || code == 200) {
		return nil
	}

	var lastErr error
	if err != nil {
		lastErr = err
	} else {
		lastErr = fmt.Errorf("remove failed status %d: %s", code, string(body))
	}

	for _, client := range c.allClients {
		if client == sc {
			continue
		}
		_, cd, e := client.Delete(path)
		if e == nil && (cd == 204 || cd == 200) {
			return nil
		}
	}
	return lastErr
}

func (c *Client) GetClientForContainer(id string) *SingleEngineClient {
	shortID := id
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}

	var runningClient *SingleEngineClient
	var anyClient *SingleEngineClient

	for _, sc := range c.allClients {
		body, code, err := sc.Get(fmt.Sprintf("/containers/%s/json", id))
		if err == nil && code == 200 {
			var details struct {
				State struct {
					Running bool `json:"Running"`
				} `json:"State"`
			}
			if errU := json.Unmarshal(body, &details); errU == nil && details.State.Running {
				runningClient = sc
				break
			}
			if anyClient == nil {
				anyClient = sc
			}
		}
		if shortID != id {
			body, code, err := sc.Get(fmt.Sprintf("/containers/%s/json", shortID))
			if err == nil && code == 200 {
				var details struct {
					State struct {
						Running bool `json:"Running"`
					} `json:"State"`
				}
				if errU := json.Unmarshal(body, &details); errU == nil && details.State.Running {
					runningClient = sc
					break
				}
				if anyClient == nil {
					anyClient = sc
				}
			}
		}
	}

	if runningClient != nil {
		return runningClient
	}
	if anyClient != nil {
		return anyClient
	}
	return c.primary
}

func (c *Client) GetContainerStats(id string) (*system.ContainerStats, error) {
	allStats, errAll := c.GetAllContainersStats()
	if errAll == nil && allStats != nil {
		if st, ok := allStats[id]; ok && st != nil {
			return st, nil
		}
		shortID := id
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}
		if st, ok := allStats[shortID]; ok && st != nil {
			return st, nil
		}
		cleanName := strings.TrimPrefix(id, "/")
		if st, ok := allStats[cleanName]; ok && st != nil {
			return st, nil
		}
	}

	sc := c.GetClientForContainer(id)
	path := fmt.Sprintf("/containers/%s/stats?stream=false", id)
	body, code, err := sc.Get(path)
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("stats status %d: %s", code, string(body))
	}

	return system.ParseDockerStats(body)
}

type LibpodContainerStatItem struct {
	ContainerID string  `json:"ContainerID"`
	Name        string  `json:"Name"`
	CPU         float64 `json:"CPU"`
	CPUNano     uint64  `json:"CPUNano"`
	SystemNano  uint64  `json:"SystemNano"`
	MemUsage    uint64  `json:"MemUsage"`
	MemLimit    uint64  `json:"MemLimit"`
	MemPerc     float64 `json:"MemPerc"`
	NetInput    uint64  `json:"NetInput"`
	NetOutput   uint64  `json:"NetOutput"`
	BlockInput  uint64  `json:"BlockInput"`
	BlockOutput uint64  `json:"BlockOutput"`
	PIDs        int     `json:"PIDs"`
	UpTime      uint64  `json:"UpTime"`
}

type LibpodStatsResponse struct {
	Error *string                   `json:"Error"`
	Stats []LibpodContainerStatItem `json:"Stats"`
}

type podmanCPUSample struct {
	cpuNano    uint64
	systemNano uint64
	recordedAt time.Time
}

var (
	podmanCPUMap = make(map[string]podmanCPUSample)
	podmanCPUMu  sync.Mutex
)

func (c *Client) GetAllContainersStats() (map[string]*system.ContainerStats, error) {
	result := make(map[string]*system.ContainerStats)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, sc := range c.allClients {
		if sc.engineInfo.IsPodman {
			wg.Add(1)
			go func(podmanClient *SingleEngineClient) {
				defer wg.Done()
				body, code, err := podmanClient.Get("/libpod/containers/stats?stream=false")
				if err == nil && code == 200 {
					var lpResp LibpodStatsResponse
					if errJson := json.Unmarshal(body, &lpResp); errJson == nil && len(lpResp.Stats) > 0 {
						now := time.Now()
						for _, s := range lpResp.Stats {
							cid := s.ContainerID
							shortID := cid
							if len(shortID) > 12 {
								shortID = shortID[:12]
							}

							podmanCPUMu.Lock()
							lastSample, hasLast := podmanCPUMap[cid]
							podmanCPUMap[cid] = podmanCPUSample{
								cpuNano:    s.CPUNano,
								systemNano: s.SystemNano,
								recordedAt: now,
							}
							podmanCPUMu.Unlock()

							var cpuPct float64
							if hasLast && s.CPUNano >= lastSample.cpuNano && s.SystemNano > lastSample.systemNano {
								cpuDelta := float64(s.CPUNano - lastSample.cpuNano)
								systemDelta := float64(s.SystemNano - lastSample.systemNano)
								if systemDelta > 0 {
									cpuPct = (cpuDelta / systemDelta) * 100.0
								}
							} else {
								// First cold-start sample: no delta yet, return 0.0 until next tick
								cpuPct = 0.0
							}

							if cpuPct < 0 {
								cpuPct = 0
							}

							st := &system.ContainerStats{
								CPUPercent:   cpuPct,
								MemUsageMB:   float64(s.MemUsage) / (1024 * 1024),
								MemLimitMB:   float64(s.MemLimit) / (1024 * 1024),
								MemPercent:   s.MemPerc,
								NetRxMB:      float64(s.NetInput) / (1024 * 1024),
								NetTxMB:      float64(s.NetOutput) / (1024 * 1024),
								BlockReadMB:  float64(s.BlockInput) / (1024 * 1024),
								BlockWriteMB: float64(s.BlockOutput) / (1024 * 1024),
							}

							cleanName := strings.TrimPrefix(s.Name, "/")
							mu.Lock()
							result[cid] = st
							result[shortID] = st
							if cleanName != "" {
								result[cleanName] = st
							}
							mu.Unlock()
						}
					}
				}
			}(sc)
		} else {
			// Docker client
			wg.Add(1)
			go func(dockerClient *SingleEngineClient) {
				defer wg.Done()
				clientCtrs, code, errList := dockerClient.Get("/containers/json")
				if errList == nil && code == 200 {
					var rawList []RawContainer
					if errU := json.Unmarshal(clientCtrs, &rawList); errU == nil {
						var subWg sync.WaitGroup
						for _, ctr := range rawList {
							if ctr.State != "running" {
								continue
							}
							subWg.Add(1)
							go func(singleCli *SingleEngineClient, id string, name string) {
								defer subWg.Done()
								body, code, err := singleCli.Get(fmt.Sprintf("/containers/%s/stats?stream=false", id))
								if err == nil && code == 200 {
									stats, errParse := system.ParseDockerStats(body)
									if errParse == nil && stats != nil {
										cleanName := strings.TrimPrefix(name, "/")
										mu.Lock()
										result[id] = stats
										if len(id) > 12 {
											result[id[:12]] = stats
										}
										if cleanName != "" {
											// Only set if not already set by a running container
											if _, exists := result[cleanName]; !exists {
												result[cleanName] = stats
											}
										}
										mu.Unlock()
									}
								}
							}(dockerClient, ctr.ID, func() string {
								if len(ctr.Names) > 0 {
									return ctr.Names[0]
								}
								return ""
							}())
						}
						subWg.Wait()
					}
				}
			}(sc)
		}
	}

	wg.Wait()
	return result, nil
}

type DockerTotalSummary struct {
	TotalContainers   int     `json:"total_containers"`
	RunningContainers int     `json:"running_containers"`
	CPUPercent        float64 `json:"cpu_percent"`
	MemUsedMB         float64 `json:"mem_used_mb"`
	MemPercent        float64 `json:"mem_percent"`
	NetRxMB           float64 `json:"net_rx_mb"`
	NetTxMB           float64 `json:"net_tx_mb"`
}

func (c *Client) GetTotalDockerStats(hostMemTotalMB uint64) (*DockerTotalSummary, error) {
	containers, err := c.ListContainers()
	if err != nil {
		return nil, err
	}

	summary := &DockerTotalSummary{
		TotalContainers: len(containers),
	}

	allStats, _ := c.GetAllContainersStats()

	for _, ctr := range containers {
		if ctr.State == "running" {
			summary.RunningContainers++
		}
		if st, ok := allStats[ctr.ID]; ok && st != nil {
			summary.CPUPercent += st.CPUPercent
			summary.MemUsedMB += st.MemUsageMB
			summary.NetRxMB += st.NetRxMB
			summary.NetTxMB += st.NetTxMB
		}
	}

	if hostMemTotalMB > 0 {
		summary.MemPercent = (summary.MemUsedMB / float64(hostMemTotalMB)) * 100.0
	}

	return summary, nil
}
