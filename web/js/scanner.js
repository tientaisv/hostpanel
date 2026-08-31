// ==============================================================================
// 🛡️ DOCKPULSE HOST MALWARE, WEBSHELL & VIRUS SCANNER CLIENT MODULE
// ==============================================================================

let scannerPollInterval = null;
let currentScannerFilter = "ALL";
let lastScannerReport = null;

async function loadScannerStatus() {
  try {
    const res = await fetch("/api/system/scanner/status");
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const data = await res.json();
    lastScannerReport = data;
    renderScannerUI(data);

    // If scanning is in progress, poll faster
    if (data.is_scanning) {
      if (!scannerPollInterval) {
        scannerPollInterval = setInterval(loadScannerStatus, 1500);
      }
    } else {
      if (scannerPollInterval) {
        clearInterval(scannerPollInterval);
        scannerPollInterval = null;
      }
    }
  } catch (err) {
    console.error("Lỗi đọc trạng thái Scanner:", err);
  }
}

function renderScannerUI(data) {
  const container = document.getElementById("host-scanner-container");
  if (!container) return;

  const isScanning = data.is_scanning;
  const status = data.status || "IDLE";
  const threats = data.threats || [];
  const totalFiles = data.total_files_scanned || 0;
  const totalDirs = data.total_dirs_scanned || 0;
  const threatsCount = data.threats_found_count || threats.length || 0;
  const isClamAV = data.is_clamav_available;
  const scanDuration = (data.scan_duration_sec || 0).toFixed(1);

  // Status Badge
  let statusBadge = `<span class="badge" style="background: rgba(255,255,255,0.08); color: var(--text-muted); font-size: 0.85rem; padding: 5px 12px;">○ Sẵn Sàng Quét</span>`;
  if (isScanning) {
    statusBadge = `<span class="badge" style="background: linear-gradient(135deg, #ef4444, #f97316); color: #fff; font-size: 0.85rem; padding: 5px 14px; font-weight: 700; animation: pulse 1.5s infinite;">⚡ ĐANG QUÉT HỆ THỐNG...</span>`;
  } else if (threatsCount > 0) {
    statusBadge = `<span class="badge" style="background: rgba(239, 68, 68, 0.2); color: #f87171; border: 1px solid rgba(239, 68, 68, 0.4); font-size: 0.85rem; padding: 5px 14px; font-weight: 700;">⚠️ PHÁT HIỆN ${threatsCount} MỐI ĐE DỌA</span>`;
  } else if (status === "COMPLETED") {
    statusBadge = `<span class="badge" style="background: rgba(34, 197, 94, 0.15); color: #4ade80; border: 1px solid rgba(34, 197, 94, 0.35); font-size: 0.85rem; padding: 5px 14px; font-weight: 600;">✅ HỆ THỐNG AN TOÀN (0 Mã Độc)</span>`;
  }

  // ClamAV Badge or Install Button
  const clamavBtn = isClamAV
    ? `<span class="badge" style="background: rgba(56, 189, 248, 0.12); color: #38bdf8; border: 1px solid rgba(56, 189, 248, 0.3); font-size: 0.8rem; padding: 4px 10px;">🛡️ ClamAV Engine Active</span>`
    : `<button class="btn btn-secondary" onclick="installClamAV()" title="Cài đặt ClamAV Antivirus để quét thêm hàng triệu mẫu virus quốc tế" style="padding: 5px 10px; font-size: 0.8rem; background: rgba(56, 189, 248, 0.08); color: #38bdf8; border-color: rgba(56, 189, 248, 0.25);">
        + Cài Đặt ClamAV Antivirus
       </button>`;

  // Scanning Live Progress Banner
  let scanningBanner = "";
  if (isScanning) {
    scanningBanner = `
      <div style="background: linear-gradient(135deg, rgba(239, 68, 68, 0.12), rgba(249, 115, 22, 0.08)); border: 1px solid rgba(239, 68, 68, 0.3); border-radius: var(--radius-md); padding: 16px; margin-bottom: 20px;">
        <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 12px; margin-bottom: 10px;">
          <div style="display: flex; align-items: center; gap: 10px;">
            <span style="font-size: 1.3rem; animation: spin 2s linear infinite; display: inline-block;">🧭</span>
            <div>
              <strong style="color: #f87171; font-size: 0.95rem;">Đang phân tích sâu các tệp tin trên hệ thống...</strong>
              <div style="font-size: 0.8rem; color: var(--text-muted); font-family: var(--font-mono); margin-top: 2px;">
                Đang quét: <span style="color: #38bdf8;">${escapeHTML(data.current_scanning_dir || "Khởi tạo luồng quét...")}</span>
              </div>
            </div>
          </div>
          <button class="btn btn-danger" onclick="abortScan()" style="padding: 6px 14px; font-size: 0.85rem;">
            🛑 Dừng Quét
          </button>
        </div>
        <div class="progress-bar" style="height: 6px; background: rgba(255,255,255,0.06); border-radius: 3px; overflow: hidden;">
          <div class="progress-fill" style="width: 100%; background: linear-gradient(90deg, #ef4444, #f97316); animation: pulse 1.5s infinite;"></div>
        </div>
        <div style="display: flex; justify-content: space-between; font-size: 0.8rem; color: var(--text-muted); margin-top: 8px;">
          <span>Đã quét: <strong style="color: var(--text-main); font-family: var(--font-mono);">${totalFiles}</strong> tệp / <strong style="color: var(--text-main); font-family: var(--font-mono);">${totalDirs}</strong> thư mục</span>
          <span>Phát hiện nghi vấn: <strong style="color: #f87171; font-family: var(--font-mono);">${threatsCount}</strong></span>
        </div>
      </div>
    `;
  }

  // Filter Threats
  const filteredThreats = threats.filter(t => {
    if (currentScannerFilter === "ALL") return true;
    if (currentScannerFilter === "CRITICAL") return t.severity === "CRITICAL";
    if (currentScannerFilter === "HIGH") return t.severity === "HIGH";
    if (currentScannerFilter === "QUARANTINED") return t.status === "QUARANTINED";
    if (currentScannerFilter === "DELETED") return t.status === "DELETED";
    return true;
  });

  // Render Threat Rows
  let threatsTableHtml = "";
  if (threats.length === 0) {
    threatsTableHtml = `
      <div style="text-align: center; padding: 36px 20px; color: var(--text-muted); font-size: 0.9rem; background: rgba(255,255,255,0.02); border-radius: var(--radius-md);">
        <div style="font-size: 2rem; margin-bottom: 8px;">🛡️</div>
        <strong>Chưa phát hiện mã độc hoặc Webshell nào trên hệ thống.</strong>
        <div style="margin-top: 4px; font-size: 0.82rem;">Hãy nhấn <strong>"⚡ Quét Nhanh"</strong> để rà soát toàn bộ thư mục nhạy cảm (/tmp, /dev/shm, /var/www, /etc/cron*).</div>
      </div>
    `;
  } else {
    threatsTableHtml = `
      <div class="table-container" style="padding: 0;">
        <table class="data-table" style="width: 100%; font-size: 0.85rem;">
          <thead>
            <tr>
              <th style="width: 110px;">Mức Độ</th>
              <th>Loại Mã Độc / Chữ Ký</th>
              <th>Đường Dẫn Tệp (File Path)</th>
              <th style="width: 90px;">Dung Lượng</th>
              <th style="width: 110px;">Trạng Thái</th>
              <th style="text-align: right; width: 220px;">Thao Tác Xử Lý</th>
            </tr>
          </thead>
          <tbody>
            ${filteredThreats.map(t => {
              const isQuarantined = t.status === "QUARANTINED";
              const isDeleted = t.status === "DELETED";

              let sevBadge = `<span class="badge badge-stopped">CRITICAL</span>`;
              if (t.severity === "HIGH") sevBadge = `<span class="badge" style="background: rgba(249, 115, 22, 0.2); color: #fb923c; border: 1px solid rgba(249, 115, 22, 0.4);">HIGH</span>`;
              if (t.severity === "MEDIUM") sevBadge = `<span class="badge" style="background: rgba(234, 179, 8, 0.2); color: #facc15; border: 1px solid rgba(234, 179, 8, 0.4);">MEDIUM</span>`;

              let statusText = `<span style="color: #f87171; font-weight: 600;">● Đang Tồn Tại</span>`;
              if (isQuarantined) statusText = `<span style="color: #38bdf8; font-weight: 600;">🔒 Đã Cách Ly</span>`;
              if (isDeleted) statusText = `<span style="color: var(--text-muted); text-decoration: line-through;">🗑️ Đã Xóa</span>`;

              return `
                <tr style="${isDeleted ? 'opacity: 0.5;' : ''}">
                  <td>${sevBadge}</td>
                  <td>
                    <div style="font-weight: 700; color: #f87171;">${escapeHTML(t.category)}</div>
                    <div style="font-size: 0.75rem; color: var(--text-muted);">${escapeHTML(t.description || t.matched_pattern)}</div>
                  </td>
                  <td>
                    <div style="font-family: var(--font-mono); color: var(--text-main); font-weight: 600; word-break: break-all;">
                      ${escapeHTML(t.file_path)}
                    </div>
                    ${t.snippet ? `
                      <div style="font-size: 0.75rem; color: var(--accent-amber); font-family: var(--font-mono); background: rgba(0,0,0,0.3); padding: 2px 6px; border-radius: 4px; margin-top: 4px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 420px;">
                        Line ${t.line_number}: ${escapeHTML(t.snippet)}
                      </div>
                    ` : ''}
                  </td>
                  <td><span style="font-family: var(--font-mono); color: var(--text-muted);">${formatHumanBytes(t.file_size)}</span></td>
                  <td>${statusText}</td>
                  <td style="text-align: right;">
                    <div style="display: flex; gap: 6px; justify-content: flex-end;">
                      <button class="btn btn-secondary" onclick="viewThreatSnippet('${escapeHTML(t.file_path)}')" title="Xem nội dung tệp" style="padding: 4px 8px; font-size: 0.8rem;" ${isDeleted ? 'disabled' : ''}>
                        👁️ Xem
                      </button>
                      <button class="btn btn-secondary" onclick="quarantineThreat('${escapeHTML(t.file_path)}')" title="Vô hiệu hóa quyền 0000 và di chuyển vào thư mục cách ly" style="padding: 4px 8px; font-size: 0.8rem; color: #38bdf8;" ${isQuarantined || isDeleted ? 'disabled' : ''}>
                        🔒 Cách Ly
                      </button>
                      <button class="btn btn-secondary" onclick="deleteThreatFile('${escapeHTML(t.file_path)}')" title="Xóa vĩnh viễn tệp độc hại" style="padding: 4px 8px; font-size: 0.8rem; color: #f87171;" ${isDeleted ? 'disabled' : ''}>
                        🗑️ Xóa
                      </button>
                    </div>
                  </td>
                </tr>
              `;
            }).join("")}
          </tbody>
        </table>
      </div>
    `;
  }

  container.innerHTML = `
    <!-- Top Action & Overview Card -->
    <div class="data-card" style="margin-bottom: 24px;">
      <div class="card-header" style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 12px;">
        <div style="display: flex; align-items: center; gap: 12px; flex-wrap: wrap;">
          <div style="font-size: 1.15rem; font-weight: 700; display: flex; align-items: center; gap: 8px;">
            <span>🛡️</span> Quét Webshell, Backdoor & Virus Máy Chủ (Host Malware Scanner)
          </div>
          ${statusBadge}
          ${clamavBtn}
        </div>

        <div style="display: flex; gap: 8px; flex-wrap: wrap;">
          <button class="btn btn-primary" onclick="startScan('QUICK')" ${isScanning ? 'disabled' : ''} style="padding: 6px 14px; font-size: 0.85rem; background: linear-gradient(135deg, #38bdf8, #2563eb);">
            ⚡ Quét Nhanh (Quick Scan)
          </button>
          <button class="btn btn-secondary" onclick="openCustomScanModal()" ${isScanning ? 'disabled' : ''} style="padding: 6px 14px; font-size: 0.85rem;">
            📁 Quét Thư Mục Chỉ Định
          </button>
          <button class="btn btn-secondary" onclick="startScan('FULL')" ${isScanning ? 'disabled' : ''} title="Quét toàn bộ hệ thống file host" style="padding: 6px 14px; font-size: 0.85rem;">
            🌐 Quét Toàn Bộ
          </button>
        </div>
      </div>

      <div class="card-body">
        ${scanningBanner}

        <!-- 4 Summary Metrics -->
        <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 14px; margin-bottom: 20px;">
          <div style="background: #090e18; padding: 12px 16px; border-radius: var(--radius-md); border: 1px solid var(--border-color);">
            <div style="font-size: 0.75rem; color: var(--text-muted); font-weight: 700; text-transform: uppercase;">📂 Tổng Tệp Đã Quét</div>
            <div style="font-size: 1.4rem; font-weight: 800; color: #38bdf8; font-family: var(--font-mono); margin-top: 2px;">
              ${totalFiles.toLocaleString()}
            </div>
            <div style="font-size: 0.75rem; color: var(--text-muted); margin-top: 4px;">Thời gian quét: ${scanDuration}s</div>
          </div>

          <div style="background: #090e18; padding: 12px 16px; border-radius: var(--radius-md); border: 1px solid var(--border-color);">
            <div style="font-size: 0.75rem; color: var(--text-muted); font-weight: 700; text-transform: uppercase;">🚨 Mã Độc Phát Hiện</div>
            <div style="font-size: 1.4rem; font-weight: 800; color: ${threatsCount > 0 ? '#ef4444' : '#34d399'}; font-family: var(--font-mono); margin-top: 2px;">
              ${threatsCount}
            </div>
            <div style="font-size: 0.75rem; color: var(--text-muted); margin-top: 4px;">Webshell / Backdoor / Miner</div>
          </div>

          <div style="background: #090e18; padding: 12px 16px; border-radius: var(--radius-md); border: 1px solid var(--border-color);">
            <div style="font-size: 0.75rem; color: var(--text-muted); font-weight: 700; text-transform: uppercase;">🔒 Đã Cách Ly An Toàn</div>
            <div style="font-size: 1.4rem; font-weight: 800; color: #a855f7; font-family: var(--font-mono); margin-top: 2px;">
              ${threats.filter(t => t.status === "QUARANTINED").length}
            </div>
            <div style="font-size: 0.75rem; color: var(--text-muted); margin-top: 4px;">Kho: /root/.dockpulse_quarantine</div>
          </div>

          <div style="background: #090e18; padding: 12px 16px; border-radius: var(--radius-md); border: 1px solid var(--border-color);">
            <div style="font-size: 0.75rem; color: var(--text-muted); font-weight: 700; text-transform: uppercase;">⏱️ Lần Quét Gần Nhất</div>
            <div style="font-size: 1.05rem; font-weight: 700; color: var(--text-main); font-family: var(--font-mono); margin-top: 6px;">
              ${escapeHTML(data.finished_at || data.started_at || "--")}
            </div>
            <div style="font-size: 0.75rem; color: var(--text-muted); margin-top: 4px;">Chế độ: ${escapeHTML(data.target_type || "QUICK")}</div>
          </div>
        </div>

        <!-- Filter Bar -->
        <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 10px; margin-bottom: 14px;">
          <div style="font-weight: 700; font-size: 0.95rem; color: var(--text-main);">
            Danh Sách Mối Đe Dọa Chi Tiết (${filteredThreats.length})
          </div>
          <div style="display: flex; gap: 6px; flex-wrap: wrap;">
            <button class="btn btn-secondary ${currentScannerFilter === 'ALL' ? 'active' : ''}" onclick="setScannerFilter('ALL')" style="padding: 3px 10px; font-size: 0.78rem;">Tất cả (${threats.length})</button>
            <button class="btn btn-secondary ${currentScannerFilter === 'CRITICAL' ? 'active' : ''}" onclick="setScannerFilter('CRITICAL')" style="padding: 3px 10px; font-size: 0.78rem; color: #f87171;">Critical (${threats.filter(t => t.severity === 'CRITICAL').length})</button>
            <button class="btn btn-secondary ${currentScannerFilter === 'QUARANTINED' ? 'active' : ''}" onclick="setScannerFilter('QUARANTINED')" style="padding: 3px 10px; font-size: 0.78rem; color: #38bdf8;">Đã cách ly (${threats.filter(t => t.status === 'QUARANTINED').length})</button>
          </div>
        </div>

        <!-- Table -->
        ${threatsTableHtml}
      </div>
    </div>
  `;
}

function setScannerFilter(filter) {
  currentScannerFilter = filter;
  if (lastScannerReport) {
    renderScannerUI(lastScannerReport);
  }
}

async function startScan(targetType = "QUICK", customPath = "", useClamAV = false) {
  try {
    const res = await fetch("/api/system/scanner/start", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        target_type: targetType,
        custom_path: customPath,
        use_clamav: useClamAV
      })
    });
    const data = await res.json();
    if (!res.ok) {
      alert(`❌ Lỗi: ${data.error}`);
    } else {
      if (typeof showToast === "function") {
        showToast(data.message, "info");
      }
      loadScannerStatus();
    }
  } catch (err) {
    alert(`❌ Lỗi kết nối: ${err.message}`);
  }
}

async function abortScan() {
  if (!confirm("Bạn có chắc muốn dừng phiên quét mã độc hiện tại không?")) return;
  try {
    const res = await fetch("/api/system/scanner/abort", { method: "POST" });
    const data = await res.json();
    if (res.ok) {
      if (typeof showToast === "function") showToast(data.message, "info");
      loadScannerStatus();
    }
  } catch (err) {
    alert("❌ Lỗi: " + err.message);
  }
}

async function quarantineThreat(filePath) {
  if (!confirm(`🔒 BẠN CÓ CHẮC MUỐN CÁCH LY TỆP NÀY?\n\nĐường dẫn: ${filePath}\n\nThao tác này sẽ gỡ bỏ hoàn toàn quyền thực thi (chmod 0000) và di chuyển tệp vào thư mục cách ly an toàn (/root/.dockpulse_quarantine).`)) {
    return;
  }

  try {
    const res = await fetch("/api/system/scanner/threat/quarantine", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ file_path: filePath })
    });
    const data = await res.json();
    if (!res.ok) {
      alert(`❌ Lỗi: ${data.error}`);
    } else {
      if (typeof showToast === "function") {
        showToast("Đã cách ly tệp an toàn!", "success");
      } else {
        alert("✅ " + data.message);
      }
      loadScannerStatus();
    }
  } catch (err) {
    alert("❌ Lỗi kết nối: " + err.message);
  }
}

async function deleteThreatFile(filePath) {
  if (!confirm(`⚠️ CẢNH BÁO XÓA VĨNH VIỄN!\n\nĐường dẫn: ${filePath}\n\nBạn có chắc chắn muốn xóa tệp này vĩnh viễn khỏi máy chủ không? Thao tác này không thể hoàn tác.`)) {
    return;
  }

  try {
    const res = await fetch("/api/system/scanner/threat/delete", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ file_path: filePath })
    });
    const data = await res.json();
    if (!res.ok) {
      alert(`❌ Lỗi: ${data.error}`);
    } else {
      if (typeof showToast === "function") {
        showToast("Đã xóa tệp độc hại vĩnh viễn!", "success");
      } else {
        alert("✅ " + data.message);
      }
      loadScannerStatus();
    }
  } catch (err) {
    alert("❌ Lỗi kết nối: " + err.message);
  }
}

async function viewThreatSnippet(filePath) {
  try {
    const res = await fetch("/api/system/scanner/threat/view", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ file_path: filePath, max_bytes: 8192 })
    });
    const data = await res.json();
    if (!res.ok) {
      alert(`❌ Lỗi đọc tệp: ${data.error}`);
      return;
    }

    document.getElementById("modal-malware-filepath").textContent = data.file_path;
    document.getElementById("modal-malware-code").textContent = data.snippet || "(Tệp rỗng hoặc nhị phân)";
    openModal("modal-view-malware-code");
  } catch (err) {
    alert("❌ Lỗi kết nối: " + err.message);
  }
}

function openCustomScanModal() {
  openModal("modal-custom-scan-path");
}

function submitCustomScan() {
  const path = document.getElementById("input-custom-scan-path").value.trim();
  const useClamAV = document.getElementById("cb-custom-scan-clamav").checked;
  if (!path) {
    alert("Vui lòng nhập đường dẫn thư mục cần quét!");
    return;
  }
  closeModal("modal-custom-scan-path");
  startScan("CUSTOM", path, useClamAV);
}

async function installClamAV() {
  if (!confirm("Bắt đầu cài đặt ClamAV Antivirus qua apt package manager? Quá trình này sẽ diễn ra ngầm trong 1-2 phút.")) {
    return;
  }
  try {
    const res = await fetch("/api/system/scanner/clamav/install", { method: "POST" });
    const data = await res.json();
    alert("🚀 " + data.message);
  } catch (err) {
    alert("❌ Lỗi: " + err.message);
  }
}

// Hook into loadSecurityAudit to also refresh scanner status
if (typeof window.loadSecurityAudit === "function") {
  const originalSecAudit = window.loadSecurityAudit;
  window.loadSecurityAudit = function() {
    originalSecAudit();
    loadScannerStatus();
  };
}

document.addEventListener("DOMContentLoaded", () => {
  loadScannerStatus();
});
