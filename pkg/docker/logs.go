package docker

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"dockpulse/pkg/ws"
)

func (c *Client) StreamLogsToWS(id string, tail string, wsConn *ws.Conn) error {
	if tail == "" {
		tail = "200"
	}

	sc := c.GetClientForContainer(id)

	// Check if container has TTY enabled
	isTTY := false
	inspectBody, code, errInsp := sc.Get(fmt.Sprintf("/containers/%s/json", id))
	if errInsp == nil && code == 200 {
		var inspectData struct {
			Config struct {
				Tty bool `json:"Tty"`
			} `json:"Config"`
		}
		if jsonErr := json.Unmarshal(inspectBody, &inspectData); jsonErr == nil {
			isTTY = inspectData.Config.Tty
		}
	}

	path := fmt.Sprintf("/containers/%s/logs?follow=1&stdout=1&stderr=1&timestamps=0&tail=%s", id, tail)

	req, err := http.NewRequest("GET", "http://localhost"+path, nil)
	if err != nil {
		return err
	}

	conn, err := sc.RawConn()
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := req.Write(conn); err != nil {
		return err
	}

	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("invalid response status from log stream: %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")

	// Docker uses application/vnd.docker.multiplexed-stream
	// Podman uses application/octet-stream for multiplexed streams
	// When Tty is disabled (default), logs are ALWAYS multiplexed with 8-byte headers
	isMultiplexed := !isTTY && (strings.Contains(contentType, "multiplexed") ||
		strings.Contains(contentType, "octet-stream") ||
		contentType == "")

	if isMultiplexed {
		// Stream frames (Docker & Podman multiplex header is 8 bytes per message: 1 byte stream type, 3 unused, 4 byte size BigEndian)
		headerBuf := make([]byte, 8)
		for {
			_, err := io.ReadFull(resp.Body, headerBuf)
			if err != nil {
				break
			}

			// Validate stream type: 0 = stdin, 1 = stdout, 2 = stderr
			streamType := headerBuf[0]
			if streamType > 2 && streamType != 0 {
				// Potential raw character fallback
				if errWs := wsConn.WriteText(string(headerBuf)); errWs != nil {
					break
				}
				continue
			}

			// Frame length is in last 4 bytes (BigEndian uint32)
			frameLen := uint32(headerBuf[4])<<24 | uint32(headerBuf[5])<<16 | uint32(headerBuf[6])<<8 | uint32(headerBuf[7])
			if frameLen == 0 {
				continue
			}

			// Cap maximum single message size to prevent excessive memory allocation
			if frameLen > 10*1024*1024 {
				frameLen = 10 * 1024 * 1024
			}

			msgBuf := make([]byte, frameLen)
			_, err = io.ReadFull(resp.Body, msgBuf)
			if err != nil {
				break
			}

			if err := wsConn.WriteText(string(msgBuf)); err != nil {
				break // WebSocket client disconnected
			}
		}
	} else {
		// Raw stream for TTY containers
		buf := make([]byte, 4096)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				if errWs := wsConn.WriteText(string(buf[:n])); errWs != nil {
					break
				}
			}
			if err != nil {
				break
			}
		}
	}

	return nil
}

func (c *Client) TruncateContainerLogs(id string) error {
	sc := c.GetClientForContainer(id)
	var targetLogPaths []string

	path := fmt.Sprintf("/containers/%s/json", id)
	body, code, err := sc.Get(path)
	if err == nil && code == 200 {
		var inspectData struct {
			LogPath string `json:"LogPath"`
		}
		if jsonErr := json.Unmarshal(body, &inspectData); jsonErr == nil && inspectData.LogPath != "" {
			targetLogPaths = append(targetLogPaths, inspectData.LogPath)
		}
	}

	// Add known Podman and Docker log locations
	targetLogPaths = append(targetLogPaths,
		fmt.Sprintf("/var/lib/containers/storage/overlay-containers/%s/userdata/ctr.log", id),
		fmt.Sprintf("/var/lib/docker/containers/%s/%s-json.log", id, id),
	)

	// Try truncating the found log files
	for _, lp := range targetLogPaths {
		if file, err := os.OpenFile(lp, os.O_WRONLY|os.O_TRUNC, 0600); err == nil {
			file.Close()
			return nil
		}
	}

	// Fallback wildcard scan in Docker directory
	logDirPattern := fmt.Sprintf("/var/lib/docker/containers/%s/", id)
	if entries, errDir := ioutil.ReadDir(logDirPattern); errDir == nil {
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".log") {
				fullPath := logDirPattern + entry.Name()
				if f, errTrunc := os.OpenFile(fullPath, os.O_WRONLY|os.O_TRUNC, 0600); errTrunc == nil {
					f.Close()
					return nil
				}
			}
		}
	}

	// Fallback wildcard scan in Podman storage directory
	podmanDirPattern := fmt.Sprintf("/var/lib/containers/storage/overlay-containers/%s/userdata/", id)
	if entries, errDir := ioutil.ReadDir(podmanDirPattern); errDir == nil {
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".log") {
				fullPath := podmanDirPattern + entry.Name()
				if f, errTrunc := os.OpenFile(fullPath, os.O_WRONLY|os.O_TRUNC, 0600); errTrunc == nil {
					f.Close()
					return nil
				}
			}
		}
	}

	return fmt.Errorf("unable to find or truncate log file for container %s", id)
}
