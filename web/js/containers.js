let rawContainersData = [];
let containersStatsMap = {};
let containersStatsInterval = null;
let containerActionInProgress = new Set(); // Track containers with pending actions

let currentStatusFilter = "all";
let currentEngineFilter = "all";
let containersViewMode = localStorage.getItem("dockpulse_containers_view_mode") || "grid";
let activeRecreateTarget = null; // { type: 'container'|'compose_stack'|'compose_service', id, name, project, service, working_dir, config_file, image }

// Initialize view mode on load
document.addEventListener("DOMContentLoaded", () => {
  setContainersViewMode(containersViewMode, false);
});

function setContainersViewMode(mode, triggerRender = true) {
  containersViewMode = mode;
  localStorage.setItem("dockpulse_containers_view_mode", mode);

  const btnGrid = document.getElementById("btn-view-grid");
  const btnTable = document.getElementById("btn-view-table");
  const gridView = document.getElementById("containers-grid-view");
  const tableView = document.getElementById("containers-table-view");

  if (btnGrid && btnTable) {
    if (mode === "grid") {
      btnGrid.classList.add("active");
      btnTable.classList.remove("active");
      if (gridView) gridView.style.display = "grid";
      if (tableView) tableView.style.display = "none";
    } else {
      btnTable.classList.add("active");
      btnGrid.classList.remove("active");
      if (gridView) gridView.style.display = "none";
      if (tableView) tableView.style.display = "block";
    }
  }

  if (triggerRender && rawContainersData.length > 0) {
    applyContainerFilters();
  }
}

function setContainerStatusFilter(filter) {
  currentStatusFilter = filter;
  const pills = document.querySelectorAll("#container-status-pills .filter-pill");
  pills.forEach(p => {
    if (p.getAttribute("data-filter") === filter) {
      p.classList.add("active");
    } else {
      p.classList.remove("active");
    }
  });
  applyContainerFilters();
}

function clearContainerSearch() {
  const searchInput = document.getElementById("search-ctrs");
  const clearBtn = document.getElementById("clear-search-btn");
  if (searchInput) {
    searchInput.value = "";
    if (clearBtn) clearBtn.style.display = "none";
    applyContainerFilters();
  }
}

async function loadContainers() {
  const tbody = document.getElementById("tbody-containers");
  const grid = document.getElementById("containers-grid-view");
  try {
    const res = await fetch("/api/containers");
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    rawContainersData = await res.json();
    
    updateFilterCounts();
    applyContainerFilters();

    // Fetch stats immediately if auto-refresh is active
    fetchContainersStats();
  } catch (err) {
    if (tbody) {
      tbody.innerHTML = `<tr><td colspan="7" style="text-align:center; color: var(--accent-red);">Lỗi tải danh sách: ${err.message}</td></tr>`;
    }
    if (grid) {
      grid.innerHTML = `<div class="data-card" style="padding: 24px; text-align: center; color: var(--accent-red); grid-column: 1 / -1;">Lỗi tải danh sách: ${err.message}</div>`;
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
      applyContainerFilters();
    }
  } catch (e) {}
}

function updateFilterCounts() {
  let running = 0;
  let stopped = 0;
  let compose = 0;
  let standalone = 0;

  rawContainersData.forEach(c => {
    if (c.state === "running") running++;
    else stopped++;

    if (c.project && c.project.trim() !== "") compose++;
    else standalone++;
  });

  const countAll = document.getElementById("count-all");
  const countRunning = document.getElementById("count-running");
  const countStopped = document.getElementById("count-stopped");
  const countCompose = document.getElementById("count-compose");
  const countStandalone = document.getElementById("count-standalone");

  if (countAll) countAll.textContent = rawContainersData.length;
  if (countRunning) countRunning.textContent = running;
  if (countStopped) countStopped.textContent = stopped;
  if (countCompose) countCompose.textContent = compose;
  if (countStandalone) countStandalone.textContent = standalone;
}

function applyContainerFilters() {
  const searchInput = document.getElementById("search-ctrs");
  const query = searchInput ? searchInput.value.toLowerCase().trim() : "";
  const clearBtn = document.getElementById("clear-search-btn");
  if (clearBtn) {
    clearBtn.style.display = query ? "block" : "none";
  }

  const engineSelect = document.getElementById("filter-engine");
  currentEngineFilter = engineSelect ? engineSelect.value : "all";

  let filtered = rawContainersData.filter(c => {
    // 1. Status Filter
    if (currentStatusFilter === "running" && c.state !== "running") return false;
    if (currentStatusFilter === "stopped" && c.state === "running") return false;
    if (currentStatusFilter === "compose" && (!c.project || c.project.trim() === "")) return false;
    if (currentStatusFilter === "standalone" && c.project && c.project.trim() !== "") return false;

    // 2. Engine Filter
    if (currentEngineFilter !== "all" && c.engine !== currentEngineFilter) return false;

    // 3. Search Query
    if (query) {
      const matchName = c.name && c.name.toLowerCase().includes(query);
      const matchImage = c.image && c.image.toLowerCase().includes(query);
      const matchID = c.short_id && c.short_id.toLowerCase().includes(query);
      const matchProject = c.project && c.project.toLowerCase().includes(query);
      const matchPorts = c.ports && c.ports.some(p => p.public_port.toString().includes(query) || p.private_port.toString().includes(query));
      const matchIPs = c.ips && Object.values(c.ips).some(ip => ip.includes(query));
      if (!matchName && !matchImage && !matchID && !matchProject && !matchPorts && !matchIPs) {
        return false;
      }
    }

    return true;
  });

  renderContainers(filtered);
}

function renderContainers(list) {
  renderContainersGrid(list);
  renderContainersTable(list);
}

// -----------------------------------------------------------------------------
// 1. BENTO GRID / CARD VIEW RENDERING
// -----------------------------------------------------------------------------
function renderContainersGrid(list) {
  const grid = document.getElementById("containers-grid-view");
  if (!grid) return;

  if (!list || list.length === 0) {
    grid.innerHTML = `
      <div class="data-card" style="padding: 32px; text-align: center; color: var(--text-muted); grid-column: 1 / -1;">
        <div style="font-size: 2rem; margin-bottom: 8px;">🔍</div>
        <div style="font-weight: 600; font-size: 1.05rem;">Không tìm thấy container nào phù hợp</div>
        <div style="font-size: 0.85rem; margin-top: 4px;">Hãy thử đổi từ khóa tìm kiếm hoặc bấm tab "Tất cả".</div>
      </div>
    `;
    return;
  }

  const hostName = window.location.hostname || "localhost";

  grid.innerHTML = list.map(c => {
    const isRunning = (c.state === "running");
    const isPaused = (c.state === "paused");
    const isStopped = !isRunning && !isPaused;
    const isBusy = containerActionInProgress.has(c.id);

    let stateClass = "badge-stopped";
    let stateIcon = "🔴";
    if (isRunning) { stateClass = "badge-running"; stateIcon = "🟢"; }
    else if (isPaused) { stateClass = "badge-paused"; stateIcon = "⏸️"; }

    const engineBadge = c.engine === 'podman'
      ? `<span style="display:inline-flex; align-items:center; gap:3px; padding:1px 6px; font-size:0.7rem; font-weight:600; border-radius:4px; background:rgba(192,132,252,0.15); color:#c084fc; border:1px solid rgba(192,132,252,0.3);">🦭 Podman</span>`
      : `<span style="display:inline-flex; align-items:center; gap:3px; padding:1px 6px; font-size:0.7rem; font-weight:600; border-radius:4px; background:rgba(56,189,248,0.15); color:#38bdf8; border:1px solid rgba(56,189,248,0.3);">🐳 Docker</span>`;

    // Realtime Metrics
    let cpuText = "0.0%";
    let memText = "0 MB (0%)";
    let netText = "📥 0 B | 📤 0 B";
    if (isRunning) {
      const st = containersStatsMap[c.id];
      if (st) {
        cpuText = `${st.cpu_percent ? st.cpu_percent.toFixed(1) : "0.0"}%`;
        const memMB = st.mem_usage_mb ? formatMBHelperCtr(st.mem_usage_mb) : "0 MB";
        const memPct = st.mem_percent ? st.mem_percent.toFixed(1) : "0.0";
        memText = `${memMB} (${memPct}%)`;
        const rxMB = st.net_rx_mb ? formatMBHelperCtr(st.net_rx_mb) : "0 B";
        const txMB = st.net_tx_mb ? formatMBHelperCtr(st.net_tx_mb) : "0 B";
        netText = `📥 ${rxMB} | 📤 ${txMB}`;
      } else {
        cpuText = "Đang đo...";
        memText = "Đang đo...";
        netText = "Đang đo...";
      }
    } else {
      cpuText = "Off";
      memText = "Off";
      netText = "Off";
    }

    // Ports
    let portsHTML = `<span style="font-size: 0.78rem; color: var(--text-muted);">Không có port public</span>`;
    if (c.ports && c.ports.length > 0) {
      portsHTML = c.ports.map(p => {
        if (p.public_port > 0) {
          const url = `http://${hostName}:${p.public_port}`;
          return `<a href="${url}" target="_blank" class="port-link" title="Mở cổng trên trình duyệt">🔗 ${p.public_port}:${p.private_port}</a>`;
        }
        return `<span style="font-family: monospace; font-size: 0.75rem; color: var(--text-muted); background: rgba(255,255,255,0.03); padding: 1px 5px; border-radius: 3px;">${p.private_port}/${p.type}</span>`;
      }).join(" ");
    }

    // Action button states
    const startDisabled = isRunning || isBusy;
    const stopDisabled = isStopped || isBusy;
    const restartDisabled = isStopped || isBusy;
    const killDisabled = isStopped || isBusy;

    const startAction = isPaused ? 'unpause' : 'start';
    const onClickStart = startDisabled ? '' : `containerAction('${c.id}','${startAction}')`;
    const onClickStop = stopDisabled ? '' : `containerAction('${c.id}','stop')`;
    const onClickRestart = restartDisabled ? '' : `containerAction('${c.id}','restart')`;
    const onClickKill = killDisabled ? '' : `confirmKillContainer('${c.id}','${escapeHTML(c.name)}')`;

    const loadingIndicator = isBusy ? `<span class="ctr-action-loading" title="Đang xử lý..." style="font-size:0.85rem;">⏳ Đang thực thi...</span>` : "";

    return `
      <div class="container-card ${isStopped ? 'card-stopped' : ''}">
        <!-- Card Header -->
        <div class="container-card-header">
          <div>
            <div class="container-card-title">
              <span>${escapeHTML(c.name)}</span>
              ${engineBadge}
            </div>
            <div class="container-card-id">ID: ${c.short_id}</div>
          </div>
          <span class="badge ${stateClass}">${stateIcon} ${c.state.toUpperCase()}</span>
        </div>

        <!-- Project Tag if part of Compose -->
        ${c.project ? `
          <div style="font-size: 0.78rem; color: var(--accent-blue); display: flex; align-items: center; gap: 4px;">
            <span>🧩 Stack:</span> <strong>${escapeHTML(c.project)}</strong>
            ${c.service ? `<span style="color: var(--text-muted); font-size: 0.72rem;">(service: ${escapeHTML(c.service)})</span>` : ''}
          </div>
        ` : `
          <div style="font-size: 0.76rem; color: var(--text-muted);">
            📦 Container độc lập (Standalone)
          </div>
        `}

        <!-- Image info -->
        <div class="container-card-image" title="${escapeHTML(c.image)}">
          🏷️ ${escapeHTML(c.image)}
        </div>

        <!-- Realtime Resource Metrics Chips -->
        <div class="container-card-metrics">
          <div class="card-metric-col">
            <span class="card-metric-label">⚡ CPU</span>
            <span class="card-metric-val" style="color: #38bdf8;">${cpuText}</span>
          </div>
          <div class="card-metric-col">
            <span class="card-metric-label">🧠 RAM</span>
            <span class="card-metric-val" style="color: #818cf8;">${memText}</span>
          </div>
          <div class="card-metric-col">
            <span class="card-metric-label">🌐 NET</span>
            <span class="card-metric-val" style="color: #a855f7; font-size: 0.72rem;">${netText}</span>
          </div>
        </div>

        <!-- Ports list -->
        <div class="container-card-ports">
          ${portsHTML}
        </div>

        <!-- Card Action Footer -->
        <div class="container-card-footer">
          <div class="card-actions-group" style="display: flex; gap: 6px; align-items: center;">
            <!-- ⚡ Recreate Button -->
            <button class="btn-icon btn-recreate" onclick="openRecreateModalForContainer('${c.id}')" title="⚡ Tái tạo Container (Recreate với image & cấu hình mới nhất)">⚡</button>

            ${loadingIndicator}

            <!-- ▶️ Start / ⏹️ Stop Toggle -->
            ${isRunning ? `
              <button class="btn-icon ctr-btn-stop" onclick="${onClickStop}" title="Dừng container (Stop)" ${stopDisabled ? 'disabled' : ''}>⏹️</button>
            ` : `
              <button class="btn-icon ctr-btn-start" onclick="${onClickStart}" title="Khởi động container (Start)" ${startDisabled ? 'disabled' : ''}>▶️</button>
            `}

            <!-- 🔄 Restart -->
            <button class="btn-icon ctr-btn-restart" onclick="${onClickRestart}" title="Khởi động lại (Restart)" ${restartDisabled ? 'disabled' : ''}>🔄</button>

            <!-- 📋 Logs -->
            <button class="btn-icon" onclick="openLogsModal('${c.id}', '${escapeHTML(c.name)}')" title="Xem Live Logs">📋</button>

            <!-- 💻 Terminal (if running) -->
            ${isRunning ? `
              <button class="btn-icon" onclick="openTerminalModal('${c.id}', '${escapeHTML(c.name)}')" title="Mở Web Terminal Shell">💻</button>
            ` : ''}
          </div>

          <div style="display: flex; gap: 6px; align-items: center;">
            <!-- 🤖 AI Diagnose -->
            <button class="btn-icon" style="color: var(--accent-blue);" onclick="diagnoseContainerWithAI('${c.id}', '${escapeHTML(c.name)}')" title="AI Phân tích lỗi">🤖</button>

            <!-- 🗑️ Delete -->
            <button class="btn-icon" style="color: var(--accent-red);" onclick="removeContainerPrompt('${c.id}', '${escapeHTML(c.name)}')" title="Xóa Container">🗑️</button>
          </div>
        </div>
      </div>
    `;
  }).join("");
}

// -----------------------------------------------------------------------------
// 2. COMPACT TABLE VIEW RENDERING
// -----------------------------------------------------------------------------
function renderContainersTable(list) {
  const tbody = document.getElementById("tbody-containers");
  if (!tbody) return;

  if (!list || list.length === 0) {
    tbody.innerHTML = `<tr><td colspan="7" style="text-align:center; color: var(--text-muted); padding: 24px;">Không tìm thấy container nào phù hợp.</td></tr>`;
    return;
  }

  const hostName = window.location.hostname || "localhost";

  tbody.innerHTML = list.map(c => {
    let stateClass = "badge-stopped";
    let stateIcon = "🔴";
    if (c.state === "running") { stateClass = "badge-running"; stateIcon = "🟢"; }
    else if (c.state === "paused") { stateClass = "badge-paused"; stateIcon = "⏸️"; }

    let statsHTML = `<span style="color: var(--text-muted); font-size: 0.8rem;">Off</span>`;
    if (c.state === "running") {
      const st = containersStatsMap[c.id];
      if (st) {
        const cpuPct = st.cpu_percent ? st.cpu_percent.toFixed(1) : "0.0";
        const memMB = st.mem_usage_mb ? formatMBHelperCtr(st.mem_usage_mb) : "0 MB";
        const memPct = st.mem_percent ? st.mem_percent.toFixed(1) : "0.0";
        const rxMB = st.net_rx_mb ? formatMBHelperCtr(st.net_rx_mb) : "0 B";
        const txMB = st.net_tx_mb ? formatMBHelperCtr(st.net_tx_mb) : "0 B";

        statsHTML = `
          <div style="font-size: 0.78rem; font-family: monospace; line-height: 1.35;">
            <div><span style="color: #38bdf8; font-weight: 600;">⚡ CPU:</span> ${cpuPct}%</div>
            <div><span style="color: #818cf8; font-weight: 600;">🧠 RAM:</span> ${memMB} (${memPct}%)</div>
            <div><span style="color: #a855f7; font-weight: 600;">🌐 NET:</span> 📥 ${rxMB} | 📤 ${txMB}</div>
          </div>
        `;
      } else {
        statsHTML = `<span style="color: var(--text-muted); font-size: 0.8rem; font-family: monospace;">⏳ Đang đo...</span>`;
      }
    }

    let portsHTML = `<span style="color: var(--text-muted); font-size: 0.8rem;">None</span>`;
    if (c.ports && c.ports.length > 0) {
      portsHTML = c.ports.map(p => {
        if (p.public_port > 0) {
          const url = `http://${hostName}:${p.public_port}`;
          return `<a href="${url}" target="_blank" class="port-link">🔗 ${p.public_port}:${p.private_port}</a>`;
        }
        return `<span style="font-family: monospace; font-size: 0.75rem; color: var(--text-muted);">${p.private_port}/${p.type}</span>`;
      }).join(" ");
    }

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

    const startDisabled   = isRunning  || isBusy;
    const stopDisabled    = isStopped  || isBusy;
    const restartDisabled = isStopped  || isBusy;
    const killDisabled    = isStopped  || isBusy;

    const startAction  = isPaused ? 'unpause' : 'start';
    const onClickStart   = startDisabled   ? '' : `containerAction('${c.id}','${startAction}')`;
    const onClickStop    = stopDisabled    ? '' : `containerAction('${c.id}','stop')`;
    const onClickRestart = restartDisabled ? '' : `containerAction('${c.id}','restart')`;
    const onClickKill    = killDisabled    ? '' : `confirmKillContainer('${c.id}','${escapeHTML(c.name)}')`;

    const engineBadge = c.engine === 'podman' 
      ? `<span style="display:inline-block; padding:1px 6px; font-size:0.7rem; font-weight:600; border-radius:4px; background:rgba(192,132,252,0.15); color:#c084fc; border:1px solid rgba(192,132,252,0.3); margin-left:6px;">🦭 Podman</span>`
      : `<span style="display:inline-block; padding:1px 6px; font-size:0.7rem; font-weight:600; border-radius:4px; background:rgba(56,189,248,0.15); color:#38bdf8; border:1px solid rgba(56,189,248,0.3); margin-left:6px;">🐳 Docker</span>`;

    return `
      <tr>
        <td>
          <div style="display: flex; align-items: center; font-weight: 700; color: var(--text-main); font-size: 0.95rem;">
            ${escapeHTML(c.name)} ${engineBadge}
          </div>
          <div style="font-family: monospace; color: var(--text-muted); font-size: 0.75rem;">${c.short_id}</div>
          ${c.project ? `<div style="font-size: 0.75rem; color: var(--accent-blue); margin-top: 2px;">🧩 ${escapeHTML(c.project)}</div>` : ''}
        </td>
        <td>
          <span class="badge ${stateClass}">${stateIcon} ${c.state.toUpperCase()}</span>
        </td>
        <td>
          <div style="max-width: 170px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 0.82rem; font-family: monospace;" title="${escapeHTML(c.image)}">
            ${escapeHTML(c.image)}
          </div>
        </td>
        <td>${statsHTML}</td>
        <td>${portsHTML}</td>
        <td>${ipsHTML}</td>
        <td>
          <div class="action-btns" style="display: flex; gap: 4px; align-items: center; flex-wrap: wrap;">
            <!-- ⚡ Recreate Button -->
            <button class="btn-icon btn-recreate" onclick="openRecreateModalForContainer('${c.id}')" title="⚡ Tái tạo Container (Recreate với image & cấu hình mới nhất)">
              ⚡
            </button>

            <!-- ▶️ Start / ⏹️ Stop Toggle -->
            ${isRunning ? `
              <button class="btn-icon ctr-btn-stop" onclick="${onClickStop}" title="Dừng container (Stop)" ${stopDisabled ? 'disabled' : ''}>⏹️</button>
            ` : `
              <button class="btn-icon ctr-btn-start" onclick="${onClickStart}" title="Khởi động container (Start)" ${startDisabled ? 'disabled' : ''}>▶️</button>
            `}

            <!-- 🔄 Restart -->
            <button class="btn-icon ctr-btn-restart" onclick="${onClickRestart}" title="Khởi động lại (Restart)" ${restartDisabled ? 'disabled' : ''}>🔄</button>

            <!-- 📋 Logs -->
            <button class="btn-icon" onclick="openLogsModal('${c.id}', '${escapeHTML(c.name)}')" title="Xem Live Logs">📋</button>

            <!-- 💻 Terminal -->
            ${isRunning ? `<button class="btn-icon" onclick="openTerminalModal('${c.id}', '${escapeHTML(c.name)}')" title="Terminal Shell">💻</button>` : ''}

            <!-- 🤖 AI Diagnose -->
            <button class="btn-icon" style="color: var(--accent-blue);" onclick="diagnoseContainerWithAI('${c.id}', '${escapeHTML(c.name)}')" title="AI Diagnose">🤖</button>

            <!-- 🗑️ Delete -->
            <button class="btn-icon" style="color: var(--accent-red);" onclick="removeContainerPrompt('${c.id}', '${escapeHTML(c.name)}')" title="Xóa Container">🗑️</button>
          </div>
        </td>
      </tr>
    `;
  }).join("");
}

// -----------------------------------------------------------------------------
// 3. RECREATE MODAL & EXECUTION
// -----------------------------------------------------------------------------
function openRecreateModalForContainer(containerId) {
  const ctr = rawContainersData.find(c => c.id === containerId || c.short_id === containerId);
  if (!ctr) return;

  activeRecreateTarget = {
    type: "container",
    id: ctr.id,
    name: ctr.name,
    image: ctr.image,
    project: ctr.project || "",
    service: ctr.service || "",
    working_dir: ctr.working_dir || "",
    config_file: ctr.config_file || "",
    engine: ctr.engine || "docker",
  };

  const titleEl = document.getElementById("recreate-modal-title");
  const infoEl = document.getElementById("recreate-target-info");
  const buildWrap = document.getElementById("recreate-opt-build-wrap");
  const outputWrap = document.getElementById("recreate-output-wrap");
  const logPre = document.getElementById("recreate-log-output");
  const btnRun = document.getElementById("btn-run-recreate");

  if (titleEl) {
    titleEl.innerHTML = `<span>🔄⚡</span> <span>Tái Lập Trình Container: <strong>${escapeHTML(ctr.name)}</strong></span>`;
  }

  if (buildWrap) {
    // Show build option only if part of compose project
    buildWrap.style.display = ctr.project ? "flex" : "none";
  }

  if (infoEl) {
    infoEl.innerHTML = `
      <div style="display: grid; grid-template-columns: auto 1fr; gap: 6px 14px; font-size: 0.88rem;">
        <span style="color: var(--text-muted);">Container Name:</span>
        <strong style="color: #38bdf8;">${escapeHTML(ctr.name)}</strong>

        <span style="color: var(--text-muted);">Image Tag:</span>
        <code style="font-size: 0.82rem; color: #a5b4fc;">${escapeHTML(ctr.image)}</code>

        <span style="color: var(--text-muted);">Phân loại:</span>
        <div>
          ${ctr.project ? `
            <span class="badge badge-paused" style="font-size:0.75rem;">🧩 Compose Stack: ${escapeHTML(ctr.project)} (service: ${escapeHTML(ctr.service || ctr.name)})</span>
          ` : `
            <span class="badge" style="background: rgba(255,255,255,0.06); font-size:0.75rem;">📦 Container độc lập</span>
          `}
        </div>

        ${ctr.working_dir ? `
          <span style="color: var(--text-muted);">Thư mục Compose:</span>
          <code style="font-size: 0.78rem; color: var(--text-secondary);">${escapeHTML(ctr.working_dir)}</code>
        ` : ''}
      </div>
    `;
  }

  if (outputWrap) outputWrap.style.display = "none";
  if (logPre) logPre.textContent = "";
  if (btnRun) {
    btnRun.disabled = false;
    btnRun.innerHTML = "🚀 Bắt Đầu Recreate";
  }

  const modal = document.getElementById("modal-recreate");
  if (modal) modal.classList.add("active");
}

function openRecreateModalForCompose(project, service = "", workingDir = "", configFile = "") {
  activeRecreateTarget = {
    type: service ? "compose_service" : "compose_stack",
    project: project,
    service: service,
    working_dir: workingDir,
    config_file: configFile,
  };

  const titleEl = document.getElementById("recreate-modal-title");
  const infoEl = document.getElementById("recreate-target-info");
  const buildWrap = document.getElementById("recreate-opt-build-wrap");
  const outputWrap = document.getElementById("recreate-output-wrap");
  const logPre = document.getElementById("recreate-log-output");
  const btnRun = document.getElementById("btn-run-recreate");

  if (titleEl) {
    if (service) {
      titleEl.innerHTML = `<span>🔄⚡</span> <span>Recreate Service: <strong>${escapeHTML(service)}</strong> (Stack: ${escapeHTML(project)})</span>`;
    } else {
      titleEl.innerHTML = `<span>🔄⚡</span> <span>Recreate Toàn Bộ Stack: <strong>${escapeHTML(project)}</strong></span>`;
    }
  }

  if (buildWrap) buildWrap.style.display = "flex";

  if (infoEl) {
    infoEl.innerHTML = `
      <div style="display: grid; grid-template-columns: auto 1fr; gap: 6px 14px; font-size: 0.88rem;">
        <span style="color: var(--text-muted);">Compose Project:</span>
        <strong style="color: #38bdf8;">${escapeHTML(project)}</strong>

        ${service ? `
          <span style="color: var(--text-muted);">Service Recreate:</span>
          <strong style="color: #818cf8;">${escapeHTML(service)}</strong>
        ` : `
          <span style="color: var(--text-muted);">Phạm vi:</span>
          <strong>Toàn bộ các container trong Stack</strong>
        `}

        ${workingDir ? `
          <span style="color: var(--text-muted);">Working Dir:</span>
          <code style="font-size: 0.8rem; color: var(--text-secondary);">${escapeHTML(workingDir)}</code>
        ` : ''}
      </div>
    `;
  }

  if (outputWrap) outputWrap.style.display = "none";
  if (logPre) logPre.textContent = "";
  if (btnRun) {
    btnRun.disabled = false;
    btnRun.innerHTML = "🚀 Bắt Đầu Recreate";
  }

  const modal = document.getElementById("modal-recreate");
  if (modal) modal.classList.add("active");
}

function closeRecreateModal() {
  const modal = document.getElementById("modal-recreate");
  if (modal) modal.classList.remove("active");
  activeRecreateTarget = null;
}

async function executeRecreateAction() {
  if (!activeRecreateTarget) return;

  const btnRun = document.getElementById("btn-run-recreate");
  const btnCancel = document.getElementById("btn-cancel-recreate");
  const outputWrap = document.getElementById("recreate-output-wrap");
  const logPre = document.getElementById("recreate-log-output");
  const statusBadge = document.getElementById("recreate-status-badge");

  const pullOpt = document.getElementById("recreate-opt-pull")?.checked ?? true;
  const buildOpt = document.getElementById("recreate-opt-build")?.checked ?? false;

  if (btnRun) {
    btnRun.disabled = true;
    btnRun.innerHTML = "⏳ Đang xử lý...";
  }
  if (btnCancel) btnCancel.disabled = true;

  if (outputWrap) outputWrap.style.display = "block";
  if (statusBadge) {
    statusBadge.className = "badge badge-paused";
    statusBadge.textContent = "Đang thực thi...";
  }
  if (logPre) {
    logPre.textContent = "⏳ Đang kết nối và chuẩn bị recreate...\n";
  }

  try {
    let apiUrl = "";
    let payload = {};

    if (activeRecreateTarget.type === "container") {
      apiUrl = "/api/containers/recreate";
      payload = {
        id: activeRecreateTarget.id,
        pull: pullOpt,
      };
    } else {
      apiUrl = "/api/compose/recreate";
      payload = {
        project: activeRecreateTarget.project,
        service: activeRecreateTarget.service || "",
        working_dir: activeRecreateTarget.working_dir || "",
        config_file: activeRecreateTarget.config_file || "",
        pull: pullOpt,
        build: buildOpt,
      };
    }

    const res = await fetch(apiUrl, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });

    const data = await res.json();

    if (!res.ok) {
      if (statusBadge) {
        statusBadge.className = "badge badge-stopped";
        statusBadge.textContent = "❌ Lỗi";
      }
      if (logPre) {
        logPre.textContent = (data.output || "") + `\n\n❌ Lỗi: ${data.error || "Không xác định"}`;
      }
      showContainerToast(`❌ Recreate thất bại: ${data.error}`, "error");
    } else {
      if (statusBadge) {
        statusBadge.className = "badge badge-running";
        statusBadge.textContent = "✅ Hoàn tất";
      }
      if (logPre) {
        logPre.textContent = data.output || "✅ Recreate thành công!";
      }
      showContainerToast("✅ Recreate thành công! Danh sách đang được làm mới.", "success");

      // Auto-reload data
      setTimeout(() => {
        loadContainers();
        if (typeof loadComposeStacks === "function") {
          loadComposeStacks();
        }
      }, 1500);
    }
  } catch (err) {
    if (statusBadge) {
      statusBadge.className = "badge badge-stopped";
      statusBadge.textContent = "❌ Lỗi kết nối";
    }
    if (logPre) {
      logPre.textContent += `\n❌ Lỗi mạng / hệ thống: ${err.message}`;
    }
    showContainerToast(`❌ Lỗi: ${err.message}`, "error");
  } finally {
    if (btnRun) {
      btnRun.disabled = false;
      btnRun.innerHTML = "Đóng";
      btnRun.onclick = closeRecreateModal;
    }
    if (btnCancel) btnCancel.disabled = false;
  }
}

// -----------------------------------------------------------------------------
// 4. ACTION HELPERS & UTILITIES
// -----------------------------------------------------------------------------
function formatMBHelperCtr(mb) {
  if (!mb || isNaN(mb) || mb === 0) return "0 MB";
  if (typeof formatBytes === "function") {
    return formatBytes(mb * 1024 * 1024, 1);
  }
  if (mb >= 1024) return (mb / 1024).toFixed(1) + " GB";
  return mb.toFixed(1) + " MB";
}

// Search Filter Listener
document.getElementById("search-ctrs")?.addEventListener("input", () => {
  applyContainerFilters();
});

function showContainerToast(msg, type = "info") {
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

const ACTION_LABELS = { start: "Start", stop: "Stop", restart: "Restart", pause: "Pause", unpause: "Unpause", kill: "Kill" };

async function confirmKillContainer(id, name) {
  if (confirm(`Bạn có chắc chắn muốn Kill (buộc dừng khẩn cấp bằng SIGKILL) container "${name}" không?\n\nLưu ý: Hành động này sẽ dừng ngay lập tức tiến trình của container mà không chờ tiến trình lưu dữ liệu.`)) {
    await containerAction(id, "kill");
  }
}

async function containerAction(id, action) {
  if (containerActionInProgress.has(id)) return;
  containerActionInProgress.add(id);
  applyContainerFilters(); // re-render to show loading state

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
