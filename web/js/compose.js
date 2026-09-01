let composeAutoRefreshInterval = null;
let currentComposeStacks = [];
const SAVED_COMPOSE_SORT_KEY = "dockpulse_compose_sort";

function getComposeSortMode() {
  const sortSelect = document.getElementById("compose-sort");
  if (sortSelect) {
    return sortSelect.value || "cpu";
  }
  return localStorage.getItem(SAVED_COMPOSE_SORT_KEY) || "cpu";
}

function syncComposeSortDropdown() {
  const sortSelect = document.getElementById("compose-sort");
  if (sortSelect) {
    const saved = localStorage.getItem(SAVED_COMPOSE_SORT_KEY);
    if (saved && sortSelect.value !== saved) {
      sortSelect.value = saved;
    }
  }
}

async function loadComposeStacks(isAuto = false) {
  const container = document.getElementById("compose-stacks-container");
  syncComposeSortDropdown();
  try {
    const res = await fetch("/api/compose?stats=true");
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    currentComposeStacks = await res.json();

    // If this is an auto-refresh tick and DOM already exists, update stats smoothly in-place
    if (isAuto && updateComposeStatsInPlace(currentComposeStacks)) {
      return;
    }

    renderComposeStacks(currentComposeStacks);
  } catch (err) {
    if (container && !isAuto) {
      container.innerHTML = `<div style="color: var(--accent-red); padding: 16px;">Lỗi tải Compose Stacks: ${err.message}</div>`;
    }
  }
}

function applyComposeSort() {
  const sortSelect = document.getElementById("compose-sort");
  if (sortSelect) {
    localStorage.setItem(SAVED_COMPOSE_SORT_KEY, sortSelect.value);
  }
  if (currentComposeStacks && currentComposeStacks.length > 0) {
    renderComposeStacks(currentComposeStacks);
  }
}

function sortStacksList(stacks, sortMode) {
  const sorted = [...stacks];
  sorted.sort((a, b) => {
    if (sortMode === "cpu") {
      const cpuA = a.total_cpu_percent || 0;
      const cpuB = b.total_cpu_percent || 0;
      if (cpuB !== cpuA) return cpuB - cpuA;
      return (b.total_mem_usage_mb || 0) - (a.total_mem_usage_mb || 0);
    } else if (sortMode === "ram") {
      const memA = a.total_mem_usage_mb || 0;
      const memB = b.total_mem_usage_mb || 0;
      if (memB !== memA) return memB - memA;
      return (b.total_cpu_percent || 0) - (a.total_cpu_percent || 0);
    } else if (sortMode === "name") {
      const pCmp = (a.project || "").localeCompare(b.project || "");
      if (pCmp !== 0) return pCmp;
      return (a.engine || "").localeCompare(b.engine || "");
    } else if (sortMode === "services") {
      const cntA = (a.services && a.services.length) || 0;
      const cntB = (b.services && b.services.length) || 0;
      if (cntB !== cntA) return cntB - cntA;
      const pCmp = (a.project || "").localeCompare(b.project || "");
      if (pCmp !== 0) return pCmp;
      return (a.engine || "").localeCompare(b.engine || "");
    }
    return 0;
  });
  return sorted;
}

function sortServicesList(services, sortMode) {
  const sorted = [...services];
  if (sortMode === "cpu") {
    sorted.sort((a, b) => (b.cpu_percent || 0) - (a.cpu_percent || 0));
  } else if (sortMode === "ram") {
    sorted.sort((a, b) => (b.mem_usage_mb || 0) - (a.mem_usage_mb || 0));
  } else if (sortMode === "name") {
    sorted.sort((a, b) => (a.service || "").localeCompare(b.service || ""));
  }
  return sorted;
}

function renderComposeStacks(stacks) {
  const container = document.getElementById("compose-stacks-container");
  if (!container) return;

  syncComposeSortDropdown();

  if (!stacks || stacks.length === 0) {
    container.innerHTML = `<div class="data-card" style="padding: 24px; text-align: center; color: var(--text-muted);">Không phát hiện project Compose / Pod nào đang chạy.</div>`;
    return;
  }

  const sortMode = getComposeSortMode();
  const sortedStacks = sortStacksList(stacks, sortMode);

  container.innerHTML = sortedStacks.map(st => {
    let stateClass = "badge-stopped";
    let stateText = "STOPPED";
    if (st.state === "running") { stateClass = "badge-running"; stateText = "RUNNING"; }
    else if (st.state === "partial") { stateClass = "badge-paused"; stateText = "PARTIAL RUNNING"; }

    const engineName = st.engine || "docker";
    const stackKey = `${st.project}|${engineName}`;
    const sortedServices = sortServicesList(st.services || [], sortMode);

    const servicesList = sortedServices.map(srv => {
      let srvBadge = srv.state === "running" ? "badge-running" : "badge-stopped";
      return `
        <div class="compose-service-row" data-role="service-row" data-service-id="${escapeHTML(srv.id || srv.name)}" style="display: flex; justify-content: space-between; align-items: center; padding: 10px 14px; background: rgba(255, 255, 255, 0.03); border-radius: 8px; margin-bottom: 6px; flex-wrap: wrap; gap: 8px;">
          <div>
            <span style="font-weight: 600; font-size: 0.95rem;">${escapeHTML(srv.service)}</span>
            <span style="color: var(--text-muted); font-size: 0.8rem; margin-left: 8px;">(${escapeHTML(srv.name)})</span>
          </div>
          <div style="display: flex; gap: 12px; align-items: center; flex-wrap: wrap;">
            ${srv.state === 'running' ? `
              <div data-role="srv-stats-box" style="font-size: 0.8rem; font-family: monospace; display: flex; gap: 12px; background: rgba(15, 23, 42, 0.4); padding: 4px 10px; border-radius: 6px; border: 1px solid rgba(255,255,255,0.05);">
                <span title="Service CPU %" data-role="srv-cpu" style="color: #38bdf8; font-weight: 600;">⚡ ${srv.cpu_percent ? srv.cpu_percent.toFixed(1) : 0.0}%</span>
                <span title="Service RAM Usage" data-role="srv-ram" style="color: #818cf8; font-weight: 600;">🧠 ${formatMBHelper(srv.mem_usage_mb)} (${srv.mem_percent ? srv.mem_percent.toFixed(1) : 0.0}%)</span>
                <span title="Service Network I/O" data-role="srv-net" style="color: #a855f7;">🌐 📥 ${formatMBHelper(srv.net_rx_mb)} | 📤 ${formatMBHelper(srv.net_tx_mb)}</span>
              </div>
            ` : ''}
            <span style="font-family: monospace; font-size: 0.8rem; color: var(--accent-blue);">${srv.ports_str}</span>
            <span class="badge ${srvBadge}" data-role="srv-badge">${srv.state.toUpperCase()}</span>
            ${srv.id ? `
              <div class="action-btns" style="display: flex; gap: 4px; align-items: center;">
                <button class="btn-icon ctr-btn-start" onclick="containerActionFromCompose('${srv.id}', 'start')" title="Start Service" ${srv.state === 'running' ? 'disabled' : ''}>▶️</button>
                <button class="btn-icon ctr-btn-stop" onclick="containerActionFromCompose('${srv.id}', 'stop')" title="Stop Service" ${srv.state !== 'running' ? 'disabled' : ''}>⏹️</button>
                <button class="btn-icon ctr-btn-restart" onclick="containerActionFromCompose('${srv.id}', 'restart')" title="Restart Service" ${srv.state !== 'running' ? 'disabled' : ''}>🔄</button>
                <button class="btn-icon ctr-btn-kill" onclick="confirmKillContainerFromCompose('${srv.id}', '${escapeHTML(srv.name || srv.service)}')" title="Kill Service (SIGKILL khẩn cấp)" ${srv.state !== 'running' ? 'disabled' : ''}>💀</button>
              </div>
            ` : ''}
          </div>
        </div>
      `;
    }).join("");

    const cpuVal = st.total_cpu_percent ? st.total_cpu_percent.toFixed(1) : "0.0";
    const memVal = st.total_mem_percent ? st.total_mem_percent.toFixed(1) : "0.0";

    return `
      <div class="data-card compose-stack-card" data-stack-key="${escapeHTML(stackKey)}" data-project="${escapeHTML(st.project)}" data-engine="${escapeHTML(engineName)}" style="padding: 20px;">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; flex-wrap: wrap; gap: 12px;">
          <div style="display: flex; align-items: center; gap: 10px;">
            <h3 style="font-size: 1.2rem;">🧩 Stack: ${escapeHTML(st.project)}</h3>
            ${engineName === 'podman' 
              ? `<span style="display:inline-block; padding:1px 6px; font-size:0.7rem; font-weight:600; border-radius:4px; background:rgba(192,132,252,0.15); color:#c084fc; border:1px solid rgba(192,132,252,0.3);">🦭 Podman</span>`
              : `<span style="display:inline-block; padding:1px 6px; font-size:0.7rem; font-weight:600; border-radius:4px; background:rgba(56,189,248,0.15); color:#38bdf8; border:1px solid rgba(56,189,248,0.3);">🐳 Docker</span>`}
            <span class="badge ${stateClass}" data-role="stack-badge">${stateText} (${st.running_count}/${st.total})</span>
          </div>
          <div style="display: flex; gap: 8px; flex-wrap: wrap;">
            <button class="btn btn-secondary" onclick="composeAction('${escapeHTML(st.project)}', '${escapeHTML(engineName)}', 'start')" ${st.state === 'running' ? 'disabled' : ''}>▶️ Start Stack</button>
            <button class="btn btn-secondary" onclick="composeAction('${escapeHTML(st.project)}', '${escapeHTML(engineName)}', 'restart')">🔄 Restart</button>
            <button class="btn btn-secondary" onclick="composeAction('${escapeHTML(st.project)}', '${escapeHTML(engineName)}', 'stop')" style="color: #f87171;" ${st.state === 'stopped' ? 'disabled' : ''}>⏹️ Stop Stack</button>
            <button class="btn btn-danger" onclick="confirmKillCompose('${escapeHTML(st.project)}', '${escapeHTML(engineName)}')" style="background: rgba(244, 63, 94, 0.2); border: 1px solid #f43f5e; color: #fda4af;" ${st.state === 'stopped' ? 'disabled' : ''}>💀 Kill Stack</button>
          </div>
        </div>

        <!-- STACK RESOURCE METRICS CARD -->
        <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 12px; background: rgba(15, 23, 42, 0.6); padding: 14px; border-radius: 10px; margin-bottom: 16px; border: 1px solid var(--border-color);">
          <div>
            <div style="font-size: 0.75rem; color: var(--text-muted); font-weight: 700; text-transform: uppercase;">⚡ Total Stack CPU</div>
            <div data-role="stack-cpu-text" style="font-size: 1.15rem; font-weight: 700; color: #38bdf8; margin-top: 2px;">${cpuVal}%</div>
            <div class="progress-bar" style="margin-top: 6px;"><div data-role="stack-cpu-bar" class="progress-fill" style="width: ${Math.min(parseFloat(cpuVal), 100)}%;"></div></div>
          </div>
          <div>
            <div style="font-size: 0.75rem; color: var(--text-muted); font-weight: 700; text-transform: uppercase;">🧠 Total Stack RAM</div>
            <div data-role="stack-ram-text" style="font-size: 1.15rem; font-weight: 700; color: #818cf8; margin-top: 2px;">
              ${formatMBHelper(st.total_mem_usage_mb)} <span style="font-size: 0.8rem; font-weight: 400; color: var(--text-muted);">(${memVal}%)</span>
            </div>
            <div class="progress-bar" style="margin-top: 6px;"><div data-role="stack-ram-bar" class="progress-fill" style="width: ${Math.min(parseFloat(memVal), 100)}%; background: #818cf8;"></div></div>
          </div>
          <div>
            <div style="font-size: 0.75rem; color: var(--text-muted); font-weight: 700; text-transform: uppercase;">🌐 Stack Network I/O</div>
            <div data-role="stack-net-text" style="font-size: 0.95rem; font-weight: 600; color: #a855f7; margin-top: 4px;">
              📥 ${formatMBHelper(st.total_net_rx_mb)} | 📤 ${formatMBHelper(st.total_net_tx_mb)}
            </div>
            <div style="font-size: 0.75rem; color: var(--text-muted); margin-top: 6px;">Total Rx / Tx aggregated</div>
          </div>
        </div>

        <div style="margin-top: 12px;">
          <div style="font-size: 0.8rem; font-weight: 700; color: var(--text-muted); text-transform: uppercase; margin-bottom: 8px;">Component Services & Resource Usage</div>
          <div data-role="services-container">${servicesList}</div>
        </div>
      </div>
    `;
  }).join("");
}

function updateComposeStatsInPlace(stacks) {
  const container = document.getElementById("compose-stacks-container");
  if (!container) return false;

  const existingCards = container.querySelectorAll(".compose-stack-card");
  if (existingCards.length === 0 || existingCards.length !== stacks.length) {
    return false; // Stacks count changed, fallback to full render
  }

  const stackMap = new Map();
  stacks.forEach(st => {
    const key = `${st.project}|${st.engine || 'docker'}`;
    stackMap.set(key, st);
  });

  for (const card of existingCards) {
    const stackKey = card.getAttribute("data-stack-key") || `${card.getAttribute("data-project")}|${card.getAttribute("data-engine") || "docker"}`;
    const st = stackMap.get(stackKey);
    if (!st) return false; // Stacks key mismatch, fallback to full render

    // Update stack state badge
    let stateClass = "badge-stopped";
    let stateText = "STOPPED";
    if (st.state === "running") { stateClass = "badge-running"; stateText = "RUNNING"; }
    else if (st.state === "partial") { stateClass = "badge-paused"; stateText = "PARTIAL RUNNING"; }

    const badgeEl = card.querySelector('[data-role="stack-badge"]');
    if (badgeEl) {
      badgeEl.className = `badge ${stateClass}`;
      badgeEl.textContent = `${stateText} (${st.running_count}/${st.total})`;
    }

    // Update stack CPU
    const cpuVal = st.total_cpu_percent ? st.total_cpu_percent.toFixed(1) : "0.0";
    const cpuTextEl = card.querySelector('[data-role="stack-cpu-text"]');
    if (cpuTextEl) cpuTextEl.textContent = `${cpuVal}%`;
    const cpuBarEl = card.querySelector('[data-role="stack-cpu-bar"]');
    if (cpuBarEl) cpuBarEl.style.width = `${Math.min(parseFloat(cpuVal), 100)}%`;

    // Update stack RAM
    const memVal = st.total_mem_percent ? st.total_mem_percent.toFixed(1) : "0.0";
    const ramTextEl = card.querySelector('[data-role="stack-ram-text"]');
    if (ramTextEl) {
      ramTextEl.innerHTML = `${formatMBHelper(st.total_mem_usage_mb)} <span style="font-size: 0.8rem; font-weight: 400; color: var(--text-muted);">(${memVal}%)</span>`;
    }
    const ramBarEl = card.querySelector('[data-role="stack-ram-bar"]');
    if (ramBarEl) ramBarEl.style.width = `${Math.min(parseFloat(memVal), 100)}%; background: #818cf8;`;

    // Update stack Net
    const netTextEl = card.querySelector('[data-role="stack-net-text"]');
    if (netTextEl) {
      netTextEl.innerHTML = `📥 ${formatMBHelper(st.total_net_rx_mb)} | 📤 ${formatMBHelper(st.total_net_tx_mb)}`;
    }

    // Update inner services stats
    const srvMap = new Map();
    (st.services || []).forEach(srv => srvMap.set(srv.id || srv.name, srv));

    const srvRows = card.querySelectorAll('[data-role="service-row"]');
    for (const sRow of srvRows) {
      const srvId = sRow.getAttribute("data-service-id");
      const srv = srvMap.get(srvId);
      if (srv) {
        // Update srv badge
        const srvBadgeEl = sRow.querySelector('[data-role="srv-badge"]');
        if (srvBadgeEl) {
          const srvBadgeClass = srv.state === "running" ? "badge-running" : "badge-stopped";
          srvBadgeEl.className = `badge ${srvBadgeClass}`;
          srvBadgeEl.textContent = srv.state.toUpperCase();
        }

        // Update srv metrics
        const cpuEl = sRow.querySelector('[data-role="srv-cpu"]');
        if (cpuEl) cpuEl.textContent = `⚡ ${srv.cpu_percent ? srv.cpu_percent.toFixed(1) : 0.0}%`;

        const ramEl = sRow.querySelector('[data-role="srv-ram"]');
        if (ramEl) ramEl.textContent = `🧠 ${formatMBHelper(srv.mem_usage_mb)} (${srv.mem_percent ? srv.mem_percent.toFixed(1) : 0.0}%)`;

        const netEl = sRow.querySelector('[data-role="srv-net"]');
        if (netEl) netEl.textContent = `🌐 📥 ${formatMBHelper(srv.net_rx_mb)} | 📤 ${formatMBHelper(srv.net_tx_mb)}`;
      }
    }
  }

  return true;
}

function formatMBHelper(mb) {
  if (!mb || isNaN(mb) || mb === 0) return "0 MB";
  if (typeof formatBytes === "function") {
    return formatBytes(mb * 1024 * 1024, 1);
  }
  if (mb >= 1024) return (mb / 1024).toFixed(1) + " GB";
  return mb.toFixed(1) + " MB";
}

async function composeAction(project, engine, action) {
  const label = action === "kill" ? "Kill" : action === "stop" ? "Stop" : action === "restart" ? "Restart" : "Start";
  try {
    const targetProject = engine ? `${project}|${engine}` : project;
    const res = await fetch("/api/compose/action", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ project: targetProject, action })
    });
    if (!res.ok) {
      const err = await res.json();
      if (typeof showToast === "function") {
        showToast(`❌ ${label} Stack "${project}" thất bại: ${err.error}`, "error");
      } else {
        alert(`Lỗi thao tác stack ${action}: ${err.error}`);
      }
    } else {
      if (typeof showToast === "function") {
        showToast(`✅ ${label} Stack "${project}" thành công!`, "success");
      }
      loadComposeStacks(false);
    }
  } catch (e) {
    if (typeof showToast === "function") {
      showToast(`❌ Lỗi hệ thống: ${e.message}`, "error");
    } else {
      alert(`Lỗi hệ thống: ${e.message}`);
    }
  }
}

async function confirmKillCompose(project, engine) {
  if (confirm(`Bạn có chắc chắn muốn Kill (buộc dừng khẩn cấp toàn bộ các container bằng SIGKILL) stack "${project}" không?\n\nLưu ý: Tất cả container thuộc stack này sẽ bị dừng ngay lập tức.`)) {
    await composeAction(project, engine, "kill");
  }
}

async function containerActionFromCompose(id, action) {
  const label = action === "kill" ? "Kill" : action === "stop" ? "Stop" : action === "restart" ? "Restart" : "Start";
  try {
    const res = await fetch("/api/containers/action", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ id, action })
    });
    if (!res.ok) {
      const err = await res.json();
      if (typeof showToast === "function") {
        showToast(`❌ ${label} service thất bại: ${err.error}`, "error");
      } else {
        alert(`Lỗi: ${err.error}`);
      }
    } else {
      if (typeof showToast === "function") {
        showToast(`✅ ${label} service thành công!`, "success");
      }
      loadComposeStacks(false);
    }
  } catch (e) {
    if (typeof showToast === "function") {
      showToast(`❌ Lỗi hệ thống: ${e.message}`, "error");
    } else {
      alert(`Lỗi hệ thống: ${e.message}`);
    }
  }
}

async function confirmKillContainerFromCompose(id, name) {
  if (confirm(`Bạn có chắc chắn muốn Kill (buộc dừng khẩn cấp bằng SIGKILL) service container "${name}" không?\n\nLưu ý: Tiến trình container sẽ bị dừng ngay lập tức.`)) {
    await containerActionFromCompose(id, "kill");
  }
}

// Setup Auto-Refresh Interval for Compose Stacks
if (!composeAutoRefreshInterval) {
  composeAutoRefreshInterval = setInterval(() => {
    const chk = document.getElementById("auto-refresh-compose-stats");
    const secCompose = document.getElementById("sec-compose");
    if (chk && chk.checked && secCompose && secCompose.classList.contains("active")) {
      loadComposeStacks(true); // in-place real-time refresh
    }
  }, 4000);
}
