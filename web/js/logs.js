let currentLogWS = null;
let currentLogContainerID = null;
let rawLogLines = [];

function openLogsModal(id, name) {
  currentLogContainerID = id;
  document.getElementById("logs-title").textContent = `📋 Live Logs - ${name}`;
  document.getElementById("log-console").textContent = "Đang kết nối live log stream...\n";
  rawLogLines = [];

  openModal("modal-logs");

  if (currentLogWS) {
    currentLogWS.close();
  }

  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  const wsURL = `${protocol}//${window.location.host}/ws/logs?id=${id}&tail=300`;

  currentLogWS = new WebSocket(wsURL);

  currentLogWS.onmessage = (event) => {
    rawLogLines.push(event.data);
    if (rawLogLines.length > 2000) {
      rawLogLines.shift(); // Keep memory lean
    }
    renderLogs();
  };

  currentLogWS.onerror = () => {
    document.getElementById("log-console").textContent += "\n[Lỗi kết nối WebSocket log stream]";
  };

  currentLogWS.onclose = () => {
    // Stream closed
  };
}

function closeLogsModal() {
  if (currentLogWS) {
    currentLogWS.close();
    currentLogWS = null;
  }
  closeModal('modal-logs');
}

function renderLogs() {
  const consoleEl = document.getElementById("log-console");
  const filterVal = document.getElementById("logs-filter").value.toLowerCase().trim();

  let lines = rawLogLines;
  if (filterVal) {
    lines = rawLogLines.filter(line => line.toLowerCase().includes(filterVal));
  }

  consoleEl.textContent = lines.join("");
  consoleEl.scrollTop = consoleEl.scrollHeight;
}

document.getElementById("logs-filter")?.addEventListener("input", () => {
  renderLogs();
});

async function clearContainerLogs() {
  if (!currentLogContainerID) return;
  if (confirm("Bạn có chắc muốn Clear (Truncate) file log của container này về 0 byte không?")) {
    try {
      const res = await fetch(`/api/containers/logs?id=${currentLogContainerID}`, {
        method: "DELETE"
      });
      if (!res.ok) {
        const err = await res.json();
        alert(`Lỗi Clear Logs: ${err.error}`);
      } else {
        rawLogLines = ["[Log file has been truncated to 0 bytes]\n"];
        renderLogs();
      }
    } catch (e) {
      alert(`Lỗi hệ thống: ${e.message}`);
    }
  }
}
