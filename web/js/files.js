let currentDirPath = "/root";

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
