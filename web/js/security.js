function escapeHTML(str) {
  if (str === null || str === undefined) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

async function loadSecurityAudit() {
  const threatsContainer = document.getElementById("security-threats-container");
  try {
    const res = await fetch("/api/system/security");
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const report = await res.json();
    renderSecurityReport(report);
  } catch (err) {
    if (threatsContainer) {
      threatsContainer.innerHTML = `<div style="color: var(--accent-red); padding: 16px;">Lỗi quét bảo mật hệ thống: ${escapeHTML(err.message)}</div>`;
    }
  }

  // Load Fail2ban & Firewall status alongside audit
  loadFail2banStatus();
  if (typeof loadFirewallStatus === "function") {
    loadFirewallStatus();
  }
}

function renderSecurityReport(report) {
  if (!report) return;

  // Threat Level Badge
  const levelEl = document.getElementById("sec-threat-level");
  if (levelEl) {
    if (report.threat_level === "CRITICAL") {
      levelEl.innerHTML = `<span style="color: var(--accent-red); font-weight: 700;">🔴 NGUY HIỂM (${report.threat_score}/100)</span>`;
    } else if (report.threat_level === "WARNING") {
      levelEl.innerHTML = `<span style="color: var(--accent-amber); font-weight: 700;">🟡 CẢNH BÁO (${report.threat_score}/100)</span>`;
    } else {
      levelEl.innerHTML = `<span style="color: var(--accent-green); font-weight: 700;">🟢 AN TOÀN (${report.threat_score}/100)</span>`;
    }
  }

  if (document.getElementById("sec-scan-time")) {
    document.getElementById("sec-scan-time").textContent = `Scan time: ${report.scan_time || '--'}`;
  }
  if (document.getElementById("sec-failed-logins")) {
    document.getElementById("sec-failed-logins").textContent = report.failed_logins_count || 0;
  }
  if (document.getElementById("sec-privileged-ctrs")) {
    document.getElementById("sec-privileged-ctrs").textContent = report.privileged_ctrs_count || 0;
  }
  if (document.getElementById("sec-exposed-ports")) {
    document.getElementById("sec-exposed-ports").textContent = report.exposed_ports_count || 0;
  }

  const threatsContainer = document.getElementById("security-threats-container");
  if (!threatsContainer) return;

  if (!report.threats || report.threats.length === 0) {
    threatsContainer.innerHTML = `
      <div style="text-align: center; padding: 30px; background: rgba(34, 197, 94, 0.05); border: 1px solid rgba(34, 197, 94, 0.2); border-radius: 12px; color: #4ade80;">
        <div style="font-size: 1.8rem; margin-bottom: 8px;">🛡️</div>
        <div style="font-weight: 700; font-size: 1.05rem;">Hệ thống đang ở trạng thái an toàn tốt!</div>
        <div style="font-size: 0.85rem; color: var(--text-muted); margin-top: 4px;">Không phát hiện tiến trình nghi vấn, tấn công Brute-force nguy hiểm hoặc rủi ro nghiêm trọng nào.</div>
      </div>
    `;
    return;
  }

  threatsContainer.innerHTML = report.threats.map(t => {
    let borderColor = "var(--border-color)";
    let badgeClass = "badge-running";
    if (t.level === "CRITICAL") {
      borderColor = "rgba(239, 68, 68, 0.4)";
      badgeClass = "badge-stopped";
    } else if (t.level === "WARNING") {
      borderColor = "rgba(245, 158, 11, 0.4)";
      badgeClass = "badge-paused";
    }

    return `
      <div style="padding: 16px; background: rgba(15, 23, 42, 0.6); border-radius: 10px; border: 1px solid ${borderColor}; display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 12px;">
        <div style="flex: 1;">
          <div style="display: flex; align-items: center; gap: 10px; margin-bottom: 6px;">
            <span class="badge ${badgeClass}">${t.level}</span>
            <span style="font-size: 0.75rem; font-weight: 700; color: var(--accent-blue); text-transform: uppercase;">[${escapeHTML(t.category)}]</span>
            <span style="font-weight: 700; font-size: 1.05rem; color: var(--text-main);">${escapeHTML(t.title)}</span>
          </div>
          <div style="font-size: 0.85rem; color: var(--text-muted); line-height: 1.4;">${escapeHTML(t.description)}</div>
          ${t.action_hint ? `<div style="font-size: 0.8rem; color: #38bdf8; margin-top: 6px; font-weight: 600;">💡 Khuyên dùng: ${escapeHTML(t.action_hint)}</div>` : ''}
        </div>
        ${t.ip_address ? `
          <button class="btn btn-danger" onclick="blockIPPrompt('${escapeHTML(t.ip_address)}')" style="padding: 6px 12px; font-size: 0.85rem;">🚫 Block IP (Firewall)</button>
        ` : ''}
      </div>
    `;
  }).join("");
}

async function blockIPPrompt(ip) {
  if (confirm(`Bạn có chắc chắn muốn thêm quy tắc Iptables chặn địa chỉ IP ${ip} không?`)) {
    try {
      const res = await fetch("/api/system/security/block-ip", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ ip })
      });
      if (!res.ok) {
        const err = await res.json();
        alert(`Lỗi chặn IP: ${err.error}`);
      } else {
        alert(`✅ Đã thêm quy tắc chặn IP ${ip} thành công!`);
        loadSecurityAudit();
      }
    } catch (e) {
      alert(`Lỗi hệ thống: ${e.message}`);
    }
  }
}

// ----------------------------------------------------
// FAIL2BAN MANAGEMENT MODULE
// ----------------------------------------------------

async function loadFail2banStatus() {
  const container = document.getElementById("fail2ban-container");
  if (!container) return;

  try {
    const res = await fetch("/api/system/fail2ban/status");
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const data = await res.json();
    renderFail2banUI(data);
  } catch (err) {
    container.innerHTML = `<div style="color: var(--accent-red); padding: 16px;">Lỗi kết nối Fail2ban: ${escapeHTML(err.message)}</div>`;
  }
}

function renderFail2banUI(data) {
  const container = document.getElementById("fail2ban-container");
  if (!container) return;

  // Case 1: Fail2ban is not installed
  if (!data.installed) {
    container.innerHTML = `
      <div style="background: rgba(239, 68, 68, 0.06); border: 1px dashed rgba(239, 68, 68, 0.3); border-radius: 12px; padding: 24px; text-align: center;">
        <div style="font-size: 2.2rem; margin-bottom: 10px;">🛡️</div>
        <h4 style="font-weight: 700; font-size: 1.1rem; color: var(--text-main); margin-bottom: 6px;">Fail2ban Chưa Được Kích Hoạt</h4>
        <p style="color: var(--text-muted); font-size: 0.9rem; max-width: 620px; margin: 0 auto 18px auto; line-height: 1.5;">
          Fail2ban sẽ liên tục giám sát nhật ký máy chủ và tự động cấm (ban) các địa chỉ IP đang cố ý quét cổng, brute-force mật khẩu SSH hoặc tấn công Web Server.
        </p>
        <button class="btn btn-primary" id="btn-install-f2b" onclick="installFail2ban()" style="padding: 9px 20px; font-weight: 600; font-size: 0.95rem;">
          ⚡ Cài Đặt & Kích Hoạt Fail2ban 1-Click
        </button>
      </div>
    `;
    return;
  }

  // Case 2: Installed but service stopped
  if (!data.active) {
    container.innerHTML = `
      <div style="background: rgba(245, 158, 11, 0.08); border: 1px solid rgba(245, 158, 11, 0.3); border-radius: 12px; padding: 20px; text-align: center;">
        <div style="font-size: 1.8rem; margin-bottom: 8px;">⚠️</div>
        <h4 style="font-weight: 700; color: var(--text-main); margin-bottom: 6px;">Fail2ban Đã Cài Đặt nhưng Đang Tạm Dừng</h4>
        <p style="color: var(--text-muted); font-size: 0.85rem; margin-bottom: 14px;">${escapeHTML(data.error_message || 'Dịch vụ fail2ban.service chưa chạy.')}</p>
        <button class="btn btn-primary" onclick="installFail2ban()" style="padding: 7px 16px; font-size: 0.9rem;">
          🔄 Khởi Động & Cấu Hình Lại Fail2ban
        </button>
      </div>
    `;
    return;
  }

  // Case 3: Fully active and running
  let allBannedRows = [];
  data.jails.forEach(jail => {
    if (jail.banned_ips && jail.banned_ips.length > 0) {
      jail.banned_ips.forEach(ip => {
        allBannedRows.push({ jail: jail.name, ip: ip });
      });
    }
  });

  const jailsHtml = data.jails.map(jail => {
    return `
      <div style="background: rgba(15, 23, 42, 0.5); border: 1px solid var(--border-color); border-radius: 10px; padding: 14px 16px; display: flex; flex-direction: column; gap: 8px;">
        <div style="display: flex; justify-content: space-between; align-items: center;">
          <div style="font-weight: 700; color: var(--text-main); font-size: 0.95rem; display: flex; align-items: center; gap: 6px;">
            <span>🔒</span> ${escapeHTML(jail.name)}
          </div>
          <span class="badge ${jail.currently_banned > 0 ? 'badge-stopped' : 'badge-running'}">
            ${jail.currently_banned} IP Banned
          </span>
        </div>
        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 6px; font-size: 0.8rem; color: var(--text-muted); margin-top: 4px;">
          <div>Thử sai gần đây: <strong style="color: var(--text-main);">${jail.currently_failed}</strong></div>
          <div>Tổng số lần dò: <strong style="color: var(--text-main);">${jail.total_failed}</strong></div>
          <div>Tổng IP đã cấm: <strong style="color: var(--text-main);">${jail.total_banned}</strong></div>
        </div>
      </div>
    `;
  }).join("");

  const bannedTableHtml = allBannedRows.length === 0 ? `
    <div style="text-align: center; padding: 20px; color: #4ade80; font-size: 0.9rem; background: rgba(34, 197, 94, 0.04); border-radius: 8px; border: 1px solid rgba(34, 197, 94, 0.15);">
      🟢 Hiện tại không có IP nào đang bị cấm. Hệ thống hoạt động an toàn.
    </div>
  ` : `
    <table class="data-table" style="width: 100%; font-size: 0.85rem;">
      <thead>
        <tr>
          <th>Địa chỉ IP Bị Chặn</th>
          <th>Jail / Dịch Vụ</th>
          <th style="text-align: right;">Hành Động</th>
        </tr>
      </thead>
      <tbody>
        ${allBannedRows.map(row => `
          <tr>
            <td><strong style="color: #f87171; font-family: monospace;">${escapeHTML(row.ip)}</strong></td>
            <td><span class="badge badge-paused">${escapeHTML(row.jail)}</span></td>
            <td style="text-align: right;">
              <button class="btn btn-secondary" onclick="unbanIP('${escapeHTML(row.jail)}', '${escapeHTML(row.ip)}')" style="padding: 4px 10px; font-size: 0.8rem;">
                🟢 Gỡ Chặn (Unban)
              </button>
            </td>
          </tr>
        `).join("")}
      </tbody>
    </table>
  `;

  container.innerHTML = `
    <!-- Top Summary Banner -->
    <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 12px; padding-bottom: 16px; margin-bottom: 16px; border-bottom: 1px solid var(--border-color);">
      <div style="display: flex; align-items: center; gap: 12px;">
        <span class="badge badge-running" style="font-size: 0.85rem; padding: 5px 12px;">● Fail2ban Active</span>
        <span style="font-size: 0.85rem; color: var(--text-muted);">Phiên bản: <strong style="color: var(--text-main);">${escapeHTML(data.version || '1.0.x')}</strong></span>
      </div>
      <div style="display: flex; gap: 16px; font-size: 0.85rem;">
        <div>Tổng số Jail: <strong style="color: var(--accent-blue);">${data.jail_count}</strong></div>
        <div>IP đang bị chặn: <strong style="color: var(--accent-red);">${data.total_banned_ips}</strong></div>
      </div>
    </div>

    <!-- Active Jails Grid -->
    <div style="margin-bottom: 20px;">
      <h5 style="font-size: 0.85rem; font-weight: 700; color: var(--text-muted); text-transform: uppercase; margin-bottom: 10px;">
        🛡️ Danh Sách Jails Giám Sát (${data.jail_count})
      </h5>
      <div style="display: grid; grid-template-columns: repeat(auto-fill, minmax(240px, 1fr)); gap: 12px;">
        ${jailsHtml}
      </div>
    </div>

    <!-- Banned IPs List -->
    <div>
      <h5 style="font-size: 0.85rem; font-weight: 700; color: var(--text-muted); text-transform: uppercase; margin-bottom: 10px; display: flex; justify-content: space-between; align-items: center;">
        <span>🚫 Danh Sách IP Đang Bị Cấm (${allBannedRows.length})</span>
      </h5>
      ${bannedTableHtml}
    </div>
  `;
}

async function installFail2ban() {
  const btn = document.getElementById("btn-install-f2b");
  if (btn) {
    btn.disabled = true;
    btn.innerHTML = `⏳ Đang tải & cài đặt Fail2ban...`;
  }

  try {
    const res = await fetch("/api/system/fail2ban/install", { method: "POST" });
    const data = await res.json();
    if (!res.ok) {
      alert(`❌ Lỗi cài đặt Fail2ban: ${data.error || 'Unknown error'}`);
    } else {
      alert(`🎉 ${data.message || 'Cài đặt và kích hoạt Fail2ban thành công!'}`);
      loadFail2banStatus();
    }
  } catch (err) {
    alert(`❌ Lỗi hệ thống: ${err.message}`);
  } finally {
    if (btn) {
      btn.disabled = false;
      btn.innerHTML = `⚡ Cài Đặt & Kích Hoạt Fail2ban 1-Click`;
    }
  }
}

async function unbanIP(jail, ip) {
  if (!confirm(`Bạn có chắc chắn muốn mở chặn (Unban) cho IP ${ip} khỏi Jail [${jail}]?`)) {
    return;
  }

  try {
    const res = await fetch("/api/system/fail2ban/unban", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ jail, ip })
    });
    const data = await res.json();
    if (!res.ok) {
      alert(`❌ Lỗi mở chặn IP: ${data.error}`);
    } else {
      alert(`✅ ${data.message}`);
      loadFail2banStatus();
    }
  } catch (err) {
    alert(`❌ Lỗi hệ thống: ${err.message}`);
  }
}

async function promptManualBan() {
  const ip = prompt("Nhập địa chỉ IP bạn muốn cấm (Ban):");
  if (!ip || !ip.trim()) return;

  const jail = prompt("Nhập tên Jail (mặc định: sshd):", "sshd") || "sshd";

  try {
    const res = await fetch("/api/system/fail2ban/ban", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ jail: jail.trim(), ip: ip.trim() })
    });
    const data = await res.json();
    if (!res.ok) {
      alert(`❌ Lỗi cấm IP: ${data.error}`);
    } else {
      alert(`✅ ${data.message}`);
      loadFail2banStatus();
    }
  } catch (err) {
    alert(`❌ Lỗi hệ thống: ${err.message}`);
  }
}
