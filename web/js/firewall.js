// ==============================================================================
// 🛡️ DOCKPULSE SYSTEM FIREWALL MANAGER CLIENT MODULE
// ==============================================================================

async function loadFirewallStatus() {
  const container = document.getElementById("firewall-container");
  if (!container) return;

  try {
    const res = await fetch("/api/system/firewall/status");
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const data = await res.json();
    renderFirewallUI(data);
  } catch (err) {
    container.innerHTML = `<div style="color: var(--accent-red); padding: 16px;">Lỗi kết nối Firewall: ${escapeHTML(err.message)}</div>`;
  }
}

function renderFirewallUI(data) {
  const container = document.getElementById("firewall-container");
  if (!container) return;

  const isActive = data.active;
  const fwType = data.type || "UFW";

  let statusBadge = isActive 
    ? `<span class="badge badge-running" style="font-size: 0.85rem; padding: 5px 12px;">● Đang Bật (Active)</span>`
    : `<span class="badge badge-stopped" style="font-size: 0.85rem; padding: 5px 12px;">○ Đang Tắt (Disabled)</span>`;

  let toggleBtn = isActive
    ? `<button class="btn btn-danger" onclick="toggleFirewall(false)" style="font-size: 0.85rem; padding: 6px 14px;">🔴 Tắt Tường Lửa</button>`
    : `<button class="btn btn-primary" onclick="toggleFirewall(true)" style="font-size: 0.85rem; padding: 6px 14px; background: #22c55e; border-color: #22c55e;">🟢 Bật Tường Lửa (An Toàn)</button>`;

  let rulesHtml = "";
  if (!data.rules || data.rules.length === 0) {
    rulesHtml = `
      <div style="text-align: center; padding: 24px; color: var(--text-muted); font-size: 0.9rem; background: rgba(255,255,255,0.02); border-radius: 8px;">
        Chưa có quy tắc mở/chặn cổng nào được cấu hình trên tường lửa.
      </div>
    `;
  } else {
    rulesHtml = `
      <table class="data-table" style="width: 100%; font-size: 0.85rem;">
        <thead>
          <tr>
            <th style="width: 50px;">#</th>
            <th>Cổng / Dịch Vụ</th>
            <th>Giao Thức</th>
            <th>Hành Động</th>
            <th>IP Nguồn</th>
            <th style="text-align: right;">Thao Tác</th>
          </tr>
        </thead>
        <tbody>
          ${data.rules.map((r, idx) => `
            <tr>
              <td><span style="color: var(--text-muted); font-family: monospace;">${escapeHTML(r.id || String(idx + 1))}</span></td>
              <td><strong style="color: #38bdf8; font-family: monospace; font-size: 0.95rem;">${escapeHTML(r.port)}</strong></td>
              <td><span class="badge" style="background: rgba(255,255,255,0.08); text-transform: uppercase;">${escapeHTML(r.protocol)}</span></td>
              <td>
                <span class="badge ${r.action === 'ALLOW' ? 'badge-running' : 'badge-stopped'}">
                  ${escapeHTML(r.action)}
                </span>
              </td>
              <td><span style="font-family: monospace;">${escapeHTML(r.from_ip || 'Anywhere')}</span></td>
              <td style="text-align: right;">
                <button class="btn btn-secondary" onclick="deleteFirewallRule('${escapeHTML(r.id)}', '${escapeHTML(r.port)}', '${escapeHTML(r.protocol)}', '${escapeHTML(r.action)}', '${escapeHTML(r.from_ip)}')" style="padding: 4px 10px; font-size: 0.8rem; color: #f87171;">
                  🗑️ Xóa
                </button>
              </td>
            </tr>
          `).join("")}
        </tbody>
      </table>
    `;
  }

  container.innerHTML = `
    <!-- Top Status Control Bar -->
    <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 12px; padding-bottom: 16px; margin-bottom: 16px; border-bottom: 1px solid var(--border-color);">
      <div style="display: flex; align-items: center; gap: 12px; flex-wrap: wrap;">
        ${statusBadge}
        <span style="font-size: 0.85rem; color: var(--text-muted);">Công nghệ: <strong style="color: var(--text-main);">${escapeHTML(fwType)}</strong></span>
        <span style="font-size: 0.85rem; color: var(--text-muted);">Tổng số quy tắc: <strong style="color: var(--accent-blue);">${data.rules_count || 0}</strong></span>
      </div>
      <div>
        ${toggleBtn}
      </div>
    </div>

    <!-- Notice info -->
    <div style="background: rgba(56, 189, 248, 0.06); border-left: 3px solid #38bdf8; border-radius: 6px; padding: 10px 14px; margin-bottom: 16px; font-size: 0.82rem; color: var(--text-muted);">
      💡 <strong style="color: #38bdf8;">Lưu ý an toàn:</strong> Khi Bật Tường Lửa, DockPulse luôn tự động mở cổng <strong>22 (SSH)</strong> và cổng <strong>3800 (Dashboard)</strong> để tránh nguy cơ mất kết nối máy chủ.
    </div>

    <!-- Rules Table -->
    <div>
      ${rulesHtml}
    </div>
  `;
}

async function toggleFirewall(enable) {
  const actionText = enable ? "BẬT" : "TẮT";
  if (!confirm(`Bạn có chắc chắn muốn ${actionText} Tường lửa của máy chủ không?`)) {
    return;
  }

  try {
    const res = await fetch("/api/system/firewall/toggle", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ enable })
    });
    const data = await res.json();
    if (!res.ok) {
      alert(`❌ Lỗi: ${data.error}`);
    } else {
      alert(`✅ ${data.message}`);
      loadFirewallStatus();
    }
  } catch (e) {
    alert(`❌ Lỗi kết nối: ${e.message}`);
  }
}

function openAddRuleModal() {
  openModal("modal-firewall-rule");
}

async function submitAddFirewallRule() {
  const action = document.getElementById("fw-action").value;
  const port = document.getElementById("fw-port").value.trim();
  const protocol = document.getElementById("fw-proto").value;
  const fromIP = document.getElementById("fw-from").value.trim();

  if (!port && !fromIP) {
    alert("Vui lòng nhập số cổng (Port) hoặc địa chỉ IP nguồn!");
    return;
  }

  const btn = document.getElementById("btn-submit-fw-rule");
  if (btn) btn.disabled = true;

  try {
    const res = await fetch("/api/system/firewall/rule/add", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ action, port, protocol, from_ip: fromIP })
    });
    const data = await res.json();
    if (!res.ok) {
      alert(`❌ Lỗi: ${data.error}`);
    } else {
      alert(`✅ ${data.message}`);
      closeModal("modal-firewall-rule");
      // Reset inputs
      document.getElementById("fw-port").value = "";
      document.getElementById("fw-from").value = "";
      loadFirewallStatus();
    }
  } catch (e) {
    alert(`❌ Lỗi hệ thống: ${e.message}`);
  } finally {
    if (btn) btn.disabled = false;
  }
}

async function deleteFirewallRule(id, port, protocol, action, fromIP) {
  if (!confirm(`Bạn có chắc chắn muốn xóa quy tắc tường lửa cho cổng/IP [${port || fromIP}] không?`)) {
    return;
  }

  try {
    const res = await fetch("/api/system/firewall/rule/delete", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ id, port, protocol, action, from_ip: fromIP })
    });
    const data = await res.json();
    if (!res.ok) {
      alert(`❌ Lỗi xóa rule: ${data.error}`);
    } else {
      alert(`✅ ${data.message}`);
      loadFirewallStatus();
    }
  } catch (e) {
    alert(`❌ Lỗi hệ thống: ${e.message}`);
  }
}

// Hook into loadSecurityAudit to also refresh firewall
if (typeof window.loadSecurityAudit === "function") {
  const origSecAudit = window.loadSecurityAudit;
  window.loadSecurityAudit = function() {
    origSecAudit();
    loadFirewallStatus();
  };
}
