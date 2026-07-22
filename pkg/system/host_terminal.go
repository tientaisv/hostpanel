package system

import (
	"io"
	"os"
	"os/exec"

	"dockpulse/pkg/ws"
)

func HandleHostWebTerminal(wsConn *ws.Conn) error {
	// Use python3 pty module to allocate a real Pseudo-TTY for full terminal support (htop, vim, color)
	cmd := exec.Command("python3", "-c", "import pty, os; os.environ['TERM']='xterm-256color'; os.environ['COLORTERM']='truecolor'; pty.spawn('/bin/bash')")
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		// Fallback to script command if python3 is unavailable
		cmd = exec.Command("script", "-q", "-c", "/bin/bash", "/dev/null")
		cmd.Env = append(os.Environ(), "TERM=xterm-256color")
		stdinPipe, _ = cmd.StdinPipe()
		stdoutPipe, _ = cmd.StdoutPipe()
		stderrPipe, _ = cmd.StderrPipe()
		if errStart := cmd.Start(); errStart != nil {
			// Final fallback to direct bash
			cmd = exec.Command("bash", "-i")
			cmd.Env = append(os.Environ(), "TERM=xterm-256color")
			stdinPipe, _ = cmd.StdinPipe()
			stdoutPipe, _ = cmd.StdoutPipe()
			stderrPipe, _ = cmd.StderrPipe()
			_ = cmd.Start()
		}
	}

	errChan := make(chan error, 3)

	// Stream stdout -> WebSocket
	go func() {
		buf := make([]byte, 2048)
		for {
			n, err := stdoutPipe.Read(buf)
			if n > 0 {
				if errWs := wsConn.WriteText(string(buf[:n])); errWs != nil {
					errChan <- errWs
					return
				}
			}
			if err != nil {
				if err != io.EOF {
					errChan <- err
				}
				return
			}
		}
	}()

	// Stream stderr -> WebSocket
	go func() {
		buf := make([]byte, 2048)
		for {
			n, err := stderrPipe.Read(buf)
			if n > 0 {
				if errWs := wsConn.WriteText(string(buf[:n])); errWs != nil {
					errChan <- errWs
					return
				}
			}
			if err != nil {
				if err != io.EOF {
					errChan <- err
				}
				return
			}
		}
	}()

	// Stream WebSocket -> Stdin
	go func() {
		for {
			_, msg, errWs := wsConn.ReadMessage()
			if errWs != nil {
				errChan <- errWs
				return
			}
			if len(msg) > 0 {
				if _, errWrite := stdinPipe.Write(msg); errWrite != nil {
					errChan <- errWrite
					return
				}
			}
		}
	}()

	<-errChan
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	return nil
}
