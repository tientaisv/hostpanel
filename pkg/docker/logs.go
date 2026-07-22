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
	path := fmt.Sprintf("/containers/%s/logs?follow=1&stdout=1&stderr=1&timestamps=0&tail=%s", id, tail)

	req, err := http.NewRequest("GET", "http://localhost"+path, nil)
	if err != nil {
		return err
	}

	conn, err := c.RawConn()
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
		return fmt.Errorf("invalid response status from docker log stream: %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	isMultiplexed := strings.Contains(contentType, "multiplexed") || (contentType == "" && !strings.Contains(contentType, "raw"))

	if isMultiplexed {
		// Stream frames (Docker stdout/stderr multiplex header is 8 bytes per message)
		headerBuf := make([]byte, 8)
		for {
			_, err := io.ReadFull(resp.Body, headerBuf)
			if err != nil {
				break
			}

			// Frame length is in last 4 bytes (BigEndian uint32)
			frameLen := uint32(headerBuf[4])<<24 | uint32(headerBuf[5])<<16 | uint32(headerBuf[6])<<8 | uint32(headerBuf[7])
			if frameLen == 0 {
				continue
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
	// Inspect container to get LogPath if possible
	logPath := fmt.Sprintf("/var/lib/docker/containers/%s/%s-json.log", id, id)

	path := fmt.Sprintf("/containers/%s/json", id)
	body, code, err := c.Get(path)
	if err == nil && code == 200 {
		var inspectData struct {
			LogPath string `json:"LogPath"`
		}
		if jsonErr := json.Unmarshal(body, &inspectData); jsonErr == nil && inspectData.LogPath != "" {
			logPath = inspectData.LogPath
		}
	}

	// Try truncating the file directly on disk
	file, err := os.OpenFile(logPath, os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		// Fallback search in /var/lib/docker/containers/<id>/
		logPathPattern := fmt.Sprintf("/var/lib/docker/containers/%s/", id)
		entries, errDir := ioutil.ReadDir(logPathPattern)
		if errDir == nil {
			for _, entry := range entries {
				if strings.HasSuffix(entry.Name(), "-json.log") {
					fullPath := logPathPattern + entry.Name()
					if f, errTrunc := os.OpenFile(fullPath, os.O_WRONLY|os.O_TRUNC, 0600); errTrunc == nil {
						f.Close()
						return nil
					}
				}
			}
		}
		return fmt.Errorf("unable to truncate log file: %v", err)
	}
	file.Close()
	return nil
}
