let rawContainersData = [];
let containersStatsMap = {};
let containersStatsInterval = null;
let containerActionInProgress = new Set(); // Track containers with pending actions

async function loadContainers() {
  const tbody = document.getElementById("tbody-containers");
  try {
    const res = await fetch("/api/containers");
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    rawContainersData = await res.json();
    renderContainers(rawContainersData);

    // Fetch stats immediately if auto-refresh is active
    fetchContainersStats();
  } catch (err) {
    if (tbody) {
      tbody.innerHTML = `<tr><td colspan="7" style="text-align:center; color: var(--accent-red);">Lỗi tải danh sách: ${err.message}</td></tr>`;
    }
  }
}

async function fetchContainersStats() {
  const chk = document.getElementById("auto-refresh-ctrs-stats");
  const secCtrs = document.getElementById("sec-containers");
  if (chk && !chk.checked) return;
  if (secCtrs && !secCtrs.classList.contains("active")) return;

  try {
    const res = await fetch("/api/containers/stats/all");
    if (res.ok) {
      containersStatsMap = await res.json();
      renderContainers(rawContainersData);
    }
  } catch (e) {}
}

function renderContainers(list) {
  const tbody = document.getElementById("tbody-containers");
  if (!tbody) return;

  if (!list || list.length === 0) {
    tbody.innerHTML = `<tr><td colspan="7" style="text-align:center; color: var(--text-muted);">Không tìm thấy container nào trên server.</td></tr>`;
    return;
  }

  const hostName = window.location.hostname || "localhost";

  tbody.innerHTML = list.map(c => {
    // State badge
    let stateClass = "badge-stopped";
    let stateIcon = "🔴";
    if (c.state === "running") { stateClass = "badge-running"; stateIcon = "🟢"; }
    else if (c.state === "paused") { stateClass = "badge-paused"; stateIcon = "⏸️"; }

    // Stats HTML
    let statsHTML = `<span style="color: var(--text-muted); font-size: 0.8rem;">Off / Stopped</span>`;
    if (c.state === "running") {
      const st = containersStatsMap[c.id];
      if (st) {
        const cpuPct = st.cpu_percent ? st.cpu_percent.toFixed(1) : "0.0";
        const memMB = st.mem_usage_mb ? formatMBHelperCtr(st.mem_usage_mb) : "0 MB";
        const memPct = st.mem_percent ? st.mem_percent.toFixed(1) : "0.0";
        const rxMB = st.net_rx_mb ? formatMBHelperCtr(st.net_rx_mb) : "0 B";
        const txMB = st.net_tx_mb ? formatMBHelperCtr(st.net_tx_mb) : "0 B";

        statsHTML = `
          <div style="font-size: 0.8rem; font-family: monospace; line-height: 1.4;">
            <div><span style="color: #38bdf8; font-weight: 600;">⚡ CPU:</span> ${cpuPct}%</div>
            <div><span style="color: #818cf8; font-weight: 600;">🧠 RAM:</span> ${memMB} (${memPct}%)</div>
            <div><span style="color: #a855f7; font-weight: 600;">🌐 NET:</span> 📥 ${rxMB} | 📤 ${txMB}</div>
          </div>
        `;
      } else {
        statsHTML = `<span style="color: var(--text-muted); font-size: 0.8rem; font-family: monospace;">⏳ Đang đo...</span>`;
      }
    }

    // Ports HTML
    let portsHTML = `<span style="color: var(--text-muted); font-size: 0.8rem;">None</span>`;
    if (c.ports && c.ports.length > 0) {
      portsHTML = c.ports.map(p => {
        if (p.public_port > 0) {
          const url = `http://${hostName}:${p.public_port}`;
          return `<a href="${url}" target="_blank" class="port-link">🔗 ${p.public_port}:${p.private_port}</a>`;
        }
        return `<span style="font-family: monospace; font-size: 0.8rem; color: var(--text-muted);">${p.private_port}/${p.type}</span>`;
      }).join(" ");
    }

    // IPs HTML
    let ipsHTML = `<span style="color: var(--text-muted); font-size: 0.8rem;">None</span>`;
    if (c.ips && Object.keys(c.ips).length > 0) {
      ipsHTML = Object.entries(c.ips).map(([net, ip]) => {
        return `<span class="ip-badge" title="Network: ${net}">${net}: ${ip}</span>`;
      }).join(" ");
    }

    const isRunning = (c.state === "running");
    const isPaused  = (c.state === "paused");
    const isStopped = !isRunning && !isPaused;
    const isBusy    = containerActionInProgress.has(c.id);

    // ---- Independent action buttons always visible ----
    // Start: enabled only when stopped or paused
    const startDisabled  = isRunning  || isBusy;
    const stopDisabled   = isStopped  || isBusy;
    const restartDisabled = isStopped || isBusy;

    const startTitle   = isRunning ? "Container đang chạy" : "Start Container";
    const stopTitle    = isStopped ? "Container đã dừng"   : "Stop Container";
    const restartTitle = isStopped ? "Container đã dừng"   : "Restart Container";

    const loadingSpan  = isBusy ? `<span class="ctr-action-loading" title="Đang xử lý...">⏳</span>` : "";

    const startAction  = isPaused ? 'unpause' : 'start';
    const onClickStart   = startDisabled  ? '' : `containerAction('${c.id}','${startAction}')`;
    const onClickStop    = stopDisabled   ? '' : `containerAction('${c.id}','stop')`;
    const onClickRestart = restartDisabled ? '' : `containerAction('${c.id}','restart')`;

    return `
      <tr>
        <td>
          <div style="font-weight: 700; color: var(--text-main); font-size: 0.95rem;">${escapeHTML(c.name)}</div>
          <div style="font-family: monospace; color: var(--text-muted); font-size: 0.75rem;">${c.short_id}</div>
          ${c.project ? `<div style="font-size: 0.75rem; color: var(--accent-blue); margin-top: 2px;">🧩 ${escapeHTML(c.project)}</div>` : ''}
        </td>
        <td>
          <span class="badge ${stateClass}">${stateIcon} ${c.state.toUpperCase()}</span>
        </td>
        <td>
          <div style="max-width: 180px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 0.85rem;" title="${escapeHTML(c.image)}">
            ${escapeHTML(c.image)}
          </div>
        </td>
        <td>${statsHTML}</td>
        <td>${portsHTML}</td>
        <td>${ipsHTML}</td>
        <td>
          <div class="action-btns">
            ${loadingSpan}

            <!-- ▶️ Start – luôn hiển thị -->
            <button class="btn-icon ctr-btn-start${startDisabled ? ' ctr-btn-disabled' : ''}"
              onclick="${onClickStart}"
              title="${startTitle}"
              ${startDisabled ? 'disabled' : ''}>▶️</button>

            <!-- ⏹️ Stop – luôn hiển thị -->
            <button class="btn-icon ctr-btn-stop${stopDisabled ? ' ctr-btn-disabled' : ''}"
              onclick="${onClickStop}"
              title="${stopTitle}"
              ${stopDisabled ? 'disabled' : ''}>⏹️</button>

            <!-- 🔄 Restart – luôn hiển thị -->
            <button class="btn-icon ctr-btn-restart${restartDisabled ? ' ctr-btn-disabled' : ''}"
              onclick="${onClickRestart}"
              title="${restartTitle}"
              ${restartDisabled ? 'disabled' : ''}>🔄</button>

            <!-- 💻 Terminal: chỉ khi đang running -->
            ${isRunning ? `<button class="btn-icon" onclick="openTerminalModal('${c.id}', '${escapeHTML(c.name)}')" title="Terminal Shell">💻</button>` : ''}

            <button class="btn-icon" onclick="openLogsModal('${c.id}', '${escapeHTML(c.name)}')" title="View Live Logs">📋</button>
            <button class="btn-icon" style="color: var(--accent-blue);" onclick="diagnoseContainerWithAI('${c.id}', '${escapeHTML(c.name)}')" title="AI Phân tích sự cố">🤖</button>
            <button class="btn-icon" style="color: var(--accent-red);" onclick="removeContainerPrompt('${c.id}', '${escapeHTML(c.name)}')" title="Remove Container">🗑️</button>
          </div>
        </td>
      </tr>
    `;
  }).join("");
}

function formatMBHelperCtr(mb) {
  if (!mb || isNaN(mb) || mb === 0) return "0 MB";
  if (typeof formatBytes === "function") {
    return formatBytes(mb * 1024 * 1024, 1);
  }
  if (mb >= 1024) return (mb / 1024).toFixed(1) + " GB";
  return mb.toFixed(1) + " MB";
}

// Search Filter
document.getElementById("search-ctrs")?.addEventListener("input", (e) => {
  const query = e.target.value.toLowerCase().trim();
  if (!query) {
    renderContainers(rawContainersData);
    return;
  }
  const filtered = rawContainersData.filter(c => {
    return c.name.toLowerCase().includes(query) ||
           c.image.toLowerCase().includes(query) ||
           c.short_id.toLowerCase().includes(query) ||
           (c.project && c.project.toLowerCase().includes(query)) ||
           (c.ports && c.ports.some(p => p.public_port.toString().includes(query))) ||
           (c.ips && Object.values(c.ips).some(ip => ip.includes(query)));
  });
  renderContainers(filtered);
});

function showContainerToast(msg, type = "info") {
  // Reuse global toast if available, otherwise show simple notification
  if (typeof showToast === "function") {
    showToast(msg, type);
    return;
  }
  const toast = document.createElement("div");
  toast.textContent = msg;
  const colors = { success: "#22c55e", error: "#ef4444", info: "#38bdf8", warn: "#f59e0b" };
  Object.assign(toast.style, {
    position: "fixed", bottom: "24px", right: "24px", zIndex: 9999,
    background: colors[type] || colors.info,
    color: "#fff", padding: "10px 18px", borderRadius: "10px",
    fontSize: "0.9rem", fontWeight: "600", boxShadow: "0 4px 20px rgba(0,0,0,0.35)",
    transition: "opacity 0.4s", opacity: "1",
  });
  document.body.appendChild(toast);
  setTimeout(() => { toast.style.opacity = "0"; setTimeout(() => toast.remove(), 450); }, 3000);
}

const ACTION_LABELS = { start: "Start", stop: "Stop", restart: "Restart", pause: "Pause", unpause: "Unpause" };

async function containerAction(id, action) {
  if (containerActionInProgress.has(id)) return;
  containerActionInProgress.add(id);
  renderContainers(rawContainersData); // re-render to show loading state

  const label = ACTION_LABELS[action] || action;
  try {
    const res = await fetch("/api/containers/action", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ id, action })
    });
    if (!res.ok) {
      const err = await res.json();
      showContainerToast(`❌ ${label} thất bại: ${err.error}`, "error");
    } else {
      showContainerToast(`✅ ${label} thành công!`, "success");
    }
  } catch (e) {
    showContainerToast(`❌ Lỗi hệ thống: ${e.message}`, "error");
  } finally {
    containerActionInProgress.delete(id);
    await loadContainers();
  }
}

async function removeContainerPrompt(id, name) {
  if (confirm(`Bạn có chắc chắn muốn xóa container "${name}" không?`)) {
    try {
      const res = await fetch("/api/containers/remove", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ id, force: true })
      });
      if (!res.ok) {
        const err = await res.json();
        showContainerToast(`❌ Xóa container thất bại: ${err.error}`, "error");
      } else {
        showContainerToast(`✅ Đã xóa container "${name}"`, "success");
        loadContainers();
      }
    } catch (e) {
      showContainerToast(`❌ Lỗi hệ thống: ${e.message}`, "error");
    }
  }
}

function escapeHTML(str) {
  if (!str) return '';
  return str.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
}

// Setup Auto-Refresh Interval for Containers Stats
if (!containersStatsInterval) {
  containersStatsInterval = setInterval(() => {
    fetchContainersStats();
  }, 4000);
}
