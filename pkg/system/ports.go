package system

import (
	"bufio"
	"bytes"
	"io/ioutil"
	"net"
	"os/exec"
	"strconv"
	"strings"
)

type PortInfo struct {
	Protocol    string `json:"protocol"`
	LocalIP     string `json:"local_ip"`
	LocalPort   int    `json:"local_port"`
	State       string `json:"state"`
	PID         int    `json:"pid"`
	ProcessName string `json:"process_name"`
}

func GetListeningPorts() ([]PortInfo, error) {
	// Try ss -tulpn first
	ports, err := getPortsViaSS()
	if err == nil && len(ports) > 0 {
		return ports, nil
	}

	// Fallback to reading /proc/net/tcp and /proc/net/udp
	return getPortsViaProcNet()
}

func getPortsViaSS() ([]PortInfo, error) {
	cmd := exec.Command("ss", "-tulpn")
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	var results []PortInfo
	scanner := bufio.NewScanner(&out)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Netid") || strings.HasPrefix(line, "State") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		proto := fields[0]
		state := fields[1]
		localAddr := fields[4]

		// Filter for listening sockets
		if state != "LISTEN" && state != "UNCONN" && proto != "udp" && proto != "udp6" {
			continue
		}

		host, portStr, err := net.SplitHostPort(localAddr)
		if err != nil {
			// Sockets like *:8080 or [::]:8080
			idx := strings.LastIndex(localAddr, ":")
			if idx != -1 {
				host = localAddr[:idx]
				portStr = localAddr[idx+1:]
			}
		}

		port, _ := strconv.Atoi(portStr)
		if port == 0 {
			continue
		}

		pid := 0
		procName := ""
		if len(fields) >= 7 {
			procStr := fields[6] // e.g. users:(("dockpulse",pid=1234,fd=3))
			if idx := strings.Index(procStr, "pid="); idx != -1 {
				sub := procStr[idx+4:]
				if endIdx := strings.IndexAny(sub, ",)"); endIdx != -1 {
					pid, _ = strconv.Atoi(sub[:endIdx])
				}
			}
			if idxStart := strings.Index(procStr, "((\""); idxStart != -1 {
				sub := procStr[idxStart+3:]
				if idxEnd := strings.Index(sub, "\""); idxEnd != -1 {
					procName = sub[:idxEnd]
				}
			}
		}

		results = append(results, PortInfo{
			Protocol:    proto,
			LocalIP:     host,
			LocalPort:   port,
			State:       state,
			PID:         pid,
			ProcessName: procName,
		})
	}

	return results, nil
}

func getPortsViaProcNet() ([]PortInfo, error) {
	var results []PortInfo

	files := []struct {
		path  string
		proto string
	}{
		{"/proc/net/tcp", "tcp"},
		{"/proc/net/tcp6", "tcp6"},
		{"/proc/net/udp", "udp"},
		{"/proc/net/udp6", "udp6"},
	}

	for _, f := range files {
		data, err := ioutil.ReadFile(f.path)
		if err != nil {
			continue
		}

		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if i == 0 || strings.TrimSpace(line) == "" {
				continue
			}

			fields := strings.Fields(line)
			if len(fields) < 4 {
				continue
			}

			// state 0A is LISTEN for TCP
			stateHex := fields[3]
			if f.proto == "tcp" || f.proto == "tcp6" {
				if stateHex != "0A" {
					continue
				}
			}

			localAddrHex := fields[1]
			parts := strings.Split(localAddrHex, ":")
			if len(parts) != 2 {
				continue
			}

			port, err := strconv.ParseInt(parts[1], 16, 64)
			if err != nil || port == 0 {
				continue
			}

			results = append(results, PortInfo{
				Protocol:  f.proto,
				LocalIP:   "0.0.0.0",
				LocalPort: int(port),
				State:     "LISTEN",
			})
		}
	}

	return results, nil
}
