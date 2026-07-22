async function loadNetworkPorts() {
  try {
    await Promise.all([loadHostPorts(), loadDockerNetworks()]);
  } catch (err) {
    console.error("Lỗi loadNetworkPorts:", err);
  }
}

async function loadHostPorts() {
  const tbody = document.getElementById("tbody-ports");
  if (!tbody) return;

  tbody.innerHTML = `<tr><td colspan="6" style="text-align:center;">⏳ Đang quét danh sách Cổng Listening trên Host...</td></tr>`;

  try {
    const res = await fetch("/api/system/ports");
    if (!res.ok) {
      const errData = await res.json().catch(() => ({}));
      tbody.innerHTML = `<tr><td colspan="6" style="text-align:center; color: var(--danger);">⚠️ Lỗi: ${escapeHtml(errData.error || "Không thể lấy danh sách cổng")}</td></tr>`;
      return;
    }
    const ports = await res.json();

    if (!ports || !Array.isArray(ports) || ports.length === 0) {
      tbody.innerHTML = `<tr><td colspan="6" style="text-align:center; color: var(--text-muted);">Không tìm thấy cổng listening nào.</td></tr>`;
      return;
    }

    let html = "";
    ports.forEach(p => {
      const proto = String(p.protocol || 'tcp').toLowerCase();
      const protoBadge = proto.startsWith("tcp") ? "badge-primary" : "badge-secondary";
      const procName = p.process_name || 'unknown';
      const procInfo = p.pid > 0 
        ? `<strong>${escapeHtml(procName)}</strong> <span style="color: var(--text-muted); font-size: 0.8rem;">(PID ${p.pid})</span>`
        : `<span style="color: var(--text-muted);">Hệ thống / Root</span>`;

      html += `
        <tr>
          <td><span class="badge ${protoBadge}">${escapeHtml(proto.toUpperCase())}</span></td>
          <td style="font-weight: 600; font-family: monospace; font-size: 1rem; color: #38bdf8;">:${p.local_port || 0}</td>
          <td style="font-family: monospace; font-size: 0.9rem;">${escapeHtml(p.local_ip || '0.0.0.0')}</td>
          <td><span class="badge badge-success">${escapeHtml(p.state || 'LISTEN')}</span></td>
          <td>${procInfo}</td>
          <td>
            ${p.pid > 0 
              ? `<button class="btn btn-danger" onclick="killPortProcess(${p.pid}, '${escapeJsString(procName)}')" style="padding: 4px 8px; font-size: 0.75rem;">⚡ Kill PID ${p.pid}</button>`
              : `-`
            }
          </td>
        </tr>
      `;
    });

    tbody.innerHTML = html;
  } catch (err) {
    console.error("Lỗi loadHostPorts:", err);
    tbody.innerHTML = `<tr><td colspan="6" style="text-align:center; color: var(--danger);">⚠️ Lỗi kết nối: ${escapeHtml(err.message)}</td></tr>`;
  }
}

async function killPortProcess(pid, name) {
  if (!confirm(`Bạn có chắc chắn muốn Kill tiến trình "${name}" (PID: ${pid}) đang chiếm cổng không?`)) return;

  try {
    const res = await fetch("/api/system/processes/kill", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ pid: pid })
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok) {
      alert("⚠️ Lỗi Kill tiến trình: " + (data.error || "Thất bại"));
      return;
    }
    alert(`✅ Đã dừng tiến trình PID ${pid}!`);
    loadHostPorts();
  } catch (err) {
    alert("⚠️ Lỗi kết nối: " + err.message);
  }
}

async function loadDockerNetworks() {
  const tbody = document.getElementById("tbody-networks");
  if (!tbody) return;

  tbody.innerHTML = `<tr><td colspan="6" style="text-align:center;">⏳ Đang tải danh sách Mạng Docker...</td></tr>`;

  try {
    const res = await fetch("/api/networks");
    if (!res.ok) {
      const errData = await res.json().catch(() => ({}));
      tbody.innerHTML = `<tr><td colspan="6" style="text-align:center; color: var(--danger);">⚠️ Lỗi: ${escapeHtml(errData.error || "Không thể tải danh sách mạng")}</td></tr>`;
      return;
    }
    const networks = await res.json();

    if (!networks || !Array.isArray(networks) || networks.length === 0) {
      tbody.innerHTML = `<tr><td colspan="6" style="text-align:center; color: var(--text-muted);">Không có mạng Docker nào.</td></tr>`;
      return;
    }

    let html = "";
    networks.forEach(net => {
      const name = net.name || "";
      const isSystemNet = ["bridge", "host", "none"].includes(name);
      const ctrKeys = Object.keys(net.containers || {});
      
      let ctrsHtml = `<span style="color: var(--text-muted); font-size: 0.85rem;">0 container</span>`;
      if (ctrKeys.length > 0) {
        const ctrBadges = ctrKeys.map(k => `<span class="badge badge-secondary" style="margin: 2px;">📦 ${escapeHtml(net.containers[k])}</span>`).join(" ");
        ctrsHtml = `<div style="display: flex; flex-wrap: wrap; gap: 4px;">${ctrBadges}</div>`;
      }

      html += `
        <tr>
          <td style="font-weight: 600; color: #f8fafc;">
            🌐 ${escapeHtml(name)}
            ${isSystemNet ? `<span class="badge badge-secondary" style="font-size: 0.7rem; margin-left: 6px;">System</span>` : ''}
          </td>
          <td style="font-family: monospace; font-size: 0.85rem;">${escapeHtml(net.short_id || '')}</td>
          <td><span class="badge badge-primary">${escapeHtml(net.driver || 'bridge')}</span></td>
          <td>${escapeHtml(net.scope || 'local')}</td>
          <td>${ctrsHtml}</td>
          <td>
            ${isSystemNet 
              ? `<span style="color: var(--text-muted); font-size: 0.8rem;">Protected</span>`
              : `<button class="btn btn-danger" onclick="deleteDockerNetwork('${escapeJsString(net.id)}', '${escapeJsString(name)}')" style="padding: 4px 8px; font-size: 0.75rem;">🗑️ Xóa Mạng</button>`
            }
          </td>
        </tr>
      `;
    });

    tbody.innerHTML = html;
  } catch (err) {
    console.error("Lỗi loadDockerNetworks:", err);
    tbody.innerHTML = `<tr><td colspan="6" style="text-align:center; color: var(--danger);">⚠️ Lỗi kết nối: ${escapeHtml(err.message)}</td></tr>`;
  }
}

async function promptCreateDockerNetwork() {
  const name = prompt("Nhập tên Mạng Docker mới cần tạo (ví dụ: my-custom-net):");
  if (!name || !name.trim()) return;

  const driver = prompt("Nhập driver mạng (bridge / overlay / macvlan):", "bridge");
  if (!driver) return;

  try {
    const res = await fetch("/api/networks/create", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name: name.trim(), driver: driver.trim() })
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok) {
      alert("⚠️ Lỗi tạo Mạng Docker: " + (data.error || "Thất bại"));
      return;
    }
    alert(`✅ Đã tạo thành công Mạng Docker "${name.trim()}"!`);
    loadDockerNetworks();
  } catch (err) {
    alert("⚠️ Lỗi kết nối: " + err.message);
  }
}

async function deleteDockerNetwork(id, name) {
  if (!confirm(`Bạn có chắc chắn muốn xóa Mạng Docker "${name}" không?`)) return;

  try {
    const res = await fetch("/api/networks/remove", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ id: id })
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok) {
      alert("⚠️ Lỗi xóa Mạng Docker: " + (data.error || "Thất bại"));
      return;
    }
    alert(`✅ Đã xóa Mạng Docker "${name}"!`);
    loadDockerNetworks();
  } catch (err) {
    alert("⚠️ Lỗi kết nối: " + err.message);
  }
}
