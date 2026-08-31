// ==============================================================================
// 🔥 DOCKPULSE SERVER KEEP-WARM (ANTI-IDLE) MANAGER CLIENT MODULE
// ==============================================================================

let warmupPollInterval = null;
let lastWarmupState = null;

async function loadWarmupStatus() {
  try {
    const res = await fetch("/api/system/warmup/status");
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const data = await res.json();
    renderWarmupUI(data);
  } catch (err) {
    console.error("Lỗi đọc trạng thái Keep-Warm:", err);
  }
}

function renderWarmupUI(data) {
  const container = document.getElementById("warmup-widget-container");
  if (!container) return;

  const isEnabled = data.enabled;
  const state = data.state || "DISABLED";
  const currentCPU = (data.current_cpu || 0).toFixed(1);
  const targetCPU = (data.target_cpu_percent || 45).toFixed(0);
  const lowThreshold = (data.low_cpu_threshold || 30).toFixed(0);
  const maxThreshold = (data.max_cpu_threshold || 65).toFixed(0);
  const remainingSec = data.phase_remaining_sec || 0;
  const elapsedSec = data.phase_elapsed_sec || 0;
  const totalPhaseSec = elapsedSec + remainingSec;
  const progressPercent = totalPhaseSec > 0 ? Math.min(100, Math.max(0, (elapsedSec / totalPhaseSec) * 100)) : 0;

  // Determine State Badge and Themes
  let stateBadge = `<span class="badge" style="background: rgba(255,255,255,0.08); color: var(--text-muted); font-size: 0.85rem; padding: 5px 12px;">○ Đang Tắt</span>`;
  let cardBorderColor = "var(--border-color)";
  let cardGlow = "none";
  let phaseTitle = "Chế độ đang tắt";
  let timerDisplay = "--:--";

  if (remainingSec > 0) {
    const m = Math.floor(remainingSec / 60);
    const s = remainingSec % 60;
    timerDisplay = `${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`;
  }

  if (isEnabled) {
    switch (state) {
      case "WARMING":
        stateBadge = `<span class="badge" style="background: linear-gradient(135deg, #ef4444, #f97316); color: #ffffff; font-size: 0.85rem; padding: 5px 14px; font-weight: 700; box-shadow: 0 0 12px rgba(249, 115, 22, 0.4); animation: pulse 2s infinite;">🔥 ĐANG LÀM NÓNG (~${targetCPU}% CPU)</span>`;
        cardBorderColor = "rgba(249, 115, 22, 0.4)";
        cardGlow = "0 0 20px rgba(239, 68, 68, 0.08)";
        phaseTitle = `Đang duy trì tải CPU an toàn > 40% (Còn lại: <strong style="color: #f97316; font-family: var(--font-mono);">${timerDisplay}</strong>)`;
        break;
      case "COOLDOWN":
        stateBadge = `<span class="badge" style="background: rgba(56, 189, 248, 0.18); color: #38bdf8; border: 1px solid rgba(56, 189, 248, 0.4); font-size: 0.85rem; padding: 5px 14px; font-weight: 600;">⏳ ĐANG NGHỈ NGƠI (30m)</span>`;
        cardBorderColor = "rgba(56, 189, 248, 0.3)";
        phaseTitle = `Phiên 30m làm nóng hoàn tất. Đang nghỉ ngơi (Còn lại: <strong style="color: #38bdf8; font-family: var(--font-mono);">${timerDisplay}</strong>)`;
        break;
      case "MONITORING":
        stateBadge = `<span class="badge" style="background: rgba(245, 158, 11, 0.15); color: #fbbf24; border: 1px solid rgba(245, 158, 11, 0.35); font-size: 0.85rem; padding: 5px 14px; font-weight: 600;">⏱️ ĐANG GIÁM SÁT NHÀN RỖI</span>`;
        cardBorderColor = "rgba(245, 158, 11, 0.3)";
        phaseTitle = `Theo dõi CPU < ${lowThreshold}% trong 30 phút để kích hoạt (Đã ghi nhận: <strong style="color: #fbbf24; font-family: var(--font-mono);">${Math.floor(data.low_cpu_timer_sec / 60)}m ${data.low_cpu_timer_sec % 60}s</strong> / 30m)`;
        break;
      case "TESTING":
        stateBadge = `<span class="badge" style="background: linear-gradient(135deg, #a855f7, #ec4899); color: #ffffff; font-size: 0.85rem; padding: 5px 14px; font-weight: 700;">⚡ CHẠY THỬ NGHIỆM (TEST)</span>`;
        cardBorderColor = "rgba(168, 85, 247, 0.4)";
        phaseTitle = `Đang test làm nóng CPU 1 phút (Còn lại: <strong style="color: #c084fc; font-family: var(--font-mono);">${timerDisplay}</strong>)`;
        break;
    }
  }

  const throttleAlert = data.is_throttled
    ? `<div style="margin-top: 10px; background: rgba(239, 68, 68, 0.12); border-left: 3px solid #ef4444; border-radius: 4px; padding: 8px 12px; font-size: 0.8rem; color: #fca5a5;">
        ⚠️ <strong>Auto-Throttle Active:</strong> CPU hệ thống tăng cao (> ${maxThreshold}%), bộ làm nóng đang tự động giảm tải để bảo vệ tài nguyên máy chủ.
       </div>`
    : "";

  container.innerHTML = `
    <div class="data-card" style="border-color: ${cardBorderColor}; box-shadow: ${cardGlow}; transition: all 0.3s ease; margin-bottom: 24px;">
      <!-- Header -->
      <div class="card-header" style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 12px;">
        <div style="display: flex; align-items: center; gap: 12px; flex-wrap: wrap;">
          <div style="font-size: 1.1rem; font-weight: 700; display: flex; align-items: center; gap: 8px; color: var(--text-main);">
            <span>🔥</span> Chế Độ Làm Nóng Máy Chủ (Anti-Idle / Keep-Warm)
          </div>
          ${stateBadge}
        </div>
        
        <div style="display: flex; align-items: center; gap: 8px; flex-wrap: wrap;">
          <button class="btn btn-secondary" onclick="openWarmupConfigModal()" title="Cài đặt ngưỡng CPU và thời gian chu kỳ" style="padding: 6px 12px; font-size: 0.82rem;">
            ⚙️ Cài Đặt
          </button>
          
          <button class="btn btn-secondary" onclick="triggerWarmupTest(60)" ${state === 'WARMING' || state === 'TESTING' ? 'disabled' : ''} title="Chạy thử nghiệm tải CPU an toàn trong 60 giây" style="padding: 6px 12px; font-size: 0.82rem; background: rgba(168, 85, 247, 0.12); color: #c084fc; border-color: rgba(168, 85, 247, 0.3);">
            ⚡ Test 1 Phút
          </button>

          <button class="btn ${isEnabled ? 'btn-danger' : 'btn-primary'}" onclick="toggleWarmup(${!isEnabled})" style="padding: 6px 16px; font-size: 0.85rem; font-weight: 600; ${!isEnabled ? 'background: #22c55e; border-color: #22c55e;' : ''}">
            ${isEnabled ? '🔴 Tắt Chế Độ' : '🟢 Bật Làm Nóng'}
          </button>
        </div>
      </div>

      <!-- Body Content -->
      <div class="card-body">
        <!-- Status message & details -->
        <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 8px; margin-bottom: 12px; font-size: 0.88rem;">
          <div style="color: var(--text-muted);">
            📌 <span>${escapeHTML(phaseTitle)}</span>
          </div>
          <div style="font-size: 0.82rem; color: var(--text-muted);">
            Phiên đã chạy: <strong style="color: var(--text-main); font-family: var(--font-mono);">${data.total_warmup_sessions || 0}</strong>
          </div>
        </div>

        <!-- Progress Bar for Current Phase -->
        ${isEnabled && totalPhaseSec > 0 ? `
          <div class="progress-bar" style="height: 6px; margin-bottom: 16px; background: rgba(255,255,255,0.06); border-radius: 3px; overflow: hidden;">
            <div class="progress-fill" style="width: ${progressPercent}%; background: ${state === 'WARMING' ? 'linear-gradient(90deg, #f97316, #ef4444)' : state === 'COOLDOWN' ? '#38bdf8' : '#fbbf24'}; transition: width 0.5s ease;"></div>
          </div>
        ` : ''}

        <!-- 4 Metric Cards Grid -->
        <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 12px;">
          <!-- Card 1: Realtime CPU -->
          <div style="background: #090e18; padding: 12px 14px; border-radius: var(--radius-md); border: 1px solid var(--border-color);">
            <div style="font-size: 0.75rem; color: var(--text-muted); font-weight: 700; text-transform: uppercase;">⚡ CPU Hệ Thống Realtime</div>
            <div style="font-size: 1.35rem; font-weight: 800; color: ${Number(currentCPU) > 65 ? '#ef4444' : Number(currentCPU) >= 40 ? '#f97316' : '#38bdf8'}; font-family: var(--font-mono); margin-top: 2px;">
              ${currentCPU}%
            </div>
            <div style="font-size: 0.75rem; color: var(--text-muted); margin-top: 4px;">Ngưỡng kích hoạt: &lt; ${lowThreshold}%</div>
          </div>

          <!-- Card 2: Target CPU -->
          <div style="background: #090e18; padding: 12px 14px; border-radius: var(--radius-md); border: 1px solid var(--border-color);">
            <div style="font-size: 0.75rem; color: var(--text-muted); font-weight: 700; text-transform: uppercase;">🎯 Mục Tiêu Duy Trì</div>
            <div style="font-size: 1.35rem; font-weight: 800; color: #34d399; font-family: var(--font-mono); margin-top: 2px;">
              ${targetCPU}% CPU
            </div>
            <div style="font-size: 0.75rem; color: var(--text-muted); margin-top: 4px;">Đảm bảo luôn &gt; 40% an toàn</div>
          </div>

          <!-- Card 3: Cycle Rule -->
          <div style="background: #090e18; padding: 12px 14px; border-radius: var(--radius-md); border: 1px solid var(--border-color);">
            <div style="font-size: 0.75rem; color: var(--text-muted); font-weight: 700; text-transform: uppercase;">🔄 Chu Kỳ Hoạt Động</div>
            <div style="font-size: 1.15rem; font-weight: 700; color: #c084fc; font-family: var(--font-mono); margin-top: 4px;">
              30m Chạy ⇄ 30m Nghỉ
            </div>
            <div style="font-size: 0.75rem; color: var(--text-muted); margin-top: 4px;">Kiểm tra lại sau mỗi chu kỳ</div>
          </div>

          <!-- Card 4: Safety Protection -->
          <div style="background: #090e18; padding: 12px 14px; border-radius: var(--radius-md); border: 1px solid var(--border-color);">
            <div style="font-size: 0.75rem; color: var(--text-muted); font-weight: 700; text-transform: uppercase;">🛡️ Bảo Vệ Quá Tải (Safety)</div>
            <div style="font-size: 1.15rem; font-weight: 700; color: #38bdf8; font-family: var(--font-mono); margin-top: 4px;">
              Max ${maxThreshold}% CPU
            </div>
            <div style="font-size: 0.75rem; color: var(--text-muted); margin-top: 4px;">Tự ngắt/giảm tải khi có tác vụ khác</div>
          </div>
        </div>

        ${throttleAlert}
      </div>
    </div>
  `;
}

async function toggleWarmup(enable) {
  const actionText = enable ? "BẬT chế độ làm nóng máy chủ tự động (30m chạy ⇄ 30m nghỉ)" : "TẮT chế độ làm nóng máy chủ";
  if (!confirm(`Bạn có chắc chắn muốn ${actionText} không?`)) {
    return;
  }

  try {
    const res = await fetch("/api/system/warmup/toggle", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ enable })
    });
    const data = await res.json();
    if (!res.ok) {
      alert(`❌ Lỗi: ${data.error}`);
    } else {
      if (typeof showToast === "function") {
        showToast(data.message, "success");
      } else {
        alert(`✅ ${data.message}`);
      }
      loadWarmupStatus();
    }
  } catch (err) {
    alert(`❌ Lỗi kết nối: ${err.message}`);
  }
}

async function triggerWarmupTest(durationSec = 60) {
  try {
    const res = await fetch("/api/system/warmup/test", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ duration_sec: durationSec })
    });
    const data = await res.json();
    if (!res.ok) {
      alert(`❌ Lỗi: ${data.error}`);
    } else {
      if (typeof showToast === "function") {
        showToast(data.message, "info");
      }
      loadWarmupStatus();
    }
  } catch (err) {
    alert(`❌ Lỗi kết nối: ${err.message}`);
  }
}

function openWarmupConfigModal() {
  fetch("/api/system/warmup/status")
    .then(r => r.json())
    .then(data => {
      document.getElementById("warmup-cfg-low-cpu").value = data.low_cpu_threshold || 30;
      document.getElementById("warmup-cfg-target-cpu").value = data.target_cpu_percent || 45;
      document.getElementById("warmup-cfg-max-cpu").value = data.max_cpu_threshold || 65;
      document.getElementById("warmup-cfg-idle-mins").value = Math.round((data.idle_check_duration_sec || 1800) / 60);
      document.getElementById("warmup-cfg-warm-mins").value = Math.round((data.warmup_duration_sec || 1800) / 60);
      document.getElementById("warmup-cfg-cool-mins").value = Math.round((data.cooldown_duration_sec || 1800) / 60);
      openModal("modal-warmup-config");
    })
    .catch(e => alert("Không thể tải cấu hình: " + e.message));
}

async function submitWarmupConfig() {
  const lowCPU = parseFloat(document.getElementById("warmup-cfg-low-cpu").value);
  const targetCPU = parseFloat(document.getElementById("warmup-cfg-target-cpu").value);
  const maxCPU = parseFloat(document.getElementById("warmup-cfg-max-cpu").value);
  const idleMins = parseInt(document.getElementById("warmup-cfg-idle-mins").value, 10);
  const warmMins = parseInt(document.getElementById("warmup-cfg-warm-mins").value, 10);
  const coolMins = parseInt(document.getElementById("warmup-cfg-cool-mins").value, 10);

  if (isNaN(lowCPU) || lowCPU < 5 || lowCPU > 80) {
    alert("Ngưỡng CPU kích hoạt không hợp lệ (5% - 80%)");
    return;
  }
  if (isNaN(targetCPU) || targetCPU < 35 || targetCPU > 75) {
    alert("CPU mục tiêu làm nóng không hợp lệ (35% - 75%)");
    return;
  }
  if (isNaN(maxCPU) || maxCPU <= targetCPU || maxCPU > 95) {
    alert("Ngưỡng ngắt khẩn cấp quá tải phải lớn hơn CPU mục tiêu và <= 95%");
    return;
  }

  const payload = {
    low_cpu_threshold: lowCPU,
    target_cpu_percent: targetCPU,
    max_cpu_threshold: maxCPU,
    idle_check_duration_sec: (idleMins || 30) * 60,
    warmup_duration_sec: (warmMins || 30) * 60,
    cooldown_duration_sec: (coolMins || 30) * 60
  };

  try {
    const res = await fetch("/api/system/warmup/config", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload)
    });
    const data = await res.json();
    if (!res.ok) {
      alert(`❌ Lỗi: ${data.error}`);
    } else {
      closeModal("modal-warmup-config");
      if (typeof showToast === "function") {
        showToast(data.message, "success");
      } else {
        alert("✅ " + data.message);
      }
      loadWarmupStatus();
    }
  } catch (err) {
    alert("❌ Lỗi lưu cấu hình: " + err.message);
  }
}

// Auto-start polling when page loads
function initWarmupModule() {
  loadWarmupStatus();
  if (!warmupPollInterval) {
    warmupPollInterval = setInterval(loadWarmupStatus, 3000);
  }
}

if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", initWarmupModule);
} else {
  initWarmupModule();
}
