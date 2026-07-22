let composeAutoRefreshInterval = null;

async function loadComposeStacks() {
  const container = document.getElementById("compose-stacks-container");
  try {
    const res = await fetch("/api/compose?stats=true");
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const stacks = await res.json();
    renderComposeStacks(stacks);
  } catch (err) {
    if (container) {
      container.innerHTML = `<div style="color: var(--accent-red); padding: 16px;">Lỗi tải Docker Compose Stacks: ${err.message}</div>`;
    }
  }
}

function renderComposeStacks(stacks) {
  const container = document.getElementById("compose-stacks-container");
  if (!container) return;

  if (!stacks || stacks.length === 0) {
    container.innerHTML = `<div class="data-card" style="padding: 24px; text-align: center; color: var(--text-muted);">Không phát hiện project Docker Compose nào đang chạy.</div>`;
    return;
  }

  // Sort stacks descending by CPU % first, then RAM usage MB
  stacks.sort((a, b) => {
    const cpuA = a.total_cpu_percent || 0;
    const cpuB = b.total_cpu_percent || 0;
    if (cpuA !== cpuB) return cpuB - cpuA;

    const memA = a.total_mem_usage_mb || 0;
    const memB = b.total_mem_usage_mb || 0;
    return memB - memA;
  });

  container.innerHTML = stacks.map(st => {
    let stateClass = "badge-stopped";
    let stateText = "STOPPED";
    if (st.state === "running") { stateClass = "badge-running"; stateText = "RUNNING"; }
    else if (st.state === "partial") { stateClass = "badge-paused"; stateText = "PARTIAL RUNNING"; }

    const servicesList = st.services.map(srv => {
      let srvBadge = srv.state === "running" ? "badge-running" : "badge-stopped";
      return `
        <div style="display: flex; justify-content: space-between; align-items: center; padding: 10px 14px; background: rgba(255, 255, 255, 0.03); border-radius: 8px; margin-bottom: 6px; flex-wrap: wrap; gap: 8px;">
          <div>
            <span style="font-weight: 600; font-size: 0.95rem;">${escapeHTML(srv.service)}</span>
            <span style="color: var(--text-muted); font-size: 0.8rem; margin-left: 8px;">(${escapeHTML(srv.name)})</span>
          </div>
          <div style="display: flex; gap: 16px; align-items: center; flex-wrap: wrap;">
            ${srv.state === 'running' ? `
              <div style="font-size: 0.8rem; font-family: monospace; display: flex; gap: 12px; background: rgba(15, 23, 42, 0.4); padding: 4px 10px; border-radius: 6px; border: 1px solid rgba(255,255,255,0.05);">
                <span title="Service CPU %" style="color: #38bdf8; font-weight: 600;">⚡ ${srv.cpu_percent ? srv.cpu_percent.toFixed(1) : 0.0}%</span>
                <span title="Service RAM Usage" style="color: #818cf8; font-weight: 600;">🧠 ${formatMBHelper(srv.mem_usage_mb)} (${srv.mem_percent ? srv.mem_percent.toFixed(1) : 0.0}%)</span>
                <span title="Service Network I/O" style="color: #a855f7;">🌐 📥 ${formatMBHelper(srv.net_rx_mb)} | 📤 ${formatMBHelper(srv.net_tx_mb)}</span>
              </div>
            ` : ''}
            <span style="font-family: monospace; font-size: 0.8rem; color: var(--accent-blue);">${srv.ports_str}</span>
            <span class="badge ${srvBadge}">${srv.state.toUpperCase()}</span>
          </div>
        </div>
      `;
    }).join("");

    const cpuVal = st.total_cpu_percent ? st.total_cpu_percent.toFixed(1) : 0.0;
    const memVal = st.total_mem_percent ? st.total_mem_percent.toFixed(1) : 0.0;

    return `
      <div class="data-card" style="padding: 20px;">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; flex-wrap: wrap; gap: 12px;">
          <div style="display: flex; align-items: center; gap: 12px;">
            <h3 style="font-size: 1.2rem;">🧩 Stack: ${escapeHTML(st.project)}</h3>
            <span class="badge ${stateClass}">${stateText} (${st.running_count}/${st.total})</span>
          </div>
          <div style="display: flex; gap: 8px;">
            <button class="btn btn-secondary" onclick="composeAction('${escapeHTML(st.project)}', 'start')">▶️ Start Stack</button>
            <button class="btn btn-secondary" onclick="composeAction('${escapeHTML(st.project)}', 'restart')">🔄 Restart</button>
            <button class="btn btn-danger" onclick="composeAction('${escapeHTML(st.project)}', 'stop')">⏹️ Stop Stack</button>
          </div>
        </div>

        <!-- STACK RESOURCE METRICS CARD -->
        <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 12px; background: rgba(15, 23, 42, 0.6); padding: 14px; border-radius: 10px; margin-bottom: 16px; border: 1px solid var(--border-color);">
          <div>
            <div style="font-size: 0.75rem; color: var(--text-muted); font-weight: 700; text-transform: uppercase;">⚡ Total Stack CPU</div>
            <div style="font-size: 1.15rem; font-weight: 700; color: #38bdf8; margin-top: 2px;">${cpuVal}%</div>
            <div class="progress-bar" style="margin-top: 6px;"><div class="progress-fill" style="width: ${Math.min(cpuVal, 100)}%;"></div></div>
          </div>
          <div>
            <div style="font-size: 0.75rem; color: var(--text-muted); font-weight: 700; text-transform: uppercase;">🧠 Total Stack RAM</div>
            <div style="font-size: 1.15rem; font-weight: 700; color: #818cf8; margin-top: 2px;">
              ${formatMBHelper(st.total_mem_usage_mb)} <span style="font-size: 0.8rem; font-weight: 400; color: var(--text-muted);">(${memVal}%)</span>
            </div>
            <div class="progress-bar" style="margin-top: 6px;"><div class="progress-fill" style="width: ${Math.min(memVal, 100)}%; background: #818cf8;"></div></div>
          </div>
          <div>
            <div style="font-size: 0.75rem; color: var(--text-muted); font-weight: 700; text-transform: uppercase;">🌐 Stack Network I/O</div>
            <div style="font-size: 0.95rem; font-weight: 600; color: #a855f7; margin-top: 4px;">
              📥 ${formatMBHelper(st.total_net_rx_mb)} | 📤 ${formatMBHelper(st.total_net_tx_mb)}
            </div>
            <div style="font-size: 0.75rem; color: var(--text-muted); margin-top: 6px;">Total Rx / Tx aggregated</div>
          </div>
        </div>

        <div style="margin-top: 12px;">
          <div style="font-size: 0.8rem; font-weight: 700; color: var(--text-muted); text-transform: uppercase; margin-bottom: 8px;">Component Services & Resource Usage</div>
          ${servicesList}
        </div>
      </div>
    `;
  }).join("");
}

function formatMBHelper(mb) {
  if (!mb || isNaN(mb) || mb === 0) return "0 MB";
  if (typeof formatBytes === "function") {
    return formatBytes(mb * 1024 * 1024, 1);
  }
  if (mb >= 1024) return (mb / 1024).toFixed(1) + " GB";
  return mb.toFixed(1) + " MB";
}

async function composeAction(project, action) {
  try {
    const res = await fetch("/api/compose/action", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ project, action })
    });
    if (!res.ok) {
      const err = await res.json();
      alert(`Lỗi thao tác stack ${action}: ${err.error}`);
    } else {
      loadComposeStacks();
    }
  } catch (e) {
    alert(`Lỗi hệ thống: ${e.message}`);
  }
}

// Setup Auto-Refresh Interval for Compose Stacks
if (!composeAutoRefreshInterval) {
  composeAutoRefreshInterval = setInterval(() => {
    const chk = document.getElementById("auto-refresh-compose-stats");
    const secCompose = document.getElementById("sec-compose");
    if (chk && chk.checked && secCompose && secCompose.classList.contains("active")) {
      loadComposeStacks();
    }
  }, 4000);
}
