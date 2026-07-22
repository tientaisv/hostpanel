let rawVolumesData = [];

async function loadVolumes() {
  const tbody = document.getElementById("tbody-volumes");
  try {
    const res = await fetch("/api/volumes");
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    rawVolumesData = await res.json();
    renderVolumes(rawVolumesData);
  } catch (err) {
    tbody.innerHTML = `<tr><td colspan="7" style="text-align:center; color: var(--accent-red);">Lỗi tải Volumes: ${err.message}</td></tr>`;
  }
}

function renderVolumes(list) {
  const tbody = document.getElementById("tbody-volumes");
  if (!list || list.length === 0) {
    tbody.innerHTML = `<tr><td colspan="7" style="text-align:center; color: var(--text-muted);">Không tìm thấy Docker Volume nào.</td></tr>`;
    return;
  }

  tbody.innerHTML = list.map(v => {
    let tagBadge = `<span class="badge badge-unused">🔴 Unused (Dangling)</span>`;
    if (v.status_tag === "in_use") {
      tagBadge = `<span class="badge badge-inuse">🟢 In Use (Active Ctr)</span>`;
    } else if (v.status_tag === "used_stopped") {
      tagBadge = `<span class="badge badge-usedstop">🟡 Attached (Stopped Ctr)</span>`;
    }

    const ctrsStr = (v.containers && v.containers.length > 0)
      ? v.containers.map(c => `<span style="font-family: monospace; background: rgba(255,255,255,0.05); padding: 2px 6px; border-radius: 4px; font-size: 0.8rem; margin-right: 4px;">${escapeHTML(c)}</span>`).join(" ")
      : `<span style="color: var(--text-muted); font-size: 0.8rem;">None</span>`;

    const sizeDisplay = v.size_display || "0 B";

    return `
      <tr>
        <td>
          <div style="font-weight: 700; color: var(--text-main); font-family: monospace;">${escapeHTML(v.name)}</div>
        </td>
        <td><span class="badge" style="background: rgba(255,255,255,0.08);">${escapeHTML(v.driver)}</span></td>
        <td>
          <div style="max-width: 250px; overflow: hidden; text-overflow: ellipsis; font-family: monospace; font-size: 0.8rem; color: var(--text-muted);" title="${escapeHTML(v.mountpoint)}">
            ${escapeHTML(v.mountpoint)}
          </div>
        </td>
        <td>
          <span style="font-weight: 700; color: var(--accent-blue); font-family: monospace;">${sizeDisplay}</span>
        </td>
        <td>${tagBadge}</td>
        <td>${ctrsStr}</td>
        <td>
          <button class="btn btn-danger" style="padding: 4px 10px; font-size: 0.75rem;" onclick="removeVolumePrompt('${escapeHTML(v.name)}')">🗑️ Remove</button>
        </td>
      </tr>
    `;
  }).join("");
}

document.getElementById("search-volumes")?.addEventListener("input", (e) => {
  const q = e.target.value.toLowerCase().trim();
  if (!q) { renderVolumes(rawVolumesData); return; }
  const filtered = rawVolumesData.filter(v => v.name.toLowerCase().includes(q) || v.mountpoint.toLowerCase().includes(q));
  renderVolumes(filtered);
});

async function removeVolumePrompt(name) {
  if (confirm(`Bạn có chắc chắn muốn xóa Volume "${name}" không?`)) {
    try {
      const res = await fetch("/api/volumes/remove", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name, force: true })
      });
      if (!res.ok) {
        const err = await res.json();
        alert(`Lỗi xóa Volume: ${err.error}`);
      } else {
        loadVolumes();
      }
    } catch (e) {
      alert(`Lỗi hệ thống: ${e.message}`);
    }
  }
}

async function pruneVolumes() {
  if (confirm("Bạn có chắc chắn muốn Prune (dọn dẹp tất cả Unused/Dangling Volumes) không? Thao tác này sẽ giải phóng dung lượng đĩa cứng.")) {
    try {
      const res = await fetch("/api/volumes/prune", { method: "POST" });
      if (!res.ok) {
        const err = await res.json();
        alert(`Lỗi Prune volumes: ${err.error}`);
      } else {
        alert("Đã hoàn tất Prune unused volumes!");
        loadVolumes();
      }
    } catch (e) {
      alert(`Lỗi hệ thống: ${e.message}`);
    }
  }
}
