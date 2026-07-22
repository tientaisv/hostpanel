let rawImagesData = [];

async function loadImages() {
  const tbody = document.getElementById("tbody-images");
  try {
    const res = await fetch("/api/images");
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    rawImagesData = await res.json();
    renderImages(rawImagesData);
  } catch (err) {
    tbody.innerHTML = `<tr><td colspan="6" style="text-align:center; color: var(--accent-red);">Lỗi tải Images: ${err.message}</td></tr>`;
  }
}

function renderImages(list) {
  const tbody = document.getElementById("tbody-images");
  if (!list || list.length === 0) {
    tbody.innerHTML = `<tr><td colspan="6" style="text-align:center; color: var(--text-muted);">Không có Docker Image nào trên host.</td></tr>`;
    return;
  }

  tbody.innerHTML = list.map(img => {
    let tagBadge = `<span class="badge badge-unused">🔴 Unused / Dangling</span>`;
    if (img.status_tag === "in_use") {
      tagBadge = `<span class="badge badge-inuse">🟢 In Use (Running)</span>`;
    } else if (img.status_tag === "used_stopped") {
      tagBadge = `<span class="badge badge-usedstop">🟡 Used (Stopped Ctr)</span>`;
    }

    const ctrsStr = (img.containers && img.containers.length > 0)
      ? img.containers.map(c => `<span style="font-family: monospace; background: rgba(255,255,255,0.05); padding: 2px 6px; border-radius: 4px; font-size: 0.8rem; margin-right: 4px;">${escapeHTML(c)}</span>`).join(" ")
      : `<span style="color: var(--text-muted); font-size: 0.8rem;">None</span>`;

    return `
      <tr>
        <td>
          <div style="font-weight: 700; color: var(--text-main);">${escapeHTML(img.tag_display)}</div>
        </td>
        <td>
          <span style="font-family: monospace; color: var(--text-muted);">${img.short_id}</span>
        </td>
        <td>${img.size_mb.toFixed(1)} MB</td>
        <td>${tagBadge}</td>
        <td>${ctrsStr}</td>
        <td>
          <button class="btn btn-danger" style="padding: 4px 10px; font-size: 0.75rem;" onclick="removeImagePrompt('${img.id}', '${escapeHTML(img.tag_display)}')">🗑️ Remove</button>
        </td>
      </tr>
    `;
  }).join("");
}

document.getElementById("search-images")?.addEventListener("input", (e) => {
  const q = e.target.value.toLowerCase().trim();
  if (!q) { renderImages(rawImagesData); return; }
  const filtered = rawImagesData.filter(img => img.tag_display.toLowerCase().includes(q) || img.short_id.toLowerCase().includes(q));
  renderImages(filtered);
});

async function removeImagePrompt(id, tag) {
  if (confirm(`Bạn có muốn xóa Image "${tag}" không?`)) {
    try {
      const res = await fetch("/api/images/remove", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ id, force: true })
      });
      if (!res.ok) {
        const err = await res.json();
        alert(`Lỗi xóa image: ${err.error}`);
      } else {
        loadImages();
      }
    } catch (e) {
      alert(`Lỗi hệ thống: ${e.message}`);
    }
  }
}

async function pruneImages() {
  if (confirm("Bạn có chắc chắn muốn Prune (dọn dẹp tất cả unused/dangling images) để giải phóng dung lượng ổ cứng không?")) {
    try {
      const res = await fetch("/api/images/prune", { method: "POST" });
      if (!res.ok) {
        const err = await res.json();
        alert(`Lỗi Prune images: ${err.error}`);
      } else {
        alert("Đã hoàn tất Prune unused images!");
        loadImages();
      }
    } catch (e) {
      alert(`Lỗi hệ thống: ${e.message}`);
    }
  }
}
