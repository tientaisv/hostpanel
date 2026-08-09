let currentDirPath = "/root";
let diskBarChart = null;
let diskPieChart = null;

async function loadFiles(dirPath) {
  if (dirPath !== undefined) {
    currentDirPath = dirPath;
  }
  const tbody = document.getElementById("tbody-files");
  const breadcrumbEl = document.getElementById("file-breadcrumbs");
  if (!tbody) return;

  tbody.innerHTML = `<tr><td colspan="6" style="text-align:center;">⏳ Đang tải danh sách tệp tại <code>${escapeHtml(currentDirPath)}</code>...</td></tr>`;

  try {
    const res = await fetch(`/api/files/list?path=${encodeURIComponent(currentDirPath)}`);
    if (!res.ok) {
      const errData = await res.json();
      tbody.innerHTML = `<tr><td colspan="6" style="text-align:center; color: var(--danger);">⚠️ Lỗi: ${escapeHtml(errData.error || "Không thể tải thư mục")}</td></tr>`;
      return;
    }
    const items = await res.json();

    // Render Breadcrumbs
    renderBreadcrumbs(currentDirPath, breadcrumbEl);

    if (!items || items.length === 0) {
      tbody.innerHTML = `<tr><td colspan="6" style="text-align:center; color: var(--text-muted);">Thư mục trống</td></tr>`;
      return;
    }

    let html = "";
    items.forEach(item => {
      const icon = item.is_dir ? "📁" : getFileIcon(item.name);
      const sizeStr = item.is_dir ? "-" : formatBytes(item.size);
      const modTimeStr = item.mod_time ? new Date(item.mod_time).toLocaleString("vi-VN") : "-";

      html += `
        <tr>
          <td style="font-weight: 500;">
            <span style="font-size: 1.1rem; margin-right: 6px;">${icon}</span>
            ${item.is_dir 
              ? `<a href="javascript:void(0)" onclick="loadFiles('${escapeJsString(item.path)}')" style="color: #60a5fa; text-decoration: none; font-weight: 600;">${escapeHtml(item.name)}</a>`
              : `<span>${escapeHtml(item.name)}</span>`
            }
          </td>
          <td><span class="badge ${item.is_dir ? 'badge-primary' : 'badge-secondary'}">${item.is_dir ? 'Directory' : 'File'}</span></td>
          <td>${sizeStr}</td>
          <td style="font-size: 0.85rem; color: var(--text-muted);">${modTimeStr}</td>
          <td style="font-family: monospace; font-size: 0.8rem;">${escapeHtml(item.mode || '-')}</td>
          <td>
            <div style="display: flex; gap: 6px;">
              ${item.is_dir
                ? `<button class="btn btn-secondary" onclick="loadFiles('${escapeJsString(item.path)}')" style="padding: 4px 8px; font-size: 0.75rem;">📂 Mở</button>`
                : `<button class="btn btn-primary" onclick="openFileEditor('${escapeJsString(item.path)}')" style="padding: 4px 8px; font-size: 0.75rem;">✏️ Edit</button>
                   <button class="btn btn-secondary" onclick="downloadFile('${escapeJsString(item.path)}')" style="padding: 4px 8px; font-size: 0.75rem;">⬇️ Tải</button>`
              }
              <button class="btn btn-danger" onclick="deleteFile('${escapeJsString(item.path)}')" style="padding: 4px 8px; font-size: 0.75rem;">🗑️ Xóa</button>
            </div>
          </td>
        </tr>
      `;
    });

    tbody.innerHTML = html;
  } catch (err) {
    tbody.innerHTML = `<tr><td colspan="6" style="text-align:center; color: var(--danger);">⚠️ Lỗi kết nối: ${escapeHtml(err.message)}</td></tr>`;
  }
}

function renderBreadcrumbs(path, el) {
  if (!el) return;
  const parts = path.split("/").filter(Boolean);
  let accumulated = "";
  let html = `<a href="javascript:void(0)" onclick="loadFiles('/')" style="color: #94a3b8; text-decoration: none;">🏠 root</a>`;

  parts.forEach((p) => {
    accumulated += "/" + p;
    const currentPath = accumulated;
    html += ` <span style="color: var(--text-muted);">/</span> <a href="javascript:void(0)" onclick="loadFiles('${escapeJsString(currentPath)}')" style="color: #60a5fa; text-decoration: none;">${escapeHtml(p)}</a>`;
  });

  el.innerHTML = html;
}

function navigateUpFileDir() {
  if (currentDirPath === "/" || currentDirPath === "") return;
  const idx = currentDirPath.lastIndexOf("/");
  if (idx <= 0) {
    loadFiles("/");
  } else {
    loadFiles(currentDirPath.substring(0, idx));
  }
}

async function openFileEditor(filePath) {
  document.getElementById("editor-filepath").textContent = filePath;
  document.getElementById("editor-content").value = "Đang tải nội dung file...";
  openModal("modal-file-editor");

  try {
    const res = await fetch(`/api/files/read?path=${encodeURIComponent(filePath)}`);
    const data = await res.json();
    if (!res.ok) {
      document.getElementById("editor-content").value = `⚠️ KHÔNG THỂ MỞ TỆP NÀY VÌ:\n${data.error || "Không thể đọc tệp"}\n\n💡 Gợi ý: Trình chỉnh sửa văn bản Web chỉ hỗ trợ mở và sửa các tệp văn bản (Text files như .env, .json, .sh, .txt, .yml, .go,...).\nĐối với tệp Nhị phân (Binary) hoặc tệp nén, vui lòng sử dụng nút "⬇️ Tải" để tải về máy.`;
      return;
    }
    document.getElementById("editor-content").value = data.content || "";
  } catch (err) {
    document.getElementById("editor-content").value = "⚠️ Lỗi kết nối: " + err.message;
  }
}

async function saveFileEditor() {
  const filePath = document.getElementById("editor-filepath").textContent;
  const content = document.getElementById("editor-content").value;

  if (!filePath) return;

  try {
    const res = await fetch("/api/files/save", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ path: filePath, content: content })
    });
    const data = await res.json();
    if (!res.ok) {
      alert("⚠️ Lỗi lưu tệp: " + (data.error || "Thất bại"));
      return;
    }
    alert("✅ Đã lưu tệp thành công!");
    closeModal("modal-file-editor");
    loadFiles();
  } catch (err) {
    alert("⚠️ Lỗi kết nối: " + err.message);
  }
}

async function promptCreateFileItem(isDir) {
  const label = isDir ? "thư mục" : "tệp mới";
  const name = prompt(`Nhập tên ${label} cần tạo trong ${currentDirPath}:`);
  if (!name || !name.trim()) return;

  const fullPath = currentDirPath.endsWith("/") ? currentDirPath + name.trim() : currentDirPath + "/" + name.trim();

  try {
    const res = await fetch("/api/files/create", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ path: fullPath, is_dir: isDir })
    });
    const data = await res.json();
    if (!res.ok) {
      alert(`⚠️ Lỗi tạo ${label}: ` + (data.error || "Thất bại"));
      return;
    }
    loadFiles();
  } catch (err) {
    alert("⚠️ Lỗi kết nối: " + err.message);
  }
}

async function deleteFile(filePath) {
  if (!confirm(`Bạn có chắc chắn muốn xóa "${filePath}" không?\nHành động này không thể hoàn tác!`)) return;

  try {
    const res = await fetch("/api/files/delete", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ path: filePath })
    });
    const data = await res.json();
    if (!res.ok) {
      alert("⚠️ Lỗi xóa: " + (data.error || "Thất bại"));
      return;
    }
    loadFiles();
  } catch (err) {
    alert("⚠️ Lỗi kết nối: " + err.message);
  }
}

function downloadFile(filePath) {
  window.open(`/api/files/download?path=${encodeURIComponent(filePath)}`, "_blank");
}

function triggerFileUpload() {
  document.getElementById("file-upload-input").click();
}

async function handleFileUploadSubmit(input) {
  if (!input.files || input.files.length === 0) return;
  const file = input.files[0];

  const formData = new FormData();
  formData.append("file", file);
  formData.append("dir", currentDirPath);

  try {
    const res = await fetch("/api/files/upload", {
      method: "POST",
      body: formData
    });
    const data = await res.json();
    if (!res.ok) {
      alert("⚠️ Lỗi tải file lên: " + (data.error || "Thất bại"));
      return;
    }
    alert(`✅ Đã tải file "${file.name}" lên thành công!`);
    input.value = "";
    loadFiles();
  } catch (err) {
    alert("⚠️ Lỗi kết nối: " + err.message);
  }
}

function getFileIcon(filename) {
  if (!filename) return '📄';
  const ext = filename.split('.').pop().toLowerCase();
  switch (ext) {
    case 'json': case 'yml': case 'yaml': case 'env': case 'conf': case 'ini': return '⚙️';
    case 'go': case 'js': case 'py': case 'sh': case 'html': case 'css': return '📜';
    case 'log': case 'txt': case 'md': return '📝';
    case 'png': case 'jpg': case 'jpeg': case 'svg': return '🖼️';
    case 'zip': case 'tar': case 'gz': return '📦';
    default: return '📄';
  }
}

async function loadDiskUsageAnalyzer(path, topN, userTriggered) {
  if (userTriggered) {
    toggleHddAnalyzer(true);
  }

  const pathInput = document.getElementById("du-path-input");
  const topSelect = document.getElementById("du-top-select");
  const tbody = document.getElementById("tbody-disk-usage");

  const targetPath = path !== undefined ? path : (pathInput ? pathInput.value.trim() : "/");
  const targetTop = topN !== undefined ? topN : (topSelect ? parseInt(topSelect.value, 10) : 15);

  if (pathInput && path !== undefined) pathInput.value = targetPath;

  if (tbody) {
    tbody.innerHTML = `<tr><td colspan="7" style="text-align:center;">⏳ Đang thực thi Go Goroutines song song quét dung lượng tại <code>${escapeHtml(targetPath)}</code>...</td></tr>`;
  }

  try {
    const res = await fetch(`/api/files/disk-usage?path=${encodeURIComponent(targetPath)}&top=${targetTop}`);
    if (!res.ok) {
      const errData = await res.json();
      if (tbody) tbody.innerHTML = `<tr><td colspan="7" style="text-align:center; color: var(--accent-red);">⚠️ Lỗi quét dung lượng: ${escapeHtml(errData.error || "Không thể quét")}</td></tr>`;
      return;
    }

    const data = await res.json();

    // Update Quick Metrics
    document.getElementById("du-total-disk").textContent = data.total_disk_gb ? `${data.total_disk_gb.toFixed(1)} GB` : "--";
    document.getElementById("du-used-disk").textContent = data.used_disk_gb ? `${data.used_disk_gb.toFixed(1)} GB (${data.disk_percent.toFixed(1)}%)` : "--";
    document.getElementById("du-free-disk").textContent = data.free_disk_gb ? `${data.free_disk_gb.toFixed(1)} GB` : "--";
    document.getElementById("du-scanned-disk").textContent = data.total_scanned_gb !== undefined ? `${data.total_scanned_gb.toFixed(2)} GB` : "--";
    document.getElementById("du-scan-time").textContent = `${data.scan_time_ms || 0} ms`;

    const topFolders = data.top_folders || [];

    // Render Table
    if (tbody) {
      if (topFolders.length === 0) {
        tbody.innerHTML = `<tr><td colspan="7" style="text-align:center; color: var(--text-muted);">Không tìm thấy thư mục nào</td></tr>`;
      } else {
        let html = "";
        topFolders.forEach((item, idx) => {
          const rank = idx + 1;
          const badgeClass = rank === 1 ? 'badge-danger' : (rank <= 3 ? 'badge-primary' : 'badge-secondary');
          const pct = item.percent ? item.percent.toFixed(1) : "0.0";
          const countsStr = `${item.file_count.toLocaleString("vi-VN")} tệp / ${item.dir_count.toLocaleString("vi-VN")} thư mục`;

          html += `
            <tr>
              <td style="text-align: center;"><span class="badge ${badgeClass}" style="padding: 4px 8px; font-weight: bold;">#${rank}</span></td>
              <td style="font-weight: 600; color: #60a5fa;">📁 ${escapeHtml(item.name)}</td>
              <td style="font-family: monospace; font-size: 0.8rem; color: var(--text-muted);">${escapeHtml(item.path)}</td>
              <td style="font-weight: 700; color: var(--accent-amber);">${escapeHtml(item.formatted_size)}</td>
              <td style="font-size: 0.8rem; color: var(--text-muted);">${countsStr}</td>
              <td>
                <div style="display: flex; align-items: center; gap: 8px;">
                  <div class="progress-bar" style="flex: 1; height: 8px; background: rgba(255,255,255,0.1); border-radius: 4px; overflow: hidden;">
                    <div style="width: ${Math.min(pct, 100)}%; height: 100%; background: linear-gradient(90deg, #38bdf8, #f59e0b); border-radius: 4px;"></div>
                  </div>
                  <span style="font-size: 0.8rem; font-weight: 600; min-width: 45px;">${pct}%</span>
                </div>
              </td>
              <td>
                ${item.name.startsWith("[") 
                  ? '-' 
                  : `<button class="btn btn-primary" onclick="navigateToFolderFromDU('${escapeJsString(item.path)}')" style="padding: 4px 10px; font-size: 0.75rem;">📂 Mở thư mục</button>`
                }
              </td>
            </tr>
          `;
        });
        tbody.innerHTML = html;
      }
    }

    // Render Chart.js
    renderDiskCharts(topFolders);

  } catch (err) {
    if (tbody) tbody.innerHTML = `<tr><td colspan="7" style="text-align:center; color: var(--accent-red);">⚠️ Lỗi kết nối: ${escapeHtml(err.message)}</td></tr>`;
  }
}

function navigateToFolderFromDU(path) {
  loadFiles(path);
  const fileTable = document.getElementById("tbody-files");
  if (fileTable) {
    fileTable.scrollIntoView({ behavior: 'smooth' });
  }
}

function renderDiskCharts(folders) {
  const barCtx = document.getElementById("chart-disk-bar");
  const pieCtx = document.getElementById("chart-disk-pie");
  if (!barCtx || !pieCtx) return;

  const labels = folders.map(f => f.name.length > 18 ? f.name.substring(0, 15) + '...' : f.name);
  const sizesMB = folders.map(f => (f.size_bytes / (1024 * 1024)).toFixed(1));

  const colors = [
    '#ef4444', '#f97316', '#f59e0b', '#10b981', '#06b6d4', 
    '#3b82f6', '#8b5cf6', '#ec4899', '#64748b', '#a855f7',
    '#14b8a6', '#0284c7', '#4f46e5', '#c026d3', '#e11d48'
  ];

  if (diskBarChart) diskBarChart.destroy();
  if (diskPieChart) diskPieChart.destroy();

  diskBarChart = new Chart(barCtx, {
    type: 'bar',
    data: {
      labels: labels,
      datasets: [{
        label: 'Dung lượng (MB)',
        data: sizesMB,
        backgroundColor: colors.slice(0, folders.length),
        borderRadius: 6
      }]
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      plugins: {
        legend: { display: false },
        title: { display: true, text: 'Top Thư Mục Chiếm Dung Lượng HDD (MB)', color: '#f0f6fc', font: { size: 14, weight: 'bold' } }
      },
      scales: {
        x: { ticks: { color: '#8b949e', font: { size: 11 } }, grid: { color: 'rgba(255,255,255,0.05)' } },
        y: { ticks: { color: '#8b949e' }, grid: { color: 'rgba(255,255,255,0.05)' } }
      }
    }
  });

  diskPieChart = new Chart(pieCtx, {
    type: 'doughnut',
    data: {
      labels: labels,
      datasets: [{
        data: sizesMB,
        backgroundColor: colors.slice(0, folders.length),
        borderWidth: 1,
        borderColor: '#0f172a'
      }]
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      plugins: {
        legend: { position: 'right', labels: { color: '#8b949e', boxWidth: 12, font: { size: 10 } } },
        title: { display: true, text: 'Phân Bổ Thư Mục (%)', color: '#f0f6fc', font: { size: 13 } }
      }
    }
  });
}

function toggleHddAnalyzer(show) {
  const body = document.getElementById("hdd-analyzer-body");
  const icon = document.getElementById("hdd-analyzer-toggle-icon");
  const text = document.getElementById("hdd-analyzer-toggle-text");
  if (!body) return;

  const isCurrentlyHidden = body.style.display === "none";
  const shouldShow = show !== undefined ? show : isCurrentlyHidden;

  if (shouldShow) {
    body.style.display = "block";
    if (icon) icon.textContent = "🙈";
    if (text) text.textContent = "Ẩn Analyzer";
    localStorage.setItem("hdd_analyzer_hidden", "false");

    if (!diskBarChart && document.getElementById("chart-disk-bar")) {
      loadDiskUsageAnalyzer(currentDirPath, 15);
    }

    setTimeout(() => {
      if (diskBarChart) diskBarChart.resize();
      if (diskPieChart) diskPieChart.resize();
    }, 50);
  } else {
    body.style.display = "none";
    if (icon) icon.textContent = "👁️";
    if (text) text.textContent = "Hiện Analyzer";
    localStorage.setItem("hdd_analyzer_hidden", "true");
  }
}

function initHddAnalyzerToggle() {
  const isExplicitlyShown = localStorage.getItem("hdd_analyzer_hidden") === "false";
  if (isExplicitlyShown) {
    toggleHddAnalyzer(true);
  } else {
    toggleHddAnalyzer(false);
  }
}

// Bind to window to guarantee global availability
window.toggleHddAnalyzer = toggleHddAnalyzer;
window.initHddAnalyzerToggle = initHddAnalyzerToggle;
window.loadDiskUsageAnalyzer = loadDiskUsageAnalyzer;

document.addEventListener("DOMContentLoaded", () => {
  initHddAnalyzerToggle();
});

