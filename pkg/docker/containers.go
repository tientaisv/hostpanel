package docker

import (
	"encoding/json"
	"fmt"
	"sync"

	"dockpulse/pkg/system"
)

type PortMapping struct {
	IP          string `json:"ip"`
	PrivatePort int    `json:"private_port"`
	PublicPort  int    `json:"public_port"`
	Type        string `json:"type"`
}

type ContainerSummary struct {
	ID         string            `json:"id"`
	ShortID    string            `json:"short_id"`
	Names      []string          `json:"names"`
	Name       string            `json:"name"`
	Image      string            `json:"image"`
	ImageID    string            `json:"image_id"`
	Command    string            `json:"command"`
	Created    int64             `json:"created"`
	State      string            `json:"state"`
	Status     string            `json:"status"`
	Ports      []PortMapping     `json:"ports"`
	IPs        map[string]string `json:"ips"` // network_name -> IP
	Labels     map[string]string `json:"labels"`
	Project    string            `json:"project"` // Compose project name
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
	body, code, err := c.Get("/containers/json?all=1")
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("docker api error status %d: %s", code, string(body))
	}

	var rawList []RawContainer
	if err := json.Unmarshal(body, &rawList); err != nil {
		return nil, err
	}

	var result []ContainerSummary
	for _, r := range rawList {
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
			proj = r.Labels["com.docker.compose.project"]
		}

		result = append(result, ContainerSummary{
			ID:         r.ID,
			ShortID:    shortID,
			Names:      r.Names,
			Name:       name,
			Image:      r.Image,
			ImageID:    r.ImageID,
			Command:    r.Command,
			Created:    r.Created,
			State:      r.State,
			Status:     r.Status,
			Ports:      ports,
			IPs:        ips,
			Labels:     r.Labels,
			Project:    proj,
		})
	}

	return result, nil
}

func (c *Client) ContainerAction(id, action string) error {
	path := fmt.Sprintf("/containers/%s/%s", id, action)
	body, code, err := c.Post(path, nil)
	if err != nil {
		return err
	}
	if code != 204 && code != 200 {
		return fmt.Errorf("action %s failed (status %d): %s", action, code, string(body))
	}
	return nil
}

func (c *Client) RemoveContainer(id string, force bool) error {
	path := fmt.Sprintf("/containers/%s?force=%t&v=true", id, force)
	body, code, err := c.Delete(path)
	if err != nil {
		return err
	}
	if code != 204 && code != 200 {
		return fmt.Errorf("remove failed (status %d): %s", code, string(body))
	}
	return nil
}

func (c *Client) GetContainerStats(id string) (*system.ContainerStats, error) {
	path := fmt.Sprintf("/containers/%s/stats?stream=false", id)
	body, code, err := c.Get(path)
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("stats status %d: %s", code, string(body))
	}

	return system.ParseDockerStats(body)
}

func (c *Client) GetAllContainersStats() (map[string]*system.ContainerStats, error) {
	containers, err := c.ListContainers()
	if err != nil {
		return nil, err
	}

	result := make(map[string]*system.ContainerStats)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, ctr := range containers {
		if ctr.State != "running" {
			continue
		}
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			stats, err := c.GetContainerStats(id)
			if err == nil && stats != nil {
				mu.Lock()
				result[id] = stats
				mu.Unlock()
			}
		}(ctr.ID)
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
