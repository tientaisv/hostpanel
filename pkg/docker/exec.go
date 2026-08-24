package docker

import (
	"bytes"
	"encoding/json"
	"fmt"

	"dockpulse/pkg/ws"
)

type ExecCreateConfig struct {
	AttachStdin  bool     `json:"AttachStdin"`
	AttachStdout bool     `json:"AttachStdout"`
	AttachStderr bool     `json:"AttachStderr"`
	Tty          bool     `json:"Tty"`
	Cmd          []string `json:"Cmd"`
}

type ExecCreateResponse struct {
	ID string `json:"Id"`
}

func (c *Client) HandleWebTerminal(containerID string, wsConn *ws.Conn) error {
	sc := c.GetClientForContainer(containerID)

	// Step 1: Create Exec Instance
	execCfg := ExecCreateConfig{
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		Cmd:          []string{"/bin/sh"},
	}
	cfgBytes, _ := json.Marshal(execCfg)

	resBody, code, err := sc.Post(fmt.Sprintf("/containers/%s/exec", containerID), cfgBytes)
	if err != nil || code != 201 {
		// Fallback to /bin/bash if /bin/sh failed or retry with sh
		execCfg.Cmd = []string{"sh"}
		cfgBytes, _ = json.Marshal(execCfg)
		resBody, code, err = sc.Post(fmt.Sprintf("/containers/%s/exec", containerID), cfgBytes)
		if err != nil || code != 201 {
			return fmt.Errorf("create exec failed status %d: %s", code, string(resBody))
		}
	}

	var execResp ExecCreateResponse
	if err := json.Unmarshal(resBody, &execResp); err != nil {
		return err
	}
	execID := execResp.ID

	// Step 2: Start Exec Session via raw Unix socket
	rawConn, err := sc.RawConn()
	if err != nil {
		return err
	}
	defer rawConn.Close()

	startPayload := []byte(`{"Detach": false, "Tty": true}`)
	reqStr := fmt.Sprintf("POST /exec/%s/start HTTP/1.1\r\nHost: localhost\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n%s", execID, len(startPayload), string(startPayload))

	if _, err := rawConn.Write([]byte(reqStr)); err != nil {
		return err
	}

	// Skip HTTP header response
	buf := make([]byte, 1024)
	n, err := rawConn.Read(buf)
	if err != nil || !bytes.Contains(buf[:n], []byte("101")) && !bytes.Contains(buf[:n], []byte("200")) {
		// Just proceed if read was fine
	}

	// Step 3: Stream stdin/stdout 2-way between WebSocket and Raw Container Terminal
	errChan := make(chan error, 2)

	// Read from container raw socket -> Write to WebSocket
	go func() {
		readBuf := make([]byte, 2048)
		for {
			nr, errRead := rawConn.Read(readBuf)
			if nr > 0 {
				if errWs := wsConn.WriteText(string(readBuf[:nr])); errWs != nil {
					errChan <- errWs
					return
				}
			}
			if errRead != nil {
				errChan <- errRead
				return
			}
		}
	}()

	// Read from WebSocket -> Write to container raw socket
	go func() {
		for {
			_, msg, errWs := wsConn.ReadMessage()
			if errWs != nil {
				errChan <- errWs
				return
			}
			if len(msg) > 0 {
				if _, errWrite := rawConn.Write(msg); errWrite != nil {
					errChan <- errWrite
					return
				}
			}
		}
	}()

	<-errChan
	return nil
}
