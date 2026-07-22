let containerTerminal = null;
let hostTerminal = null;
let fitAddonContainer = null;
let fitAddonHost = null;
let containerWS = null;
let hostWS = null;

function openTerminalModal(containerID, name) {
  document.getElementById("terminal-title").textContent = `💻 Container Terminal Shell - ${name}`;
  openModal("modal-terminal");

  const termContainer = document.getElementById("terminal-console");
  if (!termContainer) return;
  termContainer.innerHTML = "";

  if (containerWS) {
    try { containerWS.close(); } catch (e) {}
  }
  if (containerTerminal) {
    try { containerTerminal.dispose(); } catch (e) {}
  }

  containerTerminal = new Terminal({
    cursorBlink: true,
    fontSize: 14,
    fontFamily: 'Fira Code, Courier New, monospace',
    theme: {
      background: '#070a12',
      foreground: '#f8fafc',
      cursor: '#38bdf8'
    }
  });

  if (window.FitAddon && window.FitAddon.FitAddon) {
    fitAddonContainer = new window.FitAddon.FitAddon();
    containerTerminal.loadAddon(fitAddonContainer);
  }

  containerTerminal.open(termContainer);
  if (fitAddonContainer) {
    setTimeout(() => fitAddonContainer.fit(), 100);
  }

  const wsProto = window.location.protocol === "https:" ? "wss:" : "ws:";
  containerWS = new WebSocket(`${wsProto}//${window.location.host}/ws/terminal?id=${containerID}`);

  containerWS.onopen = () => {
    containerTerminal.writeln(`\x1b[32m✅ Connected to container ${name}.\x1b[0m\r\n`);
  };

  containerWS.onmessage = (event) => {
    containerTerminal.write(event.data);
  };

  containerTerminal.onData((data) => {
    if (containerWS && containerWS.readyState === WebSocket.OPEN) {
      containerWS.send(data);
    }
  });

  containerWS.onerror = () => {
    containerTerminal.writeln("\r\n\x1b[31m❌ Terminal WebSocket connection error.\x1b[0m");
  };

  containerWS.onclose = () => {
    containerTerminal.writeln("\r\n\x1b[33m🔌 Terminal Session Closed.\x1b[0m");
  };

  window.addEventListener("resize", handleResizeContainerTerm);
}

function handleResizeContainerTerm() {
  if (fitAddonContainer) {
    try { fitAddonContainer.fit(); } catch (e) {}
  }
}

function closeTerminalModal() {
  if (containerWS) {
    try { containerWS.close(); } catch (e) {}
    containerWS = null;
  }
  if (containerTerminal) {
    try { containerTerminal.dispose(); } catch (e) {}
    containerTerminal = null;
  }
  window.removeEventListener("resize", handleResizeContainerTerm);
  closeModal("modal-terminal");
}

// Host Server Web Terminal Tab Implementation
function initHostTabTerminal() {
  const termContainer = document.getElementById("host-terminal-console");
  if (!termContainer) return;
  termContainer.innerHTML = "";

  if (hostWS) {
    try { hostWS.close(); } catch (e) {}
  }
  if (hostTerminal) {
    try { hostTerminal.dispose(); } catch (e) {}
  }

  hostTerminal = new Terminal({
    cursorBlink: true,
    fontSize: 14,
    fontFamily: 'Fira Code, Courier New, monospace',
    theme: {
      background: '#070a12',
      foreground: '#38bdf8',
      cursor: '#818cf8'
    }
  });

  if (window.FitAddon && window.FitAddon.FitAddon) {
    fitAddonHost = new window.FitAddon.FitAddon();
    hostTerminal.loadAddon(fitAddonHost);
  }

  hostTerminal.open(termContainer);
  if (fitAddonHost) {
    setTimeout(() => fitAddonHost.fit(), 100);
  }

  const wsProto = window.location.protocol === "https:" ? "wss:" : "ws:";
  hostWS = new WebSocket(`${wsProto}//${window.location.host}/ws/host_terminal`);

  hostWS.onopen = () => {
    hostTerminal.writeln(`\x1b[32m⚡ CONNECTED TO HOST SERVER BASH SHELL (xterm-256color PTY Enabled)!\x1b[0m\r\n`);
  };

  hostWS.onmessage = (event) => {
    hostTerminal.write(event.data);
  };

  hostTerminal.onData((data) => {
    if (hostWS && hostWS.readyState === WebSocket.OPEN) {
      hostWS.send(data);
    }
  });

  hostWS.onerror = () => {
    hostTerminal.writeln("\r\n\x1b[31m❌ Host Terminal WebSocket Connection Error.\x1b[0m");
  };

  hostWS.onclose = () => {
    hostTerminal.writeln("\r\n\x1b[33m🔌 Host Terminal Closed.\x1b[0m");
  };

  window.addEventListener("resize", handleResizeHostTerm);
}

function handleResizeHostTerm() {
  if (fitAddonHost) {
    try { fitAddonHost.fit(); } catch (e) {}
  }
}
