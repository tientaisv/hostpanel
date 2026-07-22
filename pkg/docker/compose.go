package docker

import (
	"fmt"
	"sort"
	"dockpulse/pkg/system"
)

type ComposeService struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Service    string  `json:"service"`
	State      string  `json:"state"`
	Status     string  `json:"status"`
	Image      string  `json:"image"`
	PortsStr   string  `json:"ports_str"`
	CPUPercent float64 `json:"cpu_percent"`
	MemUsageMB float64 `json:"mem_usage_mb"`
	MemLimitMB float64 `json:"mem_limit_mb"`
	MemPercent float64 `json:"mem_percent"`
	NetRxMB    float64 `json:"net_rx_mb"`
	NetTxMB    float64 `json:"net_tx_mb"`
}

type ComposeStack struct {
	Project         string           `json:"project"`
	Services        []ComposeService `json:"services"`
	Total           int              `json:"total"`
	RunningCount    int              `json:"running_count"`
	State           string           `json:"state"` // "running", "partial", "stopped"
	TotalCPUPercent float64          `json:"total_cpu_percent"`
	TotalMemUsageMB float64          `json:"total_mem_usage_mb"`
	TotalMemLimitMB float64          `json:"total_mem_limit_mb"`
	TotalMemPercent float64          `json:"total_mem_percent"`
	TotalNetRxMB    float64          `json:"total_net_rx_mb"`
	TotalNetTxMB    float64          `json:"total_net_tx_mb"`
}

func (c *Client) ListComposeStacks() ([]ComposeStack, error) {
	return c.ListComposeStacksWithStats(false)
}

func (c *Client) ListComposeStacksWithStats(includeStats bool) ([]ComposeStack, error) {
	containers, err := c.ListContainers()
	if err != nil {
		return nil, err
	}

	var allStats map[string]*system.ContainerStats
	if includeStats {
		allStats, _ = c.GetAllContainersStats()
	}

	stacksMap := make(map[string]*ComposeStack)

	for _, ctr := range containers {
		proj := ctr.Project
		if proj == "" {
			// Skip containers not belonging to a Compose project
			continue
		}

		stack, exists := stacksMap[proj]
		if !exists {
			stack = &ComposeStack{
				Project:  proj,
				Services: make([]ComposeService, 0),
			}
			stacksMap[proj] = stack
		}

		srvName := ctr.Labels["com.docker.compose.service"]
		if srvName == "" {
			srvName = ctr.Name
		}

		portsStr := ""
		for _, p := range ctr.Ports {
			if p.PublicPort > 0 {
				portsStr += fmt.Sprintf("%d:%d ", p.PublicPort, p.PrivatePort)
			}
		}

		service := ComposeService{
			ID:       ctr.ID,
			Name:     ctr.Name,
			Service:  srvName,
			State:    ctr.State,
			Status:   ctr.Status,
			Image:    ctr.Image,
			PortsStr: portsStr,
		}

		if includeStats && allStats != nil {
			if st, ok := allStats[ctr.ID]; ok && st != nil {
				service.CPUPercent = st.CPUPercent
				service.MemUsageMB = st.MemUsageMB
				service.MemLimitMB = st.MemLimitMB
				service.MemPercent = st.MemPercent
				service.NetRxMB = st.NetRxMB
				service.NetTxMB = st.NetTxMB

				stack.TotalCPUPercent += st.CPUPercent
				stack.TotalMemUsageMB += st.MemUsageMB
				if st.MemLimitMB > stack.TotalMemLimitMB {
					stack.TotalMemLimitMB = st.MemLimitMB
				}
				stack.TotalNetRxMB += st.NetRxMB
				stack.TotalNetTxMB += st.NetTxMB
			}
		}

		stack.Services = append(stack.Services, service)
		stack.Total++
		if ctr.State == "running" {
			stack.RunningCount++
		}
	}

	result := make([]ComposeStack, 0)
	for _, stack := range stacksMap {
		if stack.RunningCount == stack.Total {
			stack.State = "running"
		} else if stack.RunningCount > 0 {
			stack.State = "partial"
		} else {
			stack.State = "stopped"
		}

		if stack.TotalMemLimitMB > 0 {
			stack.TotalMemPercent = (stack.TotalMemUsageMB / stack.TotalMemLimitMB) * 100.0
		}

		result = append(result, *stack)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].TotalCPUPercent != result[j].TotalCPUPercent {
			return result[i].TotalCPUPercent > result[j].TotalCPUPercent
		}
		if result[i].TotalMemUsageMB != result[j].TotalMemUsageMB {
			return result[i].TotalMemUsageMB > result[j].TotalMemUsageMB
		}
		return result[i].RunningCount > result[j].RunningCount
	})

	return result, nil
}

func (c *Client) StackAction(project string, action string) error {
	stacks, err := c.ListComposeStacks()
	if err != nil {
		return err
	}

	for _, s := range stacks {
		if s.Project == project {
			for _, srv := range s.Services {
				_ = c.ContainerAction(srv.ID, action)
			}
			return nil
		}
	}

	return fmt.Errorf("stack %s not found", project)
}
