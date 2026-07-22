async function loadHostProcesses() {
  const tbody = document.getElementById("tbody-processes");
  const sortVal = document.getElementById("proc-sort")?.value || "cpu";

  try {
    const res = await fetch(`/api/system/processes?sort=${sortVal}&limit=50`);
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const procs = await res.json();
    renderProcesses(procs);
  } catch (err) {
    tbody.innerHTML = `<tr><td colspan="7" style="text-align:center; color: var(--accent-red);">Lỗi quét tiến trình: ${err.message}</td></tr>`;
  }
}

function renderProcesses(list) {
  const tbody = document.getElementById("tbody-processes");
  if (!list || list.length === 0) {
    tbody.innerHTML = `<tr><td colspan="7" style="text-align:center; color: var(--text-muted);">Không tìm thấy tiến trình nào.</td></tr>`;
    return;
  }

  tbody.innerHTML = list.map((p) => {
    const cpuDisplay = p.cpu_percent ? p.cpu_percent.toFixed(1) : "0.0";
    const memBytes = (p.mem_rss_mb || 0) * 1024 * 1024;
    const memFormatted = formatHumanBytes(memBytes);
    const memPctDisplay = p.mem_percent ? p.mem_percent.toFixed(1) : "0.0";

    const isHighCPU = p.cpu_percent > 20;
    const isHighRAM = p.mem_rss_mb > 500;

    return `
      <tr>
        <td>
          <span style="font-family: monospace; font-weight: 700; color: var(--accent-blue);">${p.pid}</span>
        </td>
        <td>
          <span class="badge" style="background: rgba(255,255,255,0.08);">${escapeHTML(p.user)}</span>
        </td>
        <td>
          <div style="font-weight: 600; color: var(--text-main); font-size: 0.9rem;">${escapeHTML(p.name)}</div>
          <div style="max-width: 350px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-family: monospace; font-size: 0.75rem; color: var(--text-muted);" title="${escapeHTML(p.cmdline)}">
            ${escapeHTML(p.cmdline)}
          </div>
        </td>
        <td>
          <span style="font-weight: 700; ${isHighRAM ? 'color: var(--accent-amber);' : ''}">${memFormatted}</span>
        </td>
        <td>${memPctDisplay}%</td>
        <td>
          <span style="font-weight: 700; ${isHighCPU ? 'color: var(--accent-red);' : ''}">${cpuDisplay}%</span>
        </td>
        <td>
          <button class="btn btn-danger" style="padding: 4px 10px; font-size: 0.75rem;" onclick="killProcessPrompt(${p.pid}, '${escapeHTML(p.name)}')">💀 Kill</button>
        </td>
      </tr>
    `;
  }).join("");
}

async function killProcessPrompt(pid, name) {
  if (confirm(`Bạn có chắc chắn muốn Kill tiến trình "${name}" (PID: ${pid}) không?`)) {
    try {
      const res = await fetch("/api/system/processes/kill", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ pid })
      });
      if (!res.ok) {
        const err = await res.json();
        alert(`Lỗi Kill tiến trình: ${err.error}`);
      } else {
        loadHostProcesses();
      }
    } catch (e) {
      alert(`Lỗi hệ thống: ${e.message}`);
    }
  }
}
