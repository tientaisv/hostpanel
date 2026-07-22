async function loadSecurityAudit() {
  const threatsContainer = document.getElementById("security-threats-container");
  try {
    const res = await fetch("/api/system/security");
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const report = await res.json();
    renderSecurityReport(report);
  } catch (err) {
    if (threatsContainer) {
      threatsContainer.innerHTML = `<div style="color: var(--accent-red); padding: 16px;">Lỗi quét bảo mật hệ thống: ${err.message}</div>`;
    }
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
