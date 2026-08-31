// ==============================================================================
// ⚡ DOCKPULSE AUTO-UPDATE & 1-CLICK UPGRADE CLIENT MODULE
// ==============================================================================

let currentUpdateInfo = null;

async function checkSystemUpdate(force = false) {
  try {
    const res = await fetch(`/api/system/update/check?force=${force}`);
    if (!res.ok) return;
    const info = await res.json();
    currentUpdateInfo = info;

    // Update Header Badge
    const badgeEl = document.getElementById("update-badge-btn");
    const badgeText = document.getElementById("update-badge-text");

    if (badgeEl && badgeText) {
      if (info.has_update) {
        badgeEl.style.display = "inline-flex";
        badgeText.textContent = `⚡ Có bản mới (${info.latest_version})`;
      } else {
        badgeEl.style.display = "none";
      }
    }

    // Render in modal if open
    renderUpdateModal(info);
  } catch (e) {
    console.error("Error checking updates:", e);
  }
}

function openUpdateModal() {
  openModal("modal-update");
  if (!currentUpdateInfo) {
    checkSystemUpdate(true);
  } else {
    renderUpdateModal(currentUpdateInfo);
  }
}

function renderUpdateModal(info) {
  if (!info) return;

  const curVerEl = document.getElementById("update-cur-ver");
  const latVerEl = document.getElementById("update-lat-ver");
  const notesEl = document.getElementById("update-notes");
  const autoToggle = document.getElementById("update-auto-toggle");
  const applyBtn = document.getElementById("btn-apply-update");

  if (curVerEl) curVerEl.textContent = info.current_version || "v1.3.0";
  if (latVerEl) {
    latVerEl.textContent = info.latest_version || info.current_version;
    latVerEl.style.color = info.has_update ? "#f59e0b" : "#38bdf8";
  }

  if (notesEl) {
    if (info.has_update) {
      notesEl.textContent = info.release_notes || "Có bản cập nhật mới sẵn sàng từ GitHub.";
    } else {
      notesEl.textContent = "✅ Bạn đang chạy phiên bản mới nhất! Không có bản cập nhật nào cần cài đặt.";
    }
  }

  if (autoToggle) {
    autoToggle.checked = !!info.auto_update_enabled;
  }

  if (applyBtn) {
    if (info.has_update) {
      applyBtn.style.display = "inline-flex";
      applyBtn.disabled = false;
      applyBtn.innerHTML = `🚀 Nâng Cấp 1-Click (${info.latest_version})`;
    } else {
      applyBtn.style.display = "none";
    }
  }
}

async function checkUpdateManual(force = true) {
  const btn = document.getElementById("btn-check-update");
  const notesEl = document.getElementById("update-notes");
  if (btn) btn.disabled = true;
  if (notesEl) notesEl.textContent = "⏳ Đang kết nối GitHub kiểm tra phiên bản mới nhất...";

  await checkSystemUpdate(force);

  if (btn) btn.disabled = false;
}

async function applyUpdateNow() {
  if (!confirm("Bạn có chắc chắn muốn nâng cấp phiên bản DockPulse mới ngay bây giờ không? Dịch vụ sẽ tự động khởi động lại trong vài giây.")) {
    return;
  }

  const applyBtn = document.getElementById("btn-apply-update");
  const checkBtn = document.getElementById("btn-check-update");
  const progressBox = document.getElementById("update-progress-box");
  const progressText = document.getElementById("update-progress-text");

  if (applyBtn) applyBtn.disabled = true;
  if (checkBtn) checkBtn.disabled = true;
  if (progressBox) progressBox.style.display = "block";
  if (progressText) progressText.textContent = "🚀 Đang tải gói cập nhật & biên dịch phiên bản mới từ GitHub...";

  try {
    const res = await fetch("/api/system/update/apply", { method: "POST" });
    const data = await res.json();

    if (!res.ok) {
      alert(`❌ Lỗi cập nhật: ${data.error || 'Unknown error'}`);
      if (progressBox) progressBox.style.display = "none";
      if (applyBtn) applyBtn.disabled = false;
      if (checkBtn) checkBtn.disabled = false;
      return;
    }

    if (progressText) {
      progressText.textContent = "⚙️ Đang áp dụng cập nhật và khởi động lại dịch vụ DockPulse...";
    }

    // Start poll loop waiting for service restart
    let countdown = 8;
    const interval = setInterval(() => {
      countdown--;
      if (progressText) {
        progressText.textContent = `🔄 Đang khởi động lại... Tự động tải lại trang sau ${countdown}s...`;
      }
      if (countdown <= 0) {
        clearInterval(interval);
        window.location.reload();
      }
    }, 1000);

  } catch (err) {
    if (progressText) {
      progressText.textContent = `🔄 Đang khởi động lại dịch vụ... Tự động nạp lại trang...`;
    }
    setTimeout(() => {
      window.location.reload();
    }, 4000);
  }
}

async function toggleAutoUpdateConfig(enabled) {
  try {
    await fetch("/api/system/update/config", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ auto_update_enabled: enabled })
    });
  } catch (e) {
    console.error("Error saving auto update config:", e);
  }
}

// Check for updates on startup after 3 seconds
document.addEventListener("DOMContentLoaded", () => {
  setTimeout(() => {
    checkSystemUpdate(false);
  }, 3000);
});
