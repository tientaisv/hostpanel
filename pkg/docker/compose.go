package docker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

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
	Engine     string  `json:"engine"` // "podman" or "docker"
	WorkingDir string  `json:"working_dir,omitempty"`
	ConfigFile string  `json:"config_file,omitempty"`
}

type ComposeStack struct {
	Project         string           `json:"project"`
	WorkingDir      string           `json:"working_dir,omitempty"`
	ConfigFile      string           `json:"config_file,omitempty"`
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
	Engine          string           `json:"engine"` // "podman" or "docker"
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
			// Skip containers not belonging to a Compose project or Pod
			continue
		}

		stackKey := fmt.Sprintf("%s|%s", proj, ctr.Engine)
		stack, exists := stacksMap[stackKey]
		if !exists {
			stack = &ComposeStack{
				Project:  proj,
				Services: make([]ComposeService, 0),
				Engine:   ctr.Engine,
			}
			stacksMap[stackKey] = stack
		}

		workingDir := ""
		configFile := ""
		if ctr.Labels != nil {
			if wd, ok := ctr.Labels["com.docker.compose.project.working_dir"]; ok && wd != "" {
				workingDir = wd
			} else if wd, ok := ctr.Labels["io.podman.compose.project.working_dir"]; ok && wd != "" {
				workingDir = wd
			}
			if cf, ok := ctr.Labels["com.docker.compose.project.config_files"]; ok && cf != "" {
				configFile = cf
			} else if cf, ok := ctr.Labels["io.podman.compose.project.config_files"]; ok && cf != "" {
				configFile = cf
			}
		}

		if stack.WorkingDir == "" && workingDir != "" {
			stack.WorkingDir = workingDir
		}
		if stack.ConfigFile == "" && configFile != "" {
			stack.ConfigFile = configFile
		}

		srvName := ""
		if ctr.Labels != nil {
			if s, ok := ctr.Labels["com.docker.compose.service"]; ok && s != "" {
				srvName = s
			} else if s, ok := ctr.Labels["io.podman.compose.service"]; ok && s != "" {
				srvName = s
			}
		}
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
			ID:         ctr.ID,
			Name:       ctr.Name,
			Service:    srvName,
			State:      ctr.State,
			Status:     ctr.Status,
			Image:      ctr.Image,
			PortsStr:   portsStr,
			Engine:     ctr.Engine,
			WorkingDir: workingDir,
			ConfigFile: configFile,
		}

		if includeStats && allStats != nil {
			var st *system.ContainerStats
			if s, ok := allStats[ctr.ID]; ok && s != nil {
				st = s
			} else if s, ok := allStats[ctr.ShortID]; ok && s != nil {
				st = s
			} else if s, ok := allStats[ctr.Name]; ok && s != nil {
				st = s
			}

			if st != nil {
				service.CPUPercent = st.CPUPercent
				service.MemUsageMB = st.MemUsageMB
				service.MemLimitMB = st.MemLimitMB
				service.MemPercent = st.MemPercent
				service.NetRxMB = st.NetRxMB
				service.NetTxMB = st.NetTxMB
			}
		}

		stack.Services = append(stack.Services, service)
	}

	result := make([]ComposeStack, 0, len(stacksMap))
	for _, stack := range stacksMap {
		stack.Total = len(stack.Services)
		runningCount := 0
		var totalCPU, totalMemUsage, totalMemLimit, totalNetRx, totalNetTx float64

		for _, s := range stack.Services {
			if s.State == "running" {
				runningCount++
			}
			totalCPU += s.CPUPercent
			totalMemUsage += s.MemUsageMB
			if s.MemLimitMB > totalMemLimit {
				totalMemLimit = s.MemLimitMB
			}
			totalNetRx += s.NetRxMB
			totalNetTx += s.NetTxMB
		}
		stack.RunningCount = runningCount

		if runningCount == stack.Total && stack.Total > 0 {
			stack.State = "running"
		} else if runningCount > 0 {
			stack.State = "partial"
		} else {
			stack.State = "stopped"
		}

		stack.TotalCPUPercent = totalCPU
		stack.TotalMemUsageMB = totalMemUsage
		stack.TotalMemLimitMB = totalMemLimit
		if totalMemLimit > 0 {
			stack.TotalMemPercent = (totalMemUsage / totalMemLimit) * 100
		}
		stack.TotalNetRxMB = totalNetRx
		stack.TotalNetTxMB = totalNetTx

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

	found := false
	for _, s := range stacks {
		projKey := fmt.Sprintf("%s|%s", s.Project, s.Engine)
		if s.Project == project || projKey == project || strings.EqualFold(s.Project, project) || strings.EqualFold(projKey, project) {
			found = true
			var wg sync.WaitGroup
			for _, srv := range s.Services {
				wg.Add(1)
				go func(sid string) {
					defer wg.Done()
					_ = c.ContainerAction(sid, action)
				}(srv.ID)
			}
			wg.Wait()
		}
	}

	if found {
		return nil
	}
	return fmt.Errorf("stack %s not found", project)
}

// RecreateCompose executes compose up -d --force-recreate for a whole project or a single service
func (c *Client) RecreateCompose(project, service, workingDir, configFile string, pull bool, build bool) (string, error) {
	// Clean up project name in case "project|engine" key was passed
	if strings.Contains(project, "|") {
		parts := strings.Split(project, "|")
		project = parts[0]
	}

	// 1. Locate working directory and config file from active stacks if not passed
	if workingDir == "" || configFile == "" {
		stacks, err := c.ListComposeStacks()
		if err == nil {
			for _, s := range stacks {
				if strings.EqualFold(s.Project, project) {
					if workingDir == "" && s.WorkingDir != "" {
						workingDir = s.WorkingDir
					}
					if configFile == "" && s.ConfigFile != "" {
						configFile = s.ConfigFile
					}
					break
				}
			}
		}
	}

	// 2. Fallback search for common paths
	if workingDir == "" {
		candidates := []string{
			filepath.Join("/home/data", project),
			filepath.Join("/opt", project),
			filepath.Join("/var/www", project),
			filepath.Join("/root", project),
		}
		for _, cand := range candidates {
			if info, err := os.Stat(cand); err == nil && info.IsDir() {
				workingDir = cand
				break
			}
		}
	}

	// 3. Find Compose CLI tool
	cliCmd := ""
	var baseArgs []string
	if c.IsPodman() {
		// Prefer podman-compose on Podman systems
		if path, err := exec.LookPath("podman-compose"); err == nil {
			cliCmd = path
		} else if path, err := exec.LookPath("docker-compose"); err == nil {
			cliCmd = path
		} else if path, err := exec.LookPath("docker"); err == nil {
			if err := exec.Command("docker", "compose", "version").Run(); err == nil {
				cliCmd = path
				baseArgs = append(baseArgs, "compose")
			}
		}
	} else {
		if path, err := exec.LookPath("docker-compose"); err == nil {
			cliCmd = path
		} else if path, err := exec.LookPath("podman-compose"); err == nil {
			cliCmd = path
		} else if path, err := exec.LookPath("docker"); err == nil {
			if err := exec.Command("docker", "compose", "version").Run(); err == nil {
				cliCmd = path
				baseArgs = append(baseArgs, "compose")
			}
		}
	}

	if cliCmd == "" {
		return "", fmt.Errorf("không tìm thấy docker-compose, podman-compose hoặc docker compose CLI trên hệ thống")
	}

	// 4. Build command arguments
	var args []string
	args = append(args, baseArgs...)

	if configFile != "" {
		args = append(args, "-f", configFile)
	}

	args = append(args, "up", "-d", "--force-recreate")
	if pull {
		args = append(args, "--pull", "always")
	}
	if build {
		args = append(args, "--build")
	}
	if service != "" {
		args = append(args, service)
	}

	cmd := exec.Command(cliCmd, args...)
	if workingDir != "" {
		cmd.Dir = workingDir
	}
	cmd.Env = os.Environ()
	if c.IsPodman() {
		cmd.Env = append(cmd.Env, "DOCKER_HOST=unix:///run/podman/podman.sock")
	}

	out, err := cmd.CombinedOutput()
	outStr := strings.TrimSpace(string(out))
	cmdLineStr := fmt.Sprintf("%s %s", filepath.Base(cliCmd), strings.Join(args, " "))

	if err != nil {
		return outStr, fmt.Errorf("lỗi thực thi '%s' (Dir: %s): %v\n\n%s", cmdLineStr, workingDir, err, outStr)
	}

	resultHeader := fmt.Sprintf("✅ Thực thi thành công: %s\n📂 Thư mục làm việc: %s\n========================================\n", cmdLineStr, workingDir)
	return resultHeader + outStr, nil
}
